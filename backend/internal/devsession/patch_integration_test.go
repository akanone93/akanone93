// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/icons"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// calculator is the page of the conformance project the tests patch.
const calculator = "plugins/loans/pages/calculator.page.json"

// fixtureDir is the conformance project (Appendix A).
var fixtureDir = filepath.Join("..", "..", "..", "schema", "testdata", "documents", "loan-calculator")

// drafts serves the conformance project as the environment's draft.
type drafts struct{ t *testing.T }

func (d drafts) DraftFiles(context.Context, pgx.Tx, string) (fstest.MapFS, error) {
	m := fstest.MapFS{}
	root := os.DirFS(fixtureDir)
	err := fs.WalkDir(root, ".", func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := fs.ReadFile(root, p)
		m[p] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		d.t.Fatal(err)
	}
	return m, nil
}

// fileSigner signs with the world's file backend and the environment's key.
type fileSigner struct{ f *fixture }

func (g fileSigner) SignDocument(ctx context.Context, _ *dbgen.Queries, env dbgen.Environment, canonical []byte) ([]release.ManifestSignature, error) {
	sig, id, err := g.f.Signer.Sign(ctx, env.SigningKeyRef, canonical)
	if err != nil {
		return nil, err //nolint:wrapcheck // a test
	}
	return []release.ManifestSignature{{KeyID: id, Algorithm: "ed25519", Signature: base64.StdEncoding.EncodeToString(sig)}}, nil
}

// hub records the frames and fails the devices it is told to.
type hub struct {
	mu     sync.Mutex
	frames map[string][]*pluxv1.DevServerFrame
	fail   map[string]error
}

func (h *hub) Deliver(_, deviceID string, frame *pluxv1.DevServerFrame) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.fail[deviceID]; err != nil {
		return err
	}
	if h.frames == nil {
		h.frames = map[string][]*pluxv1.DevServerFrame{}
	}
	h.frames[deviceID] = append(h.frames[deviceID], frame)
	return nil
}

// pushFixture is a fixture whose service can push patches.
type pushFixture struct {
	*fixture
	hub *hub
	// opts are the service's options, for a second replica.
	opts devsession.Options
}

func newPushFixture(t *testing.T, patchBytes int64) *pushFixture {
	t.Helper()
	tight := map[limits.Key]int64{}
	if patchBytes != 0 {
		tight[limits.DevSessionPatchBytes] = patchBytes
	}
	return newPushFixtureWith(t, tight)
}

// newPushFixtureWith is newPushFixture with the given limits tightened.
func newPushFixtureWith(t *testing.T, tight map[limits.Key]int64) *pushFixture {
	t.Helper()
	f := newFixture(t, 0)
	h := &hub{fail: map[string]error{}}
	lim := func(ctx context.Context, tx pgx.Tx, org, _ string) (limits.Set, error) {
		set, err := f.Limits(ctx, tx, org)
		for k, v := range tight {
			if err != nil {
				break
			}
			set, err = set.Tighten(k, limits.ScopeOrganization, v)
		}
		return set, err //nolint:wrapcheck // a test
	}
	o := devsession.Options{
		DB: f.DB, Audit: f.Log, IDs: f.Gen, Limits: lim, Tokens: f.signer, Environments: []string{"development"},
		PublicBaseURL: "https://plux.example.com", Now: f.Clock,
		Drafts: drafts{t}, Signer: fileSigner{f}, Deliverer: h,
		CompileOptions: func(context.Context, pgx.Tx, string, string, string) (compiler.Options, error) {
			co := compiler.DefaultOptions()
			co.IconFont = func(set icons.Set, names []string) ([]byte, error) {
				return fmt.Appendf(nil, "font %s %v", set, names), nil
			}
			return co, nil
		},
	}
	svc, err := devsession.New(o)
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	return &pushFixture{fixture: f, hub: h, opts: o}
}

// pairDevices pairs n devices with the session and returns their IDs.
func (f *pushFixture) pairDevices(t *testing.T, s devsession.Session, n int) []string {
	t.Helper()
	var ids []string
	for i := range n {
		p, err := f.svc.GetPairing(context.Background(), f.Developer, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		res, err := f.pair(f.device(fmt.Sprintf("device-%d", i), fmt.Sprintf("jkt-%d", i)), p.Token)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, res.DeviceID)
	}
	return ids
}

func (f *pushFixture) push(s devsession.Session, patches ...devsession.DocumentPatch) (devsession.PushResult, error) {
	return f.svc.PushPatch(context.Background(), f.Developer, s.ID, patches)
}

func replace(path string, value any) devsession.DocumentPatch {
	return devsession.DocumentPatch{Path: calculator, Ops: []document.Op{{Op: "replace", Path: path, Value: value}}}
}

// envelope is the manifest of a frame as the device reads it.
type envelope struct {
	Signed     []byte `json:"signed"`
	Signatures []struct {
		KeyID string `json:"keyid"`
		Alg   string `json:"alg"`
		Sig   string `json:"sig"`
	} `json:"signatures"`
}

// manifest decodes and verifies the manifest of a frame with the public key
// of the development environment.
func (f *pushFixture) manifest(t *testing.T, b *pluxv1.DevBundleFrame) map[string]any {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(b.GetManifest(), &env); err != nil || len(env.Signatures) != 1 {
		t.Fatalf("envelope: %v, %d signatures", err, len(env.Signatures))
	}
	var ref string
	err := f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetEnvironment(ctx, storage.MustUUID(f.Envs["development"]))
		ref = row.SigningKeyRef
		return err //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}
	pub, id, err := f.Signer.PublicKey(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(env.Signatures[0].Sig)
	if err != nil || env.Signatures[0].KeyID != id || !ed25519.Verify(pub, env.Signed, raw) {
		t.Fatalf("the manifest does not verify with the environment's key %s", id)
	}
	var doc map[string]any
	if err := json.Unmarshal(env.Signed, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// frame returns the n-th bundle frame a device received.
func (f *pushFixture) frame(t *testing.T, device string, n int) *pluxv1.DevBundleFrame {
	t.Helper()
	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	if len(f.hub.frames[device]) <= n {
		t.Fatalf("device %s received %d frames, want more than %d", device, len(f.hub.frames[device]), n)
	}
	return f.hub.frames[device][n].GetBundle()
}

// Verifies: DEV-010, CMP-051, SEC-056.
// A patch produces a frame whose manifest verifies with the development key
// of the environment, names the session and carries every bundle's section.
func TestPushPatch_SignsADevelopmentBundle_DEV_010(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _ := f.open(t)
	devices := f.pairDevices(t, s, 2)
	res, err := f.push(s, replace("/state/1/default", json.Number("24")))
	if err != nil {
		t.Fatal(err)
	}
	if res.BundleSequence != 1 || res.BundleSHA256 == "" || len(res.Results) != 2 || res.Timings.Compile <= 0 || res.Timings.Sign <= 0 {
		t.Fatalf("result = %+v", res)
	}
	for _, r := range res.Results {
		if r.Status != pluxv1.DevPatchStatus_DEV_PATCH_STATUS_APPLIED {
			t.Errorf("device %s: %v", r.DeviceId, r.Status)
		}
	}
	b := f.frame(t, devices[0], 0)
	doc := f.manifest(t, b)
	ds, _ := doc["developmentSession"].(map[string]any)
	if ds["sessionId"] != s.ID || ds["sequenceSpace"] != float64(1) || doc["releaseSequence"] != float64(1) || b.GetBundleSequence() != 1 {
		t.Errorf("manifest = %v", doc)
	}
	if doc["environment"] != "development" || doc["app"] != f.App {
		t.Errorf("manifest names %v / %v", doc["app"], doc["environment"])
	}
	if len(b.GetSections()) != 2 {
		t.Fatalf("the first push carries %d bundles, want the app's and the plugin's", len(b.GetSections()))
	}
	named := map[string]bool{doc["appBundle"].(map[string]any)["hash"].(string): true}
	for _, p := range doc["plugins"].([]any) {
		named[p.(map[string]any)["hash"].(string)] = true
	}
	for ref, data := range b.GetSections() {
		if !named[ref] || len(data) == 0 {
			t.Errorf("section %s is not a bundle the manifest names", ref)
		}
	}
	if rows := f.Audit(t, "devsession.patched"); len(rows) != 1 || rows[0].TargetID != s.ID {
		t.Errorf("audit = %+v", rows)
	}
	if other := f.frame(t, devices[1], 0); other == b {
		t.Error("the devices share one frame")
	}
}

// Verifies: DEV-010, CMP-051.
// The second patch is bundle 2 and carries only the bundles that changed.
func TestPushPatch_SecondPatchSendsChangedBundles_CMP_051(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _ := f.open(t)
	devices := f.pairDevices(t, s, 1)
	if _, err := f.push(s, replace("/state/1/default", json.Number("24"))); err != nil {
		t.Fatal(err)
	}
	res, err := f.push(s, replace("/state/1/default", json.Number("36")))
	if err != nil {
		t.Fatal(err)
	}
	if res.BundleSequence != 2 || res.Full || len(res.Recompiled) == 0 {
		t.Fatalf("result = %+v", res)
	}
	b := f.frame(t, devices[0], 1)
	doc := f.manifest(t, b)
	if doc["releaseSequence"] != float64(2) || b.GetBundleSequence() != 2 {
		t.Errorf("manifest = %v", doc)
	}
	if len(b.GetSections()) != 1 {
		t.Fatalf("the second push carries %d bundles, want the plugin's only", len(b.GetSections()))
	}
	plugins, _ := doc["plugins"].([]any)
	if len(plugins) != 1 {
		t.Fatalf("plugins = %v", plugins)
	}
	hash := plugins[0].(map[string]any)["hash"].(string)
	if _, ok := b.GetSections()[hash]; !ok {
		t.Errorf("the carried bundle is not the plugin's %s", hash)
	}
}

// Verifies: DEV-010.
// A change of a page's state schema is reported as incompatible scopes of
// that page, and the devices are told they reset.
func TestPushPatch_StateSchemaChangeIsIncompatible_DEV_010(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _ := f.open(t)
	devices := f.pairDevices(t, s, 1)
	if _, err := f.push(s, replace("/state/1/default", json.Number("24"))); err != nil {
		t.Fatal(err)
	}
	res, err := f.push(s, devsession.DocumentPatch{Path: calculator, Ops: []document.Op{{Op: "add", Path: "/state/1/persistence", Value: "session"}}})
	if err != nil {
		t.Fatal(err)
	}
	scopes := f.frame(t, devices[0], 1).GetIncompatibleScopes()
	if len(scopes) != 1 || !slices.Equal(scopes["01a0c450-6c00-7012-8000-000000022cce"].GetScopes(), []string{"page.months"}) {
		t.Fatalf("incompatible scopes = %v", scopes)
	}
	if r := res.Results[0]; r.Status != pluxv1.DevPatchStatus_DEV_PATCH_STATUS_APPLIED_WITH_RESET || !slices.Equal(r.ScopesReset, []string{"page.months"}) {
		t.Errorf("result = %v", r)
	}
}

// Verifies: DEV-010.
// A patch that does not compile is refused with PLX-9207 and diagnostics,
// nothing is delivered, and the draft stays as it was.
func TestPushPatch_CompileErrorDeliversNothing_DEV_010(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _ := f.open(t)
	devices := f.pairDevices(t, s, 1)
	res, err := f.push(s, devsession.DocumentPatch{Path: calculator, Ops: []document.Op{{Op: "add", Path: "/bogus", Value: json.Number("1")}}})
	wantCode(t, err, plxerr.DevPatchRejected, "a patch that breaks the draft")
	if len(res.Diagnostics) < 2 || res.Diagnostics[0].Code != plxerr.DevPatchRejected || !res.Diagnostics[1:].HasErrors() {
		t.Errorf("diagnostics = %v", res.Diagnostics)
	}
	f.hub.mu.Lock()
	n := len(f.hub.frames[devices[0]])
	f.hub.mu.Unlock()
	if n != 0 || len(f.Audit(t, "devsession.patched")) != 0 {
		t.Errorf("a refused patch delivered %d frames", n)
	}
	res, err = f.push(s, replace("/state/1/default", json.Number("24")))
	if err != nil || res.BundleSequence != 1 {
		t.Fatalf("the next patch: %+v, %v", res, err)
	}
}

// Verifies: DEV-010.
// A patch that does not apply, or names a document the draft lacks, is
// refused with PLX-9207; so is one that read an older revision.
func TestPushPatch_RefusesPatchesThatDoNotApply_DEV_010(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _ := f.open(t)
	f.pairDevices(t, s, 1)
	_, err := f.push(s, replace("/nowhere/at/all", json.Number("1")))
	wantCode(t, err, plxerr.DevPatchRejected, "an operation that does not apply")
	_, err = f.push(s, devsession.DocumentPatch{Path: "plugins/loans/pages/missing.page.json", Ops: []document.Op{{Op: "add", Path: "/x", Value: json.Number("1")}}})
	wantCode(t, err, plxerr.DevPatchRejected, "a missing document")
	_, err = f.push(s, devsession.DocumentPatch{Path: "assets/images/logo.png", Ops: []document.Op{{Op: "add", Path: "/x", Value: json.Number("1")}}})
	wantCode(t, err, plxerr.DevPatchRejected, "a file that is not a document")
	stale := replace("/state/1/default", json.Number("24"))
	stale.IfRevision = 7
	_, err = f.push(s, stale)
	wantCode(t, err, plxerr.DevPatchRejected, "a stale revision")
}

// Verifies: DEV-010.
// Patches above devSession.patchBytes are refused whole.
func TestPushPatch_RefusesOversizePatch_DEV_010(t *testing.T) {
	f := newPushFixture(t, 64)
	s, _ := f.open(t)
	devices := f.pairDevices(t, s, 1)
	_, err := f.push(s, replace("/state/1/default", "a value well over the sixty-four bytes the limit allows in one push of patches"))
	wantCode(t, err, plxerr.LimitExceeded, "an oversize patch")
	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	if len(f.hub.frames[devices[0]]) != 0 {
		t.Error("an oversize patch was delivered")
	}
}

// Verifies: DEV-003, SEC-056.
// A session in a production environment, which cannot exist through
// CreateSession, still pushes nothing and signs nothing (PLX-9203).
func TestPushPatch_RefusesProduction_DEV_003(t *testing.T) {
	f := newPushFixture(t, 0)
	id := f.NewID(t)
	err := f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		now := storage.Timestamp(f.Now)
		_, err := dbgen.New(tx).InsertDevSession(ctx, dbgen.InsertDevSessionParams{
			ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(f.Org), AppID: storage.MustUUID(f.App),
			EnvironmentID: storage.MustUUID(f.Envs["production"]), EnvironmentKey: "production",
			CreatedByUser: f.Developer.UserID, CreatedByKind: "user", CreatedByID: f.Developer.UserID, CreatedBy: "dev",
			CreatedAt: now, ExpiresAt: storage.Timestamp(f.Now.Add(time.Hour)), Kind: "cli",
		})
		return err //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.PushPatch(context.Background(), f.Developer, id, []devsession.DocumentPatch{replace("/state/1/default", json.Number("24"))})
	wantCode(t, err, plxerr.DevSessionEnvironment, "a production environment")
	if len(f.Audit(t, "devsession.patched")) != 0 {
		t.Error("a refused push was audited as a push")
	}
}

// Verifies: DEV-010.
// A delivery that fails is reported for that device only: an offline device
// as offline, any other failure as refused without the cause's text.
func TestPushPatch_ReportsDeliveryPerDevice_DEV_010(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _ := f.open(t)
	devices := f.pairDevices(t, s, 3)
	f.hub.fail[devices[0]] = devsession.ErrDeviceOffline
	f.hub.fail[devices[1]] = errors.New("write tcp 10.0.0.1: broken pipe")
	res, err := f.push(s, replace("/state/1/default", json.Number("24")))
	if err != nil {
		t.Fatal(err)
	}
	want := []pluxv1.DevPatchStatus{
		pluxv1.DevPatchStatus_DEV_PATCH_STATUS_OFFLINE, pluxv1.DevPatchStatus_DEV_PATCH_STATUS_REFUSED, pluxv1.DevPatchStatus_DEV_PATCH_STATUS_APPLIED,
	}
	if len(res.Results) != 3 {
		t.Fatalf("results = %v", res.Results)
	}
	for i, r := range res.Results {
		if r.DeviceId != devices[i] || r.Status != want[i] {
			t.Errorf("result %d = %v, want %s %v", i, r, devices[i], want[i])
		}
	}
	if e := res.Results[1].Error; e == nil || e.Code != plxerr.DevPatchRejected.String() || e.Message == f.hub.fail[devices[1]].Error() {
		t.Errorf("error = %v", e)
	}
}

// Verifies: DEV-010.
// Ending a session drops its draft state, and a push to it is refused.
func TestPushPatch_EndedSessionIsRefused_DEV_010(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _ := f.open(t)
	f.pairDevices(t, s, 1)
	if _, err := f.push(s, replace("/state/1/default", json.Number("24"))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.EndSession(context.Background(), f.Developer, s.ID); err != nil {
		t.Fatal(err)
	}
	_, err := f.push(s, replace("/state/1/default", json.Number("36")))
	wantCode(t, err, plxerr.DevSessionExpired, "an ended session")
}

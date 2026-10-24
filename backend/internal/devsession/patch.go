// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/compiler/incremental"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// sequenceSpace is the number of the development sequence the bundles of a
// session belong to: the session's own, apart from every release sequence.
const sequenceSpace = 1

// defaultChannel is the channel a development manifest names unless the
// options say otherwise (REL-005).
const defaultChannel = "production"

// ErrDeviceOffline is what a Deliverer returns for a device whose socket is
// closed; the device receives the current draft when it reconnects.
var ErrDeviceOffline = errors.New("devsession: the device is not connected")

// Deliverer sends a frame to one paired device of a session (DEV-010). It
// must not retain or modify the frame after it returns; the service gives
// every device its own copy.
type Deliverer interface {
	Deliver(sessionID, deviceID string, frame *pluxv1.DevServerFrame) error
}

// DraftSource reads an app's current draft as the file set the compiler
// reads, in the transaction it is given. document.Service.DraftFiles is one.
type DraftSource interface {
	DraftFiles(ctx context.Context, tx pgx.Tx, appID string) (fstest.MapFS, error)
}

// CompileOptionsFunc returns the compiler options of an environment of an
// app; the service sets the development mode itself. release.Service's
// CompileOptions is one.
type CompileOptionsFunc func(ctx context.Context, tx pgx.Tx, organizationID, appID, environmentID string) (compiler.Options, error)

// ManifestSigner signs a canonical manifest with the environment's key and
// records the key's public half. release.Service is one; it is the only way
// a dev bundle is signed (SEC-056).
type ManifestSigner interface {
	SignDocument(ctx context.Context, q *dbgen.Queries, env dbgen.Environment, canonical []byte) ([]release.ManifestSignature, error)
}

var (
	_ DraftSource    = (*document.Service)(nil)
	_ ManifestSigner = (*release.Service)(nil)
)

// DocumentPatch is a JSON Patch to one document of the session's draft.
type DocumentPatch struct {
	// PluginID names the plugin the document belongs to, "" for the app's;
	// the document is found by Path, which holds the plugin's key.
	PluginID string
	// Path is the document's path in the project layout.
	Path string
	Ops  []document.Op
	// IfRevision is the revision of the document the writer last read, as
	// the session counts them; zero skips the check.
	IfRevision int64
}

// Timings is how long the stages of a push took (NFR-030).
type Timings struct {
	Compile, Sign, Deliver time.Duration
}

// PushResult is the outcome of a push.
type PushResult struct {
	// BundleSHA256 is the hash of the signed manifest that was pushed,
	// lower-case hex; the manifest names every bundle.
	BundleSHA256 string
	// BundleSequence is the bundle's number in the session's sequence.
	BundleSequence uint64
	// Results holds one entry per paired device.
	Results []*pluxv1.DevPatchResult
	// Diagnostics are the findings of compilation: the errors of a refused
	// patch, the warnings of an applied one.
	Diagnostics plxerr.Diagnostics
	// Full says the whole project had to be compiled, and Recompiled lists
	// the files that were recompiled otherwise (CMP-051).
	Full       bool
	Recompiled []string
	// Timings are the durations of compiling, signing and delivering.
	Timings Timings
}

// draftState is what a live session keeps: its draft, the incremental
// compiler's cache and the numbering of what it pushed. It lives as long as
// the session and is dropped when the session ends or expires.
type draftState struct {
	mu      sync.Mutex
	expires time.Time
	loaded  bool
	files   fstest.MapFS
	// revs counts the patches applied to each document, from one.
	revs     map[string]int64
	cache    *incremental.Cache
	sequence uint64
	// sent holds the bundle references the devices hold.
	sent map[string]bool
}

// CanPush reports whether this service has what pushing a patch needs.
func (s *Service) CanPush() bool {
	return s.o.Drafts != nil && s.o.CompileOptions != nil && s.o.Signer != nil && s.o.Deliverer != nil
}

// stateFor returns the state of a session, creating it, and drops the states
// of sessions that have expired.
func (s *Service) stateFor(id string, expires time.Time) *draftState {
	s.draftsMu.Lock()
	defer s.draftsMu.Unlock()
	now := s.now()
	for k, st := range s.drafts {
		if k != id && !now.Before(st.expires) {
			delete(s.drafts, k)
		}
	}
	st, ok := s.drafts[id]
	if !ok {
		st = &draftState{expires: expires}
		s.drafts[id] = st
	}
	return st
}

// forget drops the state of a session.
func (s *Service) forget(id string) {
	s.draftsMu.Lock()
	defer s.draftsMu.Unlock()
	delete(s.drafts, id)
}

// pushPlan is what the first transaction of a push reads.
type pushPlan struct {
	session Session
	env     dbgen.Environment
	set     limits.Set
	opts    compiler.Options
}

// PushPatch applies patches to the session's draft, compiles it
// incrementally, signs a development bundle with the key of the session's
// environment and delivers it to the paired devices (DEV-010, CMP-051). The
// patches apply all or none; a draft that does not compile is left as it
// was and nothing is delivered. Pushes to one session run one at a time.
func (s *Service) PushPatch(ctx context.Context, p auth.Principal, sessionID string, patches []DocumentPatch) (PushResult, error) {
	plan, err := s.admitPush(ctx, p, sessionID, patches)
	if err != nil {
		return PushResult{}, err
	}
	lim := relayLimitsOf(plan.set)
	lim.expires = plan.session.ExpiresAt
	s.hub.room(plan.session.ID, &lim)
	st := s.stateFor(plan.session.ID, plan.session.ExpiresAt)
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := s.load(ctx, p, st, plan); err != nil {
		return PushResult{}, err
	}
	files, revs, ids, err := applyPatches(st, patches, int(plan.set.Get(limits.DocumentJSONDepth)))
	if err != nil {
		return PushResult{}, err
	}
	started := time.Now()
	res, err := incremental.Compile(ctx, st.cache, incremental.Input{FS: files})
	if err != nil {
		return PushResult{}, fmt.Errorf("devsession: compile: %w", err)
	}
	out := PushResult{Diagnostics: res.Diagnostics, Full: res.Full, Recompiled: res.Recompiled}
	out.Timings.Compile = time.Since(started)
	if res.App == nil || res.Diagnostics.HasErrors() {
		out.Diagnostics = append(plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.DevPatchRejected, plxerr.Location{},
			"the patched draft does not compile; nothing was pushed")}, out.Diagnostics...)
		return out, plxerr.New(plxerr.DevPatchRejected, "the patched draft does not compile; nothing was pushed")
	}
	seq := st.sequence + 1
	bundles := bundlesOf(res.Result)
	doc := manifestOf(plan, files, res.Result, seq, s.now(), s.channel())
	signed, err := release.CanonicalManifest(doc)
	if err != nil {
		return out, fmt.Errorf("devsession: %w", err)
	}
	started = time.Now()
	manifest, devices, err := s.sign(ctx, p, plan, signed, seq, ids)
	out.Timings.Sign = time.Since(started)
	if err != nil {
		return out, err
	}
	st.files, st.revs, st.sequence = files, revs, seq
	frame, pages := bundleFrame(seq, manifest, bundles, st.sent, res.Pages)
	for ref := range bundles {
		st.sent[ref] = true
	}
	out.BundleSHA256, out.BundleSequence = digestOf(signed), seq
	started = time.Now()
	out.Results = s.deliver(plan.session.ID, devices, frame, pages)
	out.Timings.Deliver = time.Since(started)
	return out, nil
}

// digestOf is the lower-case hex SHA-256 of data.
func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// admitPush authorises a push and checks its size against the session's limits.
func (s *Service) admitPush(ctx context.Context, p auth.Principal, sessionID string, patches []DocumentPatch) (pushPlan, error) {
	if err := authorizeAnywhere(p, auth.DevSessionCreate); err != nil {
		return pushPlan{}, err
	}
	if !s.CanPush() {
		return pushPlan{}, errors.New("devsession: this server cannot push patches")
	}
	id, err := parseID(sessionID, "session")
	if err != nil {
		return pushPlan{}, err
	}
	if len(patches) == 0 {
		return pushPlan{}, plxerr.New(plxerr.MissingProperty, "a push names at least one patch")
	}
	plan, err := s.plan(ctx, p, id)
	if err != nil {
		return pushPlan{}, err
	}
	return plan, checkSize(plan.set, patches)
}

// channel is the channel the development manifests name.
func (s *Service) channel() string {
	if s.o.Channel != "" {
		return s.o.Channel
	}
	return defaultChannel
}

// plan authorises the push and reads what it needs of the session: it must
// be live, and its environment must be one a dev session may be in (DEV-003,
// PLX-9203).
func (s *Service) plan(ctx context.Context, p auth.Principal, id pgtype.UUID) (pushPlan, error) {
	var out pushPlan
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		session, err := s.authorized(ctx, tx, p, id, auth.DevSessionCreate, false)
		if err != nil {
			return err
		}
		out.set, err = s.o.Limits(ctx, tx, p.OrganizationID, session.AppID)
		if err != nil {
			return fmt.Errorf("devsession: read the limits: %w", err)
		}
		if !session.Live(s.now(), minutes(out.set, limits.DevSessionIdleMinutes)) {
			return expired()
		}
		out.env, err = dbgen.New(tx).GetEnvironment(ctx, storage.MustUUID(session.EnvironmentID))
		if err != nil {
			return fmt.Errorf("devsession: find the environment: %w", err)
		}
		if out.env.Production || !slices.Contains(s.o.Environments, out.env.Key) {
			return plxerr.New(plxerr.DevSessionEnvironment, "a dev bundle is not signed for the environment %q", out.env.Key)
		}
		out.opts, err = s.o.CompileOptions(ctx, tx, p.OrganizationID, session.AppID, session.EnvironmentID)
		if err != nil {
			return fmt.Errorf("devsession: compiler options: %w", err)
		}
		out.opts.Mode = compiler.Development
		out.session = session
		return nil
	})
	return out, err
}

// load reads the environment's draft the first time a session pushes.
func (s *Service) load(ctx context.Context, p auth.Principal, st *draftState, plan pushPlan) error {
	if st.loaded {
		return nil
	}
	var files fstest.MapFS
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		files, err = s.o.Drafts.DraftFiles(ctx, tx, plan.session.AppID)
		return err //nolint:wrapcheck // a domain error
	})
	if err != nil {
		return err
	}
	st.files, st.revs, st.cache, st.sent, st.loaded = files, map[string]int64{}, incremental.NewCache(plan.opts), map[string]bool{}, true
	return nil
}

// checkSize refuses patches above devSession.patchBytes.
func checkSize(set limits.Set, patches []DocumentPatch) error {
	max := set.Get(limits.DevSessionPatchBytes)
	var n int64
	for _, dp := range patches {
		n += int64(len(dp.Path) + len(dp.PluginID))
		for _, op := range dp.Ops {
			n += int64(len(op.Op) + len(op.Path) + len(op.From))
			if op.Value != nil {
				v, err := jcs.Marshal(op.Value)
				if err != nil {
					return plxerr.Wrap(plxerr.InvalidStructure, err, "a patch value cannot be encoded")
				}
				n += int64(len(v))
			}
		}
	}
	if n > max {
		return plxerr.New(plxerr.LimitExceeded, "the patches are %d bytes, above the limit devSession.patchBytes = %d", n, max)
	}
	return nil
}

// applyPatches applies the patches to a copy of the draft, all or none. It
// returns the new files, the new revisions and the document identifiers
// patched, sorted and without duplicates.
func applyPatches(st *draftState, patches []DocumentPatch, depth int) (fstest.MapFS, map[string]int64, []string, error) {
	files := maps.Clone(st.files)
	if files == nil {
		files = fstest.MapFS{}
	}
	revs := maps.Clone(st.revs)
	var ids []string
	for i, dp := range patches {
		if place, ok := schema.PlaceOf(dp.Path); !ok || place.Kind == "" {
			return nil, nil, nil, plxerr.New(plxerr.DevPatchRejected, "patch %d: %q is not a document path of the project layout", i, dp.Path)
		}
		f, ok := files[dp.Path]
		if !ok {
			return nil, nil, nil, plxerr.New(plxerr.DevPatchRejected, "patch %d: the draft has no document %q", i, dp.Path)
		}
		if dp.IfRevision != 0 && dp.IfRevision != revs[dp.Path]+1 {
			return nil, nil, nil, plxerr.New(plxerr.DevPatchRejected, "patch %d: %q is at revision %d, not %d", i, dp.Path, revs[dp.Path]+1, dp.IfRevision)
		}
		tree, err := jcs.Parse(f.Data, depth)
		if err != nil {
			return nil, nil, nil, plxerr.Wrap(plxerr.DevPatchRejected, err, "patch %d: the draft's %q is not JSON", i, dp.Path)
		}
		patched, err := document.Apply(tree, dp.Ops)
		if err != nil {
			return nil, nil, nil, plxerr.Wrap(plxerr.DevPatchRejected, err, "patch %d: %q: the patch does not apply", i, dp.Path)
		}
		data, err := jcs.Marshal(patched)
		if err != nil {
			return nil, nil, nil, plxerr.Wrap(plxerr.DevPatchRejected, err, "patch %d: the patched %q is not JSON", i, dp.Path)
		}
		files[dp.Path] = &fstest.MapFile{Data: data, Mode: 0o644}
		revs[dp.Path]++
		ids = append(ids, documentID(patched, dp.Path))
	}
	slices.Sort(ids)
	return files, revs, slices.Compact(ids), nil
}

// documentID names a patched document for the audit log: its identifier, or
// its path when it has none.
func documentID(tree any, path string) string {
	if m, ok := tree.(map[string]any); ok {
		if id, ok := m["id"].(string); ok && id != "" {
			return id
		}
	}
	return path
}

// bundlesOf lists the bundles of a compilation by their manifest reference.
func bundlesOf(r *compiler.Result) map[string]*compiler.Bundle {
	out := map[string]*compiler.Bundle{}
	for _, b := range append([]*compiler.Bundle{r.App}, r.Plugins...) {
		if b != nil {
			out["sha256:"+hex.EncodeToString(b.Hash[:])] = b
		}
	}
	return out
}

// manifestOf builds the signed part of a development manifest: the bundles
// of the whole compilation, in the session's sequence, naming the session.
func manifestOf(plan pushPlan, files fstest.MapFS, r *compiler.Result, seq uint64, at time.Time, channel string) release.SignedManifest {
	now := at.UTC().Truncate(time.Second)
	minRuntime := minRuntimeOf(files)
	ref := func(b *compiler.Bundle) release.SignedBundle {
		features := slices.Clone(b.Features)
		if features == nil {
			features = []string{}
		}
		return release.SignedBundle{
			Hash: "sha256:" + hex.EncodeToString(b.Hash[:]), Size: int64(len(b.Data)), RequiredFeatures: features, MinRuntime: minRuntime,
		}
	}
	doc := release.SignedManifest{
		Type: "manifest", SpecVersion: 1, Role: "targets",
		App: plan.session.AppID, Environment: plan.env.Key, Channel: channel, ReleaseSequence: int64(seq), //nolint:gosec // a count of pushes
		IssuedAt: now.Format(time.RFC3339), Expires: plan.session.ExpiresAt.UTC().Truncate(time.Second).Format(time.RFC3339),
		Plugins:            []release.SignedPlugin{},
		Control:            release.SignedControl{KillSwitches: []string{}},
		Experiments:        []release.SignedVariant{},
		DevelopmentSession: &release.SignedDevelopmentSession{SessionID: plan.session.ID, SequenceSpace: sequenceSpace},
	}
	doc.AppBundle = ref(r.App)
	for _, b := range r.Plugins {
		doc.Plugins = append(doc.Plugins, release.SignedPlugin{Key: b.Key, Version: int64(seq), SignedBundle: ref(b)}) //nolint:gosec // a count of pushes
	}
	slices.SortFunc(doc.Plugins, func(a, b release.SignedPlugin) int { return strings.Compare(a.Key, b.Key) })
	return doc
}

// minRuntimeOf is the app's minimum runtime version from its sources.
func minRuntimeOf(files fstest.MapFS) string {
	f, ok := files["app.json"]
	if !ok {
		return ""
	}
	var app struct {
		MinRuntimeVersion string `json:"minRuntimeVersion"`
	}
	_ = json.Unmarshal(f.Data, &app) //nolint:errcheck // validated by the compiler
	return app.MinRuntimeVersion
}

// sign signs the manifest with the environment's key and audits the push, in
// one transaction; it returns the manifest in the envelope the devices read
// and the devices to deliver to.
func (s *Service) sign(ctx context.Context, p auth.Principal, plan pushPlan, signed []byte, seq uint64, ids []string) ([]byte, []Device, error) {
	var (
		manifest []byte
		devices  []Device
	)
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		sigs, err := s.o.Signer.SignDocument(ctx, q, plan.env, signed)
		if err != nil {
			return fmt.Errorf("devsession: sign the bundle: %w", err)
		}
		manifest, err = json.Marshal(struct {
			Signed     []byte                      `json:"signed"`
			Signatures []release.ManifestSignature `json:"signatures"`
		}{signed, sigs})
		if err != nil {
			return fmt.Errorf("devsession: encode the manifest: %w", err)
		}
		rows, err := q.ListDevSessionDevices(ctx, storage.MustUUID(plan.session.ID))
		if err != nil {
			return fmt.Errorf("devsession: list the devices: %w", err)
		}
		for _, r := range rows {
			devices = append(devices, Device{ID: storage.ID(r.ID), DeviceID: r.DeviceID, Name: r.Name})
		}
		_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: audit.DevSessionPatched, TargetKind: "dev_session", TargetID: plan.session.ID,
			Detail: fmt.Sprintf("bundle %d, documents %s", seq, strings.Join(ids, ",")),
		})
		if err != nil {
			return fmt.Errorf("devsession: %w", err)
		}
		return nil
	})
	return manifest, devices, err
}

// bundleFrame builds the frame of a push: the manifest, the bundles the
// devices do not hold yet and the state scopes the compiler says are
// incompatible, per page.
func bundleFrame(seq uint64, manifest []byte, bundles map[string]*compiler.Bundle, sent map[string]bool, pages []incremental.PageVerdict) (*pluxv1.DevBundleFrame, []string) {
	f := &pluxv1.DevBundleFrame{BundleSequence: seq, Manifest: manifest, Sections: map[string][]byte{}}
	for ref, b := range bundles {
		if !sent[ref] {
			f.Sections[ref] = b.Data
		}
	}
	for _, v := range pages {
		if v.Compatible {
			continue
		}
		if f.IncompatibleScopes == nil {
			f.IncompatibleScopes = map[string]*pluxv1.DevScopeList{}
		}
		f.IncompatibleScopes[v.ID] = &pluxv1.DevScopeList{Scopes: slices.Clone(v.Incompatible)}
	}
	changed := make([]string, 0, len(pages))
	for _, v := range pages {
		changed = append(changed, v.ID)
	}
	slices.Sort(changed)
	return f, changed
}

// deliver sends the frame to every device and reports what happened to each.
func (s *Service) deliver(sessionID string, devices []Device, f *pluxv1.DevBundleFrame, pages []string) []*pluxv1.DevPatchResult {
	var reset []string
	for _, l := range f.GetIncompatibleScopes() {
		reset = append(reset, l.GetScopes()...)
	}
	slices.Sort(reset)
	reset = slices.Compact(reset)
	out := make([]*pluxv1.DevPatchResult, 0, len(devices))
	for _, d := range devices {
		frame := &pluxv1.DevServerFrame{Payload: &pluxv1.DevServerFrame_Bundle{Bundle: proto.Clone(f).(*pluxv1.DevBundleFrame)}}
		r := &pluxv1.DevPatchResult{DeviceId: d.ID}
		switch err := s.o.Deliverer.Deliver(sessionID, d.ID, frame); {
		case err == nil && len(reset) > 0:
			r.Status, r.PagesApplied, r.ScopesReset = pluxv1.DevPatchStatus_DEV_PATCH_STATUS_APPLIED_WITH_RESET, pages, reset
		case err == nil:
			r.Status, r.PagesApplied = pluxv1.DevPatchStatus_DEV_PATCH_STATUS_APPLIED, pages
		case errors.Is(err, ErrDeviceOffline):
			r.Status = pluxv1.DevPatchStatus_DEV_PATCH_STATUS_OFFLINE
		default:
			r.Status, r.Error = pluxv1.DevPatchStatus_DEV_PATCH_STATUS_REFUSED, deliveryError(err)
		}
		out = append(out, r)
	}
	return out
}

// deliveryError describes a failed delivery without the cause's text, which
// belongs to the transport.
func deliveryError(err error) *pluxv1.DevError {
	code := plxerr.DevPatchRejected
	if c, ok := plxerr.CodeOf(err); ok {
		code = c
	}
	reason := ""
	if def, ok := plxerr.Lookup(code); ok {
		reason = string(def.Reason)
	}
	return &pluxv1.DevError{Code: code.String(), Reason: reason, Message: "the bundle could not be delivered to the device"}
}

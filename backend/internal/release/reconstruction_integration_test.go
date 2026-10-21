// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/reconstruct"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// auditorUserID is the auditor's user identifier. It is a UUID because the
// row-level-security policies cast the session user to a UUID.
const auditorUserID = "0190c1f4-7b3a-7c2e-8a55-3d1f5e9b2a10"

// signQueue records the signing jobs of reconstruction releases.
type signQueue struct {
	mu   sync.Mutex
	jobs []reconstruct.SignJob
}

func (q *signQueue) Enqueue(_ context.Context, _ pgx.Tx, j reconstruct.SignJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, j)
	return nil
}

// reconFixture is a production device that synced three releases (the
// second a change, the third a rollback to the first) and a control change
// between the first two, with the reconstruction service on the release
// fixture's clock. The times are offsets from base.
type reconFixture struct {
	*controlFixture
	svc      *reconstruct.Service
	devices  *device.Service
	jobs     *signQueue
	log      *audit.Log
	auditor  auth.Principal
	base     time.Time
	deviceID string
	// hashes[sequence] maps a bundle key to the hash the release holds.
	hashes map[int64]map[string]string
	// only2 is the second release's app bundle, which no other release holds.
	only2 string
}

// at is the moment h hours after the fixture's base.
func (f *reconFixture) at(h float64) time.Time {
	return f.base.Add(time.Duration(h * float64(time.Hour)))
}

// hashesOf reads the bundle hashes of a release.
func (f *reconFixture) hashesOf(t *testing.T, seq int64) map[string]string {
	t.Helper()
	_, versions, err := f.rel.GetRelease(context.Background(), f.owner, f.app, seq)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, v := range versions {
		out[v.PluginKey] = "sha256:" + v.BundleSHA256
	}
	return out
}

// report makes the device report a release at the fixture's clock.
func (f *reconFixture) report(t *testing.T, seq int64, extra device.Report) {
	t.Helper()
	var bundles []device.Installed
	for key, hash := range f.hashes[seq] {
		sum, err := hex.DecodeString(strings.TrimPrefix(hash, "sha256:"))
		if err != nil {
			t.Fatal(err)
		}
		bundles = append(bundles, device.Installed{Key: key, SHA256: sum})
	}
	extra.Sequence, extra.Bundles, extra.Channel = seq, bundles, "production"
	id := device.Identity{
		DeviceID: f.deviceID, OrganizationID: f.org, AppID: f.app, EnvironmentID: f.envs["production"].ID,
	}
	if err := f.devices.Report(context.Background(), id, extra); err != nil {
		t.Fatalf("Report: %v", err)
	}
}

// newReconFixture builds the timeline described on reconFixture.
func newReconFixture(t *testing.T) *reconFixture {
	t.Helper()
	ctx := context.Background()
	cf := newControlFixture(t, true)
	f := &reconFixture{controlFixture: cf, jobs: &signQueue{}, base: cf.now.Truncate(time.Second), hashes: map[int64]map[string]string{}}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	f.log = audit.NewLog(gen, nil)
	clock := func() time.Time { return f.now }
	var err error
	if f.devices, err = device.NewService(device.Options{DB: f.db, IDs: gen, Now: clock}); err != nil {
		t.Fatal(err)
	}
	f.svc, err = reconstruct.NewService(reconstruct.Options{
		DB: f.db, Audit: f.log, IDs: gen, Objects: f.store, Locator: f.rel, Signer: f.rel, Jobs: f.jobs,
		PublicBaseURL: "https://plux.example.com", Now: clock,
		Limits: func(ctx context.Context, tx pgx.Tx, org, app string) (limits.Set, error) {
			return f.tenancy.Effective(ctx, tx, org, app, "")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.auditor = auth.Principal{
		Identity:       auth.Identity{Kind: auth.KindUser, ID: "auditor-1", UserID: auditorUserID, Display: "Auditor", SecondFactor: true},
		OrganizationID: f.org, Permissions: []auth.Permission{auth.ReconstructionView},
	}
	f.deviceID = f.registerDevice(t)
	prod := f.envs["production"]

	f.hashes[1] = f.hashesOf(t, 1)
	f.now = f.at(1)
	f.report(t, 1, device.Report{Theme: "dark", TextScalePercent: 120, UserRef: "user-7"})

	f.now = f.at(2)
	f.switchPlugin(t, "loans")
	f.run(t)

	f.now = f.at(3)
	f.shipChange(t)
	second, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, map[string]int64{"": 2, "loans": 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, second.Sequence, prod.ID, ""); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	f.hashes[second.Sequence] = f.hashesOf(t, second.Sequence)
	f.only2 = f.hashes[second.Sequence][""]
	f.report(t, second.Sequence, device.Report{})

	f.now = f.at(5)
	back, err := f.rel.RollbackRelease(ctx, f.owner, f.app, prod.ID, "", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	f.run(t)
	f.hashes[back.Sequence] = f.hashesOf(t, back.Sequence)
	f.report(t, back.Sequence, device.Report{})
	f.now = f.at(6)
	return f
}

// registerDevice inserts a production device of the app.
func (f *reconFixture) registerDevice(t *testing.T) string {
	t.Helper()
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	id, err := gen.New()
	if err != nil {
		t.Fatal(err)
	}
	err = f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).InsertDevice(ctx, dbgen.InsertDeviceParams{
			ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(f.org), AppID: storage.MustUUID(f.app),
			EnvironmentID: storage.MustUUID(f.envs["production"].ID), Platform: "android", SecretHash: []byte{1},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// shipChange changes one message and publishes the app and the plugin again.
func (f *reconFixture) shipChange(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, "", "edit", false); err != nil {
		t.Fatal(err)
	}
	en, err := f.docs.GetDocument(ctx, f.owner, f.app, "", "translations/en.json")
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(en.Content, []byte("Your schedule"), []byte("Your repayment plan"), 1)
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, "", "edit", en.Path, changed, en.Revision); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", f.loans} {
		if j := f.publish(t, id, false); j.State != release.StateSucceeded {
			t.Fatalf("publish %q: %+v", id, j)
		}
	}
}

// reconstruct makes a reconstruction release of the device at h and signs it
// the way the worker would.
func (f *reconFixture) reconstruct(t *testing.T, h float64) reconstruct.Opened {
	t.Helper()
	r, err := f.svc.Reconstruct(context.Background(), f.auditor, f.app, reconstruct.Subject{DeviceID: f.deviceID}, f.at(h))
	if err != nil {
		t.Fatalf("Reconstruct at %v: %v", h, err)
	}
	return f.signAndOpen(t, r.ID)
}

// signAndOpen runs the queued signing jobs and opens a release.
func (f *reconFixture) signAndOpen(t *testing.T, id string) reconstruct.Opened {
	t.Helper()
	f.jobs.mu.Lock()
	queued := f.jobs.jobs
	f.jobs.jobs = nil
	f.jobs.mu.Unlock()
	for _, j := range queued {
		if err := f.svc.Sign(context.Background(), j); err != nil {
			t.Fatalf("Sign: %v", err)
		}
	}
	o, err := f.svc.Get(context.Background(), f.auditor, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return o
}

// manifestOf parses an opened release's manifest.
func manifestOf(t *testing.T, o reconstruct.Opened) reconstruct.Manifest {
	t.Helper()
	var m reconstruct.Manifest
	if err := json.Unmarshal(o.Manifest, &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	return m
}

// bundleHashes maps a manifest's bundles by key.
func manifestBundles(m reconstruct.Manifest) map[string]string {
	out := map[string]string{}
	for _, b := range m.Bundles {
		out[b.Key] = b.Hash
	}
	return out
}

func equalHashes(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Verifies: SEC-142.
// The moment resolves to the release active then: before the first report
// nothing is retained, between reports the earlier release, after an
// activation the new one, and after a rollback the rolled-back content with
// the hashes of the first release. The bundle hashes, the control document
// version, the presentation and the channel are those of that moment.
func TestReconstructResolvesTheMomentAcrossActivationAndRollback_SEC_142(t *testing.T) {
	t.Parallel()
	f := newReconFixture(t)

	if _, err := f.svc.Reconstruct(context.Background(), f.auditor, f.app, reconstruct.Subject{DeviceID: f.deviceID}, f.at(0.5)); code(err) != plxerr.ReconstructionDataExpired {
		t.Errorf("before the first report: %v", err)
	}
	first := manifestOf(t, f.reconstruct(t, 1.5))
	if first.Release.Sequence != 1 || first.Release.Channel != "production" || !equalHashes(manifestBundles(first), f.hashes[1]) {
		t.Fatalf("first release: %+v want %v", first, f.hashes[1])
	}
	if first.Presentation.Theme != "dark" || first.Presentation.TextScalePercent != 120 || first.Subject.Kind != reconstruct.KindDevice || first.Channel != "audit" {
		t.Errorf("presentation or subject: %+v", first)
	}
	if string(first.Control.KillSwitches) != "[]" {
		t.Errorf("a kill switch before it was set: %s", first.Control.KillSwitches)
	}
	later := manifestOf(t, f.reconstruct(t, 2.5))
	if later.Release.Sequence != 1 || later.Control.Version <= first.Control.Version || !strings.Contains(string(later.Control.KillSwitches), "loans") {
		t.Errorf("control in force after the switch: %+v (before %d)", later.Control, first.Control.Version)
	}
	second := manifestOf(t, f.reconstruct(t, 4))
	if second.Release.Sequence != 2 || equalHashes(manifestBundles(second), manifestBundles(first)) || manifestBundles(second)[""] != f.only2 {
		t.Errorf("second release: %+v", second)
	}
	if second.Presentation.Theme != "dark" || second.Control.Version != later.Control.Version {
		t.Errorf("state carried over to the second report: %+v", second)
	}
	rolled := manifestOf(t, f.reconstruct(t, 5.5))
	if rolled.Release.Sequence != 3 || !equalHashes(manifestBundles(rolled), f.hashes[1]) || !equalHashes(manifestBundles(rolled), manifestBundles(first)) {
		t.Errorf("after the rollback: %+v", rolled)
	}
	byUser, err := f.svc.Reconstruct(context.Background(), f.auditor, f.app, reconstruct.Subject{UserRef: "user-7"}, f.at(1.5))
	if err != nil || byUser.DeviceID != f.deviceID || byUser.UserRef != "user-7" {
		t.Errorf("by user: %+v %v", byUser, err)
	}
	if _, err := f.svc.Reconstruct(context.Background(), f.auditor, f.app, reconstruct.Subject{UserRef: "nobody"}, f.at(1.5)); code(err) != plxerr.ReconstructionDataExpired {
		t.Errorf("an unknown user: %v", err)
	}
}

// Verifies: SEC-142, SEC-052.
// A reconstruction release is signed through the manifest path with the
// environment's key, so it verifies with the keys GetRootKeys reports, and
// its bundles are listed with download locations; it is pending until the
// worker signed it.
func TestReconstructionReleaseVerifiesWithTheEnvironmentKeys_SEC_142(t *testing.T) {
	t.Parallel()
	f := newReconFixture(t)
	ctx := context.Background()
	r, err := f.svc.Reconstruct(ctx, f.auditor, f.app, reconstruct.Subject{DeviceID: f.deviceID}, f.at(1.5))
	if err != nil || r.Signed || !strings.HasPrefix(r.OpenURI, "plux-viewer://reconstruction/"+r.ID) {
		t.Fatalf("Reconstruct: %+v %v", r, err)
	}
	pending, err := f.svc.Get(ctx, f.auditor, r.ID)
	if err != nil || pending.Manifest != nil || len(pending.Bundles) != 0 {
		t.Fatalf("a pending release: %+v %v", pending, err)
	}
	o := f.signAndOpen(t, r.ID)
	if !o.Signed || len(o.Signatures) != 1 {
		t.Fatalf("signed release: %+v", o)
	}
	keys, err := f.rel.RootKeys(ctx, f.org, f.envs["production"].ID)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := base64.StdEncoding.DecodeString(o.Signatures[0].Signature)
	if err != nil {
		t.Fatal(err)
	}
	verified := false
	for _, k := range keys {
		if k.KeyID == o.Signatures[0].KeyID && k.Role == "targets" && ed25519.Verify(k.PublicKey, o.Manifest, sig) {
			verified = true
		}
	}
	if !verified {
		t.Fatalf("the signature does not verify with %+v", keys)
	}
	tampered := bytes.Replace(o.Manifest, []byte(`"sequence":1`), []byte(`"sequence":2`), 1)
	for _, k := range keys {
		if bytes.Equal(tampered, o.Manifest) || ed25519.Verify(k.PublicKey, tampered, sig) {
			t.Error("a tampered manifest verifies")
		}
	}
	m := manifestOf(t, o)
	if len(o.Bundles) != len(m.Bundles) || len(o.Bundles) != 2 {
		t.Fatalf("bundles: %+v", o.Bundles)
	}
	for _, b := range o.Bundles {
		if !strings.HasPrefix(b.URL, "https://plux.example.com/v1/objects/bundles/") {
			t.Errorf("bundle location %q", b.URL)
		}
	}
	if err := f.svc.Sign(ctx, reconstruct.SignJob{OrganizationID: f.org, ReconstructionID: r.ID}); err != nil {
		t.Errorf("signing twice: %v", err)
	}
}

// Verifies: SEC-142.
// The audit channel is created on first use, holds no manifest, is never
// served to a device and cannot be created by a person; the production
// channel's manifest is unchanged.
func TestAuditChannelIsNeverServed_SEC_142(t *testing.T) {
	t.Parallel()
	f := newReconFixture(t)
	ctx := context.Background()
	prod := f.envs["production"]
	f.reconstruct(t, 1.5)
	f.reconstruct(t, 4)
	channels, err := f.tenancy.ListChannels(ctx, f.owner, prod.ID, tenancy.Page{Size: 10})
	if err != nil || len(channels) != 2 {
		t.Fatalf("channels: %+v %v", channels, err)
	}
	for _, c := range channels {
		if c.Key == "audit" && (c.ReleaseSequence != 0 || c.SignedReleaseSequence != 0) {
			t.Errorf("the audit channel points at a release: %+v", c)
		}
	}
	var manifests int
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM manifests m JOIN channels c ON c.id = m.channel_id WHERE c.key = 'audit'`).Scan(&manifests)
	})
	if err != nil || manifests != 0 {
		t.Errorf("manifests of the audit channel: %d %v", manifests, err)
	}
	_, err = f.rel.GetManifest(ctx, release.ManifestRequest{OrganizationID: f.org, AppID: f.app, EnvironmentID: prod.ID, Channel: "audit"})
	if code(err) != plxerr.PermissionDenied {
		t.Errorf("a device asked for the audit channel: %v", err)
	}
	served, err := f.rel.GetManifest(ctx, release.ManifestRequest{OrganizationID: f.org, AppID: f.app, EnvironmentID: prod.ID})
	if err != nil || served.Document.Channel != "production" || served.Document.ReleaseSequence != 3 {
		t.Errorf("the production manifest: %+v %v", served.Document, err)
	}
	if _, err := f.tenancy.CreateChannel(ctx, f.owner, prod.ID, "audit"); code(err) != plxerr.InvalidFormat {
		t.Errorf("creating the audit channel: %v", err)
	}
}

// Verifies: SEC-142.
// A moment older than reconstruction.retentionDays, a bundle that is no
// longer stored and a moment in the future are refused; the sweep deletes
// history and releases past the retention but keeps each device's state at
// the start of the window.
func TestReconstructionRetention_SEC_142(t *testing.T) {
	t.Parallel()
	f := newReconFixture(t)
	ctx := context.Background()
	subject := reconstruct.Subject{DeviceID: f.deviceID}
	old := f.now.AddDate(0, 0, -400)
	if _, err := f.svc.Reconstruct(ctx, f.auditor, f.app, subject, old); code(err) != plxerr.ReconstructionDataExpired {
		t.Errorf("older than the retention: %v", err)
	}
	if _, err := f.svc.Reconstruct(ctx, f.auditor, f.app, subject, f.now.Add(time.Hour)); code(err) != plxerr.InvalidFormat {
		t.Errorf("in the future: %v", err)
	}
	kept := f.reconstruct(t, 4)
	sum, err := hex.DecodeString(strings.TrimPrefix(f.only2, "sha256:"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := objects.Key(objects.KindBundle, hex.EncodeToString(sum))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reconstruct(ctx, f.auditor, f.app, subject, f.at(4)); code(err) != plxerr.ReconstructionDataExpired {
		t.Errorf("a bundle that is gone: %v", err)
	}
	if _, err := f.svc.Reconstruct(ctx, f.auditor, f.app, subject, f.at(5.5)); err != nil {
		t.Errorf("a moment whose bundles remain: %v", err)
	}
	f.now = f.now.AddDate(0, 0, 400)
	n, err := f.svc.Purge(ctx, f.org)
	if err != nil || n == 0 {
		t.Fatalf("Purge: %d %v", n, err)
	}
	if _, err := f.svc.Get(ctx, f.auditor, kept.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a purged release: %v", err)
	}
	var history int
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM device_installs`).Scan(&history)
	})
	if err != nil || history != 1 {
		t.Errorf("history kept: %d %v", history, err)
	}
}

// holdForReconstruction stores a reconstruction of the device that names
// release sequence seq, as the service would have written it.
func (f *reconFixture) holdForReconstruction(t *testing.T, seq int64) string {
	t.Helper()
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	id, err := gen.New()
	if err != nil {
		t.Fatal(err)
	}
	err = f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		env := f.envs["production"].ID
		var channel string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM channels WHERE environment_id = $1 LIMIT 1`, env).Scan(&channel); err != nil {
			return err //nolint:wrapcheck // a test helper
		}
		_, err := tx.Exec(ctx, `INSERT INTO reconstructions (id, organization_id, app_id, environment_id, channel_id, device_id,
				subject_kind, moment, release_sequence, manifest, created_by_kind, created_by_id, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, 'device', $7, $8, '{}', 'user', 'auditor-1', 'Auditor')`,
			id, f.org, f.app, env, channel, f.deviceID, f.now, seq)
		return err //nolint:wrapcheck // a test helper
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Verifies: SEC-142, REL-007.
// Release retention keeps a development release that a reconstruction
// names, however old, so the auditor can still open it (reconstructions
// expire on their own retention), and deletes it once none does.
func TestPurgeKeepsReconstructedReleases_SEC_142(t *testing.T) {
	t.Parallel()
	f := newReconFixture(t)
	ctx := context.Background()
	dev := f.envs["development"].ID
	held, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "Held")
	if err != nil {
		t.Fatal(err)
	}
	unheld, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "Unheld")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "Newest"); err != nil {
		t.Fatal(err)
	}
	reconstruction := f.holdForReconstruction(t, held.Sequence)
	f.now = f.now.AddDate(0, 0, 91)

	n, err := f.rel.PurgeReleases(ctx, f.org)
	if err != nil || n != 1 {
		t.Fatalf("purge with a reconstruction: %d %v, want only the release no reconstruction names", n, err)
	}
	if _, _, err := f.rel.GetRelease(ctx, f.viewer, f.app, unheld.Sequence); code(err) != plxerr.ResourceNotFound {
		t.Errorf("the release no reconstruction names survived: %v", err)
	}
	if _, _, err := f.rel.GetRelease(ctx, f.viewer, f.app, held.Sequence); err != nil {
		t.Errorf("the release a reconstruction names was purged: %v", err)
	}

	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM reconstructions WHERE id = $1`, reconstruction)
		return err //nolint:wrapcheck // a test helper
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.rel.PurgeReleases(ctx, f.org); err != nil || n != 1 {
		t.Fatalf("purge after the reconstruction expired: %d %v", n, err)
	}
	if _, _, err := f.rel.GetRelease(ctx, f.viewer, f.app, held.Sequence); code(err) != plxerr.ResourceNotFound {
		t.Errorf("the release survived its reconstruction: %v", err)
	}
}

// Verifies: SEC-142, SEC-102.
// Creating and opening a reconstruction need reconstruction.view and are
// audited, every opening each time; a caller without the permission is
// refused before the lookup says whether a release exists, and a request
// that names no subject, two, or an unknown device fails.
func TestReconstructionIsAuthorisedAndAudited_SEC_142(t *testing.T) {
	t.Parallel()
	f := newReconFixture(t)
	ctx := context.Background()
	subject := reconstruct.Subject{DeviceID: f.deviceID}
	if _, err := f.svc.Reconstruct(ctx, f.viewer, f.app, subject, f.at(1.5)); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer reconstructed: %v", err)
	}
	o := f.reconstruct(t, 1.5)
	if _, err := f.svc.Get(ctx, f.viewer, o.ID); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer opened: %v", err)
	}
	if _, err := f.svc.List(ctx, f.viewer, f.app, storage.Cursor{}, 10); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer listed: %v", err)
	}
	if _, err := f.svc.Get(ctx, f.viewer, "0197d0e0-0000-7000-8000-000000000000"); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer probed: %v", err)
	}
	for range 2 {
		if _, err := f.svc.Get(ctx, f.auditor, o.ID); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.auditCount(t, audit.ReconstructionCreated); n != 1 {
		t.Errorf("creations audited: %d", n)
	}
	// The release was opened once by reconstruct and twice above.
	if n := f.auditCount(t, audit.ReconstructionViewed); n != 3 {
		t.Errorf("views audited: %d", n)
	}
	list, err := f.svc.List(ctx, f.auditor, f.app, storage.Cursor{}, 10)
	if err != nil || len(list) != 1 || list[0].ID != o.ID {
		t.Errorf("List: %+v %v", list, err)
	}
	for name, s := range map[string]reconstruct.Subject{
		"neither": {}, "both": {DeviceID: f.deviceID, UserRef: "user-7"},
	} {
		if _, err := f.svc.Reconstruct(ctx, f.auditor, f.app, s, f.at(1.5)); code(err) != plxerr.InvalidFormat {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.svc.Reconstruct(ctx, f.auditor, f.app, reconstruct.Subject{DeviceID: "0197d0e0-0000-7000-8000-000000000000"}, f.at(1.5)); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an unknown device: %v", err)
	}
	if n := f.auditNamingTheDevice(t); n != 0 {
		t.Errorf("%d reconstruction audit entries name the device", n)
	}
}

// auditNamingTheDevice counts the reconstruction audit entries whose detail
// or target names the device: none may, since the chain is never erased.
func (f *reconFixture) auditNamingTheDevice(t *testing.T) int {
	t.Helper()
	var n int
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action LIKE 'reconstruction.%' AND (detail LIKE '%' || $1 || '%' OR target_id = $1)`, f.deviceID).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

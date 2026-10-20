// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/updatemeta"
)

// controlFixture is an environment enrolled in update metadata with a
// root that delegates the control role.
type controlFixture struct {
	*metadataFixture
	ref   release.ChannelRef
	scope updatemeta.ControlScope
}

func newControlFixture(t *testing.T, delegate bool) *controlFixture {
	t.Helper()
	return newControlFixtureWith(t, delegate, limits.Set{})
}

// newControlFixtureWith is newControlFixture under the given limits.
func newControlFixtureWith(t *testing.T, delegate bool, set limits.Set) *controlFixture {
	t.Helper()
	f := newMetadataFixtureWith(t, set)
	f.ceremony.noControl = !delegate
	keys := newOfflineKeys(t, 3)
	ctx := context.Background()
	if _, err := f.rel.UploadRoot(ctx, f.org, f.prod, f.ceremony.root(1, keys, 2, keys[0], keys[1])); err != nil {
		t.Fatalf("UploadRoot: %v", err)
	}
	if _, err := f.rel.RefreshMetadata(ctx, f.org); err != nil {
		t.Fatalf("RefreshMetadata: %v", err)
	}
	return &controlFixture{
		metadataFixture: f,
		ref:             release.ChannelRef{AppID: f.app, EnvironmentID: f.prod, ChannelKey: "production"},
		scope:           updatemeta.ControlScope{App: f.app, Environment: f.envs["production"].Key, Channel: "production"},
	}
}

// verifier is what a production device holds: the trusted root and the
// key environments GetRootKeys reports.
func (f *controlFixture) verifier(t *testing.T, floor int64) updatemeta.Verifier {
	t.Helper()
	ctx := context.Background()
	rootFile, err := f.rel.Metadata(ctx, f.prod, "1.root.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := updatemeta.ParseDocument(rootFile.Document)
	if err != nil {
		t.Fatal(err)
	}
	root, err := updatemeta.ParseRoot(d.Signed)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := f.rel.RootKeys(ctx, f.org, f.prod)
	if err != nil {
		t.Fatal(err)
	}
	envs := map[string]string{}
	for _, k := range keys {
		envs[k.KeyID] = k.EnvironmentType
	}
	return updatemeta.Verifier{Root: root, Production: true, KeyEnvironments: envs, Floor: updatemeta.Floor{Control: floor}, Now: f.now}
}

// served fetches the newest signed document as a device would.
func (f *controlFixture) served(t *testing.T, ifNoneMatch string) release.ServedControl {
	t.Helper()
	got, err := f.rel.GetControlDocument(context.Background(), release.ControlRequest{
		OrganizationID: f.org, EnvironmentID: f.prod, Channel: "production", IfNoneMatch: ifNoneMatch,
	})
	if err != nil {
		t.Fatalf("GetControlDocument: %v", err)
	}
	return got
}

// verified fetches and verifies the newest document.
func (f *controlFixture) verified(t *testing.T) controldoc.Document {
	t.Helper()
	doc, err := f.verifier(t, 0).Control(f.served(t, "").Signed, f.scope)
	if err != nil {
		t.Fatalf("the served document does not verify: %v", err)
	}
	return doc
}

func (f *controlFixture) switchPlugin(t *testing.T, target string) {
	t.Helper()
	if _, err := f.rel.SetKillSwitch(context.Background(), f.owner, f.ref, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: target}); err != nil {
		t.Fatalf("SetKillSwitch: %v", err)
	}
}

// Verifies: SYN-060, REL-060, SEC-052.
// A kill switch is signed by the worker's control key and a device
// verifies it from its trusted root, which delegates the role; the
// document is read only after the signature, expiry and version pass.
func TestControlKillSwitchSignedFromRoot_SYN_060(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	ctx := context.Background()
	f.switchPlugin(t, "loans")
	if _, err := f.rel.SetKillSwitch(ctx, f.owner, f.ref, controldoc.KillSwitch{Scope: controldoc.ScopeFunction, Target: "orders.total"}); err != nil {
		t.Fatal(err)
	}
	// Until the worker has run, nothing is signed, and the state says so.
	state, err := f.rel.ControlDocument(ctx, f.viewer, f.ref)
	if err != nil || state.Version != 2 || state.SignedVersion != 0 || len(state.Signed) != 0 {
		t.Fatalf("before signing: %+v %v", state, err)
	}
	f.run(t)
	doc := f.verified(t)
	want := []controldoc.KillSwitch{{Scope: controldoc.ScopePlugin, Target: "loans"}, {Scope: controldoc.ScopeFunction, Target: "orders.total"}}
	if doc.Version != 2 || !slices.Equal(doc.KillSwitches, want) {
		t.Fatalf("document = %+v", doc)
	}
	if got := doc.ExpiresAt.Sub(doc.IssuedAt); got != 24*time.Hour {
		t.Errorf("lifetime = %s, want 24h", got)
	}
	// The legacy manifest flags follow (REL-030) and the change is audited.
	c, err := f.rel.GetControl(ctx, f.viewer, f.app, f.prod, "")
	if err != nil || !slices.Equal(c.KillSwitchPlugins, []string{"loans"}) {
		t.Errorf("legacy control = %+v %v", c, err)
	}
	if n := f.auditCount(t, audit.ControlChanged); n < 2 {
		t.Errorf("%d control audit entries, want at least 2", n)
	}
	// Setting the same switch again changes nothing and costs no version.
	f.switchPlugin(t, "loans")
	if state, _ = f.rel.ControlDocument(ctx, f.viewer, f.ref); state.Version != 2 {
		t.Errorf("a repeated switch made version %d", state.Version)
	}
	// Clearing it does.
	if _, err := f.rel.ClearKillSwitch(ctx, f.owner, f.ref, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: "loans"}); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if doc = f.verified(t); doc.Version != 3 || len(doc.KillSwitches) != 1 {
		t.Errorf("after clearing: %+v", doc)
	}
}

// auditCount counts the audit entries of an action in the fixture's
// organisation.
func (f *controlFixture) auditCount(t *testing.T, action audit.Action) int {
	t.Helper()
	var n int
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = $1`, string(action)).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Verifies: SYN-060, SEC-052, PLX-3100.
// A device refuses tampered bytes, a document signed by a key the root does
// not delegate to, and a root that delegates no control role.
func TestControlRefusedWhenTamperedOrUndelegated_SYN_060(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	f.switchPlugin(t, "loans")
	f.run(t)
	file := f.served(t, "").Signed
	v := f.verifier(t, 0)
	if _, err := v.Control(file, f.scope); err != nil {
		t.Fatalf("the genuine document: %v", err)
	}
	tampered := bytes.Replace(file, []byte(`"channel":"production"`), []byte(`"channel":"productioo"`), 1)
	if bytes.Equal(tampered, file) {
		t.Fatal("the test did not tamper with the file")
	}
	if _, err := v.Control(tampered, f.scope); !errors.Is(err, updatemeta.ErrThreshold) {
		t.Errorf("tampered bytes: %v", err)
	}
	// A control key of another environment is not the delegated one.
	stranger := newControlFixture(t, true)
	stranger.switchPlugin(t, "loans")
	stranger.run(t)
	if _, err := v.Control(stranger.served(t, "").Signed, stranger.scope); !errors.Is(err, updatemeta.ErrThreshold) {
		t.Errorf("another key: %v", err)
	}
	// A root without the role accepts no control document at all.
	old := newControlFixture(t, false)
	old.switchPlugin(t, "loans")
	old.run(t)
	if _, err := old.rel.GetControlDocument(context.Background(), release.ControlRequest{OrganizationID: old.org, EnvironmentID: old.prod, Channel: "production"}); code(err) != plxerr.ResourceNotFound {
		t.Errorf("an unsigned channel is served a document: %v", err)
	}
}

// Verifies: SYN-060, PLX-3101, PLX-3102.
// A document expires after its lifetime and the sweep signs a fresh
// version before it does; a device refuses an older version.
func TestControlExpiryAndRefresh_SYN_060(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	ctx := context.Background()
	f.switchPlugin(t, "loans")
	f.run(t)
	first := f.verified(t)

	// Within half the lifetime the sweep leaves it alone.
	f.now = f.now.Add(11 * time.Hour)
	if n, err := f.rel.RefreshControl(ctx, f.org); err != nil || n != 0 {
		t.Fatalf("RefreshControl early: %d %v", n, err)
	}
	// Past half, it signs the same content under the next version.
	f.now = f.now.Add(2 * time.Hour)
	if n, err := f.rel.RefreshControl(ctx, f.org); err != nil || n != 1 {
		t.Fatalf("RefreshControl late: %d %v", n, err)
	}
	second := f.verified(t)
	if second.Version <= first.Version || !slices.Equal(second.KillSwitches, first.KillSwitches) || !second.ExpiresAt.After(first.ExpiresAt) {
		t.Fatalf("refresh: %+v then %+v", first, second)
	}
	// The old file is expired at its expiry, and below the newest version.
	f.now = first.ExpiresAt.Add(time.Second)
	oldFile := f.signedAt(t, first.Version)
	if _, err := f.verifier(t, 0).Control(oldFile, f.scope); !errors.Is(err, updatemeta.ErrExpired) {
		t.Errorf("an expired document: %v", err)
	}
	f.now = first.IssuedAt.Add(time.Minute)
	if _, err := f.verifier(t, int64(second.Version)).Control(oldFile, f.scope); !errors.Is(err, updatemeta.ErrRollback) {
		t.Errorf("an older version: %v", err)
	}
}

// signedAt reads a stored signed file of a version straight from the
// table, as a device that fetched it earlier would hold it.
func (f *controlFixture) signedAt(t *testing.T, version uint64) []byte {
	t.Helper()
	var file []byte
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT signed FROM control_documents WHERE version = $1 AND signed IS NOT NULL ORDER BY channel_id LIMIT 1`, int64(version)).Scan(&file) //nolint:gosec // a small version
	})
	if err != nil {
		t.Fatalf("no signed version %d: %v", version, err)
	}
	return file
}

// Verifies: SYN-060, REL-060.
// Versions strictly increase when two changes race: each takes the next
// one under the channel's lock, in a transaction of its own.
func TestControlVersionsIncreaseUnderConcurrentChanges_SYN_060(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	targets := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}
	var wg sync.WaitGroup
	errs := make([]error, len(targets))
	for i, target := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.rel.SetKillSwitch(context.Background(), f.owner, f.ref, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: target})
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("change %s: %v", targets[i], err)
		}
	}
	f.run(t)
	doc := f.verified(t)
	if len(doc.KillSwitches) != len(targets) {
		t.Fatalf("a change was lost: %+v", doc.KillSwitches)
	}
	var versions []int64
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT version FROM control_documents ORDER BY version`)
		if err != nil {
			return err //nolint:wrapcheck // a test
		}
		defer rows.Close()
		for rows.Next() {
			var v int64
			if err := rows.Scan(&v); err != nil {
				return err //nolint:wrapcheck // a test
			}
			versions = append(versions, v)
		}
		return rows.Err() //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range versions {
		if v != int64(i+1) {
			t.Fatalf("versions = %v, want 1..n without gaps or repeats", versions)
		}
	}
}

// Verifies: SYN-060.
// A device that holds the newest document sends its ETag back and is told
// it is current; a changed document has a new ETag.
func TestControlETag_SYN_060(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	f.switchPlugin(t, "loans")
	f.run(t)
	first := f.served(t, "")
	if first.ETag == "" || first.NotModified || len(first.Signed) == 0 {
		t.Fatalf("first fetch: %+v", first)
	}
	for _, header := range []string{first.ETag, "W/" + first.ETag, `"other", ` + first.ETag, "*"} {
		if got := f.served(t, header); !got.NotModified || len(got.Signed) != 0 || got.ETag != first.ETag {
			t.Errorf("If-None-Match %q: %+v", header, got)
		}
	}
	if got := f.served(t, `"stale"`); got.NotModified {
		t.Errorf("a stale ETag was told it is current")
	}
	f.switchPlugin(t, "other")
	f.run(t)
	if next := f.served(t, first.ETag); next.NotModified || next.ETag == first.ETag {
		t.Errorf("after a change: %+v", next)
	}
}

// Verifies: REL-070, SYN-060.
// A mandatory update names a release of the channel and survives the round
// trip through the signed document; it is refused for a release the
// channel cannot serve.
func TestControlMandatoryUpdateRoundTrip_REL_070(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	ctx := context.Background()
	current := f.currentSequence(t)
	state, err := f.rel.SetMandatoryUpdate(ctx, f.owner, f.ref, current, "update.security")
	if err != nil {
		t.Fatalf("SetMandatoryUpdate: %v", err)
	}
	if state.MandatoryUpdate == nil || state.MandatoryUpdate.ReleaseSequence != uint64(current) { //nolint:gosec // positive
		t.Fatalf("state = %+v", state.MandatoryUpdate)
	}
	f.run(t)
	doc := f.verified(t)
	m := doc.MandatoryUpdate
	if m == nil || m.ReleaseSequence != uint64(current) || m.MessageKey != "update.security" || m.ReleaseID != state.MandatoryUpdate.ReleaseID { //nolint:gosec // positive
		t.Fatalf("document = %+v", doc.MandatoryUpdate)
	}
	// A release the channel has not been promoted to cannot be mandatory,
	// and neither can one that does not exist.
	for bad, want := range map[int64]plxerr.Code{current + 5: plxerr.PreconditionFailed, 0: plxerr.InvalidFormat, -1: plxerr.InvalidFormat} {
		if _, err := f.rel.SetMandatoryUpdate(ctx, f.owner, f.ref, bad, ""); code(err) != want {
			t.Errorf("mandatory update to %d: %v, want %d", bad, err, want)
		}
	}
	if _, err := f.rel.SetMandatoryUpdate(ctx, f.owner, f.ref, current, "has space"); code(err) != plxerr.InvalidFormat {
		t.Errorf("a bad message key: %v", err)
	}
	if _, err := f.rel.ClearMandatoryUpdate(ctx, f.owner, f.ref); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if doc = f.verified(t); doc.MandatoryUpdate != nil {
		t.Errorf("the withdrawn update is still there: %+v", doc.MandatoryUpdate)
	}
	if c, _ := f.rel.GetControl(ctx, f.viewer, f.app, f.prod, ""); c.MandatoryUpdate {
		t.Errorf("the legacy flag is still on")
	}
}

// currentSequence is the sequence of the production channel's release.
func (f *controlFixture) currentSequence(t *testing.T) int64 {
	t.Helper()
	var seq int64
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT release_sequence FROM channels WHERE environment_id = $1 AND key = 'production'`, storage.MustUUID(f.prod)).Scan(&seq)
	})
	if err != nil {
		t.Fatal(err)
	}
	return seq
}

// Verifies: REL-006, SYN-060, REL-060.
// A forced rollback must name an earlier release that still exists and has
// been in production; every other name is refused with a clear
// error and nothing is written.
func TestControlForcedRollbackValidation_REL_060(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	ctx := context.Background()
	first := f.currentSequence(t)
	// A release only the development environment has had.
	devOnly, err := f.rel.RollbackRelease(ctx, f.owner, f.app, f.envs["development"].ID, "", first, "")
	if err != nil {
		t.Fatal(err)
	}
	back, err := f.rel.RollbackRelease(ctx, f.owner, f.app, f.prod, "", first, "")
	if err != nil {
		t.Fatal(err)
	}
	f.run(t)
	before, _ := f.rel.ControlDocument(ctx, f.viewer, f.ref)

	cases := map[string]struct {
		seq  int64
		want plxerr.Code
	}{
		"the current release":  {back.Sequence, plxerr.PreconditionFailed},
		"a newer release":      {back.Sequence + 3, plxerr.PreconditionFailed},
		"a missing release":    {9999, plxerr.PreconditionFailed},
		"never in production":  {devOnly.Sequence, plxerr.PreconditionFailed},
		"a non-positive value": {0, plxerr.InvalidFormat},
	}
	for name, c := range cases {
		if _, err := f.rel.SetForcedRollback(ctx, f.owner, f.ref, c.seq); code(err) != c.want {
			t.Errorf("%s (%d): err = %v, want code %d", name, c.seq, err, c.want)
		}
	}
	after, _ := f.rel.ControlDocument(ctx, f.viewer, f.ref)
	if after.Version != before.Version {
		t.Errorf("a refused rollback wrote version %d", after.Version)
	}
	state, err := f.rel.SetForcedRollback(ctx, f.owner, f.ref, first)
	if err != nil {
		t.Fatalf("an earlier release: %v", err)
	}
	f.run(t)
	doc := f.verified(t)
	if doc.ForcedRollback == nil || doc.ForcedRollback.ReleaseSequence != uint64(first) || doc.ForcedRollback.ReleaseID != state.ForcedRollback.ReleaseID { //nolint:gosec // positive
		t.Fatalf("document = %+v", doc.ForcedRollback)
	}
	if _, err := f.rel.ClearForcedRollback(ctx, f.owner, f.ref); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if doc = f.verified(t); doc.ForcedRollback != nil {
		t.Errorf("the withdrawn rollback is still there")
	}
}

// Verifies: REL-060, GOV-003.
// Switching needs killswitch.manage, so a viewer is refused, and every
// malformed target is refused.
func TestControlAuthorisationAndTargets_REL_060(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	ctx := context.Background()
	k := controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: "loans"}
	if _, err := f.rel.SetKillSwitch(ctx, f.viewer, f.ref, k); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer switched a plugin off: %v", err)
	}
	bad := []controldoc.KillSwitch{
		{Scope: "app", Target: "loans"},
		{Scope: controldoc.ScopePlugin, Target: "Not A Key"},
		{Scope: controldoc.ScopePage, Target: "home"},
		{Scope: controldoc.ScopeDataSource, Target: "0190A000-0000-7000-8000-0000000000AA"},
		{Scope: controldoc.ScopeFunction, Target: "nodot"},
	}
	for _, b := range bad {
		if _, err := f.rel.SetKillSwitch(ctx, f.owner, f.ref, b); code(err) != plxerr.InvalidFormat {
			t.Errorf("%+v: %v", b, err)
		}
	}
	for _, good := range []controldoc.KillSwitch{
		{Scope: controldoc.ScopePage, Target: "0190a000-0000-7000-8000-0000000000aa"},
		{Scope: controldoc.ScopeActionGraph, Target: "0190a000-0000-7000-8000-0000000000ab"},
		{Scope: controldoc.ScopeDataSource, Target: "0190a000-0000-7000-8000-0000000000ac"},
		{Scope: controldoc.ScopeFunction, Target: "orders.total"},
	} {
		if _, err := f.rel.SetKillSwitch(ctx, f.owner, f.ref, good); err != nil {
			t.Errorf("%+v: %v", good, err)
		}
	}
}

// Verifies: SYN-060, REL-030.
// The legacy call keeps working: its plugin switches and mandatory flag
// reach the signed document, and a switch set through the new call reaches
// the manifest's flags.
func TestControlLegacyCallFeedsTheDocument_SYN_060(t *testing.T) {
	t.Parallel()
	f := newControlFixture(t, true)
	ctx := context.Background()
	if _, err := f.rel.SetControl(ctx, f.owner, release.Control{
		AppID: f.app, EnvironmentID: f.prod, KillSwitchPlugins: []string{"loans"}, AppKillSwitch: true, MandatoryUpdate: true, Message: "Update",
	}); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	doc := f.verified(t)
	if len(doc.KillSwitches) != 1 || doc.KillSwitches[0].Target != "loans" || doc.MandatoryUpdate == nil || doc.MandatoryUpdate.ReleaseSequence != uint64(f.currentSequence(t)) { //nolint:gosec // positive
		t.Fatalf("document = %+v", doc)
	}
	f.switchPlugin(t, "other")
	c, err := f.rel.GetControl(ctx, f.viewer, f.app, f.prod, "")
	if err != nil || !c.AppKillSwitch || c.Message != "Update" || !slices.Equal(c.KillSwitchPlugins, []string{"loans", "other"}) {
		t.Errorf("legacy control after a new-style switch: %+v %v", c, err)
	}
}

// Verifies: REL-060.
// A channel may carry only as many kill switches as the registry allows.
func TestControlKillSwitchLimit_REL_060(t *testing.T) {
	t.Parallel()
	const max = 3
	set, err := limits.Defaults().Tighten(limits.ControlKillSwitches, limits.ScopeInstallation, max)
	if err != nil {
		t.Fatal(err)
	}
	f := newControlFixtureWith(t, true, set)
	ctx := context.Background()
	for _, target := range []string{"one", "two", "three"} {
		if _, err := f.rel.SetKillSwitch(ctx, f.owner, f.ref, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: target}); err != nil {
			t.Fatalf("switch %s: %v", target, err)
		}
	}
	if _, err := f.rel.SetKillSwitch(ctx, f.owner, f.ref, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: "overflow"}); code(err) != plxerr.LimitExceeded {
		t.Errorf("one switch too many: %v", err)
	}
	if _, err := f.rel.ClearKillSwitch(ctx, f.owner, f.ref, controldoc.KillSwitch{Scope: controldoc.ScopePlugin, Target: "one"}); err != nil {
		t.Errorf("clearing at the limit: %v", err)
	}
}

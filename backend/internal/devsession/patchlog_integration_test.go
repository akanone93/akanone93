// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/icons"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// logState reads a session's patch log: its last entry and its bytes.
func (f *pushFixture) logState(t *testing.T, sessionID string) dbgen.DevSessionPatchLogStateRow {
	t.Helper()
	var out dbgen.DevSessionPatchLogStateRow
	err := f.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = dbgen.New(tx).DevSessionPatchLogState(ctx, storage.MustUUID(sessionID))
		return err //nolint:wrapcheck // a test
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// revised replaces the first state's default with a revision condition.
func revised(value any, rev int64) devsession.DocumentPatch {
	p := replace("/state/1/default", value)
	p.IfRevision = rev
	return p
}

// Verifies: DEV-021, DEV-010.
// A CLI session's patches live in the database: a patch pushed on replica A
// is in the draft replica B compiles, after B replays the log, and A
// catches up with what B pushed.
func TestPushPatch_LogReplaysOnAnotherReplica_DEV_021_DEV_010(t *testing.T) {
	f := newPushFixture(t, 0)
	b, err := devsession.New(f.opts)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := f.open(t)
	f.pairDevices(t, s, 1)
	ctx := context.Background()
	if _, err := f.push(s, revised(json.Number("24"), 1)); err != nil {
		t.Fatal(err)
	}
	// Revision 2 holds only on a draft that has the first patch.
	res, err := b.PushPatch(ctx, f.Developer, s.ID, []devsession.DocumentPatch{revised(json.Number("36"), 2)})
	if err != nil {
		t.Fatalf("replica B: %v", err)
	}
	if res.BundleSequence != 2 {
		t.Errorf("replica B pushed bundle %d, want 2", res.BundleSequence)
	}
	res, err = f.push(s, revised(json.Number("48"), 3))
	if err != nil {
		t.Fatalf("replica A after B: %v", err)
	}
	if res.BundleSequence != 3 {
		t.Errorf("replica A pushed bundle %d, want 3", res.BundleSequence)
	}
	if st := f.logState(t, s.ID); st.LastSeq != 3 || st.TotalBytes <= 0 {
		t.Errorf("log = %+v, want three entries", st)
	}
	_, err = b.PushPatch(ctx, f.Developer, s.ID, []devsession.DocumentPatch{revised(json.Number("60"), 3)})
	wantCode(t, err, plxerr.DevPatchRejected, "a stale revision on a replica that caught up")
}

// Verifies: DEV-010.
// devSession.patchLogBytes bounds a session's log: a push beyond it is
// refused and leaves the log as it was.
func TestPushPatch_LogBoundRefuses_DEV_010(t *testing.T) {
	f := newPushFixtureWith(t, map[limits.Key]int64{limits.DevSessionPatchLogBytes: 120})
	s, _ := f.open(t)
	if _, err := f.push(s, replace("/state/1/default", json.Number("24"))); err != nil {
		t.Fatal(err)
	}
	before := f.logState(t, s.ID)
	_, err := f.push(s, replace("/state/1/default", json.Number("36")))
	wantCode(t, err, plxerr.LimitExceeded, "a push beyond devSession.patchLogBytes")
	if after := f.logState(t, s.ID); after != before {
		t.Errorf("log = %+v after a refused push, want %+v", after, before)
	}
}

// Verifies: DEV-010.
// The log goes with its session.
func TestEndSession_DropsPatchLog_DEV_010(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _ := f.open(t)
	if _, err := f.push(s, replace("/state/1/default", json.Number("24"))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.EndSession(context.Background(), f.Developer, s.ID); err != nil {
		t.Fatal(err)
	}
	if st := f.logState(t, s.ID); st.LastSeq != 0 {
		t.Errorf("log = %+v after the session ended", st)
	}
}

// Verifies: STU-130, DEV-010.
// A Studio session takes no patches: Studio never sends an edit twice.
func TestPushPatch_StudioSessionRefused_STU_130(t *testing.T) {
	f := newPushFixture(t, 0)
	s, _, err := f.svc.CreateSessionOfKind(context.Background(), f.Developer, f.App, f.Envs["development"], devsession.KindStudio)
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != devsession.KindStudio {
		t.Errorf("kind = %q", s.Kind)
	}
	_, err = f.push(s, replace("/state/1/default", json.Number("24")))
	wantCode(t, err, plxerr.DevPatchRejected, "a patch to a Studio session")
}

// lateSigner is fileSigner for a fixture made after the options.
type lateSigner struct{ f *fixture }

func (g *lateSigner) SignDocument(ctx context.Context, q *dbgen.Queries, env dbgen.Environment, canonical []byte) ([]release.ManifestSignature, error) {
	return fileSigner{g.f}.SignDocument(ctx, q, env, canonical)
}

// Verifies: STU-130, DEV-010, DEV-021.
// A Studio session follows the stored draft: an accepted write on replica A
// is compiled and pushed through the session's frame stream to the device
// whose socket is on replica B. A CLI session of the same app is not
// pushed to.
func TestDraftWritten_PushesToStudioSessions_STU_130_DEV_021(t *testing.T) {
	shared := sharedLayer(t)
	signer := &lateSigner{}
	var opts devsession.Options
	a := newRelayWith(t, nil, func(o *devsession.Options) {
		o.Shared, o.Drafts, o.Signer = shared, drafts{t}, signer
		o.CompileOptions = func(context.Context, pgx.Tx, string, string, string) (compiler.Options, error) {
			co := compiler.DefaultOptions()
			co.IconFont = func(set icons.Set, names []string) ([]byte, error) { return []byte(string(set)), nil }
			return co, nil
		}
		opts = *o
	})
	signer.f = a.fixture
	t.Cleanup(a.svc.Close)
	opts.Deliverer = nil
	svcB, err := devsession.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svcB.Close)
	fb := *a.fixture
	fb.svc = svcB
	b := &relay{fixture: &fb, used: map[string]bool{}}
	b.srv = newSocketServer(t, svcB, b.proof)

	ctx := context.Background()
	s, _, err := a.svc.CreateSessionOfKind(ctx, a.Developer, a.App, a.Envs["development"], devsession.KindStudio)
	if err != nil {
		t.Fatal(err)
	}
	cli, _ := a.open(t)
	d := a.pairDevice(t, s.ID, "device-1")
	k := b.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})
	k.c.SetReadLimit(1 << 24) // a first bundle carries every section
	a.svc.DraftWritten(ctx, a.Developer, a.App, "", []string{calculator})
	var bundle *pluxv1.DevBundleFrame
	for range 4 {
		if bundle = k.next(t).GetBundle(); bundle != nil {
			break
		}
	}
	if bundle == nil || bundle.GetBundleSequence() != 1 || len(bundle.GetSections()) == 0 {
		t.Fatalf("the device received %v, want the first bundle", bundle)
	}
	if got := a.Audit(t, "devsession.patched"); len(got) != 1 || got[0].TargetID != s.ID {
		t.Errorf("audit = %+v, want one push to the Studio session and none to %s", got, cli.ID)
	}
}

// newSocketServer serves a replica's device sockets.
func newSocketServer(t *testing.T, svc *devsession.Service, proof devsession.ProofFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET "+devsession.SocketPath, svc.SocketHandler(proof))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

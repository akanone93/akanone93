// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/icons"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// refusingSigner cannot sign, as a signing backend that is down.
type refusingSigner struct{}

func (refusingSigner) SignDocument(context.Context, *dbgen.Queries, dbgen.Environment, []byte) ([]release.ManifestSignature, error) {
	return nil, plxerr.New(plxerr.PreconditionFailed, "the signing backend is unavailable")
}

// brokenDrafts is the fixture's draft with one page that does not parse.
type brokenDrafts struct{ drafts }

func (d brokenDrafts) DraftFiles(ctx context.Context, tx pgx.Tx, app string) (fstest.MapFS, error) {
	m, err := d.drafts.DraftFiles(ctx, tx, app)
	m[calculator] = &fstest.MapFile{Data: []byte("{")}
	return m, err
}

// syncBuffer is a log destination the relay's goroutines may write to.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p) //nolint:wrapcheck // a test
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Verifies: DEV-010, STU-130.
// A Studio session whose draft cannot reach its devices says why: a device
// that connects is sent nothing, and the session's console receives the
// failure as an error event, the server's log a warning that names the
// codes and not the messages, which may quote the draft.
func TestWelcome_UnpushedDraftIsReported_DEV_010_STU_130(t *testing.T) {
	cases := []struct {
		name   string
		drafts devsession.DraftSource
		signer devsession.ManifestSigner
		code   string
	}{
		{"unsigned", drafts{t}, refusingSigner{}, plxerr.PreconditionFailed.String()},
		{"does not compile", brokenDrafts{drafts{t}}, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := &syncBuffer{}
			late := &lateSigner{}
			r := newRelayWith(t, nil, func(o *devsession.Options) {
				o.Shared, o.Drafts, o.Signer = sharedLayer(t), tc.drafts, tc.signer
				if tc.signer == nil {
					o.Signer = late
				}
				o.Log = slog.New(slog.NewTextHandler(logs, nil))
				o.CompileOptions = func(context.Context, pgx.Tx, string, string, string) (compiler.Options, error) {
					co := compiler.DefaultOptions()
					co.IconFont = func(set icons.Set, names []string) ([]byte, error) { return []byte(string(set)), nil }
					return co, nil
				}
			})
			late.f = r.fixture
			t.Cleanup(r.svc.Close)
			ctx := context.Background()
			s, _, err := r.svc.CreateSessionOfKind(ctx, r.Developer, r.App, r.Envs["development"], devsession.KindStudio)
			if err != nil {
				t.Fatal(err)
			}
			d := r.pairDevice(t, s.ID, "device-1")
			r.connect(t, d.Ticket, d.key, &pluxv1.DevDeviceHello{})

			errs, err := consoleErrors(ctx, r, s.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(errs) == 0 {
				t.Fatal("the session's console received no error for the draft that was not pushed")
			}
			for _, e := range errs {
				if e.GetSeverity() != pluxv1.Severity_SEVERITY_ERROR || e.GetError().GetCode() == "" || e.GetError().GetMessage() == "" {
					t.Errorf("console error %v, want a coded error with a message", e)
				}
			}
			if tc.code != "" && errs[0].GetError().GetCode() != tc.code {
				t.Errorf("console error code %q, want %q", errs[0].GetError().GetCode(), tc.code)
			}
			if n := len(r.Audit(t, "devsession.patched")); n != 0 {
				t.Errorf("%d pushes were audited for a draft that was not pushed", n)
			}
			logged := logs.String()
			if !strings.Contains(logged, "the draft was not pushed") || !strings.Contains(logged, s.ID) ||
				!strings.Contains(logged, errs[0].GetError().GetCode()) {
				t.Errorf("the server's log %q does not name the session and the code", logged)
			}
			if strings.Contains(logged, errs[0].GetError().GetMessage()) {
				t.Errorf("the server's log %q quotes the message %q", logged, errs[0].GetError().GetMessage())
			}
		})
	}
}

// consoleErrors reads the error events of a session's console, waiting for
// the first ones up to a few seconds.
func consoleErrors(ctx context.Context, r *relay, sessionID string) ([]*pluxv1.ErrorEvent, error) {
	var errs []*pluxv1.ErrorEvent
	deadline := time.Now().Add(5 * time.Second)
	for len(errs) == 0 && time.Now().Before(deadline) {
		err := r.svc.WatchFor(ctx, r.Developer, sessionID, nil, 0, 200*time.Millisecond, func(ev *pluxv1.DevEvent) error {
			if e := ev.GetError(); e != nil {
				errs = append(errs, e)
			}
			return nil
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a test
		}
	}
	if len(errs) == 0 {
		return nil, errors.New("no error event within the wait")
	}
	return errs, nil
}

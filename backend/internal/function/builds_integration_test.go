// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// declarationOf reads the declaration stored with a build, or "" if none.
func (w *world) declarationOf(t *testing.T, buildID string) string {
	t.Helper()
	var doc string
	err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetFunctionDeclaration(ctx, storage.MustUUID(buildID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		doc = string(row.Declaration)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// Verifies: FN-008, FN-016.
// The declaration the builder read from the source is stored with the build
// that succeeded, and only with that one.
func TestRunBuildStoresTheDeclarationOfASucceededBuild_FN_016(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil, nil)
	out := goodOutput(fnName)
	out.Declaration = []byte(`{"kind":"function","name":"payments.charge","placement":"server"}`)
	w.builder.set(out, nil)
	b := w.built(t, "development", fnName)
	var doc map[string]any
	if err := json.Unmarshal([]byte(w.declarationOf(t, b.ID)), &doc); err != nil || doc["name"] != fnName {
		t.Errorf("stored declaration is not the document: %v", err)
	}

	failed := goodOutput(fnName)
	failed.Declaration = out.Declaration
	failed.Diagnostics = plxerr.Diagnostics{plxerr.NewDiagnostic(plxerr.FunctionBuildFailed, plxerr.Location{}, "boom")}
	failed.Module = nil
	w.builder.set(failed, nil)
	bad := w.built(t, "development", fnName)
	if bad.State != function.BuildFailed || w.declarationOf(t, bad.ID) != "" {
		t.Errorf("a failed build stored a declaration or did not fail: %s", bad.State)
	}
}

// Verifies: FN-016.
// A build whose job gave up is failed with a diagnostic, so none stays
// running; a finished build is left alone.
func TestFailBuildEndsABuildThatNeverRan_FN_016(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	w.builder.set(goodOutput(fnName), nil)
	w.signer.err = errors.New("kms unavailable at 10.0.0.7")
	src, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], fnName, strings.NewReader("source"))
	if err != nil {
		t.Fatal(err)
	}
	queued, err := w.svc.RequestBuild(ctx, w.developer, src.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	job := w.queue.last(t)
	for range 3 {
		if err := w.svc.RunBuild(ctx, job); err == nil {
			t.Fatal("a signer fault did not return an error")
		}
	}
	if b, _ := w.svc.GetBuild(ctx, w.developer, queued.ID); b.State != function.BuildRunning {
		t.Fatalf("before the job gives up the build is %s", b.State)
	}
	if err := w.svc.FailBuild(ctx, job, 3); err != nil {
		t.Fatal(err)
	}
	b, err := w.svc.GetBuild(ctx, w.developer, queued.ID)
	if err != nil || b.State != function.BuildFailed || len(b.Diagnostics) != 1 || b.Diagnostics[0].Code != plxerr.FunctionBuildFailed {
		t.Fatalf("build = %+v, %v", b, err)
	}
	if strings.Contains(b.Diagnostics[0].Message, "10.0.0.7") || !strings.Contains(b.Diagnostics[0].Message, "3 attempts") {
		t.Errorf("the diagnostic %q should state the attempts and keep the cause out of the tenant's sight", b.Diagnostics[0].Message)
	}
	if err := w.svc.FailBuild(ctx, job, 3); err != nil {
		t.Errorf("failing a finished build twice: %v", err)
	}

	w.signer.err = nil
	done := w.built(t, "development", fnName)
	if err := w.svc.FailBuild(ctx, w.queue.last(t), 5); err != nil {
		t.Fatal(err)
	}
	if again, _ := w.svc.GetBuild(ctx, w.developer, done.ID); again.State != function.BuildSucceeded {
		t.Errorf("FailBuild changed a succeeded build to %s", again.State)
	}
}

// Verifies: LIM-001, FN-016.
// The build timeout is in milliseconds in the registry and reaches the
// builder as the duration it names.
func TestBuilderIsGivenTheBuildTimeoutInMilliseconds_LIM_001(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil, func(s *limits.Set) {
		tight, err := s.Tighten(limits.FunctionBuildTimeout, limits.ScopeApp, 90_000)
		if err != nil {
			t.Fatal(err)
		}
		*s = tight
	})
	w.builder.set(goodOutput(fnName), nil)
	w.built(t, "development", fnName)
	if got := w.builder.calls[0].Timeout; got != 90*time.Second {
		t.Errorf("timeout = %s; function.buildTimeout = 90000 ms is 90 s", got)
	}
}

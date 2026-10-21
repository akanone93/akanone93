// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/approval"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

// engine returns an approval engine over the world's database whose
// environments all have the given security profile.
func (w *world) engine(t *testing.T, profile settings.Profile) *approval.Service {
	t.Helper()
	svc, err := approval.New(approval.Options{DB: w.db, Audit: w.opts.Audit, IDs: w.opts.IDs})
	if err != nil {
		t.Fatal(err)
	}
	svc.Use(func(context.Context, string, string) (settings.Profile, error) { return profile, nil }, nil)
	return svc
}

// requestOf returns the request a refusal names.
func requestOf(t *testing.T, err error) string {
	t.Helper()
	if code(err) != plxerr.ApprovalRequired {
		t.Fatalf("want an approval to be required, got %v", err)
	}
	var e *plxerr.Error
	if !errors.As(err, &e) || e.Details["request"] == "" {
		t.Fatalf("the refusal names no request: %v", err)
	}
	return e.Details["request"]
}

// Verifies: FN-007, GOV-020, GOV-021.
// Under the maximum profile a function deployed to production needs the
// approval of two people besides the one who deploys it, whether it is
// deployed from a build, promoted from another environment or an alias is
// moved; a development environment needs none, a standard profile asks
// nothing of a function, and the permission and step-up of FN-007 still apply.
func TestProductionDeployNeedsApproval_FN_007(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	w.builder.set(goodOutput(fnName), nil)
	engine := w.engine(t, settings.Maximum)
	svc := w.service(t, func(o *function.Options) { o.Approvals = engine })
	first, second := w.member(t, "r1@example.com", "reviewer"), w.member(t, "r2@example.com", "reviewer")
	approveBy := func(request string, by ...auth.Principal) {
		t.Helper()
		for _, p := range by {
			if _, err := engine.Decide(ctx, p, w.app, request, approval.DecisionApprove, ""); err != nil {
				t.Fatalf("approving %s: %v", request, err)
			}
		}
	}

	build := w.built(t, "production", fnName)
	if _, err := svc.Deploy(ctx, w.developer, build.ID, ""); code(err) != plxerr.PermissionDenied {
		t.Errorf("a developer deploying to production is refused before approval: %v", err)
	}
	_, err := svc.Deploy(ctx, w.owner, build.ID, "prod")
	request := requestOf(t, err)
	approveBy(request, first)
	_, err = svc.Deploy(ctx, w.owner, build.ID, "prod")
	requestOf(t, err)
	approveBy(request, second)
	v, err := svc.Deploy(ctx, w.owner, build.ID, "prod")
	if err != nil || v.Version != 1 {
		t.Fatalf("the approved deploy: %v %+v", err, v)
	}
	if _, err := svc.Deploy(ctx, w.owner, build.ID, "prod"); code(err) != plxerr.ApprovalRequired {
		t.Errorf("an approval is spent by the deploy it approves: %v", err)
	}

	_, err = svc.SetAlias(ctx, w.owner, w.app, w.envs["production"], fnName, "stable", 1)
	approveBy(requestOf(t, err), first, second)
	if _, err := svc.SetAlias(ctx, w.owner, w.app, w.envs["production"], fnName, "stable", 1); err != nil {
		t.Errorf("the approved alias move: %v", err)
	}

	dev := w.built(t, "development", fnName)
	if _, err := svc.Deploy(ctx, w.developer, dev.ID, ""); err != nil {
		t.Errorf("a development deploy asks for no approval: %v", err)
	}
	_, err = svc.Promote(ctx, w.owner, w.app, w.envs["development"], w.envs["staging"], fnName, 2, "")
	if err != nil {
		t.Errorf("a promotion to a non-production environment asks for no approval: %v", err)
	}

	_, err = svc.Promote(ctx, w.owner, w.app, w.envs["development"], w.envs["production"], fnName, 2, "")
	approveBy(requestOf(t, err), first, second)
	if _, err := svc.Promote(ctx, w.owner, w.app, w.envs["development"], w.envs["production"], fnName, 2, ""); err != nil {
		t.Errorf("the approved promotion to production: %v", err)
	}

	standard := w.service(t, func(o *function.Options) { o.Approvals = w.engine(t, settings.Standard) })
	src, err := w.svc.UploadSource(ctx, w.developer, w.otherApp, w.otherEnvs["production"], fnName, strings.NewReader("source"))
	if err != nil {
		t.Fatal(err)
	}
	queued, err := w.svc.RequestBuild(ctx, w.developer, src.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.svc.RunBuild(ctx, w.queue.last(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := standard.Deploy(ctx, w.owner, queued.ID, ""); err != nil {
		t.Errorf("a standard profile asks nothing of a function deploy: %v", err)
	}
}

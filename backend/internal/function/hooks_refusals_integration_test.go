// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// unknownID is a well-formed identifier no row has.
const unknownID = "01b0c450-6c00-7000-8000-0000000000aa"

// Verifies: FN-025, GOV-001.
// Hooks and triggers are changed only by those who hold function.deploy; a
// caller who holds it nowhere learns nothing about which identifiers exist,
// and malformed or unknown identifiers are refused in the same way for
// everyone who may ask.
func TestHookAndTriggerRefusals_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	hook, _, err := tw.svc.CreateHook(ctx, tw.developer, function.HookSpec{AppID: tw.app, EnvironmentID: tw.envs["development"], Function: fnName, Alias: "live"})
	if err != nil {
		t.Fatal(err)
	}
	trig, err := tw.svc.CreateTrigger(ctx, tw.developer, tw.spec("* * * * *", ""))
	if err != nil {
		t.Fatal(err)
	}

	wantServerCode(t, tw.svc.DeleteHook(ctx, tw.viewer, hook.ID), plxerr.PermissionDenied, "a viewer deleting a hook")
	wantServerCode(t, tw.svc.DeleteTrigger(ctx, tw.viewer, trig.ID), plxerr.PermissionDenied, "a viewer deleting a trigger")
	_, err = tw.svc.RotateHookSecret(ctx, tw.viewer, hook.ID, 0)
	wantServerCode(t, err, plxerr.PermissionDenied, "a viewer rotating a secret")
	_, _, err = tw.svc.CreateHook(ctx, tw.viewer, function.HookSpec{AppID: tw.app, EnvironmentID: tw.envs["development"], Function: fnName, Alias: "live"})
	wantServerCode(t, err, plxerr.PermissionDenied, "a viewer creating a hook")

	wantServerCode(t, tw.svc.DeleteHook(ctx, tw.developer, "not-a-uuid"), plxerr.InvalidFormat, "a malformed hook identifier")
	wantServerCode(t, tw.svc.DeleteHook(ctx, tw.developer, unknownID), plxerr.ResourceNotFound, "an unknown hook")
	_, err = tw.svc.RotateHookSecret(ctx, tw.developer, unknownID, 0)
	wantServerCode(t, err, plxerr.ResourceNotFound, "rotating an unknown hook")
	wantServerCode(t, tw.svc.DeleteTrigger(ctx, tw.developer, unknownID), plxerr.ResourceNotFound, "an unknown trigger")

	hooks, err := tw.svc.ListHooks(ctx, tw.developer, tw.app)
	if err != nil || len(hooks) != 1 {
		t.Fatalf("hooks = %+v, %v", hooks, err)
	}
	if err := tw.svc.DeleteHook(ctx, tw.developer, hook.ID); err != nil {
		t.Fatal(err)
	}
	wantServerCode(t, tw.svc.DeleteHook(ctx, tw.developer, hook.ID), plxerr.ResourceNotFound, "deleting a hook twice")
	body := `{"event":"paid"}`
	if rec := hookRequest(tw.svc, tw.app, fnName, body, http.Header{}); rec.Code != http.StatusNotFound {
		t.Errorf("a deleted hook still answers: %d", rec.Code)
	}
}

// Verifies: FN-025.
// Forgetting message identifiers and trigger fires past their retention
// keeps what is fresh, and never fails on an empty store.
func TestPurgeHooks_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	if n, err := tw.svc.PurgeHooks(ctx, tw.org); err != nil || n != 0 {
		t.Fatalf("purging nothing: %d, %v", n, err)
	}
	if _, err := tw.svc.CreateTrigger(ctx, tw.developer, tw.spec("* * * * *", "")); err != nil {
		t.Fatal(err)
	}
	tw.clock.advance(time.Minute)
	if n, err := tw.svc.RunTriggers(ctx, tw.org); err != nil || n != 1 {
		t.Fatalf("the trigger ran %d times, %v", n, err)
	}
	if n, err := tw.svc.PurgeHooks(ctx, tw.org); err != nil || n != 0 {
		t.Fatalf("a fresh fire was purged: %d, %v", n, err)
	}
}

// Verifies: FN-025.
// A hook with an oversized body is refused before the signature is read, and
// a body that is not a hook of the app is not found.
func TestHookBodyLimit_FN_025(t *testing.T) {
	t.Parallel()
	tw := newTriggerWorld(t)
	ctx := context.Background()
	if _, _, err := tw.svc.CreateHook(ctx, tw.developer, function.HookSpec{AppID: tw.app, EnvironmentID: tw.envs["development"], Function: fnName, Alias: "live"}); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("x", 8<<20)
	rec := hookRequest(tw.svc, tw.app, fnName, huge, http.Header{})
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Errorf("an oversized body: %d %s", rec.Code, rec.Body)
	}
	if got := tw.invoker.count(); got != 0 {
		t.Errorf("the function ran %d times for an unsigned body", got)
	}
}

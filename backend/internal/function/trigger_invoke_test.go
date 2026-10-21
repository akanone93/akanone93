// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// serverCall is a trigger's call of the charge function.
func serverCall(input string) function.ServerCall {
	return function.ServerCall{
		OrganizationID: "org", AppID: "app", EnvironmentID: "env", Function: charge, Alias: "live", Input: []byte(input),
	}
}

// wantServerCode fails unless err carries the code.
func wantServerCode(t *testing.T, err error, want plxerr.Code, what string) {
	t.Helper()
	if got, ok := plxerr.CodeOf(err); !ok || got != want {
		t.Errorf("%s: want %v, got %v", what, want, err)
	}
}

// Verifies: FN-025.
// A trigger runs a server function at AL0 and reaches nothing a caller
// without credentials could not: functions that need a device, a higher
// assurance, or that are not on the server are refused before the runner.
func TestInvokeServerRefusals_FN_025(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig(t, nil)
	r.assured = "AL0"
	r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	r.cat.add("payments.sca", 1, function.PlacementServer, function.Declaration{SCA: true})
	r.cat.add("payments.attested", 1, function.PlacementServer, function.Declaration{Attestation: true})
	r.cat.add("payments.strong", 1, function.PlacementServer, function.Declaration{Assurance: "AL2"})
	r.cat.add("payments.local", 1, function.PlacementDevice, function.Declaration{})

	if res, err := r.inv.InvokeServer(ctx, serverCall(`{"n":1}`)); err != nil || string(res.Output) != `{"ok":true}` || res.Version != 1 {
		t.Fatalf("a plain function: %+v, %v", res, err)
	}
	for name, want := range map[string]plxerr.Code{
		"payments.sca":      plxerr.AssuranceInsufficient,
		"payments.attested": plxerr.AssuranceInsufficient,
		"payments.strong":   plxerr.AssuranceInsufficient,
		"payments.local":    plxerr.FunctionNotFound,
		"payments.missing":  plxerr.FunctionNotFound,
		"Not A Name":        plxerr.FunctionNotFound,
	} {
		c := serverCall(`{}`)
		c.Function = name
		_, err := r.inv.InvokeServer(ctx, c)
		wantServerCode(t, err, want, name)
	}
	if got := r.runner.calls(); got != 1 {
		t.Errorf("the runner ran %d times, want only the plain function", got)
	}
}

// Verifies: FN-025, SEC-007.
// A function that declares no assurance takes the environment's default, and
// a trigger, which runs at AL0, is refused when the default is higher or
// cannot be read.
func TestInvokeServerDefaultAssurance_FN_025(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig(t, nil)
	r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	_, err := r.inv.InvokeServer(ctx, serverCall(`{}`))
	wantServerCode(t, err, plxerr.AssuranceInsufficient, "an environment default of AL1")

	boom := errors.New("environment store down")
	failing := newRig(t, func(_ *rig, o *function.InvokerOptions) {
		o.DefaultAssurance = func(context.Context, string, string) (string, error) { return "", boom }
	})
	failing.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	if _, err := failing.inv.InvokeServer(ctx, serverCall(`{}`)); !errors.Is(err, boom) {
		t.Errorf("an unreadable default was %v", err)
	}
}

// Verifies: FN-025, LIM-001.
// The input limit and the limits lookup apply to a trigger as to a device.
func TestInvokeServerLimits_FN_025(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig(t, nil)
	r.assured = "AL0"
	r.cat.add(charge, 1, function.PlacementServer, function.Declaration{})
	r.tighten(t, limits.FunctionInputSize, 4)
	_, err := r.inv.InvokeServer(ctx, serverCall(`{"too":"long"}`))
	wantServerCode(t, err, plxerr.FunctionInputTooLarge, "an oversized input")

	boom := errors.New("limits store down")
	failing := newRig(t, func(_ *rig, o *function.InvokerOptions) {
		o.Limits = func(context.Context, string, string) (limits.Set, error) { return limits.Set{}, boom }
	})
	if _, err := failing.inv.InvokeServer(ctx, serverCall(`{}`)); !errors.Is(err, boom) {
		t.Errorf("an unreadable limit set was %v", err)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// ServerCall is an invocation the server makes on its own account: a
// schedule fired, or a signed inbound webhook arrived (FN-025).
type ServerCall struct {
	OrganizationID string
	AppID          string
	EnvironmentID  string
	Function       string
	// Alias names the version to run; it is resolved at the time of the
	// call, so moving the alias moves the trigger (FN-006).
	Alias       string
	Input       []byte
	Traceparent string
}

// ServerInvoker runs functions for triggers; *Invoker is one.
type ServerInvoker interface {
	InvokeServer(ctx context.Context, c ServerCall) (Result, error)
}

// serverAssurance is the assurance a trigger's invocation runs with: the
// lowest. There is no device behind it, so a function that requires more
// than AL0, attestation or a confirmed transaction (SCA) is refused rather
// than run with privileges nobody presented (SEC-080).
const serverAssurance = string(device.AL0)

// InvokeServer runs a server-placed function for a trigger (FN-025). It is
// the device path without the device: the same resolution, declaration,
// limits, rate limits, runner and observation, but never user claims,
// platform evidence or a confirmed transaction. A function that needs any
// of them, or an assurance above AL0, is PLX-6002 or PLX-7003 — a trigger
// carries no credentials, so it can reach nothing a caller without
// credentials could not.
func (i *Invoker) InvokeServer(ctx context.Context, c ServerCall) (Result, error) {
	lim, err := i.o.Limits(ctx, c.OrganizationID, c.AppID)
	if err != nil {
		return Result{}, err
	}
	if most := lim.Get(limits.FunctionInputSize); int64(len(c.Input)) > most {
		return Result{}, plxerr.New(plxerr.FunctionInputTooLarge, "the input is at most function.inputSize = %d bytes", most)
	}
	if ValidateName(c.Function) != nil {
		return Result{}, plxerr.New(plxerr.FunctionNotFound, "no such function")
	}
	res, err := i.o.Resolver.ResolveVersion(ctx, c.OrganizationID, c.AppID, c.EnvironmentID, c.Function, c.Alias)
	if err != nil {
		return Result{}, err //nolint:wrapcheck // a domain error
	}
	if res.Placement != PlacementServer {
		return Result{}, plxerr.New(plxerr.FunctionNotFound, "the function %s does not run on the server", c.Function)
	}
	inv, err := i.o.Declarations.LoadInvocable(ctx, c.OrganizationID, c.AppID, c.EnvironmentID, c.Function, res.Version)
	if err != nil {
		return Result{}, err //nolint:wrapcheck // a domain error
	}
	if err := i.serverAllowed(ctx, c, inv.Declaration); err != nil {
		return Result{}, err
	}
	for _, r := range []struct {
		key   string
		limit int64
	}{
		{"rate:fn:app:" + c.AppID, lim.Get(limits.FunctionInvocationsPerMinutePerApp)},
		{"rate:fn:" + c.EnvironmentID + ":" + c.Function, lim.Get(limits.FunctionInvocationsPerMinute)},
	} {
		if err := i.rate(ctx, r.key, r.limit); err != nil {
			return Result{}, err
		}
	}
	spec := runSpec{
		org: c.OrganizationID, app: c.AppID, env: c.EnvironmentID, inv: inv, input: c.Input,
		assurance: serverAssurance, traceparent: c.Traceparent,
	}
	start := i.o.Now()
	resp, err := i.run(ctx, lim, spec)
	took := i.o.Now().Sub(start)
	if err != nil {
		i.observe(ctx, spec, resultName(Result{}, err), took, 0, false)
		return Result{}, err
	}
	out, err := resultOf(resp, res.Version)
	i.observe(ctx, spec, resultName(out, err), took, resp.GetMemoryPeakBytes(), false)
	return out, err
}

// serverAllowed refuses a function whose declaration asks for what a
// trigger cannot present.
func (i *Invoker) serverAllowed(ctx context.Context, c ServerCall, decl Declaration) error {
	if decl.SCA || decl.Attestation {
		return plxerr.New(plxerr.AssuranceInsufficient, "the function %s needs a device (sca or attestation), and a trigger has none", c.Function)
	}
	need := decl.Assurance
	if need == "" {
		var err error
		if need, err = i.o.DefaultAssurance(ctx, c.AppID, c.EnvironmentID); err != nil {
			return err
		}
	}
	if device.Level(need).Rank() != 0 {
		return plxerr.New(plxerr.AssuranceInsufficient,
			"the function %s needs assurance %s; a trigger runs at AL0, so declare assurance AL0 for a function a trigger may call", c.Function, need)
	}
	return nil
}

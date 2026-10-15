// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"slices"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Nested runs a function for another function that is already running
// (FN-012, ADR-0064). The caller is the function the grant names; the
// callee must be in its declared capabilities.functions, the depth must
// be the caller's plus one and within function.callDepth (PLX-7005), and
// the callee runs on the path a device's invocation takes, with the
// caller's assurance and user claims. The callee is the latest version of
// the name in the caller's environment; it must be server-placed.
func (i *Invoker) Nested(ctx context.Context, g Grant, callee string, input []byte, depth uint32, traceparent string) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
	claims, err := i.o.Grants.Verify(g)
	if err != nil {
		return nil, err
	}
	caller, err := i.o.Declarations.LoadInvocable(ctx, claims.OrganizationID, claims.AppID, claims.EnvironmentID, claims.Function, claims.Version)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	if !slices.Contains(caller.Declaration.Capabilities.Functions, callee) {
		return nil, plxerr.New(plxerr.FunctionCapabilityDenied, "the function %s does not declare a call to %s", claims.Function, callee)
	}
	if depth != claims.Depth+1 {
		return nil, plxerr.New(plxerr.InvalidStructure, "a nested call runs one level below its caller")
	}
	lim, err := i.o.Limits(ctx, claims.OrganizationID, claims.AppID)
	if err != nil {
		return nil, err
	}
	if int64(depth) > lim.Get(limits.FunctionCallDepth) {
		return nil, plxerr.New(plxerr.FunctionCallDepthExceeded, "function calls nest at most function.callDepth = %d levels", lim.Get(limits.FunctionCallDepth))
	}
	if most := lim.Get(limits.FunctionInputSize); int64(len(input)) > most {
		return nil, plxerr.New(plxerr.FunctionInputTooLarge, "the input is at most function.inputSize = %d bytes", most)
	}
	inv, err := i.callee(ctx, claims, callee)
	if err != nil {
		return nil, err
	}
	for _, r := range []struct {
		key   string
		limit int64
	}{
		{"rate:fn:app:" + claims.AppID, lim.Get(limits.FunctionInvocationsPerMinutePerApp)},
		{"rate:fn:" + claims.EnvironmentID + ":" + callee, lim.Get(limits.FunctionInvocationsPerMinute)},
	} {
		if err := i.rate(ctx, r.key, r.limit); err != nil {
			return nil, err
		}
	}
	return i.run(ctx, lim, runSpec{
		org: claims.OrganizationID, app: claims.AppID, env: claims.EnvironmentID, inv: inv, input: input, depth: depth,
		assurance: claims.Assurance, claims: claims.UserClaims, traceparent: traceparent,
	})
}

// callee resolves the function a nested call names.
func (i *Invoker) callee(ctx context.Context, c GrantClaims, name string) (Invocable, error) {
	res, err := i.o.Resolver.ResolveVersion(ctx, c.OrganizationID, c.AppID, c.EnvironmentID, name, "")
	if err != nil {
		return Invocable{}, err //nolint:wrapcheck // a domain error
	}
	if res.Placement != PlacementServer {
		return Invocable{}, plxerr.New(plxerr.FunctionNotFound, "the function %s does not run on the server", name)
	}
	return i.o.Declarations.LoadInvocable(ctx, c.OrganizationID, c.AppID, c.EnvironmentID, name, res.Version) //nolint:wrapcheck // a domain error
}

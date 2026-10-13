// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// ConnectRunner calls the function runner over the internal Connect
// service (ADR-0064). It is safe for concurrent use.
type ConnectRunner struct {
	Client fnpbconnect.FunctionRunnerServiceClient
}

var _ Runner = ConnectRunner{}

// Run calls FunctionRunnerService.Invoke. A failure is returned as a
// *plxerr.Error with the runner's Plux code and the registry's generic
// title as its message: the runner's own text, which may quote what the
// function trapped on, never reaches a device (FN-022).
func (r ConnectRunner) Run(ctx context.Context, req *fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
	res, err := r.Client.Invoke(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, systemError(ctx, err)
	}
	return res.Msg, nil
}

// systemError turns the runner's failure into a system error (FN-022).
func systemError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return plxerr.New(plxerr.FunctionTimeout, "the function exceeded its time limit")
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return plxerr.New(plxerr.FunctionCancelled, "the invocation was cancelled")
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return plxerr.Wrap(plxerr.UpstreamUnavailable, err, "the function runner is unavailable")
	}
	if code, ok := runnerCode(ce); ok {
		title := "the function failed"
		if def, found := plxerr.Lookup(code); found {
			title = def.Title
		}
		return plxerr.New(code, "%s", title)
	}
	if ce.Code() == connect.CodeUnavailable || ce.Code() == connect.CodeUnknown {
		return plxerr.Wrap(plxerr.UpstreamUnavailable, err, "the function runner is unavailable")
	}
	return plxerr.Wrap(plxerr.InternalServerError, err, "the function runner failed")
}

// runnerCode reads the Plux code the runner put in its ErrorInfo detail.
// Only codes of the function area are accepted: the runner speaks for
// functions, and nothing else it says is believed.
func runnerCode(ce *connect.Error) (plxerr.Code, bool) {
	for _, d := range ce.Details() {
		v, err := d.Value()
		if err != nil {
			continue
		}
		info, ok := v.(*errdetails.ErrorInfo)
		if !ok {
			continue
		}
		code, err := plxerr.ParseCode(info.GetMetadata()["code"])
		if err == nil && code >= 7000 && code < 8000 && code != plxerr.FunctionBusinessError {
			return code, true
		}
	}
	return 0, false
}

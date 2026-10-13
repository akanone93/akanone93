// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"regexp"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// FunctionService serves the device's invocation of server functions
// (FN-020).
type FunctionService struct {
	pluxv1connect.UnimplementedFunctionServiceHandler
	h *Handlers
}

var _ pluxv1connect.FunctionServiceHandler = FunctionService{}

// FunctionService returns the FunctionService handler.
func (h *Handlers) FunctionService() FunctionService { return FunctionService{h: h} }

// traceparentForm is the W3C trace context header (version 00).
var traceparentForm = regexp.MustCompile(`^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

// traceparentOf returns the request's trace context, or "" when it is not
// a valid one (FN-032).
func traceparentOf(h http.Header) string {
	if v := h.Get("Traceparent"); traceparentForm.MatchString(v) {
		return v
	}
	return ""
}

// Invoke runs one pinned version of a server function for the
// authenticated device (FN-020). The device's identity, assurance and
// token binding come from the same authentication as GatewayService.Call;
// the domain checks the pin, the placement, the assurance, the
// attestation, the idempotency key and SCA before the module runs. A
// business error is part of the response (FN-022); a system error is a
// Connect error with its PLX code and a generic message. Nothing in this
// API accepts a result computed on a device (FN-054).
func (s FunctionService) Invoke(ctx context.Context, req *connect.Request[pluxv1.FunctionServiceInvokeRequest]) (*connect.Response[pluxv1.FunctionServiceInvokeResponse], error) {
	if s.h.FunctionInvoker == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("this server runs no server functions"))
	}
	d, err := deviceOf(ctx)
	if err != nil {
		return nil, err
	}
	m := req.Msg
	if m.GetVersion() == 0 || m.GetVersion() > math.MaxInt32 {
		return nil, plxerr.New(plxerr.FunctionNotFound, "no such function version")
	}
	res, err := s.h.FunctionInvoker.Invoke(ctx,
		function.Caller{Device: d, UserToken: req.Header().Get(headerUserToken), Traceparent: traceparentOf(req.Header())},
		function.Call{
			Function: m.GetFunction(), Version: int32(m.GetVersion()), //nolint:gosec // bounded above Input: m.GetInput(), IdempotencyKey: m.GetIdempotencyKey(),
			ChallengeID: m.GetTransactionChallengeId(), Signature: m.GetTransactionSignature(),
		})
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	out := &pluxv1.FunctionServiceInvokeResponse{Version: uint32(res.Version), Replayed: res.Replayed} //nolint:gosec // checked above
	if res.Business != nil {
		out.Result = &pluxv1.FunctionServiceInvokeResponse_BusinessError{BusinessError: &pluxv1.FunctionBusinessError{
			Code: res.Business.Code, MessageKey: res.Business.MessageKey, Params: res.Business.Params,
		}}
	} else {
		out.Result = &pluxv1.FunctionServiceInvokeResponse_Output{Output: res.Output}
	}
	return connect.NewResponse(out), nil
}

// Invoke runs a nested call for the function the grant names (FN-012).
func (s FunctionHost) Invoke(ctx context.Context, req *connect.Request[fnpb.FunctionHostServiceInvokeRequest]) (*connect.Response[fnpb.FunctionHostServiceInvokeResponse], error) {
	host, err := s.host()
	if err != nil {
		return nil, err
	}
	m := req.Msg
	res, err := host.Invoke(ctx, function.Grant(m.GetGrant()), m.GetFunction(), m.GetInput(), m.GetDepth(), m.GetTraceparent())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&fnpb.FunctionHostServiceInvokeResponse{Result: res}), nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
)

// FunctionHost serves FunctionHostService, the api role's side of what the
// function runner may ask while it runs an invocation (ADR-0064). It is
// mounted on the internal listener, which only a role holding a
// certificate of the internal CA reaches (SEC-043); every call is also
// authorised by the invocation's grant, which the domain checks (FN-012).
type FunctionHost struct {
	fnpbconnect.UnimplementedFunctionHostServiceHandler
	h *Handlers
}

var _ fnpbconnect.FunctionHostServiceHandler = FunctionHost{}

// FunctionHost returns the FunctionHostService handler.
func (h *Handlers) FunctionHost() FunctionHost { return FunctionHost{h: h} }

// host returns the domain host, or an Unimplemented error where the
// process has no function service.
func (s FunctionHost) host() (*function.Host, error) {
	if s.h.FunctionHosting == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("this server runs no function host"))
	}
	return s.h.FunctionHosting, nil
}

// GetModule returns the module the grant was issued for, with the signed
// statement the runner verifies (FN-009, FN-010).
func (s FunctionHost) GetModule(ctx context.Context, req *connect.Request[fnpb.GetModuleRequest]) (*connect.Response[fnpb.GetModuleResponse], error) {
	host, err := s.host()
	if err != nil {
		return nil, err
	}
	a, err := host.GetModule(ctx, function.Grant(req.Msg.GetGrant()), req.Msg.GetModuleSha256())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&fnpb.GetModuleResponse{Module: a.Module, Statement: a.Statement, Signature: a.Signature, KeyId: a.KeyID}), nil
}

// KvGet reads a key of the grant's namespace (FN-018).
func (s FunctionHost) KvGet(ctx context.Context, req *connect.Request[fnpb.KvGetRequest]) (*connect.Response[fnpb.KvGetResponse], error) {
	host, err := s.host()
	if err != nil {
		return nil, err
	}
	v, found, err := host.KVGet(ctx, function.Grant(req.Msg.GetGrant()), req.Msg.GetKey())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&fnpb.KvGetResponse{Found: found, Value: v}), nil
}

// KvPut writes a key of the grant's namespace (FN-018).
func (s FunctionHost) KvPut(ctx context.Context, req *connect.Request[fnpb.KvPutRequest]) (*connect.Response[fnpb.KvPutResponse], error) {
	host, err := s.host()
	if err != nil {
		return nil, err
	}
	if err := host.KVPut(ctx, function.Grant(req.Msg.GetGrant()), req.Msg.GetKey(), req.Msg.GetValue()); err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&fnpb.KvPutResponse{}), nil
}

// KvDelete removes a key of the grant's namespace (FN-018).
func (s FunctionHost) KvDelete(ctx context.Context, req *connect.Request[fnpb.KvDeleteRequest]) (*connect.Response[fnpb.KvDeleteResponse], error) {
	host, err := s.host()
	if err != nil {
		return nil, err
	}
	deleted, err := host.KVDelete(ctx, function.Grant(req.Msg.GetGrant()), req.Msg.GetKey())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&fnpb.KvDeleteResponse{Deleted: deleted}), nil
}

// GetSecret returns a secret the function declares by name (FN-012).
func (s FunctionHost) GetSecret(ctx context.Context, req *connect.Request[fnpb.GetSecretRequest]) (*connect.Response[fnpb.GetSecretResponse], error) {
	host, err := s.host()
	if err != nil {
		return nil, err
	}
	v, found, err := host.Secret(ctx, function.Grant(req.Msg.GetGrant()), req.Msg.GetName())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&fnpb.GetSecretResponse{Found: found, Value: v}), nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// editorRecheck is how often an open editor stream re-checks its credential
// (SEC-101): a revoked or expired session ends the sandbox.
const editorRecheck = 30 * time.Second

// LanguageServer serves LanguageServerService: the bridge between the Rust
// editor and a rust-analyzer in the build sandbox (FN-033, ADR-0099). The
// handler authenticates and translates; the function service authorises on the
// source's app, bounds the sessions and audits them, and the builder role runs
// the sandbox.
type LanguageServer struct {
	pluxv1connect.UnimplementedLanguageServerServiceHandler
	h *Handlers
}

var _ pluxv1connect.LanguageServerServiceHandler = LanguageServer{}

// LanguageServer returns the LanguageServerService handler.
func (h *Handlers) LanguageServer() LanguageServer { return LanguageServer{h: h} }

// Open starts a sandboxed language server on an uploaded source and streams
// its messages.
func (s LanguageServer) Open(ctx context.Context, req *connect.Request[pluxv1.LanguageServerServiceOpenRequest], stream *connect.ServerStream[pluxv1.LanguageServerServiceOpenResponse]) error {
	if s.h.Functions == nil {
		return connect.NewError(connect.CodeUnimplemented, errors.New("this server runs no function service"))
	}
	p, err := s.h.principal(ctx, req.Header(), "")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go s.recheck(ctx, cancel, req.Header())
	err = s.h.Functions.OpenEditor(ctx, p, req.Msg.GetSourceId(), func(ev function.EditorEvent) error {
		out := &pluxv1.LanguageServerServiceOpenResponse{}
		switch {
		case ev.Opened != nil:
			out.Event = &pluxv1.LanguageServerServiceOpenResponse_Opened{Opened: &pluxv1.LanguageServerOpened{
				SessionId: ev.Opened.SessionID, RootUri: ev.Opened.RootURI, SessionSeconds: ev.Opened.Seconds,
			}}
		case ev.Closed != nil:
			out.Event = &pluxv1.LanguageServerServiceOpenResponse_Closed{Closed: &pluxv1.LanguageServerClosed{Reason: ev.Closed.Reason, Detail: ev.Closed.Detail}}
		default:
			out.Event = &pluxv1.LanguageServerServiceOpenResponse_Message{Message: ev.Message}
		}
		return stream.Send(out) //nolint:wrapcheck // the stream's own error
	})
	return editorFailure(err)
}

// recheck ends the stream when its credential stops being valid; it asks
// quietly, so the stream does not extend the session (O-1).
func (s LanguageServer) recheck(ctx context.Context, cancel context.CancelFunc, header http.Header) {
	if s.h.Authenticator == nil {
		return
	}
	tick := time.NewTicker(editorRecheck)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			id, ok, err := credential(ctx, s.h.Authenticator, pluxv1connect.LanguageServerServiceOpenProcedure, header)
			if err != nil || !ok {
				cancel()
				return
			}
			if _, err := s.h.resolve(ctx, id, header, ""); err != nil {
				cancel()
				return
			}
		}
	}
}

// Send delivers one client message to the session's language server.
func (s LanguageServer) Send(ctx context.Context, req *connect.Request[pluxv1.LanguageServerServiceSendRequest]) (*connect.Response[pluxv1.LanguageServerServiceSendResponse], error) {
	if s.h.Functions == nil {
		return s.UnimplementedLanguageServerServiceHandler.Send(ctx, req) //nolint:wrapcheck // the generated Unimplemented answer
	}
	p, err := s.h.principal(ctx, req.Header(), "")
	if err != nil {
		return nil, err
	}
	m := req.Msg
	if err := s.h.Functions.SendEditor(ctx, p, m.GetSessionId(), m.GetSequence(), m.GetMessage()); err != nil {
		return nil, editorFailure(err)
	}
	return connect.NewResponse(&pluxv1.LanguageServerServiceSendResponse{}), nil
}

// editorFailure passes a Plux error on and turns the builder's own status into
// a Connect error without the role's internals.
func editorFailure(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*plxerr.Error](err); ok {
		return err
	}
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		switch ce.Code() {
		case connect.CodeNotFound, connect.CodeFailedPrecondition, connect.CodeInvalidArgument, connect.CodeResourceExhausted, connect.CodeUnimplemented:
			return connect.NewError(ce.Code(), errors.New(ce.Message()))
		default:
			return connect.NewError(connect.CodeUnavailable, errors.New("the builder could not serve the Rust editor; retry"))
		}
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err //nolint:wrapcheck // the interceptor reports it as internal
}

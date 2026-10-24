// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// StudioEvents serves StudioEventsService (STU-075, SRV-062).
type StudioEvents struct{ h *Handlers }

var _ pluxv1connect.StudioEventsServiceHandler = StudioEvents{}

// StudioEvents returns the handler.
func (h *Handlers) StudioEvents() StudioEvents { return StudioEvents{h: h} }

// Watch streams the caller's Studio events after the cursor.
func (s StudioEvents) Watch(ctx context.Context, req *connect.Request[pluxv1.StudioEventsServiceWatchRequest], stream *connect.ServerStream[pluxv1.StudioEventsServiceWatchResponse]) error {
	p, err := s.h.principal(ctx, req.Header(), "")
	if err != nil {
		return err
	}
	header := req.Header()
	// refresh re-checks the credential at every heartbeat, quietly so the
	// stream does not extend the session (O-1), and re-resolves the
	// principal: a revoked or expired session, or lost access, ends the
	// stream (SEC-101, GOV-005).
	refresh := func(ctx context.Context) (auth.Principal, error) {
		if s.h.Authenticator == nil {
			return auth.Principal{}, plxerr.New(plxerr.AuthenticationRequired, "the stream cannot re-check its credential")
		}
		id, ok, err := credential(ctx, s.h.Authenticator, pluxv1connect.StudioEventsServiceWatchProcedure, header)
		if err != nil {
			return auth.Principal{}, err
		}
		if !ok {
			return auth.Principal{}, plxerr.New(plxerr.AuthenticationRequired, "this call needs a session or an access token")
		}
		return s.h.resolve(ctx, id, header, "")
	}
	return s.h.StudioStream.Watch(ctx, p, req.Msg.GetCursor(), refresh, func(m *pluxv1.StudioEventsServiceWatchResponse) error { //nolint:wrapcheck // a domain error
		return stream.Send(m) //nolint:wrapcheck // the stream's own error
	})
}

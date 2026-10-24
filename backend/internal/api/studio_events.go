// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
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
	return s.h.StudioStream.Watch(ctx, p, req.Msg.GetCursor(), func(m *pluxv1.StudioEventsServiceWatchResponse) error { //nolint:wrapcheck // a domain error
		return stream.Send(m) //nolint:wrapcheck // the stream's own error
	})
}

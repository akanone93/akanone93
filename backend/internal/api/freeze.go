// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/freeze"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Freeze serves FreezeService: the change-freeze windows of an organisation
// (GOV-025, ADR-0076).
type Freeze struct{ h *Handlers }

var _ pluxv1connect.FreezeServiceHandler = Freeze{}

// Freeze returns the FreezeService handler.
func (h *Handlers) Freeze() Freeze { return Freeze{h: h} }

// ListFreezes lists the windows of the organisation.
func (s Freeze) ListFreezes(ctx context.Context, req *connect.Request[pluxv1.ListFreezesRequest]) (*connect.Response[pluxv1.ListFreezesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListFreezesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		windows, err := s.h.Freezes.List(ctx, p)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListFreezesResponse{}
		for _, w := range windows {
			out.Freezes = append(out.Freezes, freezeProto(w))
		}
		return out, nil
	})
}

// PutFreeze creates a window or replaces the one of the same name.
func (s Freeze) PutFreeze(ctx context.Context, req *connect.Request[pluxv1.PutFreezeRequest]) (*connect.Response[pluxv1.PutFreezeResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.PutFreezeResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		f := req.Msg.GetFreeze()
		w, err := s.h.Freezes.Put(ctx, p, governance.Window{
			Name: f.GetName(), EnvironmentKey: f.GetEnvironmentKey(), TimeZone: f.GetTimeZone(), Start: f.GetStart(),
			Duration: time.Duration(f.GetDurationMinutes()) * time.Minute, Recurrence: f.GetRecurrence(), Reason: f.GetReason(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.PutFreezeResponse{Freeze: freezeProto(w)}, nil
	})
}

// DeleteFreeze removes a window.
func (s Freeze) DeleteFreeze(ctx context.Context, req *connect.Request[pluxv1.DeleteFreezeRequest]) (*connect.Response[pluxv1.DeleteFreezeResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteFreezeResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Freezes.Delete(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteFreezeResponse{}, nil
	})
}

// freezeProto converts a stored window.
func freezeProto(w freeze.Window) *pluxv1.ChangeFreeze {
	return &pluxv1.ChangeFreeze{
		Id: w.ID, Name: w.Name, EnvironmentKey: w.EnvironmentKey, TimeZone: w.TimeZone, Start: w.Start,
		DurationMinutes: int32(w.Duration / time.Minute), //nolint:gosec // bounded by governance.freezeWindowHours
		Recurrence:      w.Recurrence, Reason: w.Reason, CreatedBy: w.CreatedBy, CreatedAt: ts(w.CreatedAt), UpdatedAt: ts(w.UpdatedAt),
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/breakglass"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// BreakGlass serves BreakGlassService: the list and the post-hoc review of
// emergency actions (GOV-024, ADR-0076).
type BreakGlass struct{ h *Handlers }

var _ pluxv1connect.BreakGlassServiceHandler = BreakGlass{}

// BreakGlass returns the BreakGlassService handler.
func (h *Handlers) BreakGlass() BreakGlass { return BreakGlass{h: h} }

// ListBreakGlass lists the emergency actions of the organisation, newest
// first.
func (s BreakGlass) ListBreakGlass(ctx context.Context, req *connect.Request[pluxv1.ListBreakGlassRequest]) (*connect.Response[pluxv1.ListBreakGlassResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListBreakGlassResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		page := req.Msg.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		recs, err := s.h.Emergencies.List(ctx, p, req.Msg.GetStatus(), after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListBreakGlassResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range recs {
			last = storage.Cursor{Time: r.InvokedAt, ID: r.ID}
			out.Invocations = append(out.Invocations, breakGlassProto(r))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(recs), size, last)
		return out, Mask(page.GetReadMask(), out.Invocations)
	})
}

// ReviewBreakGlass completes the post-hoc review of an emergency action.
func (s BreakGlass) ReviewBreakGlass(ctx context.Context, req *connect.Request[pluxv1.ReviewBreakGlassRequest]) (*connect.Response[pluxv1.ReviewBreakGlassResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ReviewBreakGlassResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		rec, err := s.h.Emergencies.Review(ctx, p, req.Msg.GetId(), req.Msg.GetComment())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ReviewBreakGlassResponse{Invocation: breakGlassProto(rec)}, nil
	})
}

// breakGlassProto converts an invocation and its review.
func breakGlassProto(r breakglass.Record) *pluxv1.BreakGlassInvocation {
	return &pluxv1.BreakGlassInvocation{
		Id: r.ID, AppId: r.AppID, EnvironmentId: r.EnvironmentID, Environment: r.Environment, Action: string(r.Action), Subject: r.Subject,
		Justification: r.Justification, InvokedById: r.InvokedBy.ID, InvokedBy: r.InvokedBy.Display, InvokedAt: ts(r.InvokedAt),
		ReviewDueAt: ts(r.ReviewDueAt), ReviewStatus: r.ReviewStatus, ReviewedBy: r.ReviewedBy, ReviewedAt: ts(r.ReviewedAt),
		ReviewComment: r.ReviewComment, Overdue: r.Overdue,
	}
}

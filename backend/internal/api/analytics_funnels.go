// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// CreateFunnel defines a funnel (ANL-020).
func (s AnalyticsHandler) CreateFunnel(ctx context.Context, req *connect.Request[pluxv1.CreateFunnelRequest]) (*connect.Response[pluxv1.CreateFunnelResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateFunnelResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg.GetFunnel()
		f, err := s.h.Analytics.CreateFunnel(ctx, p, analytics.Funnel{
			AppID: m.GetAppId(), Name: m.GetName(), Steps: m.GetSteps(), Environment: m.GetEnvironment(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateFunnelResponse{Funnel: funnelProto(f)}, nil
	})
}

// DeleteFunnel removes a funnel.
func (s AnalyticsHandler) DeleteFunnel(ctx context.Context, req *connect.Request[pluxv1.DeleteFunnelRequest]) (*connect.Response[pluxv1.DeleteFunnelResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteFunnelResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Analytics.DeleteFunnel(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteFunnelResponse{}, nil
	})
}

// ListFunnels lists funnels.
func (s AnalyticsHandler) ListFunnels(ctx context.Context, req *connect.Request[pluxv1.ListFunnelsRequest]) (*connect.Response[pluxv1.ListFunnelsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListFunnelsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		funnels, err := s.h.Analytics.ListFunnels(ctx, p, req.Msg.GetAppId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListFunnelsResponse{}
		for _, f := range funnels {
			out.Funnels = append(out.Funnels, funnelProto(f))
		}
		return out, nil
	})
}

// GetFunnelResult counts the sessions that reached each step of a funnel.
func (s AnalyticsHandler) GetFunnelResult(ctx context.Context, req *connect.Request[pluxv1.GetFunnelResultRequest]) (*connect.Response[pluxv1.GetFunnelResultResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetFunnelResultResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		f, steps, err := s.h.Analytics.FunnelResult(ctx, p, m.GetId(), m.GetFrom().AsTime(), m.GetTo().AsTime())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetFunnelResultResponse{Funnel: funnelProto(f)}
		for _, st := range steps {
			out.Steps = append(out.Steps, &pluxv1.FunnelStep{Event: st.Event, Events: st.Events, Sessions: st.Sessions, Conversion: st.Conversion})
		}
		return out, nil
	})
}

// GetOverview summarises the apps of the organisation.
func (s AnalyticsHandler) GetOverview(ctx context.Context, req *connect.Request[pluxv1.GetOverviewRequest]) (*connect.Response[pluxv1.GetOverviewResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetOverviewResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		apps, err := s.h.Analytics.Overview(ctx, p, req.Msg.GetFrom().AsTime(), req.Msg.GetTo().AsTime())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetOverviewResponse{}
		for _, a := range apps {
			out.Apps = append(out.Apps, &pluxv1.OverviewApp{
				AppId: a.AppID, Sessions: a.Sessions, ScreenViews: a.ScreenViews, Errors: a.Errors,
				FiringAlerts: int32(a.FiringAlerts), //nolint:gosec // G115: bounded by the rule limit.
			})
		}
		return out, nil
	})
}

func funnelProto(f analytics.Funnel) *pluxv1.Funnel {
	return &pluxv1.Funnel{
		Id: f.ID, AppId: f.AppID, Name: f.Name, Steps: f.Steps, Environment: f.Environment,
		Revision: f.Revision, UpdatedBy: f.UpdatedBy, UpdatedAt: ts(f.UpdatedAt),
	}
}

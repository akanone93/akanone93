// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/golden"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// Golden serves GoldenService (TST-003, ADR-0070).
type Golden struct{ h *Handlers }

var _ pluxv1connect.GoldenServiceHandler = Golden{}

// Golden returns the GoldenService handler.
func (h *Handlers) Golden() Golden { return Golden{h: h} }

// GetBaselines lists the approved image hashes of a plugin by case.
func (s Golden) GetBaselines(ctx context.Context, req *connect.Request[pluxv1.GetBaselinesRequest]) (*connect.Response[pluxv1.GetBaselinesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetBaselinesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		out, err := s.h.Goldens.Baselines(ctx, p, req.Msg.GetAppId(), req.Msg.GetPluginKey())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetBaselinesResponse{ImageSha256: out}, nil
	})
}

// UploadRun records a golden run.
func (s Golden) UploadRun(ctx context.Context, req *connect.Request[pluxv1.UploadRunRequest]) (*connect.Response[pluxv1.UploadRunResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UploadRunResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		run := golden.Run{
			AppID: m.GetAppId(), PluginKey: m.GetPluginKey(), Bundle: m.GetBundleSha256(), App: m.GetAppSha256(),
			Canonical: m.GetCanonical(), Renderer: m.GetRenderer(),
		}
		for _, c := range m.GetCases() {
			run.Cases = append(run.Cases, golden.Case{
				ID: c.GetCaseId(), Frame: c.GetFrame(), Locale: c.GetLocale(), Theme: c.GetTheme(),
				TextScale: c.GetTextScale(), SHA256: c.GetImageSha256(), Image: c.GetImage(),
			})
		}
		id, differences, err := s.h.Goldens.Upload(ctx, p, run)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UploadRunResponse{RunId: id, Differences: int32(differences)}, nil //nolint:gosec // bounded by golden.casesPerRun
	})
}

// ListDifferences lists the cases of a run that differ from the baselines.
func (s Golden) ListDifferences(ctx context.Context, req *connect.Request[pluxv1.ListDifferencesRequest]) (*connect.Response[pluxv1.ListDifferencesResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListDifferencesResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		diffs, err := s.h.Goldens.Differences(ctx, p, req.Msg.GetAppId(), req.Msg.GetRunId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListDifferencesResponse{}
		for _, d := range diffs {
			out.Differences = append(out.Differences, &pluxv1.GoldenDifference{
				CaseId: d.ID, Frame: d.Frame, Locale: d.Locale, Theme: d.Theme, TextScale: d.TextScale,
				ImageSha256: d.SHA256, BaselineSha256: d.Baseline,
			})
		}
		return out, nil
	})
}

// Approve promotes the images of a run's differences to baselines.
func (s Golden) Approve(ctx context.Context, req *connect.Request[pluxv1.ApproveRequest]) (*connect.Response[pluxv1.ApproveResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ApproveResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		approved, err := s.h.Goldens.Approve(ctx, p, req.Msg.GetAppId(), req.Msg.GetRunId(), req.Msg.GetCaseIds())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ApproveResponse{ApprovedCaseIds: approved}, nil
	})
}

// GetStatus says what the publish gate sees for the exact bundles.
func (s Golden) GetStatus(ctx context.Context, req *connect.Request[pluxv1.GetGoldenStatusRequest]) (*connect.Response[pluxv1.GetGoldenStatusResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetGoldenStatusResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		st, err := s.h.Goldens.StatusOf(ctx, p, m.GetAppId(), m.GetPluginKey(), m.GetBundleSha256(), m.GetAppSha256())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetGoldenStatusResponse{Found: st.Found, RunId: st.RunID, Cases: int32(st.Cases), Differences: int32(st.Differences)}, nil //nolint:gosec // bounded by golden.casesPerRun
	})
}

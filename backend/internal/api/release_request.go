// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/releasereq"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// ReleaseRequest serves ReleaseRequestService (STU-100, ADR-0097).
type ReleaseRequest struct{ h *Handlers }

var _ pluxv1connect.ReleaseRequestServiceHandler = ReleaseRequest{}

// ReleaseRequest returns the ReleaseRequestService handler.
func (h *Handlers) ReleaseRequest() ReleaseRequest { return ReleaseRequest{h: h} }

func requestProto(r releasereq.Request) *pluxv1.ReleaseRequest {
	out := &pluxv1.ReleaseRequest{
		Id: r.ID, AppId: r.AppID, EnvironmentId: r.EnvironmentID, Channel: r.Channel, State: string(r.State), Hold: string(r.Hold),
		HoldReason: r.HoldReason, Revision: r.Revision, Acknowledged: r.Acknowledged, ContentHash: r.ContentHash,
		ChangelogNotes: r.ChangelogNotes, Label: r.Label, RolloutPlan: r.RolloutPlan, ApprovalRequestId: r.ApprovalRequestID,
		ApprovedHash: r.ApprovedHash, ReleaseSequence: r.ReleaseSequence, CreatedBy: r.CreatedBy, CreatedByName: r.CreatedByName,
		CreatedAt: timestamppb.New(r.CreatedAt), UpdatedAt: timestamppb.New(r.UpdatedAt),
	}
	if !r.EndedAt.IsZero() {
		out.EndedAt = timestamppb.New(r.EndedAt)
	}
	for _, p := range r.Plugins {
		out.Plugins = append(out.Plugins, &pluxv1.SelectedPlugin{PluginKey: p.PluginKey, Reason: p.Reason})
	}
	for _, b := range r.Builds {
		out.Builds = append(out.Builds, &pluxv1.BuildRef{
			PluginKey: b.PluginKey, JobId: b.JobID, State: b.State, Version: b.Version, BundleSha256: b.BundleSHA256,
			Errors: int32(b.Errors), Warnings: b.Warnings, //nolint:gosec // bounded by the job's diagnostics
		})
	}
	return out
}

func diffProto(d releasereq.VisualDiff) *pluxv1.VisualDiffJob {
	out := &pluxv1.VisualDiffJob{
		Id: d.ID, ReleaseRequestId: d.RequestID, ContentHash: d.ContentHash, State: d.State, Error: d.Error,
		CreatedAt: timestamppb.New(d.CreatedAt),
	}
	if !d.FinishedAt.IsZero() {
		out.FinishedAt = timestamppb.New(d.FinishedAt)
	}
	for _, s := range d.Screens {
		out.Screens = append(out.Screens, &pluxv1.VisualDiffScreen{
			PluginKey: s.PluginKey, Screen: s.Screen, Device: s.Device, BeforeSha256: s.BeforeSHA256, AfterSha256: s.AfterSHA256, Changed: s.Changed,
		})
	}
	return out
}

func selected(in []*pluxv1.SelectedPlugin) []releasereq.Selected {
	out := make([]releasereq.Selected, 0, len(in))
	for _, p := range in {
		out = append(out, releasereq.Selected{PluginKey: p.GetPluginKey(), Reason: p.GetReason()})
	}
	return out
}

// CreateReleaseRequest opens a request, or returns the one already open.
func (s ReleaseRequest) CreateReleaseRequest(ctx context.Context, req *connect.Request[pluxv1.CreateReleaseRequestRequest]) (*connect.Response[pluxv1.CreateReleaseRequestResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateReleaseRequestResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		r, existing, err := s.h.ReleaseRequests.Create(ctx, p, releasereq.CreateInput{
			AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Channel: m.GetChannel(), Plugins: selected(m.GetPlugins()),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateReleaseRequestResponse{Request: requestProto(r), Existing: existing}, nil
	})
}

// GetReleaseRequest returns a request and the newest diff of its content.
func (s ReleaseRequest) GetReleaseRequest(ctx context.Context, req *connect.Request[pluxv1.GetReleaseRequestRequest]) (*connect.Response[pluxv1.GetReleaseRequestResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetReleaseRequestResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.ReleaseRequests.Get(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.GetReleaseRequestResponse{Request: requestProto(r)}
		d, found, err := s.h.ReleaseRequests.Diff(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		if found {
			out.VisualDiff = diffProto(d)
		}
		return out, nil
	})
}

// ListReleaseRequests lists an app's requests, newest first.
func (s ReleaseRequest) ListReleaseRequests(ctx context.Context, req *connect.Request[pluxv1.ListReleaseRequestsRequest]) (*connect.Response[pluxv1.ListReleaseRequestsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListReleaseRequestsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		page := req.Msg.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		rs, err := s.h.ReleaseRequests.List(ctx, p, req.Msg.GetAppId(), req.Msg.GetOpenOnly(), after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListReleaseRequestsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range rs {
			last = storage.Cursor{Time: r.CreatedAt, ID: r.ID}
			out.Requests = append(out.Requests, requestProto(r))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(rs), size, last)
		return out, nil
	})
}

// stepOf reads the payload of an advance call.
func stepOf(m *pluxv1.AdvanceReleaseRequestRequest) (releasereq.Step, error) {
	var step releasereq.Step
	switch v := m.GetStep().(type) {
	case *pluxv1.AdvanceReleaseRequestRequest_Selection:
		step = releasereq.Step{Kind: releasereq.StateSelect, Selection: selected(v.Selection.GetPlugins())}
	case *pluxv1.AdvanceReleaseRequestRequest_Builds:
		step = releasereq.Step{Kind: releasereq.StateBuild}
		for _, b := range v.Builds.GetBuilds() {
			step.Builds = append(step.Builds, releasereq.BuildRef{PluginKey: b.GetPluginKey(), JobID: b.GetJobId()})
		}
	case *pluxv1.AdvanceReleaseRequestRequest_Reports:
		step = releasereq.Step{Kind: releasereq.StateReports, AcknowledgedWarnings: v.Reports.GetAcknowledgedWarnings()}
	case *pluxv1.AdvanceReleaseRequestRequest_Changelog:
		step = releasereq.Step{Kind: releasereq.StateChangelog, Notes: v.Changelog.GetNotes(), Label: v.Changelog.GetLabel()}
	case *pluxv1.AdvanceReleaseRequestRequest_Target:
		step = releasereq.Step{Kind: releasereq.StateTarget, EnvironmentID: v.Target.GetEnvironmentId(), Channel: v.Target.GetChannel()}
	case *pluxv1.AdvanceReleaseRequestRequest_Rollout:
		step = releasereq.Step{Kind: releasereq.StateRollout, Plan: v.Rollout.GetPlan()}
	case nil:
		// A step with no payload is the diffs step.
		step = releasereq.Step{Kind: releasereq.StateDiffs}
	}
	if want := m.GetExpectedState(); want != "" && releasereq.State(want) != step.Kind {
		return step, plxerr.New(plxerr.InvalidFormat, "expected_state %q does not match the payload for the %s step", want, step.Kind)
	}
	return step, nil
}

// AdvanceReleaseRequest completes the request's current step.
func (s ReleaseRequest) AdvanceReleaseRequest(ctx context.Context, req *connect.Request[pluxv1.AdvanceReleaseRequestRequest]) (*connect.Response[pluxv1.AdvanceReleaseRequestResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.AdvanceReleaseRequestResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		step, err := stepOf(req.Msg)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		r, err := s.h.ReleaseRequests.Advance(ctx, p, req.Msg.GetAppId(), req.Msg.GetId(), step)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.AdvanceReleaseRequestResponse{Request: requestProto(r)}, nil
	})
}

// RefreezeReleaseRequest returns a request to the build step.
func (s ReleaseRequest) RefreezeReleaseRequest(ctx context.Context, req *connect.Request[pluxv1.RefreezeReleaseRequestRequest]) (*connect.Response[pluxv1.RefreezeReleaseRequestResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RefreezeReleaseRequestResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.ReleaseRequests.Refreeze(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RefreezeReleaseRequestResponse{Request: requestProto(r)}, nil
	})
}

// RequestVisualDiff queues the render job of the request's content.
func (s ReleaseRequest) RequestVisualDiff(ctx context.Context, req *connect.Request[pluxv1.RequestVisualDiffRequest]) (*connect.Response[pluxv1.RequestVisualDiffResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RequestVisualDiffResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		d, err := s.h.ReleaseRequests.RequestDiff(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RequestVisualDiffResponse{Job: diffProto(d)}, nil
	})
}

// RequestReleaseApproval is the approval step.
func (s ReleaseRequest) RequestReleaseApproval(ctx context.Context, req *connect.Request[pluxv1.RequestReleaseApprovalRequest]) (*connect.Response[pluxv1.RequestReleaseApprovalResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RequestReleaseApprovalResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.ReleaseRequests.RequestApproval(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RequestReleaseApprovalResponse{Request: requestProto(r)}, nil
	})
}

// CompleteReleaseRequest is the release step.
func (s ReleaseRequest) CompleteReleaseRequest(ctx context.Context, req *connect.Request[pluxv1.CompleteReleaseRequestRequest]) (*connect.Response[pluxv1.CompleteReleaseRequestResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CompleteReleaseRequestResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.ReleaseRequests.Complete(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CompleteReleaseRequestResponse{Request: requestProto(r)}, nil
	})
}

// CancelReleaseRequest ends an open request.
func (s ReleaseRequest) CancelReleaseRequest(ctx context.Context, req *connect.Request[pluxv1.CancelReleaseRequestRequest]) (*connect.Response[pluxv1.CancelReleaseRequestResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CancelReleaseRequestResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.ReleaseRequests.Cancel(ctx, p, req.Msg.GetAppId(), req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CancelReleaseRequestResponse{Request: requestProto(r)}, nil
	})
}

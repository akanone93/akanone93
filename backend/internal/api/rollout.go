// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/rollout"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// RolloutAdmin serves RolloutService: staged rollouts, their targeting and
// health gates (REL-050 to REL-053).
type RolloutAdmin struct {
	pluxv1connect.UnimplementedRolloutServiceHandler
	h *Handlers
}

var _ pluxv1connect.RolloutServiceHandler = RolloutAdmin{}

// RolloutAdmin returns the RolloutService handler.
func (h *Handlers) RolloutAdmin() RolloutAdmin { return RolloutAdmin{h: h} }

// CreateRollout schedules a rollout.
func (s RolloutAdmin) CreateRollout(ctx context.Context, req *connect.Request[pluxv1.CreateRolloutRequest]) (*connect.Response[pluxv1.CreateRolloutResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateRolloutResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		in := rollout.CreateInput{
			AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Channel: m.GetChannel(), Candidate: m.GetCandidateSequence(),
			Stages: stagesOf(m.GetStages()), Rule: m.GetRule(), Segments: m.GetSegments(),
			AllowDevices: m.GetAllowDevices(), DenyDevices: m.GetDenyDevices(), Health: gateOf(m.GetHealth()),
		}
		if m.GetStartAt() != nil {
			in.StartAt = m.GetStartAt().AsTime()
		}
		if m.GetEndAt() != nil {
			in.EndAt = m.GetEndAt().AsTime()
		}
		r, err := s.h.Rollouts.Create(ctx, p, in)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateRolloutResponse{Rollout: rolloutProto(r)}, nil
	})
}

// GetRollout returns one rollout.
func (s RolloutAdmin) GetRollout(ctx context.Context, req *connect.Request[pluxv1.GetRolloutRequest]) (*connect.Response[pluxv1.GetRolloutResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetRolloutResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Rollouts.Get(ctx, p, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetRolloutResponse{Rollout: rolloutProto(r)}, nil
	})
}

// ListRollouts lists an app's rollouts, newest first.
func (s RolloutAdmin) ListRollouts(ctx context.Context, req *connect.Request[pluxv1.ListRolloutsRequest]) (*connect.Response[pluxv1.ListRolloutsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListRolloutsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		page := req.Msg.GetPage()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err
		}
		rollouts, err := s.h.Rollouts.List(ctx, p, req.Msg.GetAppId(), req.Msg.GetOnlyOpen(), after.ID, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListRolloutsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, r := range rollouts {
			last = storage.Cursor{ID: r.ID}
			out.Rollouts = append(out.Rollouts, rolloutProto(r))
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(rollouts), size, last)
		return out, nil
	})
}

// PauseRollout stops a rollout from advancing.
func (s RolloutAdmin) PauseRollout(ctx context.Context, req *connect.Request[pluxv1.PauseRolloutRequest]) (*connect.Response[pluxv1.PauseRolloutResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.PauseRolloutResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Rollouts.Pause(ctx, p, req.Msg.GetId(), req.Msg.GetReason())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.PauseRolloutResponse{Rollout: rolloutProto(r)}, nil
	})
}

// ResumeRollout continues a paused rollout.
func (s RolloutAdmin) ResumeRollout(ctx context.Context, req *connect.Request[pluxv1.ResumeRolloutRequest]) (*connect.Response[pluxv1.ResumeRolloutResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ResumeRolloutResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Rollouts.Resume(ctx, p, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.ResumeRolloutResponse{Rollout: rolloutProto(r)}, nil
	})
}

// AbortRollout ends a rollout and reverts its channel.
func (s RolloutAdmin) AbortRollout(ctx context.Context, req *connect.Request[pluxv1.AbortRolloutRequest]) (*connect.Response[pluxv1.AbortRolloutResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.AbortRolloutResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Rollouts.Abort(ctx, p, req.Msg.GetId(), req.Msg.GetReason())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.AbortRolloutResponse{Rollout: rolloutProto(r)}, nil
	})
}

// stagesOf converts the wire stages.
func stagesOf(in []*pluxv1.Stage) []rollout.Stage {
	out := make([]rollout.Stage, len(in))
	for i, s := range in {
		out[i] = rollout.Stage{BasisPoints: int(s.GetBasisPoints()), DurationSeconds: s.GetDurationSeconds()}
	}
	return out
}

// gateOf converts a wire health gate; an absent one is unarmed.
func gateOf(g *pluxv1.HealthGate) rollout.Gate {
	if g == nil {
		return rollout.Gate{}
	}
	out := rollout.Gate{
		MinSample: g.GetMinSample(), WindowSeconds: g.GetWindowSeconds(),
		MaxCrashFreeDrop:             int(g.GetMaxCrashFreeDropBasisPoints()),
		MaxErrorRateIncrease:         int(g.GetMaxErrorRateIncreaseBasisPoints()),
		MaxActionErrorRateIncrease:   int(g.GetMaxActionErrorRateIncreaseBasisPoints()),
		MaxFunctionErrorRateIncrease: int(g.GetMaxFunctionErrorRateIncreaseBasisPoints()),
	}
	for _, c := range g.GetCustom() {
		out.Custom = append(out.Custom, rollout.CustomGate{Event: c.GetEvent(), MaxIncreaseBasisPoints: int(c.GetMaxIncreaseBasisPoints())})
	}
	if g.GetAction() == pluxv1.HealthAction_HEALTH_ACTION_ROLLBACK {
		out.Action = rollout.ActionRollback
	}
	return out
}

// gateProto converts a health gate for the wire.
func gateProto(g rollout.Gate) *pluxv1.HealthGate {
	out := &pluxv1.HealthGate{
		MinSample: g.MinSample, WindowSeconds: g.WindowSeconds,
		MaxCrashFreeDropBasisPoints:             int32(g.MaxCrashFreeDrop),             //nolint:gosec // validated small
		MaxErrorRateIncreaseBasisPoints:         int32(g.MaxErrorRateIncrease),         //nolint:gosec // validated small
		MaxActionErrorRateIncreaseBasisPoints:   int32(g.MaxActionErrorRateIncrease),   //nolint:gosec // validated small
		MaxFunctionErrorRateIncreaseBasisPoints: int32(g.MaxFunctionErrorRateIncrease), //nolint:gosec // validated small
		Action:                                  pluxv1.HealthAction_HEALTH_ACTION_PAUSE,
	}
	if g.Action == rollout.ActionRollback {
		out.Action = pluxv1.HealthAction_HEALTH_ACTION_ROLLBACK
	}
	for _, c := range g.Custom {
		out.Custom = append(out.Custom, &pluxv1.CustomMetricGate{Event: c.Event, MaxIncreaseBasisPoints: int32(c.MaxIncreaseBasisPoints)}) //nolint:gosec // validated small
	}
	return out
}

// rolloutStates maps states to the wire.
var rolloutStates = map[rollout.State]pluxv1.RolloutState{
	rollout.StateScheduled: pluxv1.RolloutState_ROLLOUT_STATE_SCHEDULED, rollout.StateActive: pluxv1.RolloutState_ROLLOUT_STATE_ACTIVE,
	rollout.StatePaused: pluxv1.RolloutState_ROLLOUT_STATE_PAUSED, rollout.StateClosing: pluxv1.RolloutState_ROLLOUT_STATE_CLOSING,
	rollout.StateCompleted: pluxv1.RolloutState_ROLLOUT_STATE_COMPLETED, rollout.StateEnded: pluxv1.RolloutState_ROLLOUT_STATE_ENDED,
	rollout.StateAborted: pluxv1.RolloutState_ROLLOUT_STATE_ABORTED, rollout.StateRolledBack: pluxv1.RolloutState_ROLLOUT_STATE_ROLLED_BACK,
}

// rolloutProto converts a rollout for the wire.
func rolloutProto(r rollout.Rollout) *pluxv1.Rollout {
	out := &pluxv1.Rollout{
		Id: r.ID, AppId: r.AppID, EnvironmentId: r.EnvironmentID, EnvironmentKey: r.Environment, Channel: r.Channel,
		CandidateSequence: r.CandidateSequence, PreviousSequence: r.PreviousSequence, State: rolloutStates[r.State], StateReason: r.Reason,
		StageIndex: int32(r.StageIndex), BasisPoints: int32(r.BasisPoints), //nolint:gosec // an index and a share of at most 10000
		StartAt: ts(r.StartAt), EndAt: ts(r.EndAt), AllowDevices: r.AllowDevices, DenyDevices: r.DenyDevices,
		Health: gateProto(r.Health), LastCheck: r.LastCheck, LastCheckAt: ts(r.LastCheckAt), RevertedSequence: r.RevertedSequence,
		CreatedBy: r.CreatedBy, CreatedAt: ts(r.CreatedAt), UpdatedAt: ts(r.UpdatedAt),
	}
	for _, s := range r.Stages {
		out.Stages = append(out.Stages, &pluxv1.Stage{BasisPoints: int32(s.BasisPoints), DurationSeconds: s.DurationSeconds}) //nolint:gosec // validated to at most 10000
	}
	out.Rule, out.Segments = rolloutRules(r)
	return out
}

// rolloutRules renders a rollout's targeting as the JSON it was given in.
func rolloutRules(r rollout.Rollout) ([]byte, [][]byte) {
	rule, segments, err := r.Targeting.JSON()
	if err != nil {
		return nil, nil
	}
	return rule, segments
}

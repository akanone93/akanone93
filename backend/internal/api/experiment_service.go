// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// ExperimentAdmin serves ExperimentService: experiments, their holdout, their
// results and the promotion of a winner (ABT-001, ABT-003, ABT-004, ABT-005).
type ExperimentAdmin struct {
	pluxv1connect.UnimplementedExperimentServiceHandler
	h *Handlers
}

var _ pluxv1connect.ExperimentServiceHandler = ExperimentAdmin{}

// ExperimentAdmin returns the ExperimentService handler.
func (h *Handlers) ExperimentAdmin() ExperimentAdmin { return ExperimentAdmin{h: h} }

var experimentStates = map[pluxv1.ExperimentState]experiment.State{
	pluxv1.ExperimentState_EXPERIMENT_STATE_DRAFT:     experiment.StateDraft,
	pluxv1.ExperimentState_EXPERIMENT_STATE_RUNNING:   experiment.StateRunning,
	pluxv1.ExperimentState_EXPERIMENT_STATE_STOPPED:   experiment.StateStopped,
	pluxv1.ExperimentState_EXPERIMENT_STATE_CONCLUDED: experiment.StateConcluded,
}

func protoExperimentState(s experiment.State) pluxv1.ExperimentState {
	for wire, state := range experimentStates {
		if state == s {
			return wire
		}
	}
	return pluxv1.ExperimentState_EXPERIMENT_STATE_UNSPECIFIED
}

func experimentProto(e experiment.Experiment) *pluxv1.Experiment {
	return &pluxv1.Experiment{
		Id: e.ID, AppId: e.AppID, EnvironmentId: e.EnvironmentID, Key: e.Key, State: protoExperimentState(e.State),
		DefinitionJson: string(e.Document), Salt: e.Salt, LayerSharePercent: float64(e.LayerShareBP) / 100,
		LayerStart: e.LayerStart, LayerEnd: e.LayerEnd, StartedAt: ts(e.StartedAt), StoppedAt: ts(e.StoppedAt),
		Winner: e.Winner, PromoteJobId: e.PromoteJobID, CreatedBy: actorProto(e.CreatedBy),
		CreatedAt: ts(e.CreatedAt), UpdatedAt: ts(e.UpdatedAt),
	}
}

func experimentResult(e experiment.Experiment, err error) (*pluxv1.ExperimentResponse, error) {
	if err != nil {
		return nil, err
	}
	return &pluxv1.ExperimentResponse{Experiment: experimentProto(e)}, nil
}

// CreateExperiment registers a draft experiment.
func (s ExperimentAdmin) CreateExperiment(ctx context.Context, req *connect.Request[pluxv1.CreateExperimentRequest]) (*connect.Response[pluxv1.ExperimentResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ExperimentResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		return experimentResult(s.h.Experiments.CreateExperiment(ctx, p, experiment.CreateExperimentRequest{
			AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Document: m.GetDocument(),
		}))
	})
}

// GetExperiment returns one experiment.
func (s ExperimentAdmin) GetExperiment(ctx context.Context, req *connect.Request[pluxv1.GetExperimentRequest]) (*connect.Response[pluxv1.ExperimentResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ExperimentResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		return experimentResult(s.h.Experiments.GetExperiment(ctx, p, req.Msg.GetId()))
	})
}

// ListExperiments lists an app's experiments.
func (s ExperimentAdmin) ListExperiments(ctx context.Context, req *connect.Request[pluxv1.ListExperimentsRequest]) (*connect.Response[pluxv1.ListExperimentsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListExperimentsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		list, err := s.h.Experiments.ListExperiments(ctx, p, m.GetAppId(), m.GetEnvironmentId(), experimentStates[m.GetState()])
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListExperimentsResponse{}
		for _, e := range list {
			out.Experiments = append(out.Experiments, experimentProto(e))
		}
		return out, nil
	})
}

// DeleteExperiment discards a draft.
func (s ExperimentAdmin) DeleteExperiment(ctx context.Context, req *connect.Request[pluxv1.DeleteExperimentRequest]) (*connect.Response[pluxv1.DeleteExperimentResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteExperimentResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Experiments.DeleteExperiment(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteExperimentResponse{}, nil
	})
}

// StartExperiment writes an experiment's allocation into the control documents.
func (s ExperimentAdmin) StartExperiment(ctx context.Context, req *connect.Request[pluxv1.StartExperimentRequest]) (*connect.Response[pluxv1.ExperimentResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ExperimentResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		return experimentResult(s.h.Experiments.StartExperiment(ctx, p, req.Msg.GetId(), experiment.StartOptions{LayerShare: req.Msg.GetLayerSharePercent()}))
	})
}

// StopExperiment takes an experiment out of the control documents.
func (s ExperimentAdmin) StopExperiment(ctx context.Context, req *connect.Request[pluxv1.StopExperimentRequest]) (*connect.Response[pluxv1.ExperimentResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.ExperimentResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		return experimentResult(s.h.Experiments.StopExperiment(ctx, p, req.Msg.GetId()))
	})
}

func holdoutProto(h experiment.Holdout) *pluxv1.HoldoutResponse {
	return &pluxv1.HoldoutResponse{AppId: h.AppID, EnvironmentId: h.EnvironmentID, Percent: h.Percent}
}

// SetHoldout sets the share kept out of every experiment of an environment.
func (s ExperimentAdmin) SetHoldout(ctx context.Context, req *connect.Request[pluxv1.SetHoldoutRequest]) (*connect.Response[pluxv1.HoldoutResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.HoldoutResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		h, err := s.h.Experiments.SetHoldout(ctx, p, experiment.Holdout{AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Percent: m.GetPercent()})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return holdoutProto(h), nil
	})
}

// GetHoldout returns an environment's holdout.
func (s ExperimentAdmin) GetHoldout(ctx context.Context, req *connect.Request[pluxv1.GetHoldoutRequest]) (*connect.Response[pluxv1.HoldoutResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.HoldoutResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		h, err := s.h.Experiments.GetHoldout(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return holdoutProto(h), nil
	})
}

// GetResults analyses an experiment.
func (s ExperimentAdmin) GetResults(ctx context.Context, req *connect.Request[pluxv1.GetResultsRequest]) (*connect.Response[pluxv1.GetResultsResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetResultsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		r, err := s.h.Experiments.Analyse(ctx, p, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return resultsProto(r), nil
	})
}

// PromoteExperiment makes a variant the app and starts the publish.
func (s ExperimentAdmin) PromoteExperiment(ctx context.Context, req *connect.Request[pluxv1.PromoteExperimentRequest]) (*connect.Response[pluxv1.PromoteExperimentResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.PromoteExperimentResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		promo, err := s.h.Experiments.Promote(ctx, p, experiment.PromoteRequest{
			ExperimentID: m.GetId(), Variant: m.GetVariant(), EnvironmentID: m.GetEnvironmentId(), Label: m.GetLabel(), Notes: m.GetNotes(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.PromoteExperimentResponse{Experiment: experimentProto(promo.Experiment), PublishJobId: promo.Job.ID}, nil
	})
}

func resultsProto(r experiment.Result) *pluxv1.GetResultsResponse {
	out := &pluxv1.GetResultsResponse{
		ExperimentId: r.ExperimentID, Key: r.Key, State: protoExperimentState(r.State), At: ts(r.At),
		SampleRatioPValue: r.SampleRatio.PValue, SampleRatioMismatch: r.SampleRatio.Mismatch, SampleRatioCode: int32(r.SampleRatioCode),
		Primary: metricProto(r.Primary),
	}
	for _, v := range r.Variants {
		out.Variants = append(out.Variants, &pluxv1.VariantCount{Name: v.Name, Control: v.Control, Subjects: v.Subjects, Share: v.Share})
	}
	for _, g := range r.Guardrails {
		out.Guardrails = append(out.Guardrails, metricProto(g))
	}
	return out
}

func metricProto(m experiment.MetricResult) *pluxv1.MetricResult {
	out := &pluxv1.MetricResult{Event: m.Metric.Event, Property: m.Metric.Property, Kind: string(m.Metric.Kind), Direction: string(m.Metric.Direction)}
	for _, v := range m.Means {
		out.Means = append(out.Means, &pluxv1.VariantMean{Variant: v.Variant, Subjects: v.Subjects, Mean: v.Mean})
	}
	for _, c := range m.Comparisons {
		e := c.Estimate
		out.Comparisons = append(out.Comparisons, &pluxv1.Comparison{
			Variant: c.Variant, Diff: e.Diff, Lower: e.Lower, Upper: e.Upper, PValue: e.PValue, RelativeDiff: c.RelativeDiff,
			Improved: c.Improved, Worsened: c.Worsened, GuardrailTripped: c.GuardrailTripped, Cuped: e.CUPED, VarianceReduction: e.VarianceReduction,
		})
	}
	return out
}

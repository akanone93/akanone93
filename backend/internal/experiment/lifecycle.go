// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// StartOptions tune a start.
type StartOptions struct {
	// LayerShare is the share of the experiment's layer it takes, in percent
	// above 0 and up to 100; zero means the whole layer. Ignored for an
	// experiment outside a layer.
	LayerShare float64
}

// StartExperiment allocates a draft experiment: it takes its range of the
// layer (PLX-8200 when none is free), asks the approval gate, and writes the
// allocation into the control documents of the environment's channels
// (ABT-001, ABT-004, ABT-007).
func (s *Service) StartExperiment(ctx context.Context, p auth.Principal, id string, o StartOptions) (Experiment, error) {
	draft, err := s.GetExperiment(ctx, p, id)
	if err != nil {
		return Experiment{}, err
	}
	if err := authorizeExperiments(p, draft.AppID); err != nil {
		return Experiment{}, err
	}
	rule, err := s.segmentRule(ctx, p, draft)
	if err != nil {
		return Experiment{}, err
	}
	var out Experiment
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, env, err := lockExperiment(ctx, q, p, id)
		if err != nil {
			return err
		}
		if row.State != string(StateDraft) {
			return plxerr.New(plxerr.PreconditionFailed, "experiment %q is %s; only a draft starts", row.Key, row.State)
		}
		if err := s.checkActiveCount(ctx, tx, p, row); err != nil {
			return err
		}
		started, err := s.start(ctx, q, row, rule, o)
		if err != nil {
			return err
		}
		if out, err = experimentOf(started); err != nil {
			return err
		}
		if err := s.approval.RequireLaunch(ctx, Launch{Principal: p, Environment: env.Key, Experiment: out}); err != nil {
			return err //nolint:wrapcheck // the gate's refusal is a domain error
		}
		return s.afterExperimentChange(ctx, tx, p, env, audit.ExperimentStarted, out, "experiment "+out.Key+" started")
	})
	return out, err
}

// segmentRule resolves the segment an experiment targets, before any
// transaction is open; nil when it targets everyone.
func (s *Service) segmentRule(ctx context.Context, p auth.Principal, e Experiment) (*controldoc.Rule, error) {
	if e.Definition.Segment == "" {
		return nil, nil
	}
	rule, err := s.segments.Rule(ctx, p, e.AppID, e.Definition.Segment)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return &rule, nil
}

// lockExperiment takes the lock of an experiment's environment, which makes
// the starts and stops of its experiments take turns, then the experiment's
// row.
func lockExperiment(ctx context.Context, q *dbgen.Queries, p auth.Principal, id string) (dbgen.Experiment, dbgen.Environment, error) {
	uid, err := parseID(id, "experiment")
	if err != nil {
		return dbgen.Experiment{}, dbgen.Environment{}, err
	}
	peek, err := q.GetExperiment(ctx, uid)
	if err != nil {
		return dbgen.Experiment{}, dbgen.Environment{}, failure(err, "experiment")
	}
	if err := authorizeExperiments(p, storage.ID(peek.AppID)); err != nil {
		return dbgen.Experiment{}, dbgen.Environment{}, err
	}
	if _, err := q.LockEnvironmentForExperiments(ctx, peek.EnvironmentID); err != nil {
		return dbgen.Experiment{}, dbgen.Environment{}, failure(err, "environment")
	}
	row, err := q.GetExperimentForUpdate(ctx, uid)
	if err != nil {
		return dbgen.Experiment{}, dbgen.Environment{}, failure(err, "experiment")
	}
	env, err := q.GetEnvironment(ctx, row.EnvironmentID)
	if err != nil {
		return dbgen.Experiment{}, dbgen.Environment{}, failure(err, "environment")
	}
	return row, env, nil
}

// checkActiveCount refuses a start that would take an app past
// experiments.activePerApp.
func (s *Service) checkActiveCount(ctx context.Context, tx pgx.Tx, p auth.Principal, row dbgen.Experiment) error {
	lim, err := s.limitsOf(ctx, tx, p, storage.ID(row.AppID))
	if err != nil {
		return err
	}
	running, err := dbgen.New(tx).CountRunningExperiments(ctx, row.AppID)
	if err != nil {
		return failure(err, "experiment")
	}
	if max := lim.Get(limits.ExperimentsActivePerApp); running >= max {
		return plxerr.New(plxerr.LimitExceeded, "an app may run at most %d experiments at once (experiments.activePerApp); stop one first", max)
	}
	return nil
}

// start takes the experiment's range of its layer and marks it running.
func (s *Service) start(ctx context.Context, q *dbgen.Queries, row dbgen.Experiment, rule *controldoc.Rule, o StartOptions) (dbgen.Experiment, error) {
	params := dbgen.StartExperimentParams{ID: row.ID, StartedAt: storage.Timestamp(s.now().UTC())}
	if row.Layer != "" {
		size, err := layerSize(o)
		if err != nil {
			return dbgen.Experiment{}, err
		}
		held, err := q.ListRunningExperimentsInLayer(ctx, dbgen.ListRunningExperimentsInLayerParams{EnvironmentID: row.EnvironmentID, Layer: row.Layer})
		if err != nil {
			return dbgen.Experiment{}, failure(err, "experiment")
		}
		r, err := allocate(row.Layer, spansOf(held), size)
		if err != nil {
			return dbgen.Experiment{}, err
		}
		from, to := int32(r.start), int32(r.end)                                          //nolint:gosec // within 10000
		params.LayerShareBp, params.LayerStart, params.LayerEnd = int32(size), &from, &to //nolint:gosec // within 10000
	}
	if rule != nil {
		stored, err := json.Marshal(rule)
		if err != nil {
			return dbgen.Experiment{}, fmt.Errorf("experiment: %w", err)
		}
		params.SegmentRule = stored
	}
	started, err := q.StartExperiment(ctx, params)
	if err != nil {
		return dbgen.Experiment{}, failure(err, "experiment")
	}
	return started, nil
}

// layerSize is the number of buckets a start asks for.
func layerSize(o StartOptions) (uint32, error) {
	if o.LayerShare == 0 {
		return controldoc.BucketCount, nil
	}
	return shareBP(o.LayerShare)
}

// spansOf lists the ranges the running experiments of a layer hold.
func spansOf(rows []dbgen.Experiment) []span {
	var out []span
	for _, r := range rows {
		if r.LayerStart != nil && r.LayerEnd != nil {
			out = append(out, span{start: uint32(*r.LayerStart), end: uint32(*r.LayerEnd), key: r.Key}) //nolint:gosec // checked by the table
		}
	}
	return out
}

// StopExperiment takes a running experiment out of the control documents
// and frees its range of the layer. Its results stay readable.
func (s *Service) StopExperiment(ctx context.Context, p auth.Principal, id string) (Experiment, error) {
	var out Experiment
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, env, err := lockExperiment(ctx, q, p, id)
		if err != nil {
			return err
		}
		stopped, err := q.StopExperiment(ctx, dbgen.StopExperimentParams{ID: row.ID, StoppedAt: storage.Timestamp(s.now().UTC())})
		if err != nil {
			if row.State != string(StateRunning) {
				return plxerr.New(plxerr.PreconditionFailed, "experiment %q is %s; only a running experiment stops", row.Key, row.State)
			}
			return failure(err, "experiment")
		}
		if out, err = experimentOf(stopped); err != nil {
			return err
		}
		return s.afterExperimentChange(ctx, tx, p, env, audit.ExperimentStopped, out, "experiment "+out.Key+" stopped")
	})
	return out, err
}

// afterExperimentChange publishes the environment's state to its control
// documents and audits the change, in the change's transaction.
func (s *Service) afterExperimentChange(ctx context.Context, tx pgx.Tx, p auth.Principal, env dbgen.Environment, action audit.Action, e Experiment, detail string) error {
	if err := s.syncControl(ctx, tx, p, env, detail); err != nil {
		return err
	}
	return s.record(ctx, tx, p, audit.Entry{Action: action, TargetKind: "experiment", TargetID: e.ID, Detail: detail})
}

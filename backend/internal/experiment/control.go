// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// syncControl rebuilds the environment's flags, running experiments and
// holdout from the stored state and writes them into the control documents of
// its channels, in the caller's transaction. The caller holds the
// environment's lock when it changes experiments, so two changes never
// overwrite each other's view.
func (s *Service) syncControl(ctx context.Context, tx pgx.Tx, p auth.Principal, env dbgen.Environment, detail string) error {
	ex, err := buildExperimentation(ctx, dbgen.New(tx), env)
	if err != nil {
		return err
	}
	if err := s.o.Control.ApplyExperimentation(ctx, tx, p, storage.ID(env.ID), ex, detail); err != nil {
		return fmt.Errorf("experiment: %w", err)
	}
	return nil
}

// buildExperimentation reads what an environment's control documents say
// about flags and experiments.
func buildExperimentation(ctx context.Context, q *dbgen.Queries, env dbgen.Environment) (release.Experimentation, error) {
	var ex release.Experimentation
	rows, err := q.ListFeatureFlags(ctx, env.ID)
	if err != nil {
		return ex, failure(err, "flag")
	}
	depth := int(limits.Defaults().Get(limits.DocumentJSONDepth))
	for _, row := range rows {
		f, err := flagValueOf(row, depth)
		if err != nil {
			return ex, err
		}
		ex.Flags = append(ex.Flags, f)
	}
	running, err := q.ListRunningExperiments(ctx, env.ID)
	if err != nil {
		return ex, failure(err, "experiment")
	}
	for _, row := range running {
		a, err := allocationOf(row)
		if err != nil {
			return ex, err
		}
		ex.Experiments = append(ex.Experiments, a)
	}
	h, err := q.GetExperimentHoldout(ctx, env.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return ex, failure(err, "holdout")
	case h.BasisPoints > 0:
		ex.Holdout = &controldoc.Holdout{BasisPoints: uint32(h.BasisPoints), Salt: h.Salt} //nolint:gosec // checked by the table
	}
	return ex, nil
}

// flagValueOf converts a stored flag to its entry in the control document;
// the default is written in canonical form, as jsonb does not keep it.
func flagValueOf(row dbgen.FeatureFlag, depth int) (controldoc.FlagValue, error) {
	def, err := jcs.Canonicalize(row.DefaultValue, depth)
	if err != nil {
		return controldoc.FlagValue{}, fmt.Errorf("experiment: the stored default of flag %q: %w", row.Key, err)
	}
	f := controldoc.FlagValue{Key: row.Key, Type: controldoc.FlagType(row.Type), Default: string(def)}
	if err := json.Unmarshal(row.Segments, &f.Segments); err != nil {
		return controldoc.FlagValue{}, fmt.Errorf("experiment: the stored segments of flag %q: %w", row.Key, err)
	}
	return f, nil
}

// allocationOf converts a running experiment to its allocation.
func allocationOf(row dbgen.Experiment) (controldoc.Allocation, error) {
	def, err := parseDefinition(row.Definition)
	if err != nil {
		return controldoc.Allocation{}, err
	}
	variants, err := variantsOf(def)
	if err != nil {
		return controldoc.Allocation{}, err
	}
	start, end, err := window(def)
	if err != nil {
		return controldoc.Allocation{}, err
	}
	a := controldoc.Allocation{
		Key: row.Key, Scope: scopeOf(def), Salt: row.Salt, Variants: variants, Layer: row.Layer,
		HoldoutBP: holdoutBP(def), StartsAt: start, EndsAt: end,
	}
	if row.LayerStart != nil && row.LayerEnd != nil {
		a.LayerStart, a.LayerEnd = uint32(*row.LayerStart), uint32(*row.LayerEnd) //nolint:gosec // checked by the table
	}
	if len(row.SegmentRule) > 0 {
		a.Segment = new(controldoc.Rule)
		if err := json.Unmarshal(row.SegmentRule, a.Segment); err != nil {
			return controldoc.Allocation{}, fmt.Errorf("experiment: the stored segment of experiment %q: %w", row.Key, err)
		}
	}
	return a, nil
}

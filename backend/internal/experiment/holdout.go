// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Holdout is the share of an environment's subjects kept out of every
// experiment (ABT-004).
type Holdout struct {
	AppID, EnvironmentID string
	// Percent is the share, 0 to 100, with at most two decimals.
	Percent float64
}

// holdoutBasisPoints converts a percentage to basis points.
func holdoutBasisPoints(percent float64) (int32, error) {
	bp := math.Round(percent * 100)
	if percent < 0 || percent > 100 || math.Abs(percent*100-bp) > weightEpsilon {
		return 0, plxerr.New(plxerr.OutOfRange, "a holdout is 0 to 100 percent with at most two decimals, not %v", percent)
	}
	return int32(bp), nil //nolint:gosec // bounded by the check above
}

// SetHoldout sets the share of the environment's subjects that every
// experiment leaves out and shows the control; zero removes it. The salt that
// picks the subjects is drawn once, so a subject in the holdout stays in it
// when the share grows (ABT-004).
func (s *Service) SetHoldout(ctx context.Context, p auth.Principal, h Holdout) (Holdout, error) {
	if err := authorizeExperiments(p, h.AppID); err != nil {
		return Holdout{}, err
	}
	bp, err := holdoutBasisPoints(h.Percent)
	if err != nil {
		return Holdout{}, err
	}
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environment(ctx, q, h.AppID, h.EnvironmentID)
		if err != nil {
			return err
		}
		if _, err := q.LockEnvironmentForExperiments(ctx, env.ID); err != nil {
			return failure(err, "environment")
		}
		salt, err := s.holdoutSalt(ctx, q, env)
		if err != nil {
			return err
		}
		by := p.Actor()
		if _, err := q.UpsertExperimentHoldout(ctx, dbgen.UpsertExperimentHoldoutParams{
			EnvironmentID: env.ID, OrganizationID: env.OrganizationID, BasisPoints: bp, Salt: salt,
			UpdatedByKind: by.Kind, UpdatedByID: by.ID, UpdatedBy: by.Display, UpdatedAt: storage.Timestamp(s.now().UTC()),
		}); err != nil {
			return failure(err, "holdout")
		}
		detail := fmt.Sprintf("experiment holdout %.2f%%", h.Percent)
		if err := s.syncControl(ctx, tx, p, env, detail); err != nil {
			return err
		}
		return s.record(ctx, tx, p, audit.Entry{Action: audit.ExperimentHoldoutChanged, TargetKind: "environment", TargetID: storage.ID(env.ID), Detail: detail})
	})
	return h, err
}

// holdoutSalt returns the environment's holdout salt, drawing one the first
// time.
func (s *Service) holdoutSalt(ctx context.Context, q *dbgen.Queries, env dbgen.Environment) (string, error) {
	row, err := q.GetExperimentHoldout(ctx, env.ID)
	switch {
	case err == nil:
		return row.Salt, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return "", failure(err, "holdout")
	}
	salt, err := s.o.IDs.New()
	if err != nil {
		return "", fmt.Errorf("experiment: %w", err)
	}
	return salt, nil
}

// GetHoldout returns the environment's holdout; zero percent when none is set.
func (s *Service) GetHoldout(ctx context.Context, p auth.Principal, appID, environmentID string) (Holdout, error) {
	if err := authorizeRead(p, appID); err != nil {
		return Holdout{}, err
	}
	out := Holdout{AppID: appID, EnvironmentID: environmentID}
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environment(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		row, err := q.GetExperimentHoldout(ctx, env.ID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil
		case err != nil:
			return failure(err, "holdout")
		}
		out.Percent = float64(row.BasisPoints) / 100
		return nil
	})
	return out, err
}

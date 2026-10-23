// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ApplyExperimentation sets the flags, experiments and global holdout that
// every channel of an environment carries in its control document (ABT-006,
// ABT-001, ABT-004): for each channel it takes the channel's lock, writes the
// next version with the kill switches, mandatory update and forced rollback
// left as they are, queues the signature and audits the change. A channel
// whose document already says the same is not touched.
//
// It runs in the caller's transaction, so the change of flags or experiments
// and the new documents commit together, and it does not authorise: the
// caller holds the permission that governs what it changes (flags.manage or
// experiments.manage) and has checked it for the environment. detail is the
// text of the audit entries.
func (s *Service) ApplyExperimentation(ctx context.Context, tx pgx.Tx, p auth.Principal, environmentID string, ex Experimentation, detail string) error {
	q := dbgen.New(tx)
	id, err := parseID(environmentID, "environment")
	if err != nil {
		return err
	}
	env, err := q.GetEnvironment(ctx, id)
	if err != nil {
		return failure(err, "environment")
	}
	channels, err := q.ListChannelsOfEnvironment(ctx, env.ID)
	if err != nil {
		return failure(err, "channel")
	}
	change := controlChange{
		detail: detail,
		apply: func(_ context.Context, _ *dbgen.Queries, _ controlTarget, c *controlContent) error {
			c.Experimentation = ex
			return nil
		},
	}
	for _, listed := range channels {
		ch, err := q.GetChannelForUpdate(ctx, dbgen.GetChannelForUpdateParams{EnvironmentID: env.ID, Key: listed.Key})
		if err != nil {
			return failure(err, "channel")
		}
		if err := s.writeControl(ctx, tx, p, controlTarget{app: env.AppID, env: env, ch: ch}, change); err != nil {
			return err
		}
	}
	return nil
}

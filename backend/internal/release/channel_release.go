// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ChannelRelease is a channel's current release as a host project embeds
// it (SYN-007, GEN-001): its sequence, 0 before the channel's first; its
// bundles with the signatures publish made; whether it is delivered sealed
// (SEC-053); and the environment's newest root, nil for none.
type ChannelRelease struct {
	Sequence     int64
	Confidential bool
	Versions     []Version
	Root         []byte
}

// ChannelRelease reads a channel's current release for the project
// download, in the organisation's tenancy; the caller has checked who may
// download (GEN-001). A channel that does not exist has no release.
func (s *Service) ChannelRelease(ctx context.Context, org, envID, channel string) (ChannelRelease, error) {
	var out ChannelRelease
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := q.GetEnvironment(ctx, storage.MustUUID(envID))
		if err != nil {
			return failure(err, "environment")
		}
		roots, err := q.ListRootsSince(ctx, dbgen.ListRootsSinceParams{EnvironmentID: env.ID, Version: 0})
		if err != nil {
			return fmt.Errorf("release: roots: %w", err)
		}
		if len(roots) > 0 {
			out.Root = roots[len(roots)-1].Document
		}
		ch, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: channel})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return failure(err, "channel")
		}
		if out.Sequence = ch.ReleaseSequence; out.Sequence == 0 {
			return nil
		}
		rel, err := q.GetRelease(ctx, dbgen.GetReleaseParams{AppID: env.AppID, Sequence: out.Sequence})
		if err != nil {
			return failure(err, "release")
		}
		out.Confidential = rel.Confidential
		versions, err := q.ListReleaseVersions(ctx, rel.ID)
		if err != nil {
			return failure(err, "version")
		}
		for _, v := range versions {
			out.Versions = append(out.Versions, versionOf(v))
		}
		return nil
	})
	if err != nil {
		return ChannelRelease{}, fmt.Errorf("release: %w", err)
	}
	return out, nil
}

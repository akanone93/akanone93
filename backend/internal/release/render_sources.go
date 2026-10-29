// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ProjectFiles returns the two projects of a visual diff (O-9, ADR-0097) as
// file sets in the Git layout: after is what a release of the built versions
// would hold, chosen exactly as CreateRelease chooses (the versions asked for,
// else the environment's last release's, else the newest), and before is what
// the channel serves now. before is nil when the channel serves no release
// yet, so that every screen is new.
//
// It is for the render job of a release request, which runs in the tenant of
// the organisation and not as a user; the caller has judged access already.
func (s *Service) ProjectFiles(ctx context.Context, tx pgx.Tx, appID, environmentID, channel string, built map[string]int64) (before, after fs.FS, err error) {
	q := dbgen.New(tx)
	app, env, err := s.environment(ctx, q, appID, environmentID)
	if err != nil {
		return nil, nil, err
	}
	chosen, missing, err := s.chooseVersions(ctx, q, app, env, built)
	if err != nil {
		return nil, nil, err
	}
	if len(missing) > 0 {
		return nil, nil, plxerr.New(plxerr.PluginNotPublished, "the project to render is incomplete: %s", missing[0].Message)
	}
	afterFiles, err := s.o.Documents.SourceFiles(ctx, tx, appID, sourcesOf(chosen))
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // a domain error
	}
	ch, err := q.GetChannel(ctx, dbgen.GetChannelParams{EnvironmentID: env.ID, Key: channel})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, afterFiles, nil
	case err != nil:
		return nil, nil, failure(err, "channel")
	case ch.ReleaseSequence <= 0:
		return nil, afterFiles, nil
	}
	_, served, err := s.releaseRow(ctx, q, appID, ch.ReleaseSequence)
	if err != nil {
		return nil, nil, err
	}
	beforeFiles, err := s.o.Documents.SourceFiles(ctx, tx, appID, sourcesOf(served))
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // a domain error
	}
	return beforeFiles, afterFiles, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package releasereq

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
)

// ReleaseBuilds reads publish jobs from the release service.
type ReleaseBuilds struct{ Releases *release.Service }

var _ Builds = ReleaseBuilds{}

// Build implements Builds. The job must belong to the app; a job of another
// organisation is not found (row-level security).
func (b ReleaseBuilds) Build(ctx context.Context, p auth.Principal, appID, jobID string) (Build, error) {
	job, err := b.Releases.GetPublishJob(ctx, p, jobID)
	if err != nil {
		return Build{}, err //nolint:wrapcheck // a domain error
	}
	if job.AppID != appID {
		return Build{}, plxerr.New(plxerr.ResourceNotFound, "no such publish job")
	}
	out := Build{State: job.State}
	for _, d := range job.Diagnostics {
		switch d.Severity {
		case plxerr.SeverityError:
			out.Errors++
		case plxerr.SeverityWarning:
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s@%s", d.Code, d.File))
		}
	}
	if job.State == release.StateSucceeded {
		v, err := b.Releases.GetVersion(ctx, p, job.PluginID, job.Version)
		if err != nil {
			return Build{}, err //nolint:wrapcheck // a domain error
		}
		out.PluginKey, out.Version, out.BundleSHA256 = v.PluginKey, v.Version, v.BundleSHA256
	}
	return out, nil
}

// ReleaseMaker makes and promotes app releases with the release service.
type ReleaseMaker struct{ Releases *release.Service }

var _ Releaser = ReleaseMaker{}

// Create implements Releaser.
func (m ReleaseMaker) Create(ctx context.Context, p auth.Principal, appID, environmentID string, versions map[string]int64, notes string) (int64, error) {
	r, diags, err := m.Releases.CreateRelease(ctx, p, appID, environmentID, versions, notes)
	if err != nil {
		return 0, err //nolint:wrapcheck // a domain error
	}
	if r.ID == "" {
		return 0, plxerr.New(plxerr.ReleaseInconsistent, "the release cannot be made: %d diagnostic(s)", len(diags))
	}
	return r.Sequence, nil
}

// Promote implements Releaser.
func (m ReleaseMaker) Promote(ctx context.Context, p auth.Principal, appID string, sequence int64, environmentID, channel string) error {
	_, err := m.Releases.PromoteRelease(ctx, p, appID, sequence, environmentID, channel)
	return err //nolint:wrapcheck // a domain error
}

// ReleaseSources reads the projects of a visual diff from the release
// service (O-9).
type ReleaseSources struct{ Releases *release.Service }

var _ Sources = ReleaseSources{}

// Projects implements Sources.
func (r ReleaseSources) Projects(ctx context.Context, tx pgx.Tx, appID, environmentID, channel string, built map[string]int64) (before, after fs.FS, err error) {
	return r.Releases.ProjectFiles(ctx, tx, appID, environmentID, channel, built) //nolint:wrapcheck // a domain error
}

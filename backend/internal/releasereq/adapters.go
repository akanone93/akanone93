// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package releasereq

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

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
		var v release.Version
		if job.PluginID == "" {
			// The app bundle's job: its versions belong to the app.
			v, err = b.Releases.GetAppVersion(ctx, p, appID, job.Version)
		} else {
			v, err = b.Releases.GetVersion(ctx, p, job.PluginID, job.Version)
		}
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
		// Say why, with the findings themselves: the publisher acts on them.
		return 0, plxerr.New(plxerr.ReleaseInconsistent, "the release cannot be made: %s", summarise(diags))
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

// summarise names the first errors of a refused release, at most three, and
// how many more there are.
func summarise(diags plxerr.Diagnostics) string {
	var parts []string
	errs := 0
	for _, d := range diags {
		if d.Severity != plxerr.SeverityError {
			continue
		}
		errs++
		if len(parts) < 3 {
			parts = append(parts, fmt.Sprintf("%s %s", d.Code, d.Message))
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d diagnostic(s)", len(diags))
	}
	if n := errs - len(parts); n > 0 {
		parts = append(parts, fmt.Sprintf("and %d more", n))
	}
	return strings.Join(parts, "; ")
}

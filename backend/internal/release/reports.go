// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ReportKind names a report a release carries.
type ReportKind string

// The kinds of report.
const (
	// ReportAccessibility is the accessibility report (A11Y-008).
	ReportAccessibility ReportKind = "accessibility"
	// ReportLocalisation is the localisation report (I18N-008).
	ReportLocalisation ReportKind = "localisation"
)

// ReportFormat says how a report is returned.
type ReportFormat string

// The formats of a report.
const (
	ReportJSON ReportFormat = "json"
	ReportHTML ReportFormat = "html"
)

// Content types of the report formats.
const (
	contentTypeJSON = "application/json"
	contentTypeHTML = "text/html; charset=utf-8"
)

// reportDepth bounds the nesting of a report's JSON.
const reportDepth = 32

// reportProducers is the registration point of the reports a release
// carries: for each kind, how to take its canonical JSON from the
// compilation of the release's sources. A producer returns nil when the
// compilation has no such report. A new report kind adds its entry here
// and its renderer to reportRenderers.
func reportProducers() map[ReportKind]func(*compiler.Result) (any, error) {
	return map[ReportKind]func(*compiler.Result) (any, error){
		ReportAccessibility: func(res *compiler.Result) (any, error) {
			if res.Accessibility == nil {
				return nil, nil
			}
			return res.Accessibility, nil
		},
		ReportLocalisation: func(res *compiler.Result) (any, error) {
			if res.Localisation == nil {
				return nil, nil
			}
			return res.Localisation, nil
		},
	}
}

// reportRenderers is the registration point of the HTML form of a report:
// for each kind, a function from the stored canonical JSON to a standalone
// page. A kind without an entry has no HTML form, and one without a
// producer is not stored, so asking for it is a "not found".
func reportRenderers() map[ReportKind]func(report []byte) ([]byte, error) {
	return map[ReportKind]func([]byte) ([]byte, error){
		ReportAccessibility: renderA11yHTML,
		ReportLocalisation:  renderL10nHTML,
	}
}

// canonicalReports produces the canonical JSON of every report a
// compilation yields, by kind.
func canonicalReports(res *compiler.Result) (map[ReportKind][]byte, error) {
	out := map[ReportKind][]byte{}
	for kind, produce := range reportProducers() {
		v, err := produce(res)
		if err != nil {
			return nil, fmt.Errorf("release: %s report: %w", kind, err)
		}
		if v == nil {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("release: %s report: %w", kind, err)
		}
		if out[kind], err = jcs.Canonicalize(raw, reportDepth); err != nil {
			return nil, fmt.Errorf("release: %s report: %w", kind, err)
		}
	}
	return out, nil
}

// storeReports records the reports of a new release.
func storeReports(ctx context.Context, q *dbgen.Queries, rel dbgen.Release, reports map[ReportKind][]byte) error {
	kinds := make([]ReportKind, 0, len(reports))
	for k := range reports {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, k := range kinds {
		if err := q.InsertReleaseReport(ctx, dbgen.InsertReleaseReportParams{
			ReleaseID: rel.ID, Kind: string(k), OrganizationID: rel.OrganizationID, Content: reports[k],
		}); err != nil {
			return failure(err, "release report")
		}
	}
	return nil
}

// GetReport returns a report of a release as canonical JSON, or as a
// standalone HTML page rendered from it, with its content type (A11Y-008).
// A release created before the report existed, and a kind no report is
// stored for, are not found.
func (s *Service) GetReport(ctx context.Context, p auth.Principal, appID string, sequence int64, kind ReportKind, format ReportFormat) ([]byte, string, error) {
	if err := authorize(p, auth.PluginRead, appID); err != nil {
		return nil, "", err
	}
	if _, ok := reportProducers()[kind]; !ok {
		return nil, "", plxerr.New(plxerr.ResourceNotFound, "the release carries no %q report", string(kind))
	}
	render := reportRenderers()[kind]
	if format == "" {
		format = ReportJSON
	}
	if format != ReportJSON && (format != ReportHTML || render == nil) {
		return nil, "", plxerr.New(plxerr.InvalidFormat, "the %q report has no %q form", string(kind), string(format))
	}
	var content []byte
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, _, err := s.releaseRow(ctx, q, appID, sequence)
		if err != nil {
			return err
		}
		content, err = q.GetReleaseReport(ctx, dbgen.GetReleaseReportParams{ReleaseID: row.ID, Kind: string(kind)})
		if err != nil {
			return failure(err, string(kind)+" report")
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if format == ReportJSON {
		return content, contentTypeJSON, nil
	}
	page, err := render(content)
	if err != nil {
		return nil, "", fmt.Errorf("release: %s report: %w", kind, err)
	}
	return page, contentTypeHTML, nil
}

// copyReports gives a release the reports of the earlier release whose
// content it reproduces (REL-006).
func copyReports(ctx context.Context, q *dbgen.Queries, to, from pgtype.UUID) error {
	if err := q.CopyReleaseReports(ctx, dbgen.CopyReleaseReportsParams{ReleaseID: to, EarlierID: from}); err != nil {
		return failure(err, "release report")
	}
	return nil
}

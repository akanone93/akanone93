// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package testrun holds the runs of Plux Test scenarios that `plux test` and
// CI report to the server (TST-001, TST-002), so Studio lists them per app
// and shows the result of every scenario and step. A run is one row under
// row-level security; its scenarios are kept as JSON beside the counts.
package testrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// IDs generates identifiers for the rows this package writes.
type IDs interface{ New() (string, error) }

// Options are what the service needs.
type Options struct {
	DB  *storage.DB
	IDs IDs
	// Limits are the installation's limits (LIM-001); zero means the
	// defaults.
	Limits limits.Set
}

// Service stores reported runs.
type Service struct{ o Options }

// New returns a service.
func New(o Options) *Service {
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	return &Service{o: o}
}

// The outcomes of a step or scenario.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Step is the outcome of one step or expectation.
type Step struct {
	Index       int32  `json:"index"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
}

// Scenario is the outcome of one scenario.
type Scenario struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"durationMs"`
	Steps      []Step `json:"steps"`
}

// Report is a run to record.
type Report struct {
	AppID     string
	Key       string
	Source    string
	Label     string
	Scenarios []Scenario
}

// Run is a recorded run. Scenarios is empty in a list.
type Run struct {
	ID         string
	AppID      string
	Key        string
	Source     string
	Label      string
	Passed     int32
	Failed     int32
	Skipped    int32
	DurationMs int64
	CreatedBy  string
	CreatedAt  time.Time
	Scenarios  []Scenario
}

// maxText bounds every free-text field of a report.
const maxText = 4096

func authorize(p auth.Principal, want auth.Permission, appID string) error {
	if err := p.AuthorizeApp(want, appID); err != nil {
		return fmt.Errorf("testrun: %w", err)
	}
	return nil
}

func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

func failure(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such %s", what)
	}
	return fmt.Errorf("testrun: %s: %w", what, err)
}

func validStatus(s string) bool {
	return s == StatusPassed || s == StatusFailed || s == StatusSkipped
}

func tooLong(fields ...string) bool {
	for _, f := range fields {
		if len(f) > maxText {
			return true
		}
	}
	return false
}

// validate checks a report against the limits and for consistency.
func (s *Service) validate(r Report) error {
	if n := len(r.Key); n < 1 || n > 128 {
		return plxerr.New(plxerr.InvalidFormat, "the report key must be 1 to 128 bytes")
	}
	if tooLong(r.Source, r.Label) {
		return plxerr.New(plxerr.InvalidFormat, "the source and the label may have %d bytes at most", maxText)
	}
	if len(r.Scenarios) == 0 {
		return plxerr.New(plxerr.InvalidFormat, "a run needs at least one scenario")
	}
	total := int64(len(r.Scenarios))
	for _, sc := range r.Scenarios {
		total += int64(len(sc.Steps))
	}
	if max := s.o.Limits.Get(limits.TestStepsPerRun); total > max {
		return plxerr.New(plxerr.LimitExceeded, "the run has %d scenarios and steps; test.stepsPerRun allows %d", total, max)
	}
	seen := map[string]bool{}
	for _, sc := range r.Scenarios {
		if sc.ID == "" || seen[sc.ID] || !validStatus(sc.Status) || tooLong(sc.ID, sc.Name) {
			return plxerr.New(plxerr.InvalidFormat, "scenario %q needs a unique identifier and the status passed, failed or skipped", sc.ID)
		}
		seen[sc.ID] = true
		for _, st := range sc.Steps {
			if !validStatus(st.Status) || tooLong(st.Description, st.Message) {
				return plxerr.New(plxerr.InvalidFormat, "a step of scenario %q needs the status passed, failed or skipped", sc.ID)
			}
		}
	}
	return nil
}

func tally(scs []Scenario) (passed, failed, skipped int32, ms int64) {
	for _, sc := range scs {
		switch sc.Status {
		case StatusPassed:
			passed++
		case StatusFailed:
			failed++
		default:
			skipped++
		}
		ms += sc.DurationMs
	}
	return passed, failed, skipped, ms
}

func runOf(row dbgen.TestRun, withResults bool) (Run, error) {
	r := Run{
		ID: storage.ID(row.ID), AppID: storage.ID(row.AppID), Key: row.ReportKey, Source: row.Source, Label: row.Label,
		Passed: row.Passed, Failed: row.Failed, Skipped: row.Skipped, DurationMs: row.DurationMs, CreatedBy: row.CreatedBy,
		CreatedAt: storage.Time(row.CreatedAt),
	}
	if withResults {
		if err := json.Unmarshal(row.Results, &r.Scenarios); err != nil {
			return Run{}, fmt.Errorf("testrun: stored results: %w", err)
		}
	}
	return r, nil
}

// Report records a run and returns it. The same key reported again returns
// the run the first report recorded. It needs plugin.edit.
func (s *Service) Report(ctx context.Context, p auth.Principal, r Report) (Run, error) {
	if err := authorize(p, auth.PluginEdit, r.AppID); err != nil {
		return Run{}, err
	}
	app, err := storage.UUID(r.AppID)
	if err != nil {
		return Run{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	if err := s.validate(r); err != nil {
		return Run{}, err
	}
	results, err := json.Marshal(r.Scenarios)
	if err != nil {
		return Run{}, fmt.Errorf("testrun: %w", err)
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Run{}, fmt.Errorf("testrun: %w", err)
	}
	runID, err := storage.UUID(id)
	if err != nil {
		return Run{}, fmt.Errorf("testrun: %w", err)
	}
	org, err := storage.UUID(p.OrganizationID)
	if err != nil {
		return Run{}, fmt.Errorf("testrun: %w", err)
	}
	passed, failed, skipped, ms := tally(r.Scenarios)
	var out Run
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.GetApp(ctx, app); err != nil {
			return failure(err, "app")
		}
		row, err := q.InsertTestRun(ctx, dbgen.InsertTestRunParams{
			ID: runID, OrganizationID: org, AppID: app, ReportKey: r.Key, Source: r.Source, Label: r.Label,
			Passed: passed, Failed: failed, Skipped: skipped, DurationMs: ms, Results: results, CreatedBy: p.Actor().ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			row, err = q.GetTestRunByKey(ctx, dbgen.GetTestRunByKeyParams{AppID: app, ReportKey: r.Key})
		}
		if err != nil {
			return failure(err, "test run")
		}
		out, err = runOf(row, true)
		return err
	})
	return out, err
}

// List lists an app's runs, newest first; before is the run to continue
// before. It needs app.read.
func (s *Service) List(ctx context.Context, p auth.Principal, appID string, before storage.Cursor, size int32) ([]Run, error) {
	if err := authorize(p, auth.AppRead, appID); err != nil {
		return nil, err
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	params := dbgen.ListTestRunsParams{AppID: app, PageSize: size}
	if before.ID != "" {
		if params.BeforeID, err = storage.UUID(before.ID); err != nil {
			return nil, plxerr.New(plxerr.InvalidPageToken, "the page token is not valid")
		}
	}
	var out []Run
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListTestRuns(ctx, params)
		if err != nil {
			return failure(err, "test runs")
		}
		for _, row := range rows {
			r, err := runOf(row, false)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return nil
	})
	return out, err
}

// Get returns one run with its results. It needs app.read.
func (s *Service) Get(ctx context.Context, p auth.Principal, appID, runID string) (Run, error) {
	if err := authorize(p, auth.AppRead, appID); err != nil {
		return Run{}, err
	}
	app, err := storage.UUID(appID)
	if err != nil {
		return Run{}, plxerr.New(plxerr.InvalidFormat, "the app identifier is not valid")
	}
	id, err := storage.UUID(runID)
	if err != nil {
		return Run{}, plxerr.New(plxerr.InvalidFormat, "the run identifier is not valid")
	}
	var out Run
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetTestRun(ctx, dbgen.GetTestRunParams{AppID: app, ID: id})
		if err != nil {
			return failure(err, "test run")
		}
		out, err = runOf(row, true)
		return err
	})
	return out, err
}

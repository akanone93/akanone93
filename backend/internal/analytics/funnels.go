// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// minFunnelSteps is the fewest steps a funnel has; the most come from the
// analytics.funnelSteps limit.
const minFunnelSteps = 2

// Funnel is an ordered list of event names of an app (ANL-020).
type Funnel struct {
	ID, AppID, Name string
	Steps           []string
	// Environment is an environment key, empty for all; EnvironmentID is
	// its identifier.
	Environment, EnvironmentID string
	Revision                   int64
	UpdatedBy                  string
	UpdatedAt                  time.Time
}

// FunnelStep is the events of one step over a range.
type FunnelStep struct {
	Event  string
	Events int64
	// Conversion is the share of the first step's events, 0 to 1.
	Conversion float64
}

// Validate checks the name and the steps; maxSteps is the analytics.funnelSteps
// limit.
func (f Funnel) Validate(maxSteps int64) error {
	if f.Name == "" || len(f.Name) > maxNameBytes {
		return plxerr.New(plxerr.InvalidFormat, "the funnel name is 1 to %d bytes", maxNameBytes)
	}
	if len(f.Steps) < minFunnelSteps || int64(len(f.Steps)) > maxSteps {
		return plxerr.New(plxerr.OutOfRange, "a funnel has %d to %d steps, the limit analytics.funnelSteps", minFunnelSteps, maxSteps)
	}
	for _, st := range f.Steps {
		if st == "" || len(st) > maxMetricBytes {
			return plxerr.New(plxerr.InvalidFormat, "a step is an event name of 1 to %d bytes", maxMetricBytes)
		}
	}
	return nil
}

// CreateFunnel defines a funnel. It needs analytics.manage on the app and is
// audited.
func (s *Service) CreateFunnel(ctx context.Context, p auth.Principal, f Funnel) (Funnel, error) {
	if err := p.AuthorizeApp(auth.AnalyticsManage, f.AppID); err != nil {
		return Funnel{}, fmt.Errorf("analytics: %w", err)
	}
	if err := f.Validate(s.o.Limits.Get(limits.AnalyticsFunnelSteps)); err != nil {
		return Funnel{}, err
	}
	steps, err := json.Marshal(f.Steps)
	if err != nil {
		return Funnel{}, fmt.Errorf("analytics: encode the steps: %w", err)
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Funnel{}, fmt.Errorf("analytics: %w", err)
	}
	var out Funnel
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		n, err := q.CountFunnels(ctx, storage.MustUUID(p.OrganizationID))
		if err != nil {
			return fmt.Errorf("analytics: count the funnels: %w", err)
		}
		if n >= s.maxRules() {
			return plxerr.New(plxerr.LimitExceeded, "an organisation has at most %d funnels (api.pageSize)", s.maxRules())
		}
		env, err := s.funnelEnvironment(ctx, q, f)
		if err != nil {
			return err
		}
		row, err := q.InsertFunnel(ctx, dbgen.InsertFunnelParams{
			ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), AppID: storage.MustUUID(f.AppID),
			Name: f.Name, Steps: steps, EnvironmentID: env, CreatedBy: p.Actor().ID, CreatedAt: storage.Timestamp(s.now()),
		})
		if err != nil {
			return funnelFailure(err)
		}
		if out, err = funnelOf(row); err != nil {
			return err
		}
		out.Environment = f.Environment
		return s.auditFunnel(ctx, tx, p, audit.FunnelSet, out, "created")
	})
	return out, err
}

// funnelEnvironment resolves the environment key of a funnel to its row.
func (*Service) funnelEnvironment(ctx context.Context, q *dbgen.Queries, f Funnel) (pgtype.UUID, error) {
	if f.Environment == "" {
		return pgtype.UUID{}, nil
	}
	env, err := q.GetEnvironmentByKey(ctx, dbgen.GetEnvironmentByKeyParams{AppID: storage.MustUUID(f.AppID), Key: f.Environment})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, plxerr.New(plxerr.ResourceNotFound, "the app has no environment %q", f.Environment)
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("analytics: environment: %w", err)
	}
	return env.ID, nil
}

// DeleteFunnel removes a funnel. It needs analytics.manage on its app.
func (s *Service) DeleteFunnel(ctx context.Context, p auth.Principal, funnelID string) error {
	id, err := storage.UUID(funnelID)
	if err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the funnel identifier is not valid")
	}
	return s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		cur, err := s.ownedFunnel(ctx, q, p, id, auth.AnalyticsManage)
		if err != nil {
			return err
		}
		if _, err := q.DeleteFunnel(ctx, dbgen.DeleteFunnelParams{ID: id, OrganizationID: cur.OrganizationID}); err != nil {
			return fmt.Errorf("analytics: delete the funnel: %w", err)
		}
		f, err := funnelOf(cur)
		if err != nil {
			return err
		}
		return s.auditFunnel(ctx, tx, p, audit.FunnelDeleted, f, "deleted")
	})
}

// ownedFunnel loads a funnel of the principal's organisation and checks the
// permission on its app; a funnel of another organisation is not found.
func (*Service) ownedFunnel(ctx context.Context, q *dbgen.Queries, p auth.Principal, id pgtype.UUID, want auth.Permission) (dbgen.Funnel, error) {
	row, err := q.GetFunnel(ctx, dbgen.GetFunnelParams{ID: id, OrganizationID: storage.MustUUID(p.OrganizationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Funnel{}, plxerr.New(plxerr.ResourceNotFound, "there is no such funnel")
	}
	if err != nil {
		return dbgen.Funnel{}, fmt.Errorf("analytics: load the funnel: %w", err)
	}
	if err := p.AuthorizeApp(want, storage.ID(row.AppID)); err != nil {
		return dbgen.Funnel{}, fmt.Errorf("analytics: %w", err)
	}
	return row, nil
}

// ListFunnels lists the funnels of an app, or, with no app, those of the
// organisation on apps the principal may read analytics of.
func (s *Service) ListFunnels(ctx context.Context, p auth.Principal, appID string) ([]Funnel, error) {
	app, err := s.readScope(p, appID)
	if err != nil {
		return nil, err
	}
	var out []Funnel
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		rows, err := q.ListFunnels(ctx, dbgen.ListFunnelsParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), AppID: app, RowLimit: int32(s.maxRules()), //nolint:gosec // G115: a page size.
		})
		if err != nil {
			return fmt.Errorf("analytics: list the funnels: %w", err)
		}
		for _, row := range rows {
			if !p.HoldsOnApp(auth.AnalyticsRead, storage.ID(row.AppID)) {
				continue
			}
			f, err := s.funnelWithEnvironment(ctx, q, row)
			if err != nil {
				return err
			}
			out = append(out, f)
		}
		return nil
	})
	return out, err
}

func (*Service) funnelWithEnvironment(ctx context.Context, q *dbgen.Queries, row dbgen.Funnel) (Funnel, error) {
	f, err := funnelOf(row)
	if err != nil || !row.EnvironmentID.Valid {
		return f, err
	}
	env, err := q.GetEnvironment(ctx, row.EnvironmentID)
	if err != nil {
		return Funnel{}, fmt.Errorf("analytics: environment: %w", err)
	}
	f.Environment = env.Key
	return f, nil
}

// FunnelResult counts the events of every step of a funnel over a range,
// from the daily rollups. It needs analytics.read on the funnel's app.
func (s *Service) FunnelResult(ctx context.Context, p auth.Principal, funnelID string, from, to time.Time) (Funnel, []FunnelStep, error) {
	id, err := storage.UUID(funnelID)
	if err != nil {
		return Funnel{}, nil, plxerr.New(plxerr.InvalidFormat, "the funnel identifier is not valid")
	}
	var f Funnel
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := s.ownedFunnel(ctx, q, p, id, auth.AnalyticsRead)
		if err != nil {
			return err
		}
		f, err = s.funnelWithEnvironment(ctx, q, row)
		return err
	})
	if err != nil {
		return Funnel{}, nil, err
	}
	steps := make([]FunnelStep, 0, len(f.Steps))
	for _, ev := range f.Steps {
		series, err := s.Query(ctx, p, Query{AppID: f.AppID, EnvironmentID: f.EnvironmentID, Metric: ev, Granularity: Day, From: from, To: to})
		if err != nil {
			return Funnel{}, nil, err
		}
		var n int64
		for _, pt := range series.Points {
			n += pt.Events
		}
		steps = append(steps, FunnelStep{Event: ev, Events: n})
	}
	for i := range steps {
		if steps[0].Events > 0 {
			steps[i].Conversion = float64(steps[i].Events) / float64(steps[0].Events)
		}
	}
	return f, steps, nil
}

// OverviewApp is the headline numbers of one app.
type OverviewApp struct {
	AppID                         string
	Sessions, ScreenViews, Errors int64
	FiringAlerts                  int
}

// Overview summarises every app of the organisation the principal may read
// analytics of, over a range.
func (s *Service) Overview(ctx context.Context, p auth.Principal, from, to time.Time) ([]OverviewApp, error) {
	if err := p.Authorize(auth.AnalyticsRead); err != nil {
		return nil, fmt.Errorf("analytics: %w", err)
	}
	var apps []string
	err := s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListActiveAppIDs(ctx, dbgen.ListActiveAppIDsParams{OrganizationID: storage.MustUUID(p.OrganizationID), RowLimit: int32(s.maxRules())}) //nolint:gosec // G115: a page size.
		if err != nil {
			return fmt.Errorf("analytics: list the apps: %w", err)
		}
		for _, r := range rows {
			if id := storage.ID(r); p.HoldsOnApp(auth.AnalyticsRead, id) {
				apps = append(apps, id)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]OverviewApp, 0, len(apps))
	for _, app := range apps {
		o := OverviewApp{AppID: app}
		for _, m := range []struct {
			name string
			into *int64
		}{{"session_start", &o.Sessions}, {"screen_view", &o.ScreenViews}, {EventError, &o.Errors}} {
			series, err := s.Query(ctx, p, Query{AppID: app, Metric: m.name, Granularity: Day, From: from, To: to})
			if err != nil {
				return nil, err
			}
			for _, pt := range series.Points {
				*m.into += pt.Events
			}
		}
		states, err := s.ListAlertStates(ctx, p, app, PhaseFiring)
		if err != nil {
			return nil, err
		}
		o.FiringAlerts = len(states)
		out = append(out, o)
	}
	return out, nil
}

func (s *Service) auditFunnel(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, f Funnel, what string) error {
	_, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		Action: action, TargetKind: "funnel", TargetID: f.ID, OrganizationID: p.OrganizationID, Actor: p.Actor(),
		Detail: fmt.Sprintf("%s %q: %d steps", what, f.Name, len(f.Steps)),
	})
	return err //nolint:wrapcheck // wrapped by the audit log
}

func funnelFailure(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return plxerr.New(plxerr.DuplicateKey, "the app already has a funnel of that name")
	}
	if errors.As(err, &pg) && pg.Code == "23503" {
		return plxerr.New(plxerr.ResourceNotFound, "the app or environment does not exist")
	}
	return fmt.Errorf("analytics: save the funnel: %w", err)
}

func funnelOf(row dbgen.Funnel) (Funnel, error) {
	f := Funnel{
		ID: storage.ID(row.ID), AppID: storage.ID(row.AppID), Name: row.Name, EnvironmentID: storage.ID(row.EnvironmentID),
		Revision: row.Revision, UpdatedBy: row.UpdatedBy, UpdatedAt: storage.Time(row.UpdatedAt),
	}
	if err := json.Unmarshal(row.Steps, &f.Steps); err != nil {
		return Funnel{}, fmt.Errorf("analytics: the steps of funnel %s: %w", f.ID, err)
	}
	return f, nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// The data classes retention applies to (maintainer decision Q17).
const (
	// ClassRawEvents are the raw runtime events.
	ClassRawEvents = "raw_events"
	// ClassRollups are the rollups at every granularity.
	ClassRollups = "rollups"
	// ClassAuditProduction is the audit log of production environments.
	// Its retention is recorded and shown, but nothing deletes audit
	// entries yet: the chain must stay verifiable (SEC-140), and a way to
	// cut it without breaking verification is a later milestone's.
	ClassAuditProduction = "audit_production"
)

// Classes lists the data classes, in order.
func Classes() []string { return []string{ClassAuditProduction, ClassRawEvents, ClassRollups} }

// maxRetentionDays is the longest retention that can be set: a hundred
// years, which is "for ever" for any real policy; zero means indefinitely.
const maxRetentionDays = 36500

// Defaults are the installation's retention when an organisation sets none.
// Rollups default to indefinitely.
type Defaults struct {
	RawDays, AuditDays int
}

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// Options configures a Service.
type Options struct {
	DB    *storage.DB
	Store Store
	Audit *audit.Log
	IDs   IDs
	// Limits supply api.pageSize, the most points a query returns.
	Limits   limits.Set
	Now      func() time.Time
	Defaults Defaults
	Window   Window
	// Organizations calls f with every organisation's identifier.
	Organizations func(ctx context.Context, f func(org string) error) error
	// Export configures the OTLP export.
	Export ExportOptions
	// Notify announces alerts that fire and resolve (ANL-030); without it
	// rules are still evaluated and their states kept, but nobody is told.
	Notify notify.Publisher
	// Objects reads the retained source maps and bundles that error groups
	// are symbolicated from (ANL-040, CMP-041); without it groups are listed
	// without a document path.
	Objects objects.Store
	// Exporter runs the enterprise Parquet and webhook exports (ANL-011);
	// without it the exports are refused with PLX-8600 and none is run.
	Exporter Exporter
	// Crypter seals the signing secrets of the webhook export; it is
	// required with an Exporter.
	Crypter signing.Crypter
	// WebhookClient posts the webhook export; the default is an SSRF-safe
	// client.
	WebhookClient *http.Client
}

// Service is analytics for the API and the worker.
type Service struct {
	o   Options
	now func() time.Time

	mu sync.Mutex
	// compactedAt is when this process last dropped old partitions, so the
	// five-minute round does it once a day.
	compactedAt time.Time
}

// New returns the service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Store == nil || o.IDs == nil || o.Audit == nil || o.Organizations == nil {
		return nil, errors.New("analytics: a database, store, identifiers, audit log and organisation iterator are required")
	}
	if o.Window.Location == nil {
		return nil, errors.New("analytics: the maintenance window is required")
	}
	if o.Exporter != nil {
		if o.Crypter == nil {
			return nil, errors.New("analytics: the file exports need a crypter for their signing secrets")
		}
		if o.WebhookClient == nil {
			client, err := defaultWebhookClient()
			if err != nil {
				return nil, err
			}
			o.WebhookClient = client
		}
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	export, err := o.Export.withDefaults()
	if err != nil {
		return nil, err
	}
	o.Export = export
	return &Service{o: o, now: now}, nil
}

func (s *Service) inOrg(ctx context.Context, org, user string, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org, UserID: user}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// Query reads the rollups of one app and event name (ANL-010).
func (s *Service) Query(ctx context.Context, p auth.Principal, q Query) (Series, error) {
	if err := p.AuthorizeApp(auth.AnalyticsRead, q.AppID); err != nil {
		return Series{}, fmt.Errorf("analytics: %w", err)
	}
	if q.Metric == "" || len(q.Metric) > 64 {
		return Series{}, plxerr.New(plxerr.InvalidFormat, "the metric is an event name of at most 64 bytes")
	}
	if !q.Granularity.Valid() {
		return Series{}, plxerr.New(plxerr.InvalidEnumValue, "%q is not a granularity; use 5m, 1h or 1d", q.Granularity)
	}
	if q.From.IsZero() || q.To.IsZero() || !q.To.After(q.From) {
		return Series{}, plxerr.New(plxerr.OutOfRange, "the time range is empty or reversed")
	}
	for _, d := range q.GroupBy {
		if !slices.Contains(Dimensions(), d) {
			return Series{}, plxerr.New(plxerr.InvalidEnumValue, "%q is not a dimension; use %v", d, Dimensions())
		}
	}
	if maxRows := int(s.o.Limits.Get(limits.APIPageSize)); q.Limit <= 0 || q.Limit > maxRows {
		q.Limit = maxRows
	}
	series, err := s.o.Store.Query(ctx, p.OrganizationID, q)
	if err != nil {
		return Series{}, fmt.Errorf("analytics: query the store: %w", err)
	}
	return series, nil
}

// Setting is the retention of one data class.
type Setting struct {
	Class string
	// Days is nil for "indefinitely".
	Days *int
	// Default says the installation default applies: the organisation set
	// nothing.
	Default bool
	// Enforced says deletion at this retention happens; it does not yet
	// for the production audit log.
	Enforced  bool
	ChangedBy string
	ChangedAt time.Time
}

// Change is a retention change, scheduled or finished.
type Change struct {
	ID, Class   string
	Days        *int
	State       string
	RequestedBy string
	RequestedAt time.Time
	// ApplyAt is when a scheduled change takes effect.
	ApplyAt    time.Time
	FinishedAt time.Time
}

func changeOf(r dbgen.RetentionChange) Change {
	return Change{
		ID: storage.ID(r.ID), Class: r.DataClass, Days: intPtr(r.Days), State: r.State, RequestedBy: r.RequestedBy,
		RequestedAt: storage.Time(r.RequestedAt), ApplyAt: storage.Time(r.ApplyAt), FinishedAt: storage.Time(r.FinishedAt),
	}
}

func intPtr(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

func int32Ptr(p *int) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p) //nolint:gosec // bounded by maxRetentionDays
	return &v
}

// defaultFor is the installation default of a class.
func (s *Service) defaultFor(class string) *int {
	switch class {
	case ClassRawEvents:
		d := s.o.Defaults.RawDays
		return &d
	case ClassAuditProduction:
		d := s.o.Defaults.AuditDays
		return &d
	}
	return nil
}

// GetRetention lists the effective retention of every data class.
func (s *Service) GetRetention(ctx context.Context, p auth.Principal) ([]Setting, error) {
	if err := p.Authorize(auth.AnalyticsRead); err != nil {
		return nil, fmt.Errorf("analytics: %w", err)
	}
	var out []Setting
	err := s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.settings(ctx, dbgen.New(tx), p.OrganizationID)
		return err
	})
	return out, err
}

func (s *Service) settings(ctx context.Context, q *dbgen.Queries, org string) ([]Setting, error) {
	rows, err := q.ListRetentionSettings(ctx, storage.MustUUID(org))
	if err != nil {
		return nil, fmt.Errorf("analytics: list retention: %w", err)
	}
	set := map[string]dbgen.RetentionSetting{}
	for _, r := range rows {
		set[r.DataClass] = r
	}
	out := make([]Setting, 0, len(Classes()))
	for _, c := range Classes() {
		st := Setting{Class: c, Enforced: c != ClassAuditProduction}
		if r, ok := set[c]; ok {
			st.Days, st.ChangedBy, st.ChangedAt = intPtr(r.Days), r.ChangedBy, storage.Time(r.ChangedAt)
		} else {
			st.Days, st.Default = s.defaultFor(c), true
		}
		out = append(out, st)
	}
	return out, nil
}

// validateDays checks a requested retention; zero means indefinitely.
func validateDays(class string, days int) (*int, error) {
	if !slices.Contains(Classes(), class) {
		return nil, plxerr.New(plxerr.InvalidEnumValue, "%q is not a data class; use %v", class, Classes())
	}
	if days < 0 || days > maxRetentionDays {
		return nil, plxerr.New(plxerr.OutOfRange, "retention is 1 to %d days, or 0 for indefinitely", maxRetentionDays)
	}
	if days == 0 {
		return nil, nil
	}
	return &days, nil
}

func describe(days *int) string {
	if days == nil {
		return "indefinitely"
	}
	return fmt.Sprintf("%d days", *days)
}

// SetRetention changes the retention of a data class. By default the change
// is scheduled for the next time the maintenance window is open and the
// result carries PLX-8301; with immediate it takes effect now and the data
// past the new retention is deleted at once (Q17). Every change is audited.
func (s *Service) SetRetention(ctx context.Context, p auth.Principal, class string, days int, immediate bool) (Change, plxerr.Diagnostics, error) {
	if err := p.Authorize(auth.RetentionManage); err != nil {
		return Change{}, nil, fmt.Errorf("analytics: %w", err)
	}
	d, err := validateDays(class, days)
	if err != nil {
		return Change{}, nil, err
	}
	id, err := s.o.IDs.New()
	if err != nil {
		return Change{}, nil, fmt.Errorf("analytics: %w", err)
	}
	now := s.now()
	row := dbgen.InsertRetentionChangeParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), DataClass: class, Days: int32Ptr(d),
		RequestedBy: p.Actor().ID, RequestedAt: storage.Timestamp(now),
	}
	var diags plxerr.Diagnostics
	if immediate {
		row.State, row.ApplyAt, row.FinishedAt = "applied", storage.Timestamp(now), storage.Timestamp(now)
	} else {
		row.State, row.ApplyAt = "scheduled", storage.Timestamp(s.o.Window.NextOpen(now))
	}
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		return s.recordRetention(ctx, tx, p, row, d, immediate)
	})
	if err != nil {
		return Change{}, nil, err
	}
	ch := Change{
		ID: id, Class: class, Days: d, State: row.State, RequestedBy: row.RequestedBy, RequestedAt: now, ApplyAt: storage.Time(row.ApplyAt),
	}
	if immediate {
		ch.FinishedAt = now
		if _, err := s.purgeOrg(ctx, p.OrganizationID); err != nil {
			return ch, nil, err
		}
		return ch, nil, nil
	}
	diags = append(diags, plxerr.NewDiagnostic(plxerr.RetentionChangeScheduled, plxerr.Location{},
		"the retention of %s becomes %s at %s, in the maintenance window", class, describe(d), ch.ApplyAt.In(s.o.Window.Location).Format(time.RFC3339)))
	return ch, diags, nil
}

// recordRetention stores a retention change inside the transaction: it
// supersedes a waiting change of the class, inserts the new one, applies the
// setting when the change is immediate and audits the outcome.
func (s *Service) recordRetention(ctx context.Context, tx pgx.Tx, p auth.Principal, row dbgen.InsertRetentionChangeParams, d *int, immediate bool) error {
	q := dbgen.New(tx)
	class := row.DataClass
	// A newer request for a class replaces a waiting one.
	replaced, err := q.SupersedeScheduledChange(ctx, dbgen.SupersedeScheduledChangeParams{
		OrganizationID: row.OrganizationID, DataClass: class, FinishedAt: row.RequestedAt,
	})
	if err != nil {
		return fmt.Errorf("analytics: supersede: %w", err)
	}
	if replaced > 0 {
		if _, err := s.o.Audit.Append(ctx, tx, audit.Entry{
			Action: audit.RetentionCancelled, TargetKind: "retention", TargetID: class,
			Detail: class + ": the waiting change was replaced by a newer request", OrganizationID: p.OrganizationID, Actor: p.Actor(),
		}); err != nil {
			return fmt.Errorf("analytics: %w", err)
		}
	}
	if err := q.InsertRetentionChange(ctx, row); err != nil {
		return fmt.Errorf("analytics: record the change: %w", err)
	}
	action, detail := audit.RetentionScheduled, fmt.Sprintf("%s: %s, applies %s", class, describe(d), storage.Time(row.ApplyAt).Format(time.RFC3339))
	if immediate {
		action, detail = audit.RetentionSet, fmt.Sprintf("%s: %s, at once", class, describe(d))
		if err := q.UpsertRetentionSetting(ctx, dbgen.UpsertRetentionSettingParams{
			OrganizationID: row.OrganizationID, DataClass: class, Days: row.Days, ChangedBy: row.RequestedBy,
		}); err != nil {
			return fmt.Errorf("analytics: set the retention: %w", err)
		}
	}
	if _, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		Action: action, TargetKind: "retention", TargetID: class, Detail: detail,
		OrganizationID: p.OrganizationID, Actor: p.Actor(),
	}); err != nil {
		return fmt.Errorf("analytics: %w", err)
	}
	return nil
}

// ListChanges lists the organisation's retention changes, newest first.
func (s *Service) ListChanges(ctx context.Context, p auth.Principal, onlyScheduled bool, limit int) ([]Change, error) {
	if err := p.Authorize(auth.AnalyticsRead); err != nil {
		return nil, fmt.Errorf("analytics: %w", err)
	}
	if maxRows := int(s.o.Limits.Get(limits.APIPageSize)); limit <= 0 || limit > maxRows {
		limit = maxRows
	}
	var out []Change
	err := s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListRetentionChanges(ctx, dbgen.ListRetentionChangesParams{
			OrganizationID: storage.MustUUID(p.OrganizationID), OnlyScheduled: onlyScheduled, RowLimit: int32(limit), //nolint:gosec // bounded by api.pageSize
		})
		if err != nil {
			return fmt.Errorf("analytics: list changes: %w", err)
		}
		for _, r := range rows {
			out = append(out, changeOf(r))
		}
		return nil
	})
	return out, err
}

// CancelChange withdraws a scheduled retention change.
func (s *Service) CancelChange(ctx context.Context, p auth.Principal, id string) (Change, error) {
	if err := p.Authorize(auth.RetentionManage); err != nil {
		return Change{}, fmt.Errorf("analytics: %w", err)
	}
	cid, err := storage.UUID(id)
	if err != nil {
		return Change{}, plxerr.New(plxerr.InvalidFormat, "the change identifier is not valid")
	}
	var out Change
	err = s.inOrg(ctx, p.OrganizationID, p.UserID, func(ctx context.Context, tx pgx.Tx) error {
		r, err := dbgen.New(tx).FinishRetentionChange(ctx, dbgen.FinishRetentionChangeParams{
			ID: cid, OrganizationID: storage.MustUUID(p.OrganizationID), State: "cancelled", FinishedAt: storage.Timestamp(s.now()),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.ResourceNotFound, "no such scheduled retention change")
		}
		if err != nil {
			return fmt.Errorf("analytics: cancel: %w", err)
		}
		out = changeOf(r)
		_, err = s.o.Audit.Append(ctx, tx, audit.Entry{
			Action: audit.RetentionCancelled, TargetKind: "retention", TargetID: r.DataClass,
			Detail:         fmt.Sprintf("%s: %s, was due %s", r.DataClass, describe(intPtr(r.Days)), storage.Time(r.ApplyAt).Format(time.RFC3339)),
			OrganizationID: p.OrganizationID, Actor: p.Actor(),
		})
		return err //nolint:wrapcheck // wrapped by the audit log
	})
	return out, err
}

// applyDue applies the scheduled changes that are due, if the maintenance
// window is open, and returns how many it applied.
func (s *Service) applyDue(ctx context.Context, org string) (int, error) {
	now := s.now()
	if !s.o.Window.Open(now) {
		return 0, nil
	}
	var applied int
	err := s.inOrg(ctx, org, "", func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		orgID := storage.MustUUID(org)
		due, err := q.ScheduledRetentionChangesSince(ctx, dbgen.ScheduledRetentionChangesSinceParams{OrganizationID: orgID, ApplyAt: storage.Timestamp(now)})
		if err != nil {
			return fmt.Errorf("analytics: due changes: %w", err)
		}
		for _, c := range due {
			if _, err := q.FinishRetentionChange(ctx, dbgen.FinishRetentionChangeParams{ID: c.ID, OrganizationID: orgID, State: "applied", FinishedAt: storage.Timestamp(now)}); err != nil {
				return fmt.Errorf("analytics: finish: %w", err)
			}
			if err := q.UpsertRetentionSetting(ctx, dbgen.UpsertRetentionSettingParams{OrganizationID: orgID, DataClass: c.DataClass, Days: c.Days, ChangedBy: c.RequestedBy}); err != nil {
				return fmt.Errorf("analytics: set the retention: %w", err)
			}
			sys := auth.System(org)
			if _, err := s.o.Audit.Append(ctx, tx, audit.Entry{
				Action: audit.RetentionApplied, TargetKind: "retention", TargetID: c.DataClass,
				Detail:         fmt.Sprintf("%s: %s, requested by %s", c.DataClass, describe(intPtr(c.Days)), c.RequestedBy),
				OrganizationID: org, Actor: sys.Actor(),
			}); err != nil {
				return fmt.Errorf("analytics: %w", err)
			}
			applied++
		}
		return nil
	})
	return applied, err
}

// purgeOrg deletes what an organisation's effective retention no longer
// keeps, and returns the age of its raw-event cut-off, nil when raw events
// are kept indefinitely.
func (s *Service) purgeOrg(ctx context.Context, org string) (*time.Time, error) {
	var settings []Setting
	err := s.inOrg(ctx, org, "", func(ctx context.Context, tx pgx.Tx) error {
		var err error
		settings, err = s.settings(ctx, dbgen.New(tx), org)
		return err
	})
	if err != nil {
		return nil, err
	}
	now := s.now()
	var pu Purge
	var rawCut *time.Time
	for _, st := range settings {
		if st.Days == nil {
			continue
		}
		cut := now.AddDate(0, 0, -*st.Days)
		switch st.Class {
		case ClassRawEvents:
			pu.RawBefore, rawCut = cut, &cut
		case ClassRollups:
			pu.RollupsBefore = cut
		}
	}
	if _, err := s.o.Store.Purge(ctx, org, pu); err != nil {
		return nil, err //nolint:wrapcheck // wrapped by the store
	}
	return rawCut, nil
}

// Maintain is one run of the worker's analytics duties: it readies storage,
// folds new events into rollups, applies due retention changes and deletes
// what is past retention. A failure for one organisation does not stop the
// others; the errors are joined.
func (s *Service) Maintain(ctx context.Context) error {
	now := s.now()
	var errs []error
	if err := s.o.Store.Prepare(ctx, now, 3); err != nil {
		errs = append(errs, err)
	}
	oldest := now
	bounded := true
	orgs := 0
	err := s.o.Organizations(ctx, func(org string) error {
		orgs++
		if err := s.o.Store.Rollup(ctx, org, now); err != nil {
			errs = append(errs, err)
		}
		if _, err := s.applyDue(ctx, org); err != nil {
			errs = append(errs, err)
			return nil
		}
		cut, err := s.purgeOrg(ctx, org)
		switch {
		case err != nil:
			errs = append(errs, err)
			bounded = false
		case cut == nil:
			bounded = false
		case cut.Before(oldest):
			oldest = *cut
		}
		return nil
	})
	if err != nil {
		errs = append(errs, err)
		bounded = false
	}
	// A partition is dropped only when every organisation's raw cut-off is
	// past it; the installation default bounds an organisation without
	// settings, and any organisation keeping events indefinitely keeps all.
	if bounded && orgs > 0 && s.compactDue(now) {
		if _, err := s.o.Store.Compact(ctx, oldest); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// compactionEvery is how often old partitions are looked for.
const compactionEvery = 24 * time.Hour

// compactDue reports whether partitions should be compacted now, and notes
// that they were.
func (s *Service) compactDue(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.compactedAt) < compactionEvery {
		return false
	}
	s.compactedAt = now
	return true
}

// Rollups returns the store the service reads, for consumers such as the
// health gates of rollouts that count events by release (REL-053).
func (s *Service) Rollups() Store { return s.o.Store }

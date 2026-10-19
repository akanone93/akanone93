// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auditexport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/ext/export"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

const (
	// backoffBase is the wait after the first failure; it doubles with each
	// further one.
	backoffBase = 10 * time.Second
	// backoffCap bounds the wait.
	backoffCap = 15 * time.Minute
	// notifyAfter is the number of consecutive failures at which an
	// unreachable destination is reported, once.
	notifyAfter = 3
	// maxErrorLength bounds the error kept on a destination.
	maxErrorLength = 512
	// DefaultRunBudget is how long one run keeps starting batches.
	DefaultRunBudget = 10 * time.Second
)

// Unreachable describes a destination that has failed repeatedly
// (PLX-8500).
type Unreachable struct {
	OrganizationID string
	DestinationID  string
	Name           string
	Kind           string
	Failures       int32
	LastError      string
	NextAttemptAt  time.Time
}

// Notifier receives the report of an unreachable destination. The
// notification service of the platform implements it; nil reports nothing
// beyond the destination's status and the log.
type Notifier interface {
	DestinationUnreachable(ctx context.Context, n Unreachable) error
}

// Organizations visits every organisation.
type Organizations interface {
	ForEachOrganization(ctx context.Context, f func(organizationID string) error) error
}

// ExporterOptions configures an Exporter.
type ExporterOptions struct {
	Options
	Organizations Organizations
	// Notifier receives the report of an unreachable destination; nil
	// reports none.
	Notifier Notifier
	Log      *slog.Logger
	// Jitter returns a number in [0, 1) for the backoff; nil uses
	// math/rand/v2.
	Jitter func() float64
	// RunBudget is how long a run keeps starting batches; zero uses
	// DefaultRunBudget.
	RunBudget time.Duration
}

// Exporter delivers audit chains to their destinations. It runs in the
// worker role only (ADR-0006).
type Exporter struct {
	o      ExporterOptions
	open   opener
	now    func() time.Time
	jitter func() float64
}

// NewExporter returns the exporter.
func NewExporter(o ExporterOptions) (*Exporter, error) {
	if err := o.check(); err != nil {
		return nil, err
	}
	if o.Organizations == nil {
		return nil, errors.New("auditexport: the exporter needs the organisations to visit")
	}
	x := &Exporter{o: o, open: o.opener(), now: o.Now, jitter: o.Jitter}
	if x.now == nil {
		x.now = time.Now
	}
	if x.jitter == nil {
		x.jitter = rand.Float64 //nolint:gosec // backoff jitter, not a secret
	}
	if x.o.RunBudget <= 0 {
		x.o.RunBudget = DefaultRunBudget
	}
	if x.o.Log == nil {
		x.o.Log = slog.New(slog.DiscardHandler)
	}
	return x, nil
}

// Run makes one pass: every enabled destination whose backoff has passed
// gets batches until it has caught up, fails, or the run's budget is spent.
// A destination that fails does not stop the others; its failure is
// recorded on it. The licence is read from ctx (edition.WithProvider), which
// a worker job does not carry by itself. Run returns an error only for a
// failure of the server's own storage.
func (x *Exporter) Run(ctx context.Context) error {
	stop := x.now().Add(x.o.RunBudget)
	var errs []error
	err := x.o.Organizations.ForEachOrganization(ctx, func(org string) error {
		errs = append(errs, x.organization(ctx, org, stop))
		return nil
	})
	return errors.Join(append(errs, err)...)
}

// organization serves the due destinations of one organisation.
func (x *Exporter) organization(ctx context.Context, org string, stop time.Time) error {
	var ids []string
	err := x.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListDueAuditExportDestinations(ctx, dbgen.ListDueAuditExportDestinationsParams{
			OrganizationID: storage.MustUUID(org), Now: storage.Timestamp(x.now()),
		})
		for _, id := range rows {
			ids = append(ids, storage.ID(id))
		}
		return err //nolint:wrapcheck // wrapped below
	})
	if err != nil {
		return fmt.Errorf("auditexport: list the due destinations: %w", err)
	}
	var errs []error
	for _, id := range ids {
		errs = append(errs, x.destination(ctx, org, id, stop))
	}
	return errors.Join(errs...)
}

// held is the sink a run keeps across the batches of one destination.
type held struct {
	sink    export.Sink
	release func()
}

// close releases the sink.
func (h *held) close() {
	if h.sink == nil {
		return
	}
	_ = closeSink(h.sink)
	h.release()
	h.sink = nil
}

// destination delivers batches to one destination until it has caught up.
func (x *Exporter) destination(ctx context.Context, org, id string, stop time.Time) error {
	var h held
	defer h.close()
	for ctx.Err() == nil && x.now().Before(stop) {
		more, err := x.step(ctx, org, id, &h)
		if err != nil || !more {
			return err
		}
	}
	return nil
}

// outcome is what one step reports besides its error.
type outcome struct {
	more   bool
	report *Unreachable
}

// step delivers at most one batch in a transaction that holds the
// destination's row, so no other worker delivers to it meanwhile. The
// cursor moves in the same transaction, after the sink's acknowledgement.
func (x *Exporter) step(ctx context.Context, org, id string, h *held) (bool, error) {
	var out outcome
	err := x.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = x.batch(ctx, tx, org, id, h)
		return err
	})
	if err != nil {
		return false, err //nolint:wrapcheck // InTx wraps its own failures
	}
	if out.report != nil && x.o.Notifier != nil {
		if err := x.o.Notifier.DestinationUnreachable(ctx, *out.report); err != nil {
			x.o.Log.WarnContext(ctx, "audit export: the unreachable destination could not be reported",
				slog.String("destination", out.report.Name), slog.String("error", err.Error()))
		}
	}
	return out.more, nil
}

// batch reads, delivers and records one batch inside the transaction.
func (x *Exporter) batch(ctx context.Context, tx pgx.Tx, org, id string, h *held) (outcome, error) {
	q := dbgen.New(tx)
	row, err := q.LockAuditExportDestination(ctx, storage.MustUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return outcome{}, nil // deleted, or another worker holds it
	}
	if err != nil {
		return outcome{}, fmt.Errorf("auditexport: lock the destination: %w", err)
	}
	now := x.now()
	if !row.Enabled || storage.Time(row.NextAttemptAt).After(now) {
		return outcome{}, nil
	}
	set, err := x.o.Limits(ctx, tx, org)
	if err != nil {
		return outcome{}, fmt.Errorf("auditexport: read the limits: %w", err)
	}
	size := int32(min(set.Get(limits.AuditExportBatchEntries), math.MaxInt32)) //nolint:gosec // bounded by min
	entries, err := x.o.Audit.List(ctx, tx, org, row.CursorSequence, size)
	if err != nil || len(entries) == 0 {
		return outcome{}, err //nolint:wrapcheck // a domain error
	}
	if cause := x.deliver(ctx, row, entries, h); cause != nil {
		report, err := x.fail(ctx, q, row, cause, now)
		return outcome{report: report}, err
	}
	last := entries[len(entries)-1]
	if err := q.AdvanceAuditExportCursor(ctx, dbgen.AdvanceAuditExportCursorParams{
		ID: row.ID, CursorSequence: last.Sequence, CursorHash: last.EntryHash,
		ExportedCount: int64(len(entries)), NextAttemptAt: storage.Timestamp(now),
	}); err != nil {
		return outcome{}, fmt.Errorf("auditexport: advance the cursor: %w", err)
	}
	if row.AllowPrivate {
		x.o.Log.WarnContext(ctx, "audit export: "+WarningPrivateAddress, slog.String("destination", row.Name))
	}
	return outcome{more: int32(len(entries)) == size}, nil //nolint:gosec // bounded by size
}

// deliver checks that the entries continue the destination's cursor, builds
// the sink if the run has none and hands it the batch. It returns why the
// batch was not acknowledged.
func (x *Exporter) deliver(ctx context.Context, row dbgen.AuditExportDestination, entries []audit.Entry, h *held) error {
	if entries[0].Sequence != row.CursorSequence+1 || entries[0].PreviousHash != row.CursorHash {
		return fmt.Errorf("the audit chain does not continue from the cursor at entry %d", row.CursorSequence)
	}
	if h.sink == nil {
		sink, release, err := x.open.open(ctx, row)
		if err != nil {
			return err
		}
		h.sink, h.release = sink, release
	}
	if err := h.sink.Deliver(ctx, recordsOf(entries)); err != nil {
		h.close() // a failed connection is not reused
		return err
	}
	return nil
}

// fail records a refused batch: the cursor stays, the failure count rises
// and the next attempt waits for the backoff. It returns the report to send
// when the destination has just crossed the threshold.
func (x *Exporter) fail(ctx context.Context, q *dbgen.Queries, row dbgen.AuditExportDestination, cause error, now time.Time) (*Unreachable, error) {
	failures := row.Failures + 1
	next := now.Add(x.backoff(failures))
	text := describe(row.Name, cause)
	if err := q.RecordAuditExportFailure(ctx, dbgen.RecordAuditExportFailureParams{
		ID: row.ID, Failures: failures, NextAttemptAt: storage.Timestamp(next), LastError: text, LastErrorAt: storage.Timestamp(now),
	}); err != nil {
		return nil, fmt.Errorf("auditexport: record the failure: %w", err)
	}
	x.o.Log.WarnContext(ctx, "audit export: delivery failed",
		slog.String("destination", row.Name), slog.Int("failures", int(failures)), slog.String("error", text))
	if failures != notifyAfter {
		return nil, nil
	}
	return &Unreachable{
		OrganizationID: storage.ID(row.OrganizationID), DestinationID: storage.ID(row.ID), Name: row.Name, Kind: row.Kind,
		Failures: failures, LastError: text, NextAttemptAt: next,
	}, nil
}

// backoff is the wait after n consecutive failures: exponential from
// backoffBase, capped at backoffCap, and spread over its upper half so
// destinations that failed together do not retry together.
func (x *Exporter) backoff(n int32) time.Duration {
	d := backoffCap
	if n >= 1 && n <= 20 {
		d = min(backoffBase<<(n-1), backoffCap)
	}
	return d/2 + time.Duration(float64(d/2)*x.jitter())
}

// describe renders why a destination failed, as an error with a PLX code:
// the cause's own when it has one, PLX-8500 otherwise. It never holds a
// URL's path or query, which can carry a token.
func describe(name string, cause error) string {
	var urlErr *url.Error
	text := cause.Error()
	if errors.As(cause, &urlErr) {
		text = urlErr.Op + ": " + urlErr.Err.Error()
	}
	text = clean(text)
	if _, ok := plxerr.CodeOf(cause); ok && !errors.As(cause, &urlErr) {
		return truncate(text)
	}
	return truncate(plxerr.New(plxerr.SIEMDestinationUnreachable, "the destination %q did not accept the batch: %s", name, text).Error())
}

// clean replaces control characters.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

// truncate bounds an error text at a character boundary.
func truncate(s string) string {
	if len(s) <= maxErrorLength {
		return s
	}
	cut := maxErrorLength
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// recordsOf converts entries for a sink.
func recordsOf(entries []audit.Entry) []export.Record {
	out := make([]export.Record, len(entries))
	for i, e := range entries {
		out[i] = export.Record{
			ID: e.ID, OrganizationID: e.OrganizationID, Sequence: e.Sequence, At: e.At,
			ActorKind: e.Actor.Kind, ActorID: e.Actor.ID, ActorDisplay: e.Actor.Display, Action: string(e.Action),
			TargetKind: e.TargetKind, TargetID: e.TargetID, SourceIP: e.SourceIP, UserAgent: e.UserAgent,
			RequestID: e.RequestID, BeforeHash: e.BeforeHash, AfterHash: e.AfterHash, Detail: e.Detail,
			PreviousHash: e.PreviousHash, EntryHash: e.EntryHash,
		}
	}
	return out
}

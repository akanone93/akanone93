// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package freeze

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/governance"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Guard decides whether a production change is held back by a freeze. The
// approval engine and the release service hold one; a nil guard holds
// nothing back.
type Guard interface {
	// Check returns PLX-8010 when a freeze window is in force for the
	// environment now, and nil otherwise. The caller asks only for changes
	// to production environments, in the transaction of the change.
	Check(ctx context.Context, tx pgx.Tx, environmentKey string) error
}

// Check is Guard.Check for a guard that may be nil, which holds nothing back.
func Check(ctx context.Context, g Guard, tx pgx.Tx, environmentKey string) error {
	if g == nil {
		return nil
	}
	return g.Check(ctx, tx, environmentKey)
}

// IDs generates the identifiers of windows.
type IDs interface{ New() (string, error) }

// LimitsFunc returns the limits in force for an organisation, read in the
// transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID string) (limits.Set, error)

// Options configures a Service.
type Options struct {
	DB    *storage.DB
	Audit *audit.Log
	IDs   IDs
	// Limits reads governance.freezeWindowsPerOrg and
	// governance.freezeWindowHours.
	Limits LimitsFunc
	// Calendar evaluates the windows; nil is the community calendar.
	Calendar governance.FreezeCalendar
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Service stores freeze windows and implements Guard.
type Service struct {
	o   Options
	now func() time.Time
}

var _ Guard = (*Service)(nil)

// New returns the service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Limits == nil {
		return nil, errors.New("freeze: a database, an audit log, identifiers and limits are required")
	}
	if o.Calendar == nil {
		o.Calendar = governance.NoFreezes{}
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, now: now}, nil
}

// Check implements Guard.
func (s *Service) Check(ctx context.Context, tx pgx.Tx, environmentKey string) error {
	if !edition.Readable(ctx, edition.FeatureChangeFreezes) {
		return nil
	}
	rows, err := dbgen.New(tx).ListChangeFreezes(ctx)
	if err != nil {
		return fmt.Errorf("freeze: list the windows: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	windows := make([]governance.Window, len(rows))
	for i, r := range rows {
		windows[i] = windowOf(r)
	}
	occ, held, err := s.o.Calendar.InForce(ctx, windows, environmentKey, s.now())
	if err != nil {
		return fmt.Errorf("freeze: evaluate the windows: %w", err)
	}
	if !held {
		return nil
	}
	return (&plxerr.Error{Code: plxerr.ChangeFreezeActive, Message: refusal(occ)}).
		WithDetail("window", occ.Window.Name).WithDetail("until", occ.End.UTC().Format(time.RFC3339))
}

// refusal words the refusal of a change inside a window.
func refusal(occ governance.Occurrence) string {
	msg := fmt.Sprintf("the change freeze %q is in force until %s", occ.Window.Name, occ.End.UTC().Format(time.RFC3339))
	if occ.Window.Reason != "" {
		msg += " (" + occ.Window.Reason + ")"
	}
	return msg + "; in an emergency, use break-glass"
}

// windowOf converts a stored window.
func windowOf(r dbgen.ChangeFreeze) governance.Window {
	return governance.Window{
		ID: storage.ID(r.ID), Name: r.Name, EnvironmentKey: r.EnvironmentKey, TimeZone: r.TimeZone, Start: r.StartLocal,
		Duration: time.Duration(r.DurationMinutes) * time.Minute, Recurrence: r.Recurrence, Reason: r.Reason,
	}
}

// Window is a stored freeze window with its provenance.
type Window struct {
	governance.Window
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// stored converts a row to a Window.
func stored(r dbgen.ChangeFreeze) Window {
	return Window{Window: windowOf(r), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}

// List returns the organisation's windows, ordered by name. It needs
// organization.read and the read licence (GOV-025).
func (s *Service) List(ctx context.Context, p auth.Principal) ([]Window, error) {
	if err := p.Authorize(auth.OrganizationRead); err != nil {
		return nil, fmt.Errorf("freeze: %w", err)
	}
	if err := edition.RequireRead(ctx, edition.FeatureChangeFreezes); err != nil {
		return nil, fmt.Errorf("freeze: %w", err)
	}
	var out []Window
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListChangeFreezes(ctx)
		if err != nil {
			return fmt.Errorf("freeze: list the windows: %w", err)
		}
		for _, r := range rows {
			out = append(out, stored(r))
		}
		return nil
	})
	return out, err
}

// Put creates a window, or replaces the one of the same name. It needs
// freezes.manage, which carries a step-up, and the licence's
// FeatureChangeFreezes for writing; the calendar checks the window, the
// registry bounds its duration and the number of windows (GOV-025).
func (s *Service) Put(ctx context.Context, p auth.Principal, w governance.Window) (Window, error) {
	if err := p.Authorize(auth.FreezesManage); err != nil {
		return Window{}, fmt.Errorf("freeze: %w", err)
	}
	if err := edition.Require(ctx, edition.FeatureChangeFreezes); err != nil {
		return Window{}, fmt.Errorf("freeze: %w", err)
	}
	if err := s.check(ctx, w); err != nil {
		return Window{}, err
	}
	var out Window
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.checkLimits(ctx, tx, p.OrganizationID, w); err != nil {
			return err
		}
		row, err := s.write(ctx, tx, p, w)
		if err != nil {
			return err
		}
		out = stored(row)
		return s.audit(ctx, tx, p, audit.FreezeSet, out.ID, fmt.Sprintf("%s: %s from %s %s for %s", out.Name, out.Recurrence, out.Start, out.TimeZone, out.Duration))
	})
	return out, err
}

// check validates the window's form: the name and the calendar's reading.
func (s *Service) check(ctx context.Context, w governance.Window) error {
	if w.Name == "" || len(w.Name) > maxName {
		return plxerr.New(plxerr.InvalidFormat, "a freeze window has a name of 1 to %d bytes", maxName)
	}
	if len(w.Reason) > maxReason {
		return plxerr.New(plxerr.LimitExceeded, "the reason of a freeze window is at most %d bytes", maxReason)
	}
	if w.Duration%time.Minute != 0 {
		return plxerr.New(plxerr.InvalidFormat, "the duration of a freeze window is a whole number of minutes")
	}
	if err := s.o.Calendar.Validate(ctx, w); err != nil {
		return calendarFailure(err)
	}
	return nil
}

// Bounds on the text of a window; they keep a notification or an error
// message readable and are not capacity limits.
const (
	maxName   = 100
	maxReason = 500
)

// calendarFailure turns a calendar's refusal into the caller's.
func calendarFailure(err error) error {
	switch {
	case errors.Is(err, governance.ErrInvalidWindow):
		return plxerr.Wrap(plxerr.InvalidFormat, err, "the freeze window is not valid")
	case errors.Is(err, governance.ErrUnavailable):
		return plxerr.New(plxerr.EnterpriseFeatureNotLicensed, "the enterprise feature %q is not available in this server", string(edition.FeatureChangeFreezes))
	}
	return fmt.Errorf("freeze: %w", err)
}

// checkLimits refuses a window the registry does not allow: one too long, or
// one that would be more windows than the organisation may have.
func (s *Service) checkLimits(ctx context.Context, tx pgx.Tx, organizationID string, w governance.Window) error {
	set, err := s.o.Limits(ctx, tx, organizationID)
	if err != nil {
		return fmt.Errorf("freeze: read the limits: %w", err)
	}
	if longest := time.Duration(set.Get(limits.GovernanceFreezeWindowHours)) * time.Hour; w.Duration > longest {
		return plxerr.New(plxerr.LimitExceeded, "a freeze window lasts at most %d hours (governance.freezeWindowHours)", set.Get(limits.GovernanceFreezeWindowHours))
	}
	q := dbgen.New(tx)
	if _, err := q.GetChangeFreezeByName(ctx, w.Name); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("freeze: find the window: %w", err)
	}
	have, err := q.CountChangeFreezes(ctx)
	if err != nil {
		return fmt.Errorf("freeze: count the windows: %w", err)
	}
	if allowed := set.Get(limits.GovernanceFreezeWindowsPerOrg); have >= allowed {
		return plxerr.New(plxerr.LimitExceeded, "the organisation has %d freeze windows; governance.freezeWindowsPerOrg allows %d", have, allowed)
	}
	return nil
}

// write stores the window.
func (s *Service) write(ctx context.Context, tx pgx.Tx, p auth.Principal, w governance.Window) (dbgen.ChangeFreeze, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return dbgen.ChangeFreeze{}, fmt.Errorf("freeze: %w", err)
	}
	row, err := dbgen.New(tx).InsertChangeFreeze(ctx, dbgen.InsertChangeFreezeParams{
		ID: storage.MustUUID(id), OrganizationID: storage.MustUUID(p.OrganizationID), Name: w.Name, EnvironmentKey: w.EnvironmentKey,
		TimeZone: w.TimeZone, StartLocal: w.Start, DurationMinutes: int32(w.Duration / time.Minute), //nolint:gosec // bounded by governance.freezeWindowHours
		Recurrence: w.Recurrence, Reason: w.Reason, CreatedBy: p.Actor().Display, CreatedAt: storage.Timestamp(s.now()),
	})
	if err != nil {
		return dbgen.ChangeFreeze{}, fmt.Errorf("freeze: store the window: %w", err)
	}
	return row, nil
}

// Delete removes a window. It needs freezes.manage and the licence for
// writing.
func (s *Service) Delete(ctx context.Context, p auth.Principal, id string) error {
	if err := p.Authorize(auth.FreezesManage); err != nil {
		return fmt.Errorf("freeze: %w", err)
	}
	if err := edition.Require(ctx, edition.FeatureChangeFreezes); err != nil {
		return fmt.Errorf("freeze: %w", err)
	}
	uid, err := storage.UUID(id)
	if err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the freeze window identifier is not valid")
	}
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).DeleteChangeFreeze(ctx, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.ResourceNotFound, "no such freeze window")
		}
		if err != nil {
			return fmt.Errorf("freeze: delete the window: %w", err)
		}
		return s.audit(ctx, tx, p, audit.FreezeDeleted, id, row.Name)
	})
}

// audit appends the entry of a change to a window.
func (s *Service) audit(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, id, detail string) error {
	_, err := s.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: action, TargetKind: "change_freeze", TargetID: id, Detail: detail,
	})
	if err != nil {
		return fmt.Errorf("freeze: %w", err)
	}
	return nil
}

// inOrg runs f in a transaction of the principal's organisation.
func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f) //nolint:wrapcheck // InTx wraps its own failures
}

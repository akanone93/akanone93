// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// Lock is a draft's exclusive editing lock (SRV-040).
type Lock struct {
	AppID    string
	PluginID string
	// Scope is empty for the draft's own lock and "translations/<locale>"
	// for the lock of one locale's translations document (I18N-008).
	Scope       string
	Holder      audit.Actor
	Session     string
	AcquiredAt  time.Time
	ExpiresAt   time.Time
	RequestedBy []audit.Actor
	// LastEditAt is the holder's last accepted write under the lock, or
	// when it was taken (plan §4.3 #16).
	LastEditAt time.Time
	// TakeoverAllowedAt is the earliest time another user may take the
	// lock over (L-1, SRV-041); set by TakeoverRule.
	TakeoverAllowedAt time.Time
}

// LockLeaseLapse is how long a lock goes without a heartbeat before its
// lease counts as lapsed and it may be taken over at once: two missed
// heartbeats (L-1). The lock itself still expires LockTTL after the last
// heartbeat (SRV-040).
const LockLeaseLapse = 2 * LockHeartbeat

// TakeoverRule sets when another user may take the lock over: once the
// holder has made no edit for idle (studio.lockTakeoverIdleMinutes), or
// at once when the lease has lapsed (L-1, SRV-041).
func (l Lock) TakeoverRule(idle time.Duration) Lock {
	lastHeartbeat := l.ExpiresAt.Add(-LockTTL)
	allowed := l.LastEditAt.Add(idle)
	if lapsed := lastHeartbeat.Add(LockLeaseLapse); lapsed.Before(allowed) {
		allowed = lapsed
	}
	l.TakeoverAllowedAt = allowed
	return l
}

// translationsScopePrefix starts the scope of a locale's lock.
const translationsScopePrefix = "translations/"

// TranslationsScope is the lock scope of one locale's translations
// document, which translators edit beside the draft's own lock (I18N-008).
func TranslationsScope(locale string) string { return translationsScopePrefix + locale }

// scopeOf returns the lock scope that saving a document needs: that of its
// locale for a translations document of the app, the draft's own otherwise.
func scopeOf(path string) string {
	if place, ok := schema.PlaceOf(path); ok && place.Kind == schema.KindTranslations && place.Plugin == "" {
		return TranslationsScope(place.Key)
	}
	return ""
}

// checkScope refuses a lock scope that is not the draft's own or the lock
// of a locale of the app-level translations.
func (d draft) checkScope(scope string) error {
	if scope == "" {
		return nil
	}
	if locale, ok := strings.CutPrefix(scope, translationsScopePrefix); ok && d.plugin == nil && localePattern.MatchString(locale) {
		return nil
	}
	return plxerr.New(plxerr.InvalidFormat, "%q is not a lock scope; it is empty or translations/<locale> of the app", scope)
}

// localePattern is the locale of the document schema
// (common.schema.json#/$defs/locale): a BCP 47 language, script and region.
var localePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z][a-z]{3})?(-([A-Z]{2}|[0-9]{3}))?$`)

// The lock of a draft lives in locks; that of one locale of its
// translations in translation_locks, a table of its own with the same
// columns (SRV-021). These functions pick the table by scope and return
// the row in the shape of locks.

// localeOf returns the locale of a translations scope.
func localeOf(scope string) string { return strings.TrimPrefix(scope, translationsScopePrefix) }

// fromTranslationLock reads a locale lock as a lock row.
func fromTranslationLock(r dbgen.TranslationLock) dbgen.Lock {
	return dbgen.Lock{
		DraftID: r.DraftID, OrganizationID: r.OrganizationID, HolderKind: r.HolderKind, HolderID: r.HolderID,
		HolderDisplay: r.HolderDisplay, Session: r.Session, AcquiredAt: r.AcquiredAt, HeartbeatAt: r.HeartbeatAt,
		ExpiresAt: r.ExpiresAt, PreviousSession: r.PreviousSession, PreviousHolder: r.PreviousHolder,
		RequestedBy: r.RequestedBy, LastEditAt: r.LastEditAt,
	}
}

// getLock reads the lock of a scope, locking its row when forUpdate.
func getLock(ctx context.Context, q *dbgen.Queries, d draft, scope string, forUpdate bool) (dbgen.Lock, error) {
	switch {
	case scope == "" && forUpdate:
		return q.GetLockForUpdate(ctx, d.row.ID) //nolint:wrapcheck // callers wrap
	case scope == "":
		return q.GetLock(ctx, d.row.ID) //nolint:wrapcheck // callers wrap
	case forUpdate:
		r, err := q.GetTranslationLockForUpdate(ctx, dbgen.GetTranslationLockForUpdateParams{DraftID: d.row.ID, Locale: localeOf(scope)})
		return fromTranslationLock(r), err //nolint:wrapcheck // callers wrap
	default:
		r, err := q.GetTranslationLock(ctx, dbgen.GetTranslationLockParams{DraftID: d.row.ID, Locale: localeOf(scope)})
		return fromTranslationLock(r), err //nolint:wrapcheck // callers wrap
	}
}

// putLock takes the lock of a scope, replacing whatever held it.
func putLock(ctx context.Context, q *dbgen.Queries, d draft, scope string, a dbgen.PutLockParams) (dbgen.Lock, error) {
	if scope == "" {
		return q.PutLock(ctx, a) //nolint:wrapcheck // callers wrap
	}
	r, err := q.PutTranslationLock(ctx, dbgen.PutTranslationLockParams{
		DraftID: d.row.ID, Locale: localeOf(scope), OrganizationID: a.OrganizationID,
		HolderKind: a.HolderKind, HolderID: a.HolderID, HolderDisplay: a.HolderDisplay, Session: a.Session,
		ExpiresAt: a.ExpiresAt, PreviousSession: a.PreviousSession, PreviousHolder: a.PreviousHolder,
		LastEditAt: a.LastEditAt,
	})
	return fromTranslationLock(r), err //nolint:wrapcheck // callers wrap
}

// touchLock records an accepted write by the session holding the lock of
// a scope as the holder's last edit (plan §4.3 #16); it changes nothing
// when another session holds it.
func touchLock(ctx context.Context, q *dbgen.Queries, d draft, scope, session string, at pgtype.Timestamptz) error {
	if scope == "" {
		return q.TouchLock(ctx, dbgen.TouchLockParams{DraftID: d.row.ID, Session: session, LastEditAt: at}) //nolint:wrapcheck // callers wrap
	}
	return q.TouchTranslationLock(ctx, dbgen.TouchTranslationLockParams{ //nolint:wrapcheck // callers wrap
		DraftID: d.row.ID, Locale: localeOf(scope), Session: session, LastEditAt: at,
	})
}

// renewLock extends the lock of a scope.
func renewLock(ctx context.Context, q *dbgen.Queries, d draft, scope string, expires pgtype.Timestamptz) (dbgen.Lock, error) {
	if scope == "" {
		return q.RenewLock(ctx, dbgen.RenewLockParams{DraftID: d.row.ID, ExpiresAt: expires}) //nolint:wrapcheck // callers wrap
	}
	r, err := q.RenewTranslationLock(ctx, dbgen.RenewTranslationLockParams{DraftID: d.row.ID, Locale: localeOf(scope), ExpiresAt: expires})
	return fromTranslationLock(r), err //nolint:wrapcheck // callers wrap
}

// deleteLock gives the lock of a scope up.
func deleteLock(ctx context.Context, q *dbgen.Queries, d draft, scope string) (int64, error) {
	if scope == "" {
		return q.DeleteLock(ctx, d.row.ID) //nolint:wrapcheck // callers wrap
	}
	return q.DeleteTranslationLock(ctx, dbgen.DeleteTranslationLockParams{DraftID: d.row.ID, Locale: localeOf(scope)}) //nolint:wrapcheck // callers wrap
}

// requestLock records who asked for the lock of a scope.
func requestLock(ctx context.Context, q *dbgen.Queries, d draft, scope string, requester []byte) (dbgen.Lock, error) {
	if scope == "" {
		return q.RequestLock(ctx, dbgen.RequestLockParams{DraftID: d.row.ID, Requester: requester}) //nolint:wrapcheck // callers wrap
	}
	r, err := q.RequestTranslationLock(ctx, dbgen.RequestTranslationLockParams{DraftID: d.row.ID, Locale: localeOf(scope), Requester: requester})
	return fromTranslationLock(r), err //nolint:wrapcheck // callers wrap
}

// lockPermission is what taking or asking for a lock needs: the draft's
// edit permission, or, for a locale's lock, app.manage or translations.edit.
func (d draft) lockPermission(p auth.Principal, scope string) auth.Permission {
	if scope == "" || p.HoldsOnApp(auth.AppManage, d.app) {
		return d.editPermission()
	}
	return auth.TranslationsEdit
}

// Held reports whether the lock is in force at a moment.
func (l Lock) Held(at time.Time) bool { return l.Session != "" && at.Before(l.ExpiresAt) }

// checkSession refuses an editing session that is not a short printable
// string: the lock compares it, and the audit log records it.
func checkSession(session string) error {
	if session == "" || len(session) > 128 {
		return plxerr.New(plxerr.InvalidFormat, "an editing session is 1 to 128 characters")
	}
	for _, r := range session {
		if r < 0x21 || r > 0x7e {
			return plxerr.New(plxerr.InvalidFormat, "an editing session is printable ASCII without spaces")
		}
	}
	return nil
}

// AcquireLock takes a draft's editing lock (SRV-040). A lock held by
// someone else is refused with PLX-8020 unless force is set, which needs
// plugin.lock.override; a takeover snapshots the draft first, so the
// previous holder's work is recoverable, and is audited with both
// parties (SRV-041). The holder's own other session may take its lock
// back without the permission. Force succeeds only once the holder has
// made no edit for studio.lockTakeoverIdleMinutes or the lease has lapsed
// (L-1); before that it is refused with PLX-8020 naming takeoverAllowedAt.
func (s *Service) AcquireLock(ctx context.Context, p auth.Principal, appID, pluginID, session string, force bool) (Lock, string, error) {
	return s.AcquireScopedLock(ctx, p, appID, pluginID, session, "", force)
}

// AcquireScopedLock is AcquireLock for a lock scope: empty for the draft's
// own lock, translations/<locale> for one locale of the app's translations,
// which is held beside the draft lock (I18N-008).
func (s *Service) AcquireScopedLock(ctx context.Context, p auth.Principal, appID, pluginID, session, scope string, force bool) (Lock, string, error) {
	if err := checkSession(session); err != nil {
		return Lock{}, "", err
	}
	var (
		out       Lock
		preserved string
	)
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, pluginID)
		if err != nil {
			return err
		}
		if err := d.checkScope(scope); err != nil {
			return err
		}
		if err := authorize(p, d.lockPermission(p, scope), appID); err != nil {
			return err
		}
		if _, err := q.GetDraftForUpdate(ctx, d.row.ID); err != nil {
			return failure(err, "draft")
		}
		lim, err := s.limitsFor(ctx, tx, d)
		if err != nil {
			return err
		}
		out, preserved, err = s.takeLock(ctx, tx, q, d, p, session, scope, force, takeoverIdle(lim))
		return err
	})
	if err == nil {
		s.lockChanged(ctx, p, out, false)
	}
	return out, preserved, err
}

// takeLock takes the lock of a resolved draft whose row the caller holds
// locked, snapshotting it first when it is taken over.
func (s *Service) takeLock(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, d draft, p auth.Principal, session, scope string, force bool, idle time.Duration) (Lock, string, error) {
	current, held, err := s.currentLock(ctx, q, d, scope)
	if err != nil {
		return Lock{}, "", err
	}
	takeover := held && (current.Holder.ID != p.ID || current.Holder.Kind != p.Kind)
	if takeover && !force {
		return Lock{}, "", lockHeld(current)
	}
	preserved := ""
	if takeover {
		if err := authorize(p, auth.PluginLockOverride, d.app); err != nil {
			return Lock{}, "", err
		}
		if rule := current.TakeoverRule(idle); s.now().Before(rule.TakeoverAllowedAt) {
			e := plxerr.New(plxerr.EditingLockHeld, "%s holds the editing lock and edited it %s; it may be taken over from %s",
				display(current.Holder), current.LastEditAt.Format(time.RFC3339), rule.TakeoverAllowedAt.Format(time.RFC3339))
			return Lock{}, "", withDetail(withDetail(e, "holder", current.Holder.ID), "takeoverAllowedAt", rule.TakeoverAllowedAt.Format(time.RFC3339))
		}
		if preserved, err = s.snapshotDraft(ctx, q, d, p, reasonTakeover); err != nil {
			return Lock{}, "", err
		}
	}
	previousSession, previousHolder := "", ""
	if held && current.Session != session {
		previousSession, previousHolder = current.Session, current.Holder.ID
	}
	row, err := putLock(ctx, q, d, scope, dbgen.PutLockParams{
		DraftID: d.row.ID, OrganizationID: d.row.OrganizationID,
		HolderKind: p.Kind, HolderID: p.ID, HolderDisplay: p.Display, Session: session,
		ExpiresAt:       storage.Timestamp(s.now().Add(LockTTL)),
		PreviousSession: previousSession, PreviousHolder: previousHolder,
		LastEditAt: storage.Timestamp(s.now()),
	})
	if err != nil {
		return Lock{}, "", fmt.Errorf("document: take the lock: %w", err)
	}
	e := audit.Entry{Action: audit.LockAcquired, TargetKind: "draft", TargetID: storage.ID(d.row.ID)}
	if scope != "" {
		e.Detail = "lock " + scope
	}
	if takeover {
		e.Action = audit.LockTakenOver
		e.Detail = "took the lock from " + current.Holder.Kind + " " + current.Holder.ID + ", last edit " + current.LastEditAt.UTC().Format(time.RFC3339) + "; their draft is snapshot " + preserved
		if scope != "" {
			e.Detail = "took the lock " + scope + " from " + current.Holder.Kind + " " + current.Holder.ID + ", last edit " + current.LastEditAt.UTC().Format(time.RFC3339) + "; their draft is snapshot " + preserved
		}
	}
	return lockOf(row, d, scope), preserved, s.recordEntry(ctx, tx, p, e)
}

// RenewLock is the holder's heartbeat; it returns who has asked for the
// lock since it was taken, which is how the holder is told (SRV-041).
func (s *Service) RenewLock(ctx context.Context, p auth.Principal, appID, pluginID, session string) (Lock, error) {
	return s.RenewScopedLock(ctx, p, appID, pluginID, session, "")
}

// RenewScopedLock is RenewLock for a lock scope (see AcquireScopedLock).
func (s *Service) RenewScopedLock(ctx context.Context, p auth.Principal, appID, pluginID, session, scope string) (Lock, error) {
	var out Lock
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, pluginID)
		if err != nil {
			return err
		}
		if err := d.checkScope(scope); err != nil {
			return err
		}
		if err := s.holds(ctx, q, d, p, session, scope); err != nil {
			return err
		}
		row, err := renewLock(ctx, q, d, scope, storage.Timestamp(s.now().Add(LockTTL)))
		if err != nil {
			return fmt.Errorf("document: renew the lock: %w", err)
		}
		out = lockOf(row, d, scope)
		return nil
	})
	return out, err
}

// ReleaseLock gives the lock up.
func (s *Service) ReleaseLock(ctx context.Context, p auth.Principal, appID, pluginID, session string) error {
	return s.ReleaseScopedLock(ctx, p, appID, pluginID, session, "")
}

// ReleaseScopedLock is ReleaseLock for a lock scope (see AcquireScopedLock).
func (s *Service) ReleaseScopedLock(ctx context.Context, p auth.Principal, appID, pluginID, session, scope string) error {
	err := s.releaseScopedLock(ctx, p, appID, pluginID, session, scope)
	if err == nil {
		s.lockChanged(ctx, p, Lock{AppID: appID, PluginID: pluginID, Scope: scope}, true)
	}
	return err
}

// releaseScopedLock deletes the lock and records it.
func (s *Service) releaseScopedLock(ctx context.Context, p auth.Principal, appID, pluginID, session, scope string) error {
	return s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, pluginID)
		if err != nil {
			return err
		}
		if err := d.checkScope(scope); err != nil {
			return err
		}
		if err := s.holds(ctx, q, d, p, session, scope); err != nil {
			return err
		}
		if _, err := deleteLock(ctx, q, d, scope); err != nil {
			return fmt.Errorf("document: release the lock: %w", err)
		}
		e := audit.Entry{Action: audit.LockReleased, TargetKind: "draft", TargetID: storage.ID(d.row.ID)}
		if scope != "" {
			e.Detail = "lock " + scope
		}
		return s.recordEntry(ctx, tx, p, e)
	})
}

// RequestLock asks the holder for the lock; the holder sees the request
// on their next heartbeat (SRV-041).
func (s *Service) RequestLock(ctx context.Context, p auth.Principal, appID, pluginID string) (Lock, error) {
	return s.RequestScopedLock(ctx, p, appID, pluginID, "")
}

// RequestScopedLock is RequestLock for a lock scope (see AcquireScopedLock).
func (s *Service) RequestScopedLock(ctx context.Context, p auth.Principal, appID, pluginID, scope string) (Lock, error) {
	var out Lock
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, pluginID)
		if err != nil {
			return err
		}
		if err := d.checkScope(scope); err != nil {
			return err
		}
		if err := authorize(p, d.lockPermission(p, scope), appID); err != nil {
			return err
		}
		current, held, err := s.currentLock(ctx, q, d, scope)
		if err != nil {
			return err
		}
		if !held {
			return plxerr.New(plxerr.PreconditionFailed, "nobody holds the lock; acquire it")
		}
		requester, err := json.Marshal([]audit.Actor{p.Actor()})
		if err != nil {
			return fmt.Errorf("document: %w", err)
		}
		row, err := requestLock(ctx, q, d, scope, requester)
		if err != nil {
			return fmt.Errorf("document: request the lock: %w", err)
		}
		out = lockOf(row, d, scope)
		return s.recordEntry(ctx, tx, p, audit.Entry{
			Action: audit.LockRequested, TargetKind: "draft", TargetID: storage.ID(d.row.ID),
			Detail: "asked " + current.Holder.Kind + " " + current.Holder.ID + " for the lock",
		})
	})
	return out, err
}

// GetLock returns a draft's lock; an unheld lock has an empty session.
func (s *Service) GetLock(ctx context.Context, p auth.Principal, appID, pluginID string) (Lock, error) {
	return s.GetScopedLock(ctx, p, appID, pluginID, "")
}

// GetScopedLock is GetLock for a lock scope (see AcquireScopedLock).
func (s *Service) GetScopedLock(ctx context.Context, p auth.Principal, appID, pluginID, scope string) (Lock, error) {
	var out Lock
	err := s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		d, err := s.resolve(ctx, q, p, appID, pluginID)
		if err != nil {
			return err
		}
		if err := d.checkScope(scope); err != nil {
			return err
		}
		current, held, err := s.currentLock(ctx, q, d, scope)
		if err != nil {
			return err
		}
		if held {
			lim, err := s.limitsFor(ctx, tx, d)
			if err != nil {
				return err
			}
			out = current.TakeoverRule(takeoverIdle(lim))
		} else {
			out = Lock{AppID: d.app, PluginID: d.pluginID(), Scope: scope}
		}
		return nil
	})
	return out, err
}

// currentLock reads a draft's lock and whether it is in force.
func (s *Service) currentLock(ctx context.Context, q *dbgen.Queries, d draft, scope string) (Lock, bool, error) {
	row, err := getLock(ctx, q, d, scope, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lock{}, false, nil
	}
	if err != nil {
		return Lock{}, false, fmt.Errorf("document: read the lock: %w", err)
	}
	l := lockOf(row, d, scope)
	return l, l.Held(s.now()), nil
}

// holds refuses a principal and session that do not hold the lock.
func (s *Service) holds(ctx context.Context, q *dbgen.Queries, d draft, p auth.Principal, session, scope string) error {
	current, held, err := s.currentLock(ctx, q, d, scope)
	if err != nil {
		return err
	}
	if !held {
		return plxerr.New(plxerr.EditingLockHeld, "acquire the editing lock first; it expires %s after the last heartbeat", LockTTL)
	}
	if current.Session != session || current.Holder.ID != p.ID || current.Holder.Kind != p.Kind {
		return lockHeld(current)
	}
	return nil
}

// lockHeld is the refusal of a write or a lock held by someone else,
// naming the holder (SRV-040).
func lockHeld(l Lock) error {
	e := plxerr.New(plxerr.EditingLockHeld, "%s holds the editing lock until %s", display(l.Holder), l.ExpiresAt.Format(time.RFC3339))
	return withDetail(withDetail(e, "holder", l.Holder.ID), "expiresAt", l.ExpiresAt.Format(time.RFC3339))
}

// display names an actor for a message.
func display(a audit.Actor) string {
	if a.Display != "" {
		return a.Display
	}
	return a.Kind + " " + a.ID
}

// lockOf converts a stored lock.
func lockOf(row dbgen.Lock, d draft, scope string) Lock {
	var requested []audit.Actor
	_ = json.Unmarshal(row.RequestedBy, &requested)
	return Lock{
		AppID: d.app, PluginID: d.pluginID(), Scope: scope,
		Holder:     audit.Actor{Kind: row.HolderKind, ID: row.HolderID, Display: row.HolderDisplay},
		Session:    row.Session,
		AcquiredAt: storage.Time(row.AcquiredAt), ExpiresAt: storage.Time(row.ExpiresAt),
		RequestedBy: requested,
		LastEditAt:  storage.Time(row.LastEditAt),
	}
}

// takeoverIdle is studio.lockTakeoverIdleMinutes of a draft's limits.
func takeoverIdle(lim limits.Set) time.Duration {
	return time.Duration(lim.Get(limits.StudioLockTakeoverIdleMinutes)) * time.Minute
}

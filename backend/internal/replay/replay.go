// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package replay

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

// day is how far back the per-device count of replays looks.
const day = 24 * time.Hour

// Allowed answers whether the security profile of an environment lets its
// devices record replays (`allowSessionReplay`, ANL-050).
type Allowed func(ctx context.Context, appID, environmentID string) (bool, error)

// Options configures a Service.
type Options struct {
	DB     *storage.DB
	Limits limits.Set
	Now    func() time.Time
	// Allowed reads the environment's security profile; without it replay is
	// allowed wherever the device had consent.
	Allowed Allowed
}

// Service stores and serves replays.
type Service struct {
	o   Options
	now func() time.Time
}

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	if o.DB == nil {
		return nil, errors.New("replay: a database is required")
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	return &Service{o: o, now: now}, nil
}

// Source is the device that sent a replay.
type Source struct {
	OrganizationID, AppID, EnvironmentID, DeviceID string
	// ReplayConsent is the consent the runtime declared; a replay without it
	// is refused (SEC-161).
	ReplayConsent bool
	// Bytes is the size of the request after decompression; zero means it is
	// measured from the events.
	Bytes int
}

// Upload is a replay as a device sent it.
type Upload struct {
	ID              string
	StartedAt       time.Time
	DurationMs      int32
	ReleaseSequence int64
	Truncated       bool
	Events          []Event
}

// Result is what an upload did.
type Result struct {
	// Stored is false when the replay was already stored.
	Stored bool
	// Dropped counts the events removed because a replay may not hold them.
	Dropped int
}

// Replay is the summary of a stored replay.
type Replay struct {
	ID, AppID, DeviceID, Route    string
	StartedAt, ExpiresAt          time.Time
	DurationMs, EventCount, Bytes int32
	ReleaseSequence               int64
	Truncated                     bool
}

// Ingest checks and stores one replay. Consent, the security profile and the
// replay.* limits are the server's to enforce: the runtime filters first, and
// the server does not trust it to have done so (SEC-161, SCH-012).
func (s *Service) Ingest(ctx context.Context, src Source, u Upload) (Result, error) {
	if !src.ReplayConsent {
		return Result{}, plxerr.New(plxerr.PermissionDenied, "a session replay needs the user's consent to the replay category, which the runtime did not declare")
	}
	if s.o.Allowed != nil {
		ok, err := s.o.Allowed(ctx, src.AppID, src.EnvironmentID)
		if err != nil {
			return Result{}, fmt.Errorf("replay: %w", err)
		}
		if !ok {
			return Result{}, plxerr.New(plxerr.PermissionDenied, "the security profile of this environment does not allow session replay (allowSessionReplay)")
		}
	}
	id, err := storage.UUID(u.ID)
	if err != nil || u.ID == "" {
		return Result{}, plxerr.New(plxerr.InvalidFormat, "the replay identifier is not a UUID")
	}
	if err := s.checkSize(src, u); err != nil {
		return Result{}, err
	}
	now := s.now()
	retention := time.Duration(s.o.Limits.Get(limits.ReplayRetentionDays)) * day
	if u.StartedAt.IsZero() || u.StartedAt.After(now.Add(time.Hour)) || u.StartedAt.Before(now.Add(-retention)) || u.DurationMs < 0 || u.ReleaseSequence < 0 {
		return Result{}, plxerr.New(plxerr.OutOfRange, "the replay time is missing or out of range")
	}
	events, dropped := Sanitise(u.Events)
	raw, err := json.Marshal(events)
	if err != nil {
		return Result{}, fmt.Errorf("replay: encode the events: %w", err)
	}
	if int64(len(raw)) > s.o.Limits.Get(limits.ReplayBytesPerReplay) {
		return Result{}, plxerr.New(plxerr.IngestionOverQuota, "the replay is %d bytes, more than %s = %d", len(raw), limits.ReplayBytesPerReplay, s.o.Limits.Get(limits.ReplayBytesPerReplay))
	}
	res := Result{Dropped: dropped}
	err = s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: src.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		n, err := q.CountSessionReplaysSince(ctx, dbgen.CountSessionReplaysSinceParams{
			OrganizationID: storage.MustUUID(src.OrganizationID), DeviceID: storage.MustUUID(src.DeviceID), CreatedAt: storage.Timestamp(now.Add(-day)),
		})
		if err != nil {
			return fmt.Errorf("replay: count the device's replays: %w", err)
		}
		if max := s.o.Limits.Get(limits.ReplayPerDevicePerDay); n >= max {
			return plxerr.New(plxerr.IngestionOverQuota, "the device sent %d replays today, the limit %s = %d", n, limits.ReplayPerDevicePerDay, max)
		}
		rows, err := q.InsertSessionReplay(ctx, dbgen.InsertSessionReplayParams{
			ID: id, OrganizationID: storage.MustUUID(src.OrganizationID), AppID: storage.MustUUID(src.AppID),
			EnvironmentID: storage.MustUUID(src.EnvironmentID), DeviceID: storage.MustUUID(src.DeviceID),
			StartedAt: storage.Timestamp(u.StartedAt), DurationMs: u.DurationMs, EventCount: int32(len(events)), //nolint:gosec // G115: bounded by replay.eventsPerReplay.
			Bytes: int32(len(raw)), ReleaseSequence: u.ReleaseSequence, Truncated: u.Truncated, //nolint:gosec // G115: bounded by replay.bytesPerReplay.
			Route: firstRoute(events), Events: raw, CreatedAt: storage.Timestamp(now), ExpiresAt: storage.Timestamp(now.Add(retention)),
		})
		if err != nil {
			return fmt.Errorf("replay: store the replay: %w", err)
		}
		res.Stored = rows == 1
		return nil
	})
	return res, err //nolint:wrapcheck // wrapped inside
}

// checkSize refuses a replay over replay.eventsPerReplay events or
// replay.bytesPerReplay bytes.
func (s *Service) checkSize(src Source, u Upload) error {
	if max := s.o.Limits.Get(limits.ReplayEventsPerReplay); int64(len(u.Events)) > max {
		return plxerr.New(plxerr.LimitExceeded, "%d events, more than %s = %d", len(u.Events), limits.ReplayEventsPerReplay, max)
	}
	if max := s.o.Limits.Get(limits.ReplayBytesPerReplay); src.Bytes > 0 && int64(src.Bytes) > max {
		return plxerr.New(plxerr.IngestionOverQuota, "the replay is %d bytes, more than %s = %d", src.Bytes, limits.ReplayBytesPerReplay, max)
	}
	return nil
}

func firstRoute(events []Event) string {
	for _, e := range events {
		if e.Kind == KindScreen {
			return e.Route
		}
	}
	return ""
}

// List lists an environment's replays, newest first. It needs analytics.read
// on the app.
func (s *Service) List(ctx context.Context, p auth.Principal, appID, envID string, before storage.Cursor, size int32) ([]Replay, error) {
	if err := p.AuthorizeApp(auth.AnalyticsRead, appID); err != nil {
		return nil, fmt.Errorf("replay: %w", err)
	}
	app, err1 := storage.UUID(appID)
	env, err2 := storage.UUID(envID)
	if err1 != nil || err2 != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the app or environment identifier is not valid")
	}
	var out []Replay
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListSessionReplays(ctx, dbgen.ListSessionReplaysParams{
			AppID: app, EnvironmentID: env, Now: storage.Timestamp(s.now()),
			BeforeTime: before.BeforeTime(), BeforeID: before.BeforeID(), PageSize: size,
		})
		if err != nil {
			return fmt.Errorf("replay: list: %w", err)
		}
		for _, r := range rows {
			out = append(out, Replay{
				ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), DeviceID: storage.ID(r.DeviceID), Route: r.Route,
				StartedAt: storage.Time(r.StartedAt), ExpiresAt: storage.Time(r.ExpiresAt), DurationMs: r.DurationMs,
				EventCount: r.EventCount, Bytes: r.Bytes, ReleaseSequence: r.ReleaseSequence, Truncated: r.Truncated,
			})
		}
		return nil
	})
	return out, err //nolint:wrapcheck // wrapped inside
}

// Get returns one replay with its events. It needs analytics.read on the
// app; a replay of another organisation or app is not found.
func (s *Service) Get(ctx context.Context, p auth.Principal, appID, id string) (Replay, []Event, error) {
	if err := p.AuthorizeApp(auth.AnalyticsRead, appID); err != nil {
		return Replay{}, nil, fmt.Errorf("replay: %w", err)
	}
	app, err1 := storage.UUID(appID)
	rid, err2 := storage.UUID(id)
	if err1 != nil || err2 != nil {
		return Replay{}, nil, plxerr.New(plxerr.InvalidFormat, "the app or replay identifier is not valid")
	}
	var (
		out    Replay
		events []Event
	)
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		r, err := dbgen.New(tx).GetSessionReplay(ctx, dbgen.GetSessionReplayParams{ID: rid, AppID: app, ExpiresAt: storage.Timestamp(s.now())})
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.ResourceNotFound, "there is no such replay")
		}
		if err != nil {
			return fmt.Errorf("replay: get: %w", err)
		}
		if err := json.Unmarshal(r.Events, &events); err != nil {
			return fmt.Errorf("replay: decode the events: %w", err)
		}
		out = Replay{
			ID: storage.ID(r.ID), AppID: storage.ID(r.AppID), DeviceID: storage.ID(r.DeviceID), Route: r.Route,
			StartedAt: storage.Time(r.StartedAt), ExpiresAt: storage.Time(r.ExpiresAt), DurationMs: r.DurationMs,
			EventCount: r.EventCount, Bytes: r.Bytes, ReleaseSequence: r.ReleaseSequence, Truncated: r.Truncated,
		}
		return nil
	})
	return out, events, err //nolint:wrapcheck // wrapped inside
}

// Purge deletes the organisation's replays whose retention has passed
// (replay.retentionDays) and returns how many. It is idempotent.
func (s *Service) Purge(ctx context.Context, organizationID string) (int64, error) {
	var n int64
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).PurgeSessionReplays(ctx, dbgen.PurgeSessionReplaysParams{
			OrganizationID: storage.MustUUID(organizationID), ExpiresAt: storage.Timestamp(s.now()),
		})
		if err != nil {
			return fmt.Errorf("replay: purge: %w", err)
		}
		return nil
	})
	return n, err //nolint:wrapcheck // wrapped inside
}

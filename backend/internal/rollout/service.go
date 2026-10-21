// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Defaults of the health gate, used where a rollout leaves a value zero. They
// are statistical choices, not quotas: a sample this size keeps one crash in a
// few hundred sessions from deciding a rollout (ADR-0078).
const (
	DefaultMinSample     int64 = 200
	DefaultGateWindow          = 6 * time.Hour
	DefaultCheckInterval       = 5 * time.Minute
)

// selectionTTL is how long a replica reuses a channel's serving rollouts
// before reading them again, the same second the manifest cache keeps a
// manifest: inside the minute a pause or rollback may take to reach online
// devices (REL-006), and what lets a replica answer thousands of devices a
// second (NFR-020).
const selectionTTL = time.Second

// IDs generates identifiers.
type IDs interface{ New() (string, error) }

// LimitsFunc returns the limits in force for an app, read in the caller's
// transaction.
type LimitsFunc func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error)

// Releases is what rollouts need of the release service; *release.Service is
// one.
type Releases interface {
	// PrepareRollout authorises the principal and checks the candidate.
	PrepareRollout(ctx context.Context, p auth.Principal, appID, envID, channelKey string, candidate int64) (release.RolloutChannel, error)
	// PromoteRelease points a channel at a release.
	PromoteRelease(ctx context.Context, p auth.Principal, appID string, sequence int64, envID, channelKey string) (release.Release, error)
	// RollbackRelease creates a release equal to an earlier one and points
	// the channel at it.
	RollbackRelease(ctx context.Context, p auth.Principal, appID, envID, channelKey string, toSequence int64, notes string) (release.Release, error)
	// EnqueueChannelManifest asks the worker to sign a channel's manifests.
	EnqueueChannelManifest(ctx context.Context, tx pgx.Tx, organizationID, channelID string) error
}

// Options configures a Service.
type Options struct {
	DB       *storage.DB
	Audit    *audit.Log
	IDs      IDs
	Releases Releases
	// Limits reads an app's limits (rollouts.activePerApp).
	Limits LimitsFunc
	// Signals counts a release's events for the health gates; nil leaves
	// every gate unevaluated.
	Signals Signals
	// Notify is told when a gate pauses or rolls back a rollout; nil tells
	// no one. A failure to notify is logged and never undoes the decision.
	Notify notify.Publisher
	// Random supplies the salts; nil uses crypto/rand.
	Random io.Reader
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// MinSample, GateWindow and CheckInterval default a gate's minimum sample
	// and window, and set how often a rollout's gate is evaluated; zero
	// uses the package defaults.
	MinSample     int64
	GateWindow    time.Duration
	CheckInterval time.Duration
	// Log receives the problems that do not fail a call; nil discards them.
	Log *slog.Logger
}

// Service creates, runs and ends rollouts, and says which release a device
// receives.
type Service struct {
	o   Options
	now func() time.Time
	log *slog.Logger

	mu    sync.Mutex
	cache map[string]servingEntry
}

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.IDs == nil || o.Releases == nil || o.Limits == nil {
		return nil, errors.New("rollout: a database, an audit log, identifiers, releases and limits are required")
	}
	if o.Random == nil {
		o.Random = rand.Reader
	}
	if o.MinSample <= 0 {
		o.MinSample = DefaultMinSample
	}
	if o.GateWindow <= 0 {
		o.GateWindow = DefaultGateWindow
	}
	if o.CheckInterval <= 0 {
		o.CheckInterval = DefaultCheckInterval
	}
	s := &Service{o: o, now: o.Now, log: o.Log}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	return s, nil
}

// inOrg runs f in a transaction bound to an organisation.
func (s *Service) inOrg(ctx context.Context, org string, f func(context.Context, pgx.Tx) error) error {
	return s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: org}, f) //nolint:wrapcheck // InTx wraps its own failures
}

// newID generates an identifier.
func (s *Service) newID() (string, error) {
	id, err := s.o.IDs.New()
	if err != nil {
		return "", fmt.Errorf("rollout: %w", err)
	}
	return id, nil
}

// systemActor is who the audit log records for what the scheduler does.
func systemActor() audit.Actor { return auth.System("").Actor() }

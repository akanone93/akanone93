// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

const (
	// epochKeyPrefix names the cache key that holds an environment's
	// configuration epoch: epochKeyPrefix + app + ":" + environment. A
	// change writes the new epoch there before it commits, which is how
	// every replica learns of it (DAT-092).
	epochKeyPrefix = "plux:config:version:"
	// epochTTL is how long the epoch key lives. The key is expendable: a
	// replica that finds it missing seeds it from the database.
	epochTTL = 30 * 24 * time.Hour
	// configMaxAge bounds how long a replica trusts a snapshot without
	// reading the database, which covers a cache store that lost writes
	// in a failover.
	configMaxAge = time.Minute
	// maxSnapshots bounds the memory: past it, the cache starts over.
	maxSnapshots = 1024
)

// EpochKey is the cache key of an environment's configuration epoch.
func EpochKey(appID, environmentID string) string {
	return epochKeyPrefix + appID + ":" + environmentID
}

// StoredConfig is an environment's configuration as the store holds it,
// with sensitive values opened.
type StoredConfig struct {
	// Epoch rises with every change; LiveEpoch with every change of a
	// live value; StrictEpoch with every change of a strict live value.
	Epoch, LiveEpoch, StrictEpoch int64
	Values                        []StoredValue
}

// StoredValue is one value of a StoredConfig.
type StoredValue struct {
	Key       string
	Tier      ConfigTier
	Type      ConfigType
	Sensitive bool
	Strict    bool
	Text      string
	Version   int64
}

// ConfigSource reads configuration from the database; *Configs is one.
type ConfigSource interface {
	// Epoch returns the environment's current epoch, zero when it has no
	// configuration.
	Epoch(ctx context.Context, organizationID, environmentID string) (int64, error)
	// Load returns the environment's configuration. It reads the epochs
	// before the values, so that the values are never older than the epoch
	// says.
	Load(ctx context.Context, organizationID, environmentID string) (StoredConfig, error)
}

// ConfigSnapshot is what a replica remembers of an environment's
// configuration, valid for its epoch (DAT-092).
type ConfigSnapshot struct {
	Epoch, LiveEpoch, StrictEpoch int64

	values map[string]StoredValue
	live   []LiveValue
	loaded time.Time
}

// LiveValue is a live value as a device receives it (DAT-094).
type LiveValue struct {
	Key     string
	Type    ConfigType
	Strict  bool
	Text    string
	Version int64
}

// Live returns the live values, by key.
func (s *ConfigSnapshot) Live() []LiveValue { return slices.Clone(s.live) }

// ConfigResolver resolves server-held values for the gateway with no
// stale read: every use first reads the environment's epoch from the
// shared cache store and reloads the values from the database when the
// epoch is not the one the replica holds. When the epoch cannot be read
// the use is refused (DAT-092, PLX-6069).
type ConfigResolver struct {
	// Source loads configuration from the database.
	Source ConfigSource
	// Cache is the shared cache store that holds the epochs.
	Cache cache.Cache
	// Now is the clock; nil uses time.Now.
	Now func() time.Time

	mu        sync.Mutex
	snapshots map[string]*ConfigSnapshot
}

func (r *ConfigResolver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func unavailable(err error) error {
	return plxerr.Wrap(plxerr.ConfigStoreUnavailable, err, "the current server-held configuration cannot be checked")
}

// Epoch returns the environment's current epoch with one read of the
// cache store. A missing key is seeded from the database.
func (r *ConfigResolver) Epoch(ctx context.Context, organizationID, appID, environmentID string) (int64, error) {
	key := EpochKey(appID, environmentID)
	raw, found, err := r.Cache.Get(ctx, key)
	if err != nil {
		return 0, unavailable(err)
	}
	if found {
		return parseEpoch(raw)
	}
	epoch, err := r.Source.Epoch(ctx, organizationID, environmentID)
	if err != nil {
		return 0, unavailable(err)
	}
	first, err := r.Cache.SetNX(ctx, key, []byte(strconv.FormatInt(epoch, 10)), epochTTL)
	if err != nil {
		return 0, unavailable(err)
	}
	if first {
		return epoch, nil
	}
	// A writer set the key since: its value is the newer one.
	if raw, found, err = r.Cache.Get(ctx, key); err != nil || !found {
		return 0, unavailable(errors.New("gateway: the epoch is not readable"))
	}
	return parseEpoch(raw)
}

func parseEpoch(raw []byte) (int64, error) {
	epoch, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || epoch < 0 {
		return 0, unavailable(errors.New("gateway: the stored epoch is not a number"))
	}
	return epoch, nil
}

// Snapshot returns the environment's configuration as of its current
// epoch.
func (r *ConfigResolver) Snapshot(ctx context.Context, organizationID, appID, environmentID string) (*ConfigSnapshot, error) {
	epoch, err := r.Epoch(ctx, organizationID, appID, environmentID)
	if err != nil {
		return nil, err
	}
	now := r.now()
	r.mu.Lock()
	s := r.snapshots[environmentID]
	r.mu.Unlock()
	if s != nil && s.Epoch == epoch && now.Sub(s.loaded) < configMaxAge {
		return s, nil
	}
	stored, err := r.Source.Load(ctx, organizationID, environmentID)
	if err != nil {
		return nil, unavailable(err)
	}
	// A database behind the cache store means a change that has not
	// committed yet: its old state is the truth, and the mismatch makes
	// the next use read again.
	if stored.Epoch > epoch {
		// The cache store lost a write; the database is the truth.
		if err := r.Cache.Set(ctx, EpochKey(appID, environmentID), []byte(strconv.FormatInt(stored.Epoch, 10)), epochTTL); err != nil {
			return nil, unavailable(err)
		}
	}
	s = newSnapshot(stored, now)
	r.mu.Lock()
	if r.snapshots == nil || len(r.snapshots) >= maxSnapshots {
		r.snapshots = map[string]*ConfigSnapshot{}
	}
	r.snapshots[environmentID] = s
	r.mu.Unlock()
	return s, nil
}

func newSnapshot(c StoredConfig, now time.Time) *ConfigSnapshot {
	s := &ConfigSnapshot{Epoch: c.Epoch, LiveEpoch: c.LiveEpoch, StrictEpoch: c.StrictEpoch, values: make(map[string]StoredValue, len(c.Values)), loaded: now}
	for _, v := range c.Values {
		s.values[v.Key] = v
		if v.Tier == TierLive {
			s.live = append(s.live, LiveValue{Key: v.Key, Type: v.Type, Strict: v.Strict, Text: v.Text, Version: v.Version})
		}
	}
	slices.SortFunc(s.live, func(a, b LiveValue) int {
		switch {
		case a.Key < b.Key:
			return -1
		case a.Key > b.Key:
			return 1
		}
		return 0
	})
	return s
}

// bound returns a server-bound value; a live value or a missing one is
// refused, because an operation binds only what the gateway holds.
func (s *ConfigSnapshot) bound(key string) (StoredValue, error) {
	v, ok := s.values[key]
	if !ok {
		return StoredValue{}, plxerr.New(plxerr.PreconditionFailed, "the operation binds a configuration value that is not set")
	}
	if v.Tier != TierServerBound {
		return StoredValue{}, plxerr.New(plxerr.PreconditionFailed, "the operation binds a configuration value that is not server-bound")
	}
	return v, nil
}

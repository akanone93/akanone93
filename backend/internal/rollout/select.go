// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package rollout

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// maxServingEntries bounds the cache of serving rollouts; past it the cache
// starts over.
const maxServingEntries = 10000

// servingEntry is the rollouts a channel serves, as read until a time.
type servingEntry struct {
	rollouts []Rollout
	until    time.Time
}

var _ release.RolloutSelector = (*Service)(nil)

// SelectRollout is the release service's question at manifest time (Q10): the
// identifier of the rollout whose candidate the requesting device is inside,
// or "" for the channel's own release. A device outside a rollout therefore
// receives the previous release.
func (s *Service) SelectRollout(ctx context.Context, r release.ManifestRequest) (string, error) {
	rollouts, err := s.serving(ctx, r.OrganizationID, r.EnvironmentID, r.Channel)
	if err != nil {
		return "", err
	}
	d := deviceOf(r)
	for _, ro := range rollouts {
		if ro.Includes(d) {
			return ro.ID, nil
		}
	}
	return "", nil
}

// deviceOf reads what a manifest request tells about its device.
func deviceOf(r release.ManifestRequest) Device {
	t := r.Targeting
	return Device{
		ID: r.DeviceID, UserHash: t.UserHash, UserAttributes: t.UserAttributes,
		Attributes: Attributes{
			Build: r.HostBuild, RuntimeVersion: t.RuntimeVersion, OS: t.OS, OSVersion: t.OSVersion,
			DeviceClass: t.DeviceClass, Locale: t.Locale, Region: t.Region, Channel: r.Channel,
		},
	}
}

// serving returns the rollouts of a channel that offer their candidate, from
// the cache when it is fresh.
func (s *Service) serving(ctx context.Context, org, env, channel string) ([]Rollout, error) {
	key := org + "/" + env + "/" + channel
	now := s.now()
	s.mu.Lock()
	e, ok := s.cache[key]
	s.mu.Unlock()
	if ok && now.Before(e.until) {
		return e.rollouts, nil
	}
	var rows []dbgen.Rollout
	err := s.inOrg(ctx, org, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).ListServingRollouts(ctx, dbgen.ListServingRolloutsParams{EnvironmentID: storage.MustUUID(env), Key: channel})
		return err //nolint:wrapcheck // one statement
	})
	if err != nil {
		return nil, fmt.Errorf("rollout: serving rollouts: %w", err)
	}
	rollouts, err := rolloutsOf(rows)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.cache == nil || len(s.cache) >= maxServingEntries {
		s.cache = map[string]servingEntry{}
	}
	s.cache[key] = servingEntry{rollouts: rollouts, until: now.Add(selectionTTL)}
	s.mu.Unlock()
	return rollouts, nil
}

// forget drops the cached rollouts, after this process changed one.
func (s *Service) forget() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// rolloutsOf converts rows.
func rolloutsOf(rows []dbgen.Rollout) ([]Rollout, error) {
	out := make([]Rollout, 0, len(rows))
	for _, row := range rows {
		r, err := rolloutOf(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

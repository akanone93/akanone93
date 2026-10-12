// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"context"
	"errors"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// graceKeyPrefix keys the marker that a device's re-attestation met an
// outage of the attestation provider.
const graceKeyPrefix = "device:attestation-outage:"

// graceRemaining returns how much of the outage grace of a device is left:
// the grace starts when its attestation falls due, and a device that never
// attested, or an environment whose grace is zero, has none (SEC-009).
func (s *Service) graceRemaining(row dbgen.Device, conf Settings) time.Duration {
	if !row.AttestedAt.Valid || conf.AttestationOutageGrace <= 0 {
		return 0
	}
	due := storage.Time(row.AttestedAt).Add(conf.ReattestationInterval)
	return max(due.Add(conf.AttestationOutageGrace).Sub(s.now()), 0)
}

// graceOnOutage decides the outcome of a re-attestation whose verification
// failed with err. Evidence that fails verification is always refused. An
// outage of the provider is refused where the environment's
// attestationOutageGrace is zero or spent; otherwise the device keeps the
// level and attestation time it last had, and the marker lets its refreshes
// continue until the grace ends (SEC-009). A first registration never gets
// here: with no last known level there is nothing to keep.
func (s *Service) graceOnOutage(ctx context.Context, row dbgen.Device, conf Settings, err error) (Device, error) {
	if !errors.Is(err, attest.ErrProviderUnavailable) || s.cache == nil {
		return Device{}, err
	}
	remaining := s.graceRemaining(row, conf)
	if remaining <= 0 {
		return Device{}, err
	}
	if cacheErr := s.cache.Set(ctx, graceKeyPrefix+storage.ID(row.ID), []byte{1}, remaining); cacheErr != nil {
		return Device{}, err
	}
	return deviceOf(row), nil
}

// graceHeld reports whether a device whose attestation is due may keep
// refreshing: its re-attestation met an outage of the provider, and the
// grace of its environment has not ended. The marker is the cache's, so
// that losing it fails closed.
func (s *Service) graceHeld(ctx context.Context, row dbgen.Device, conf Settings) bool {
	if s.cache == nil || s.graceRemaining(row, conf) <= 0 {
		return false
	}
	_, found, err := s.cache.Get(ctx, graceKeyPrefix+storage.ID(row.ID))
	return err == nil && found
}

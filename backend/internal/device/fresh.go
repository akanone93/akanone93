// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// RequireFreshAttestation refuses a device whose last verified attestation
// is older than the environment's re-attestation interval (SEC-006), or
// that has none. It is what a call that declares `attestation` checks
// (FN-020): the outage grace of SEC-009 keeps a device's level for
// ordinary calls, and does not apply here, because the call asks for
// current evidence.
func (s *Service) RequireFreshAttestation(ctx context.Context, d Identity) error {
	id, err := storage.UUID(d.DeviceID)
	if err != nil {
		return plxerr.New(plxerr.InvalidFormat, "the device identifier is not valid")
	}
	var row dbgen.Device
	err = s.db.InTx(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetDevice(ctx, id)
		return err //nolint:wrapcheck // translated below
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return plxerr.New(plxerr.ResourceNotFound, "no such device")
	}
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}
	if row.RevokedAt.Valid {
		return plxerr.New(plxerr.DeviceRevoked, "the device was revoked")
	}
	conf, err := s.settingsFor(ctx, storage.ID(row.AppID), storage.ID(row.EnvironmentID))
	if err != nil {
		return err
	}
	if !row.AttestedAt.Valid || s.now().Sub(storage.Time(row.AttestedAt)) > conf.ReattestationInterval {
		return plxerr.New(plxerr.ReattestationRequired, "this call needs a fresh attestation of the device")
	}
	return nil
}

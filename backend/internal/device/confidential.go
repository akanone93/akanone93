// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"context"
	"crypto/ecdh"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// ConfidentialRecipient returns the key-agreement key a confidential
// release's content key is wrapped to for device d, or refuses with
// PLX-6050 (SEC-053, ADR-0055). The device must not be revoked, must hold
// at least the assurance level the environment's settings require for
// syncing, and must have registered a key-agreement key. A device below
// the level, or without a key, gets no content key and so cannot open the
// release.
func (s *Service) ConfidentialRecipient(ctx context.Context, d Identity) (*ecdh.PublicKey, error) {
	id, err := storage.UUID(d.DeviceID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the device identifier is not valid")
	}
	var row dbgen.Device
	err = s.db.InTx(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetDevice(ctx, id)
		return err //nolint:wrapcheck // translated below
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, plxerr.New(plxerr.ResourceNotFound, "no such device")
	}
	if err != nil {
		return nil, fmt.Errorf("device: %w", err)
	}
	conf, err := s.settingsFor(ctx, storage.ID(row.AppID), storage.ID(row.EnvironmentID))
	if err != nil {
		return nil, err
	}
	return recipientOf(row, conf)
}

// recipientOf applies the eligibility rule to a device row.
func recipientOf(row dbgen.Device, conf Settings) (*ecdh.PublicKey, error) {
	switch {
	case row.RevokedAt.Valid:
		return nil, plxerr.New(plxerr.DeviceRevoked, "the device was revoked")
	case Level(row.AssuranceLevel).Rank() < conf.MinAssuranceForSync.Rank():
		return nil, plxerr.New(plxerr.ConfidentialKeyUnavailable,
			"confidential content needs assurance %s, and this device has %s", conf.MinAssuranceForSync, row.AssuranceLevel)
	case len(row.AgreementPublicKey) == 0:
		return nil, plxerr.New(plxerr.ConfidentialKeyUnavailable, "this device registered no key-agreement key")
	}
	pub, _, err := parseP256(row.AgreementPublicKey, "key-agreement key")
	if err != nil {
		return nil, fmt.Errorf("device: the stored key-agreement key: %w", err)
	}
	key, err := pub.ECDH()
	if err != nil {
		return nil, fmt.Errorf("device: the stored key-agreement key: %w", err)
	}
	return key, nil
}

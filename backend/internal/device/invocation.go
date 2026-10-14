// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// InvocationEvidence is the platform evidence of one function invocation
// that declares `attestation` (FN-020): the device proves, with its
// attested hardware, that this request, for this function and input,
// comes from the genuine app.
type InvocationEvidence struct {
	// Binding is what the evidence must be bound to, see
	// binding.InvocationHash. It contains the DPoP proof identifier of the
	// request, which the replay cache makes single-use.
	Binding [32]byte
	// AppAttestAssertion is an iOS device's assertion over Binding.
	AppAttestAssertion []byte
	// PlayIntegrityToken is an Android device's Play Integrity
	// standard-request token whose requestHash is Binding.
	PlayIntegrityToken string
}

// VerifyInvocationEvidence checks the per-request evidence of a function
// invocation (FN-020, SEC-003, SEC-004): on iOS an App Attest assertion
// over the binding, verified with the device's attested key against a
// strictly increasing counter that is stored with a compare-and-set; on
// Android a Play Integrity token bound to it. Evidence a device does not
// carry, or that is not bound to this request, is PLX-6001.
func (s *Service) VerifyInvocationEvidence(ctx context.Context, d Identity, e InvocationEvidence) error {
	row, err := s.liveDevice(ctx, d)
	if err != nil {
		return err
	}
	switch {
	case len(row.AppAttestPublicKey) > 0:
		if len(e.AppAttestAssertion) == 0 {
			return plxerr.New(plxerr.AttestationFailed, "this function needs an App Attest assertion for the request")
		}
		counter, err := s.checkAssertion(ctx, row, e.AppAttestAssertion, e.Binding)
		if err != nil {
			return err
		}
		return s.recordAssertion(ctx, row, *counter)
	case row.Platform == "android":
		if e.PlayIntegrityToken == "" {
			return plxerr.New(plxerr.AttestationFailed, "this function needs a Play Integrity token for the request")
		}
		return s.checkIntegrity(ctx, row, e.PlayIntegrityToken, base64.RawURLEncoding.EncodeToString(e.Binding[:]))
	default:
		return plxerr.New(plxerr.AttestationFailed, "this device cannot produce platform evidence for a request")
	}
}

// liveDevice reads the device of an authenticated call and refuses one
// that was revoked.
func (s *Service) liveDevice(ctx context.Context, d Identity) (dbgen.Device, error) {
	id, err := storage.UUID(d.DeviceID)
	if err != nil {
		return dbgen.Device{}, plxerr.New(plxerr.InvalidFormat, "the device identifier is not valid")
	}
	var row dbgen.Device
	err = s.db.InTx(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetDevice(ctx, id)
		return err //nolint:wrapcheck // translated below
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Device{}, plxerr.New(plxerr.ResourceNotFound, "no such device")
	}
	if err != nil {
		return dbgen.Device{}, fmt.Errorf("device: %w", err)
	}
	if row.RevokedAt.Valid {
		return dbgen.Device{}, plxerr.New(plxerr.DeviceRevoked, "the device was revoked")
	}
	return row, nil
}

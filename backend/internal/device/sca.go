// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest"
	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// SCAKey is the public half of a device's strong customer authentication
// key, which the server verified at enrolment (SEC-027).
type SCAKey struct {
	// Public is the key that verifies transaction signatures.
	Public *ecdsa.PublicKey
	// JKT is its RFC 7638 thumbprint.
	JKT string
	// Storage is where the attestation proved the private key lives.
	Storage KeyStorage
	// Provider is the evidence that backed the enrolment: "key_attestation"
	// for an Android chain, "app_attest" or "development".
	Provider string
	// EnrolledAt is when the key was stored.
	EnrolledAt time.Time
}

// SCAEnrolment is what a device presents to enrol its SCA key (SEC-027).
type SCAEnrolment struct {
	// Challenge is a registration challenge the evidence is bound to.
	Challenge []byte
	// KeyJWK is the SCA public key as a JSON Web Key.
	KeyJWK []byte
	// Evidence attests the key: an Android Key Attestation chain of the
	// SCA key, or an App Attest attestation bound to the key's
	// thumbprint.
	Evidence Evidence
}

// scaProvider names the evidence behind an SCA key.
const (
	scaProviderKeyAttestation = "key_attestation"
	scaProviderAppAttest      = "app_attest"
)

// EnrolSCAKey stores the SCA public key of an authenticated device after
// verifying its evidence against a challenge this server issued. A device
// has one SCA key; enrolling again replaces it, as a change of enrolled
// biometrics invalidates the old key (SEC-027).
//
// On Android the evidence is the Key Attestation chain of the SCA key
// itself: it must be hardware-backed and must demand user authentication.
// On iOS App Attest cannot attest a Secure Enclave key, so the evidence is
// an App Attest attestation whose client data hash binds the key's
// thumbprint: the genuine app vouches for the key, and the access control
// of the key (.biometryCurrentSet) is the app's to set.
func (s *Service) EnrolSCAKey(ctx context.Context, d Identity, e SCAEnrolment) (SCAKey, error) {
	row, err := s.deviceForReattestation(ctx, storage.MustUUID(d.DeviceID))
	if err != nil {
		return SCAKey{}, err
	}
	if err := e.Evidence.matches(row.Platform); err != nil {
		return SCAKey{}, err
	}
	appID, envID := storage.ID(row.AppID), storage.ID(row.EnvironmentID)
	conf, err := s.settingsFor(ctx, appID, envID)
	if err != nil {
		return SCAKey{}, err
	}
	if err := s.consumeChallenge(ctx, e.Challenge, appID, envID, conf.RegistrationChallengeTTL); err != nil {
		return SCAKey{}, err
	}
	production, err := s.environmentIsProduction(ctx, row.EnvironmentID)
	if err != nil {
		return SCAKey{}, err
	}
	key, err := s.verifySCA(ctx, appID, production, e)
	if err != nil {
		return SCAKey{}, err
	}
	der, err := x509.MarshalPKIXPublicKey(key.Public)
	if err != nil {
		return SCAKey{}, fmt.Errorf("device: encode the SCA key: %w", err)
	}
	err = s.db.InTx(ctx, storage.Tenant{OrganizationID: storage.ID(row.OrganizationID)}, func(ctx context.Context, tx pgx.Tx) error {
		if err := putSCAKey(ctx, tx, row.ID, row.OrganizationID, der, key); err != nil {
			return err
		}
		return s.recordSCA(ctx, tx, row, key)
	})
	if err != nil {
		return SCAKey{}, err //nolint:wrapcheck // InTx wraps its own failures
	}
	key.EnrolledAt = s.now()
	return key, nil
}

// recordSCA audits an enrolment; the detail names the key's thumbprint,
// never the key.
func (s *Service) recordSCA(ctx context.Context, tx pgx.Tx, row dbgen.Device, key SCAKey) error {
	if s.audit == nil {
		return nil
	}
	_, err := s.audit.Append(ctx, tx, audit.Entry{
		OrganizationID: storage.ID(row.OrganizationID),
		Actor:          audit.Actor{Kind: "device", ID: storage.ID(row.ID), Display: "device " + storage.ID(row.ID)},
		Action:         audit.SCAKeyEnrolled, TargetKind: "device", TargetID: storage.ID(row.ID),
		Detail: fmt.Sprintf("jkt %s, storage %s, evidence %s", key.JKT, key.Storage, key.Provider),
	})
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}
	return nil
}

// putSCAKey stores a verified key for a device.
func putSCAKey(ctx context.Context, tx pgx.Tx, device, organization pgtype.UUID, der []byte, key SCAKey) error {
	err := dbgen.New(tx).UpsertDeviceSCAKey(ctx, dbgen.UpsertDeviceSCAKeyParams{
		DeviceID: device, OrganizationID: organization,
		PublicKey: der, Jkt: key.JKT, KeyStorage: string(key.Storage), AttestationProvider: key.Provider,
	})
	if err != nil {
		return fmt.Errorf("device: store the SCA key: %w", err)
	}
	return nil
}

// verifySCA checks the evidence of an SCA key.
func (s *Service) verifySCA(ctx context.Context, appID string, production bool, e SCAEnrolment) (SCAKey, error) {
	key, _, jkt, err := parseKey(e.KeyJWK)
	if err != nil {
		return SCAKey{}, plxerr.New(plxerr.InvalidFormat, "the SCA key is not an EC P-256 public key in JWK form")
	}
	switch {
	case e.Evidence.Development != nil:
		if production {
			return SCAKey{}, plxerr.New(plxerr.DevProviderInProduction, "the development provider is not accepted in a production environment")
		}
		if !s.developmentProvider {
			return SCAKey{}, plxerr.New(plxerr.AttestationFailed, "the development provider is not enabled")
		}
		return SCAKey{Public: key, JKT: jkt, Storage: KeyStorageSoftware, Provider: string(ProviderDevelopment)}, nil
	case e.Evidence.Android != nil:
		return s.verifySCAAndroid(ctx, appID, key, jkt, e)
	default:
		return s.verifySCAIOS(ctx, appID, key, jkt, e)
	}
}

// verifySCAAndroid checks a Key Attestation chain of the SCA key: the
// chain must lead to a trusted root, be bound to the challenge, name one
// of the app's builds, protect the key in secure hardware and demand user
// authentication before each use. A Play Integrity token is not asked
// for: the DPoP key's attestation covers the installation.
func (s *Service) verifySCAAndroid(ctx context.Context, appID string, key *ecdsa.PublicKey, jkt string, e SCAEnrolment) (SCAKey, error) {
	trust, err := s.trustOf(ctx, appID)
	if err != nil {
		return SCAKey{}, err
	}
	if s.attestors.KeyAttestation == nil || len(trust.AndroidPackages) == 0 {
		return SCAKey{}, plxerr.New(plxerr.AttestationUnavailable, "Android attestation is not configured for this app")
	}
	chain := e.Evidence.Android.KeyAttestationChain
	if len(chain) == 0 {
		return SCAKey{}, plxerr.New(plxerr.AttestationFailed, "the SCA key needs a key attestation chain")
	}
	res, err := s.attestors.KeyAttestation.Verify(chain, e.Challenge, keyattest.Policy{
		PackageNames: trust.AndroidPackages, SigningCertDigests: trust.AndroidCertDigests,
		RequireHardware: true, RequireUserAuth: true,
	})
	if err != nil {
		return SCAKey{}, err //nolint:wrapcheck // a domain error
	}
	if res.PublicKey == nil || !res.PublicKey.Equal(key) {
		return SCAKey{}, plxerr.New(plxerr.AttestationFailed, "the attested key is not the SCA key")
	}
	return SCAKey{Public: key, JKT: jkt, Storage: androidStorage(res), Provider: scaProviderKeyAttestation}, nil
}

// verifySCAIOS checks an App Attest attestation whose client data hash
// binds the SCA key's thumbprint to the challenge.
func (s *Service) verifySCAIOS(ctx context.Context, appID string, key *ecdsa.PublicKey, jkt string, e SCAEnrolment) (SCAKey, error) {
	trust, err := s.trustOf(ctx, appID)
	if err != nil {
		return SCAKey{}, err
	}
	if s.attestors.AppAttest == nil || trust.IOSAppID == "" {
		return SCAKey{}, plxerr.New(plxerr.AttestationUnavailable, "iOS attestation is not configured for this app")
	}
	ios := e.Evidence.IOS
	if len(ios.KeyID) == 0 || len(ios.AttestationObject) == 0 {
		return SCAKey{}, plxerr.New(plxerr.AttestationFailed, "iOS evidence needs an App Attest key identifier and attestation object")
	}
	env := appattest.Development
	if trust.AppAttestProduction {
		env = appattest.Production
	}
	if _, err := s.attestors.AppAttest.VerifyAttestation(ios.AttestationObject, ios.KeyID, appattest.ClientDataHash(e.Challenge, jkt), trust.IOSAppID, env); err != nil {
		return SCAKey{}, err //nolint:wrapcheck // a domain error
	}
	return SCAKey{Public: key, JKT: jkt, Storage: KeyStorageSecureEnclave, Provider: scaProviderAppAttest}, nil
}

// SCAKey returns the SCA key a device enrolled, or ResourceNotFound when
// it has none. It reads in the device's organisation.
func (s *Service) SCAKey(ctx context.Context, d Identity) (SCAKey, error) {
	var row dbgen.DeviceScaKey
	err := s.db.InTx(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetDeviceSCAKey(ctx, storage.MustUUID(d.DeviceID))
		return err //nolint:wrapcheck // translated below
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SCAKey{}, plxerr.New(plxerr.ResourceNotFound, "the device has no SCA key")
	}
	if err != nil {
		return SCAKey{}, fmt.Errorf("device: %w", err)
	}
	parsed, err := x509.ParsePKIXPublicKey(row.PublicKey)
	if err != nil {
		return SCAKey{}, fmt.Errorf("device: the stored SCA key does not parse: %w", err)
	}
	pub, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return SCAKey{}, errors.New("device: the stored SCA key is not an ECDSA key")
	}
	return SCAKey{
		Public: pub, JKT: row.Jkt, Storage: KeyStorage(row.KeyStorage), Provider: row.AttestationProvider,
		EnrolledAt: storage.Time(row.EnrolledAt),
	}, nil
}

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

	"github.com/nightCode42/plux3/backend/internal/attest/binding"
	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/dpop"
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
	// SCA key, or an App Attest assertion over the binding hash with the
	// App Attest key stored at registration (IOSEvidence.Assertion).
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
// an assertion with the App Attest key stored at registration over the
// binding hash of the challenge and the thumbprints of the DPoP,
// key-agreement and new SCA keys: the genuine app vouches for the key, and
// the access control of the key (.biometryCurrentSet) is the app's to set.
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
	key, counter, err := s.verifyEnrolment(ctx, row, production, e)
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
		if counter != nil {
			if err := advanceCounter(ctx, tx, row, *counter); err != nil {
				return err
			}
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

// verifyEnrolment checks the evidence of a later SCA enrolment. On iOS it
// also returns the App Attest counter the assertion advanced to.
func (s *Service) verifyEnrolment(ctx context.Context, row dbgen.Device, production bool, e SCAEnrolment) (SCAKey, *uint32, error) {
	appID := storage.ID(row.AppID)
	if e.Evidence.IOS == nil {
		key, err := s.verifySCA(ctx, appID, production, e)
		return key, nil, err
	}
	key, _, jkt, err := parseKey(e.KeyJWK)
	if err != nil {
		return SCAKey{}, nil, plxerr.New(plxerr.InvalidFormat, "the SCA key is not an EC P-256 public key in JWK form")
	}
	counter, err := s.verifySCAAssertion(ctx, row, jkt, e)
	if err != nil {
		return SCAKey{}, nil, err
	}
	return SCAKey{Public: key, JKT: jkt, Storage: KeyStorageSecureEnclave, Provider: scaProviderAppAttest}, &counter, nil
}

// verifySCA checks the evidence of an SCA key that comes with its device's
// registration or on Android or in a development build.
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
		return SCAKey{}, plxerr.New(plxerr.AttestationFailed, "an iOS SCA key is vouched for by the registration's evidence or by an assertion")
	}
}

// verifySCAAndroid checks a Key Attestation chain of the SCA key: the
// chain must lead to a trusted root, be bound to the challenge, name one
// of the app's builds, protect the key in secure hardware and demand user
// authentication before each use. A Play Integrity token is not asked
// for: the DPoP key's attestation covers the installation, and its
// binding the thumbprint of this key at registration.
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
	res, err := s.attestors.KeyAttestation.Verify(ctx, chain, e.Challenge, keyattest.Policy{
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

// verifySCAAssertion checks the App Attest assertion of an SCA enrolment
// on iOS. It is made with the key attested at registration over the
// binding hash of the fresh challenge, the device's registered DPoP and
// key-agreement thumbprints and the new SCA key's, so it vouches for
// exactly this set of keys and cannot be replayed. It returns the counter
// the assertion advanced to.
func (s *Service) verifySCAAssertion(ctx context.Context, row dbgen.Device, scaJKT string, e SCAEnrolment) (uint32, error) {
	if len(e.Evidence.IOS.Assertion) == 0 {
		return 0, plxerr.New(plxerr.AttestationFailed, "an iOS SCA enrolment needs an App Attest assertion")
	}
	if row.DpopJkt == nil {
		return 0, plxerr.New(plxerr.AttestationFailed, "the device has no DPoP key")
	}
	trust, err := s.trustOf(ctx, storage.ID(row.AppID))
	if err != nil {
		return 0, err
	}
	if s.attestors.AppAssertions == nil || trust.IOSAppID == "" {
		return 0, plxerr.New(plxerr.AttestationUnavailable, "iOS attestation is not configured for this app")
	}
	parsed, err := x509.ParsePKIXPublicKey(row.AppAttestPublicKey)
	pub, ok := parsed.(*ecdsa.PublicKey)
	if err != nil || !ok {
		return 0, plxerr.New(plxerr.AttestationFailed, "the device has no usable App Attest key")
	}
	if row.AppAttestCounter < 0 || row.AppAttestCounter > int64(^uint32(0)) {
		return 0, plxerr.New(plxerr.AttestationFailed, "the stored App Attest counter is out of range")
	}
	kaJKT, err := storedAgreementThumbprint(row)
	if err != nil {
		return 0, err
	}
	hash := binding.Hash(e.Challenge, *row.DpopJkt, kaJKT, scaJKT)
	counter, err := s.attestors.AppAssertions.VerifyAssertion(e.Evidence.IOS.Assertion, pub, hash, trust.IOSAppID, uint32(row.AppAttestCounter))
	if err != nil {
		return 0, err //nolint:wrapcheck // a domain error
	}
	return counter, nil
}

// advanceCounter stores the App Attest counter an assertion reached, only
// while the stored counter is still the one the assertion was checked
// against: of two enrolments that carry the same assertion, one loses
// (SEC-025).
func advanceCounter(ctx context.Context, tx pgx.Tx, row dbgen.Device, counter uint32) error {
	changed, err := dbgen.New(tx).AdvanceAppAttestCounter(ctx, dbgen.AdvanceAppAttestCounterParams{
		ID: row.ID, Previous: row.AppAttestCounter, Counter: int64(counter),
	})
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}
	if changed == 0 {
		return plxerr.New(plxerr.AttestationFailed, "the App Attest assertion was already used")
	}
	return nil
}

// storedAgreementThumbprint is the thumbprint of the key-agreement key a
// device registered, empty when it has none.
func storedAgreementThumbprint(row dbgen.Device) (string, error) {
	if len(row.AgreementPublicKey) == 0 {
		return "", nil
	}
	pub, _, err := parseP256(row.AgreementPublicKey, "key-agreement key")
	if err != nil {
		return "", plxerr.New(plxerr.AttestationFailed, "the stored key-agreement key does not parse")
	}
	jkt, err := dpop.Thumbprint(pub)
	if err != nil {
		return "", err //nolint:wrapcheck // a domain error
	}
	return jkt, nil
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

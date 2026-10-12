// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest"
	"github.com/nightCode42/plux3/backend/internal/attest/binding"
	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/attest/playintegrity"
	"github.com/nightCode42/plux3/backend/internal/dpop"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// integrityMaxAge is the oldest Play Integrity verdict a registration
// accepts. The verdict is also bound to the challenge, which expires
// sooner; this bounds a verdict that was requested long before it.
const integrityMaxAge = 10 * time.Minute

// maxBuildID bounds the build identifier of a development build.
const maxBuildID = 64

// AndroidEvidence is the proof an Android device offers (SEC-003).
type AndroidEvidence struct {
	// KeyAttestationChain is the Key Attestation chain of the DPoP key,
	// DER certificates, leaf first.
	KeyAttestationChain [][]byte
	// PlayIntegrityToken is the Play Integrity token bound to the
	// challenge and the key.
	PlayIntegrityToken string
}

// IOSEvidence is the proof an iOS device offers (SEC-003).
type IOSEvidence struct {
	// KeyID is the App Attest key identifier.
	KeyID []byte
	// AttestationObject is the CBOR attestation object App Attest
	// returned.
	AttestationObject []byte
	// Assertion is an App Attest assertion over the binding hash, made
	// with the key attested at registration; it is the evidence of an SCA
	// key enrolled after registration and is empty otherwise.
	Assertion []byte
}

// DevelopmentEvidence marks a development build (SEC-004).
type DevelopmentEvidence struct {
	// BuildID identifies the development build.
	BuildID string
}

// Evidence is the platform evidence for one key; exactly one provider
// form is set.
type Evidence struct {
	Android     *AndroidEvidence
	IOS         *IOSEvidence
	Development *DevelopmentEvidence
}

// AttestedRegistration is what a device presents to register with a
// hardware-bound key (SEC-001, SEC-005).
type AttestedRegistration struct {
	AppID, Environment                         string
	Platform, OSVersion, RuntimeVersion, Build string
	// Challenge is the value CreateChallenge returned.
	Challenge []byte
	// DPoPKeyJWK is the device's DPoP public key as a JSON Web Key.
	DPoPKeyJWK []byte
	// KeyStorage is where the device claims the key lives; the
	// attestation must prove at least that.
	KeyStorage KeyStorage
	// Evidence is the attestation of the key.
	Evidence Evidence
	// SCAKeyJWK is the device's strong customer authentication key as a
	// JSON Web Key, and SCAEvidence its attestation; both empty for a
	// device that enrols the key later (SEC-027).
	SCAKeyJWK   []byte
	SCAEvidence *Evidence
	// AgreementKeyJWK is the device's key-agreement public key as a JSON
	// Web Key, for confidential bundles (SEC-053); empty when it has none.
	AgreementKeyJWK []byte
	// AgreementKeyChain is the Android Key Attestation chain of the
	// key-agreement key, DER certificates, leaf first; set on Android when
	// the device has the key and on no other platform.
	AgreementKeyChain [][]byte
}

// subject is what an evidence check needs to know about the device and
// the environment it registers in.
type subject struct {
	appID, envID string
	production   bool
	challenge    []byte
	key          *ecdsa.PublicKey
	jkt          string
	// kaJKT and scaJKT are the thumbprints of the device's key-agreement
	// and SCA keys, empty for a key it does not have; the evidence is bound
	// to them with the DPoP key's (SEC-004).
	kaJKT, scaJKT string
	claim         KeyStorage
	settings      Settings
}

// bindingHash is the value the platform evidence must be bound to.
func (sub subject) bindingHash() [32]byte {
	return binding.Hash(sub.challenge, sub.jkt, sub.kaJKT, sub.scaJKT)
}

// appAttestRecord is what is kept of an App Attest attestation.
type appAttestRecord struct {
	keyID, publicKey, receipt []byte
	counter                   int64
}

// verified is the outcome of an evidence check.
type verified struct {
	proof     Proof
	verdicts  []string
	risk      int32
	appAttest *appAttestRecord
}

// RegisterAttested registers a device that presents a DPoP key and
// platform attestation bound to a challenge this server issued (SEC-001,
// SEC-005–SEC-008). The device's assurance level is computed from what
// the evidence proved.
func (s *Service) RegisterAttested(ctx context.Context, r AttestedRegistration) (Device, error) {
	if err := checkDescription(r.Platform, r.OSVersion, r.RuntimeVersion, r.Build); err != nil {
		return Device{}, err
	}
	if err := r.Evidence.matches(r.Platform); err != nil {
		return Device{}, err
	}
	k, err := parseRegistrationKeys(r)
	if err != nil {
		return Device{}, err
	}
	key, canonical, jkt, agreement, kaKey, kaJKT := k.key, k.canonical, k.jkt, k.agreement, k.kaKey, k.kaJKT
	found, err := s.findEnvironment(ctx, r.AppID, r.Environment)
	if err != nil {
		return Device{}, err
	}
	appID, envID := canonicalID(r.AppID), storage.ID(found.EnvironmentID)
	conf, err := s.settingsFor(ctx, appID, envID)
	if err != nil {
		return Device{}, err
	}
	if err := s.consumeChallenge(ctx, r.Challenge, appID, envID, conf.RegistrationChallengeTTL); err != nil {
		return Device{}, err
	}
	sub := subject{
		appID: appID, envID: envID, production: found.Production, challenge: r.Challenge,
		key: key, jkt: jkt, kaJKT: kaJKT, scaJKT: k.scaJKT, claim: r.KeyStorage, settings: conf,
	}
	v, err := s.verify(ctx, sub, r.Evidence)
	if err != nil {
		return Device{}, err
	}
	if err := s.verifyAgreementKey(ctx, sub, v.proof, kaKey, r); err != nil {
		return Device{}, err
	}
	sca, err := s.verifyRegistrationSCA(ctx, appID, found.Production, r)
	if err != nil {
		return Device{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Device{}, err
	}
	params := dbgen.InsertAttestedDeviceParams{
		ID: storage.MustUUID(id), OrganizationID: found.OrganizationID, AppID: storage.MustUUID(appID),
		EnvironmentID: found.EnvironmentID, Platform: r.Platform, OsVersion: r.OSVersion,
		RuntimeVersion: r.RuntimeVersion, HostBuild: r.Build, AssuranceLevel: string(Assess(v.proof, conf)),
		DpopJkt: &jkt, DpopPublicKey: canonical, KeyStorage: string(v.proof.KeyStorage),
		AttestationVerdicts: v.verdicts, AttestationRiskMetric: v.risk, AttestedAt: storage.Timestamp(s.now()),
		AgreementPublicKey: agreement,
	}
	provider := string(v.proof.Provider)
	params.AttestationProvider = &provider
	if a := v.appAttest; a != nil {
		params.AppAttestKeyID, params.AppAttestPublicKey, params.AppAttestReceipt, params.AppAttestCounter = a.keyID, a.publicKey, a.receipt, a.counter
	}
	var d Device
	err = s.db.InTx(ctx, storage.Tenant{OrganizationID: storage.ID(found.OrganizationID)}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).InsertAttestedDevice(ctx, params)
		if err != nil {
			return fmt.Errorf("device: register: %w", err)
		}
		d = deviceOf(row)
		if sca == nil {
			return nil
		}
		return putSCAKey(ctx, tx, row.ID, row.OrganizationID, sca.der, sca.key)
	})
	if isUniqueViolation(err) {
		return Device{}, plxerr.New(plxerr.AttestationFailed, "a device with this key is already registered")
	}
	if err != nil {
		return Device{}, fmt.Errorf("device: %w", err)
	}
	return d, nil
}

// registrationKeys are the keys a registration submits, parsed.
type registrationKeys struct {
	key       *ecdsa.PublicKey
	canonical []byte
	jkt       string
	agreement []byte
	kaKey     *ecdsa.PublicKey
	kaJKT     string
	scaJKT    string
}

// parseRegistrationKeys parses the DPoP, key-agreement and SCA keys of a
// registration.
func parseRegistrationKeys(r AttestedRegistration) (registrationKeys, error) {
	var k registrationKeys
	var err error
	if k.key, k.canonical, k.jkt, err = parseKey(r.DPoPKeyJWK); err != nil {
		return k, err
	}
	if k.agreement, k.kaKey, k.kaJKT, err = parseAgreementKey(r.AgreementKeyJWK); err != nil {
		return k, err
	}
	k.scaJKT, err = registrationSCAThumbprint(r)
	return k, err
}

// registeredSCA is a verified SCA key on its way into the database.
type registeredSCA struct {
	key SCAKey
	der []byte
}

// verifyRegistrationSCA verifies the SCA key a registration brings, when
// it brings one (SEC-027). A key without evidence, and evidence without a
// key, are refused.
func (s *Service) verifyRegistrationSCA(ctx context.Context, appID string, production bool, r AttestedRegistration) (*registeredSCA, error) {
	if len(r.SCAKeyJWK) == 0 && r.SCAEvidence == nil {
		return nil, nil //nolint:nilnil // the device has no SCA key yet
	}
	if len(r.SCAKeyJWK) == 0 || r.SCAEvidence == nil {
		return nil, plxerr.New(plxerr.AttestationFailed, "an SCA key and its evidence come together")
	}
	if err := r.SCAEvidence.matches(r.Platform); err != nil {
		return nil, err
	}
	key, err := s.registrationSCAKey(ctx, appID, production, r)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKIXPublicKey(key.Public)
	if err != nil {
		return nil, fmt.Errorf("device: encode the SCA key: %w", err)
	}
	return &registeredSCA{key: key, der: der}, nil
}

// registrationSCAKey verifies the SCA key of a registration. An iOS device
// reuses the registration's own App Attest evidence for the key, which
// cannot be attested apart from the DPoP key: the request that carries
// the key was made by the attested app, and the key is stored with the
// Secure Enclave as its storage. Any other evidence is verified for the
// SCA key itself (SEC-027).
func (s *Service) registrationSCAKey(ctx context.Context, appID string, production bool, r AttestedRegistration) (SCAKey, error) {
	if ios, reg := r.SCAEvidence.IOS, r.Evidence.IOS; ios != nil && reg != nil &&
		bytes.Equal(ios.KeyID, reg.KeyID) && bytes.Equal(ios.AttestationObject, reg.AttestationObject) {
		key, _, jkt, err := parseKey(r.SCAKeyJWK)
		if err != nil {
			return SCAKey{}, plxerr.New(plxerr.InvalidFormat, "the SCA key is not an EC P-256 public key in JWK form")
		}
		return SCAKey{Public: key, JKT: jkt, Storage: KeyStorageSecureEnclave, Provider: scaProviderAppAttest}, nil
	}
	return s.verifySCA(ctx, appID, production, SCAEnrolment{Challenge: r.Challenge, KeyJWK: r.SCAKeyJWK, Evidence: *r.SCAEvidence})
}

// checkDescription validates what a device says about itself.
func checkDescription(platform string, fields ...string) error {
	if !platforms[platform] {
		return plxerr.New(plxerr.InvalidFormat, "platform %q is not one of android, ios, web, macos, windows, linux", platform)
	}
	for _, f := range fields {
		if len(f) > 64 {
			return plxerr.New(plxerr.InvalidFormat, "a version or build field is longer than 64 characters")
		}
	}
	return nil
}

// matches checks that exactly one provider form is set and that it
// fits the platform: Android evidence comes from an Android device.
func (e Evidence) matches(platform string) error {
	set := 0
	for _, present := range []bool{e.Android != nil, e.IOS != nil, e.Development != nil} {
		if present {
			set++
		}
	}
	if set != 1 {
		return plxerr.New(plxerr.AttestationFailed, "the evidence must name exactly one provider")
	}
	if (e.Android != nil && platform != "android") || (e.IOS != nil && platform != "ios") {
		return plxerr.New(plxerr.AttestationFailed, "the evidence is not from the %s platform", platform)
	}
	return nil
}

// parseKey reads a DPoP public key from a JWK. It returns the key, the
// JWK in the form the server keeps (the key's members and no others) and
// the RFC 7638 thumbprint.
func parseKey(raw []byte) (*ecdsa.PublicKey, []byte, string, error) {
	pub, canonical, err := parseP256(raw, "DPoP key")
	if err != nil {
		return nil, nil, "", err
	}
	jkt, err := dpop.Thumbprint(pub)
	if err != nil {
		return nil, nil, "", err //nolint:wrapcheck // a domain error
	}
	return pub, canonical, jkt, nil
}

// parseP256 reads an EC P-256 public key from a JWK; what names the key
// in messages. It returns the key and the JWK in the form the server
// keeps (the key's members and no others).
func parseP256(raw []byte, what string) (*ecdsa.PublicKey, []byte, error) {
	var jwk jose.JSONWebKey
	if err := json.Unmarshal(raw, &jwk); err != nil || !jwk.Valid() || !jwk.IsPublic() {
		return nil, nil, plxerr.New(plxerr.InvalidFormat, "the %s is not a public key in JWK form", what)
	}
	pub, ok := jwk.Key.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, nil, plxerr.New(plxerr.InvalidFormat, "the %s is not an EC P-256 key", what)
	}
	canonical, err := json.Marshal(jose.JSONWebKey{Key: pub})
	if err != nil {
		return nil, nil, fmt.Errorf("device: encode the %s: %w", what, err)
	}
	return pub, canonical, nil
}

// parseAgreementKey reads the key-agreement key a registration offers
// (SEC-053): the JWK in the form the server keeps, the key and its
// thumbprint; all empty for a device that has none.
func parseAgreementKey(raw []byte) ([]byte, *ecdsa.PublicKey, string, error) {
	if len(raw) == 0 {
		return nil, nil, "", nil
	}
	pub, canonical, err := parseP256(raw, "key-agreement key")
	if err != nil {
		return nil, nil, "", err
	}
	jkt, err := dpop.Thumbprint(pub)
	if err != nil {
		return nil, nil, "", err //nolint:wrapcheck // a domain error
	}
	return canonical, pub, jkt, nil
}

// registrationSCAThumbprint is the thumbprint of the SCA key a
// registration brings, empty when it brings none.
func registrationSCAThumbprint(r AttestedRegistration) (string, error) {
	if len(r.SCAKeyJWK) == 0 {
		return "", nil
	}
	_, _, jkt, err := parseKey(r.SCAKeyJWK)
	if err != nil {
		return "", plxerr.New(plxerr.InvalidFormat, "the SCA key is not an EC P-256 public key in JWK form")
	}
	return jkt, nil
}

// verifyAgreementKey checks that the key-agreement key of a registration
// is as well protected as the DPoP key. The binding already covers the
// key's thumbprint; on Android the key's own Key Attestation chain, made
// with the registration challenge, must also prove that it is the key the
// chain attests, and that it lives in hardware wherever the DPoP key does,
// so a rooted device cannot register a software key beside an attested
// one (SEC-004, SEC-053).
func (s *Service) verifyAgreementKey(ctx context.Context, sub subject, proof Proof, key *ecdsa.PublicKey, r AttestedRegistration) error {
	chain := r.AgreementKeyChain
	if key == nil {
		if len(chain) > 0 {
			return plxerr.New(plxerr.AttestationFailed, "a key attestation chain came without a key-agreement key")
		}
		return nil
	}
	if r.Evidence.Android == nil {
		if len(chain) > 0 {
			return plxerr.New(plxerr.AttestationFailed, "the evidence is not from the android platform")
		}
		return nil
	}
	if len(chain) == 0 {
		return plxerr.New(plxerr.AttestationFailed, "the key-agreement key needs a key attestation chain")
	}
	trust, err := s.trustOf(ctx, sub.appID)
	if err != nil {
		return err
	}
	if s.attestors.KeyAttestation == nil || len(trust.AndroidPackages) == 0 {
		return plxerr.New(plxerr.AttestationUnavailable, "Android attestation is not configured for this app")
	}
	res, err := s.attestors.KeyAttestation.Verify(chain, sub.challenge, keyattest.Policy{
		PackageNames: trust.AndroidPackages, SigningCertDigests: trust.AndroidCertDigests,
		RequireHardware: proof.KeyStorage.hardware(),
	})
	if err != nil {
		return err //nolint:wrapcheck // a domain error
	}
	if res.PublicKey == nil || !res.PublicKey.Equal(key) {
		return plxerr.New(plxerr.AttestationFailed, "the attested key is not the key-agreement key")
	}
	return nil
}

// settingsFor resolves the security settings of an environment.
func (s *Service) settingsFor(ctx context.Context, appID, envID string) (Settings, error) {
	if s.config != nil {
		v, err := s.config(ctx, appID, envID)
		if err != nil {
			return Settings{}, fmt.Errorf("device: the security configuration: %w", err)
		}
		conf, err := SettingsOf(v)
		if err != nil {
			return Settings{}, err
		}
		return s.withInstallationLifetime(conf), nil
	}
	profile := settings.Standard
	if s.profiles != nil {
		p, err := s.profiles(ctx, appID, envID)
		if err != nil {
			return Settings{}, fmt.Errorf("device: the security profile: %w", err)
		}
		profile = p
	}
	conf, err := SettingsFor(profile)
	if err != nil {
		return Settings{}, err
	}
	return s.withInstallationLifetime(conf), nil
}

// withInstallationLifetime applies the installation's access token
// lifetime (auth.device.accessTokenTTL). Like every override it may only
// tighten, so a stricter profile or environment keeps its shorter value.
func (s *Service) withInstallationLifetime(conf Settings) Settings {
	if s.accessTokenLifetime > 0 && s.accessTokenLifetime < conf.AccessTokenLifetime {
		conf.AccessTokenLifetime = s.accessTokenLifetime
	}
	return conf
}

// trustOf returns the trust configuration of an app.
func (s *Service) trustOf(ctx context.Context, appID string) (TrustConfig, error) {
	if s.appTrust == nil {
		return TrustConfig{}, plxerr.New(plxerr.AttestationUnavailable, "attestation is not configured for this app")
	}
	t, err := s.appTrust(ctx, appID)
	if err != nil {
		return TrustConfig{}, plxerr.Wrap(plxerr.AttestationUnavailable, err, "the attestation configuration of the app is unavailable")
	}
	return t, nil
}

// verify checks the evidence of whichever provider it names, then checks
// the key storage the device claimed against what was proven.
func (s *Service) verify(ctx context.Context, sub subject, e Evidence) (verified, error) {
	var (
		v   verified
		err error
	)
	if e.Development != nil {
		v, err = s.verifyDevelopment(sub, *e.Development)
	} else {
		var trust TrustConfig
		if trust, err = s.trustOf(ctx, sub.appID); err != nil {
			return verified{}, err
		}
		if e.Android != nil {
			v, err = s.verifyAndroid(sub, trust, *e.Android)
		} else {
			v, err = s.verifyIOS(sub, trust, *e.IOS)
		}
	}
	if err != nil {
		return verified{}, err
	}
	if err := sub.checkStorage(v.proof.KeyStorage); err != nil {
		return verified{}, err
	}
	return v, nil
}

// checkStorage refuses a claim the proof does not support, and a
// software key where the environment demands hardware (SEC-001).
func (sub subject) checkStorage(proven KeyStorage) error {
	if !proven.covers(sub.claim) {
		return plxerr.New(plxerr.AttestationFailed, "the key storage the device claims is not what the attestation proves")
	}
	if sub.settings.requireHardware() && !proven.hardware() {
		return plxerr.New(plxerr.KeyNotHardwareBacked, "this environment accepts only hardware-backed keys")
	}
	return nil
}

// covers reports whether a proven storage supports a claimed one. A
// claim of software, or none, is always supported.
func (k KeyStorage) covers(claim KeyStorage) bool {
	switch claim {
	case KeyStorageUnspecified, KeyStorageSoftware:
		return true
	case KeyStorageTEE:
		return k == KeyStorageTEE || k == KeyStorageStrongBox
	default:
		return k == claim
	}
}

// verifyAndroid checks a Key Attestation chain and a Play Integrity
// token. The attested key must be the DPoP key itself, or the chain
// would prove another key (SEC-003).
func (s *Service) verifyAndroid(sub subject, trust TrustConfig, e AndroidEvidence) (verified, error) {
	if s.attestors.KeyAttestation == nil || s.attestors.PlayIntegrity == nil ||
		len(trust.AndroidPackages) == 0 || trust.PlayIntegrity == nil {
		return verified{}, plxerr.New(plxerr.AttestationUnavailable, "Android attestation is not configured for this app")
	}
	if len(e.KeyAttestationChain) == 0 || e.PlayIntegrityToken == "" {
		return verified{}, plxerr.New(plxerr.AttestationFailed, "Android evidence needs a key attestation chain and a Play Integrity token")
	}
	res, err := s.attestors.KeyAttestation.Verify(e.KeyAttestationChain, sub.challenge, keyattest.Policy{
		PackageNames: trust.AndroidPackages, SigningCertDigests: trust.AndroidCertDigests,
		RequireHardware: sub.settings.requireHardware(),
	})
	if err != nil {
		return verified{}, err //nolint:wrapcheck // a domain error
	}
	if res.PublicKey == nil || !res.PublicKey.Equal(sub.key) {
		return verified{}, plxerr.New(plxerr.AttestationFailed, "the attested key is not the DPoP key")
	}
	verdict, err := s.attestors.PlayIntegrity.Verify(*trust.PlayIntegrity, e.PlayIntegrityToken, playintegrity.Expect{
		PackageName: res.PackageName, RequestHash: binding.RequestHash(sub.challenge, sub.jkt, sub.kaJKT, sub.scaJKT),
		CertDigests: trust.AndroidCertDigests, MaxAge: integrityMaxAge,
	})
	if err != nil {
		return verified{}, err //nolint:wrapcheck // a domain error
	}
	labels := make([]string, 0, len(verdict.Device))
	for _, l := range verdict.Device {
		labels = append(labels, string(l))
	}
	return verified{
		proof: Proof{
			Provider: ProviderPlayIntegrity, KeyStorage: androidStorage(res),
			AppRecognised: verdict.AppRecognised, DeviceVerdict: verdict.Strongest(),
		},
		verdicts: labels,
	}, nil
}

// androidStorage says where a verified Android key lives.
func androidStorage(r keyattest.Result) KeyStorage {
	switch {
	case !r.HardwareBacked():
		return KeyStorageSoftware
	case r.KeyMintLevel == keyattest.SecurityStrongBox:
		return KeyStorageStrongBox
	default:
		return KeyStorageTEE
	}
}

// verifyIOS checks an App Attest attestation bound to the challenge and
// the thumbprints of the device's keys. App Attest does not say where the DPoP key
// lives, so the device's claim stands: a Secure Enclave claim earns AL2,
// anything else AL1 (SEC-007).
func (s *Service) verifyIOS(sub subject, trust TrustConfig, e IOSEvidence) (verified, error) {
	if s.attestors.AppAttest == nil || trust.IOSAppID == "" {
		return verified{}, plxerr.New(plxerr.AttestationUnavailable, "iOS attestation is not configured for this app")
	}
	if len(e.KeyID) == 0 || len(e.AttestationObject) == 0 {
		return verified{}, plxerr.New(plxerr.AttestationFailed, "iOS evidence needs an App Attest key identifier and attestation object")
	}
	env := appattest.Development
	if trust.AppAttestProduction {
		env = appattest.Production
	}
	att, err := s.attestors.AppAttest.VerifyAttestation(e.AttestationObject, e.KeyID, sub.bindingHash(), trust.IOSAppID, env)
	if err != nil {
		return verified{}, err //nolint:wrapcheck // a domain error
	}
	der, err := x509.MarshalPKIXPublicKey(att.PublicKey)
	if err != nil {
		return verified{}, fmt.Errorf("device: encode the App Attest key: %w", err)
	}
	storageOf := KeyStorageSoftware
	if sub.claim == KeyStorageSecureEnclave {
		storageOf = KeyStorageSecureEnclave
	}
	return verified{
		proof:     Proof{Provider: ProviderAppAttest, KeyStorage: storageOf},
		verdicts:  []string{},
		appAttest: &appAttestRecord{keyID: e.KeyID, publicKey: der, receipt: att.Receipt, counter: int64(att.Counter)},
	}, nil
}

// verifyDevelopment accepts a development build, in an environment that
// is not production only and only where the installation enables the
// development provider (SEC-008). It proves nothing, so the device stays
// at AL0 with a software key.
func (s *Service) verifyDevelopment(sub subject, e DevelopmentEvidence) (verified, error) {
	if sub.production {
		return verified{}, plxerr.New(plxerr.DevProviderInProduction, "the development provider is not accepted in a production environment")
	}
	if !s.developmentProvider {
		return verified{}, plxerr.New(plxerr.AttestationFailed, "the development provider is not enabled")
	}
	if e.BuildID == "" || len(e.BuildID) > maxBuildID {
		return verified{}, plxerr.New(plxerr.InvalidFormat, "the development build identifier is empty or longer than %d characters", maxBuildID)
	}
	return verified{
		proof:    Proof{Provider: ProviderDevelopment, KeyStorage: KeyStorageSoftware},
		verdicts: []string{},
	}, nil
}

// isUniqueViolation reports whether a database error is a unique-index
// violation.
func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

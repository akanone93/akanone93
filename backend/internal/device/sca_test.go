// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"testing"

	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest"
	"github.com/nightCode42/plux3/backend/internal/attest/binding"
	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// scaKeyAttestor returns the key a chain "attests" and records the policy
// it was asked to enforce.
type scaKeyAttestor struct {
	key    *ecdsa.PublicKey
	policy keyattest.Policy
	level  keyattest.SecurityLevel
	err    error
}

func (a *scaKeyAttestor) Verify(_ [][]byte, _ []byte, p keyattest.Policy) (keyattest.Result, error) {
	a.policy = p
	if a.err != nil {
		return keyattest.Result{}, a.err
	}
	return keyattest.Result{AttestationLevel: a.level, KeyMintLevel: a.level, PublicKey: a.key, UserAuthRequired: true}, nil
}

// scaAppAttestor accepts an attestation and records the client data hash.
type scaAppAttestor struct{ hash [32]byte }

func (a *scaAppAttestor) VerifyAttestation(_, _ []byte, h [32]byte, _ string, _ appattest.Environment) (appattest.Attestation, error) {
	a.hash = h
	return appattest.Attestation{}, nil
}

func jwkOf(t *testing.T, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	raw, err := json.Marshal(jose.JSONWebKey{Key: pub})
	if err != nil {
		t.Fatalf("encode the key: %v", err)
	}
	return raw
}

func newSCAKey(t *testing.T) *ecdsa.PublicKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}
	return &k.PublicKey
}

// Verifies: SEC-027.
// An Android SCA key must be the key the chain attests, hardware-backed
// and demanding user authentication; the verifier is asked for all three.
func TestVerifySCAAndroid(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	trust := TrustConfig{AndroidPackages: []string{"com.example.app"}}
	key := newSCAKey(t)
	evidence := Evidence{Android: &AndroidEvidence{KeyAttestationChain: [][]byte{{1}}}}

	attestor := &scaKeyAttestor{key: key, level: keyattest.SecurityStrongBox}
	s := &Service{attestors: Attestors{KeyAttestation: attestor}, appTrust: func(context.Context, string) (TrustConfig, error) { return trust, nil }}
	got, err := s.verifySCA(ctx, "app", false, SCAEnrolment{Challenge: []byte("c"), KeyJWK: jwkOf(t, key), Evidence: evidence})
	if err != nil {
		t.Fatalf("a valid SCA key: %v", err)
	}
	if got.Storage != KeyStorageStrongBox || got.Provider != scaProviderKeyAttestation || got.JKT == "" {
		t.Errorf("verified key = %+v", got)
	}
	if !attestor.policy.RequireHardware || !attestor.policy.RequireUserAuth {
		t.Errorf("policy = %+v; the key must be hardware-backed and demand user authentication", attestor.policy)
	}

	other := newSCAKey(t)
	attestor.key = other
	if _, err := s.verifySCA(ctx, "app", false, SCAEnrolment{KeyJWK: jwkOf(t, key), Evidence: evidence}); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("a chain for another key: %v", err)
	}
	attestor.key, attestor.err = key, plxerr.New(plxerr.AttestationFailed, "key does not require user authentication")
	if _, err := s.verifySCA(ctx, "app", false, SCAEnrolment{KeyJWK: jwkOf(t, key), Evidence: evidence}); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("a key without user authentication: %v", err)
	}
	if _, err := s.verifySCA(ctx, "app", false, SCAEnrolment{KeyJWK: []byte(`{"kty":"oct"}`), Evidence: evidence}); errCode(err) != plxerr.InvalidFormat {
		t.Errorf("a key that is not P-256: %v", err)
	}
	empty := Evidence{Android: &AndroidEvidence{}}
	attestor.err = nil
	if _, err := s.verifySCA(ctx, "app", false, SCAEnrolment{KeyJWK: jwkOf(t, key), Evidence: empty}); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("no chain: %v", err)
	}
}

// Verifies: SEC-027, SEC-004.
// An iOS SCA key enrolled after registration is vouched for by an
// assertion with the registration's App Attest key over the binding hash of
// the challenge and all three thumbprints, so an assertion made for another
// set of keys does not carry over.
func TestVerifySCAAssertion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	trust := TrustConfig{IOSAppID: "TEAM.com.example.app"}
	assertor := &devicetest.AppAssertor{Counter: 5}
	s := &Service{attestors: Attestors{AppAssertions: assertor}, appTrust: func(context.Context, string) (TrustConfig, error) { return trust, nil }}

	appAttestKey := newSCAKey(t)
	der, err := x509.MarshalPKIXPublicKey(appAttestKey)
	if err != nil {
		t.Fatal(err)
	}
	_, agreementRaw := agreementJWK(t)
	agreement, _, kaJKT, err := parseAgreementKey(agreementRaw)
	if err != nil {
		t.Fatal(err)
	}
	dpopJKT := "dpop-jkt"
	row := dbgen.Device{DpopJkt: &dpopJKT, AppAttestPublicKey: der, AppAttestCounter: 2, AgreementPublicKey: agreement}
	sca := newSCAKey(t)
	_, _, scaJKT, err := parseKey(jwkOf(t, sca))
	if err != nil {
		t.Fatal(err)
	}
	challenge := []byte("challenge")
	enrolment := SCAEnrolment{Challenge: challenge, KeyJWK: jwkOf(t, sca), Evidence: Evidence{IOS: &IOSEvidence{Assertion: []byte("assertion")}}}

	key, counter, err := s.verifyEnrolment(ctx, row, false, enrolment)
	if err != nil || counter == nil || *counter != 5 {
		t.Fatalf("a valid enrolment: %+v %v %v", key, counter, err)
	}
	if key.Storage != KeyStorageSecureEnclave || key.Provider != scaProviderAppAttest || key.JKT != scaJKT {
		t.Errorf("verified key = %+v", key)
	}
	if want := binding.Hash(challenge, dpopJKT, kaJKT, scaJKT); assertor.ClientDataHash != want || assertor.LastCounter != 2 {
		t.Errorf("the assertion was checked against %x after %d, want %x", assertor.ClientDataHash, assertor.LastCounter, want)
	}

	// A device without a key-agreement key is bound with the empty string.
	row.AgreementPublicKey = nil
	if _, _, err := s.verifyEnrolment(ctx, row, false, enrolment); err != nil {
		t.Fatalf("no agreement key: %v", err)
	}
	if want := binding.Hash(challenge, dpopJKT, "", scaJKT); assertor.ClientDataHash != want {
		t.Error("an absent key-agreement key is not bound as the empty string")
	}
	row.AgreementPublicKey = agreement

	assertor.Err = plxerr.New(plxerr.AttestationFailed, "bad assertion")
	if _, _, err := s.verifyEnrolment(ctx, row, false, enrolment); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("a refused assertion: %v", err)
	}
	assertor.Err = nil
	none := SCAEnrolment{Challenge: challenge, KeyJWK: jwkOf(t, sca), Evidence: Evidence{IOS: &IOSEvidence{KeyID: []byte("id"), AttestationObject: []byte("object")}}}
	if _, _, err := s.verifyEnrolment(ctx, row, false, none); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("an attestation instead of an assertion: %v", err)
	}
	noKey := row
	noKey.AppAttestPublicKey = nil
	if _, _, err := s.verifyEnrolment(ctx, noKey, false, enrolment); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("a device without an App Attest key: %v", err)
	}
}

// Verifies: SEC-027, SEC-008.
// The development provider vouches for an SCA key only outside production
// and only where the installation enables it.
func TestVerifySCADevelopment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	key := newSCAKey(t)
	evidence := Evidence{Development: &DevelopmentEvidence{BuildID: "dev"}}
	enrolment := SCAEnrolment{KeyJWK: jwkOf(t, key), Evidence: evidence}

	on := &Service{developmentProvider: true}
	if got, err := on.verifySCA(ctx, "app", false, enrolment); err != nil || got.Storage != KeyStorageSoftware {
		t.Errorf("development, enabled: %+v %v", got, err)
	}
	if _, err := on.verifySCA(ctx, "app", true, enrolment); errCode(err) != plxerr.DevProviderInProduction {
		t.Errorf("development in production: %v", err)
	}
	if _, err := (&Service{}).verifySCA(ctx, "app", false, enrolment); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("development, disabled: %v", err)
	}
}

// Verifies: SEC-027.
// A registration brings an SCA key and its evidence together or not at
// all.
func TestRegistrationSCAComesWithEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := &Service{developmentProvider: true}
	if got, err := s.verifyRegistrationSCA(ctx, "app", false, AttestedRegistration{Platform: "android"}); got != nil || err != nil {
		t.Errorf("no SCA key: %v %v", got, err)
	}
	key := jwkOf(t, newSCAKey(t))
	if _, err := s.verifyRegistrationSCA(ctx, "app", false, AttestedRegistration{Platform: "android", SCAKeyJWK: key}); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("a key without evidence: %v", err)
	}
	evidence := &Evidence{Development: &DevelopmentEvidence{BuildID: "dev"}}
	if _, err := s.verifyRegistrationSCA(ctx, "app", false, AttestedRegistration{Platform: "android", SCAEvidence: evidence}); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("evidence without a key: %v", err)
	}
	got, err := s.verifyRegistrationSCA(ctx, "app", false, AttestedRegistration{Platform: "android", SCAKeyJWK: key, SCAEvidence: evidence})
	if err != nil || got == nil || len(got.der) == 0 {
		t.Errorf("a development SCA key: %+v %v", got, err)
	}
}

// Verifies: SEC-027.
// An iOS registration may reuse its own App Attest evidence for the SCA
// key, which the binding hash covers; other iOS evidence is refused.
func TestRegistrationSCAReusesIOSEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	attestor := &scaAppAttestor{}
	trust := TrustConfig{IOSAppID: "TEAM.com.example.app"}
	s := &Service{attestors: Attestors{AppAttest: attestor}, appTrust: func(context.Context, string) (TrustConfig, error) { return trust, nil }}
	key := jwkOf(t, newSCAKey(t))
	same := Evidence{IOS: &IOSEvidence{KeyID: []byte("id"), AttestationObject: []byte("object")}}
	got, err := s.verifyRegistrationSCA(ctx, "app", false, AttestedRegistration{Platform: "ios", Evidence: same, SCAKeyJWK: key, SCAEvidence: &same})
	if err != nil || got == nil || got.key.Storage != KeyStorageSecureEnclave {
		t.Fatalf("reused evidence: %+v %v", got, err)
	}
	if attestor.hash != ([32]byte{}) {
		t.Error("reused evidence was verified again")
	}
	other := Evidence{IOS: &IOSEvidence{KeyID: []byte("id2"), AttestationObject: []byte("object2")}}
	if got, err := s.verifyRegistrationSCA(ctx, "app", false, AttestedRegistration{Platform: "ios", Evidence: same, SCAKeyJWK: key, SCAEvidence: &other}); errCode(err) != plxerr.AttestationFailed || got != nil {
		t.Errorf("other iOS evidence must be refused: %+v %v", got, err)
	}
}

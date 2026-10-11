// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/go-jose/go-jose/v4"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest"
	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
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

// Verifies: SEC-027.
// An iOS SCA key is vouched for by an App Attest attestation whose client
// data hash binds the key's thumbprint, so an attestation made for
// another key does not carry over.
func TestVerifySCAIOS(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	trust := TrustConfig{IOSAppID: "TEAM.com.example.app"}
	key := newSCAKey(t)
	attestor := &scaAppAttestor{}
	s := &Service{attestors: Attestors{AppAttest: attestor}, appTrust: func(context.Context, string) (TrustConfig, error) { return trust, nil }}
	challenge := []byte("challenge")
	evidence := Evidence{IOS: &IOSEvidence{KeyID: []byte("id"), AttestationObject: []byte("object")}}
	got, err := s.verifySCA(ctx, "app", false, SCAEnrolment{Challenge: challenge, KeyJWK: jwkOf(t, key), Evidence: evidence})
	if err != nil {
		t.Fatalf("a valid SCA key: %v", err)
	}
	if got.Storage != KeyStorageSecureEnclave || got.Provider != scaProviderAppAttest {
		t.Errorf("verified key = %+v", got)
	}
	if want := appattest.ClientDataHash(challenge, got.JKT); attestor.hash != want {
		t.Errorf("the client data hash does not bind the challenge and the SCA key's thumbprint")
	}
	noID := Evidence{IOS: &IOSEvidence{AttestationObject: []byte("object")}}
	if _, err := s.verifySCA(ctx, "app", false, SCAEnrolment{Challenge: challenge, KeyJWK: jwkOf(t, key), Evidence: noID}); errCode(err) != plxerr.AttestationFailed {
		t.Errorf("no key identifier: %v", err)
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

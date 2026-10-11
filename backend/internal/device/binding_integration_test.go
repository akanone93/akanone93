// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest"
	"github.com/nightCode42/plux3/backend/internal/attest/binding"
	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/attest/playintegrity"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// androidKeys is a registration of an Android device with all three keys.
type androidKeys struct {
	dpopJWK, kaJWK, scaJWK []byte
	dpopJKT, kaJKT, scaJKT string
}

// newAndroidKeys makes the fakes vouch for three hardware keys: the DPoP
// key through the registration evidence, the others through chains led by
// agreementLeaf and scaLeaf.
func newAndroidKeys(t *testing.T, r *rig) androidKeys {
	t.Helper()
	dpopKey, dpopJWK := devicetest.NewKey(t)
	kaKey, kaJWK := devicetest.NewKey(t)
	scaKey, scaJWK := devicetest.NewKey(t)
	r.android(&dpopKey.PublicKey, keyattest.SecurityTrustedEnvironment, playintegrity.LabelDevice)
	r.androidKey(agreementLeaf, &kaKey.PublicKey, keyattest.SecurityTrustedEnvironment)
	r.androidKey(scaLeaf, &scaKey.PublicKey, keyattest.SecurityStrongBox)
	return androidKeys{
		dpopJWK: dpopJWK, kaJWK: kaJWK, scaJWK: scaJWK,
		dpopJKT: thumbprint(t, &dpopKey.PublicKey), kaJKT: thumbprint(t, &kaKey.PublicKey), scaJKT: thumbprint(t, &scaKey.PublicKey),
	}
}

func (k androidKeys) registration(r *rig, challenge []byte) device.AttestedRegistration {
	scaEvidence := device.Evidence{Android: &device.AndroidEvidence{KeyAttestationChain: [][]byte{scaLeaf, {2}}}}
	return device.AttestedRegistration{
		AppID: r.app, Environment: "production", Platform: "android", OSVersion: "15", RuntimeVersion: "1.0.0", Build: "42",
		Challenge: challenge, DPoPKeyJWK: k.dpopJWK, KeyStorage: device.KeyStorageTEE, Evidence: androidEvidence(),
		AgreementKeyJWK: k.kaJWK, AgreementKeyChain: [][]byte{agreementLeaf, {2}},
		SCAKeyJWK: k.scaJWK, SCAEvidence: &scaEvidence,
	}
}

// Verifies: SEC-003, SEC-005, SEC-027, SEC-053.
// The Play Integrity request hash of an Android registration covers the
// challenge and the thumbprints of the DPoP, key-agreement and SCA keys:
// a verdict made for any other set of keys has another hash.
func TestRegisterAndroidBindsEveryKey(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	k := newAndroidKeys(t, r)
	challenge := r.challenge(t, "production")
	if _, err := r.svc.RegisterAttested(context.Background(), k.registration(r, challenge)); err != nil {
		t.Fatalf("RegisterAttested: %v", err)
	}
	got := r.play.Expect.RequestHash
	if want := binding.RequestHash(challenge, k.dpopJKT, k.kaJKT, k.scaJKT); got != want {
		t.Errorf("request hash = %q, want the binding of all three keys %q", got, want)
	}
	for name, other := range map[string]string{
		"DPoP key":          binding.RequestHash(challenge, "other", k.kaJKT, k.scaJKT),
		"key-agreement key": binding.RequestHash(challenge, k.dpopJKT, "other", k.scaJKT),
		"SCA key":           binding.RequestHash(challenge, k.dpopJKT, k.kaJKT, "other"),
		"no SCA key":        binding.RequestHash(challenge, k.dpopJKT, k.kaJKT, ""),
	} {
		if got == other {
			t.Errorf("a different %s has the same request hash", name)
		}
	}
}

// Verifies: SEC-001, SEC-053, SEC-027.
// On Android the key-agreement and SCA keys are attested like the DPoP
// key: their chains must be present, attest the submitted keys and, where
// the DPoP key is in hardware, put them in hardware too, so a rooted device
// cannot add a software key to an attested registration.
func TestRegisterAndroidKeyChains(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig(t)

	register := func(t *testing.T, mutate func(*device.AttestedRegistration, *androidKeys)) error {
		t.Helper()
		k := newAndroidKeys(t, r)
		reg := k.registration(r, r.challenge(t, "production"))
		if mutate != nil {
			mutate(&reg, &k)
		}
		_, err := r.svc.RegisterAttested(ctx, reg)
		return err
	}
	if err := register(t, nil); err != nil {
		t.Fatalf("all three keys in hardware: %v", err)
	}
	// The key attestor was asked for the SCA key last, with the registration
	// challenge, hardware and user authentication.
	if !r.key.Policy.RequireHardware || !r.key.Policy.RequireUserAuth {
		t.Errorf("SCA policy = %+v", r.key.Policy)
	}

	softKA := func(reg *device.AttestedRegistration, k *androidKeys) {
		kaKey, kaJWK := devicetest.NewKey(t)
		reg.AgreementKeyJWK = kaJWK
		r.androidKey(agreementLeaf, &kaKey.PublicKey, keyattest.SecuritySoftware)
	}
	if err := register(t, softKA); code(err) != plxerr.KeyNotHardwareBacked {
		t.Errorf("a software key-agreement key beside a hardware DPoP key: %v", err)
	}
	if err := register(t, func(reg *device.AttestedRegistration, k *androidKeys) {
		scaKey, scaJWK := devicetest.NewKey(t)
		reg.SCAKeyJWK = scaJWK
		r.androidKey(scaLeaf, &scaKey.PublicKey, keyattest.SecuritySoftware)
	}); code(err) != plxerr.KeyNotHardwareBacked {
		t.Errorf("a software SCA key beside a hardware DPoP key: %v", err)
	}
	if err := register(t, func(reg *device.AttestedRegistration, k *androidKeys) {
		scaKey, scaJWK := devicetest.NewKey(t)
		reg.SCAKeyJWK = scaJWK
		r.key.Results[string(scaLeaf)] = keyattest.Result{
			AttestationLevel: keyattest.SecurityStrongBox, KeyMintLevel: keyattest.SecurityStrongBox, PublicKey: &scaKey.PublicKey,
		}
	}); code(err) != plxerr.AttestationFailed {
		t.Errorf("an SCA key that does not require user authentication: %v", err)
	}
	if err := register(t, func(reg *device.AttestedRegistration, _ *androidKeys) { reg.AgreementKeyChain = nil }); code(err) != plxerr.AttestationFailed {
		t.Errorf("a key-agreement key without a chain: %v", err)
	}
	if err := register(t, func(reg *device.AttestedRegistration, _ *androidKeys) { reg.AgreementKeyJWK = nil }); code(err) != plxerr.AttestationFailed {
		t.Errorf("a chain without a key-agreement key: %v", err)
	}
	if err := register(t, func(reg *device.AttestedRegistration, _ *androidKeys) {
		_, other := devicetest.NewKey(t)
		reg.AgreementKeyJWK = other
	}); code(err) != plxerr.AttestationFailed {
		t.Errorf("a chain that attests another key-agreement key: %v", err)
	}
	if err := register(t, func(reg *device.AttestedRegistration, _ *androidKeys) {
		_, other := devicetest.NewKey(t)
		reg.SCAKeyJWK = other
	}); code(err) != plxerr.AttestationFailed {
		t.Errorf("a chain that attests another SCA key: %v", err)
	}
	if err := register(t, func(reg *device.AttestedRegistration, _ *androidKeys) { reg.Evidence = developmentEvidence() }); code(err) == 0 {
		t.Error("Android chains with development evidence were accepted")
	}

	// A software DPoP key does not hold the other keys to a higher bar.
	soft := func(reg *device.AttestedRegistration, k *androidKeys) {
		dpopKey, dpopJWK := devicetest.NewKey(t)
		reg.DPoPKeyJWK, reg.KeyStorage = dpopJWK, device.KeyStorageUnspecified
		r.android(&dpopKey.PublicKey, keyattest.SecuritySoftware, playintegrity.LabelDevice)
		kaKey, kaJWK := devicetest.NewKey(t)
		reg.AgreementKeyJWK = kaJWK
		r.androidKey(agreementLeaf, &kaKey.PublicKey, keyattest.SecuritySoftware)
	}
	if err := register(t, soft); err != nil {
		t.Errorf("a software key-agreement key beside a software DPoP key: %v", err)
	}
}

// Verifies: SEC-004, SEC-027, SEC-053.
// An iOS registration binds the App Attest client data hash to all three
// keys; an SCA key enrolled later is vouched for by an assertion with the
// stored App Attest key over the binding of the fresh challenge and the
// registered DPoP and key-agreement thumbprints, and the counter advances.
func TestIOSBindsEveryKeyAndEnrolsSCAByAssertion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig(t)
	appKey, _ := devicetest.NewKey(t)
	r.apple.Attestation = appattest.Attestation{PublicKey: &appKey.PublicKey, Receipt: []byte("receipt")}
	dpopKey, dpopJWK := devicetest.NewKey(t)
	kaKey, kaJWK := devicetest.NewKey(t)
	scaKey, scaJWK := devicetest.NewKey(t)
	dpopJKT, kaJKT, scaJKT := thumbprint(t, &dpopKey.PublicKey), thumbprint(t, &kaKey.PublicKey), thumbprint(t, &scaKey.PublicKey)

	challenge := r.challenge(t, "production")
	ios := iosEvidence()
	d, err := r.svc.RegisterAttested(ctx, device.AttestedRegistration{
		AppID: r.app, Environment: "production", Platform: "ios", OSVersion: "18", RuntimeVersion: "1.0.0", Build: "42",
		Challenge: challenge, DPoPKeyJWK: dpopJWK, KeyStorage: device.KeyStorageSecureEnclave, Evidence: ios,
		AgreementKeyJWK: kaJWK, SCAKeyJWK: scaJWK, SCAEvidence: &ios,
	})
	if err != nil {
		t.Fatalf("RegisterAttested: %v", err)
	}
	if want := binding.Hash(challenge, dpopJKT, kaJKT, scaJKT); r.apple.ClientDataHash != want {
		t.Errorf("the client data hash does not bind all three keys")
	}

	// A new SCA key, enrolled with an assertion.
	newKey, newJWK := devicetest.NewKey(t)
	newJKT := thumbprint(t, &newKey.PublicKey)
	identity := device.Identity{DeviceID: d.ID, OrganizationID: r.org, AppID: d.AppID, EnvironmentID: d.EnvironmentID}
	fresh := r.challenge(t, "production")
	r.assert.Counter = 1
	enrol := device.SCAEnrolment{Challenge: fresh, KeyJWK: newJWK, Evidence: device.Evidence{IOS: &device.IOSEvidence{Assertion: []byte("assertion")}}}
	got, err := r.svc.EnrolSCAKey(ctx, identity, enrol)
	if err != nil || got.JKT != newJKT || got.Storage != device.KeyStorageSecureEnclave {
		t.Fatalf("EnrolSCAKey = %+v, %v", got, err)
	}
	if want := binding.Hash(fresh, dpopJKT, kaJKT, newJKT); r.assert.ClientDataHash != want || r.assert.AppID != iosAppID {
		t.Errorf("the assertion was checked against %x for %q", r.assert.ClientDataHash, r.assert.AppID)
	}
	var counter int64
	err = r.db.InTx(ctx, storage.Tenant{OrganizationID: r.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT app_attest_counter FROM devices WHERE id = $1`, d.ID).Scan(&counter)
	})
	if err != nil || counter != 1 {
		t.Errorf("stored counter = %d, %v", counter, err)
	}
	if stored, err := r.svc.SCAKey(ctx, identity); err != nil || stored.JKT != newJKT {
		t.Errorf("SCAKey = %+v, %v", stored, err)
	}

	// An assertion that does not advance the counter, and an attestation in
	// its place, are refused.
	enrol.Challenge = r.challenge(t, "production")
	if _, err := r.svc.EnrolSCAKey(ctx, identity, enrol); code(err) != plxerr.AttestationFailed {
		t.Errorf("a replayed assertion: %v", err)
	}
	enrol.Challenge = r.challenge(t, "production")
	enrol.Evidence = device.Evidence{IOS: &device.IOSEvidence{KeyID: []byte("key"), AttestationObject: []byte("object")}}
	if _, err := r.svc.EnrolSCAKey(ctx, identity, enrol); code(err) != plxerr.AttestationFailed {
		t.Errorf("an attestation instead of an assertion: %v", err)
	}
}

// Verifies: SEC-006, SEC-003.
// A re-attestation is bound to the keys the device registered, so the fresh
// verdict vouches for the same set.
func TestReattestBindsRegisteredKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig(t)
	k := newAndroidKeys(t, r)
	d, err := r.svc.RegisterAttested(ctx, k.registration(r, r.challenge(t, "production")))
	if err != nil {
		t.Fatal(err)
	}
	challenge := r.challenge(t, "production")
	if _, err := r.svc.Reattest(ctx, d.ID, challenge, androidEvidence()); err != nil {
		t.Fatalf("Reattest: %v", err)
	}
	if want := binding.RequestHash(challenge, k.dpopJKT, k.kaJKT, k.scaJKT); r.play.Expect.RequestHash != want {
		t.Errorf("the re-attestation request hash does not bind the registered keys")
	}
}

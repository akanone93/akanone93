// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device_test

import (
	"context"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: SEC-027.
// A device that registers with an SCA key and its evidence has the key
// stored, and one that enrols later replaces it; the evidence needs a
// challenge, which is spent.
func TestSCAKeyEnrolment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig(t)
	_, dpopJWK := devicetest.NewKey(t)
	scaKey, scaJWK := devicetest.NewKey(t)
	dev := developmentEvidence()
	d, err := r.svc.RegisterAttested(ctx, device.AttestedRegistration{
		AppID: r.app, Environment: "development", Platform: "linux", OSVersion: "6", RuntimeVersion: "1.0.0", Build: "42",
		Challenge: r.challenge(t, "development"), DPoPKeyJWK: dpopJWK, KeyStorage: device.KeyStorageSoftware, Evidence: dev,
		SCAKeyJWK: scaJWK, SCAEvidence: &dev,
	})
	if err != nil {
		t.Fatalf("RegisterAttested with an SCA key: %v", err)
	}
	identity := device.Identity{DeviceID: d.ID, OrganizationID: r.org, AppID: d.AppID, EnvironmentID: d.EnvironmentID}
	got, err := r.svc.SCAKey(ctx, identity)
	if err != nil || !got.Public.Equal(&scaKey.PublicKey) || got.Provider != "development" {
		t.Fatalf("SCAKey = %+v, %v", got, err)
	}

	replacement, replacementJWK := devicetest.NewKey(t)
	challenge := r.challenge(t, "development")
	enrol := device.SCAEnrolment{Challenge: challenge, KeyJWK: replacementJWK, Evidence: dev}
	if _, err := r.svc.EnrolSCAKey(ctx, identity, enrol); err != nil {
		t.Fatalf("EnrolSCAKey: %v", err)
	}
	got, err = r.svc.SCAKey(ctx, identity)
	if err != nil || !got.Public.Equal(&replacement.PublicKey) {
		t.Errorf("the key after enrolling again: %+v, %v", got, err)
	}
	if _, err := r.svc.EnrolSCAKey(ctx, identity, enrol); code(err) != plxerr.RegistrationChallengeInvalid {
		t.Errorf("the same challenge twice: %v", err)
	}
	if _, err := r.svc.SCAKey(ctx, device.Identity{DeviceID: "00000000-0000-7000-8000-000000000001", OrganizationID: identity.OrganizationID}); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a device with no key: %v", err)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest"
	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/attest/playintegrity"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

// outage is what the Key Attestation verifier returns while its status
// list cannot be had.
func outage() error {
	return plxerr.Wrap(plxerr.AttestationUnavailable, errors.Join(attest.ErrProviderUnavailable, errors.New("no list")), "the attestation status list is unavailable")
}

// outageRig registers an AL3 Android device and returns the rig, the
// device and its key.
func outageRig(t *testing.T) (*rig, device.Device) {
	t.Helper()
	r := newRig(t)
	key, jwk := devicetest.NewKey(t)
	r.android(&key.PublicKey, keyattest.SecurityTrustedEnvironment, playintegrity.LabelStrong)
	d, err := r.register(t, "production", "android", jwk, device.KeyStorageTEE, androidEvidence())
	if err != nil || d.AssuranceLevel != "AL3" {
		t.Fatalf("register = %+v, %v", d, err)
	}
	return r, d
}

// Verifies: SEC-009.
// While the provider is unavailable, a device whose attestation is due
// keeps its last known level for the grace of its environment, and loses
// it when the grace ends; the standard grace is 24 hours.
func TestOutageGraceKeepsTheLastKnownLevel(t *testing.T) {
	t.Parallel()
	r, d := outageRig(t)
	ctx := context.Background()

	// Due, and the provider is down: the device keeps its level.
	r.now = r.now.Add(25 * time.Hour)
	if _, err := r.refresh(d, d.DPoPJKT, "p"); code(err) != plxerr.ReattestationRequired {
		t.Fatalf("a due attestation without an observed outage: %v", err)
	}
	r.key.Err = outage()
	kept, err := r.svc.Reattest(ctx, d.ID, r.challenge(t, "production"), androidEvidence())
	if err != nil || kept.AssuranceLevel != "AL3" || !kept.AttestedAt.Equal(d.AttestedAt) {
		t.Fatalf("Reattest during an outage = %+v, %v", kept, err)
	}
	tok, err := r.refresh(d, d.DPoPJKT, "p")
	if err != nil || tok.Assurance != "AL3" {
		t.Fatalf("a refresh within the grace = %+v, %v", tok, err)
	}

	// The grace ends 24 hours after the attestation fell due.
	r.now = d.AttestedAt.Add(24*time.Hour + 24*time.Hour + time.Second)
	if _, err := r.refresh(d, d.DPoPJKT, "p"); code(err) != plxerr.ReattestationRequired {
		t.Errorf("a refresh after the grace: %v", err)
	}
	if _, err := r.svc.Reattest(ctx, d.ID, r.challenge(t, "production"), androidEvidence()); code(err) != plxerr.AttestationUnavailable {
		t.Errorf("Reattest after the grace: %v", err)
	}

	// The provider is back: the device attests and is current again.
	r.key.Err = nil
	again, err := r.svc.Reattest(ctx, d.ID, r.challenge(t, "production"), androidEvidence())
	if err != nil || !again.AttestedAt.After(d.AttestedAt) {
		t.Fatalf("Reattest after the outage = %+v, %v", again, err)
	}
	if _, err := r.refresh(d, d.DPoPJKT, "p"); err != nil {
		t.Errorf("a refresh after re-attesting: %v", err)
	}
}

// Verifies: SEC-009.
// Under the strict profile the grace is one hour, under maximum zero: the
// outage then fails closed with PLX-6009.
func TestOutageGraceFollowsTheProfile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		profile settings.Profile
		// afterDue is when, past the attestation falling due, the
		// device tries again, and whether the grace still holds.
		afterDue time.Duration
		kept     bool
	}{
		{settings.Strict, 30 * time.Minute, true},
		{settings.Strict, 61 * time.Minute, false},
		{settings.Maximum, time.Second, false},
	} {
		t.Run(string(tc.profile)+"/"+tc.afterDue.String(), func(t *testing.T) {
			t.Parallel()
			r, d := outageRig(t)
			r.profiles[r.envs["production"]] = tc.profile
			r.now = d.AttestedAt.Add(24*time.Hour + tc.afterDue)
			r.key.Err = outage()
			_, err := r.svc.Reattest(context.Background(), d.ID, r.challenge(t, "production"), androidEvidence())
			if tc.kept {
				if err != nil {
					t.Fatalf("Reattest = %v", err)
				}
				if _, err := r.refresh(d, d.DPoPJKT, "p"); err != nil {
					t.Errorf("a refresh within the grace: %v", err)
				}
				return
			}
			if code(err) != plxerr.AttestationUnavailable {
				t.Errorf("Reattest = %v, want PLX-6009", err)
			}
			if _, err := r.refresh(d, d.DPoPJKT, "p"); code(err) != plxerr.ReattestationRequired {
				t.Errorf("a refresh without grace: %v", err)
			}
		})
	}
}

// Verifies: SEC-009.
// Only an outage of the provider is graced: evidence that fails
// verification, and an attestation the server is not configured for, are
// refused however much grace is left; and a first registration has no
// last known level to keep.
func TestOutageGraceIsOnlyForOutages(t *testing.T) {
	t.Parallel()
	r, d := outageRig(t)
	ctx := context.Background()
	r.now = r.now.Add(25 * time.Hour)

	r.key.Err = plxerr.New(plxerr.AttestationFailed, "the chain is revoked")
	if _, err := r.svc.Reattest(ctx, d.ID, r.challenge(t, "production"), androidEvidence()); code(err) != plxerr.AttestationFailed {
		t.Errorf("failing evidence: %v", err)
	}
	r.key.Err = plxerr.New(plxerr.AttestationUnavailable, "Android attestation is not configured for this app")
	if _, err := r.svc.Reattest(ctx, d.ID, r.challenge(t, "production"), androidEvidence()); code(err) != plxerr.AttestationUnavailable {
		t.Errorf("a missing configuration: %v", err)
	}
	if _, err := r.refresh(d, d.DPoPJKT, "p"); code(err) != plxerr.ReattestationRequired {
		t.Errorf("a refresh after refused attempts: %v", err)
	}

	r.key.Err = outage()
	_, jwk := devicetest.NewKey(t)
	if _, err := r.register(t, "production", "android", jwk, device.KeyStorageTEE, androidEvidence()); code(err) != plxerr.AttestationUnavailable {
		t.Errorf("a first registration during an outage: %v", err)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package device_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest"
	"github.com/nightCode42/plux3/backend/internal/attest/appattest/receipt"
	"github.com/nightCode42/plux3/backend/internal/attest/appattest/receipt/receipttest"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// receiptRig is a rig whose service asks a simulator of Apple's data
// service for the fraud-metric receipt of an iOS device's key.
type receiptRig struct {
	*rig
	sim     *receipttest.Server
	client  *receipt.Client
	appKey  *ecdsa.PrivateKey
	initial []byte
	logs    *bytes.Buffer
}

func newReceiptRig(t *testing.T) *receiptRig {
	t.Helper()
	appKey, _ := devicetest.NewKey(t)
	rr := &receiptRig{appKey: appKey, logs: &bytes.Buffer{}}
	signer := receipttest.NewSigner(t)
	authority := receipttest.NewAuthority(t, time.Now())
	// The clock follows the rig's once it exists.
	clock := func() time.Time {
		if rr.rig == nil {
			return time.Now()
		}
		return rr.rig.now
	}
	rr.sim = receipttest.New(t, receipttest.Config{
		Authority: authority, JWTKey: signer.Public(), KeyID: "ABCDE12345", TeamID: "TEAMID1234", AppID: iosAppID, Key: &appKey.PublicKey, Now: clock,
	})
	var err error
	rr.client, err = receipt.New(receipt.Config{
		KeyID: "ABCDE12345", TeamID: "TEAMID1234", Signer: signer, HTTP: http.DefaultClient, Now: clock,
		Hosts: receipt.Hosts{Production: rr.sim.URL, Development: rr.sim.URL}, Roots: authority.Roots(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rr.initial = authority.Issue(t, receipttest.IssueOptions{AppID: iosAppID, Key: &appKey.PublicKey, Type: "ATTEST", OmitRisk: true})
	rr.rig = newRigWith(t, func(o *device.Options) {
		o.Attestors.FraudReceipt = rr.client
		o.Log = slog.New(slog.NewTextHandler(rr.logs, nil))
	})
	rr.apple.Attestation = appattest.Attestation{PublicKey: &appKey.PublicKey, Receipt: rr.initial}
	return rr
}

// storedReceipt reads the receipt kept for a device.
func (rr *receiptRig) storedReceipt(t *testing.T, deviceID string) []byte {
	t.Helper()
	var stored []byte
	err := rr.db.InTx(context.Background(), storage.Tenant{OrganizationID: rr.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT app_attest_receipt FROM devices WHERE id = $1`, deviceID).Scan(&stored)
	})
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

// registerIOS registers an iOS device with a Secure Enclave claim in the
// production environment, whose profile is set first.
func (rr *receiptRig) registerIOS(t *testing.T, profile settings.Profile, claim device.KeyStorage) (device.Device, error) {
	t.Helper()
	rr.profiles[rr.envs["production"]] = profile
	_, jwk := devicetest.NewKey(t)
	return rr.register(t, "production", "ios", jwk, claim, iosEvidence())
}

// Verifies: SEC-007.
// An iOS device whose Secure Enclave key has a good receipt registers at
// AL3 and keeps the refreshed receipt and its risk metric.
func TestRegisterIOSFraudReceiptAL3_SEC_007(t *testing.T) {
	t.Parallel()
	rr := newReceiptRig(t)
	rr.sim.SetRisk(2)
	d, err := rr.registerIOS(t, settings.Standard, device.KeyStorageSecureEnclave)
	if err != nil {
		t.Fatal(err)
	}
	if d.AssuranceLevel != "AL3" || d.RiskMetric != 2 {
		t.Errorf("RegisterAttested = level %s risk %d, want AL3 and 2", d.AssuranceLevel, d.RiskMetric)
	}
	stored := rr.storedReceipt(t, d.ID)
	if bytes.Equal(stored, rr.initial) {
		t.Error("the attestation's receipt was kept instead of the refreshed one")
	}
	if got, err := rr.client.Verify(stored, receipt.Expect{AppID: iosAppID, Key: &rr.appKey.PublicKey}); err != nil || got.RiskMetric != 2 {
		t.Errorf("the stored receipt = %+v, %v", got, err)
	}
	if !bytes.Equal(rr.sim.Received(), rr.initial) {
		t.Error("Apple was not sent the attestation's receipt")
	}
	if rr.logs.Len() != 0 {
		t.Errorf("a clean registration logged %q", rr.logs)
	}
}

// Verifies: SEC-007.
// Whatever goes wrong at Apple, registration succeeds at AL2 with the
// attestation's own receipt, and PLX-6080 is logged once and nothing else.
func TestRegisterIOSAppleOutage_SEC_007(t *testing.T) {
	t.Parallel()
	for _, mode := range []receipttest.Mode{
		receipttest.ModeUnauthorized, receipttest.ModeTooManyRequests, receipttest.ModeServerError, receipttest.ModeNotModified,
		receipttest.ModeTampered, receipttest.ModeExpired, receipttest.ModeWrongApp, receipttest.ModeWrongKey, receipttest.ModeUntrusted,
	} {
		rr := newReceiptRig(t)
		rr.sim.SetMode(mode)
		d, err := rr.registerIOS(t, settings.Standard, device.KeyStorageSecureEnclave)
		if err != nil {
			t.Fatalf("mode %d: registration failed: %v", mode, err)
		}
		if d.AssuranceLevel != "AL2" || d.RiskMetric != 0 {
			t.Errorf("mode %d: level %s risk %d, want AL2 and 0", mode, d.AssuranceLevel, d.RiskMetric)
		}
		if !bytes.Equal(rr.storedReceipt(t, d.ID), rr.initial) {
			t.Errorf("mode %d: the attestation's receipt was not kept", mode)
		}
		if n := strings.Count(rr.logs.String(), "PLX-6080"); n != 1 || strings.Count(rr.logs.String(), "\n") != 1 {
			t.Errorf("mode %d: log = %q", mode, rr.logs)
		}
		if strings.Contains(rr.logs.String(), "Bearer") || strings.Contains(rr.logs.String(), "eyJ") {
			t.Errorf("mode %d: the log holds a token", mode)
		}
	}
}

// Verifies: SEC-007.
// The risk metric decides AL3 at the profile's riskMetricMaxAL3: 5 under
// standard, 3 under strict, 1 under maximum; one above it stays at AL2.
func TestRegisterIOSRiskBoundary_SEC_007(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		profile settings.Profile
		limit   int
	}{{settings.Standard, 5}, {settings.Strict, 3}, {settings.Maximum, 1}} {
		for _, risk := range []int{0, c.limit, c.limit + 1} {
			rr := newReceiptRig(t)
			rr.sim.SetRisk(risk)
			d, err := rr.registerIOS(t, c.profile, device.KeyStorageSecureEnclave)
			if err != nil {
				t.Fatal(err)
			}
			want := "AL3"
			if risk > c.limit {
				want = "AL2"
			}
			if d.AssuranceLevel != want || d.RiskMetric != int32(risk) {
				t.Errorf("%s with risk %d: level %s risk %d, want %s", c.profile, risk, d.AssuranceLevel, d.RiskMetric, want)
			}
		}
	}
}

// Verifies: SEC-007, SEC-001.
// A software key stays at AL1, and Apple is not asked about it.
func TestRegisterIOSSoftwareKeyIgnoresReceipt_SEC_007(t *testing.T) {
	t.Parallel()
	rr := newReceiptRig(t)
	d, err := rr.registerIOS(t, settings.Standard, device.KeyStorageSoftware)
	if err != nil || d.AssuranceLevel != "AL1" {
		t.Errorf("a software key = %+v, %v", d, err)
	}
	if rr.sim.Requests() != 0 {
		t.Errorf("Apple was asked %d times about a software key", rr.sim.Requests())
	}
}

// Verifies: SEC-006, SEC-007.
// Re-attestation asks Apple again: a device that registered during an
// outage reaches AL3 once Apple answers.
func TestReattestIOSFraudReceipt_SEC_007(t *testing.T) {
	t.Parallel()
	rr := newReceiptRig(t)
	rr.sim.SetMode(receipttest.ModeServerError)
	d, err := rr.registerIOS(t, settings.Standard, device.KeyStorageSecureEnclave)
	if err != nil || d.AssuranceLevel != "AL2" {
		t.Fatalf("registration during the outage = %+v, %v", d, err)
	}
	rr.now = rr.now.Add(time.Hour)
	rr.sim.SetMode(receipttest.ModeOK)
	rr.sim.SetRisk(1)
	again, err := rr.svc.Reattest(context.Background(), d.ID, rr.challenge(t, "production"), iosEvidence())
	if err != nil || again.AssuranceLevel != "AL3" || again.RiskMetric != 1 {
		t.Errorf("Reattest = %+v, %v", again, err)
	}
	if bytes.Equal(rr.storedReceipt(t, d.ID), rr.initial) {
		t.Error("the refreshed receipt was not stored")
	}

	rr.now = rr.now.Add(time.Hour)
	rr.sim.SetMode(receipttest.ModeServerError)
	down, err := rr.svc.Reattest(context.Background(), d.ID, rr.challenge(t, "production"), iosEvidence())
	if err != nil || down.AssuranceLevel != "AL2" || down.RiskMetric != 0 {
		t.Errorf("Reattest during an outage = %+v, %v", down, err)
	}
}

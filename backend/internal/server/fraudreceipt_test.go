// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest/receipt/receipttest"
	"github.com/nightCode42/plux3/backend/internal/config"
)

// Verifies: SEC-007.
// The receipt client exists only when the feature is enabled, and then
// only with a signer.
func TestBuildFraudReceipt_SEC_007(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	if c, err := buildFraudReceipt(cfg, nil); c != nil || err != nil {
		t.Errorf("disabled = %v, %v, want nil", c, err)
	}
	cfg.Attestation.Apple.FraudReceipt = config.FraudReceipt{Enabled: true, KeyID: "ABCDE12345", TeamID: "TEAM123456"}
	if _, err := buildFraudReceipt(cfg, nil); err == nil {
		t.Error("enabled without a signer was accepted")
	}
	if c, err := buildFraudReceipt(cfg, receipttest.NewSigner(t)); c == nil || err != nil {
		t.Errorf("enabled = %v, %v, want a client", c, err)
	}
}

// TestReceiptSignerRefusesOtherRoles checks that the adapter handed to the
// api role can sign with Apple's key only, never with another imported key.
// Verifies: SEC-123.
func TestReceiptSignerRefusesOtherRoles_SEC_123(t *testing.T) {
	_, err := receiptSigner{}.SignES256(context.Background(), "some-other-role", make([]byte, 32))
	if err == nil {
		t.Fatal("signed with a role other than the Apple receipt key")
	}
}

// TestFraudReceiptOffByDefault checks that no client is built, and no key
// store opened, while the feature is off.
// Verifies: SEC-007.
func TestFraudReceiptOffByDefault_SEC_007(t *testing.T) {
	c, err := fraudReceiptFor(&config.Config{}, nil, nil)
	if err != nil || c != nil {
		t.Fatalf("fraudReceiptFor with the feature off = %v, %v; want nil, nil", c, err)
	}
}

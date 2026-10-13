// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
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

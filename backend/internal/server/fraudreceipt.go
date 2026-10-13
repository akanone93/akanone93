// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"errors"
	"fmt"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest/receipt"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// fraudReceiptTimeout bounds one request to Apple's data service, which
// registration waits on.
const fraudReceiptTimeout = 10 * time.Second

// buildFraudReceipt builds the client of Apple's fraud-metric receipt
// (SEC-007, ADR-0065). It returns nil when the feature is off, which leaves
// iOS devices at AL2. The signer holds the imported App Store Connect key
// under the role apple-appattest-receipt; the caller passes the
// signing.Imported of the installation and sets the result as
// device.Attestors.FraudReceipt.
func buildFraudReceipt(cfg *config.Config, signer receipt.ES256Signer) (device.FraudReceiptClient, error) {
	f := cfg.Attestation.Apple.FraudReceipt
	if !f.Enabled {
		return nil, nil
	}
	if signer == nil {
		return nil, errors.New("server: attestation.apple.fraudReceipt needs the imported key signer")
	}
	c, err := receipt.New(receipt.Config{
		KeyID: f.KeyID, TeamID: f.TeamID, Signer: signer, Now: time.Now, Timeout: fraudReceiptTimeout,
		HTTP: httpx.NewClient(httpx.Options{Timeout: fraudReceiptTimeout}),
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return c, nil
}

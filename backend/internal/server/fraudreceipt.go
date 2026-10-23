// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest/appattest/receipt"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// fraudReceiptTimeout bounds one request to Apple's data service, which
// registration waits on.
const fraudReceiptTimeout = 3 * time.Second

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
	client, err := httpx.NewClient(httpx.Options{Destination: httpx.KindAppleReceipt, Timeout: fraudReceiptTimeout})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	c, err := receipt.New(receipt.Config{
		KeyID: f.KeyID, TeamID: f.TeamID, Signer: signer, Now: time.Now, Timeout: fraudReceiptTimeout,
		HTTP: client,
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return c, nil
}

// receiptSigner exposes exactly one imported key, Apple's developer key, to
// the device service: the api role holds no signer of its own (L-3,
// ADR-0006), and this adapter cannot be used for any other role (SEC-123,
// ADR-0065). It is safe for concurrent use.
type receiptSigner struct{ keys *signing.Imported }

// SignES256 signs with the Apple key and refuses every other role.
func (r receiptSigner) SignES256(ctx context.Context, role string, digest []byte) ([]byte, error) {
	if role != receipt.Role {
		return nil, fmt.Errorf("server: the receipt signer signs only with %q, not %q", receipt.Role, role)
	}
	sig, err := r.keys.SignES256(ctx, role, digest)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return sig, nil
}

// fraudReceiptFor builds the receipt client over the imported key store when
// the feature is on; it returns nil when it is off.
func fraudReceiptFor(cfg *config.Config, db *storage.DB, crypter signing.Crypter) (device.FraudReceiptClient, error) {
	if !cfg.Attestation.Apple.FraudReceipt.Enabled {
		return nil, nil
	}
	keys, err := signing.NewImported(signing.ImportedOptions{
		Crypter: crypter, Store: signing.NewDBImportedKeyStore(db), Rand: rand.Reader, Now: time.Now,
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return buildFraudReceipt(cfg, receiptSigner{keys: keys})
}

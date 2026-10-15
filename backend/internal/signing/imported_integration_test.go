// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package signing_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
)

// Verifies: SEC-123.
// Over PostgreSQL an imported key signs, is stored only as an envelope,
// is replaced by a second import of its role, and is unreadable with
// another backend's wrapping key.
func TestImportedKeysInPostgres_SEC_123(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := storagetest.Open(t)
	crypter := backend(t)
	imp, err := signing.NewImported(signing.ImportedOptions{
		Crypter: crypter, Store: signing.NewDBImportedKeyStore(db), Rand: rand.Reader,
		Now: func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 123_000_000, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := imp.SignES256(ctx, roleReceipt, digestOf("x")); !errors.Is(err, signing.ErrNoKey) {
		t.Fatalf("SignES256 before any import = %v", err)
	}
	first := newP256(t)
	if err := imp.Import(ctx, roleReceipt, bytes.Clone(first.der), meta); err != nil {
		t.Fatalf("Import: %v", err)
	}
	digest := digestOf("header.claims")
	sig, err := imp.SignES256(ctx, roleReceipt, digest)
	if err != nil || !verifyJWS(&first.key.PublicKey, digest, sig) {
		t.Fatalf("SignES256 = %v", err)
	}
	info, err := imp.Info(ctx, roleReceipt)
	if err != nil || info.KeyID != meta.KeyID || info.TeamID != meta.TeamID || !info.CreatedAt.Equal(time.Date(2026, 10, 1, 12, 0, 0, 123_000_000, time.UTC)) {
		t.Errorf("Info = %+v, %v", info, err)
	}

	var ciphertext, wrapped []byte
	if err := db.Pool().QueryRow(ctx, `SELECT ciphertext, wrapped_data_key FROM imported_keys WHERE role = $1`, roleReceipt).Scan(&ciphertext, &wrapped); err != nil {
		t.Fatal(err)
	}
	d := scalar(first.key)
	if bytes.Contains(ciphertext, first.der) || bytes.Contains(ciphertext, d) || bytes.Contains(wrapped, d) {
		t.Error("the key is stored in the clear")
	}

	second := newP256(t)
	if err := imp.Import(ctx, roleReceipt, bytes.Clone(second.der), signing.ImportedKeyMeta{KeyID: "NEWKEY0001", TeamID: meta.TeamID}); err != nil {
		t.Fatalf("a second Import: %v", err)
	}
	pub, got, err := imp.PublicKey(ctx, roleReceipt)
	if err != nil || got.KeyID != "NEWKEY0001" || !pub.Equal(&second.key.PublicKey) {
		t.Errorf("after rotation PublicKey = %v, %v", got, err)
	}
	var rows int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM imported_keys`).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("%d rows after a rotation: %v", rows, err)
	}

	other, err := signing.NewImported(signing.ImportedOptions{
		Crypter: backend(t), Store: signing.NewDBImportedKeyStore(db), Rand: rand.Reader, Now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.SignES256(ctx, roleReceipt, digest); !errors.Is(err, plxerr.Sentinel(plxerr.ImportedKeyInvalid)) {
		t.Errorf("another backend signed: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `UPDATE imported_keys SET key_id = 'EDITED' WHERE role = $1`, roleReceipt); err != nil {
		t.Fatal(err)
	}
	if _, err := imp.SignES256(ctx, roleReceipt, digest); !errors.Is(err, plxerr.Sentinel(plxerr.ImportedKeyInvalid)) {
		t.Errorf("a row with an edited key ID signed: %v", err)
	}
}

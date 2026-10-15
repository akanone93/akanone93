// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package signing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// DBImportedKeyStore keeps imported-key envelopes in PostgreSQL, one row
// per role (SEC-123). The table is installation-wide, like
// installation_secrets, so every transaction runs without a tenant. It
// is safe for concurrent use.
type DBImportedKeyStore struct{ db *storage.DB }

// NewDBImportedKeyStore returns the store over a database.
func NewDBImportedKeyStore(db *storage.DB) *DBImportedKeyStore {
	return &DBImportedKeyStore{db: db}
}

// GetImportedKey returns the envelope of a role, or an error wrapping
// ErrNoKey.
func (s *DBImportedKeyStore) GetImportedKey(ctx context.Context, role string) (ImportedKeyRecord, error) {
	var row dbgen.ImportedKey
	err := s.db.InTx(ctx, storage.Tenant{}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetImportedKey(ctx, role)
		if err != nil {
			return fmt.Errorf("select: %w", err)
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportedKeyRecord{}, fmt.Errorf("signing: role %q: %w", role, ErrNoKey)
	}
	if err != nil {
		return ImportedKeyRecord{}, fmt.Errorf("signing: read the imported key %q: %w", role, err)
	}
	return ImportedKeyRecord{
		Role: row.Role, KeyID: row.KeyID, TeamID: row.TeamID,
		WrappedDataKey: row.WrappedDataKey, WrapKeyID: row.WrapKeyID, Nonce: row.Nonce, Ciphertext: row.Ciphertext,
		PublicKeySHA256: row.PublicKeySha256, CreatedAt: row.CreatedAt.Time,
	}, nil
}

// PutImportedKey stores an envelope and replaces the role's previous one
// in the same statement, so a reader sees the old key or the new one.
func (s *DBImportedKeyStore) PutImportedKey(ctx context.Context, rec ImportedKeyRecord) error {
	err := s.db.InTx(ctx, storage.Tenant{}, func(ctx context.Context, tx pgx.Tx) error {
		err := dbgen.New(tx).PutImportedKey(ctx, dbgen.PutImportedKeyParams{
			Role: rec.Role, KeyID: rec.KeyID, TeamID: rec.TeamID,
			WrappedDataKey: rec.WrappedDataKey, WrapKeyID: rec.WrapKeyID, Nonce: rec.Nonce, Ciphertext: rec.Ciphertext,
			PublicKeySha256: rec.PublicKeySHA256, CreatedAt: pgtype.Timestamptz{Time: rec.CreatedAt, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("signing: write the imported key %q: %w", rec.Role, err)
	}
	return nil
}

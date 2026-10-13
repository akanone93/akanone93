// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// KVScope names one namespace of the key-value store: a function in an app
// and an environment (FN-018).
type KVScope struct {
	OrganizationID string
	AppID          string
	EnvironmentID  string
	Function       string
}

// KVGet returns the value of a key; found is false when the key does not
// exist. It is for the host service the function runner reaches, which has
// authorised the call with the invocation's grant (E8).
func (s *Service) KVGet(ctx context.Context, sc KVScope, key string) (value []byte, found bool, err error) {
	err = s.inKV(ctx, sc, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries, env dbgen.Environment) error {
		v, err := q.GetFunctionKV(ctx, dbgen.GetFunctionKVParams{EnvironmentID: env.ID, FunctionName: sc.Function, Key: key})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("function: read a key: %w", err)
		}
		value, found = v, true
		return nil
	})
	return value, found, err
}

// KVPut stores a value. The limits function.kvValueBytes, function.kvKeys
// and function.kvBytes are checked in the transaction that writes, under
// a lock that serialises the writes of one namespace, so that concurrent
// writes cannot together exceed them; a write that would is refused with
// PLX-7016 (FN-018).
func (s *Service) KVPut(ctx context.Context, sc KVScope, key string, value []byte) error {
	if key == "" {
		return plxerr.New(plxerr.InvalidFormat, "a key must not be empty")
	}
	return s.inKV(ctx, sc, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, env dbgen.Environment) error {
		if err := q.LockFunction(ctx, lockScope("kv", env.ID, sc.Function)); err != nil {
			return fmt.Errorf("function: lock a namespace: %w", err)
		}
		replaced := int64(-1)
		old, err := q.GetFunctionKVForUpdate(ctx, dbgen.GetFunctionKVForUpdateParams{EnvironmentID: env.ID, FunctionName: sc.Function, Key: key})
		switch {
		case err == nil:
			replaced = int64(old)
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("function: read a key: %w", err)
		}
		use, err := q.FunctionKVUsage(ctx, dbgen.FunctionKVUsageParams{EnvironmentID: env.ID, FunctionName: sc.Function})
		if err != nil {
			return fmt.Errorf("function: measure a namespace: %w", err)
		}
		lim, err := s.o.Limits(ctx, tx, sc.OrganizationID, sc.AppID)
		if err != nil {
			return fmt.Errorf("function: %w", err)
		}
		if err := checkKVPut(lim, kvUsage{Keys: use.Keys, Bytes: use.TotalBytes}, replaced, int64(len(key)), int64(len(value))); err != nil {
			return err
		}
		if err := q.UpsertFunctionKV(ctx, dbgen.UpsertFunctionKVParams{
			OrganizationID: env.OrganizationID, AppID: env.AppID, EnvironmentID: env.ID, FunctionName: sc.Function, Key: key, Value: value,
		}); err != nil {
			return fmt.Errorf("function: write a key: %w", err)
		}
		return nil
	})
}

// KVDelete removes a key and reports whether it existed.
func (s *Service) KVDelete(ctx context.Context, sc KVScope, key string) (existed bool, err error) {
	err = s.inKV(ctx, sc, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries, env dbgen.Environment) error {
		n, err := q.DeleteFunctionKV(ctx, dbgen.DeleteFunctionKVParams{EnvironmentID: env.ID, FunctionName: sc.Function, Key: key})
		if err != nil {
			return fmt.Errorf("function: delete a key: %w", err)
		}
		existed = n > 0
		return nil
	})
	return existed, err
}

// inKV runs an operation on a namespace in a transaction bound to its
// organisation, after checking that the environment belongs to the app.
func (s *Service) inKV(ctx context.Context, sc KVScope, f func(context.Context, pgx.Tx, *dbgen.Queries, dbgen.Environment) error) error {
	if err := ValidateName(sc.Function); err != nil {
		return err
	}
	return s.inTenant(ctx, storage.Tenant{OrganizationID: sc.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, sc.AppID, sc.EnvironmentID)
		if err != nil {
			return err
		}
		return f(ctx, tx, q, env)
	})
}

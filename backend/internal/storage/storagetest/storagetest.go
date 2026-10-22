// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package storagetest connects integration tests to a real PostgreSQL
// (QA-005).
//
// The database is named by PLUX_TEST_DATABASE_URL — the local server in
// development, a service container in CI. Each test gets its own schema,
// migrated from scratch and dropped afterwards, so tests are independent
// and can run in parallel. Without the variable the tests skip, so
// `go test ./...` stays useful on a machine with no database.
package storagetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/storage"
)

// URLVariable is the environment variable naming the test database.
const URLVariable = "PLUX_TEST_DATABASE_URL"

// URL returns the configured database URL, or "" when there is none.
func URL() string { return os.Getenv(URLVariable) }

// Skip skips the test when no database is configured, saying how to
// provide one.
func Skip(t testing.TB) string {
	t.Helper()
	url := URL()
	if url == "" {
		t.Skipf("set %s to run this integration test (QA-005)", URLVariable)
	}
	return url
}

// SchemaURL returns a connection URL whose search path is a fresh,
// empty schema of the test's own, dropped when the test ends. It is for
// tests that start a whole process, which migrates the schema itself.
func SchemaURL(t testing.TB) string {
	t.Helper()
	url := Skip(t)
	schema := "test_" + token()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect to %s: %v", URLVariable, err)
	}
	defer func() { _ = admin.Close(ctx) }()
	//nolint:misspell // Sanitize is pgx's own spelling
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { dropSchema(t, url, schema) })
	return withSchema(url, schema)
}

// Open returns a database whose search path is a schema of its own, with
// every migration applied. The schema is dropped when the test ends.
func Open(t testing.TB) *storage.DB {
	t.Helper()
	migrations, err := storage.LoadMigrations(storage.Migrations())
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	return OpenWith(t, migrations)
}

// OpenWith is Open with a migration set of the test's own, for testing
// the migration machinery itself.
func OpenWith(t testing.TB, migrations []storage.Migration) *storage.DB {
	t.Helper()
	url := Skip(t)
	schema := "test_" + token()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect to %s: %v", URLVariable, err)
	}
	//nolint:misspell // Sanitize is pgx's own spelling
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create schema: %v", err)
	}
	if err := admin.Close(ctx); err != nil {
		t.Fatalf("close the admin connection: %v", err)
	}

	db, err := storage.Open(ctx, storage.Options{
		URL:            withSchema(url, schema),
		MaxConnections: 4,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Migrations run under one advisory lock for the whole database
	// (SRV-021), so every test of every package in parallel waits its
	// turn: the wait grows with the number of tests and migrations, not
	// with this test. It gets a budget of its own.
	migrateCtx, cancelMigrate := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelMigrate()
	if err := db.Migrate(migrateCtx, migrations); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		dropSchema(t, url, schema)
	})
	return db
}

// dropSchema drops a test's schema one table at a time, each in a
// transaction of its own, and then the schema. A migrated schema holds
// hundreds of tables, and dropping it whole locks every object in one
// transaction: the cleanups of tests that ended together exhausted the
// server's shared lock table ("out of shared memory"). Dropped table by
// table, no transaction holds more than a few locks, and no test waits for
// another.
func dropSchema(t testing.TB, url, schema string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Logf("drop schema %s: %v", schema, err)
		return
	}
	defer func() { _ = c.Close(ctx) }()
	rows, err := c.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = $1", schema)
	if err != nil {
		t.Logf("drop schema %s: list its tables: %v", schema, err)
		return
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Logf("drop schema %s: list its tables: %v", schema, err)
		return
	}
	for _, table := range tables {
		//nolint:misspell // Sanitize is pgx's own spelling
		if _, err := c.Exec(ctx, "DROP TABLE IF EXISTS "+pgx.Identifier{schema, table}.Sanitize()+" CASCADE"); err != nil {
			t.Logf("drop table %s.%s: %v", schema, table, err)
		}
	}
	//nolint:misspell // Sanitize is pgx's own spelling
	if _, err := c.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
		t.Logf("drop schema %s: %v", schema, err)
	}
}

// withSchema returns the URL with the search path set to one schema, so
// every connection of the pool sees only the test's own tables.
func withSchema(url, schema string) string {
	sep := "?"
	for i := range url {
		if url[i] == '?' {
			sep = "&"
			break
		}
	}
	return url + sep + "search_path=" + schema
}

// token returns a short random identifier for a schema name.
func token() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("storagetest: %v", err))
	}
	return hex.EncodeToString(b[:])
}

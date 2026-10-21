// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package storage_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
)

// A fixed cast for the restriction tests: one organisation, two apps with an
// environment each, two people in a team and one outside it.
const (
	restrictedOrg   = "01a0c450-6c00-7000-8000-0000000000c0"
	restrictedAppA  = "01a0c450-6c00-7000-8000-0000000000a1"
	restrictedAppB  = "01a0c450-6c00-7000-8000-0000000000a2"
	restrictedEnvA  = "01a0c450-6c00-7000-8000-0000000000e1"
	restrictedEnvB  = "01a0c450-6c00-7000-8000-0000000000e2"
	restrictedTeam  = "01a0c450-6c00-7000-8000-0000000000d0"
	personAda       = "01a0c450-6c00-7000-8000-0000000000b1"
	personBob       = "01a0c450-6c00-7000-8000-0000000000b2"
	personCy        = "01a0c450-6c00-7000-8000-0000000000b3"
	restrictionByID = "01a0c450-6c00-7000-8000-0000000000f0"
)

// seedRestrictions creates the cast: Ada and Bob are in the team, Cy is not.
func seedRestrictions(t *testing.T, db *storage.DB) {
	t.Helper()
	ctx := context.Background()
	pool := db.Pool()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, key, name) VALUES ($1, 'acme', 'Acme')`, restrictedOrg); err != nil {
		t.Fatal(err)
	}
	for id, email := range map[string]string{personAda: "ada@example.com", personBob: "bob@example.com", personCy: "cy@example.com"} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, display_name) VALUES ($1, $2, $2)`, id, email); err != nil {
			t.Fatal(err)
		}
	}
	err := db.InTx(ctx, storage.Tenant{OrganizationID: restrictedOrg}, func(ctx context.Context, tx pgx.Tx) error {
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO apps (id, organization_id, key, name) VALUES ($1, $2, 'a', 'A'), ($3, $2, 'b', 'B')`, []any{restrictedAppA, restrictedOrg, restrictedAppB}},
			{
				`INSERT INTO environments (id, organization_id, app_id, key, name) VALUES ($1, $2, $3, 'production', 'P'), ($4, $2, $5, 'production', 'P')`,
				[]any{restrictedEnvA, restrictedOrg, restrictedAppA, restrictedEnvB, restrictedAppB},
			},
			{`INSERT INTO teams (id, organization_id, key, name) VALUES ($1, $2, 'web', 'Web')`, []any{restrictedTeam, restrictedOrg}},
			{
				`INSERT INTO memberships (id, organization_id, user_id, team_id, role) VALUES
				('01a0c450-6c00-7000-8000-0000000001a1', $1, $2, $3, 'viewer'), ('01a0c450-6c00-7000-8000-0000000001a2', $1, $4, $3, 'viewer')`,
				[]any{restrictedOrg, personAda, restrictedTeam, personBob},
			},
		} {
			if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// restrict inserts a restriction as the service would.
func restrict(t *testing.T, db *storage.DB, kind string, team, user *string, patterns []string, app, environment *string) {
	t.Helper()
	err := db.InTx(context.Background(), storage.Tenant{OrganizationID: restrictedOrg}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO restriction_policies (id, organization_id, subject_kind, team_id, user_id, patterns, app_id, environment)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, restrictionByID, restrictedOrg, kind, team, user, patterns, app, environment)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func ptr(s string) *string { return &s }

// seen reads which of the two apps' environments, and which apps, a person
// can see, querying as that person through storage with no service in
// between.
func seen(t *testing.T, db *storage.DB, person string) (environments, apps int) {
	t.Helper()
	err := db.InTx(context.Background(), storage.Tenant{OrganizationID: restrictedOrg, UserID: person}, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM environments`).Scan(&environments); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM apps`).Scan(&apps)
	})
	if err != nil {
		t.Fatal(err)
	}
	return environments, apps
}

// Verifies: GOV-006, SEC-102.
// A person barred from an app by a restriction on every permission cannot
// read the app's rows or write them, whichever way the restriction reaches
// them: on the person, on their team or on the organisation; the others, and
// a restriction that does not name every permission, leave the rows visible.
func TestRestrictedPersonIsBarredByRowLevelSecurity_GOV_006(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name                   string
		kind                   string
		team, user             *string
		patterns               []string
		app, environment       *string
		adaEnvs, adaApps       int
		bobEnvs, cyEnvs, cyApp int
	}{
		{"the person", "user", nil, ptr(personAda), []string{"*"}, ptr(restrictedAppA), nil, 1, 1, 2, 2, 2},
		{"the team", "team", ptr(restrictedTeam), nil, []string{"*"}, ptr(restrictedAppA), nil, 1, 1, 1, 2, 2},
		{"the organisation", "organization", nil, nil, []string{"*"}, ptr(restrictedAppA), nil, 1, 1, 1, 1, 1},
		{"the organisation on every app", "organization", nil, nil, []string{"*"}, nil, nil, 0, 0, 0, 0, 0},
		{"one permission only", "user", nil, ptr(personAda), []string{"plugin.edit"}, ptr(restrictedAppA), nil, 2, 2, 2, 2, 2},
		{"a wildcard that is not everything", "user", nil, ptr(personAda), []string{"plugin.*", "release.*"}, ptr(restrictedAppA), nil, 2, 2, 2, 2, 2},
		{"one environment only", "user", nil, ptr(personAda), []string{"*"}, ptr(restrictedAppA), ptr("production"), 2, 2, 2, 2, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.Open(t)
			seedRestrictions(t, db)
			restrict(t, db, c.kind, c.team, c.user, c.patterns, c.app, c.environment)
			if envs, apps := seen(t, db, personAda); envs != c.adaEnvs || apps != c.adaApps {
				t.Errorf("Ada sees %d environments and %d apps, want %d and %d", envs, apps, c.adaEnvs, c.adaApps)
			}
			if envs, _ := seen(t, db, personBob); envs != c.bobEnvs {
				t.Errorf("Bob sees %d environments, want %d", envs, c.bobEnvs)
			}
			if envs, apps := seen(t, db, personCy); envs != c.cyEnvs || apps != c.cyApp {
				t.Errorf("Cy sees %d environments and %d apps, want %d and %d", envs, apps, c.cyEnvs, c.cyApp)
			}
		})
	}
}

// Verifies: GOV-006, SEC-102.
// The bar holds for writes: a barred person cannot create, change or delete
// a row of the app, and can write those of the other app.
func TestRestrictedPersonCannotWriteTheApp_GOV_006(t *testing.T) {
	t.Parallel()
	db := storagetest.Open(t)
	seedRestrictions(t, db)
	restrict(t, db, "user", nil, ptr(personAda), []string{"*"}, ptr(restrictedAppA), nil)
	ctx := context.Background()
	asAda := storage.Tenant{OrganizationID: restrictedOrg, UserID: personAda}
	err := db.InTx(ctx, asAda, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO environments (id, organization_id, app_id, key, name) VALUES
			('01a0c450-6c00-7000-8000-0000000000e3', $1, $2, 'staging', 'S')`, restrictedOrg, restrictedAppA)
		return err
	})
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "42501" {
		t.Errorf("insert into the barred app: %v, want a row-level security violation", err)
	}
	err = db.InTx(ctx, asAda, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE environments SET name = 'x' WHERE app_id = $1`, restrictedAppA)
		if err == nil && tag.RowsAffected() != 0 {
			t.Errorf("updated %d rows of the barred app", tag.RowsAffected())
		}
		tag, err = tx.Exec(ctx, `DELETE FROM environments WHERE app_id = $1`, restrictedAppA)
		if err == nil && tag.RowsAffected() != 0 {
			t.Errorf("deleted %d rows of the barred app", tag.RowsAffected())
		}
		return err
	})
	if err != nil {
		t.Errorf("update and delete: %v", err)
	}
	err = db.InTx(ctx, asAda, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE environments SET name = 'ok' WHERE app_id = $1`, restrictedAppB)
		return err
	})
	if err != nil {
		t.Errorf("update of the other app: %v", err)
	}
}

// Verifies: GOV-006, SEC-102.
// With no signed-in person in the transaction, as in system work and
// workloads, the bar does not apply: the service layer decides there.
func TestBarDoesNotApplyWithoutAPerson_GOV_006(t *testing.T) {
	t.Parallel()
	db := storagetest.Open(t)
	seedRestrictions(t, db)
	restrict(t, db, "organization", nil, nil, []string{"*"}, nil, nil)
	var n int
	err := db.InTx(context.Background(), storage.Tenant{OrganizationID: restrictedOrg}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM environments`).Scan(&n)
	})
	if err != nil || n != 2 {
		t.Errorf("environments = %d, %v; want 2", n, err)
	}
}

// Verifies: GOV-006.
// A restriction cannot be changed, only deleted and made again; and
// deleting a team or a person deletes their restrictions.
func TestRestrictionRowsAreImmutable_GOV_006(t *testing.T) {
	t.Parallel()
	db := storagetest.Open(t)
	seedRestrictions(t, db)
	restrict(t, db, "team", ptr(restrictedTeam), nil, []string{"plugin.*"}, nil, nil)
	ctx := context.Background()
	tenant := storage.Tenant{OrganizationID: restrictedOrg}
	err := db.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE restriction_policies SET patterns = '{*}'`)
		return err
	})
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23001" {
		t.Errorf("UPDATE = %v, want restrict_violation", err)
	}
	var subject string
	if err := db.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT subject_id::text FROM restriction_policies`).Scan(&subject)
	}); err != nil || subject != restrictedTeam {
		t.Errorf("subject_id = %q, %v; want the team", subject, err)
	}
	var left int
	if err := db.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM teams WHERE id = $1`, restrictedTeam); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM restriction_policies`).Scan(&left)
	}); err != nil || left != 0 {
		t.Errorf("restrictions after deleting the team = %d, %v; want 0", left, err)
	}
}

// Verifies: GOV-006.
// The shape of a restriction is enforced by the table: the subject matches
// its kind, patterns are not empty and an environment needs an app.
func TestRestrictionShapeIsEnforced_GOV_006(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		kind       string
		team, user *string
		patterns   []string
		app, env   *string
	}{
		"organisation with a subject": {"organization", nil, ptr(personAda), []string{"*"}, nil, nil},
		"user without a person":       {"user", nil, nil, []string{"*"}, nil, nil},
		"team and person":             {"team", ptr(restrictedTeam), ptr(personAda), []string{"*"}, nil, nil},
		"no patterns":                 {"organization", nil, nil, []string{}, nil, nil},
		"environment without an app":  {"organization", nil, nil, []string{"*"}, nil, ptr("production")},
		"unknown kind":                {"group", nil, nil, []string{"*"}, nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.Open(t)
			seedRestrictions(t, db)
			err := db.InTx(context.Background(), storage.Tenant{OrganizationID: restrictedOrg}, func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO restriction_policies (id, organization_id, subject_kind, team_id, user_id, patterns, app_id, environment)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, restrictionByID, restrictedOrg, c.kind, c.team, c.user, c.patterns, c.app, c.env)
				return err
			})
			var pg *pgconn.PgError
			if !errors.As(err, &pg) || pg.Code != "23514" {
				t.Errorf("insert = %v, want a check violation", err)
			}
		})
	}
}

// Verifies: GOV-006, SEC-102.
// Every table with row-level security and an app_id column carries the
// restrictive policy that bars a restricted person, so that a migration that
// adds an app-scoped table without calling plux_app_policy fails here. The
// one exception is the table the policy's function reads.
func TestEveryAppTableHasTheRestrictivePolicy_GOV_006(t *testing.T) {
	t.Parallel()
	db := storagetest.Open(t)
	rows, err := db.Pool().Query(context.Background(), `
		SELECT c.relname,
		       EXISTS (SELECT 1 FROM pg_policy p
		                WHERE p.polrelid = c.oid AND p.polname = 'plux_app_unrestricted' AND NOT p.polpermissive),
		       a.atttypid = 'uuid'::regtype
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'app_id' AND NOT a.attisdropped
		 WHERE c.relkind = 'r' AND c.relrowsecurity AND n.nspname = current_schema()
		 ORDER BY c.relname`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var name string
		var policy, isUUID bool
		if err := rows.Scan(&name, &policy, &isUUID); err != nil {
			t.Fatal(err)
		}
		seen++
		switch {
		case name == "restriction_policies":
			if policy {
				t.Errorf("%s carries the policy, but its own function reads it", name)
			}
		case !isUUID:
			t.Errorf("%s: app_id is not a uuid, so plux_app_restricted cannot guard it", name)
		case !policy:
			t.Errorf("%s has an app_id column and row-level security but no restrictive plux_app_unrestricted policy; call plux_app_policy on it", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen < 10 {
		t.Fatalf("only %d app tables were examined", seen)
	}
	var onApps bool
	if err := db.Pool().QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM pg_policy p WHERE p.polrelid = 'apps'::regclass AND p.polname = 'plux_app_unrestricted' AND NOT p.polpermissive)`).Scan(&onApps); err != nil || !onApps {
		t.Errorf("apps carries the restrictive policy = %v, %v", onApps, err)
	}
}

// Verifies: GOV-006.
// The permissions version of an organisation rises in the transaction of
// every change to grants, app access, custom roles, memberships and
// restrictions, and a rolled-back change leaves it alone.
func TestPermissionsVersionRises_GOV_006(t *testing.T) {
	t.Parallel()
	db := storagetest.Open(t)
	seedRestrictions(t, db)
	ctx := context.Background()
	tenant := storage.Tenant{OrganizationID: restrictedOrg}
	version := func() int64 {
		t.Helper()
		var v int64
		if err := db.Pool().QueryRow(ctx, `SELECT permissions_version FROM organizations WHERE id = $1`, restrictedOrg).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	base := version() // the seeded memberships already rose it
	if base < 2 {
		t.Fatalf("version after two memberships = %d", base)
	}
	steps := []struct{ name, sql string }{
		{"a grant", `INSERT INTO role_grants (id, organization_id, user_id, role, scope) VALUES ('01a0c450-6c00-7000-8000-0000000002a1', $1, '` + personCy + `', 'viewer', 'organization')`},
		{"a grant's role", `UPDATE role_grants SET role = 'developer'`},
		{"app access", `INSERT INTO app_access (id, organization_id, app_id, user_id, role) VALUES ('01a0c450-6c00-7000-8000-0000000002a2', $1, '` + restrictedAppA + `', '` + personCy + `', 'viewer')`},
		{"a custom role", `INSERT INTO custom_roles (id, organization_id, key, name, permissions) VALUES ('01a0c450-6c00-7000-8000-0000000002a3', $1, 'r', 'R', '{app.read}')`},
		{"a membership", `UPDATE memberships SET role = 'developer' WHERE user_id = '` + personAda + `'`},
		{"a restriction", `INSERT INTO restriction_policies (id, organization_id, subject_kind, patterns) VALUES ('01a0c450-6c00-7000-8000-0000000002a4', $1, 'organization', '{plugin.*}')`},
		{"a restriction removed", `DELETE FROM restriction_policies`},
		{"a grant removed", `DELETE FROM role_grants`},
	}
	for _, s := range steps {
		before := version()
		if err := db.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
			var args []any
			if strings.Contains(s.sql, "$1") {
				args = append(args, restrictedOrg)
			}
			_, err := tx.Exec(ctx, s.sql, args...)
			return err
		}); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if after := version(); after <= before {
			t.Errorf("%s: version %d -> %d, want it to rise", s.name, before, after)
		}
	}
	before := version()
	errRollback := errors.New("roll back")
	err := db.InTx(ctx, tenant, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE memberships SET role = 'viewer'`); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) || version() != before {
		t.Errorf("a rolled-back change moved the version: %d -> %d (%v)", before, version(), err)
	}
}

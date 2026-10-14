// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// promoteFunction copies a function version from the development
// environment to another one, as plux fn promote does (FN-006); passed
// overrides tests_passed of the copy.
func (f *fixture) promoteFunction(t *testing.T, to, name string, version int32, passed bool) {
	t.Helper()
	f.copyFunction(t, to, name, version, passed, nil)
}

// copyFunction is promoteFunction with the module hash of the copy replaced
// when module is not nil.
func (f *fixture) copyFunction(t *testing.T, to, name string, version int32, passed bool, module []byte) {
	t.Helper()
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO function_versions (organization_id, app_id, environment_id, name, version, build_id, placement,
		    module_key, module_sha256, abi_version, input_schema, output_schema, provenance, statement, signature, key_id, tests_passed)
		  SELECT organization_id, app_id, $1::uuid, name, version, build_id, placement, module_key, coalesce($6::bytea, module_sha256), abi_version,
		    input_schema, output_schema, provenance, statement, signature, key_id, $2
		    FROM function_versions WHERE environment_id = $3::uuid AND name = $4 AND version = $5`,
			f.envs[to].ID, passed, f.envs["development"].ID, name, version, module)
		return err
	})
	if err != nil {
		t.Fatalf("promote a function: %v", err)
	}
}

// Verifies: FN-006, FN-007.
// A release made in development is promoted to production only after every
// function version it pins was promoted there; a refusal names the
// function, the version and the environment, and tells how to promote. The
// promoted release then pins its versions in production.
func TestReleasePromotionNeedsFunctionPromotion_FN_006_FN_007(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	prod := f.envs["production"].ID
	f.grantFunctions(t, [][2]string{{"loans.quote", ""}, {"loans.rate", ""}})
	f.deploy(t, deployed{"loans.quote", 1, "server", true, ""}, deployed{"loans.rate", 1, "server", true, ""})
	f.publishAll(t)
	rel, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, "")
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}

	_, err = f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, prod, "")
	if code(err) != plxerr.FunctionNotFound {
		t.Fatalf("promoting a release before its functions: %v", err)
	}
	for _, want := range []string{"loans.quote", "version 1", "production", "plux fn promote loans.quote@1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not mention %q", err, want)
		}
	}
	f.promoteFunction(t, "production", "loans.quote", 1, true)
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, prod, ""); code(err) != plxerr.FunctionNotFound || !strings.Contains(err.Error(), "loans.rate") {
		t.Errorf("promoting a release with one function missing: %v", err)
	}
	f.promoteFunction(t, "production", "loans.rate", 1, true)
	promoted, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, prod, "")
	if err != nil {
		t.Fatalf("promoting a release after its functions: %v", err)
	}
	wantPinned(t, promoted.Functions, deployed{"loans.quote", 1, "server", true, ""}, deployed{"loans.rate", 1, "server", true, ""})
}

// Verifies: FN-006.
// A promoted copy with another module is not the version the release pins.
func TestReleasePromotionComparesTheModule_FN_006(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	f.grantFunctions(t, [][2]string{{"loans.quote", ""}})
	f.deploy(t, deployed{"loans.quote", 1, "server", true, ""})
	f.publishAll(t)
	rel, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	f.copyFunction(t, "production", "loans.quote", 1, true, bytes.Repeat([]byte{2}, 32))
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, f.envs["production"].ID, ""); code(err) != plxerr.FunctionNotFound {
		t.Errorf("promoting a release over another module: %v", err)
	}
}

// Verifies: FN-031.
// In a production environment a release whose pinned version failed its
// tests is refused, however the version got there.
func TestReleasePromotionRefusesFailedTests_FN_031(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	f.grantFunctions(t, [][2]string{{"loans.quote", ""}})
	f.deploy(t, deployed{"loans.quote", 1, "server", false, ""})
	f.publishAll(t)
	rel, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, "")
	if err != nil {
		t.Fatalf("a development release with failing tests: %v", err)
	}
	f.promoteFunction(t, "production", "loans.quote", 1, false)
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, rel.Sequence, f.envs["production"].ID, ""); code(err) != plxerr.FunctionTestsFailed {
		t.Errorf("promoting a release with failing tests to production: %v", err)
	}
}

// Verifies: FN-006, REL-006.
// A rollback in production needs the functions of the release it returns to
// there, and the channel's pins are found in the promoted-to environment.
func TestRollbackAndPinsInPromotedEnvironment_FN_006(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	dev, prod := f.envs["development"].ID, f.envs["production"].ID
	f.grantFunctions(t, [][2]string{{"loans.quote", ""}})
	f.deploy(t, deployed{"loans.quote", 1, "server", true, ""})
	f.publishAll(t)
	first, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "First")
	if err != nil {
		t.Fatal(err)
	}
	f.deploy(t, deployed{"loans.quote", 2, "server", true, ""})
	second, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "Second")
	if err != nil {
		t.Fatal(err)
	}

	// Production holds only version 2, so the first release cannot return.
	f.promoteFunction(t, "production", "loans.quote", 2, true)
	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, second.Sequence, prod, ""); err != nil {
		t.Fatalf("promoting the second release: %v", err)
	}
	if _, err := f.rel.RollbackRelease(ctx, f.owner, f.app, prod, "", first.Sequence, ""); code(err) != plxerr.FunctionNotFound {
		t.Fatalf("rolling back to a release whose function was not promoted: %v", err)
	}
	f.promoteFunction(t, "production", "loans.quote", 1, true)
	back, err := f.rel.RollbackRelease(ctx, f.owner, f.app, prod, "", first.Sequence, "")
	if err != nil {
		t.Fatalf("rolling back after the promotion: %v", err)
	}
	wantPinned(t, back.Functions, deployed{"loans.quote", 1, "server", true, ""})

	// The channel of production points at the rollback, which pins version 1
	// for the environment the first release was created in; the lookup is by
	// the environment of the channel.
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		for _, c := range []struct {
			env     string
			version int32
			found   bool
		}{{prod, 1, true}, {prod, 2, false}, {dev, 1, false}} {
			id := storage.MustUUID(c.env)
			_, err := q.GetPinnedFunction(ctx, dbgen.GetPinnedFunctionParams{AppID: storage.MustUUID(f.app), EnvironmentID: id, Name: "loans.quote", Version: c.version})
			if found := err == nil; found != c.found {
				t.Errorf("pin of version %d in %s: found = %v (%v), want %v", c.version, c.env, found, err, c.found)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

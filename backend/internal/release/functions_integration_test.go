// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// grantFunctions grants functions to the loans plugin in its draft; each
// grant is a function name and an alias, which may be empty.
func (f *fixture) grantFunctions(t *testing.T, grants [][2]string) {
	t.Helper()
	ctx := context.Background()
	const path = "plugins/loans/plugin.json"
	if _, _, err := f.docs.AcquireLock(ctx, f.owner, f.app, f.loans, "grant", false); err != nil {
		t.Fatal(err)
	}
	doc, err := f.docs.GetDocument(ctx, f.owner, f.app, f.loans, path)
	if err != nil {
		t.Fatal(err)
	}
	var plugin map[string]any
	if err := json.Unmarshal(doc.Content, &plugin); err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	var fns []any
	for _, g := range grants {
		id, err := gen.New()
		if err != nil {
			t.Fatal(err)
		}
		grant := map[string]any{"id": id, "function": g[0]}
		if g[1] != "" {
			grant["alias"] = g[1]
		}
		fns = append(fns, grant)
	}
	plugin["capabilities"].(map[string]any)["functions"] = fns
	content, _ := json.Marshal(plugin)
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, f.loans, "grant", path, content, doc.Revision); err != nil {
		t.Fatal(err)
	}
}

// deployed describes a function version a test stores.
type deployed struct {
	name      string
	version   int32
	placement string
	passed    bool
	alias     string
}

// moduleOf is the module hash a test version carries.
func moduleOf(d deployed) []byte {
	sum := sha256.Sum256([]byte(d.name + "@" + string(rune('0'+d.version))))
	return sum[:]
}

// deploy stores function versions (and aliases) in the development
// environment, as a build and a deploy would have.
func (f *fixture) deploy(t *testing.T, versions ...deployed) {
	t.Helper()
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	env := f.envs["development"].ID
	err := f.db.InTx(context.Background(), storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		for _, d := range versions {
			source, _ := gen.New()
			build, _ := gen.New()
			sum := moduleOf(d)
			steps := []struct {
				sql  string
				args []any
			}{
				{`INSERT INTO function_sources (id, organization_id, app_id, environment_id, name, object_key, sha256, size_bytes)
				  VALUES ($1, $2, $3, $4, $5, 'src', $6, 1)`, []any{source, f.org, f.app, env, d.name, sum}},
				{
					`INSERT INTO function_builds (id, organization_id, app_id, environment_id, source_id, name, placement, state, module_key,
				     module_sha256, abi_version, functions_manifest, provenance, statement, signature, key_id)
				  VALUES ($1, $2, $3, $4, $5, $6, $7, 'succeeded', 'mod', $8, 1, '{}', '{}', '\x01', '\x01', 'k')`,
					[]any{build, f.org, f.app, env, source, d.name, d.placement, sum},
				},
				{
					`INSERT INTO function_versions (organization_id, app_id, environment_id, name, version, build_id, placement, module_key,
				     module_sha256, abi_version, input_schema, output_schema, provenance, statement, signature, key_id, tests_passed)
				  VALUES ($1, $2, $3, $4, $5, $6, $7, 'mod', $8, 1, '{}', '{}', '{}', '\x01', '\x01', 'k', $9)`,
					[]any{f.org, f.app, env, d.name, d.version, build, d.placement, sum, d.passed},
				},
			}
			for _, s := range steps {
				if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
					return err
				}
			}
			if d.alias != "" {
				if _, err := tx.Exec(ctx, `INSERT INTO function_aliases (organization_id, app_id, environment_id, name, alias, version)
				  VALUES ($1, $2, $3, $4, $5, $6)`, f.org, f.app, env, d.name, d.alias, d.version); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
}

// publishAll publishes the app and the loans plugin.
func (f *fixture) publishAll(t *testing.T) {
	t.Helper()
	f.publish(t, "", false)
	f.publish(t, f.loans, false)
}

func wantPinned(t *testing.T, got []release.ReleaseFunction, want ...deployed) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("pinned %+v, want %d functions", got, len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.Name != w.name || g.Version != w.version || g.Placement != w.placement || g.ModuleSHA256 != hex.EncodeToString(moduleOf(w)) {
			t.Errorf("pinned[%d] = %+v, want %+v", i, g, w)
		}
	}
}

// Verifies: FN-006.
// A release resolves every function its plugins grant when it is created
// (an alias, or the newest version) and keeps what it resolved: later
// versions and moved aliases change neither the release nor a rollback to
// it.
func TestReleasePinsFunctions_FN_006(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	dev := f.envs["development"].ID
	f.grantFunctions(t, [][2]string{{"loans.quote", ""}, {"loans.rate", "stable"}, {"loans.score", ""}})
	f.deploy(t,
		deployed{"loans.quote", 1, "server", true, ""}, deployed{"loans.quote", 2, "server", true, ""},
		deployed{"loans.rate", 1, "server", true, "stable"}, deployed{"loans.rate", 2, "server", true, ""},
		deployed{"loans.score", 1, "device", true, ""})
	f.publishAll(t)

	first, diags, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "First")
	if err != nil || diags.HasErrors() {
		t.Fatalf("CreateRelease: %v %v", diags, err)
	}
	pinned := []deployed{{"loans.quote", 2, "server", true, ""}, {"loans.rate", 1, "server", true, ""}, {"loans.score", 1, "device", true, ""}}
	wantPinned(t, first.Functions, pinned...)

	got, _, err := f.rel.GetRelease(ctx, f.viewer, f.app, first.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	wantPinned(t, got.Functions, pinned...)

	// A newer version and a moved alias do not change a release made.
	f.deploy(t, deployed{"loans.quote", 3, "server", true, ""})
	err = f.db.InTx(ctx, storage.Tenant{OrganizationID: f.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE function_aliases SET version = 2 WHERE name = 'loans.rate'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "Second")
	if err != nil {
		t.Fatal(err)
	}
	wantPinned(t, second.Functions, deployed{"loans.quote", 3, "server", true, ""}, deployed{"loans.rate", 2, "server", true, ""}, pinned[2])
	listed, err := f.rel.ListReleases(ctx, f.viewer, f.app, "", 0, 10)
	if err != nil || len(listed) != 2 {
		t.Fatalf("ListReleases: %v %v", listed, err)
	}
	wantPinned(t, listed[1].Functions, pinned...)

	back, err := f.rel.RollbackRelease(ctx, f.owner, f.app, dev, "", first.Sequence, "")
	if err != nil {
		t.Fatal(err)
	}
	wantPinned(t, back.Functions, pinned...)
}

// Verifies: FN-006.
// A release is refused, and nothing of it recorded, when a function it
// references is not deployed or has no such alias. Failing tests refuse a
// release only in a production environment (FN-031), so the development
// release here is not refused for them.
func TestReleaseRefusesUnresolvableFunctions_FN_006(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		grants [][2]string
		deploy []deployed
		want   plxerr.Code
	}{
		{"not deployed", [][2]string{{"loans.quote", ""}}, nil, plxerr.FunctionNotFound},
		{"no such alias", [][2]string{{"loans.quote", "stable"}}, []deployed{{"loans.quote", 1, "server", true, ""}}, plxerr.FunctionAliasNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			ctx := context.Background()
			f.grantFunctions(t, c.grants)
			f.deploy(t, c.deploy...)
			f.publishAll(t)
			_, _, err := f.rel.CreateRelease(ctx, f.owner, f.app, f.envs["development"].ID, nil, "")
			if code(err) != c.want {
				t.Fatalf("CreateRelease = %v, want %s", err, c.want)
			}
			if rs, err := f.rel.ListReleases(ctx, f.viewer, f.app, "", 0, 10); err != nil || len(rs) != 0 {
				t.Errorf("a refused release was recorded: %v %v", rs, err)
			}
		})
	}
}

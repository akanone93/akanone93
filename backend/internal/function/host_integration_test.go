// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// secrets is the environment's server-held values, by name.
type secrets map[string]string

func (s secrets) Secret(_ context.Context, _, _, _, name string) (string, bool, error) {
	v, ok := s[name]
	return v, ok, nil
}

// declare stores the declaration of a build, as the builder's document
// step does.
func (w *world) declare(t *testing.T, buildID, declaration string) {
	t.Helper()
	err := w.db.InTx(context.Background(), storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO function_declarations (build_id, organization_id, declaration) VALUES ($1, $2, $3::jsonb)`, buildID, w.org, declaration)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// deployed builds, declares and deploys a function in an environment of
// the world's app.
func (w *world) deployed(t *testing.T, env, name, declaration string) function.Version {
	t.Helper()
	w.builder.set(goodOutput(name), nil)
	b := w.built(t, env, name)
	w.declare(t, b.ID, declaration)
	v, err := w.svc.Deploy(context.Background(), w.developer, b.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// grantFor issues a grant for a deployed version.
func (w *world) grantFor(t *testing.T, g *function.Grants, env string, v function.Version) function.Grant {
	t.Helper()
	token, _, err := g.Issue(function.GrantClaims{
		OrganizationID: w.org, AppID: w.app, EnvironmentID: w.envs[env], Function: v.Name, Version: v.Version,
		ModuleSHA256: function.HexSHA256(v.ModuleSHA256),
	}, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func newHost(t *testing.T, w *world, clock func() time.Time, s function.SecretReader) (*function.Host, *function.Grants) {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	grants, err := function.NewGrants(key, clock, nil)
	if err != nil {
		t.Fatal(err)
	}
	host, err := function.NewHost(function.HostOptions{Service: w.svc, Grants: grants, Secrets: s})
	if err != nil {
		t.Fatal(err)
	}
	return host, grants
}

const declaredKV = `{"name":"payments.charge","placement":"server","capabilities":{"kv":true,"secrets":["API_KEY","UNSET"]}}`

// Verifies: FN-009, FN-012.
// The host serves the grant's own module with the build's signed
// statement and nothing else, and refuses an expired or foreign grant.
func TestHostModule_FN_012(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	now := time.Now()
	host, grants := newHost(t, w, func() time.Time { return now }, nil)
	v := w.deployed(t, "development", fnName, declaredKV)
	other := w.deployed(t, "development", "payments.refund", declaredKV)
	grant := w.grantFor(t, grants, "development", v)

	a, err := host.GetModule(ctx, grant, function.HexSHA256(v.ModuleSHA256))
	if err != nil {
		t.Fatalf("GetModule: %v", err)
	}
	if !bytes.Equal(a.Module, []byte("\x00asm module of "+fnName)) || !bytes.Equal(a.Statement, v.Statement) || !bytes.Equal(a.Signature, v.Signature) || a.KeyID != v.KeyID || a.KeyID == "" {
		t.Errorf("artifact = %q %q %q %q", a.Module, a.Statement, a.Signature, a.KeyID)
	}
	if _, err := host.GetModule(ctx, grant, function.HexSHA256(other.ModuleSHA256)); code(err) != plxerr.PermissionDenied {
		t.Errorf("another function's module: %v", err)
	}
	if _, err := host.GetModule(ctx, grant, "nothex"); code(err) != plxerr.PermissionDenied {
		t.Errorf("a malformed hash: %v", err)
	}
	if _, err := host.GetModule(ctx, grant+"x", function.HexSHA256(v.ModuleSHA256)); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("a tampered grant: %v", err)
	}
	now = now.Add(time.Minute)
	if _, err := host.GetModule(ctx, grant, function.HexSHA256(v.ModuleSHA256)); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("an expired grant: %v", err)
	}
}

// Verifies: FN-012, FN-018.
// The key-value namespace is the grant's: the same key under another
// function or another environment is another value, and a function that
// does not declare kv has no store.
func TestHostKeyValueIsolation_FN_018(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	host, grants := newHost(t, w, nil, nil)
	mine := w.grantFor(t, grants, "development", w.deployed(t, "development", fnName, declaredKV))
	sibling := w.grantFor(t, grants, "development", w.deployed(t, "development", "payments.refund", declaredKV))
	staging := w.grantFor(t, grants, "staging", w.deployed(t, "staging", fnName, declaredKV))
	closed := w.grantFor(t, grants, "development", w.deployed(t, "development", "payments.audit", `{"name":"payments.audit","placement":"server"}`))

	if err := host.KVPut(ctx, mine, "counter", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if v, found, err := host.KVGet(ctx, mine, "counter"); err != nil || !found || string(v) != "1" {
		t.Errorf("own key = %q %v %v", v, found, err)
	}
	for name, g := range map[string]function.Grant{"another function": sibling, "another environment": staging} {
		if _, found, err := host.KVGet(ctx, g, "counter"); err != nil || found {
			t.Errorf("%s sees the key: %v %v", name, found, err)
		}
		if deleted, err := host.KVDelete(ctx, g, "counter"); err != nil || deleted {
			t.Errorf("%s deleted the key: %v %v", name, deleted, err)
		}
	}
	if _, found, _ := host.KVGet(ctx, mine, "counter"); !found {
		t.Error("another namespace's delete removed the key")
	}
	if _, _, err := host.KVGet(ctx, closed, "counter"); code(err) != plxerr.FunctionCapabilityDenied {
		t.Errorf("a function without kv: %v", err)
	}
	if err := host.KVPut(ctx, closed, "counter", []byte("1")); code(err) != plxerr.FunctionCapabilityDenied {
		t.Errorf("a write without kv: %v", err)
	}
	if deleted, err := host.KVDelete(ctx, mine, "counter"); err != nil || !deleted {
		t.Errorf("own delete: %v %v", deleted, err)
	}
}

// Verifies: FN-012, SEC-092.
// A secret is served by declared name only; a name the declaration does not
// carry is PLX-7003, and a declared name the environment does not hold is
// reported as not found.
func TestHostSecrets_FN_012(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t, nil, nil)
	host, grants := newHost(t, w, nil, secrets{"API_KEY": "s3cret", "OTHER": "x"})
	g := w.grantFor(t, grants, "development", w.deployed(t, "development", fnName, declaredKV))

	if v, found, err := host.Secret(ctx, g, "API_KEY"); err != nil || !found || string(v) != "s3cret" {
		t.Errorf("declared secret = %q %v %v", v, found, err)
	}
	if _, found, err := host.Secret(ctx, g, "UNSET"); err != nil || found {
		t.Errorf("declared but unset: %v %v", found, err)
	}
	for _, name := range []string{"OTHER", "", "api_key", "API_KEY "} {
		if _, found, err := host.Secret(ctx, g, name); code(err) != plxerr.FunctionCapabilityDenied || found {
			t.Errorf("undeclared %q: %v %v", name, found, err)
		}
	}
}

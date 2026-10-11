// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/storagetest"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

type ids struct{ g *uuid7.Generator }

func (i ids) New() (string, error) {
	u, err := i.g.New()
	return u.String(), err
}

// world is an organisation with an owner, a viewer and an app, against a
// fresh schema.
type world struct {
	db       *storage.DB
	tenancy  *tenancy.Service
	registry *gateway.Registry
	org      string
	owner    auth.Principal
	viewer   auth.Principal
	app      string
	envs     map[string]string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	ctx := context.Background()
	db := storagetest.Open(t)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gen := ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
	log := audit.NewLog(gen, nil)
	authService, err := auth.NewService(auth.Options{DB: db, Audit: log, Cache: cache.NewMemory(nil), Crypter: backend, IDs: gen, VerificationURI: "https://p.example/device"})
	if err != nil {
		t.Fatal(err)
	}
	w := &world{db: db, envs: map[string]string{}}
	if w.tenancy, err = tenancy.NewService(tenancy.Options{DB: db, Audit: log, Auth: authService, Crypter: backend, IDs: gen, SigningKeyPrefix: "targets"}); err != nil {
		t.Fatal(err)
	}
	if w.registry, err = gateway.NewRegistry(gateway.RegistryOptions{DB: db, Audit: log, IDs: gen}); err != nil {
		t.Fatal(err)
	}
	admin, invitation, err := authService.Bootstrap(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authService.AcceptInvitation(ctx, invitation, "Admin", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	adminID := auth.Identity{Kind: auth.KindUser, ID: admin.ID, UserID: admin.ID, Display: "Admin", SecondFactor: true, InstallationAdmin: true}
	o, err := w.tenancy.CreateOrganization(ctx, adminID, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	w.org = o.ID
	if w.owner, err = authService.Resolve(ctx, adminID, w.org); err != nil {
		t.Fatal(err)
	}
	a, err := w.tenancy.CreateApp(ctx, w.owner, "demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	w.app = a.ID
	envs, err := w.tenancy.ListEnvironments(ctx, w.owner, w.app, tenancy.Page{Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range envs {
		w.envs[e.Key] = e.ID
	}
	m, _, err := w.tenancy.AddMember(ctx, w.owner, "", "viewer@example.com", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if w.viewer, err = authService.Resolve(ctx, auth.Identity{Kind: auth.KindUser, ID: m.UserID, UserID: m.UserID, Display: "v", SecondFactor: true}, w.org); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w *world) upstream(env, name string) gateway.Upstream {
	return gateway.Upstream{
		AppID: w.app, EnvironmentID: w.envs[env], Name: name, BaseURL: "https://api.bank.example/v1",
		Auth: gateway.Auth{Kind: gateway.AuthAPIKey, Header: "X-Api-Key", KeySecret: "bank_key"}, Methods: []string{"GET", "POST"},
		Operations: []gateway.Operation{{
			Name: "pay", Methods: []string{"POST"}, PathPrefix: "/payments", Assurance: "AL2",
			Transaction: &gateway.Transaction{Amount: "/amount", Currency: "/currency", Payee: "/payee"},
		}},
	}
}

func code(err error) plxerr.Code {
	c, _ := plxerr.CodeOf(err)
	return c
}

// Verifies: SEC-031, SEC-140.
// An upstream is registered, listed, changed and removed by those who
// manage the app, each change is audited, and a viewer can read but not
// change; the audit never carries a credential.
func TestRegistryLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	u, err := w.registry.Create(ctx, w.owner, w.upstream("production", "bank"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == "" || u.Version != 1 || u.Operations[0].Transaction.Payee != "/payee" || u.Assurance != "AL0" {
		t.Errorf("created = %+v", u)
	}
	if _, err := w.registry.Create(ctx, w.owner, w.upstream("production", "bank")); code(err) != plxerr.ResourceExists {
		t.Errorf("the same name twice: %v", err)
	}
	if _, err := w.registry.Create(ctx, w.viewer, w.upstream("production", "other")); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer registering: %v", err)
	}
	bad := w.upstream("production", "bad")
	bad.BaseURL = "http://api.bank.example"
	if _, err := w.registry.Create(ctx, w.owner, bad); code(err) != plxerr.InvalidFormat {
		t.Errorf("a cleartext upstream: %v", err)
	}
	if _, err := w.registry.Create(ctx, w.owner, w.upstream("nonexistent", "bank")); err == nil {
		t.Error("an environment of no app")
	}

	got, err := w.registry.Get(ctx, w.viewer, u.ID)
	if err != nil || got.Name != "bank" {
		t.Errorf("a viewer reading: %v %+v", err, got)
	}
	if _, err := w.registry.Create(ctx, w.owner, w.upstream("development", "bank")); err != nil {
		t.Fatalf("the same name in another environment: %v", err)
	}
	all, err := w.registry.List(ctx, w.viewer, w.app, "", storage.Cursor{}, 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("List: %v %d", err, len(all))
	}
	one, err := w.registry.List(ctx, w.viewer, w.app, w.envs["production"], storage.Cursor{}, 10)
	if err != nil || len(one) != 1 {
		t.Fatalf("List of one environment: %v %d", err, len(one))
	}
	next, err := w.registry.List(ctx, w.viewer, w.app, "", storage.Cursor{ID: all[0].EnvironmentID, Key: all[0].Name}, 10)
	if err != nil || len(next) != 1 || next[0].ID != all[1].ID {
		t.Errorf("the page after the first: %v %+v", err, next)
	}

	u.Timeout = 3 * time.Second
	u.Operations = nil
	changed, err := w.registry.Update(ctx, w.owner, u)
	if err != nil || changed.Version != 2 || changed.Timeout != 3*time.Second || len(changed.Operations) != 0 {
		t.Errorf("Update: %v %+v", err, changed)
	}
	moved := u
	moved.EnvironmentID = w.envs["development"]
	if _, err := w.registry.Update(ctx, w.owner, moved); code(err) != plxerr.InvalidFormat {
		t.Errorf("moving to another environment: %v", err)
	}
	if _, err := w.registry.Update(ctx, w.viewer, u); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer updating: %v", err)
	}

	resolved, err := w.registry.Resolve(ctx, w.org, w.envs["production"], "bank")
	if err != nil || resolved.ID != u.ID {
		t.Errorf("Resolve: %v %+v", err, resolved)
	}
	if _, err := w.registry.Resolve(ctx, w.org, w.envs["production"], "nope"); code(err) != plxerr.GatewayUpstreamUnknown {
		t.Errorf("an unknown name: %v", err)
	}
	if _, err := w.registry.Resolve(ctx, "00000000-0000-7000-8000-000000000000", w.envs["production"], "bank"); code(err) != plxerr.GatewayUpstreamUnknown {
		t.Errorf("another organisation reading the upstream: %v", err)
	}

	if err := w.registry.Delete(ctx, w.viewer, u.ID); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer deleting: %v", err)
	}
	if err := w.registry.Delete(ctx, w.owner, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := w.registry.Resolve(ctx, w.org, w.envs["production"], "bank"); code(err) != plxerr.GatewayUpstreamUnknown {
		t.Errorf("a deleted upstream still resolves: %v", err)
	}
	var entries int
	err = w.db.InTx(ctx, storage.Tenant{OrganizationID: w.org}, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = ANY($1)`,
			[]string{string(audit.UpstreamCreated), string(audit.UpstreamUpdated), string(audit.UpstreamDeleted)}).Scan(&entries)
	})
	if err != nil || entries != 4 {
		t.Errorf("audit entries for the changes = %d, %v (two creations, an update, a deletion)", entries, err)
	}
}

// Verifies: SEC-026.
// An environment has at most one identity provider, which can be read,
// replaced and removed by those who manage the app.
func TestIdentityProviderLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	idp := gateway.IdentityProvider{
		AppID: w.app, EnvironmentID: w.envs["production"], Issuer: "https://login.example", JWKSURL: "https://login.example/keys",
		Audiences: []string{"app"}, ForwardClaims: []string{"email"},
	}
	if _, err := w.registry.GetProvider(ctx, w.owner, w.app, idp.EnvironmentID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("none set: %v", err)
	}
	if _, err := w.registry.ProviderFor(ctx, w.org, idp.EnvironmentID); code(err) != plxerr.PreconditionFailed {
		t.Errorf("none set, for a device: %v", err)
	}
	if err := w.registry.SetProvider(ctx, w.viewer, idp); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer setting: %v", err)
	}
	if err := w.registry.SetProvider(ctx, w.owner, idp); err != nil {
		t.Fatalf("SetProvider: %v", err)
	}
	idp.Issuer = "https://login2.example"
	if err := w.registry.SetProvider(ctx, w.owner, idp); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, err := w.registry.ProviderFor(ctx, w.org, idp.EnvironmentID)
	if err != nil || got.Issuer != idp.Issuer || got.ForwardClaims[0] != "email" {
		t.Errorf("ProviderFor: %v %+v", err, got)
	}
	if err := w.registry.DeleteProvider(ctx, w.owner, w.app, idp.EnvironmentID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if _, err := w.registry.ProviderFor(ctx, w.org, idp.EnvironmentID); code(err) != plxerr.PreconditionFailed {
		t.Errorf("after the delete: %v", err)
	}
}

// Verifies: SEC-106, SEC-031.
// The gateway opens the secret an upstream names, in the upstream's
// organisation and no other.
func TestGatewayOpensEnvironmentSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	if _, err := w.tenancy.SetSecret(ctx, w.owner, w.envs["production"], "bank_key", "s3cret-value"); err != nil {
		t.Fatal(err)
	}
	got, err := w.tenancy.OpenEnvironmentSecret(ctx, w.org, w.envs["production"], "bank_key")
	if err != nil || string(got) != "s3cret-value" {
		t.Fatalf("OpenEnvironmentSecret: %v %q", err, got)
	}
	if _, err := w.tenancy.OpenEnvironmentSecret(ctx, w.org, w.envs["production"], "other"); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a secret that does not exist: %v", err)
	}
	if _, err := w.tenancy.OpenEnvironmentSecret(ctx, "00000000-0000-7000-8000-000000000000", w.envs["production"], "bank_key"); err == nil {
		t.Error("another organisation opened the secret")
	}
	var _ gateway.SecretReader = secretsOf{w.tenancy}
}

// secretsOf adapts the tenancy service the way the server does.
type secretsOf struct{ t *tenancy.Service }

func (s secretsOf) OpenSecret(ctx context.Context, org, env, key string) ([]byte, error) {
	return s.t.OpenEnvironmentSecret(ctx, org, env, key)
}

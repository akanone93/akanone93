// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// federated is the governance organisation with an OpenID Connect provider
// of its own: the test IdP stands in for the people's identity provider.
type federated struct {
	*governance
	idp      *idp
	provider auth.Provider
}

// newFederated builds the governance organisation whose service reaches the
// test IdP through its client, so that its certificate is trusted.
func newFederated(t *testing.T) *federated {
	t.Helper()
	p := newIDP(t)
	trust := func(o *auth.Options) { o.HTTPClient = p.srv.Client() }
	g := newGovernance(t, trust)
	return &federated{governance: g, idp: p}
}

// spec is the organisation's OpenID Connect provider, with the client secret
// the IdP checks.
func (f *federated) spec(jit bool) auth.ProviderSpec {
	return auth.ProviderSpec{
		Key: "acme-sso", Kind: auth.ProviderOIDC, Name: "Acme SSO", Enabled: true, JIT: jit,
		OIDC:         &auth.OIDCSettings{Issuer: f.idp.srv.URL, ClientID: "studio", RedirectURL: "https://plux.example/auth/callback"},
		ClientSecret: "s3cret",
	}
}

// ownerScope is the provider scope of the organisation's owner.
func (f *federated) ownerScope(t *testing.T) auth.ProviderScope {
	t.Helper()
	sc, err := auth.OrganizationProviders(f.owner)
	if err != nil {
		t.Fatalf("OrganizationProviders: %v", err)
	}
	return sc
}

// createProvider configures the organisation's provider and keeps it.
func (f *federated) createProvider(t *testing.T) auth.Provider {
	t.Helper()
	p, err := f.svc.CreateProvider(context.Background(), f.ownerScope(t), f.spec(false))
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	f.provider = p
	return p
}

// setJIT switches just-in-time provisioning on or off, keeping the stored
// client secret.
func (f *federated) setJIT(t *testing.T, on bool) {
	t.Helper()
	spec := f.spec(on)
	spec.ClientSecret = ""
	if _, err := f.svc.UpdateProvider(context.Background(), f.ownerScope(t), f.provider.ID, spec); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
}

// setMapping replaces the organisation's provider's mapping document.
func (f *federated) setMapping(t *testing.T, m auth.GroupMapping) {
	t.Helper()
	if _, err := f.svc.SetMapping(context.Background(), f.ownerScope(t), f.provider.ID, m); err != nil {
		t.Fatalf("SetMapping: %v", err)
	}
}

// federatedSignIn signs a person in through the organisation's provider. The
// extra claims are added to the ID token before the code is redeemed.
func (f *federated) federatedSignIn(t *testing.T, subject, email string, verified bool, extra map[string]any) (auth.Session, auth.Challenge, error) {
	t.Helper()
	ctx := context.Background()
	login, err := f.svc.StartProviderLogin(ctx, f.provider.ID)
	if err != nil {
		t.Fatalf("StartProviderLogin: %v", err)
	}
	code := f.idp.authorize(t, login, subject, email, verified, "")
	f.idp.addClaims(code, extra)
	return f.svc.CompleteOIDCLogin(ctx, login.State, code)
}

// mustSignIn is federatedSignIn for a sign-in that must yield a session.
func (f *federated) mustSignIn(t *testing.T, subject, email string, extra map[string]any) auth.Session {
	t.Helper()
	session, challenge, err := f.federatedSignIn(t, subject, email, true, extra)
	if err != nil || session.Secret == "" || challenge.Secret != "" {
		t.Fatalf("federated sign-in of %s = %+v, %+v, %v", email, session, challenge, err)
	}
	return session
}

// addClaims adds claims to the ID token that a code redeems to.
func (p *idp) addClaims(code string, extra map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range extra {
		p.claims[code][k] = v
	}
}

// bootstrapAdmin makes the installation's first administrator, who has
// accepted the invitation and signed in without a second factor.
func bootstrapAdmin(t *testing.T, f *fixture) auth.Identity {
	t.Helper()
	ctx := context.Background()
	_, invitation, err := f.svc.Bootstrap(ctx, "root@example.com")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := f.svc.AcceptInvitation(ctx, invitation, "Root", password); err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	return f.signIn(t, "root@example.com")
}

// auditEntries lists the audit chain of a tenant: an organisation's, or the
// installation's when the organisation is empty.
func auditEntries(t *testing.T, f *fixture, organizationID string) []audit.Entry {
	t.Helper()
	tenant := storage.Tenant{Scope: storage.ScopeInstallation}
	if organizationID != "" {
		tenant = storage.Tenant{OrganizationID: organizationID}
	}
	var entries []audit.Entry
	if err := f.db.InTx(context.Background(), tenant, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		entries, err = f.log.List(ctx, tx, organizationID, 0, 200)
		return err
	}); err != nil {
		t.Fatalf("audit list: %v", err)
	}
	return entries
}

// scanIn scans the one row a query returns in a transaction of the tenant, so
// that row-level security applies to the read as it does to the service.
func scanIn(t *testing.T, f *fixture, tenant storage.Tenant, query string, dest []any, args ...any) {
	t.Helper()
	if err := f.db.InTx(context.Background(), tenant, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(dest...)
	}); err != nil {
		t.Fatalf("scan %q: %v", query, err)
	}
}

// hasAudit reports whether a chain holds an entry with the action.
func hasAudit(entries []audit.Entry, action audit.Action) bool {
	for _, e := range entries {
		if e.Action == action {
			return true
		}
	}
	return false
}

// Verifies: GOV-004, SEC-092.
// An organisation's providers are administered with sso.manage and only by
// that organisation: a viewer is refused, another organisation's provider is
// not found, and the client secret is stored sealed and never returned. Each
// change is audited.
func TestProviderCRUD_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	ctx := context.Background()
	if _, err := auth.OrganizationProviders(f.bob); code(err) != plxerr.PermissionDenied {
		t.Errorf("a viewer administers providers: %v", err)
	}
	sc := f.ownerScope(t)
	noSecret := f.spec(false)
	noSecret.ClientSecret = ""
	if _, err := f.svc.CreateProvider(ctx, sc, noSecret); code(err) != plxerr.InvalidStructure {
		t.Errorf("a provider without a client secret: %v", err)
	}
	if _, err := f.svc.CreateProvider(ctx, sc, auth.ProviderSpec{Key: "Not A Key", Kind: auth.ProviderOIDC, Name: "x"}); code(err) != plxerr.InvalidFormat {
		t.Errorf("a provider with a malformed key: %v", err)
	}
	p := f.createProvider(t)
	if _, err := f.svc.CreateProvider(ctx, sc, f.spec(false)); code(err) != plxerr.ResourceExists {
		t.Errorf("a second provider with the same key: %v", err)
	}
	if p.Kind != auth.ProviderOIDC || p.OIDC == nil || !p.OIDC.HasSecret || p.OIDC.Issuer != f.idp.srv.URL || !p.Enabled || p.JIT {
		t.Errorf("created provider = %+v", p)
	}
	if got, err := f.svc.GetProvider(ctx, sc, p.ID); err != nil || got.ID != p.ID || got.OrganizationID != f.org {
		t.Errorf("GetProvider = %+v, %v", got, err)
	}
	listed, err := f.svc.ListProviders(ctx, sc, "", 10)
	if err != nil || len(listed) != 1 || listed[0].ID != p.ID {
		t.Errorf("ListProviders = %+v, %v", listed, err)
	}
	f.updateName(t, p.ID)
	f.assertOtherOrganisationCannotSee(t, p.ID)
	f.assertNoSecretLeaks(t, p.ID)
	if err := f.svc.DeleteProvider(ctx, sc, p.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if _, err := f.svc.GetProvider(ctx, sc, p.ID); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a deleted provider: %v", err)
	}
	entries := auditEntries(t, f.fixture, f.org)
	for _, a := range []audit.Action{audit.ProviderCreated, audit.ProviderUpdated, audit.ProviderDeleted} {
		if !hasAudit(entries, a) {
			t.Errorf("no %s entry in the organisation's chain", a)
		}
	}
}

// updateName renames the provider, which keeps its stored secret.
func (f *federated) updateName(t *testing.T, id string) {
	t.Helper()
	spec := f.spec(false)
	spec.ClientSecret = ""
	spec.Name = "Acme Single Sign-On"
	updated, err := f.svc.UpdateProvider(context.Background(), f.ownerScope(t), id, spec)
	if err != nil || updated.Name != "Acme Single Sign-On" || !updated.OIDC.HasSecret {
		t.Fatalf("UpdateProvider = %+v, %v", updated, err)
	}
	spec.Kind = auth.ProviderSAML
	if _, err := f.svc.UpdateProvider(context.Background(), f.ownerScope(t), id, spec); code(err) != plxerr.InvalidEnumValue {
		t.Errorf("changing a provider's kind: %v", err)
	}
}

// assertOtherOrganisationCannotSee checks that an organisation's owner
// cannot read, change or delete another organisation's provider, and that
// the provider is not listed there.
func (f *federated) assertOtherOrganisationCannotSee(t *testing.T, id string) {
	t.Helper()
	ctx := context.Background()
	other := f.organisation(t, "other", map[string]auth.Role{f.owner.UserID: auth.RoleOwner})
	sc, err := auth.OrganizationProviders(f.principalOf(t, "ada@example.com", other))
	if err != nil {
		t.Fatalf("OrganizationProviders(other): %v", err)
	}
	if _, err := f.svc.GetProvider(ctx, sc, id); code(err) != plxerr.ResourceNotFound {
		t.Errorf("another organisation reads a provider: %v", err)
	}
	if _, err := f.svc.UpdateProvider(ctx, sc, id, f.spec(true)); code(err) != plxerr.ResourceNotFound {
		t.Errorf("another organisation changes a provider: %v", err)
	}
	if err := f.svc.DeleteProvider(ctx, sc, id); code(err) != plxerr.ResourceNotFound {
		t.Errorf("another organisation deletes a provider: %v", err)
	}
	if listed, err := f.svc.ListProviders(ctx, sc, "", 10); err != nil || len(listed) != 0 {
		t.Errorf("another organisation lists %+v, %v", listed, err)
	}
}

// assertNoSecretLeaks checks that the client secret is not in the provider's
// view, its stored settings or its audit entries.
func (f *federated) assertNoSecretLeaks(t *testing.T, id string) {
	t.Helper()
	ctx := context.Background()
	got, err := f.svc.GetProvider(ctx, f.ownerScope(t), id)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "s3cret") {
		t.Errorf("a provider's view carries the client secret: %+v", got)
	}
	var config string
	var sealed []byte
	scanIn(t, f.fixture, storage.Tenant{OrganizationID: f.org}, `SELECT config::text, secret FROM identity_providers WHERE id = $1`,
		[]any{&config, &sealed}, id)
	if strings.Contains(config, "s3cret") || bytes.Contains(sealed, []byte("s3cret")) || len(sealed) == 0 {
		t.Errorf("the client secret is stored in the clear or not at all")
	}
	for _, e := range auditEntries(t, f.fixture, f.org) {
		if strings.Contains(e.Detail, "s3cret") {
			t.Errorf("audit entry %s carries the client secret", e.Action)
		}
	}
}

// Verifies: GOV-004, SEC-092.
// The installation's providers are administered by an installation
// administrator who presented a second factor. An ordinary member is refused
// and so is an administrator without the factor.
func TestInstallationProviders_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	ctx := context.Background()
	f.person(t, "ada@example.com")
	ada := f.signIn(t, "ada@example.com")
	if _, err := auth.InstallationProviders(ada); code(err) != plxerr.PermissionDenied {
		t.Errorf("an ordinary person administers the installation's providers: %v", err)
	}
	admin := bootstrapAdmin(t, f)
	if _, err := auth.InstallationProviders(admin); code(err) != plxerr.MultiFactorRequired {
		t.Errorf("an administrator without a second factor: %v", err)
	}
	_, admin = f.enrol(t, admin)
	sc, err := auth.InstallationProviders(admin)
	if err != nil {
		t.Fatalf("InstallationProviders: %v", err)
	}
	spec := auth.ProviderSpec{
		Key: "shared-sso", Kind: auth.ProviderOIDC, Name: "Shared SSO", Enabled: true,
		OIDC:         &auth.OIDCSettings{Issuer: "https://idp.example", ClientID: "plux", RedirectURL: "https://plux.example/auth/callback"},
		ClientSecret: "s3cret",
	}
	p, err := f.svc.CreateProvider(ctx, sc, spec)
	if err != nil || p.OrganizationID != "" {
		t.Fatalf("CreateProvider = %+v, %v", p, err)
	}
	if got, err := f.svc.GetProvider(ctx, sc, p.ID); err != nil || got.Key != "shared-sso" {
		t.Errorf("GetProvider = %+v, %v", got, err)
	}
	if !hasAudit(auditEntries(t, f, ""), audit.ProviderCreated) {
		t.Error("no identity_provider.created entry in the installation's chain")
	}
}

// Verifies: GOV-004, SEC-100.
// A disabled provider is not an available sign-in: it starts none, and
// people cannot sign in through it.
func TestDisabledProviderIsNotASignInOption_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFederated(t)
	ctx := context.Background()
	f.createProvider(t)
	spec := f.spec(false)
	spec.ClientSecret = ""
	spec.Enabled = false
	if _, err := f.svc.UpdateProvider(ctx, f.ownerScope(t), f.provider.ID, spec); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if _, err := f.svc.StartProviderLogin(ctx, f.provider.ID); code(err) != plxerr.AuthenticationRequired {
		t.Errorf("a disabled provider starts a sign-in: %v", err)
	}
}

// Verifies: GOV-004, SEC-100.
// auth.studio.allowPasswordLogin may be false only once a provider of the
// installation exists. Until then password sign-in still works, so nobody is
// locked out, and the last installation provider cannot be removed while
// password sign-in is off.
func TestPasswordLoginPolicy_GOV_004(t *testing.T) {
	t.Parallel()
	f := newFixtureWith(t, nil, failedSignIns, func(o *auth.Options) { o.PasswordLoginDisabled = true })
	ctx := context.Background()
	f.person(t, "ada@example.com")
	if err := f.svc.CheckPasswordPolicy(ctx); err == nil {
		t.Error("password sign-in is off with no provider")
	}
	if _, _, err := f.svc.StartPasswordLogin(ctx, "ada@example.com", password); err != nil {
		t.Errorf("password sign-in refused with no provider: %v", err)
	}
	admin := bootstrapAdmin(t, f)
	_, admin = f.enrol(t, admin)
	sc, err := auth.InstallationProviders(admin)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.svc.CreateProvider(ctx, sc, auth.ProviderSpec{
		Key: "shared-sso", Kind: auth.ProviderOIDC, Name: "Shared SSO", Enabled: true,
		OIDC:         &auth.OIDCSettings{Issuer: "https://idp.example", ClientID: "plux", RedirectURL: "https://plux.example/auth/callback"},
		ClientSecret: "s3cret",
	})
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if err := f.svc.CheckPasswordPolicy(ctx); err != nil {
		t.Errorf("password sign-in off with a provider: %v", err)
	}
	if _, _, err := f.svc.StartPasswordLogin(ctx, "ada@example.com", password); code(err) != plxerr.PreconditionFailed {
		t.Errorf("password sign-in with a provider: %v", err)
	}
	if err := f.svc.DeleteProvider(ctx, sc, p.ID); code(err) != plxerr.PreconditionFailed {
		t.Errorf("the last provider removed while password sign-in is off: %v", err)
	}
}

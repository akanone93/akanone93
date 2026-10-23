// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file is what every federated sign-in shares, whether the provider
// speaks OpenID Connect or SAML (GOV-004, ADR-0074): the provider has
// authenticated a person; Plux decides who they are. An identity is linked
// to an account by the provider's issuer and subject; a first sign-in links
// it by the address the provider vouches for, and, where the provider says
// so, creates the account just in time. The provider's group mapping then
// makes the person's grants and teams match their groups, and the sign-in
// is recorded with the authentication context the provider reported.

// federatedHTTPTimeout bounds a request to a provider's endpoints.
const federatedHTTPTimeout = 15 * time.Second

// cachedOIDC is an OpenID Connect provider built from its stored settings.
// It is rebuilt when the provider is changed.
type cachedOIDC struct {
	updated time.Time
	p       *OIDCProvider
}

// httpClient is the SSRF-safe client for provider endpoints (SEC-105).
func (s *Service) httpClient() (*http.Client, error) {
	if s.o.HTTPClient != nil {
		return s.o.HTTPClient, nil
	}
	client, err := httpx.NewClient(httpx.Options{Destination: httpx.KindOIDC, Timeout: federatedHTTPTimeout})
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	return client, nil
}

// forgetProvider drops a provider's cached runtime after a change.
func (s *Service) forgetProvider(id pgtype.UUID) {
	s.mu.Lock()
	delete(s.oidc, storage.ID(id))
	s.mu.Unlock()
}

// oidcTargetFor returns the provider a sign-in goes through. An empty
// providerID is the provider in the configuration file.
func (s *Service) oidcTargetFor(ctx context.Context, providerID string) (oidcTarget, error) {
	if providerID == "" {
		if err := s.oidcRequired(); err != nil {
			return oidcTarget{}, err
		}
		return oidcTarget{provider: s.o.OIDC}, nil
	}
	id, err := parseID(providerID, "provider")
	if err != nil {
		return oidcTarget{}, err
	}
	var row dbgen.IdentityProvider
	err = s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetIdentityProvider(ctx, id)
		return err //nolint:wrapcheck // examined below
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!row.Enabled || row.Kind != ProviderOIDC)) {
		return oidcTarget{}, plxerr.New(plxerr.AuthenticationRequired, "the sign-in provider is not available")
	}
	if err != nil {
		return oidcTarget{}, fmt.Errorf("auth: read a provider: %w", err)
	}
	p, err := s.oidcRuntime(ctx, row)
	if err != nil {
		return oidcTarget{}, err
	}
	return oidcTarget{provider: p, row: &row}, nil
}

// oidcRuntime builds, or returns the cached, OpenID Connect provider of a
// stored row.
func (s *Service) oidcRuntime(ctx context.Context, row dbgen.IdentityProvider) (*OIDCProvider, error) {
	key, updated := storage.ID(row.ID), storage.Time(row.UpdatedAt)
	s.mu.Lock()
	cached, ok := s.oidc[key]
	s.mu.Unlock()
	if ok && cached.updated.Equal(updated) {
		return cached.p, nil
	}
	var c storedOIDC
	if err := json.Unmarshal(row.Config, &c); err != nil {
		return nil, fmt.Errorf("auth: read a provider's settings: %w", err)
	}
	secret, err := s.openProviderSecret(ctx, row)
	if err != nil {
		return nil, err
	}
	client, err := s.httpClient()
	if err != nil {
		return nil, err
	}
	p, err := NewOIDCProvider(OIDCConfig{
		Issuer: c.Issuer, ClientID: c.ClientID, ClientSecret: string(secret), RedirectURL: c.RedirectURL, Client: client,
	}, s.now)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.PreconditionFailed, err, "the sign-in provider is misconfigured")
	}
	s.mu.Lock()
	s.oidc[key] = cachedOIDC{updated: updated, p: p}
	s.mu.Unlock()
	return p, nil
}

// SignInProvider is a provider as the sign-in page offers it.
type SignInProvider struct {
	// ID is empty for the provider in the configuration file.
	ID              string
	Key             string
	Kind            string
	Name            string
	OrganizationKey string
}

// ListSignInProviders lists what a person may choose to sign in with: the
// provider in the configuration file, the installation's enabled providers
// and, when an organisation is named, its own. An unknown organisation is
// answered like one without providers, so the call does not tell which
// organisations exist. SAML providers are listed only while the licence
// allows them.
func (s *Service) ListSignInProviders(ctx context.Context, organizationKey string, samlAllowed bool) ([]SignInProvider, error) {
	var out []SignInProvider
	if s.o.OIDC != nil {
		out = append(out, SignInProvider{Key: "default", Kind: ProviderOIDC, Name: "Single sign-on"})
	}
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		var org dbgen.Organization
		if organizationKey != "" {
			row, err := q.GetOrganizationByKey(ctx, organizationKey)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("auth: read an organisation: %w", err)
			}
			org = row
		}
		rows, err := q.ListSignInProviders(ctx, dbgen.ListSignInProvidersParams{OrganizationID: org.ID, PageSize: 200})
		if err != nil {
			return fmt.Errorf("auth: list the sign-in providers: %w", err)
		}
		for _, r := range rows {
			if r.Kind == ProviderSAML && !samlAllowed {
				continue
			}
			p := SignInProvider{ID: storage.ID(r.ID), Key: r.Key, Kind: r.Kind, Name: r.Name}
			if r.OrganizationID.Valid {
				p.OrganizationKey = organizationKey
			}
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

// externalLogin is a person a provider has authenticated, before Plux has
// decided who they are.
type externalLogin struct {
	// row is nil for the provider in the configuration file, which has no
	// mapping and does not provision.
	row           *dbgen.IdentityProvider
	issuer        string
	subject       string
	email         string
	emailVerified bool
	name          string
	groups        []string
	// sid, acr and amr are what the provider reported of the session and
	// the authentication.
	sid string
	acr string
	amr []string
}

// tenant is where the account is looked up: an organisation's provider may
// see only that organisation's members.
func (e externalLogin) tenant() storage.Tenant {
	t := storage.Tenant{Scope: storage.ScopeInstallation}
	if e.row != nil && e.row.OrganizationID.Valid {
		t.OrganizationID = storage.ID(e.row.OrganizationID)
	}
	return t
}

// signInExternal decides who a provider's person is and signs them in: a
// session, or a challenge when the account has a confirmed second factor.
func (s *Service) signInExternal(ctx context.Context, e externalLogin) (Session, Challenge, error) {
	user, ok, err := s.accountFor(ctx, e)
	if err != nil {
		return Session{}, Challenge{}, err
	}
	if !ok {
		return Session{}, Challenge{}, signInRefused()
	}
	if e.row != nil {
		if err := s.applyMapping(ctx, *e.row, user.ID, e.groups); err != nil {
			return Session{}, Challenge{}, err
		}
	}
	return s.finishExternal(ctx, e, user.ID)
}

// accountFor finds the account of a provider's person, linking it by
// verified address on a first sign-in and creating it when the provider
// provisions. It reports false when no account may be used.
func (s *Service) accountFor(ctx context.Context, e externalLogin) (dbgen.User, bool, error) {
	var (
		user    dbgen.User
		allowed bool
	)
	err := s.inTx(ctx, e.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		user, allowed, err = s.resolveAccount(ctx, tx, e)
		if err != nil || !allowed {
			return err
		}
		if user.DisabledAt.Valid {
			allowed = false
			return s.refusal(ctx, tx, user)
		}
		return nil
	})
	return user, allowed, err
}

// resolveAccount is the lookup and linking behind accountFor.
func (s *Service) resolveAccount(ctx context.Context, tx pgx.Tx, e externalLogin) (dbgen.User, bool, error) {
	q := dbgen.New(tx)
	link, err := q.GetUserIdentity(ctx, dbgen.GetUserIdentityParams{Issuer: e.issuer, Subject: e.subject})
	switch {
	case err == nil:
		row, err := q.GetUser(ctx, link.UserID)
		if err != nil {
			return dbgen.User{}, false, fmt.Errorf("auth: read a user: %w", err)
		}
		return row, true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return dbgen.User{}, false, fmt.Errorf("auth: read an identity: %w", err)
	}
	email := strings.ToLower(strings.TrimSpace(e.email))
	if email == "" || !e.emailVerified {
		return dbgen.User{}, false, nil
	}
	row, err := q.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		return s.linkExisting(ctx, tx, e, row)
	case errors.Is(err, pgx.ErrNoRows):
		return s.provision(ctx, tx, e, email)
	default:
		return dbgen.User{}, false, fmt.Errorf("auth: read a user: %w", err)
	}
}

// linkExisting links a provider's identity to the account with its address.
// A provider of one organisation may link only to a person who already
// belongs to that organisation: otherwise whoever controls such a provider
// could sign in as anyone with an account anywhere.
func (s *Service) linkExisting(ctx context.Context, tx pgx.Tx, e externalLogin, row dbgen.User) (dbgen.User, bool, error) {
	q := dbgen.New(tx)
	if e.row != nil && e.row.OrganizationID.Valid {
		member, err := q.UserHasAccessIn(ctx, dbgen.UserHasAccessInParams{OrganizationID: e.row.OrganizationID, UserID: row.ID})
		if err != nil {
			return dbgen.User{}, false, fmt.Errorf("auth: read memberships: %w", err)
		}
		if member == nil || !*member {
			return dbgen.User{}, false, nil
		}
	}
	if err := s.link(ctx, tx, e, row.ID); err != nil {
		return dbgen.User{}, false, err
	}
	if row.InvitationHash != nil {
		// Signing in through the provider accepts the invitation: the
		// provider has proved the address it was sent to.
		var err error
		if row, err = q.AcceptInvitationExternally(ctx, row.ID); err != nil {
			return dbgen.User{}, false, fmt.Errorf("auth: accept the invitation: %w", err)
		}
	}
	return row, true, s.record(ctx, tx, audit.Entry{
		Actor:  audit.Actor{Kind: KindUser, ID: storage.ID(row.ID), Display: row.DisplayName},
		Action: audit.IdentityLinked, TargetKind: "user", TargetID: storage.ID(row.ID), Detail: e.issuer,
	})
}

// link records that a provider's subject is an account.
func (s *Service) link(ctx context.Context, tx pgx.Tx, e externalLogin, user pgtype.UUID) error {
	id, err := s.newID()
	if err != nil {
		return err
	}
	if _, err := dbgen.New(tx).LinkUserIdentity(ctx, dbgen.LinkUserIdentityParams{
		ID: storage.MustUUID(id), UserID: user, Issuer: e.issuer, Subject: e.subject,
	}); err != nil {
		return fmt.Errorf("auth: link an identity: %w", err)
	}
	return nil
}

// provision creates the account of a person the provider vouches for, when
// the provider is set to and the address is new. The account has no password
// and no invitation: the provider is its only way in, and it holds only what
// the mapping gives.
func (s *Service) provision(ctx context.Context, tx pgx.Tx, e externalLogin, email string) (dbgen.User, bool, error) {
	if e.row == nil || !e.row.JitProvisioning {
		return dbgen.User{}, false, nil
	}
	email, err := normaliseEmail(email)
	if err != nil {
		return dbgen.User{}, false, nil //nolint:nilerr // an address that cannot be stored is not provisioned
	}
	id, err := s.newID()
	if err != nil {
		return dbgen.User{}, false, err
	}
	display := strings.TrimSpace(e.name)
	if display == "" {
		display, _, _ = strings.Cut(email, "@")
	}
	row, err := dbgen.New(tx).CreateUser(ctx, dbgen.CreateUserParams{ID: storage.MustUUID(id), Email: email, DisplayName: display})
	if err != nil {
		return dbgen.User{}, false, fmt.Errorf("auth: provision a user: %w", err)
	}
	if err := s.link(ctx, tx, e, row.ID); err != nil {
		return dbgen.User{}, false, err
	}
	return row, true, s.record(ctx, tx, audit.Entry{
		Actor: providerActor(*e.row), Action: audit.UserProvisioned,
		TargetKind: "user", TargetID: id, Detail: "provider " + e.row.Key,
	})
}

// finishExternal signs a resolved account in and records the sign-in with
// what the provider reported of it.
func (s *Service) finishExternal(ctx context.Context, e externalLogin, userID pgtype.UUID) (Session, Challenge, error) {
	var (
		session   Session
		challenge Challenge
		refused   bool
	)
	err := s.inTx(ctx, e.tenant(), func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		// The account may have been disabled while its mapping was applied.
		user, err := q.GetUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("auth: read a user: %w", err)
		}
		if user.DisabledAt.Valid {
			refused = true
			return s.refusal(ctx, tx, user)
		}
		if session, challenge, err = s.signedIn(ctx, tx, user); err != nil {
			return err
		}
		return s.recordFederated(ctx, tx, e, user, session)
	})
	if err != nil {
		return Session{}, Challenge{}, err
	}
	if refused {
		return Session{}, Challenge{}, signInRefused()
	}
	return session, challenge, nil
}

// recordFederated keeps the sign-in so that a back-channel logout can find
// its session, and audits it with the authentication context.
func (s *Service) recordFederated(ctx context.Context, tx pgx.Tx, e externalLogin, user dbgen.User, session Session) error {
	q := dbgen.New(tx)
	id, err := s.newID()
	if err != nil {
		return err
	}
	var provider, sess pgtype.UUID
	key := "default"
	if e.row != nil {
		provider, key = e.row.ID, e.row.Key
	}
	if session.ID != "" {
		sess = storage.MustUUID(session.ID)
	}
	amr := e.amr
	if amr == nil {
		amr = []string{}
	}
	if _, err := q.RecordFederatedSignIn(ctx, dbgen.RecordFederatedSignInParams{
		ID: storage.MustUUID(id), UserID: user.ID, ProviderID: provider, Issuer: e.issuer, Subject: e.subject,
		Sid: e.sid, Acr: e.acr, Amr: amr, SessionID: sess,
	}); err != nil {
		return fmt.Errorf("auth: record a federated sign-in: %w", err)
	}
	if err := q.DeleteOldFederatedSignIns(ctx, dbgen.DeleteOldFederatedSignInsParams{
		UserID: user.ID, SignedInAt: storage.Timestamp(s.now().Add(-2 * s.o.SessionTTL)),
	}); err != nil {
		return fmt.Errorf("auth: prune federated sign-ins: %w", err)
	}
	return s.record(ctx, tx, audit.Entry{
		Actor:  audit.Actor{Kind: KindUser, ID: storage.ID(user.ID), Display: user.DisplayName},
		Action: audit.FederatedSignIn, TargetKind: "user", TargetID: storage.ID(user.ID),
		Detail: fmt.Sprintf("provider=%s acr=%s amr=%s", key, e.acr, strings.Join(e.amr, ",")),
	})
}

// hasInstallationProvider reports whether people can sign in through a
// provider of the whole installation: the one in the configuration file or
// an enabled one in the database.
func (s *Service) hasInstallationProvider(ctx context.Context) (bool, error) {
	if s.o.OIDC != nil {
		return true, nil
	}
	var n int64
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).CountEnabledInstallationProviders(ctx)
		return err //nolint:wrapcheck // wrapped below
	})
	if err != nil {
		return false, fmt.Errorf("auth: count the installation's providers: %w", err)
	}
	return n > 0, nil
}

// passwordLoginOff reports whether password sign-in is refused:
// auth.studio.allowPasswordLogin is false and a provider exists to sign in
// with instead. Without one the setting is not honoured, so that nobody is
// locked out.
func (s *Service) passwordLoginOff(ctx context.Context) (bool, error) {
	if !s.o.PasswordLoginDisabled {
		return false, nil
	}
	return s.hasInstallationProvider(ctx)
}

// CheckPasswordPolicy refuses a configuration that turns password sign-in
// off while no installation provider exists, which nobody could sign in to:
// auth.studio.allowPasswordLogin may be false only once a provider exists.
func (s *Service) CheckPasswordPolicy(ctx context.Context) error {
	if !s.o.PasswordLoginDisabled {
		return nil
	}
	ok, err := s.hasInstallationProvider(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("auth: auth.studio.allowPasswordLogin is false, but no identity provider of the installation exists; configure one first")
	}
	return nil
}

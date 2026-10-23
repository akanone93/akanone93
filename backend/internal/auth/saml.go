// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// This file signs people in with a SAML 2.0 provider (GOV-004). The
// protocol, including every XML signature check, is the enterprise
// extension's (ext/identity.SAMLProvider); the core hands it a provider's
// configuration, the clock, the skew bound from the limits registry and a
// replay cache, and treats the verified identity it returns exactly like an
// OpenID Connect one (signInExternal).

// samlLoginTTL is how long a SAML sign-in may take at the provider.
const samlLoginTTL = 10 * time.Minute

// replayCache is the shared cache as the extension sees it: it remembers
// assertion identifiers, in Valkey or, on a single node, in memory.
type replayCache struct{ c cache.Cache }

// Seen records an assertion identifier and reports whether it was known.
func (r replayCache) Seen(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	fresh, err := r.c.SetNX(ctx, "saml:replay:"+key, []byte{1}, ttl)
	if err != nil {
		return false, fmt.Errorf("auth: remember an assertion: %w", err)
	}
	return !fresh, nil
}

// SAMLLogin is a SAML sign-in handed to the identity provider.
type SAMLLogin struct {
	// URL is where the browser goes to sign in.
	URL string
	// State is the relay state, which comes back with the response.
	State     string
	ExpiresAt time.Time
}

// samlProvider loads an enabled SAML provider for a sign-in and checks that
// the licence still allows it to be used. A licence that has expired keeps
// existing sign-ins working (GOV-030); one that is absent or invalid does
// not.
func (s *Service) samlProvider(ctx context.Context, providerID string) (dbgen.IdentityProvider, identity.SAMLConfig, error) {
	if err := edition.RequireRead(ctx, edition.FeatureSAML); err != nil {
		return dbgen.IdentityProvider{}, identity.SAMLConfig{}, err //nolint:wrapcheck // a domain error
	}
	id, err := parseID(providerID, "provider")
	if err != nil {
		return dbgen.IdentityProvider{}, identity.SAMLConfig{}, err
	}
	var row dbgen.IdentityProvider
	err = s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetIdentityProvider(ctx, id)
		return err //nolint:wrapcheck // examined below
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!row.Enabled || row.Kind != ProviderSAML)) {
		return dbgen.IdentityProvider{}, identity.SAMLConfig{}, plxerr.New(plxerr.AuthenticationRequired, "the sign-in provider is not available")
	}
	if err != nil {
		return dbgen.IdentityProvider{}, identity.SAMLConfig{}, fmt.Errorf("auth: read a provider: %w", err)
	}
	cfg, err := s.samlConfig(ctx, row)
	return row, cfg, err
}

// samlConfig builds the extension's view of a stored SAML provider, with the
// service provider's key opened for the call.
func (s *Service) samlConfig(ctx context.Context, row dbgen.IdentityProvider) (identity.SAMLConfig, error) {
	var c storedSAML
	if err := json.Unmarshal(row.Config, &c); err != nil {
		return identity.SAMLConfig{}, fmt.Errorf("auth: read a provider's settings: %w", err)
	}
	key, err := s.openProviderSecret(ctx, row)
	if err != nil {
		return identity.SAMLConfig{}, err
	}
	return identity.SAMLConfig{
		EntityID: c.EntityID, ACSURL: c.ACSURL, IdP: c.IdP, DecryptionKeyPEM: key, DecryptionCertPEM: []byte(c.SPCertificate),
		AllowIdPInitiated: c.AllowIdPInitiated, EmailAttribute: c.EmailAttribute, NameAttribute: c.NameAttribute,
		GroupsAttribute: c.GroupsAttribute,
	}, nil
}

// StartSAMLLogin begins a SAML sign-in and returns where to send the
// browser.
func (s *Service) StartSAMLLogin(ctx context.Context, providerID string) (SAMLLogin, error) {
	row, cfg, err := s.samlProvider(ctx, providerID)
	if err != nil {
		return SAMLLogin{}, err
	}
	state, err := NewSecret(PrefixOIDCState, 32)
	if err != nil {
		return SAMLLogin{}, err
	}
	start, err := s.o.SAML.Start(ctx, cfg, state.Value, s.now())
	if err != nil {
		return SAMLLogin{}, samlFailure(err)
	}
	expires := s.now().Add(samlLoginTTL)
	err = s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.DeleteExpiredSAMLLogins(ctx); err != nil {
			return fmt.Errorf("auth: prune sign-ins: %w", err)
		}
		return q.CreateSAMLLogin(ctx, dbgen.CreateSAMLLoginParams{ //nolint:wrapcheck // wrapped below
			StateHash: state.Hash, ProviderID: row.ID, RequestID: start.RequestID, ExpiresAt: storage.Timestamp(expires),
		})
	})
	if err != nil {
		return SAMLLogin{}, fmt.Errorf("auth: record the sign-in: %w", err)
	}
	return SAMLLogin{URL: start.URL, State: state.Value, ExpiresAt: expires}, nil
}

// SAMLMetadata returns the service provider's metadata for a provider, which
// the identity provider's administrator imports.
func (s *Service) SAMLMetadata(ctx context.Context, providerID string) ([]byte, error) {
	_, cfg, err := s.samlProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	doc, err := s.o.SAML.ServiceProviderMetadata(ctx, cfg, cfg.EntityID)
	if err != nil {
		return nil, samlFailure(err)
	}
	return doc, nil
}

// CompleteSAMLLogin ends a SAML sign-in with the response the identity
// provider posted. A response that answers a sign-in of ours needs its relay
// state; one without is identity-provider-initiated, accepted only where the
// provider allows it. Like a password, it yields a session, or a challenge
// when the account has a confirmed second factor.
func (s *Service) CompleteSAMLLogin(ctx context.Context, providerID, samlResponse, relayState string) (Session, Challenge, error) {
	row, cfg, err := s.samlProvider(ctx, providerID)
	if err != nil {
		return Session{}, Challenge{}, err
	}
	requestID, err := s.takeSAMLLogin(ctx, row, relayState)
	if err != nil {
		return Session{}, Challenge{}, err
	}
	skew := time.Duration(s.o.Limits.Get(limits.AuthSamlClockSkewSeconds)) * time.Second
	who, err := s.o.SAML.Verify(ctx, identity.SAMLVerify{
		Config: cfg, SAMLResponse: samlResponse, RequestID: requestID, Now: s.now(), ClockSkew: skew,
		Replay: replayCache{s.o.Cache},
	})
	if err != nil {
		return Session{}, Challenge{}, samlFailure(err)
	}
	ext := externalLogin{
		row: &row, issuer: "saml:" + who.Issuer, subject: who.Subject, email: who.Email, emailVerified: who.EmailVerified,
		name: who.DisplayName, groups: who.Groups, sid: who.SessionIndex, acr: who.AuthnContext,
	}
	return s.signInExternal(ctx, ext)
}

// takeSAMLLogin consumes the sign-in a relay state names and returns the
// request the response must answer; "" for a response the identity provider
// started.
func (s *Service) takeSAMLLogin(ctx context.Context, row dbgen.IdentityProvider, relayState string) (string, error) {
	if relayState == "" {
		return "", nil
	}
	var login dbgen.SamlLogin
	err := s.inTx(ctx, storage.Tenant{Scope: storage.ScopeInstallation}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		login, err = dbgen.New(tx).TakeSAMLLogin(ctx, HashSecret(relayState))
		return err //nolint:wrapcheck // examined below
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (s.now().After(storage.Time(login.ExpiresAt)) || login.ProviderID != row.ID)) {
		return "", plxerr.New(plxerr.SAMLResponseInvalid, "the sign-in has expired or belongs to another provider; sign in again")
	}
	if err != nil {
		return "", fmt.Errorf("auth: read the sign-in: %w", err)
	}
	return login.RequestID, nil
}

// samlFailure maps what the extension reports to the API's codes. Its detail
// stays in the error's cause, out of the message.
func samlFailure(err error) error {
	if _, ok := plxerr.CodeOf(err); ok {
		return err
	}
	if errors.Is(err, identity.ErrReplayed) {
		return plxerr.Wrap(plxerr.SAMLAssertionReplayed, err, "the SAML assertion was already used; sign in again")
	}
	return plxerr.Wrap(plxerr.SAMLResponseInvalid, err, "the SAML response is not valid")
}

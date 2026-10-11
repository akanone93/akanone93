// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
)

// IdentityProvider is the customer identity provider an environment's
// user tokens come from (SEC-026).
type IdentityProvider struct {
	AppID, EnvironmentID string
	// Issuer is the `iss` the provider's tokens carry.
	Issuer string
	// JWKSURL is where the provider publishes its keys; HTTPS only.
	JWKSURL string
	// Audiences are the `aud` values accepted.
	Audiences []string
	// ForwardClaims are the claims passed to upstreams; `sub` always is.
	ForwardClaims []string
}

// Validate checks a provider's registration.
func (p IdentityProvider) Validate() error {
	u, err := url.Parse(p.JWKSURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return invalid("the keys URL of an identity provider is an https URL without credentials")
	}
	if p.Issuer == "" || len(p.Issuer) > 512 {
		return invalid("an identity provider needs an issuer")
	}
	if len(p.Audiences) == 0 {
		return plxerr.New(plxerr.MissingProperty, "an identity provider needs at least one audience")
	}
	for _, a := range append(append([]string{}, p.Audiences...), p.ForwardClaims...) {
		if a == "" || len(a) > 256 || strings.ContainsAny(a, "\r\n") {
			return invalid("an audience or claim name is empty or too long")
		}
	}
	return nil
}

// SetProvider sets the identity provider of an environment (SEC-026).
func (r *Registry) SetProvider(ctx context.Context, p auth.Principal, idp IdentityProvider) error {
	if err := p.AuthorizeApp(auth.AppManage, idp.AppID); err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	if err := idp.Validate(); err != nil {
		return err
	}
	err := r.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, idp.AppID, idp.EnvironmentID)
		if err != nil {
			return err
		}
		claims := idp.ForwardClaims
		if claims == nil {
			claims = []string{}
		}
		if err := q.UpsertUserIdentityProvider(ctx, dbgen.UpsertUserIdentityProviderParams{
			EnvironmentID: env.ID, OrganizationID: env.OrganizationID, AppID: env.AppID,
			Issuer: idp.Issuer, JwksUrl: idp.JWKSURL, Audiences: idp.Audiences, ForwardClaims: claims,
		}); err != nil {
			return failure(err, "identity provider")
		}
		return r.recordProvider(ctx, tx, p, audit.UserIdentityProviderSet, idp)
	})
	if err == nil {
		r.forgetProvider(idp.EnvironmentID)
	}
	return err
}

// GetProvider returns the identity provider of an environment.
func (r *Registry) GetProvider(ctx context.Context, p auth.Principal, appID, environmentID string) (IdentityProvider, error) {
	if err := p.AuthorizeApp(auth.AppRead, appID); err != nil {
		return IdentityProvider{}, fmt.Errorf("gateway: %w", err)
	}
	var out IdentityProvider
	err := r.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		row, err := q.GetUserIdentityProvider(ctx, env.ID)
		if err != nil {
			return failure(err, "identity provider")
		}
		out = providerOf(row)
		return nil
	})
	return out, err
}

// DeleteProvider removes the identity provider of an environment.
func (r *Registry) DeleteProvider(ctx context.Context, p auth.Principal, appID, environmentID string) error {
	if err := p.AuthorizeApp(auth.AppManage, appID); err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	err := r.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		env, err := environmentOf(ctx, q, appID, environmentID)
		if err != nil {
			return err
		}
		n, err := q.DeleteUserIdentityProvider(ctx, env.ID)
		if err != nil {
			return failure(err, "identity provider")
		}
		if n == 0 {
			return plxerr.New(plxerr.ResourceNotFound, "no such identity provider")
		}
		return r.recordProvider(ctx, tx, p, audit.UserIdentityProviderDeleted, IdentityProvider{AppID: appID, EnvironmentID: environmentID})
	})
	if err == nil {
		r.forgetProvider(environmentID)
	}
	return err
}

// ProviderFor returns the identity provider of the environment a device
// belongs to; PreconditionFailed when it has none. The answer may be up
// to resolveTTL old.
func (r *Registry) ProviderFor(ctx context.Context, organizationID, environmentID string) (IdentityProvider, error) {
	key := "idp/" + environmentID
	now := r.now()
	r.mu.Lock()
	hit, ok := r.providers[key]
	r.mu.Unlock()
	if ok && now.Before(hit.until) {
		return hit.provider, nil
	}
	env, err := storage.UUID(environmentID)
	if err != nil {
		return IdentityProvider{}, plxerr.New(plxerr.PreconditionFailed, "this environment has no identity provider")
	}
	var idp IdentityProvider
	err = r.o.DB.InTx(ctx, storage.Tenant{OrganizationID: organizationID}, func(ctx context.Context, tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetUserIdentityProvider(ctx, env)
		if errors.Is(err, pgx.ErrNoRows) {
			return plxerr.New(plxerr.PreconditionFailed, "this environment has no identity provider")
		}
		if err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
		idp = providerOf(row)
		return nil
	})
	if err != nil {
		return IdentityProvider{}, err //nolint:wrapcheck // InTx wraps its own failures
	}
	r.mu.Lock()
	if r.providers == nil || len(r.providers) >= maxResolved {
		r.providers = map[string]cachedProvider{}
	}
	r.providers[key] = cachedProvider{provider: idp, until: now.Add(resolveTTL)}
	r.mu.Unlock()
	return idp, nil
}

// cachedProvider is one remembered provider.
type cachedProvider struct {
	provider IdentityProvider
	until    time.Time
}

func (r *Registry) forgetProvider(environmentID string) {
	r.mu.Lock()
	delete(r.providers, "idp/"+environmentID)
	r.mu.Unlock()
}

func (r *Registry) recordProvider(ctx context.Context, tx pgx.Tx, p auth.Principal, action audit.Action, idp IdentityProvider) error {
	_, err := r.o.Audit.Append(ctx, tx, audit.Entry{
		OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: action,
		TargetKind: "environment", TargetID: idp.EnvironmentID, Detail: "issuer " + idp.Issuer,
	})
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}
	return nil
}

func providerOf(row dbgen.UserIdentityProvider) IdentityProvider {
	return IdentityProvider{
		AppID: storage.ID(row.AppID), EnvironmentID: storage.ID(row.EnvironmentID), Issuer: row.Issuer,
		JWKSURL: row.JwksUrl, Audiences: row.Audiences, ForwardClaims: row.ForwardClaims,
	}
}

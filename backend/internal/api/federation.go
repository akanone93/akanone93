// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// Federation serves FederationService: identity providers, group mappings
// and SCIM tokens (GOV-004, ADR-0074).
type Federation struct{ h *Handlers }

var _ pluxv1connect.FederationServiceHandler = Federation{}

// Federation returns the FederationService handler.
func (h *Handlers) Federation() Federation { return Federation{h: h} }

// installationKey is the page-token scope of the installation's lists.
const installationKey = "installation"

// scope is the authority a provider call runs under: the installation's
// when wide is set, or when the call names no organisation; an
// organisation's otherwise. It also returns the key page tokens are bound
// to.
func (s Federation) scope(ctx context.Context, header http.Header, wide bool) (auth.ProviderScope, string, error) {
	id, err := identity(ctx)
	if err != nil {
		return auth.ProviderScope{}, "", err
	}
	if wide || (header.Get(OrganizationHeader) == "" && id.OrganizationID == "") {
		sc, err := auth.InstallationProviders(id)
		return sc, installationKey, err //nolint:wrapcheck // a domain error
	}
	p, err := s.h.principal(ctx, header, "")
	if err != nil {
		return auth.ProviderScope{}, "", err
	}
	sc, err := auth.OrganizationProviders(p)
	return sc, p.OrganizationID, err //nolint:wrapcheck // a domain error
}

// ListSignInProviders lists what a person may sign in with.
func (s Federation) ListSignInProviders(ctx context.Context, req *connect.Request[pluxv1.ListSignInProvidersRequest]) (*connect.Response[pluxv1.ListSignInProvidersResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListSignInProvidersResponse, error) {
		providers, err := s.h.Auth.ListSignInProviders(ctx, req.Msg.GetOrganizationKey(), edition.Readable(ctx, edition.FeatureSAML))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListSignInProvidersResponse{}
		for _, p := range providers {
			out.Providers = append(out.Providers, &pluxv1.SignInProvider{
				Id: p.ID, Key: p.Key, Kind: p.Kind, Name: p.Name, OrganizationKey: p.OrganizationKey,
			})
		}
		return out, nil
	})
}

// StartSamlLogin begins a SAML sign-in.
func (s Federation) StartSamlLogin(ctx context.Context, req *connect.Request[pluxv1.StartSamlLoginRequest]) (*connect.Response[pluxv1.StartSamlLoginResponse], error) {
	login, err := s.h.Auth.StartSAMLLogin(ctx, req.Msg.GetProviderId())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&pluxv1.StartSamlLoginResponse{Url: login.URL, State: login.State, ExpiresAt: ts(login.ExpiresAt)}), nil
}

// CompleteSamlLogin ends a SAML sign-in: a session cookie, or a
// second-factor challenge.
func (s Federation) CompleteSamlLogin(ctx context.Context, req *connect.Request[pluxv1.CompleteSamlLoginRequest]) (*connect.Response[pluxv1.CompleteSamlLoginResponse], error) {
	m := req.Msg
	session, challenge, err := s.h.Auth.CompleteSAMLLogin(ctx, m.GetProviderId(), m.GetSamlResponse(), m.GetRelayState())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	out := connect.NewResponse(&pluxv1.CompleteSamlLoginResponse{})
	if challenge.Secret != "" {
		out.Msg.Challenge = &pluxv1.MfaChallenge{Id: challenge.Secret, Kinds: challenge.Kinds, ExpiresAt: ts(challenge.ExpiresAt)}
		return out, nil
	}
	out.Msg.Session = sessionProto(session)
	out.Header().Add("Set-Cookie", SessionCookieHeader(session.Secret, session.ExpiresAt, s.h.now()))
	return out, nil
}

// GetSamlMetadata returns the service provider's SAML metadata.
func (s Federation) GetSamlMetadata(ctx context.Context, req *connect.Request[pluxv1.GetSamlMetadataRequest]) (*connect.Response[pluxv1.GetSamlMetadataResponse], error) {
	doc, err := s.h.Auth.SAMLMetadata(ctx, req.Msg.GetProviderId())
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	return connect.NewResponse(&pluxv1.GetSamlMetadataResponse{MetadataXml: doc}), nil
}

// ListIdentityProviders lists the providers of the organisation or the
// installation.
func (s Federation) ListIdentityProviders(ctx context.Context, req *connect.Request[pluxv1.ListIdentityProvidersRequest]) (*connect.Response[pluxv1.ListIdentityProvidersResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListIdentityProvidersResponse, error) {
		sc, key, err := s.scope(ctx, req.Header(), req.Msg.GetInstallationWide())
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "key", Filters: map[string]func(Item) string{
			"kind": func(m Item) string { return m.(*pluxv1.IdentityProvider).GetKind() },
		}}
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, key, page)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		providers, err := s.h.Auth.ListProviders(ctx, sc, after.Key, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListIdentityProvidersResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, p := range providers {
			last = storage.Cursor{Key: p.Key}
			if item := providerProto(p); q.Keep(clauses, item) {
				out.Providers = append(out.Providers, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, key, page, len(providers), size, last)
		return out, Mask(page.GetReadMask(), out.Providers)
	})
}

// GetIdentityProvider returns one provider.
func (s Federation) GetIdentityProvider(ctx context.Context, req *connect.Request[pluxv1.GetIdentityProviderRequest]) (*connect.Response[pluxv1.GetIdentityProviderResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetIdentityProviderResponse, error) {
		sc, _, err := s.scope(ctx, req.Header(), false)
		if err != nil {
			return nil, err
		}
		p, err := s.h.Auth.GetProvider(ctx, sc, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetIdentityProviderResponse{Provider: providerProto(p)}, nil
	})
}

// CreateIdentityProvider configures a provider.
func (s Federation) CreateIdentityProvider(ctx context.Context, req *connect.Request[pluxv1.CreateIdentityProviderRequest]) (*connect.Response[pluxv1.CreateIdentityProviderResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateIdentityProviderResponse, error) {
		sc, _, err := s.scope(ctx, req.Header(), req.Msg.GetInstallationWide())
		if err != nil {
			return nil, err
		}
		p, err := s.h.Auth.CreateProvider(ctx, sc, specOf(req.Msg.GetProvider()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateIdentityProviderResponse{Provider: providerProto(p)}, nil
	})
}

// UpdateIdentityProvider changes a provider.
func (s Federation) UpdateIdentityProvider(ctx context.Context, req *connect.Request[pluxv1.UpdateIdentityProviderRequest]) (*connect.Response[pluxv1.UpdateIdentityProviderResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UpdateIdentityProviderResponse, error) {
		sc, _, err := s.scope(ctx, req.Header(), false)
		if err != nil {
			return nil, err
		}
		in := req.Msg.GetProvider()
		p, err := s.h.Auth.UpdateProvider(ctx, sc, in.GetId(), specOf(in))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UpdateIdentityProviderResponse{Provider: providerProto(p)}, nil
	})
}

// DeleteIdentityProvider removes a provider.
func (s Federation) DeleteIdentityProvider(ctx context.Context, req *connect.Request[pluxv1.DeleteIdentityProviderRequest]) (*connect.Response[pluxv1.DeleteIdentityProviderResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteIdentityProviderResponse, error) {
		sc, _, err := s.scope(ctx, req.Header(), false)
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.DeleteProvider(ctx, sc, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteIdentityProviderResponse{}, nil
	})
}

// GetGroupMapping returns a provider's mapping document.
func (s Federation) GetGroupMapping(ctx context.Context, req *connect.Request[pluxv1.GetGroupMappingRequest]) (*connect.Response[pluxv1.GetGroupMappingResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetGroupMappingResponse, error) {
		sc, _, err := s.scope(ctx, req.Header(), false)
		if err != nil {
			return nil, err
		}
		m, err := s.h.Auth.GetMapping(ctx, sc, req.Msg.GetProviderId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetGroupMappingResponse{Mapping: mappingProto(m)}, nil
	})
}

// SetGroupMapping replaces a provider's mapping document.
func (s Federation) SetGroupMapping(ctx context.Context, req *connect.Request[pluxv1.SetGroupMappingRequest]) (*connect.Response[pluxv1.SetGroupMappingResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetGroupMappingResponse, error) {
		sc, _, err := s.scope(ctx, req.Header(), false)
		if err != nil {
			return nil, err
		}
		m, err := s.h.Auth.SetMapping(ctx, sc, req.Msg.GetProviderId(), mappingOf(req.Msg.GetMapping()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetGroupMappingResponse{Mapping: mappingProto(m)}, nil
	})
}

// ListScimTokens lists the organisation's SCIM tokens.
func (s Federation) ListScimTokens(ctx context.Context, req *connect.Request[pluxv1.ListScimTokensRequest]) (*connect.Response[pluxv1.ListScimTokensResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.ListScimTokensResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := Query{OrderBy: "created_at"}
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, p.OrganizationID, page)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		tokens, err := s.h.Auth.ListScimTokens(ctx, p, after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListScimTokensResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, t := range tokens {
			last = storage.Cursor{Time: t.CreatedAt, ID: t.ID}
			if item := scimTokenProto(t); q.Keep(clauses, item) {
				out.Tokens = append(out.Tokens, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, p.OrganizationID, page, len(tokens), size, last)
		return out, Mask(page.GetReadMask(), out.Tokens)
	})
}

// CreateScimToken makes a SCIM token and returns its secret once.
func (s Federation) CreateScimToken(ctx context.Context, req *connect.Request[pluxv1.CreateScimTokenRequest]) (*connect.Response[pluxv1.CreateScimTokenResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateScimTokenResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		t, secret, err := s.h.Auth.CreateScimToken(ctx, p, req.Msg.GetName())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateScimTokenResponse{Token: scimTokenProto(t), Secret: secret}, nil
	})
}

// RotateScimToken replaces a SCIM token, the old one working for an overlap.
func (s Federation) RotateScimToken(ctx context.Context, req *connect.Request[pluxv1.RotateScimTokenRequest]) (*connect.Response[pluxv1.RotateScimTokenResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RotateScimTokenResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		next, secret, prev, err := s.h.Auth.RotateScimToken(ctx, p, req.Msg.GetId(), time.Duration(req.Msg.GetOverlapSeconds())*time.Second)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RotateScimTokenResponse{Token: scimTokenProto(next), Secret: secret, Previous: scimTokenProto(prev)}, nil
	})
}

// RevokeScimToken ends a SCIM token at once.
func (s Federation) RevokeScimToken(ctx context.Context, req *connect.Request[pluxv1.RevokeScimTokenRequest]) (*connect.Response[pluxv1.RevokeScimTokenResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.RevokeScimTokenResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Auth.RevokeScimToken(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.RevokeScimTokenResponse{}, nil
	})
}

// scimTokenProto converts a SCIM token.
func scimTokenProto(t auth.ScimToken) *pluxv1.ScimToken {
	return &pluxv1.ScimToken{
		Id: t.ID, OrganizationId: t.OrganizationID, Name: t.Name, Prefix: t.Prefix,
		CreatedAt: ts(t.CreatedAt), ExpiresAt: ts(t.ExpiresAt), RevokedAt: ts(t.RevokedAt), LastUsedAt: ts(t.LastUsedAt),
	}
}

// providerProto converts a provider; a secret is never part of it.
func providerProto(p auth.Provider) *pluxv1.IdentityProvider {
	out := &pluxv1.IdentityProvider{
		Id: p.ID, OrganizationId: p.OrganizationID, Key: p.Key, Kind: p.Kind, Name: p.Name,
		Enabled: p.Enabled, JitProvisioning: p.JIT, CreatedAt: ts(p.CreatedAt), UpdatedAt: ts(p.UpdatedAt),
	}
	switch {
	case p.OIDC != nil:
		out.Config = &pluxv1.IdentityProvider_Oidc{Oidc: &pluxv1.OidcProviderConfig{
			Issuer: p.OIDC.Issuer, ClientId: p.OIDC.ClientID, RedirectUrl: p.OIDC.RedirectURL, HasClientSecret: p.OIDC.HasSecret,
		}}
	case p.SAML != nil:
		c := p.SAML
		out.Config = &pluxv1.IdentityProvider_Saml{Saml: &pluxv1.SamlProviderConfig{
			IdpEntityId: c.IdP.EntityID, IdpSsoUrl: c.IdP.SSOURL, IdpSigningCertificates: c.IdP.SigningCertificates,
			EntityId: c.EntityID, AcsUrl: c.ACSURL, AllowIdpInitiated: c.AllowIdPInitiated,
			EmailAttribute: c.EmailAttribute, NameAttribute: c.NameAttribute, GroupsAttribute: c.GroupsAttribute,
			ServiceProviderCertificatePem: c.ServiceProviderCertificate,
		}}
	}
	return out
}

// specOf converts a provider as a caller sent it.
func specOf(p *pluxv1.IdentityProvider) auth.ProviderSpec {
	spec := auth.ProviderSpec{Key: p.GetKey(), Kind: p.GetKind(), Name: p.GetName(), Enabled: p.GetEnabled(), JIT: p.GetJitProvisioning()}
	if o := p.GetOidc(); o != nil {
		spec.OIDC = &auth.OIDCSettings{Issuer: o.GetIssuer(), ClientID: o.GetClientId(), RedirectURL: o.GetRedirectUrl()}
		spec.ClientSecret = o.GetClientSecret()
	}
	if c := p.GetSaml(); c != nil {
		spec.SAML = &auth.SAMLSettings{
			EntityID: c.GetEntityId(), ACSURL: c.GetAcsUrl(), AllowIdPInitiated: c.GetAllowIdpInitiated(),
			EmailAttribute: c.GetEmailAttribute(), NameAttribute: c.GetNameAttribute(), GroupsAttribute: c.GetGroupsAttribute(),
		}
		spec.IdPMetadata = c.GetIdpMetadataXml()
	}
	return spec
}

// mappingProto converts a mapping document.
func mappingProto(m auth.GroupMapping) *pluxv1.GroupMapping {
	out := &pluxv1.GroupMapping{GroupsClaim: m.GroupsClaim}
	for _, r := range m.Rules {
		rule := &pluxv1.MappingRule{Group: r.Group, Organization: r.Organization, Teams: r.Teams}
		for _, g := range r.Grants {
			rule.Grants = append(rule.Grants, &pluxv1.GrantRule{Role: g.Role, Scope: string(g.Scope), App: g.App, Environment: g.Environment})
		}
		out.Rules = append(out.Rules, rule)
	}
	return out
}

// mappingOf converts a mapping document as a caller sent it.
func mappingOf(m *pluxv1.GroupMapping) auth.GroupMapping {
	out := auth.GroupMapping{GroupsClaim: m.GetGroupsClaim()}
	for _, r := range m.GetRules() {
		rule := auth.MappingRule{Group: r.GetGroup(), Organization: r.GetOrganization(), Teams: r.GetTeams()}
		for _, g := range r.GetGrants() {
			rule.Grants = append(rule.Grants, auth.GrantRule{Role: g.GetRole(), Scope: auth.Scope(g.GetScope()), App: g.GetApp(), Environment: g.GetEnvironment()})
		}
		out.Rules = append(out.Rules, rule)
	}
	return out
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/storage"
)

// SecurityAdmin serves SecurityAdminService: the security configuration
// (SEC-182), the gateway's upstreams and the identity provider of an
// environment's users (SEC-031, SEC-026) and the audit checkpoints
// (SEC-141). Where the process has no gateway, the upstream procedures
// answer Unimplemented.
type SecurityAdmin struct {
	pluxv1connect.UnimplementedSecurityAdminServiceHandler
	h           *Handlers
	checkpoints CheckpointSource
}

var _ pluxv1connect.SecurityAdminServiceHandler = SecurityAdmin{}

// SecurityAdmin returns the SecurityAdminService handler.
func (h *Handlers) SecurityAdmin() SecurityAdmin {
	return SecurityAdmin{h: h, checkpoints: h.AuditCheckpoints}
}

// SetSecurityConfig sets an environment's profile and overrides (SEC-180,
// SEC-182).
func (s SecurityAdmin) SetSecurityConfig(ctx context.Context, req *connect.Request[pluxv1.SetSecurityConfigRequest]) (*connect.Response[pluxv1.SetSecurityConfigResponse], error) {
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetSecurityConfigResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg
		version, err := s.h.SecurityConfig.Set(ctx, p, seccfg.Change{
			AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Profile: m.GetProfile(),
			Overrides: m.GetOverridesJson(), ExpectedVersion: m.GetExpectedVersion(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetSecurityConfigResponse{Version: version}, nil
	})
}

// GetEffectiveSecurityConfig returns an environment's profile merged with
// its overrides, and the hash of the device document the manifest pins
// (SEC-180, SEC-182).
func (s SecurityAdmin) GetEffectiveSecurityConfig(ctx context.Context, req *connect.Request[pluxv1.GetEffectiveSecurityConfigRequest]) (*connect.Response[pluxv1.GetEffectiveSecurityConfigResponse], error) {
	return read(ctx, func(ctx context.Context) (*pluxv1.GetEffectiveSecurityConfigResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		c, err := s.h.SecurityConfig.Get(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		effective, err := c.Values.EffectiveJSON()
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		_, sum, err := c.DeviceDocument()
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetEffectiveSecurityConfigResponse{
			Profile: string(c.Values.Profile()), Version: c.Version, EffectiveJson: effective, Sha256: sum[:],
		}, nil
	})
}

// CreateUpstream registers an upstream the gateway may reach (SEC-031).
func (s SecurityAdmin) CreateUpstream(ctx context.Context, req *connect.Request[pluxv1.CreateUpstreamRequest]) (*connect.Response[pluxv1.CreateUpstreamResponse], error) {
	if s.h.Upstreams == nil {
		return s.UnimplementedSecurityAdminServiceHandler.CreateUpstream(ctx, req)
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.CreateUpstreamResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		u, err := s.h.Upstreams.Create(ctx, p, upstreamFromProto(req.Msg.GetUpstream()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.CreateUpstreamResponse{Upstream: upstreamProto(u)}, nil
	})
}

// GetUpstream returns one upstream.
func (s SecurityAdmin) GetUpstream(ctx context.Context, req *connect.Request[pluxv1.GetUpstreamRequest]) (*connect.Response[pluxv1.GetUpstreamResponse], error) {
	if s.h.Upstreams == nil {
		return s.UnimplementedSecurityAdminServiceHandler.GetUpstream(ctx, req)
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.GetUpstreamResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		u, err := s.h.Upstreams.Get(ctx, p, req.Msg.GetId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetUpstreamResponse{Upstream: upstreamProto(u)}, nil
	})
}

// ListUpstreams lists an app's upstreams.
func (s SecurityAdmin) ListUpstreams(ctx context.Context, req *connect.Request[pluxv1.ListUpstreamsRequest]) (*connect.Response[pluxv1.ListUpstreamsResponse], error) {
	if s.h.Upstreams == nil {
		return s.UnimplementedSecurityAdminServiceHandler.ListUpstreams(ctx, req)
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.ListUpstreamsResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		q := keyQuery(func(m Item) string { return m.(*pluxv1.Upstream).GetName() })
		page := req.Msg.GetPage()
		clauses, err := q.Parse(page)
		if err != nil {
			return nil, err
		}
		scope := req.Msg.GetAppId() + "/" + req.Msg.GetEnvironmentId()
		after, size, err := s.h.Pages.Request(req.Spec().Procedure, scope, page)
		if err != nil {
			return nil, err
		}
		upstreams, err := s.h.Upstreams.List(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId(), after, size)
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		out := &pluxv1.ListUpstreamsResponse{Page: &pluxv1.PageResult{}}
		var last storage.Cursor
		for _, u := range upstreams {
			last = storage.Cursor{ID: u.EnvironmentID, Key: u.Name}
			if item := upstreamProto(u); q.Keep(clauses, item) {
				out.Upstreams = append(out.Upstreams, item)
			}
		}
		out.Page.NextPageToken = s.h.Pages.Next(req.Spec().Procedure, scope, page, len(upstreams), size, last)
		return out, Mask(page.GetReadMask(), out.Upstreams)
	})
}

// UpdateUpstream replaces an upstream's settings.
func (s SecurityAdmin) UpdateUpstream(ctx context.Context, req *connect.Request[pluxv1.UpdateUpstreamRequest]) (*connect.Response[pluxv1.UpdateUpstreamResponse], error) {
	if s.h.Upstreams == nil {
		return s.UnimplementedSecurityAdminServiceHandler.UpdateUpstream(ctx, req)
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.UpdateUpstreamResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		u, err := s.h.Upstreams.Update(ctx, p, upstreamFromProto(req.Msg.GetUpstream()))
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.UpdateUpstreamResponse{Upstream: upstreamProto(u)}, nil
	})
}

// DeleteUpstream removes an upstream.
func (s SecurityAdmin) DeleteUpstream(ctx context.Context, req *connect.Request[pluxv1.DeleteUpstreamRequest]) (*connect.Response[pluxv1.DeleteUpstreamResponse], error) {
	if s.h.Upstreams == nil {
		return s.UnimplementedSecurityAdminServiceHandler.DeleteUpstream(ctx, req)
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteUpstreamResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Upstreams.Delete(ctx, p, req.Msg.GetId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteUpstreamResponse{}, nil
	})
}

// SetUserIdentityProvider sets the identity provider of an environment's
// users (SEC-026).
func (s SecurityAdmin) SetUserIdentityProvider(ctx context.Context, req *connect.Request[pluxv1.SetUserIdentityProviderRequest]) (*connect.Response[pluxv1.SetUserIdentityProviderResponse], error) {
	if s.h.Upstreams == nil {
		return s.UnimplementedSecurityAdminServiceHandler.SetUserIdentityProvider(ctx, req)
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.SetUserIdentityProviderResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		m := req.Msg.GetProvider()
		err = s.h.Upstreams.SetProvider(ctx, p, gateway.IdentityProvider{
			AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Issuer: m.GetIssuer(), JWKSURL: m.GetJwksUrl(),
			Audiences: m.GetAudiences(), ForwardClaims: m.GetForwardClaims(),
		})
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.SetUserIdentityProviderResponse{}, nil
	})
}

// GetUserIdentityProvider returns the identity provider of an
// environment's users.
func (s SecurityAdmin) GetUserIdentityProvider(ctx context.Context, req *connect.Request[pluxv1.GetUserIdentityProviderRequest]) (*connect.Response[pluxv1.GetUserIdentityProviderResponse], error) {
	if s.h.Upstreams == nil {
		return s.UnimplementedSecurityAdminServiceHandler.GetUserIdentityProvider(ctx, req)
	}
	return read(ctx, func(ctx context.Context) (*pluxv1.GetUserIdentityProviderResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		idp, err := s.h.Upstreams.GetProvider(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId())
		if err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.GetUserIdentityProviderResponse{Provider: &pluxv1.UserIdentityProvider{
			AppId: idp.AppID, EnvironmentId: idp.EnvironmentID, Issuer: idp.Issuer, JwksUrl: idp.JWKSURL,
			Audiences: idp.Audiences, ForwardClaims: idp.ForwardClaims,
		}}, nil
	})
}

// DeleteUserIdentityProvider removes the identity provider of an
// environment's users.
func (s SecurityAdmin) DeleteUserIdentityProvider(ctx context.Context, req *connect.Request[pluxv1.DeleteUserIdentityProviderRequest]) (*connect.Response[pluxv1.DeleteUserIdentityProviderResponse], error) {
	if s.h.Upstreams == nil {
		return s.UnimplementedSecurityAdminServiceHandler.DeleteUserIdentityProvider(ctx, req)
	}
	return mutate(ctx, s.h, req, func(ctx context.Context) (*pluxv1.DeleteUserIdentityProviderResponse, error) {
		p, err := s.h.principal(ctx, req.Header(), "")
		if err != nil {
			return nil, err
		}
		if err := s.h.Upstreams.DeleteProvider(ctx, p, req.Msg.GetAppId(), req.Msg.GetEnvironmentId()); err != nil {
			return nil, err //nolint:wrapcheck // a domain error
		}
		return &pluxv1.DeleteUserIdentityProviderResponse{}, nil
	})
}

// upstreamFromProto converts a registration; a missing message is an
// empty upstream, which validation refuses.
func upstreamFromProto(m *pluxv1.Upstream) gateway.Upstream {
	u := gateway.Upstream{
		ID: m.GetId(), AppID: m.GetAppId(), EnvironmentID: m.GetEnvironmentId(), Name: m.GetName(), BaseURL: m.GetBaseUrl(),
		AllowPrivate: m.GetAllowPrivateAddresses(), Timeout: time.Duration(m.GetTimeoutMs()) * time.Millisecond,
		Methods: m.GetAllowedMethods(), Assurance: m.GetRequiresAssurance(), Audience: m.GetAudience(),
		Auth: gateway.Auth{Kind: gateway.AuthNone},
	}
	switch a := m.GetAuth().(type) {
	case *pluxv1.Upstream_Mtls:
		u.Auth = gateway.Auth{Kind: gateway.AuthMTLS, MTLSSecret: a.Mtls.GetClientCertSecretRef()}
	case *pluxv1.Upstream_OauthClientCredentials:
		o := a.OauthClientCredentials
		u.Auth = gateway.Auth{
			Kind: gateway.AuthOAuth, TokenURL: o.GetTokenUrl(), ClientID: o.GetClientId(), ClientSecret: o.GetClientSecretRef(), Scopes: o.GetScopes(),
		}
	case *pluxv1.Upstream_ApiKey:
		u.Auth = gateway.Auth{Kind: gateway.AuthAPIKey, Header: a.ApiKey.GetHeader(), KeySecret: a.ApiKey.GetSecretRef()}
	}
	for _, op := range m.GetOperations() {
		o := gateway.Operation{
			Name: op.GetName(), Methods: op.GetMethods(), PathPrefix: op.GetPathPrefix(), Assurance: op.GetRequiresAssurance(), Plugins: op.GetPlugins(),
		}
		if t := op.GetTransaction(); t != nil {
			o.Transaction = &gateway.Transaction{Amount: t.GetAmount(), Currency: t.GetCurrency(), Payee: t.GetPayee(), Summary: t.GetSummary()}
		}
		u.Operations = append(u.Operations, o)
	}
	return u
}

// upstreamProto converts an upstream; it carries references to secrets,
// never a secret (SEC-092).
func upstreamProto(u gateway.Upstream) *pluxv1.Upstream {
	m := &pluxv1.Upstream{
		Id: u.ID, AppId: u.AppID, EnvironmentId: u.EnvironmentID, Name: u.Name, BaseUrl: u.BaseURL,
		AllowPrivateAddresses: u.AllowPrivate, TimeoutMs: int32(u.Timeout.Milliseconds()), //nolint:gosec // bounded by gateway.timeout
		AllowedMethods: u.Methods, RequiresAssurance: u.Assurance, Audience: u.Audience,
	}
	switch u.Auth.Kind {
	case gateway.AuthMTLS:
		m.Auth = &pluxv1.Upstream_Mtls{Mtls: &pluxv1.UpstreamMtls{ClientCertSecretRef: u.Auth.MTLSSecret}}
	case gateway.AuthOAuth:
		m.Auth = &pluxv1.Upstream_OauthClientCredentials{OauthClientCredentials: &pluxv1.UpstreamOAuthClientCredentials{
			TokenUrl: u.Auth.TokenURL, ClientId: u.Auth.ClientID, ClientSecretRef: u.Auth.ClientSecret, Scopes: u.Auth.Scopes,
		}}
	case gateway.AuthAPIKey:
		m.Auth = &pluxv1.Upstream_ApiKey{ApiKey: &pluxv1.UpstreamApiKey{Header: u.Auth.Header, SecretRef: u.Auth.KeySecret}}
	}
	for _, op := range u.Operations {
		o := &pluxv1.UpstreamOperation{
			Name: op.Name, Methods: op.Methods, PathPrefix: op.PathPrefix, RequiresAssurance: op.Assurance, Plugins: op.Plugins,
		}
		if t := op.Transaction; t != nil {
			o.Transaction = &pluxv1.UpstreamTransaction{Amount: t.Amount, Currency: t.Currency, Payee: t.Payee, Summary: t.Summary}
		}
		m.Operations = append(m.Operations, o)
	}
	return m
}

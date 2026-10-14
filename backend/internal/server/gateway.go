// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/observability"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// JWKSPath is where the keys that verify assertions are published, per
// class of environment: JWKSPath + "production" or "development"
// (SEC-031).
const JWKSPath = "/.well-known/plux/gateway-jwks/"

// limitsTTL is how long a replica reuses the limits it read for an app.
const limitsTTL = 30 * time.Second

// GatewayParts are the pieces of the secure gateway a process serves
// (SEC-030–SEC-032); nil where the process runs neither the api nor the
// gateway role.
type GatewayParts struct {
	Gateway   *gateway.Gateway
	Upstreams *gateway.Registry
	Exchange  *gateway.Exchanger
	// Configs administers server-held configuration values (DAT-090) and
	// Resolver serves them to the gateway and to devices (DAT-092,
	// DAT-094).
	Configs  *gateway.Configs
	Resolver *gateway.ConfigResolver
	// Keys lists the public token keys the published key sets are made of.
	Keys func(ctx context.Context) ([]signing.TokenKey, error)
	// SCA issues and verifies the challenges of strong customer
	// authentication, which a function invocation shares (FN-020).
	SCA *gateway.SCA
}

// gatewayDeps are what the gateway is built on.
type gatewayDeps struct {
	cfg      *config.Config
	db       *storage.DB
	cache    cache.Cache
	set      limits.Set
	signer   signing.TokenSigner
	crypter  signing.Crypter
	ids      interface{ New() (string, error) }
	audit    *audit.Log
	tenancy  *tenancy.Service
	devices  *device.Service
	configs  *seccfg.Service
	trust    *DeviceTrust
	metrics  *observability.Metrics
	secretOf gateway.SecretReader
	// identityHTTP reads identity providers' key sets; nil is the
	// SSRF-safe client.
	identityHTTP *http.Client
	// functions answers whether a function is deployed in an environment,
	// for the audience of a user token for a function; nil when the
	// installation runs no functions.
	functions *function.Service
}

// buildGateway assembles the gateway for a process with the api or the
// gateway role. It returns nil for any other, which holds no token signer.
func buildGateway(d gatewayDeps) (*GatewayParts, error) {
	if d.trust == nil || d.signer == nil || !d.cfg.ServesHTTP() {
		return nil, nil //nolint:nilnil // the process serves no devices
	}
	registry, err := gateway.NewRegistry(gateway.RegistryOptions{DB: d.db, Audit: d.audit, IDs: d.ids})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	base := strings.TrimSuffix(d.cfg.Server.PublicBaseURL, "/")
	egress, err := newEgress(d)
	if err != nil {
		return nil, err
	}
	keys := d.trust.Tokens.Keys
	configs, err := gateway.NewConfigs(gateway.ConfigOptions{
		DB: d.db, Audit: d.audit, Crypter: d.crypter, Cache: d.cache, Limits: configLimits(d.tenancy, d.set),
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	resolver := &gateway.ConfigResolver{Source: configs, Cache: d.cache}
	users := &gateway.UserTokens{Signer: d.signer, Keys: keys, Issuer: base}
	sca := &gateway.SCA{Cache: d.cache, Keys: d.devices}
	gw, err := gateway.New(gateway.Options{
		Upstreams:  registry,
		Egress:     egress,
		Assertions: &gateway.Assertions{Signer: d.signer, Issuer: base},
		Users:      users,
		SCA:        sca,
		Config:     resolver,
		Limits:     appLimits(d.db, d.tenancy, d.set),
		Count:      d.cache.Increment,
		Production: d.devices.EnvironmentIsProduction,
		Observe:    observeGateway(d.metrics),
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	identityHTTP := d.identityHTTP
	if identityHTTP == nil {
		identityHTTP = httpx.NewClient(httpx.Options{Timeout: 10 * time.Second})
	}
	exchange := &gateway.Exchanger{
		Providers: registry.ProviderFor, Upstreams: registry.Resolve, Production: d.devices.EnvironmentIsProduction,
		Lifetime: tokenLifetimeOf(d.configs, d.cfg), Tokens: users,
		HTTP: identityHTTP,
	}
	if d.functions != nil {
		exchange.Functions = functionDeployed(d.functions)
	}
	return &GatewayParts{Gateway: gw, Upstreams: registry, Exchange: exchange, Configs: configs, Resolver: resolver, Keys: keys, SCA: sca}, nil
}

// functionDeployed checks that a function has a version in the device's
// environment, which is all the exchange of a user token for that function
// needs to know (SEC-026).
func functionDeployed(s *function.Service) func(context.Context, device.Identity, string) error {
	return func(ctx context.Context, d device.Identity, name string) error {
		_, err := s.ResolveVersion(ctx, d.OrganizationID, d.AppID, d.EnvironmentID, name, "")
		return err //nolint:wrapcheck // a domain error
	}
}

// configLimits returns the limits in force for an app inside a
// transaction, for the configuration store (LIM-002).
func configLimits(ten *tenancy.Service, installation limits.Set) func(context.Context, pgx.Tx, string, string) (limits.Set, error) {
	return func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error) {
		if ten == nil {
			return installation, nil
		}
		return ten.Effective(ctx, tx, organizationID, appID, "") //nolint:wrapcheck // a domain error
	}
}

// observeGateway records a finished gateway call as an RPC of the
// gateway: the upstream stands in the method label and the status in the
// code label, so the call shows in the catalogued metrics with upstream,
// status and duration, and with nothing else (SEC-031, OBS-002).
func observeGateway(m *observability.Metrics) func(string, int, time.Duration) {
	if m == nil {
		return nil
	}
	return func(upstream string, status int, d time.Duration) {
		m.RPC("plux.gateway", upstream, strconv.Itoa(status), d)
	}
}

// tokenLifetimeOf returns how long a user token lives for a device: the
// access token lifetime of its environment's security configuration, so
// that the user token never outlives the device's own token.
func tokenLifetimeOf(configs *seccfg.Service, cfg *config.Config) func(context.Context, device.Identity) (time.Duration, error) {
	return func(ctx context.Context, d device.Identity) (time.Duration, error) {
		life := cfg.Auth.Device.AccessTokenTTL.Duration()
		if configs == nil {
			return life, nil
		}
		v, _, err := configs.Effective(ctx, d.AppID, d.EnvironmentID)
		if err != nil {
			return 0, fmt.Errorf("server: the security configuration: %w", err)
		}
		if s := time.Duration(v.Get(settings.AccessTokenLifetime).Int()) * time.Second; s > 0 && (life <= 0 || s < life) {
			life = s
		}
		return life, nil
	}
}

// appLimits returns the limits in force for the app a device belongs to,
// which an organisation or the app may have tightened (LIM-002). A replica
// reuses what it read for limitsTTL.
func appLimits(db *storage.DB, ten *tenancy.Service, installation limits.Set) func(context.Context, device.Identity) (limits.Set, error) {
	type entry struct {
		set   limits.Set
		until time.Time
	}
	var (
		mu      sync.Mutex
		entries = map[string]entry{}
	)
	return func(ctx context.Context, d device.Identity) (limits.Set, error) {
		now := time.Now()
		mu.Lock()
		e, ok := entries[d.AppID]
		mu.Unlock()
		if ok && now.Before(e.until) {
			return e.set, nil
		}
		set := installation
		if ten != nil {
			err := db.InTx(ctx, storage.Tenant{OrganizationID: d.OrganizationID}, func(ctx context.Context, tx pgx.Tx) error {
				var err error
				set, err = ten.Effective(ctx, tx, d.OrganizationID, d.AppID, "")
				return err //nolint:wrapcheck // a domain error
			})
			if err != nil {
				return limits.Set{}, err //nolint:wrapcheck // InTx wraps its own failures
			}
		}
		mu.Lock()
		if len(entries) >= 4096 {
			entries = map[string]entry{}
		}
		entries[d.AppID] = entry{set: set, until: now.Add(limitsTTL)}
		mu.Unlock()
		return set, nil
	}
}

// registerGateway mounts the gateway's services, its streaming route and
// the published key sets on the mux, behind the same device
// authentication as the rest of the device API.
func (s *Server) registerGateway(parts *GatewayParts, h *api.Handlers, deviceAuth api.DeviceAuth, opts connect.HandlerOption) {
	for _, register := range []func() (string, http.Handler){
		func() (string, http.Handler) { return pluxv1connect.NewGatewayServiceHandler(h.GatewayService(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewScaServiceHandler(h.ScaService(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewConfigServiceHandler(h.ConfigService(), opts) },
	} {
		path, handler := register()
		s.Register(path, handler)
	}
	s.Register(api.GatewayRoutePrefix, api.GatewayRoute{Auth: deviceAuth, Gateway: parts.Gateway})
	s.Register("GET "+JWKSPath+"{class}", s.jwksHandler(parts))
}

// jwksHandler serves the key set of one class of environment.
func (s *Server) jwksHandler(parts *GatewayParts) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var class signing.TokenClass
		switch r.PathValue("class") {
		case "production":
			class = signing.TokenProduction
		case "development":
			class = signing.TokenDevelopment
		default:
			http.NotFound(w, r)
			return
		}
		keys, err := parts.Keys(r.Context())
		if err != nil {
			s.log.ErrorContext(r.Context(), "gateway key set unavailable", "error", err.Error())
			http.Error(w, "the keys are unavailable", http.StatusServiceUnavailable)
			return
		}
		body, err := gateway.JWKS(keys, class)
		if err != nil {
			http.Error(w, "the keys are unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/jwk-set+json")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_, _ = w.Write(body)
	})
}

// secrets lets the gateway open the environment secrets that hold an
// upstream's credentials (SEC-106).
type secrets struct{ t *tenancy.Service }

// OpenSecret opens a secret of an environment.
func (s secrets) OpenSecret(ctx context.Context, organizationID, environmentID, key string) ([]byte, error) {
	return s.t.OpenEnvironmentSecret(ctx, organizationID, environmentID, key) //nolint:wrapcheck // a domain error
}

// newEgress builds the gateway's connections to upstreams, trusting the
// system's authorities and, if configured, the file of extra ones
// (gateway.upstreamCAFile).
func newEgress(d gatewayDeps) (*gateway.Egress, error) {
	e := gateway.NewEgress(d.secretOf, nil)
	file := d.cfg.Gateway.UpstreamCAFile
	if file == "" {
		return e, nil
	}
	pem, err := os.ReadFile(file) //nolint:gosec // the operator names the file
	if err != nil {
		return nil, fmt.Errorf("server: gateway.upstreamCAFile: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("server: gateway.upstreamCAFile holds no PEM certificate")
	}
	return e.WithRoots(pool), nil
}

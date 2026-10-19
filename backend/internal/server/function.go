// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/buildclient"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/observability"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/idempotency"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// functionQueue enqueues function builds (FN-016).
type functionQueue struct{ q *JobQueue }

// Enqueue adds a build job on the function queue in the caller's
// transaction.
func (f functionQueue) Enqueue(ctx context.Context, tx pgx.Tx, job function.BuildJob) error {
	return f.q.insert(ctx, tx, job, jobs.QueueFunction)
}

// functionBuildWorker runs function.build jobs.
// FN-016: after the last attempt the build is failed, so that none stays
// running for ever.
type functionBuildWorker struct {
	river.WorkerDefaults[function.BuildJob]
	runner buildRunner
	log    *slog.Logger
}

// buildRunner is what the worker needs of the function service.
type buildRunner interface {
	RunBuild(ctx context.Context, job function.BuildJob) error
	FailBuild(ctx context.Context, job function.BuildJob, attempts int) error
}

// Work runs one build; on the job's last attempt a failure also fails the
// build with a diagnostic.
func (w *functionBuildWorker) Work(ctx context.Context, job *river.Job[function.BuildJob]) error {
	err := w.runner.RunBuild(ctx, job.Args)
	if err == nil || job.Attempt < job.MaxAttempts {
		return err //nolint:wrapcheck // a domain error
	}
	w.log.ErrorContext(ctx, "a function build gave up", slog.String("build", job.Args.BuildID),
		slog.Int("attempts", job.Attempt), slog.String("error", err.Error()))
	if failErr := w.runner.FailBuild(ctx, job.Args, job.Attempt); failErr != nil {
		return errors.Join(err, failErr)
	}
	return err //nolint:wrapcheck // a domain error
}

// buildFunctions assembles the function service, or returns nil when the
// installation has no object store to keep sources and modules in. The
// service has no Builder and no ArtifactSigner here: the worker role that
// runs builds is given them where it is assembled.
func buildFunctions(db *storage.DB, work WorkDeps, log *audit.Log, ten *tenancy.Service, configs *seccfg.Service, gen interface{ New() (string, error) }, retention time.Duration, link *runnerLink, notifier *notify.Service) (*function.Service, error) {
	if work.Objects == nil {
		return nil, nil //nolint:nilnil // the installation runs no functions
	}
	var signer function.ArtifactSigner
	if work.Builder != nil && work.Signer != nil {
		// FN-009: only the worker holds a signing.Signer, so only it can
		// sign the statement of a build.
		s, err := buildclient.NewSigner(work.Signer, work.FunctionKey)
		if err != nil {
			return nil, fmt.Errorf("server: %w", err)
		}
		signer = s
	}
	o := function.Options{
		DB: db, Audit: log, Objects: work.Objects, IDs: gen, Jobs: functionQueue{work.Queue},
		Builder: work.Builder, Signer: signer, Linker: work.Linker, Retention: retention, Log: work.Log,
		Secrets: notifier, Notify: notifier,
		DeviceFunctions: func(ctx context.Context, appID, environmentID string) (bool, error) {
			values, _, err := configs.Effective(ctx, appID, environmentID)
			if err != nil {
				return false, err //nolint:wrapcheck // a domain error
			}
			return values.Get(settings.DeviceFunctions).Bool(), nil
		},
		Limits: func(ctx context.Context, tx pgx.Tx, organizationID, appID string) (limits.Set, error) {
			return ten.Effective(ctx, tx, organizationID, appID, "") //nolint:wrapcheck // a domain error
		},
	}
	if link != nil {
		// FN-023: a deploy asks the runner to compile the module, so the
		// first invocation finds it compiled.
		o.Preparer = function.ConnectPreparer{Client: link.client, Grants: link.grants}
		if work.Metrics != nil {
			o.PrepareMetrics = work.Metrics
		}
	}
	svc, err := function.NewService(o)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return svc, nil
}

// runnerLink is the api role's connection to the function runner: the
// client that calls it over mutual TLS with the role's internal identity
// (SEC-043, FN-020) and the grants that authorise what it fetches back.
type runnerLink struct {
	client fnpbconnect.FunctionRunnerServiceClient
	grants *function.Grants
}

// buildRunnerLink opens the grants and the runner client, or returns nil
// where the process is not the api role, keeps no functions or has no
// runner URL configured. The grants' secret is created on first use and
// shared by every replica (ADR-0064).
func buildRunnerLink(ctx context.Context, cfg *config.Config, db *storage.DB, crypter signing.Crypter, work WorkDeps) (*runnerLink, error) {
	if work.Objects == nil || !(cfg.Has(config.RoleAPI) || cfg.Has(config.RoleWorker)) || cfg.Functions.RunnerURL == "" {
		return nil, nil //nolint:nilnil // the process calls no runner
	}
	grants, err := function.OpenGrants(ctx, db, crypter, nil)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	t := cfg.Server.InternalTLS
	client, err := httpx.NewInternalClient(httpx.InternalTLSOptions{CAFile: t.CAFile, CertFile: t.CertFile, KeyFile: t.KeyFile}, runnerTimeout)
	if err != nil {
		return nil, fmt.Errorf("server: functions: %w", err)
	}
	return &runnerLink{client: fnpbconnect.NewFunctionRunnerServiceClient(client, cfg.Functions.RunnerURL), grants: grants}, nil
}

// serverSecrets reads the secrets a function declares from the
// environment's server-held values (ADR-0061, ADR-0064).
type serverSecrets struct{ r *gateway.ConfigResolver }

// Secret returns the text of a server-bound value of the environment.
func (s serverSecrets) Secret(ctx context.Context, organizationID, appID, environmentID, name string) (string, bool, error) {
	return s.r.ServerValue(ctx, organizationID, appID, environmentID, name) //nolint:wrapcheck // a domain error
}

// functionDeps are what the api role's function runtime is built on.
type functionDeps struct {
	cfg     *config.Config
	db      *storage.DB
	crypter signing.Crypter
	cache   cache.Cache
	set     limits.Set
	service *function.Service
	// link is the connection to the runner, nil where none is configured.
	link    *runnerLink
	gateway *GatewayParts
	tenancy *tenancy.Service
	configs *seccfg.Service
	devices *device.Service
	keys    *idempotency.Store
	metrics *observability.Metrics
}

// functionRuntime is the api role's side of server functions.
type functionRuntime struct {
	host    *function.Host
	invoker *function.Invoker
}

// buildFunctionRuntime assembles the api role's side of server functions:
// the host service the runner calls, and, where the runner's URL is
// configured, the invoker behind FunctionService. It returns nil where the
// process is not the api role or has no function service. The grants'
// secret is created on first use and shared by every replica (ADR-0064).
func buildFunctionRuntime(ctx context.Context, d functionDeps) (*functionRuntime, error) {
	if d.service == nil || !d.cfg.Has(config.RoleAPI) {
		return nil, nil //nolint:nilnil // the process serves no functions
	}
	var grants *function.Grants
	if d.link != nil {
		grants = d.link.grants
	} else {
		var err error
		if grants, err = function.OpenGrants(ctx, d.db, d.crypter, nil); err != nil {
			return nil, fmt.Errorf("server: %w", err)
		}
	}
	o := function.HostOptions{Service: d.service, Grants: grants}
	if d.gateway != nil && d.gateway.Resolver != nil {
		o.Secrets = serverSecrets{r: d.gateway.Resolver}
	}
	rt := &functionRuntime{}
	if d.link != nil {
		inv, err := buildInvoker(d, d.link)
		if err != nil {
			return nil, err
		}
		rt.invoker, o.Invoker = inv, inv
	}
	var err error
	if rt.host, err = function.NewHost(o); err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return rt, nil
}

// runnerTimeout is the backstop of a call to the runner; the invocation's
// own deadline ends a call sooner.
const runnerTimeout = 2 * time.Minute

// buildInvoker assembles the invoker that calls the runner over the link
// (SEC-043, FN-020).
func buildInvoker(d functionDeps, link *runnerLink) (*function.Invoker, error) {
	appSet := appLimits(d.db, d.tenancy, d.set)
	o := function.InvokerOptions{
		Declarations: d.service, Resolver: d.service, Pins: d.service, Environment: d.service, Grants: link.grants,
		Runner: function.ConnectRunner{Client: link.client},
		Keys:   d.keys,
		Limits: func(ctx context.Context, organizationID, appID string) (limits.Set, error) {
			return appSet(ctx, device.Identity{OrganizationID: organizationID, AppID: appID})
		},
		Count: d.cache.Increment,
		DefaultAssurance: func(ctx context.Context, appID, environmentID string) (string, error) {
			values, _, err := d.configs.Effective(ctx, appID, environmentID)
			if err != nil {
				return "", err //nolint:wrapcheck // a domain error
			}
			return values.Get(settings.DefaultFunctionAssurance).Text(), nil
		},
		Attestations: d.devices,
		Logs:         d.service, Observer: d.service,
	}
	if d.metrics != nil {
		o.Metrics = d.metrics
	}
	if d.gateway != nil && d.gateway.SCA != nil {
		o.SCA = functionSCA{sca: d.gateway.SCA}
	}
	if d.gateway != nil && d.gateway.Exchange != nil {
		o.Users = functionUsers{tokens: d.gateway.Exchange.Tokens, production: d.devices.EnvironmentIsProduction}
	}
	inv, err := function.NewInvoker(o)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return inv, nil
}

// functionSCA lets the invoker use the gateway's strong customer
// authentication: a function invocation is a transaction whose operation is
// the function and its version and whose bound body is the input (FN-020,
// SEC-027, SEC-028).
type functionSCA struct{ sca *gateway.SCA }

// CreateInvocationChallenge issues the challenge for an input.
func (f functionSCA) CreateInvocationChallenge(ctx context.Context, d device.Identity, name string, version int32, inputSHA256, summary []byte, lifetime time.Duration) (function.SCAChallenge, error) {
	c, err := f.sca.CreateInvocation(ctx, d, gateway.FunctionTransaction{Name: name, Version: version}, inputSHA256, summary, lifetime)
	if err != nil {
		return function.SCAChallenge{}, err //nolint:wrapcheck // a domain error
	}
	return function.SCAChallenge{ID: c.ID, Bytes: c.Bytes, ExpiresAt: c.ExpiresAt, Summary: c.Summary}, nil
}

// VerifyInvocation spends the challenge and checks the signature and that
// the input is the one the user confirmed.
func (f functionSCA) VerifyInvocation(ctx context.Context, d device.Identity, p function.SCAProof, lifetime time.Duration) error {
	return f.sca.VerifyInvocation(ctx, d, gateway.FunctionTransaction{Name: p.Function, Version: p.Version}, p.Input, p.ChallengeID, p.Signature, lifetime) //nolint:wrapcheck // a domain error
}

// functionUsers verifies the user token a device presents to a function
// (SEC-026). The token is bound to the device, the app, the environment
// and the audience the exchange named, which for a function is
// function:<name> (gateway.FunctionAudience), so that a token for an
// upstream is of no use to a function.
type functionUsers struct {
	tokens     *gateway.UserTokens
	production func(ctx context.Context, environmentID string) (bool, error)
}

// VerifyUser returns the verified user as JSON.
func (u functionUsers) VerifyUser(ctx context.Context, token string, d device.Identity, name string) ([]byte, error) {
	production, err := u.production(ctx, d.EnvironmentID)
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	user, err := u.tokens.Verify(ctx, token, gateway.BoundTo{
		DeviceID: d.DeviceID, AppID: d.AppID, EnvironmentID: d.EnvironmentID, Upstream: gateway.FunctionAudience(name), Production: production,
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // a domain error
	}
	raw, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("server: encode the user's claims: %w", err)
	}
	return raw, nil
}

// hostEnvelopeBytes is the room, beyond the largest function input, that a
// host call needs for its framing.
const hostEnvelopeBytes = 1 << 20

// registerFunctionHost mounts FunctionHostService on the internal
// listener, where only a role with a certificate of the internal CA can
// reach it (SEC-043). Nothing is mounted without a host.
func (s *Server) registerFunctionHost(h *api.Handlers) {
	if h.FunctionHosting == nil {
		return
	}
	opts := connect.WithHandlerOptions(
		connect.WithInterceptors(s.InternalInterceptors()...),
		connect.WithReadMaxBytes(int(min(s.limits.Get(limits.FunctionInputSize)+hostEnvelopeBytes, 1<<31-1))),
	)
	path, handler := fnpbconnect.NewFunctionHostServiceHandler(h.FunctionHost(), opts)
	s.RegisterInternal(path, handler)
}

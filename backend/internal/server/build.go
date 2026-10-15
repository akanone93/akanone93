// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/compiler/media"

	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/observability"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// Built is an assembled process together with the function that
// releases everything it opened, in the reverse order it was opened.
type Built struct {
	Server *Server
	Close  func()
}

// BuildOption changes how Build assembles a process.
type BuildOption func(*buildOptions)

type buildOptions struct {
	identityHTTP *http.Client
	builder      function.Builder
	linker       function.Linker
}

// WithIdentityProviderClient makes the gateway read the key sets of
// customers' identity providers (SEC-026) with c instead of the SSRF-safe
// client (SEC-105). It is for embedders and tests whose identity provider
// sits on a private address or behind a private authority; a process that
// serves customers never sets it.
func WithIdentityProviderClient(c *http.Client) BuildOption {
	return func(o *buildOptions) { o.identityHTTP = c }
}

// WithFunctionBuilder gives the worker role b to build function sources and
// l to link the device functions of a plugin, in place of the route to the
// builder role that builder.address configures (FN-016, FN-051). The worker
// still signs what they return with the function-artifact key, and the
// service still judges it. It is for embedders and tests that stand in for
// the builder role with artefacts built before; a process that serves
// customers never sets it.
func WithFunctionBuilder(b function.Builder, l function.Linker) BuildOption {
	return func(o *buildOptions) { o.builder, o.linker = b, l }
}

// newTracer starts the process's tracing.
func newTracer(ctx context.Context, cfg *config.Config, version string) (*observability.Tracer, error) {
	return observability.NewTracer(ctx, observability.TraceOptions{ //nolint:wrapcheck // reported as is by Build
		Endpoint:    cfg.Observability.OTLPEndpoint,
		SampleRatio: cfg.Observability.TraceSampleRatio,
		Service:     "plux-server",
		Role:        RoleLabel(cfg),
		Version:     version,
	})
}

// newNoDataServer assembles a process whose only roles are egress or builder:
// it holds no database, cache, storage or signing credential (ADR-0063,
// ADR-0064).
func newNoDataServer(cfg *config.Config, log *slog.Logger, metrics *observability.Metrics, tracer *observability.Tracer) (*Server, error) {
	limitSet, err := cfg.LimitSet()
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return New(Deps{Config: cfg, Log: log, Metrics: metrics, Tracer: tracer, Limits: limitSet})
}

// Build opens the dependencies the configured roles need and assembles
// a Server. The role decides what is built at all: a process without
// the worker role gets no job client that runs, and — from the next
// milestone — no signing client (L-3, L-4, ADR-0006).
func Build(ctx context.Context, cfg *config.Config, log *slog.Logger, version string, opts ...BuildOption) (*Built, error) {
	var o buildOptions
	for _, apply := range opts {
		apply(&o)
	}
	var closers []func()
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	fail := func(err error) (*Built, error) {
		closeAll()
		return nil, err
	}

	metrics := observability.NewMetrics()
	tracer, err := newTracer(ctx, cfg, version)
	if err != nil {
		return fail(err)
	}

	if cfg.NoDataAccess() {
		// fnrunner, egress and builder hold no database, cache, storage or
		// signing credential (FN-013, FN-016, ADR-0063, ADR-0064).
		srv, err := newNoDataServer(cfg, log, metrics, tracer)
		if err != nil {
			return fail(err)
		}
		return &Built{Server: srv, Close: closeAll}, nil
	}

	db, err := storage.Open(ctx, storage.Options{
		URL:            cfg.Database.URL.Value(),
		MaxConnections: cfg.Database.MaxConnections,
		ConnectTimeout: 15 * time.Second,
		Log:            log,
	})
	if err != nil {
		return fail(err)
	}
	closers = append(closers, db.Close)

	if cfg.Database.MigrateOnStart {
		if err := migrateAll(ctx, db); err != nil {
			return fail(err)
		}
	}

	store, err := buildObjects(ctx, cfg)
	if err != nil {
		return fail(err)
	}

	shared, err := buildCache(cfg)
	if err != nil {
		return fail(err)
	}
	closers = append(closers, func() { _ = shared.Close() })

	limitSet, err := cfg.LimitSet()
	if err != nil {
		return fail(err)
	}

	services, jobClient, err := buildWork(ctx, cfg, log, metrics, db, shared, limitSet, store, o)
	if err != nil {
		return fail(err)
	}

	srv, err := New(Deps{
		Config: cfg, Log: log, Metrics: metrics, Tracer: tracer,
		DB: db, Objects: store, Cache: shared, Jobs: jobClient, Limits: limitSet,
		KMS: services.KMS,
	})
	if err != nil {
		return fail(err)
	}
	if err := registerRole(cfg, srv, services); err != nil {
		return fail(err)
	}
	return &Built{Server: srv, Close: closeAll}, nil
}

// registerRole mounts the handlers of the process's serving role: the
// API's, or the gateway's alone.
func registerRole(cfg *config.Config, srv *Server, services *Services) error {
	switch {
	case cfg.Has(config.RoleAPI):
		return srv.RegisterAPI(services)
	case cfg.Has(config.RoleGateway):
		return srv.RegisterGateway(services)
	}
	return nil
}

// buildWork assembles the domain services and the job client that runs
// their background work (SRV-024).
func buildWork(ctx context.Context, cfg *config.Config, log *slog.Logger, metrics *observability.Metrics, db *storage.DB, shared cache.Cache, set limits.Set, store objects.Store, o buildOptions) (*Services, *jobs.Client, error) {
	backend, err := BuildSigning(cfg)
	if err != nil {
		return nil, nil, err
	}
	if !backend.AllowedInProduction() {
		log.WarnContext(ctx, "the file signing backend keeps keys on disk; it is for development only, and production environments refuse it (SEC-056, SEC-120)",
			slog.String("directory", cfg.Signing.Directory))
	}
	queue := &JobQueue{}
	deps := WorkDeps{Objects: store, Queue: queue, Log: log, Metrics: metrics, IdentityHTTP: o.identityHTTP}
	if cfg.Has(config.RoleWorker) {
		// Only the worker transcodes and signs: compiling the codecs costs
		// seconds of CPU the api role need not pay, and the api role must
		// never hold a signer (SRV-052, ADR-0006). They compile while the
		// server starts and serves; asset jobs wait for them.
		codecs := media.StartCodecs(ctx)
		deps.Codecs = codecs
		go func() {
			if err := codecs.Ready(ctx); err != nil && ctx.Err() == nil {
				log.ErrorContext(ctx, "the image codecs failed to compile; asset jobs will fail", slog.Any("error", err))
			}
		}()
		deps.SVG = svgCompiler(ctx, cfg, log)
		deps.Signer = backend
		deps.ProductionSigning = backend.AllowedInProduction()
		if err := attachBuilder(cfg, store, &deps); err != nil {
			return nil, nil, err
		}
		if o.builder != nil {
			deps.Builder, deps.Linker, deps.FunctionKey = o.builder, o.linker, cfg.Signing.Keys.FunctionArtifact
		}
	}
	services, err := BuildServices(ctx, cfg, db, shared, set, backend, deps)
	if pinger, ok := backend.(interface{ Ping(context.Context) error }); ok && err == nil {
		services.KMS = pinger.Ping
	}
	if err != nil {
		return nil, nil, err
	}
	workers := jobs.NewWorkers()
	if deps.Codecs != nil {
		jobs.AddWorker(workers, &assetWorker{svc: services})
	}
	if deps.Signer != nil && services.Releases != nil {
		jobs.AddWorker(workers, &publishWorker{svc: services})
		jobs.AddWorker(workers, &manifestWorker{svc: services})
		jobs.AddWorker(workers, &deltaWorker{svc: services})
	}
	if services.Functions != nil && deps.Builder != nil && deps.Signer != nil {
		jobs.AddWorker(workers, &functionBuildWorker{runner: services.Functions, log: log})
	}
	periodic := append(MaintenanceJobs(workers, services, log),
		CheckpointJobs(workers, services, cfg.Audit.CheckpointInterval.Duration(), log)...)
	periodic = append(periodic, AttestationStatusJobs(workers, services, log)...)
	jobClient, err := jobs.New(jobs.Options{
		Pool:     db.Pool(),
		Workers:  workers,
		Run:      cfg.Has(config.RoleWorker),
		Periodic: periodic,
		Log:      log,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("server: %w", err)
	}
	queue.client = jobClient
	return services, jobClient, nil
}

// migrateAll applies the server's migrations and River's (SRV-021).
func migrateAll(ctx context.Context, db *storage.DB) error {
	migrations, err := storage.LoadMigrations(storage.Migrations())
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	if err := db.Migrate(ctx, migrations); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	if err := jobs.Migrate(ctx, db.Pool()); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// buildObjects opens the configured object store (SRV-023).
func buildObjects(ctx context.Context, cfg *config.Config) (objects.Store, error) {
	o := cfg.ObjectStorage
	switch o.Backend {
	case "filesystem":
		store, err := objects.NewFilesystem(o.Directory, o.CDNBaseURL)
		if err != nil {
			return nil, fmt.Errorf("server: object storage: %w", err)
		}
		return store, nil
	case "s3":
		store, err := objects.NewS3(ctx, objects.S3Options{
			Endpoint:        o.Endpoint,
			Region:          o.Region,
			Bucket:          o.Bucket,
			PathStyle:       o.PathStyle,
			AccessKeyID:     o.AccessKeyID,
			SecretAccessKey: o.SecretAccessKey.Value(),
			CDNBaseURL:      o.CDNBaseURL,
		})
		if err != nil {
			return nil, fmt.Errorf("server: object storage: %w", err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("server: unknown object storage backend %q", o.Backend)
	}
}

// buildCache opens the configured shared cache.
func buildCache(cfg *config.Config) (cache.Cache, error) {
	switch cfg.Cache.Backend {
	case "memory":
		return cache.NewMemory(nil), nil
	case "valkey":
		shared, err := cache.NewValkey(cfg.Cache.ValkeyURL.Value())
		if err != nil {
			return nil, fmt.Errorf("server: cache: %w", err)
		}
		return shared, nil
	default:
		return nil, fmt.Errorf("server: unknown cache backend %q", cfg.Cache.Backend)
	}
}

// RoleLabel renders the roles of a process, such as "api+worker", for
// logs and the trace resource.
func RoleLabel(cfg *config.Config) string {
	parts := make([]string, len(cfg.Server.Roles))
	for i, r := range cfg.Server.Roles {
		parts[i] = string(r)
	}
	return strings.Join(parts, "+")
}

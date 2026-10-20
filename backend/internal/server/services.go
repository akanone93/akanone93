// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/compiler/media"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
	"github.com/nightCode42/plux3/backend/internal/updatemeta"

	"connectrpc.com/connect"
	"github.com/riverqueue/river"

	"github.com/nightCode42/plux3/backend/ext/export"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/backend/internal/analytics"
	"github.com/nightCode42/plux3/backend/internal/api"
	"github.com/nightCode42/plux3/backend/internal/attest/statuslist"
	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auditexport"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/buildinfo"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/device"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/gitsync"
	"github.com/nightCode42/plux3/backend/internal/golden"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/jobs"
	"github.com/nightCode42/plux3/backend/internal/localisation"
	"github.com/nightCode42/plux3/backend/internal/notify"
	"github.com/nightCode42/plux3/backend/internal/observability"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/push"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/scheduler"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/schema/uuid7"
	"github.com/nightCode42/plux3/backend/internal/search"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
	"github.com/nightCode42/plux3/backend/internal/signing"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/idempotency"
	"github.com/nightCode42/plux3/backend/internal/telemetry"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// Services are the domain services of this phase, built once per
// process and shared by the api role's handlers and the worker role's
// jobs. They hold envelope encryption only: signing is the worker's, and
// arrives with the publish pipeline.
type Services struct {
	Audit     *audit.Log
	Auth      *auth.Service
	Tenancy   *tenancy.Service
	Documents *document.Service
	Releases  *release.Service
	Devices   *device.Service
	Events    *telemetry.Service
	// Analytics queries rollups, manages retention and the OTLP export, and
	// does the worker's analytics duties (ANL-010, ANL-011).
	Analytics *analytics.Service
	// EventConsumer writes queued telemetry batches; nil unless the process
	// is a worker and the cache is Valkey (ADR-0080).
	EventConsumer *telemetry.Consumer
	Idempotency   *idempotency.Store
	Pages         *api.Pages
	// Identity is the SAML and SCIM implementation the server serves
	// (GOV-004); the community defaults refuse with PLX-8600.
	Identity identity.Extensions
	// AuditCheckpoints lists the signed audit checkpoints (SEC-141).
	AuditCheckpoints *audit.CheckpointReader
	// Checkpointer signs them; nil where the process holds no signer,
	// which is every process without the worker role (ADR-0006).
	Checkpointer *audit.Checkpointer
	// SecurityConfig holds the environments' security configurations
	// (SEC-182).
	SecurityConfig *seccfg.Service
	// Functions administers functions (FN-006, FN-016); nil where the
	// installation has no object store.
	Functions *function.Service
	// FunctionRuntime is the api role's side of server functions: the
	// runner's host service and the invoker behind FunctionService; nil
	// where the process is not the api role (ADR-0064).
	FunctionRuntime *functionRuntime
	// AttestationStatus is Google's attestation status list, which the
	// worker role keeps current (SEC-002).
	AttestationStatus *statuslist.List
	// Metrics is the process's metrics; nil records none.
	Metrics *observability.Metrics
	// DeviceTrust authenticates device calls; nil where the process does
	// not run the api or the gateway role.
	DeviceTrust *DeviceTrust
	// Gateway is the secure gateway; nil where the process runs neither
	// role (SEC-030).
	Gateway *GatewayParts
	// Notify delivers notifications and outbound webhooks (SRV-062,
	// SRV-063); other services publish to it.
	Notify *notify.Service
	// Push stores device push tokens and sends the silent push of control
	// document changes (SYN-060); off unless push.enabled is set.
	Push *push.Service
	// Scheduler drains the notification queues and fires function
	// schedules on the elected leader; nil outside the worker role
	// (FN-025).
	Scheduler *scheduler.Scheduler
	// Goldens holds golden-test baselines and runs (TST-003); nil where the
	// installation has no object store.
	Goldens *golden.Service
	// AuditExport administers the audit export destinations (SEC-143).
	AuditExport *auditexport.Service
	// AuditExporter delivers the audit chains to them; nil where the process
	// holds no signer, which is every process without the worker role.
	AuditExporter *auditexport.Exporter
	// Search is the full-text search (SRV-061).
	Search *search.Service
	// GitExport exports apps to Git on publish (GOV-011).
	GitExport *gitsync.Service
	// KMS checks the key management service, when the signing backend
	// is one; nil for the development file backend (SRV-007).
	KMS func(context.Context) error
}

// WorkDeps are what the services need beyond the database: where files
// are stored, the job queue, and — in the worker role only — the codecs
// and the signer (SRV-052, ADR-0006). The zero value refuses uploads and
// leaves releases out.
type WorkDeps struct {
	// AnalyticsStore is the enterprise edition's analytics store; nil uses
	// PostgreSQL (ANL-010).
	AnalyticsStore analytics.Store
	Objects        objects.Store
	Queue          *JobQueue
	Codecs         *media.Codecs
	// SVG compiles SVGs; nil where the server has no SVG compiler.
	SVG    *media.SVGCompiler
	Signer signing.Signer
	// ProductionSigning reports whether the signer may sign for
	// production environments (SEC-056).
	ProductionSigning bool
	// Log receives the warnings of the services; nil uses the default
	// logger.
	Log *slog.Logger
	// Builder compiles function sources on the builder role; only the
	// worker role is given one, and only where builder.address is set.
	Builder function.Builder
	// Linker links the device functions of a plugin into one module on the
	// builder role (FN-051); set with Builder.
	Linker function.Linker
	// FunctionKey is the key reference that signs the statements of
	// function builds (signing.keys.functionArtifact).
	FunctionKey string
	// AuditSinks builds the enterprise audit export sinks; nil is the
	// community edition, which exports over syslog only (SEC-143).
	AuditSinks export.Provider
	// AuditNotifier receives the report of an unreachable audit export
	// destination (PLX-8500); nil reports none.
	AuditNotifier auditexport.Notifier
	// Metrics receives the device side's metrics, the DPoP replay cache's
	// health among them; nil records none.
	Metrics *observability.Metrics
	// IdentityHTTP reads identity providers' key sets; nil is the
	// SSRF-safe client (SEC-105).
	IdentityHTTP *http.Client
	// Identity holds the enterprise implementations of SAML and SCIM
	// (GOV-004); a nil field is the community default.
	Identity identity.Extensions
}

// scanner returns the configured malware scanner, or nil (SRV-060).
func scanner(cfg *config.Config) document.Scanner {
	u, err := url.Parse(cfg.Assets.MalwareScanner)
	if cfg.Assets.MalwareScanner == "" || err != nil {
		return nil
	}
	if u.Scheme == "unix" {
		return document.Clamd{Network: "unix", Address: u.Path}
	}
	return document.Clamd{Network: "tcp", Address: u.Host}
}

// svgCompiler returns the configured SVG compiler, or nil — logged — when
// it is not configured or not installed, which fails SVG assets (CMP-031).
func svgCompiler(ctx context.Context, cfg *config.Config, log *slog.Logger) *media.SVGCompiler {
	a := cfg.Assets
	if a.SVGCompiler == "" {
		log.WarnContext(ctx, "no SVG compiler is configured (assets.svgCompiler); SVG assets will fail")
		return nil
	}
	for _, path := range []string{a.SVGCompiler, a.PathOps} {
		if _, err := os.Stat(path); err != nil {
			log.WarnContext(ctx, "the SVG compiler is not installed; SVG assets will fail", slog.String("path", path), slog.Any("error", err))
			return nil
		}
	}
	return &media.SVGCompiler{Path: a.SVGCompiler, PathOps: a.PathOps, Timeout: svgTimeout, MaxOutput: svgMaxOutput}
}

// The bounds of one SVG compilation: far above what an icon or an
// illustration needs, far below what would hold up the asset queue.
const (
	svgTimeout   = 30 * time.Second
	svgMaxOutput = 16 << 20
)

// JobQueue enqueues the services' jobs on the job client, which is built
// after the services it runs jobs for; Build sets it before serving.
type JobQueue struct{ client *jobs.Client }

// insert adds a job on a queue in the caller's transaction.
func (q *JobQueue) insert(ctx context.Context, tx pgx.Tx, args river.JobArgs, queue string) error {
	if q == nil || q.client == nil {
		return errors.New("server: the job client is not ready")
	}
	if _, err := q.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: queue, MaxAttempts: 5}); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// assetQueue enqueues transcoding jobs (CMP-030).
type assetQueue struct{ q *JobQueue }

func (a assetQueue) Enqueue(ctx context.Context, tx pgx.Tx, job document.AssetJob) error {
	return a.q.insert(ctx, tx, job, jobs.QueueAsset)
}

// gitQueue enqueues Git export jobs (GOV-011).
type gitQueue struct{ q *JobQueue }

func (g gitQueue) Enqueue(ctx context.Context, tx pgx.Tx, job gitsync.ExportJob) error {
	return g.q.insert(ctx, tx, job, jobs.QueueExport)
}

// publishQueue enqueues publish jobs (SRV-050).
type publishQueue struct{ q *JobQueue }

func (p publishQueue) Enqueue(ctx context.Context, tx pgx.Tx, job release.Work) error {
	if _, ok := job.(release.ControlJob); ok {
		// A switch change must not wait behind a long publish (NFR-024).
		return p.q.insert(ctx, tx, job, jobs.QueueControl)
	}
	return p.q.insert(ctx, tx, job, jobs.QueuePublish)
}

// manifestWorker signs manifests (REL-031).
type manifestWorker struct {
	river.WorkerDefaults[release.ManifestJob]
	svc *Services
}

// Work signs one channel's manifest.
func (w *manifestWorker) Work(ctx context.Context, job *river.Job[release.ManifestJob]) error {
	return w.svc.Releases.SignManifest(ctx, job.Args) //nolint:wrapcheck // a domain error
}

// gitExportWorker pushes publishes to Git remotes (GOV-011). River retries
// a failed push with backoff.
type gitExportWorker struct {
	river.WorkerDefaults[gitsync.ExportJob]
	svc *Services
}

// Work pushes one version.
func (w *gitExportWorker) Work(ctx context.Context, job *river.Job[gitsync.ExportJob]) error {
	return w.svc.GitExport.Work(ctx, job.Args) //nolint:wrapcheck // a domain error
}

// controlWorker signs control documents (SYN-060).
type controlWorker struct {
	river.WorkerDefaults[release.ControlJob]
	svc *Services
}

// Work signs one channel's newest control document.
func (w *controlWorker) Work(ctx context.Context, job *river.Job[release.ControlJob]) error {
	return w.svc.Releases.SignControl(ctx, job.Args) //nolint:wrapcheck // a domain error
}

// deltaWorker precomputes deltas after a publish (REL-022).
type deltaWorker struct {
	river.WorkerDefaults[release.DeltaJob]
	svc *Services
}

// Work computes one version's deltas.
func (w *deltaWorker) Work(ctx context.Context, job *river.Job[release.DeltaJob]) error {
	return w.svc.Releases.PrecomputeDeltas(ctx, job.Args) //nolint:wrapcheck // a domain error
}

// publishWorker runs publishes (SRV-050).
type publishWorker struct {
	river.WorkerDefaults[release.Job]
	svc *Services
}

// Work runs one publish. A publish waiting for assets is snoozed, which
// River does not count as an attempt; RunPublish bounds the wait itself.
func (w *publishWorker) Work(ctx context.Context, job *river.Job[release.Job]) error {
	return snoozePending(w.svc.Releases.RunPublish(ctx, job.Args))
}

// snoozePending turns a publish's wait for assets into a snooze.
func snoozePending(err error) error {
	if pending, ok := errors.AsType[*release.AssetsPendingError](err); ok {
		return river.JobSnooze(pending.RetryAfter) //nolint:wrapcheck // River recognises a snooze by its type
	}
	return err //nolint:wrapcheck // a domain error
}

// assetWorker transcodes uploaded images (CMP-030).
type assetWorker struct {
	river.WorkerDefaults[document.AssetJob]
	svc *Services
}

// Work runs one asset job.
func (w *assetWorker) Work(ctx context.Context, job *river.Job[document.AssetJob]) error {
	return w.svc.Documents.ProcessAsset(ctx, job.Args) //nolint:wrapcheck // a domain error
}

// ids adapts the UUIDv7 generator to the string identifiers the domain
// packages use (SCH-002).
type ids struct{ g *uuid7.Generator }

// New returns a fresh identifier.
func (i ids) New() (string, error) {
	id, err := i.g.New()
	if err != nil {
		return "", fmt.Errorf("server: %w", err)
	}
	return id.String(), nil
}

// NewIDs returns the identifier generator every service shares.
func NewIDs() interface{ New() (string, error) } {
	return ids{g: uuid7.NewGenerator(time.Now, rand.Reader)}
}

// BuildSigning opens the configured signing backend (SEC-120). Only this
// function and the signing package know which one it is.
func BuildSigning(cfg *config.Config) (signing.Backend, error) {
	switch cfg.Signing.Backend {
	case "file":
		b, err := signing.NewFile(cfg.Signing.Directory)
		if err != nil {
			return nil, fmt.Errorf("server: signing: %w", err)
		}
		return b, nil
	case "vault":
		v := cfg.Signing.Vault
		b, err := signing.NewVault(signing.VaultOptions{
			Address: v.Address, Token: v.Token.Value(), Mount: v.Mount, Namespace: v.Namespace, WrapKey: v.WrapKey,
		})
		if err != nil {
			return nil, fmt.Errorf("server: signing: %w", err)
		}
		return b, nil
	case "pkcs11":
		b, err := signing.NewPKCS11(signing.PKCS11Options{Socket: cfg.Signing.PKCS11.Socket, WrapKey: cfg.Signing.PKCS11.WrapKey})
		if err != nil {
			return nil, fmt.Errorf("server: signing: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("server: signing backend %q is not available in this phase", cfg.Signing.Backend)
	}
}

// BuildServices assembles the domain services. They are given only the
// envelope-encryption half of the signing backend (L-3, ADR-0006).
func BuildServices(ctx context.Context, cfg *config.Config, db *storage.DB, shared cache.Cache, set limits.Set, backend signing.Crypter, work WorkDeps) (*Services, error) {
	if db == nil || shared == nil {
		return nil, errors.New("server: the services need a database and a cache")
	}
	tokenSigner, err := tokenSignerFor(cfg, backend)
	if err != nil {
		return nil, err
	}
	backend = signing.CrypterOnly(backend)
	gen := NewIDs()
	log := audit.NewLog(gen, nil)
	core, err := buildCore(cfg, db, shared, set, backend, gen, log, work)
	if err != nil {
		return nil, err
	}
	// auth.studio.allowPasswordLogin may be false only once a provider
	// exists, or nobody could sign in.
	if err := core.auth.CheckPasswordPolicy(ctx); err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	authService, tenancyService, docs := core.auth, core.tenancy, core.docs
	store, pages, err := buildEdge(ctx, db, backend, set)
	if err != nil {
		return nil, err
	}
	configs, err := seccfg.New(seccfg.Options{DB: db, Audit: log, Limits: set, Tenancy: tenancyService})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	devices, trust, status, err := buildDeviceSide(ctx, cfg, db, shared, set, deviceDeps{
		crypter: backend, signer: tokenSigner, ids: gen, audit: log, log: work.Log, configs: configs, metrics: work.Metrics,
	})
	if err != nil {
		return nil, err
	}
	reportDeviceAssurance(work.Metrics, devices)
	events, analyticsService, consumer, err := buildAnalytics(cfg, db, shared, set, gen, log, tenancyService, work)
	if err != nil {
		return nil, err
	}
	runnerLink, err := buildRunnerLink(ctx, cfg, db, backend, work)
	if err != nil {
		return nil, err
	}
	notifier, err := buildNotify(cfg, db, backend, gen, log, tenancyService, work)
	if err != nil {
		return nil, err
	}
	functions, err := buildFunctions(db, work, log, tenancyService, configs, gen, time.Duration(cfg.Telemetry.RetentionDays)*24*time.Hour, runnerLink, notifier)
	if err != nil {
		return nil, err
	}
	gw, err := buildGateway(gatewayDeps{
		cfg: cfg, db: db, cache: shared, set: set, signer: tokenSigner, crypter: backend, ids: gen, audit: log, tenancy: tenancyService,
		devices: devices, configs: configs, trust: trust, metrics: work.Metrics, secretOf: secrets{t: tenancyService},
		identityHTTP: work.IdentityHTTP, functions: functions,
	})
	if err != nil {
		return nil, err
	}
	gitExport, err := gitsync.New(gitsync.Options{
		DB: db, Audit: log, Documents: docs, Secrets: tenancyService, Jobs: gitQueue{work.Queue}, IDs: gen, Limits: set,
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	searchService, err := search.NewService(search.Options{DB: db, Tenancy: tenancyService})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	pusher, err := buildPush(cfg, db, set, backend, tenancyService, work)
	if err != nil {
		return nil, err
	}
	releases, err := buildReleases(cfg, db, set, work, releaseDeps{published: gitExport, push: pusher, ids: gen, audit: log, tenancy: tenancyService, documents: docs, devices: devices, configs: configs, crypter: backend, gateway: gatewayRegistry(gw), functions: functions})
	if err != nil {
		return nil, err
	}
	checkpointer, err := buildCheckpointer(cfg, db, log, gen, work.Signer)
	if err != nil {
		return nil, err
	}
	fnRuntime, err := buildFunctionRuntime(ctx, functionDeps{
		cfg: cfg, db: db, crypter: backend, cache: shared, set: set, service: functions, link: runnerLink, gateway: gw,
		tenancy: tenancyService, configs: configs, devices: devices, keys: store, metrics: work.Metrics,
	})
	if err != nil {
		return nil, err
	}
	if err := attachTriggerInvoker(functions, fnRuntime, functionDeps{
		cfg: cfg, db: db, cache: shared, set: set, service: functions, link: runnerLink, gateway: gw,
		tenancy: tenancyService, configs: configs, devices: devices, keys: store, metrics: work.Metrics,
	}); err != nil {
		return nil, err
	}
	var goldens *golden.Service
	if work.Objects != nil {
		goldens = golden.New(golden.Options{DB: db, Audit: log, Objects: work.Objects, IDs: gen, Limits: set})
	}
	auditExport, auditExporter, err := buildAuditExport(db, log, gen, backend, tenancyService, work)
	if err != nil {
		return nil, err
	}
	return &Services{
		Goldens: goldens, Notify: notifier, AuditExport: auditExport, AuditExporter: auditExporter, Search: searchService, GitExport: gitExport, Push: pusher,
		Audit: log, Auth: authService, Tenancy: tenancyService, Documents: docs, Releases: releases,
		Devices: devices, Events: events, Analytics: analyticsService, EventConsumer: consumer,
		Idempotency: store, Pages: pages, DeviceTrust: trust, Gateway: gw,
		AuditCheckpoints: audit.NewCheckpointReader(db, log), Checkpointer: checkpointer, SecurityConfig: configs,
		AttestationStatus: status, Metrics: work.Metrics, Functions: functions, FunctionRuntime: fnRuntime,
		Identity: work.Identity.OrDefault(),
	}, nil
}

// buildAuditExport builds the administration of the audit export
// destinations and, in the worker role only, the exporter (SEC-143).
func buildAuditExport(db *storage.DB, log *audit.Log, gen auditexport.IDs, backend signing.Crypter, ten *tenancy.Service, work WorkDeps) (*auditexport.Service, *auditexport.Exporter, error) {
	o := auditexport.Options{
		DB: db, Audit: log, IDs: gen, Crypter: backend, Provider: work.AuditSinks,
		Limits: func(ctx context.Context, tx pgx.Tx, org string) (limits.Set, error) {
			return ten.Effective(ctx, tx, org, "", "") //nolint:wrapcheck // a domain error
		},
	}
	service, err := auditexport.NewService(o)
	if err != nil {
		return nil, nil, fmt.Errorf("server: %w", err)
	}
	if work.Signer == nil {
		return service, nil, nil
	}
	exporter, err := auditexport.NewExporter(auditexport.ExporterOptions{
		Options: o, Organizations: ten, Notifier: work.AuditNotifier, Log: work.Log,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("server: %w", err)
	}
	return service, exporter, nil
}

// reportDeviceAssurance feeds the devices-by-assurance gauge from the device
// service. It does nothing without a metrics sink.
func reportDeviceAssurance(m *observability.Metrics, devices *device.Service) {
	if m == nil {
		return
	}
	m.DevicesByAssurance(func(ctx context.Context) ([]observability.DeviceAssurance, error) {
		counts, err := devices.AssuranceCounts(ctx)
		out := make([]observability.DeviceAssurance, len(counts))
		for i, c := range counts {
			out[i] = observability.DeviceAssurance{App: c.AppID, Level: c.Level, Count: c.Count}
		}
		return out, err //nolint:wrapcheck // a domain error
	})
}

// coreServices are the services the others are built on: sign-in, tenancy
// and documents.
type coreServices struct {
	auth    *auth.Service
	tenancy *tenancy.Service
	docs    *document.Service
}

// buildCore assembles the services of sign-in, tenancy and documents.
func buildCore(cfg *config.Config, db *storage.DB, shared cache.Cache, set limits.Set, backend signing.Crypter, gen interface{ New() (string, error) }, log *audit.Log, work WorkDeps) (coreServices, error) {
	issuers, err := trustedIssuers(cfg)
	if err != nil {
		return coreServices{}, err
	}
	oidc, err := oidcProvider(cfg)
	if err != nil {
		return coreServices{}, err
	}
	authService, err := auth.NewService(auth.Options{
		DB: db, Audit: log, Cache: shared, Limits: set, Crypter: backend, IDs: gen,
		SessionTTL:      cfg.Auth.Studio.SessionTTL.Duration(),
		CITokenTTL:      cfg.Auth.CI.TokenTTL.Duration(),
		Issuer:          "Plux",
		VerificationURI: verificationURI(cfg),
		Issuers:         issuers,
		OIDC:            oidc,
		WebAuthn:        webAuthn(cfg),

		HTTPClient: work.IdentityHTTP, SAML: work.Identity.OrDefault().SAML,
		PasswordLoginDisabled: !cfg.Auth.Studio.AllowPasswordLogin,
	})
	if err != nil {
		return coreServices{}, fmt.Errorf("server: %w", err)
	}
	tenancyService, err := tenancy.NewService(tenancy.Options{
		DB: db, Audit: log, Auth: authService, Crypter: backend, IDs: gen, Limits: set,
		TrashDays: cfg.Retention.TrashDays, SigningKeyPrefix: cfg.Signing.Keys.Targets,
	})
	if err != nil {
		return coreServices{}, fmt.Errorf("server: %w", err)
	}
	docs, err := document.NewService(document.Options{
		DB: db, Audit: log, Tenancy: tenancyService, IDs: gen, Limits: set,
		SnapshotDays: cfg.Retention.SnapshotDays, CompilerVersion: buildinfo.Get().Version,
		Objects: work.Objects, Jobs: assetQueue{work.Queue}, Scanner: scanner(cfg), Codecs: work.Codecs, SVG: work.SVG,
	})
	if err != nil {
		return coreServices{}, fmt.Errorf("server: %w", err)
	}
	for kind, k := range docs.TrashKinds() {
		tenancyService.RegisterTrashKind(kind, k)
	}
	return coreServices{auth: authService, tenancy: tenancyService, docs: docs}, nil
}

// buildEdge builds what the API edge keeps beside the domain services:
// the idempotency store and the page-token authenticator.
func buildEdge(ctx context.Context, db *storage.DB, backend signing.Crypter, set limits.Set) (*idempotency.Store, *api.Pages, error) {
	store, err := idempotency.NewStore(db, backend, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("server: %w", err)
	}
	key, err := signing.InstallationKey(ctx, db, backend, "page-token")
	if err != nil {
		return nil, nil, fmt.Errorf("server: %w", err)
	}
	pages, err := api.NewPages(key, int32(min(set.Get(limits.APIPageSize), 1<<30)), nil) //nolint:gosec // bounded by min
	if err != nil {
		return nil, nil, fmt.Errorf("server: %w", err)
	}
	return store, pages, nil
}

// buildCheckpointer returns the audit checkpointer, or nil where there is
// no signer to give it.
func buildCheckpointer(cfg *config.Config, db *storage.DB, log *audit.Log, gen audit.IDs, signer signing.Signer) (*audit.Checkpointer, error) {
	if signer == nil {
		return nil, nil
	}
	c, err := audit.NewCheckpointer(audit.CheckpointerOptions{
		DB: db, Log: log, Signer: signer, Key: cfg.Signing.Keys.Audit, IDs: gen,
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return c, nil
}

// releaseDeps are the services the release service is built on.
type releaseDeps struct {
	// published follows every recorded version (GOV-011).
	published release.Published
	ids       interface{ New() (string, error) }
	audit     *audit.Log
	tenancy   *tenancy.Service
	documents *document.Service
	devices   *device.Service
	configs   *seccfg.Service
	crypter   signing.Crypter
	// gateway is the upstream registry the release check reads; nil where
	// the process serves no gateway.
	gateway *gateway.Registry
	// functions supplies the function catalogue and device modules a
	// compilation uses; nil where the installation runs no functions.
	functions *function.Service
	// push sends the silent push of a signed control document.
	push *push.Service
}

// buildReleases assembles the release service, or returns nil when the
// installation has no object store to publish into.
// publishMetrics hands the release service the process's metrics, or no
// recorder at all when there are none (never a nil pointer in an interface).
func publishMetrics(m *observability.Metrics) release.PublishMetrics {
	if m == nil {
		return nil
	}
	return m
}

func buildReleases(cfg *config.Config, db *storage.DB, set limits.Set, work WorkDeps, deps releaseDeps) (*release.Service, error) {
	if work.Objects == nil {
		return nil, nil
	}
	releases, err := release.NewService(release.Options{
		DB: db, Audit: deps.audit, Tenancy: deps.tenancy, Documents: deps.documents, Objects: work.Objects, IDs: deps.ids,
		Metrics: publishMetrics(work.Metrics), Jobs: publishQueue{work.Queue}, Signer: work.Signer, ProductionSigning: work.ProductionSigning, Limits: set,
		Devices: deps.devices, Gateway: deps.gateway, SecurityConfig: deps.configs, PublicBaseURL: cfg.Server.PublicBaseURL,
		Crypter: deps.crypter, Recipients: deps.devices, Functions: functionsOf(deps.functions),
		CompilerVersion: buildinfo.Get().Version, DevelopmentDays: cfg.Retention.DevelopmentReleaseDays,
		Metadata: metadataOptions(cfg), Published: deps.published, ControlNotifier: controlNotifier{push: deps.push},
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	deps.tenancy.RegisterUsage(releases.Usage)
	// A change of an environment's configuration has its manifests signed
	// again, so that they pin the new version (SEC-182).
	deps.configs.OnChange(releases.EnqueueEnvironmentManifests)
	return releases, nil
}

// functionsOf returns the service as the release service's supplier of
// functions, or nil, not a typed nil, where there is none.
func functionsOf(svc *function.Service) release.Functions {
	if svc == nil {
		return nil
	}
	return svc
}

// metadataOptions are the update-metadata settings of the configuration
// (SEC-050).
func metadataOptions(cfg *config.Config) release.MetadataOptions {
	e := cfg.UpdateMetadata.Expiry
	return release.MetadataOptions{
		Expiry: updatemeta.Expiry{
			Timestamp: e.Timestamp.Duration(), Snapshot: e.Snapshot.Duration(),
			Targets: e.Targets.Duration(), Root: e.Root.Duration(),
		},
		RootThreshold:     cfg.UpdateMetadata.RootThreshold,
		SnapshotKeyPrefix: cfg.Signing.Keys.Snapshot, TimestampKeyPrefix: cfg.Signing.Keys.Timestamp,
		ControlKeyPrefix: cfg.Signing.Keys.Control,
	}
}

// verificationURI is where a person approves `plux login`: Studio's
// device page, under the installation's public URL (CLI-002).
func verificationURI(cfg *config.Config) string {
	if cfg.Server.PublicBaseURL == "" {
		return ""
	}
	return strings.TrimSuffix(cfg.Server.PublicBaseURL, "/") + "/device"
}

// oidcProvider builds the OpenID Connect provider people sign in with,
// when one is configured (SEC-100).
func oidcProvider(cfg *config.Config) (*auth.OIDCProvider, error) {
	o := cfg.Auth.Studio.OIDC
	if o.Issuer == "" {
		return nil, nil //nolint:nilnil // no provider is configured
	}
	client, err := httpx.NewClient(httpx.Options{Destination: httpx.KindOIDC, Timeout: 15 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	p, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: o.Issuer, ClientID: o.ClientID, ClientSecret: string(o.ClientSecret), RedirectURL: o.RedirectURL,
		Client: client,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return p, nil
}

// webAuthn names the relying party security keys are registered for:
// the host of the public base URL, which serves Studio and its
// backend-for-frontend. Without a public base URL, security keys are
// off.
func webAuthn(cfg *config.Config) auth.WebAuthnConfig {
	u, err := url.Parse(cfg.Server.PublicBaseURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return auth.WebAuthnConfig{}
	}
	return auth.WebAuthnConfig{RPID: u.Hostname(), RPName: "Plux", Origins: []string{u.Scheme + "://" + u.Host}}
}

// trustedIssuers builds a verifier for each configured CI provider
// (SRV-064). Their discovery documents and keys are fetched through the
// SSRF-safe client (SEC-105).
func trustedIssuers(cfg *config.Config) (map[string]auth.TrustedIssuer, error) {
	client, err := httpx.NewClient(httpx.Options{Destination: httpx.KindCIIssuer, Timeout: 15 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	out := map[string]auth.TrustedIssuer{}
	for _, is := range cfg.Auth.CI.Issuers {
		v, err := auth.NewVerifier(is.Issuer, client, nil)
		if err != nil {
			return nil, fmt.Errorf("server: CI issuer %s: %w", is.Issuer, err)
		}
		out[strings.TrimSuffix(is.Issuer, "/")] = auth.TrustedIssuer{
			Verifier: v, Audience: is.Audience, SubjectPattern: is.SubjectPattern,
		}
	}
	return out, nil
}

// RegisterAPI mounts the services of this phase on the api role's mux,
// each behind the standard interceptor chain with authentication. It
// refuses services built without the device trust the api role needs.
func (s *Server) RegisterAPI(svc *Services) error {
	if svc.DeviceTrust == nil {
		return errors.New("server: the api role needs services built with device trust (the api role in the configuration)")
	}
	limiter := api.RateLimiter{Window: time.Minute}
	if s.cache != nil {
		limiter.Count = s.cache.Increment
	}
	h := s.handlersFor(svc, limiter)
	people := api.Authentication(svc.Auth, api.IdentityPublic, s.trusted, limiter, s.limits.Get(limits.APIRequestsPerMinute))
	deviceAuth := s.deviceAuth(svc, limiter, people)
	authn := api.DeviceAuthentication(deviceAuth)
	// api.requestSize bounds a body as sent, before a handler reads it
	// (httpx.MaxBytes), and also each message after decompression: Connect
	// accepts gzip bodies, and a small one could otherwise expand without
	// bound in memory.
	opts := connect.WithHandlerOptions(
		connect.WithInterceptors(s.Interceptors(authn)...),
		connect.WithReadMaxBytes(int(s.limits.Get(limits.APIRequestSize))),
	)
	registrations := apiRegistrations(svc, h, opts)
	for _, register := range registrations {
		path, handler := register()
		s.Register(path, handler)
	}
	s.registerFunctionHost(h)
	if svc.Functions != nil {
		s.Register(HooksRoute, svc.Functions.HookHandler())
	}
	if h.FunctionInvoker != nil {
		s.Register(pluxv1connect.NewFunctionServiceHandler(h.FunctionService(), opts))
	}
	if svc.Gateway != nil {
		s.registerGateway(svc.Gateway, h, deviceAuth, opts)
	}
	if s.objects != nil {
		s.Register("GET "+release.ObjectsPath, s.objectHandler())
	}
	if svc.Releases != nil {
		s.Register(MetadataRoute, s.metadataHandler(svc.Releases))
	}
	s.Register(api.BackChannelLogoutRoute, h.BackChannelLogout())
	s.Register(api.SCIMRoute, h.SCIM(svc.Identity.OrDefault().SCIM))
	return nil
}

// handlersFor collects the services into the handler set of the api role,
// leaving out what the build did not provide.
func (s *Server) handlersFor(svc *Services, limiter api.RateLimiter) *api.Handlers {
	h := &api.Handlers{
		Auth: svc.Auth, Tenancy: svc.Tenancy, Documents: svc.Documents, Releases: svc.Releases, Devices: svc.Devices, Events: svc.Events,
		Analytics: svc.Analytics, Idempotency: svc.Idempotency, Pages: svc.Pages, SecurityConfig: svc.SecurityConfig, Functions: svc.Functions,
		Limiter: limiter, Limits: s.limits, Edition: s.edition, Push: svc.Push,
	}
	if svc.AuditCheckpoints != nil {
		h.AuditCheckpoints = svc.AuditCheckpoints
	}
	if svc.Documents != nil {
		h.Translations = localisation.New(svc.Documents, NewIDs(), s.limits)
	}
	if rt := svc.FunctionRuntime; rt != nil {
		h.FunctionHosting, h.FunctionInvoker = rt.host, rt.invoker
	}
	if g := svc.Gateway; g != nil {
		h.Gateway, h.Upstreams, h.Exchange = g.Gateway, g.Upstreams, g.Exchange
		h.Configs, h.LiveConfig = g.Configs, g.Resolver
	}
	return h
}

// apiRegistrations lists the Connect services to mount, by service group:
// the always-present ones first, then those that depend on a built service.
func apiRegistrations(svc *Services, h *api.Handlers, opts connect.HandlerOption) []func() (string, http.Handler) {
	registrations := []func() (string, http.Handler){
		func() (string, http.Handler) { return pluxv1connect.NewIdentityServiceHandler(h.Identity(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewOrgServiceHandler(h.Org(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewGovernanceServiceHandler(h.Governance(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewFederationServiceHandler(h.Federation(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewAppServiceHandler(h.App(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewPluginServiceHandler(h.Plugin(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewDocumentServiceHandler(h.Document(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewComponentServiceHandler(h.Component(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewTemplateServiceHandler(h.Template(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewAssetServiceHandler(h.Asset(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewDeviceServiceHandler(h.Device(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewTokenServiceHandler(h.Token(), opts) },
		func() (string, http.Handler) { return pluxv1connect.NewTelemetryServiceHandler(h.Telemetry(), opts) },
	}
	if svc.Releases != nil {
		registrations = append(registrations,
			func() (string, http.Handler) { return pluxv1connect.NewPublishServiceHandler(h.Publish(), opts) },
			func() (string, http.Handler) { return pluxv1connect.NewReleaseServiceHandler(h.Release(), opts) },
			func() (string, http.Handler) { return pluxv1connect.NewManifestServiceHandler(h.Manifest(), opts) },
			func() (string, http.Handler) {
				return pluxv1connect.NewNativeCatalogueServiceHandler(h.NativeCatalogue(), opts)
			},
			func() (string, http.Handler) { return pluxv1connect.NewControlServiceHandler(h.Control(), opts) })
	}
	if h.Translations != nil {
		registrations = append(registrations, func() (string, http.Handler) {
			return pluxv1connect.NewLocalisationServiceHandler(h.Localisation(), opts)
		})
	}
	if svc.Search != nil {
		h.Searches = svc.Search
		registrations = append(registrations, func() (string, http.Handler) {
			return pluxv1connect.NewSearchServiceHandler(h.Search(), opts)
		})
	}
	if svc.GitExport != nil {
		h.GitExports = svc.GitExport
		registrations = append(registrations, func() (string, http.Handler) {
			return pluxv1connect.NewGitExportServiceHandler(h.GitExport(), opts)
		})
	}
	if svc.Analytics != nil {
		registrations = append(registrations, func() (string, http.Handler) {
			return pluxv1connect.NewAnalyticsServiceHandler(h.AnalyticsService(), opts)
		})
	}
	if svc.Goldens != nil {
		h.Goldens = svc.Goldens
		registrations = append(registrations, func() (string, http.Handler) {
			return pluxv1connect.NewGoldenServiceHandler(h.Golden(), opts)
		})
	}
	if svc.AuditExport != nil {
		h.AuditExport = svc.AuditExport
		registrations = append(registrations, func() (string, http.Handler) {
			return pluxv1connect.NewAuditServiceHandler(h.Audit(), opts)
		})
	}
	if svc.SecurityConfig != nil {
		registrations = append(registrations, func() (string, http.Handler) {
			return pluxv1connect.NewSecurityAdminServiceHandler(h.SecurityAdmin(), opts)
		})
	}
	registrations = append(registrations, func() (string, http.Handler) {
		return pluxv1connect.NewLicenseServiceHandler(h.License(), opts)
	})
	if svc.Functions != nil {
		registrations = append(registrations, func() (string, http.Handler) {
			return pluxv1connect.NewFunctionAdminServiceHandler(h.FunctionAdmin(), opts)
		})
	}
	return append(registrations, notifyRegistrations(svc, h, opts)...)
}

// rejectionsOf feeds the reasons of refused proofs into the metrics.
func rejectionsOf(m *observability.Metrics) func(string) {
	if m == nil {
		return nil
	}
	return m.DPoPRejected
}

// deviceAuth is the device authentication of this server's listener.
func (s *Server) deviceAuth(svc *Services, limiter api.RateLimiter, people api.Around) api.DeviceAuth {
	trust := svc.DeviceTrust
	return api.DeviceAuth{
		Devices: svc.Devices, Tokens: trust.Tokens, Proofs: trust.Proofs, Nonces: trust.Nonces, Replay: trust.Replay,
		BaseURL: s.cfg.Server.PublicBaseURL, Window: trust.Window, FallbackWindow: trust.FallbackWindow, Windows: trust.Windows, ReplayPolicy: trust.ReplayPolicy,
		Rejected: rejectionsOf(s.metrics), Limiter: limiter, PerDevice: s.limits.Get(limits.APIRequestsPerMinutePerDevice), People: people,
	}
}

// RegisterGateway mounts what the gateway role serves on its own: the
// gateway and SCA services, the streaming route and the published key
// sets, behind device authentication. A person's call is refused, because
// the role offers none of the rest of the API (ADR-0006, ADR-0056).
func (s *Server) RegisterGateway(svc *Services) error {
	if svc.DeviceTrust == nil || svc.Gateway == nil {
		return errors.New("server: the gateway role needs services built with device trust and a gateway")
	}
	limiter := api.RateLimiter{Window: time.Minute}
	if s.cache != nil {
		limiter.Count = s.cache.Increment
	}
	h := &api.Handlers{
		Devices: svc.Devices, Pages: svc.Pages, Limiter: limiter, Limits: s.limits,
		Gateway: svc.Gateway.Gateway, Upstreams: svc.Gateway.Upstreams, Exchange: svc.Gateway.Exchange,
		LiveConfig: svc.Gateway.Resolver,
	}
	people := func(context.Context, api.Call, func(context.Context) error) error {
		return plxerr.New(plxerr.PermissionDenied, "this role serves devices only")
	}
	deviceAuth := s.deviceAuth(svc, limiter, people)
	opts := connect.WithHandlerOptions(
		connect.WithInterceptors(s.Interceptors(api.DeviceAuthentication(deviceAuth))...),
		connect.WithReadMaxBytes(int(s.limits.Get(limits.APIRequestSize))),
	)
	s.registerGateway(svc.Gateway, h, deviceAuth, opts)
	return nil
}

// Maintenance is the periodic sweep of the worker role: it purges the
// trash past its retention (GOV-031) and draft history past its own
// (SRV-031), forgets idempotency keys older
// than a day (SRV-005) and deletes expired credentials.
type Maintenance struct{}

// Kind names the job.
func (Maintenance) Kind() string { return "maintenance.sweep" }

// InsertOpts puts the sweep on the maintenance queue, at most one
// waiting at a time.
func (Maintenance) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueMaintenance, UniqueOpts: river.UniqueOpts{ByPeriod: 10 * time.Minute}}
}

// maintenanceWorker runs the sweep.
type maintenanceWorker struct {
	river.WorkerDefaults[Maintenance]
	svc *Services
	log *slog.Logger
}

// sweepTotals counts what one sweep did, for its log line.
type sweepTotals struct {
	snapshots, released, signed int
	resign                      []error
}

// Work runs one sweep. Each part runs even if another fails, and the
// job fails if any did, so River retries it.
func (w *maintenanceWorker) Work(ctx context.Context, _ *river.Job[Maintenance]) error {
	var errs []error
	purged, err := w.svc.Tenancy.PurgeExpired(ctx, 100)
	errs = append(errs, err)
	var t sweepTotals
	err = w.svc.Tenancy.ForEachOrganization(ctx, func(org string) error {
		return w.sweepOrganization(ctx, org, &t)
	})
	errs = append(errs, err)
	errs = append(errs, t.resign...)
	keys, err := w.svc.Idempotency.Expire(ctx)
	errs = append(errs, err)
	creds, err := w.svc.Auth.Expire(ctx)
	errs = append(errs, err)
	if w.log != nil {
		w.log.InfoContext(ctx, "maintenance sweep",
			slog.Int("trashPurged", purged), slog.Int("snapshotsPurged", t.snapshots), slog.Int("releasesPurged", t.released), slog.Int("manifestsSigned", t.signed), slog.Int64("idempotencyKeysExpired", keys),
			slog.Int64("credentialsExpired", creds))
	}
	return errors.Join(errs...)
}

// sweepOrganization runs the sweep's parts for one organisation.
func (w *maintenanceWorker) sweepOrganization(ctx context.Context, org string, t *sweepTotals) error {
	if w.svc.Releases != nil {
		if err := w.sweepReleases(ctx, org, t); err != nil {
			return err
		}
	}
	if _, err := w.svc.Devices.ExpireTokens(ctx, org); err != nil {
		return err //nolint:wrapcheck // a domain error
	}
	if w.svc.Functions != nil {
		if _, err := w.svc.Functions.PurgeObservations(ctx, org); err != nil {
			return err //nolint:wrapcheck // a domain error
		}
	}
	if _, err := w.svc.Documents.RequeueSVGs(ctx, org); err != nil {
		return err //nolint:wrapcheck // a domain error
	}
	n, err := w.svc.Documents.PurgeSnapshots(ctx, org)
	t.snapshots += n
	return err //nolint:wrapcheck // a domain error
}

// sweepReleases purges releases and re-signs the manifests and update
// metadata of one organisation.
func (w *maintenanceWorker) sweepReleases(ctx context.Context, org string, t *sweepTotals) error {
	n, err := w.svc.Releases.PurgeReleases(ctx, org)
	t.released += n
	if err != nil {
		return err //nolint:wrapcheck // a domain error
	}
	// Signing failures of one organisation, such as an expired root, are
	// reported without stopping the sweep of the others; the job still
	// fails, so River retries it.
	m, err := w.svc.Releases.RefreshManifests(ctx, org)
	t.signed += m
	t.resign = append(t.resign, err)
	// The timestamp and snapshot are re-signed before they expire
	// (SEC-050).
	m, err = w.svc.Releases.RefreshMetadata(ctx, org)
	t.signed += m
	t.resign = append(t.resign, err)
	// A control document is re-signed before it expires, so that devices
	// always find one in force (SYN-060).
	m, err = w.svc.Releases.RefreshControl(ctx, org)
	t.signed += m
	t.resign = append(t.resign, err)
	if _, err := w.svc.Releases.PurgeManifests(ctx, org); err != nil {
		return err //nolint:wrapcheck // a domain error
	}
	if _, err := w.svc.Releases.PurgeMetadata(ctx, org); err != nil {
		return err //nolint:wrapcheck // a domain error
	}
	return nil
}

// MaintenanceJobs registers the sweep's worker and returns its schedule.
func MaintenanceJobs(workers *jobs.Workers, svc *Services, log *slog.Logger) []*river.PeriodicJob {
	jobs.AddWorker(workers, &maintenanceWorker{svc: svc, log: log})
	return []*river.PeriodicJob{jobs.Every(time.Hour, Maintenance{})}
}

// gatewayRegistry returns the upstream registry of the gateway, if any.
func gatewayRegistry(g *GatewayParts) *gateway.Registry {
	if g == nil {
		return nil
	}
	return g.Upstreams
}

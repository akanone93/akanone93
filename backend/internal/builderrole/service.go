// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// headerTimeout bounds how long a peer may take to send request headers.
const headerTimeout = 10 * time.Second

// requestEnvelopeBytes is the room, beyond the largest source archive, that a
// request needs for its other fields and framing.
const requestEnvelopeBytes = 1 << 20

// buildOutputBytes bounds the standard output and error of each tool of a
// build. It protects the role's memory from a build script that prints without
// end; it is not a limit customers see.
const buildOutputBytes = 8 << 20

// Options configures a Service.
type Options struct {
	// Listen is the address of the mutual-TLS listener.
	Listen string
	// TLS must require and verify a client certificate (httpx.InternalServerTLS).
	TLS *tls.Config
	// Tools configures the pipeline: Sandbox, Cargo, Rustc, WasmOpt, SDKDir,
	// TempDir, Identity, Clock, Getenv and Allowlist. Schemas, Limits and
	// SourceBytes are set by the Service.
	Tools builder.Config
	// Unsandboxed states that Tools.Sandbox does not isolate builds
	// (builder.sandbox: none); the Service then warns on start and on every
	// build.
	Unsandboxed bool
	// Limits is the registry in force for the role (LIM-001).
	Limits limits.Set
	// Concurrency is the number of builds at once; it must be positive.
	Concurrency int
	// LanguageServer is the language server executable of the Rust editor
	// (rust-analyzer; FN-033), run in Tools.Sandbox. Empty refuses editor
	// sessions. LanguageArgs and LanguageEnv are its arguments and added
	// environment; LanguageSessions is how many sessions the role runs at
	// once, on top of Concurrency.
	LanguageServer   string
	LanguageArgs     []string
	LanguageEnv      []string
	LanguageSessions int
	// Clock and Random feed the function host.
	Clock  func() time.Time
	Random io.Reader
	// Log receives the role's records; nil discards them.
	Log *slog.Logger
}

// Service serves the BuilderService. It is a component of the process: Start
// returns once the socket is open, Stop drains the builds in flight and
// releases the function host. It is safe for concurrent use.
type Service struct {
	o      Options
	engine *fnhost.Engine
	loader *schema.Loader
	slots  chan struct{}
	lang   *languageSessions
	log    *slog.Logger
	http   *http.Server
	tls    *tls.Config

	mu     sync.Mutex
	ln     net.Listener
	active sync.WaitGroup
}

// New prepares a Service and starts its function host. No socket is opened
// until Start.
func New(ctx context.Context, o Options) (*Service, error) {
	if o.TLS == nil || o.TLS.ClientAuth != tls.RequireAndVerifyClientCert {
		return nil, errors.New("builderrole.New: the listener must require and verify a client certificate (SEC-043)")
	}
	if o.Concurrency < 1 || o.Clock == nil || o.Random == nil {
		return nil, errors.New("builderrole.New: Concurrency, Clock and Random are required")
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	validator, err := schema.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("builderrole.New: %w", err)
	}
	engine, err := fnhost.NewEngine(ctx, fnhost.Config{
		Clock: o.Clock, Random: o.Random, MaxMemoryBytes: uint64(o.Limits.Get(limits.FunctionMemory)), //nolint:gosec // G115: registry values are non-negative.
	})
	if err != nil {
		return nil, fmt.Errorf("builderrole.New: %w", err)
	}
	s := &Service{
		o: o, engine: engine, loader: schema.NewLoader(validator, schema.DefaultMigrator(), o.Limits),
		slots: make(chan struct{}, o.Concurrency), log: o.Log, lang: newLanguageSessions(o.LanguageSessions),
	}
	s.tls = o.TLS.Clone()
	s.tls.MinVersion = tls.VersionTLS13
	mux := http.NewServeMux()
	maxRequest := int(min(maxSource()+requestEnvelopeBytes, 1<<31-1))
	path, handler := fnpbconnect.NewBuilderServiceHandler(s, connect.WithReadMaxBytes(maxRequest))
	mux.Handle(path, handler)
	s.http = &http.Server{
		Handler: mux, TLSConfig: s.tls, ReadHeaderTimeout: headerTimeout,
		ErrorLog: slog.NewLogLogger(o.Log.Handler(), slog.LevelDebug),
	}
	return s, nil
}

// maxSource is the registry maximum of function.sourceArchiveSize (LIM-001):
// the ceiling of a request and of the source a build extracts. The worker
// enforced the app's own limit when the archive was uploaded.
func maxSource() int64 {
	def, _ := limits.Lookup(limits.FunctionSourceArchiveSize)
	return def.Max
}

// Name identifies the service as a component of the process.
func (*Service) Name() string { return "builder" }

// Start listens and serves in the background.
func (s *Service) Start(ctx context.Context) error {
	if s.o.Unsandboxed {
		s.log.WarnContext(ctx, unsandboxedWarning)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.o.Listen)
	if err != nil {
		return fmt.Errorf("builderrole.Start: %w", err)
	}
	return s.Serve(tls.NewListener(ln, s.tls))
}

// unsandboxedWarning is the distinct warning of builder.sandbox: none
// (ADR-0063).
const unsandboxedWarning = "builder.sandbox is none: uploaded Rust sources build without a sandbox, so a build script or procedural macro runs with this process's privileges; it is for development only"

// Serve serves on a listener that already terminates TLS with the service's
// configuration, in the background. Tests and Start use it.
func (s *Service) Serve(ln net.Listener) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		_ = ln.Close()
		return errors.New("builderrole.Serve: already serving")
	}
	s.ln = ln
	s.active.Add(1)
	go func() {
		defer s.active.Done()
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("the builder listener failed", slog.String("error", err.Error()))
		}
	}()
	return nil
}

// Addr is the address the service listens on, or nil before Start.
func (s *Service) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Stop stops accepting requests, waits for those in flight until the context
// ends, then closes the connections and releases the function host.
func (s *Service) Stop(ctx context.Context) error {
	shutdownErr := s.http.Shutdown(ctx)
	if shutdownErr != nil {
		_ = s.http.Close()
	}
	s.active.Wait()
	if err := errors.Join(shutdownErr, s.engine.Close(ctx)); err != nil {
		return fmt.Errorf("builderrole.Stop: %w", err)
	}
	return nil
}

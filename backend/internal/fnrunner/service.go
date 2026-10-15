// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
)

// headerTimeout bounds how long a peer may take to send request headers.
const headerTimeout = 10 * time.Second

// Service serves the Runner on a mutual-TLS listener (SEC-043). It is a
// component of the process: Start returns once the socket is open, Stop
// drains the requests in flight and releases the Runner. It is safe for
// concurrent use and owns the goroutine of its HTTP server.
type Service struct {
	runner *Runner
	listen string
	tls    *tls.Config
	log    *slog.Logger
	http   *http.Server

	mu     sync.Mutex
	ln     net.Listener
	active sync.WaitGroup
}

// NewService prepares a Service. tlsConfig must require and verify a client
// certificate (httpx.InternalServerTLS); no socket is opened until Start.
func NewService(runner *Runner, listen string, tlsConfig *tls.Config, log *slog.Logger) (*Service, error) {
	if tlsConfig == nil || tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		return nil, errors.New("fnrunner.NewService: the listener must require and verify a client certificate (SEC-043)")
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	mux := http.NewServeMux()
	path, handler := fnpbconnect.NewFunctionRunnerServiceHandler(runner)
	mux.Handle(path, handler)
	conf := tlsConfig.Clone()
	conf.MinVersion = tls.VersionTLS13
	return &Service{
		runner: runner, listen: listen, tls: conf, log: log,
		http: &http.Server{
			Handler: mux, TLSConfig: conf, ReadHeaderTimeout: headerTimeout,
			ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelDebug),
		},
	}, nil
}

// Name identifies the service as a component of the process.
func (*Service) Name() string { return "fnrunner" }

// Start listens and serves in the background.
func (s *Service) Start(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.listen)
	if err != nil {
		return fmt.Errorf("fnrunner.Start: %w", err)
	}
	return s.Serve(tls.NewListener(ln, s.tls))
}

// Serve serves on a listener that already terminates TLS with the
// service's configuration, in the background. Tests and Start use it.
func (s *Service) Serve(ln net.Listener) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		_ = ln.Close()
		return errors.New("fnrunner.Serve: already serving")
	}
	s.ln = ln
	s.active.Add(1)
	go func() {
		defer s.active.Done()
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("the fnrunner listener failed", slog.String("error", err.Error()))
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

// Stop stops accepting requests, waits for those in flight until the
// context ends, then closes the connections and releases the Runner.
func (s *Service) Stop(ctx context.Context) error {
	shutdownErr := s.http.Shutdown(ctx)
	if shutdownErr != nil {
		_ = s.http.Close()
	}
	s.active.Wait()
	closeErr := s.runner.Close(ctx)
	if err := errors.Join(shutdownErr, closeErr); err != nil {
		return fmt.Errorf("fnrunner.Stop: %w", err)
	}
	return nil
}

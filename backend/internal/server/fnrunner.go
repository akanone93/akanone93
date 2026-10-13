// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/nightCode42/plux3/backend/internal/egress"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/fnrunner"
	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// hostCallTimeout is the backstop of a call to the api role's
// FunctionHostService; the invocation's own deadline ends a call sooner.
const hostCallTimeout = 10 * time.Minute

// moduleEnvelopeBytes is the room, beyond the largest module, that a
// GetModule response needs for its statement, signature and framing.
const moduleEnvelopeBytes = 1 << 20

// buildFnRunner prepares the fnrunner role: the runner of function modules
// that holds no database, storage or signing credential (FN-013,
// ADR-0064). Files and keys are read now, so a bad configuration stops the
// process before it serves; the engine starts with the component.
func (s *Server) buildFnRunner() error {
	f := s.cfg.FnRunner
	tlsOpts := httpx.InternalTLSOptions{CAFile: f.InternalTLS.CAFile, CertFile: f.InternalTLS.CertFile, KeyFile: f.InternalTLS.KeyFile}
	serverTLS, err := httpx.InternalServerTLS(tlsOpts)
	if err != nil {
		return fmt.Errorf("server: fnrunner: %w", err)
	}
	clientTLS, err := httpx.InternalClientTLS(tlsOpts)
	if err != nil {
		return fmt.Errorf("server: fnrunner: %w", err)
	}
	hostHTTP, err := httpx.NewInternalClient(tlsOpts, hostCallTimeout)
	if err != nil {
		return fmt.Errorf("server: fnrunner: %w", err)
	}
	keys := make(map[string]ed25519.PublicKey, len(f.ArtifactKeys))
	for _, k := range f.ArtifactKeys {
		key, err := k.Decode()
		if err != nil {
			return fmt.Errorf("server: fnrunner: artifact key %q: %w", k.KeyID, err)
		}
		keys[k.KeyID] = key
	}
	var reg prometheus.Registerer
	if s.metrics != nil {
		reg = s.metrics.Registry()
	}
	log := s.log.With("component", "fnrunner")
	s.components = append(s.components, &fnrunnerComponent{build: func(ctx context.Context) (*fnrunner.Service, error) {
		runner, err := fnrunner.New(ctx, fnrunner.Options{
			Keys: keys,
			Host: fnpbconnect.NewFunctionHostServiceClient(hostHTTP, f.APIURL,
				connect.WithReadMaxBytes(int(min(f.MaxModuleBytes+moduleEnvelopeBytes, 1<<31-1)))),
			Egress:         &egress.Dialer{Addr: f.EgressAddr, TLS: clientTLS},
			Clock:          time.Now,
			Random:         rand.Reader,
			MaxMemoryBytes: f.MaxMemoryBytes, CacheDir: f.CacheDir,
			MaxModules: f.MaxModules, MaxModuleBytes: f.MaxModuleBytes,
			MaxConcurrent: f.MaxConcurrent, HTTPResponseBytes: f.HTTPResponseBytes,
			Log: log, Registerer: reg,
		})
		if err != nil {
			return nil, fmt.Errorf("server: %w", err)
		}
		svc, err := fnrunner.NewService(runner, f.Listen, serverTLS, log)
		if err != nil {
			_ = runner.Close(ctx)
			return nil, fmt.Errorf("server: %w", err)
		}
		return svc, nil
	}})
	return nil
}

// fnrunnerComponent starts the runner's service with the process and drains
// it on shutdown.
type fnrunnerComponent struct {
	build func(context.Context) (*fnrunner.Service, error)
	svc   *fnrunner.Service
}

// Name implements Component.
func (*fnrunnerComponent) Name() string { return "fnrunner" }

// Start implements Component.
func (c *fnrunnerComponent) Start(ctx context.Context) error {
	svc, err := c.build(ctx)
	if err != nil {
		return err
	}
	if err := svc.Start(ctx); err != nil {
		_ = svc.Stop(ctx)
		return fmt.Errorf("server: %w", err)
	}
	c.svc = svc
	return nil
}

// Stop implements Component.
func (c *fnrunnerComponent) Stop(ctx context.Context) error {
	if c.svc == nil {
		return nil
	}
	if err := c.svc.Stop(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"github.com/nightCode42/plux3/backend/internal/buildclient"
	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/builderrole"
	"github.com/nightCode42/plux3/backend/internal/config"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// buildBuilder prepares the builder role: the compiler of uploaded function
// sources, which holds no database, object-store or signing credential
// (FN-016, ADR-0063). The sandbox is the Linux namespaces unless
// builder.sandbox is none, which the role warns about on start and with every
// build. The role starts with the process and drains on shutdown.
func (s *Server) buildBuilder() error {
	b := s.cfg.Builder
	tlsConfig, err := httpx.InternalServerTLS(httpx.InternalTLSOptions{
		CAFile: b.InternalTLS.CAFile, CertFile: b.InternalTLS.CertFile, KeyFile: b.InternalTLS.KeyFile,
	})
	if err != nil {
		return fmt.Errorf("server: builder: %w", err)
	}
	var sandbox builder.Sandbox = builder.LinuxSandbox{}
	if b.Sandbox == "none" {
		sandbox = builder.NoSandbox{}
	}
	identity := b.Identity
	if identity == "" {
		if identity, err = os.Hostname(); err != nil {
			identity = "builder"
		}
	}
	log := s.log.With("component", "builder")
	s.components = append(s.components, &builderComponent{build: func(ctx context.Context) (*builderrole.Service, error) {
		svc, err := builderrole.New(ctx, builderrole.Options{
			Listen: b.Listen, TLS: tlsConfig,
			Tools: builder.Config{
				Sandbox: sandbox, Clock: time.Now, Identity: identity, SDKDir: b.SDKDir, TempDir: b.WorkDir,
				Cargo: b.Cargo, Rustc: b.Rustc, WasmOpt: b.WasmOpt,
			},
			Unsandboxed: b.Sandbox == "none",
			Limits:      s.limits, Concurrency: b.Concurrency,
			Clock: time.Now, Random: rand.Reader, Log: log,
		})
		if err != nil {
			return nil, fmt.Errorf("server: %w", err)
		}
		return svc, nil
	}})
	return nil
}

// builderComponent starts the role's service, and with it the function host,
// when the process starts, and drains it on shutdown.
type builderComponent struct {
	build func(context.Context) (*builderrole.Service, error)
	svc   *builderrole.Service
}

// Name implements Component.
func (*builderComponent) Name() string { return "builder" }

// Start implements Component.
func (c *builderComponent) Start(ctx context.Context) error {
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
func (c *builderComponent) Stop(ctx context.Context) error {
	if c.svc == nil {
		return nil
	}
	if err := c.svc.Stop(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// buildEnvelopeBytes is the room, beyond the largest module, that a build
// response needs for the schemas, the test results and the framing.
const buildEnvelopeBytes = 4 << 20

// buildBuilderClient prepares the worker's route to the builder role over the
// mutual TLS of server.internalTLS (FN-016, SEC-043). The HTTP client's own
// timeout is the longest build the registry allows; each build also has its
// own deadline.
func buildBuilderClient(cfg *config.Config, store objects.Store) (*buildclient.Client, error) {
	t := cfg.Server.InternalTLS
	def, _ := limits.Lookup(limits.FunctionBuildTimeout)
	moduleDef, _ := limits.Lookup(limits.FunctionModuleSize)
	longest := time.Duration(def.Max)*time.Millisecond*buildclient.BuildSteps + 2*time.Minute
	httpClient, err := httpx.NewInternalClient(httpx.InternalTLSOptions{CAFile: t.CAFile, CertFile: t.CertFile, KeyFile: t.KeyFile}, longest)
	if err != nil {
		return nil, fmt.Errorf("server: builder client: %w", err)
	}
	client, err := buildclient.NewClient(buildclient.Options{
		HTTP: httpClient, URL: cfg.Builder.Address, Objects: store,
		MaxResponseBytes: int(min(moduleDef.Max+buildEnvelopeBytes, 1<<31-1)),
	})
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}
	return client, nil
}

// attachBuilder gives the worker its route to the builder role and the name
// of the key that signs what the role builds, where builder.address is set
// (FN-009, FN-016).
func attachBuilder(cfg *config.Config, store objects.Store, deps *WorkDeps) error {
	if cfg.Builder.Address == "" {
		return nil
	}
	client, err := buildBuilderClient(cfg, store)
	if err != nil {
		return err
	}
	deps.Builder, deps.Linker, deps.FunctionKey = client, client, cfg.Signing.Keys.FunctionArtifact
	return nil
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
)

// fnrunnerConfig is the body of a process that runs only the fnrunner role
// (and egress, when the roles say so).
func fnrunnerConfig(roles, addr, certFile, keyFile string) string {
	key := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	tlsFiles := "  internalTLS:\n    caFile: " + certFile + "\n    certFile: " + certFile + "\n    keyFile: " + keyFile + "\n"
	return "server:\n  roles: [" + roles + "]\n  shutdownGrace: 5s\n" +
		"fnrunner:\n  listen: \"" + addr + "\"\n" + tlsFiles +
		"  apiURL: \"https://127.0.0.1:1\"\n  egressAddr: \"127.0.0.1:2\"\n" +
		"  artifactKeys:\n    - keyID: k1\n      publicKey: " + key + "\n" +
		"egress:\n  listen: \"127.0.0.1:0\"\n" + tlsFiles
}

// Verifies: FN-013, SEC-043.
// A process that runs only fnrunner (with egress) starts without a database,
// serves FunctionRunnerService over mutual TLS, registers its metrics in the
// process registry and drains on shutdown.
func TestFnRunnerRoleRunsWithoutData_FN_013(t *testing.T) {
	t.Parallel()
	certFile, keyFile, pool := testIdentity(t)
	addr := freeAddr(t)
	cfg := testConfig(t, fnrunnerConfig("fnrunner, egress", addr, certFile, keyFile))
	if !cfg.NoDataAccess() || cfg.Database.URL != "" {
		t.Fatalf("roles = %v, database = %q", cfg.Server.Roles, cfg.Database.URL)
	}
	built, err := Build(context.Background(), cfg, discard(), "test")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer built.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- built.Server.Run(ctx) }()

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	mutual := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: pool, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13,
	}}}
	client := fnpbconnect.NewFunctionRunnerServiceClient(mutual, "https://"+addr)
	var invokeErr error
	deadline := time.Now().Add(10 * time.Second)
	for {
		// A malformed module reference is refused by the runner itself, so the
		// answer proves the handler is mounted without any backing service.
		_, invokeErr = client.Prepare(context.Background(), connect.NewRequest(&fnpb.PrepareRequest{ModuleSha256: "not-a-digest"}))
		if connect.CodeOf(invokeErr) == connect.CodeInvalidArgument {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the fnrunner listener never answered: %v", invokeErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	families, err := built.Server.metrics.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range families {
		found = found || f.GetName() == "plux_function_module_cache_total"
	}
	if !found {
		t.Error("the process registry lacks the function metrics")
	}

	anonymous := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: pool, MinVersion: tls.VersionTLS13,
	}}}
	if _, err := fnpbconnect.NewFunctionRunnerServiceClient(anonymous, "https://"+addr).
		Prepare(context.Background(), connect.NewRequest(&fnpb.PrepareRequest{ModuleSha256: "not-a-digest"})); err == nil {
		t.Error("a client without a certificate was served")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after shutdown")
	}
}

// Verifies: FN-013.
// A configuration that names missing identity files stops the process at
// build time, before anything listens.
func TestFnRunnerRoleFailsEarlyOnBadIdentity_FN_013(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t, fnrunnerConfig("fnrunner", "127.0.0.1:0", "/nonexistent.crt", "/nonexistent.key"))
	if _, err := Build(context.Background(), cfg, discard(), "test"); err == nil {
		t.Fatal("Build accepted missing certificate files")
	}
}

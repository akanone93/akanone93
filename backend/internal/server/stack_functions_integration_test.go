// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// functionRoles is what a stack that runs server functions adds to the
// server: an internal listener for the api role, the fnrunner and egress
// roles in a process of their own, and the identity every one of them
// presents to the others (SEC-043). One test authority signs it all, as in
// the other tests of internal TLS (tls_test.go).
type functionRoles struct {
	certFile, keyFile string
	pool              *x509.CertPool
	// apiAddr is the api role's internal listener; runnerAddr and egressAddr
	// those of the two roles that run beside it.
	apiAddr, runnerAddr, egressAddr string
	// artifactKey is the public half of the function-artifact key the
	// stack's worker signs builds with, the only key the runner trusts
	// (FN-009).
	artifactKeyID, artifactKey string
}

// newFunctionRoles chooses the addresses and creates the identity. The
// signing directory is the server's: its function-artifact key exists
// before the server starts, so that the runner can be told to trust it.
func newFunctionRoles(t *testing.T, signingDir string) functionRoles {
	t.Helper()
	certFile, keyFile, pool := testIdentity(t)
	backend, err := signing.NewFile(signingDir)
	if err != nil {
		t.Fatal(err)
	}
	public, keyID, err := backend.PublicKey(t.Context(), "function-artifact")
	if err != nil {
		t.Fatal(err)
	}
	return functionRoles{
		certFile: certFile, keyFile: keyFile, pool: pool,
		apiAddr: freeAddr(t), runnerAddr: freeAddr(t), egressAddr: freeAddr(t),
		artifactKeyID: keyID, artifactKey: base64.StdEncoding.EncodeToString(public),
	}
}

// tlsYAML is the role's identity in a configuration, at the given indent of
// its section.
func (f functionRoles) tlsYAML() string {
	return "  internalTLS:\n    caFile: " + f.certFile + "\n    certFile: " + f.certFile + "\n    keyFile: " + f.keyFile + "\n"
}

// serverYAML is added to the server section of the api and worker process:
// the internal listener that serves FunctionHostService to the runner.
func (f functionRoles) serverYAML() string {
	return "  internalListen: \"" + f.apiAddr + "\"\n" + f.tlsYAML()
}

// extraYAML is added to the configuration of that process: where the api
// role finds the runner.
func (f functionRoles) extraYAML() string {
	return "functions:\n  runnerURL: \"https://" + f.runnerAddr + "\"\n"
}

// runnerYAML is the configuration of the process that runs fnrunner and
// egress. Outbound HTTP of a function goes to the egress role at its
// address, which refuses loopback and private destinations as in
// production.
func (f functionRoles) runnerYAML() string {
	return "server:\n  roles: [fnrunner, egress]\n  shutdownGrace: 5s\n" +
		"fnrunner:\n  listen: \"" + f.runnerAddr + "\"\n" + f.tlsYAML() +
		"  apiURL: \"https://" + f.apiAddr + "\"\n  egressAddr: \"" + f.egressAddr + "\"\n" +
		"  artifactKeys:\n    - keyID: " + f.artifactKeyID + "\n      publicKey: " + f.artifactKey + "\n" +
		"egress:\n  listen: \"" + f.egressAddr + "\"\n" + f.tlsYAML()
}

// startRunner runs the fnrunner and egress roles until the test ends and
// returns a client of the runner with the api role's identity. It returns
// once the runner answers.
func (f functionRoles) startRunner(t *testing.T) fnpbconnect.FunctionRunnerServiceClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	built, err := Build(ctx, testConfig(t, f.runnerYAML()), discard(), "test")
	if err != nil {
		cancel()
		t.Fatalf("Build the fnrunner and egress roles: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- built.Server.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done; built.Close() })

	cert, err := tls.LoadX509KeyPair(f.certFile, f.keyFile)
	if err != nil {
		t.Fatal(err)
	}
	mutual := &http.Client{Timeout: 5 * time.Minute, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: f.pool, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13,
	}}}
	client := fnpbconnect.NewFunctionRunnerServiceClient(mutual, "https://"+f.runnerAddr)
	deadline := time.Now().Add(30 * time.Second)
	for {
		// A malformed module reference is refused by the runner itself, so the
		// answer proves the handler is mounted.
		_, err := client.Prepare(ctx, connect.NewRequest(&fnpb.PrepareRequest{ModuleSha256: "not-a-digest"}))
		if connect.CodeOf(err) == connect.CodeInvalidArgument {
			return client
		}
		select {
		case err := <-done:
			t.Fatalf("the fnrunner process stopped while starting: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("the fnrunner listener never answered: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// signingDirOf is where a stack keeps its signing keys.
func signingDirOf(dir string) string { return filepath.Join(dir, "keys") }

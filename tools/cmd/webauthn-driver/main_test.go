// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// environment returns a getenv over a map.
func environment(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// providerPEM is the certificate of a throwaway TLS server, as the walkthrough
// hands it to the hooks.
func providerPEM(t *testing.T) string {
	t.Helper()
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
}

// TestRunUsage_SEC103 refuses what is not a subcommand and every
// configuration with a missing piece, before anything touches the network.
//
// Verifies: SEC-103.
func TestRunUsage_SEC103(t *testing.T) {
	full := map[string]string{
		"WALKTHROUGH_SERVER": "http://127.0.0.1:1", "WALKTHROUGH_ORG": "o", "WALKTHROUGH_ACTOR": "alice",
		"WALKTHROUGH_EMAIL": "alice@walkthrough.example", "WALKTHROUGH_DRIVER_DIR": t.TempDir(), "WALKTHROUGH_IDP_CA_PEM": providerPEM(t),
	}
	for _, args := range [][]string{nil, {"login"}, {"token", "extra"}} {
		var stderr bytes.Buffer
		if code := run(t.Context(), args, environment(full), &bytes.Buffer{}, &stderr); code != exitUsage || !strings.Contains(stderr.String(), "usage:") {
			t.Errorf("run(%v) = %d, %q; want usage", args, code, stderr.String())
		}
	}
	for name := range full {
		env := map[string]string{}
		for k, v := range full {
			if k != name {
				env[k] = v
			}
		}
		var stderr bytes.Buffer
		if code := run(t.Context(), []string{"token"}, environment(env), &bytes.Buffer{}, &stderr); code != exitUsage || !strings.Contains(stderr.String(), name) {
			t.Errorf("without %s: run = %d, %q; want a usage error naming it", name, code, stderr.String())
		}
	}
}

// TestRunFailure_SEC103 reports a failed ceremony on standard error and prints
// no token.
//
// Verifies: SEC-103.
func TestRunFailure_SEC103(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	env := environment(map[string]string{
		"WALKTHROUGH_SERVER": server.URL, "WALKTHROUGH_ORG": "o", "WALKTHROUGH_ACTOR": "alice",
		"WALKTHROUGH_EMAIL": "alice@walkthrough.example", "WALKTHROUGH_DRIVER_DIR": t.TempDir(), "WALKTHROUGH_IDP_CA_PEM": providerPEM(t),
		"WALKTHROUGH_STEPUP_URL": "https://plux.test/stepup?request=r",
	})
	for _, sub := range []string{"token", "stepup"} {
		var stdout, stderr bytes.Buffer
		if code := run(t.Context(), []string{sub}, env, &stdout, &stderr); code != exitFailed || stdout.Len() != 0 || !strings.Contains(stderr.String(), sub) {
			t.Errorf("%s: code %d, stdout %q, stderr %q", sub, code, stdout.String(), stderr.String())
		}
	}
}

// TestClients_SEC103 builds the TLS clients: they need a certificate to trust.
//
// Verifies: SEC-103.
func TestClients_SEC103(t *testing.T) {
	if _, err := httpClient([]byte("not a certificate"), false); err == nil {
		t.Error("httpClient accepted text that holds no certificate")
	}
	if _, err := serverClient(filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Error("serverClient accepted a CA file that does not exist")
	}
	file := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(file, []byte(providerPEM(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err := serverClient(file); err != nil || c == nil {
		t.Errorf("serverClient(%s) = %v, %v", file, c, err)
	}
	if c, err := serverClient(""); err != nil || c == nil {
		t.Errorf("serverClient(\"\") = %v, %v", c, err)
	}
	if orDefault("", "acme") != "acme" || orDefault("x", "acme") != "x" {
		t.Error("orDefault does not default")
	}
}

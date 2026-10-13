// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/observability"
)

// Verifies: FN-017, SEC-043.
// A process that runs only the egress role starts without a database,
// serves the proxy over mutual TLS, and drains on shutdown.
func TestEgressRoleRunsAlone_FN_017(t *testing.T) {
	t.Parallel()
	certFile, keyFile, pool := identity(t)
	addr := freeAddr(t)
	cfg := testConfig(t, "server:\n  roles: [egress]\n  shutdownGrace: 5s\negress:\n  listen: \""+addr+"\"\n"+
		"  internalTLS:\n    caFile: "+certFile+"\n    certFile: "+certFile+"\n    keyFile: "+keyFile+"\n")
	if !cfg.NoDataAccess() || cfg.Database.URL != "" {
		t.Fatalf("configuration = %+v", cfg.Server.Roles)
	}
	built, err := Build(context.Background(), cfg, discard(), "test")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer built.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- built.Server.Run(ctx) }()

	// A client with the internal identity gets through the handshake and a
	// refusal of a non-CONNECT request; one without is turned away.
	client := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}
	var answer string
	deadline := time.Now().Add(5 * time.Second)
	for {
		c := client.Clone()
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			t.Fatal(err)
		}
		c.Certificates = []tls.Certificate{cert}
		conn, err := tls.Dial("tcp", addr, c)
		if err == nil {
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			_, _ = fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
			buf := make([]byte, 64)
			n, _ := conn.Read(buf)
			answer = string(buf[:n])
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the egress listener never came up: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.HasPrefix(answer, "HTTP/1.1 405") {
		t.Errorf("answer = %q", answer)
	}
	if conn, err := tls.Dial("tcp", addr, client); err == nil {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
		if n, err := conn.Read(make([]byte, 1)); err == nil || n != 0 {
			t.Error("a client without a certificate was served")
		}
		_ = conn.Close()
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

// Verifies: FN-017.
func TestEgressMetricsJoinTheProcessRegistry_FN_017(t *testing.T) {
	t.Parallel()
	certFile, keyFile, _ := identity(t)
	cfg := testConfig(t, "server:\n  roles: [egress]\negress:\n  listen: \"127.0.0.1:0\"\n"+
		"  internalTLS:\n    caFile: "+certFile+"\n    certFile: "+certFile+"\n    keyFile: "+keyFile+"\n")
	m := observability.NewMetrics()
	if _, err := New(Deps{Config: cfg, Log: discard(), Metrics: m}); err != nil {
		t.Fatalf("New: %v", err)
	}
	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range families {
		found[f.GetName()] = true
	}
	if !found["plux_egress_tunnels_total"] || !found["plux_egress_bytes_total"] {
		t.Errorf("registry lacks the egress metrics: %v", found)
	}
}

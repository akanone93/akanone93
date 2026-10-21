// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package cachetest

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// binaries are the server executables tried, in order.
var binaries = []string{"valkey-server", "redis-server"}

// Server is a running server that a test can stop and start again on the
// same address, to simulate an outage.
type Server struct {
	// URL is the redis:// URL clients connect to.
	URL string

	t    testing.TB
	bin  string
	port string
	cmd  *exec.Cmd
}

// Start runs a server for the test. It skips the test when no server
// binary is installed.
func Start(t testing.TB) *Server {
	t.Helper()
	bin := ""
	for _, name := range binaries {
		if path, err := exec.LookPath(name); err == nil {
			bin = path
			break
		}
	}
	if bin == "" {
		t.Skip("install valkey-server or redis-server to run this test (QA-005)")
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	if err := ln.Close(); err != nil {
		t.Fatalf("release the port: %v", err)
	}
	s := &Server{URL: "redis://127.0.0.1:" + port, t: t, bin: bin, port: port}
	s.Restart()
	t.Cleanup(s.Stop)
	return s
}

// Restart starts the server if it is stopped, and waits until it answers.
func (s *Server) Restart() {
	s.t.Helper()
	if s.cmd != nil {
		return
	}
	//nolint:gosec // a test helper runs the server binary it found on PATH
	cmd := exec.CommandContext(context.WithoutCancel(s.t.Context()), s.bin,
		"--port", s.port, "--bind", "127.0.0.1", "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		s.t.Fatalf("start %s: %v", s.bin, err)
	}
	s.cmd = cmd
	s.waitUntilReady()
}

// Stop kills the server and waits for it to exit. Stopping a stopped
// server does nothing.
func (s *Server) Stop() {
	if s.cmd == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	s.cmd = nil
}

// waitUntilReady waits for the port to accept connections.
func (s *Server) waitUntilReady() {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var d net.Dialer
		c, err := d.DialContext(s.t.Context(), "tcp", net.JoinHostPort("127.0.0.1", s.port))
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("%s did not accept connections on port %s", s.bin, strconv.Quote(s.port))
}

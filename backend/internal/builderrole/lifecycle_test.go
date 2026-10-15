// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Verifies: FN-016.
func TestRefusalSeparatesTheSourcesFaultFromTheRolesFailure_FN_016(t *testing.T) {
	sourceFault := &plxerr.Error{Code: plxerr.InvalidProjectLayout, Message: "no Cargo.toml", Err: errors.New("walked 3 files")}
	resp, err := refusal(fmt.Errorf("extract: %w", sourceFault))
	if err != nil {
		t.Fatalf("a source fault is a response, not an RPC error: %v", err)
	}
	if len(resp.GetDiagnosticsJson()) == 0 {
		t.Fatal("a source fault must carry diagnostics")
	}
	if got := string(resp.GetDiagnosticsJson()); !strings.Contains(got, plxerr.InvalidProjectLayout.String()) || !strings.Contains(got, "no Cargo.toml: walked 3 files") {
		t.Errorf("diagnostics = %s; want the code and the message joined with its cause", got)
	}

	cases := map[string]struct {
		err  error
		code connect.Code
	}{
		"sandbox denied":    {fmt.Errorf("run: %w", builder.ErrSandboxUnavailable), connect.CodeFailedPrecondition},
		"cancelled":         {context.Canceled, connect.CodeCanceled},
		"deadline exceeded": {fmt.Errorf("build: %w", context.DeadlineExceeded), connect.CodeDeadlineExceeded},
		"anything else":     {errors.New("disk on fire"), connect.CodeInternal},
	}
	for name, c := range cases {
		resp, err := refusal(c.err)
		if resp != nil || codeOf(err) != c.code {
			t.Errorf("%s: response %v, error %v; want %v", name, resp, err, c.code)
		}
	}
}

// Verifies: FN-051.
func TestLinkRefusalMirrorsTheBuildRefusal_FN_051(t *testing.T) {
	resp, err := linkRefusal(&plxerr.Error{Code: plxerr.InvalidProjectLayout, Message: "bad layout"})
	if err != nil || !strings.Contains(string(resp.GetDiagnosticsJson()), plxerr.InvalidProjectLayout.String()) || len(resp.GetModule()) != 0 {
		t.Errorf("a source fault: response %v, error %v; want diagnostics and no module", resp, err)
	}
	resp, err = linkRefusal(builder.ErrSandboxUnavailable)
	if resp != nil || codeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a refused sandbox: response %v, error %v; want a failed precondition", resp, err)
	}
}

// Verifies: FN-051, FN-016.
func TestLinkReportsAMalformedArchiveAsDiagnostics_FN_051(t *testing.T) {
	s := newService(t, builder.NoSandbox{}, false, nil, nil)
	resp, err := s.Link(t.Context(), connect.NewRequest(&fnpb.LinkRequest{Sources: []*fnpb.LinkSource{linkSource("bank.a", gzipOf(t, "not a tar"))}}))
	if err != nil {
		t.Fatalf("a malformed archive is the source's fault: %v", err)
	}
	if len(resp.Msg.GetDiagnosticsJson()) == 0 || len(resp.Msg.GetModule()) != 0 {
		t.Errorf("response = %v; want diagnostics and no module", resp.Msg)
	}
}

// Verifies: FN-051, FN-016.
func TestLinkRefusesWhenTheKernelDeniesTheSandbox_FN_051(t *testing.T) {
	s := newService(t, refusingSandbox{}, false, nil, nil)
	device := archive(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"fee\"\n", "Cargo.lock": lockFile, "vendor/": "",
		"function.json": strings.NewReplacer("bank.transferFee", "bank.a", `"server"`, `"device"`).Replace(declaration),
	})
	_, err := s.Link(t.Context(), connect.NewRequest(&fnpb.LinkRequest{Sources: []*fnpb.LinkSource{linkSource("bank.a", device)}}))
	if codeOf(err) != connect.CodeFailedPrecondition || !strings.Contains(err.Error(), "builder.sandbox") {
		t.Errorf("error = %v; a refused sandbox must refuse the link and name the setting", err)
	}
}

// Verifies: LIM-001.
func TestARequestOfZeroAsksForTheRegistryMaximum_LIM_001(t *testing.T) {
	def, _ := limits.Lookup(limits.FunctionBuildTimeout)
	if got := ceiling(limits.FunctionBuildTimeout, 0); got != def.Max {
		t.Errorf("ceiling(0) = %d; want the maximum %d", got, def.Max)
	}
	if got := ceiling(limits.FunctionBuildTimeout, -5); got != def.Max {
		t.Errorf("ceiling(-5) = %d; a negative request asks for the maximum %d", got, def.Max)
	}
	if got := ceiling(limits.FunctionBuildTimeout, def.Max+1); got != def.Max {
		t.Errorf("ceiling(max+1) = %d; the maximum caps the request", got)
	}
	if got := ceiling(limits.FunctionBuildTimeout, 7); got != 7 {
		t.Errorf("ceiling(7) = %d; a smaller request stands", got)
	}
}

// Verifies: SEC-043, FN-016.
func TestTheServiceListensOnStartAndStopsOnStop_SEC_043(t *testing.T) {
	s := newService(t, builder.NoSandbox{}, false, nil, nil)
	if s.Name() != "builder" {
		t.Errorf("name = %q", s.Name())
	}
	if s.Addr() != nil {
		t.Error("the service reports an address before it listens")
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	addr := s.Addr()
	if addr == nil {
		t.Fatal("no address after Start")
	}
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr.String())
	if err != nil {
		t.Fatalf("the listener does not accept connections: %v", err)
	}
	_ = conn.Close()

	extra, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Serve(tls.NewListener(extra, s.tls)); err == nil {
		t.Error("a second listener was accepted")
	}
	if _, err := extra.Accept(); err == nil {
		t.Error("the refused listener must be closed")
	}
	if err := s.Stop(context.WithoutCancel(t.Context())); err != nil {
		t.Errorf("stop: %v", err)
	}
	if _, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr.String()); err == nil {
		t.Error("the listener still accepts connections after Stop")
	}
}

// Verifies: SEC-043.
func TestStartFailsWhenTheAddressCannotBeBound_SEC_043(t *testing.T) {
	held, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	s := newService(t, builder.NoSandbox{}, false, nil, nil)
	s.o.Listen = held.Addr().String()
	if err := s.Start(t.Context()); err == nil || !strings.Contains(err.Error(), "builderrole.Start") {
		t.Errorf("start on a bound address: %v", err)
	}
	if s.Addr() != nil {
		t.Error("a failed start must not report an address")
	}
}

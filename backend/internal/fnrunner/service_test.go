// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Verifies: SEC-043, FN-013.
// The runner serves FunctionRunnerService over mutual TLS: a role with the
// internal certificate invokes, one without is turned away, and Stop drains
// and closes the runner.
func TestServiceRequiresMutualTLS_SEC_043(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	conf, err := httpx.InternalServerTLS(h.id)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(h.runner, "127.0.0.1:0", conf, nil)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Name() != "fnrunner" || svc.Addr() != nil {
		t.Errorf("name %q, address %v before Start", svc.Name(), svc.Addr())
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	spare, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Serve(spare); err == nil {
		t.Error("a second Serve was accepted")
	}
	base := "https://" + svc.Addr().String()

	hc, err := httpx.NewInternalClient(h.id, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := fnpbconnect.NewFunctionRunnerServiceClient(hc, base)
	resp, err := client.Invoke(context.Background(), connect.NewRequest(h.request(`{"over":"mtls"}`)))
	if err != nil {
		t.Fatalf("Invoke with the internal identity: %v", err)
	}
	if got := outputJSON(t, resp.Msg); got["over"] != "mtls" {
		t.Errorf("output = %v", got)
	}
	if _, err := client.Prepare(context.Background(), connect.NewRequest(&fnpb.PrepareRequest{ModuleSha256: h.probeDigest(), Grant: "g"})); err != nil {
		t.Errorf("Prepare: %v", err)
	}
	// The error crosses the wire with its Plux code.
	msg := h.request("{}")
	msg.Limits.TimeoutMs = 0
	if _, err := client.Invoke(context.Background(), connect.NewRequest(msg)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a request without limits: %v", err)
	}

	anonymous := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: mustPool(t, h), MinVersion: tls.VersionTLS13,
	}}, Timeout: 5 * time.Second}
	if _, err := fnpbconnect.NewFunctionRunnerServiceClient(anonymous, base).Invoke(context.Background(), connect.NewRequest(h.request("{}"))); err == nil {
		t.Error("a caller without a client certificate was served")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if _, err := (&net.Dialer{Timeout: time.Second}).DialContext(context.Background(), "tcp", svc.Addr().String()); err == nil {
		t.Error("the listener is still open after Stop")
	}
}

// Verifies: SEC-043.
// A listener that does not verify client certificates is refused.
func TestNewServiceRefusesOpenListeners_SEC_043(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	for name, conf := range map[string]*tls.Config{"none": nil, "no client auth": {MinVersion: tls.VersionTLS13}} {
		if _, err := NewService(h.runner, "127.0.0.1:0", conf, nil); err == nil {
			t.Errorf("%s: NewService accepted the configuration", name)
		}
	}
	conf, err := httpx.InternalServerTLS(h.id)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(h.runner, "256.0.0.1:99999", conf, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err == nil {
		t.Error("Start listened on an invalid address")
	}
}

// Verifies: FN-010.
// Errors cross the boundary as Connect errors with their Plux code; a plain
// error reveals nothing.
func TestToConnect_FN_010(t *testing.T) {
	t.Parallel()
	err := toConnect(plxerr.New(plxerr.FunctionTimeout, "too slow"))
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded || plxCode(t, err) != "PLX-7001" {
		t.Errorf("timeout: %v", err)
	}
	err = toConnect(plxerr.New(plxerr.AssetRejected, "unmapped"))
	if connect.CodeOf(err) != connect.CodeInternal || plxCode(t, err) != plxerr.AssetRejected.String() {
		t.Errorf("unmapped code: %v", err)
	}
	err = toConnect(errors.New("secret detail"))
	if connect.CodeOf(err) != connect.CodeInternal || err.Error() == "" || strings.Contains(err.Error(), "secret detail") {
		t.Errorf("plain error: %v", err)
	}
	if got := codeOf(errors.New("x")); got != plxerr.InternalServerError {
		t.Errorf("codeOf(plain) = %v", got)
	}
	if got := resultOf(plxerr.New(plxerr.Code(1), "unregistered")); got == "" {
		t.Error("resultOf an unregistered code is empty")
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package renderworker_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/plxgolden"
	"github.com/nightCode42/plux3/backend/internal/renderpb"
	"github.com/nightCode42/plux3/backend/internal/renderworker"
)

// identity makes a self-signed certificate that serves as the internal CA and
// as the identity of both ends.
func identity(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "plux-internal"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// Verifies: STU-100, SEC-043.
// The worker answers only a client that presents a certificate of the internal
// CA, over TLS 1.3, and stops when its context ends.
func TestServeRequiresMutualTLS_STU_100(t *testing.T) {
	t.Parallel()
	cert, pool := identity(t)
	fonts, err := plxgolden.LoadFonts(fontsDir)
	if err != nil {
		t.Fatal(err)
	}
	h, err := renderworker.New(renderworker.Options{Fonts: fonts, WorkDir: t.TempDir(), Execute: (&fakeFlutter{}).execute})
	if err != nil {
		t.Fatal(err)
	}
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- renderworker.Serve(ctx, addr, &tls.Config{
			Certificates: []tls.Certificate{cert}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13,
		}, h, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()

	get := func(certs ...tls.Certificate) (*http.Response, error) {
		c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, Certificates: certs, MinVersion: tls.VersionTLS13}}}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+addr+"/livez", nil)
		if err != nil {
			return nil, err //nolint:wrapcheck // a test helper
		}
		return c.Do(req) //nolint:wrapcheck // a test helper
	}
	var resp *http.Response
	for range 50 {
		if resp, err = get(cert); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("a client with the internal identity: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("/livez = %d", resp.StatusCode)
	}
	if r, err := get(); err == nil {
		_ = r.Body.Close()
		t.Error("a client without a certificate was served")
	}
	stop()
	if err := <-done; err != nil {
		t.Errorf("Serve after the context ended: %v", err)
	}
	if renderworker.MaxRequestBytes() <= 0 {
		t.Error("the request bound is not positive")
	}
}

// Verifies: STU-100, LIM-001.
// The limits the server sends bind: a project with more pages than the
// organisation's golden.casesPerRun is refused.
func TestRenderHonoursTheOrganisationsLimits_STU_100(t *testing.T) {
	t.Parallel()
	c := startWorker(t, &fakeFlutter{})
	packed, err := renderworker.Pack(project(t), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := c.Render(context.Background(), connect.NewRequest(&renderpb.RenderRequest{
		After: packed, Devices: []string{smallest},
		Limits: &renderpb.RenderLimits{Cases: 3},
	}))
	if err == nil {
		stream.Receive()
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition || !strings.Contains(err.Error(), "case") {
		t.Errorf("5 cases against a limit of 3: %v", err)
	}
}

// Verifies: STU-100.
// A worker that is already rendering refuses the next call as exhausted, so
// that the server queues the job again instead of piling renders up.
func TestRenderIsRefusedWhileBusy_STU_100(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	started := make(chan struct{})
	fonts, err := plxgolden.LoadFonts(fontsDir)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeFlutter{}
	h, err := renderworker.New(renderworker.Options{Fonts: fonts, WorkDir: t.TempDir(), Execute: func(ctx context.Context, p *plxgolden.Plan, w string, l limitsSet) ([]plxgolden.Rendered, error) {
		started <- struct{}{}
		<-release
		return fake.execute(ctx, p, w, l)
	}})
	if err != nil {
		t.Fatal(err)
	}
	c := serve(t, h)
	first := make(chan error, 1)
	go func() { _, err := render(t, c, nil, project(t), smallest); first <- err }()
	<-started
	if _, err := render(t, c, nil, project(t), smallest); connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("a second render while busy: %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Errorf("the first render: %v", err)
	}
}

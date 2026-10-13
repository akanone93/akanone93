// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package egress

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// pki is a throw-away internal certificate authority.
type pki struct {
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	pem    []byte
	serial int64
}

// newPKI creates a CA that is valid for an hour.
func newPKI(t *testing.T) *pki {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "internal-ca"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &pki{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), serial: 1}
}

// issue creates a certificate for 127.0.0.1, usable as server and client
// certificate, and returns it with its PEM files.
func (p *pki) issue(t *testing.T, dir, name string) (cert tls.Certificate, certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p.serial++
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(p.serial), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.cert, &key.PublicKey, p.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	certFile, keyFile = filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")
	for path, data := range map[string][]byte{certFile: certPEM, keyFile: keyPEM} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cert, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert, certFile, keyFile
}

// clientConfig is the mutual-TLS identity of a role that talks to the
// proxy; with a nil certificate it presents none.
func (p *pki) clientConfig(cert *tls.Certificate) *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(p.cert)
	c := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}
	if cert != nil {
		c.Certificates = []tls.Certificate{*cert}
	}
	return c
}

// fakeResolver answers from a table and counts its lookups.
type fakeResolver struct {
	mu    sync.Mutex
	names map[string][]netip.Addr
	calls []string
}

func (r *fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, host)
	addrs, ok := r.names[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	return addrs, nil
}

func (r *fakeResolver) lookups() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// fakeDialer records the addresses it is asked for. A dial succeeds with
// one end of an in-memory pipe, handed to dialled, unless failFor says
// the address fails.
type fakeDialer struct {
	mu      sync.Mutex
	asked   []string
	failFor map[string]bool
	dialled chan net.Conn
}

func newFakeDialer() *fakeDialer { return &fakeDialer{dialled: make(chan net.Conn, 16)} }

func (d *fakeDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.mu.Lock()
	d.asked = append(d.asked, address)
	fail := d.failFor[address]
	d.mu.Unlock()
	if fail {
		return nil, errors.New("connection refused")
	}
	ours, theirs := net.Pipe()
	d.dialled <- theirs
	return ours, nil
}

func (d *fakeDialer) addresses() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.asked...)
}

// syncBuffer is a log sink safe for concurrent writers.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// harness is a running proxy with its test doubles.
type harness struct {
	srv      *Server
	addr     string
	pki      *pki
	client   tls.Certificate
	resolver *fakeResolver
	dialer   *fakeDialer
	reg      *prometheus.Registry
	logs     *syncBuffer
}

// startProxy runs a proxy on a loopback port with mutual TLS. tweak may
// change the options before the server is built.
func startProxy(t *testing.T, tweak func(*Options)) *harness {
	t.Helper()
	ca := newPKI(t)
	dir := t.TempDir()
	_, certFile, keyFile := ca.issue(t, dir, "egress")
	clientCert, _, _ := ca.issue(t, dir, "fnrunner")
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, ca.pem, 0o600); err != nil {
		t.Fatal(err)
	}
	conf, err := httpx.InternalServerTLS(httpx.InternalTLSOptions{CAFile: caFile, CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		pki: ca, client: clientCert, resolver: &fakeResolver{names: map[string][]netip.Addr{}},
		dialer: newFakeDialer(), reg: prometheus.NewRegistry(), logs: &syncBuffer{},
	}
	opts := Options{
		Listen: "127.0.0.1:0", TLS: conf, Resolver: h.resolver, Dialer: h.dialer, Registerer: h.reg,
		Log: slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
	if tweak != nil {
		tweak(&opts)
	}
	h.srv, err = New(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.srv.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	h.addr = h.srv.Addr().String()
	return h
}

// dialer returns a client Dialer with the harness's client identity.
func (h *harness) clientDialer() *Dialer {
	return &Dialer{Addr: h.addr, TLS: h.pki.clientConfig(&h.client), Timeout: 5 * time.Second}
}

// rawConnect opens a mutual-TLS connection, writes the request text as
// is and reads one response. The connection is returned for the cases
// where it is a tunnel.
func (h *harness) rawConnect(t *testing.T, request string) (*http.Response, net.Conn, *bufio.Reader) {
	t.Helper()
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: h.pki.clientConfig(&h.client)}
	conn, err := d.DialContext(context.Background(), "tcp", h.addr)
	if err != nil {
		t.Fatalf("dial the proxy: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("read the response: %v", err)
	}
	return resp, conn, br
}

// connectRequest renders a CONNECT request with the two headers.
func connectRequest(target, allow, function string) string {
	r := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n"
	if allow != "" {
		r += HeaderAllow + ": " + allow + "\r\n"
	}
	if function != "" {
		r += HeaderFunction + ": " + function + "\r\n"
	}
	return r + "\r\n"
}

// counter reads one labelled counter from the harness's registry.
func (h *harness) counter(t *testing.T, name, label, value string) float64 {
	t.Helper()
	families, err := h.reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == label && l.GetValue() == value {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	t.Fatalf("no series %s{%s=%q}", name, label, value)
	return 0
}

// eventually polls cond until it holds or two seconds pass.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// addrs parses addresses for a resolver table.
func addrs(t *testing.T, s ...string) []netip.Addr {
	t.Helper()
	out := make([]netip.Addr, len(s))
	for i, v := range s {
		a, err := netip.ParseAddr(v)
		if err != nil {
			t.Fatal(fmt.Errorf("address %q: %w", v, err))
		}
		out[i] = a
	}
	return out
}

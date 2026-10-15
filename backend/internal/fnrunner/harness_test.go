// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	"github.com/nightCode42/plux3/backend/internal/egress"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/httpx"
)

// identity writes a self-signed certificate for 127.0.0.1 that serves as
// server certificate, client certificate and CA alike (SEC-043).
func identity(t *testing.T) httpx.InternalTLSOptions {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "plux-test"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	write := func(path string, block *pem.Block) {
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	write(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return httpx.InternalTLSOptions{CAFile: certFile, CertFile: certFile, KeyFile: keyFile}
}

// probe reads a conformance module of schema/testdata/functions.
func probe(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "schema", "testdata", "functions", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// digest is the lower-case hex SHA-256 of b.
func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// signer signs artifacts the way the builder's statement does, as far as
// the runner reads them.
type signer struct {
	id  string
	key ed25519.PrivateKey
}

func newSigner(t *testing.T, id string) signer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return signer{id: id, key: key}
}

func (s signer) public() ed25519.PublicKey { return s.key.Public().(ed25519.PublicKey) } //nolint:forcetypeassert // an Ed25519 key

// artifact returns a GetModule response whose statement names the module.
func (s signer) artifact(module []byte) *fnpb.GetModuleResponse {
	statement := []byte(fmt.Sprintf(`{"abiVersion":1,"moduleSha256":%q}`, digest(module)))
	return s.sign(module, statement)
}

// sign signs an arbitrary statement.
func (s signer) sign(module, statement []byte) *fnpb.GetModuleResponse {
	return &fnpb.GetModuleResponse{
		Module: module, Statement: statement, Signature: ed25519.Sign(s.key, statement), KeyId: s.id,
	}
}

// fakeAPI is the api role's FunctionHostService. It records every grant it
// is presented with. It is safe for concurrent use.
type fakeAPI struct {
	fnpbconnect.UnimplementedFunctionHostServiceHandler

	mu        sync.Mutex
	modules   map[string]*fnpb.GetModuleResponse
	fetches   int
	gate      chan struct{} // when set, GetModule waits for it to close
	kv        map[string][]byte
	secrets   map[string][]byte
	nested    func(*fnpb.FunctionHostServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error)
	failWith  connect.Code // when non-zero, kv, secret and nested calls fail with it
	grants    map[string][]string
	depths    []uint32
	traceFrom []string
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		modules: map[string]*fnpb.GetModuleResponse{}, kv: map[string][]byte{}, secrets: map[string][]byte{},
		grants: map[string][]string{},
	}
}

// enter records a call and returns the configured failure.
func (a *fakeAPI) enter(op, grant string, h http.Header) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.grants[op] = append(a.grants[op], grant)
	a.traceFrom = append(a.traceFrom, h.Get("traceparent"))
	if a.failWith != 0 && op != "GetModule" {
		return connect.NewError(a.failWith, errors.New("fake failure"))
	}
	return nil
}

func (a *fakeAPI) calls(op string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.grants[op]...)
}

func (a *fakeAPI) GetModule(_ context.Context, req *connect.Request[fnpb.GetModuleRequest]) (*connect.Response[fnpb.GetModuleResponse], error) {
	if err := a.enter("GetModule", req.Msg.GetGrant(), req.Header()); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.fetches++
	gate, resp := a.gate, a.modules[req.Msg.GetModuleSha256()]
	a.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if resp == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such module"))
	}
	return connect.NewResponse(resp), nil
}

func (a *fakeAPI) KvGet(_ context.Context, req *connect.Request[fnpb.KvGetRequest]) (*connect.Response[fnpb.KvGetResponse], error) {
	if err := a.enter("KvGet", req.Msg.GetGrant(), req.Header()); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.kv[req.Msg.GetKey()]
	return connect.NewResponse(&fnpb.KvGetResponse{Found: ok, Value: v}), nil
}

func (a *fakeAPI) KvPut(_ context.Context, req *connect.Request[fnpb.KvPutRequest]) (*connect.Response[fnpb.KvPutResponse], error) {
	if err := a.enter("KvPut", req.Msg.GetGrant(), req.Header()); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.kv[req.Msg.GetKey()] = req.Msg.GetValue()
	return connect.NewResponse(&fnpb.KvPutResponse{}), nil
}

func (a *fakeAPI) KvDelete(_ context.Context, req *connect.Request[fnpb.KvDeleteRequest]) (*connect.Response[fnpb.KvDeleteResponse], error) {
	if err := a.enter("KvDelete", req.Msg.GetGrant(), req.Header()); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.kv[req.Msg.GetKey()]
	delete(a.kv, req.Msg.GetKey())
	return connect.NewResponse(&fnpb.KvDeleteResponse{Deleted: ok}), nil
}

func (a *fakeAPI) GetSecret(_ context.Context, req *connect.Request[fnpb.GetSecretRequest]) (*connect.Response[fnpb.GetSecretResponse], error) {
	if err := a.enter("GetSecret", req.Msg.GetGrant(), req.Header()); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.secrets[req.Msg.GetName()]
	return connect.NewResponse(&fnpb.GetSecretResponse{Found: ok, Value: v}), nil
}

func (a *fakeAPI) Invoke(_ context.Context, req *connect.Request[fnpb.FunctionHostServiceInvokeRequest]) (*connect.Response[fnpb.FunctionHostServiceInvokeResponse], error) {
	if err := a.enter("Invoke", req.Msg.GetGrant(), req.Header()); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.depths = append(a.depths, req.Msg.GetDepth())
	nested := a.nested
	a.mu.Unlock()
	if nested == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such function"))
	}
	out, err := nested(req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&fnpb.FunctionHostServiceInvokeResponse{Result: out}), nil
}

// startAPI serves the fake over mutual TLS and returns a client for it.
func startAPI(t *testing.T, id httpx.InternalTLSOptions, api *fakeAPI) fnpbconnect.FunctionHostServiceClient {
	t.Helper()
	conf, err := httpx.InternalServerTLS(id)
	if err != nil {
		t.Fatal(err)
	}
	_, h := fnpbconnect.NewFunctionHostServiceHandler(api)
	mux := http.NewServeMux()
	mux.Handle("/", h)
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = conf
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	hc, err := httpx.NewInternalClient(id, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return fnpbconnect.NewFunctionHostServiceClient(hc, srv.URL)
}

// fakeResolver resolves the test names to loopback.
type fakeResolver struct{}

func (fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if host == "upstream.test" || host == "other.test" {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	return nil, errors.New("no such host")
}

// startEgress runs a real egress proxy and returns the dialer for it. The
// proxy reaches loopback only when allowLoopback is set.
func startEgress(t *testing.T, id httpx.InternalTLSOptions, allowLoopback bool, ports ...int) *egress.Dialer {
	t.Helper()
	conf, err := httpx.InternalServerTLS(id)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := egress.New(egress.Options{
		Listen: "127.0.0.1:0", TLS: conf, Policy: httpx.Policy{AllowLoopback: allowLoopback},
		Ports: ports, Resolver: fakeResolver{}, ConnectTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := proxy.Stop(ctx); err != nil {
			t.Errorf("Stop the proxy: %v", err)
		}
	})
	clientTLS, err := httpx.InternalClientTLS(id)
	if err != nil {
		t.Fatal(err)
	}
	return &egress.Dialer{Addr: proxy.Addr().String(), TLS: clientTLS, Timeout: 5 * time.Second}
}

// upstream starts a plain HTTP server on loopback and returns its port.
func upstream(t *testing.T, h http.Handler) int {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().(*net.TCPAddr).Port //nolint:forcetypeassert // a TCP listener
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

// zeroReader is the deterministic randomness of the tests.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

// harness is a runner with its doubles.
type harness struct {
	t      *testing.T
	runner *Runner
	api    *fakeAPI
	signer signer
	logs   *syncBuffer
	id     httpx.InternalTLSOptions
	reg    *registry
}

// harnessOptions are the knobs tests turn.
type harnessOptions struct {
	egress *egress.Dialer
	tweak  func(*Options)
}

// newHarness builds a runner whose api double serves probe.wasm and
// probe-device.wasm, signed with the configured key.
func newHarness(t *testing.T, ho harnessOptions) *harness {
	t.Helper()
	id := identity(t)
	api := newFakeAPI()
	s := newSigner(t, "k1")
	for _, name := range []string{"probe.wasm", "probe-device.wasm"} {
		m := probe(t, name)
		api.modules[digest(m)] = s.artifact(m)
	}
	h := &harness{t: t, api: api, signer: s, logs: &syncBuffer{}, id: id, reg: newRegistry()}
	opts := Options{
		Keys: map[string]ed25519.PublicKey{"k1": s.public()},
		Host: startAPI(t, id, api), Egress: ho.egress,
		Clock: time.Now, Random: zeroReader{}, MaxMemoryBytes: 512 << 20,
		MaxModules: 8, MaxModuleBytes: 64 << 20, MaxConcurrent: 8, HTTPResponseBytes: 1 << 20,
		Log:        slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Registerer: h.reg.reg,
	}
	if opts.Egress == nil {
		opts.Egress = &egress.Dialer{Addr: "127.0.0.1:1", TLS: nil}
	}
	if ho.tweak != nil {
		ho.tweak(&opts)
	}
	r, err := New(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(context.Background()); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	h.runner = r
	return h
}

// probeDigest is the digest of probe.wasm.
func (h *harness) probeDigest() string { return digest(probe(h.t, "probe.wasm")) }

// request is an invocation of the probe with generous limits.
func (h *harness) request(input string) *fnpb.FunctionRunnerServiceInvokeRequest {
	return &fnpb.FunctionRunnerServiceInvokeRequest{
		Function: "demo.probe", Version: 3, ModuleSha256: h.probeDigest(), Input: []byte(input),
		Limits: &fnpb.InvocationLimits{
			TimeoutMs: 10000, MemoryBytes: 64 << 20, OutputBytes: 1 << 20, InputBytes: 1 << 20,
			HttpCalls: 10, CallDepth: 3, LogBytes: 4096,
		},
		Capabilities: &fnpb.Capabilities{},
		Grant:        "grant-1", App: "app-1", Environment: "prod", AssuranceLevel: "AL2",
		Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
}

// call runs entry index and returns the output, which must be set.
func (h *harness) call(index uint32, msg *fnpb.FunctionRunnerServiceInvokeRequest) (*fnpb.FunctionRunnerServiceInvokeResponse, error) {
	h.t.Helper()
	resp, err := h.runner.invoke(context.Background(), msg, index)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// plxCode returns the Plux code carried by a Connect error.
func plxCode(t *testing.T, err error) string {
	t.Helper()
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("error %v is not a Connect error", err)
	}
	for _, d := range ce.Details() {
		v, derr := d.Value()
		if derr != nil {
			continue
		}
		if info, ok := v.(*errdetails.ErrorInfo); ok {
			return info.GetMetadata()["code"]
		}
	}
	t.Fatalf("error %v carries no ErrorInfo", err)
	return ""
}

// eventually polls cond until it holds or five seconds pass.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// mustPool returns the harness's CA as a pool.
func mustPool(t *testing.T, h *harness) *x509.CertPool {
	t.Helper()
	pemBytes, err := os.ReadFile(h.id.CAFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		t.Fatal("no certificate in the CA file")
	}
	return pool
}

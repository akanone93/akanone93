// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// declaration is a valid server-placed function document.
const declaration = `{"schemaVersion":"1.0.0","kind":"function","id":"0194f2a0-7b1c-7d3e-8a5f-3c4d5e6f7a8b","name":"bank.transferFee","placement":"server"}`

// lockFile is a Cargo.lock with the crate itself.
const lockFile = "version = 4\n\n[[package]]\nname = \"fee\"\nversion = \"0.1.0\"\n"

// archive packs the files as a gzip-compressed tar archive.
func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range files {
		if strings.HasSuffix(name, "/") {
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// request is a build request for the archive.
func request(archive []byte) *fnpb.BuildRequest {
	sum := sha256.Sum256(archive)
	return &fnpb.BuildRequest{
		BuildId: "b1", Source: archive, SourceSha256: hex.EncodeToString(sum[:]), Name: "bank.transferFee",
		Limits: &fnpb.BuildLimits{TimeoutMs: 1000, MemoryBytes: 1 << 30},
	}
}

// refusingSandbox stands in for a kernel that denies namespaces.
type refusingSandbox struct{}

func (refusingSandbox) Run(context.Context, builder.Command) (builder.Result, error) {
	return builder.Result{}, builder.ErrSandboxUnavailable
}

// blockingSandbox holds every command until released.
type blockingSandbox struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingSandbox) Run(ctx context.Context, _ builder.Command) (builder.Result, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return builder.Result{}, builder.ErrSandboxUnavailable
}

// pki is a throw-away internal certificate authority.
type pki struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newPKI(t *testing.T) *pki {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "internal-ca"}, NotBefore: time.Now().Add(-time.Minute),
		NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &pki{cert: cert, key: key, pool: pool}
}

// issue creates a certificate for 127.0.0.1 usable on either side.
func (p *pki) issue(t *testing.T, serial int64) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "role"}, NotBefore: time.Now().Add(-time.Minute),
		NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.cert, &key.PublicKey, p.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// newService returns a Service over the sandbox with the logger.
func newService(t *testing.T, sandbox builder.Sandbox, unsandboxed bool, log *slog.Logger, serverTLS *tls.Config) *Service {
	t.Helper()
	if serverTLS == nil {
		serverTLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert}
	}
	s, err := New(t.Context(), Options{
		Listen: "127.0.0.1:0", TLS: serverTLS,
		Tools: builder.Config{
			Sandbox: sandbox, Clock: time.Now, Identity: "builder-test", SDKDir: t.TempDir(),
			Cargo: "true", Rustc: "true", WasmOpt: []string{"true"},
		},
		Unsandboxed: unsandboxed, Limits: limits.Defaults(), Concurrency: 1,
		Clock: time.Now, Random: rand.Reader, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.WithoutCancel(t.Context())) })
	return s
}

// diagnostics decodes the diagnostics of a response.
func diagnostics(t *testing.T, r *fnpb.BuildResponse) plxerr.Diagnostics {
	t.Helper()
	var ds plxerr.Diagnostics
	if err := json.Unmarshal(r.GetDiagnosticsJson(), &ds); err != nil {
		t.Fatalf("diagnostics: %v", err)
	}
	return ds
}

// codeOf returns the connect code of an error.
func codeOf(err error) connect.Code { return connect.CodeOf(err) }

// Verifies: FN-016.
func TestBuildRefusesAWrongRequest_FN_016(t *testing.T) {
	s := newService(t, builder.NoSandbox{}, false, nil, nil)
	good := archive(t, map[string]string{"Cargo.toml": "[package]"})
	empty := request(good)
	empty.Source = nil
	wrongSum := request(good)
	wrongSum.SourceSha256 = strings.Repeat("0", 64)
	noName := request(good)
	noName.Name = ""
	for name, req := range map[string]*fnpb.BuildRequest{"empty source": empty, "wrong hash": wrongSum, "no name": noName} {
		_, err := s.Build(t.Context(), connect.NewRequest(req))
		if codeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

// Verifies: FN-016, FN-008.
func TestBuildReportsASourceThatDoesNotBuildAsDiagnostics_FN_016(t *testing.T) {
	s := newService(t, builder.NoSandbox{}, false, nil, nil)
	cases := map[string]struct {
		files map[string]string
		code  plxerr.Code
	}{
		"no document":       {map[string]string{"Cargo.toml": "[package]"}, plxerr.InvalidProjectLayout},
		"wrong name":        {map[string]string{"Cargo.toml": "[package]", "function.json": strings.Replace(declaration, "bank.transferFee", "bank.other", 1)}, plxerr.FunctionDeclarationMismatch},
		"invalid placement": {map[string]string{"Cargo.toml": "[package]", "function.json": strings.Replace(declaration, `"server"`, `"cloud"`, 1)}, plxerr.InvalidEnumValue},
	}
	for name, c := range cases {
		resp, err := s.Build(t.Context(), connect.NewRequest(request(archive(t, c.files))))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ds := diagnostics(t, resp.Msg)
		if len(resp.Msg.GetModule()) != 0 || !ds.HasErrors() || ds[len(ds)-1].Code != c.code && ds[0].Code != c.code {
			t.Errorf("%s: module %d bytes, diagnostics %v; want %s", name, len(resp.Msg.GetModule()), ds, c.code)
		}
	}
	resp, err := s.Build(t.Context(), connect.NewRequest(request(append([]byte(nil), gzipOf(t, "not a tar")...))))
	if err != nil || !diagnostics(t, resp.Msg).HasErrors() {
		t.Errorf("a malformed archive must be a diagnostic: %v", err)
	}
}

// gzipOf compresses text, which is not a tar archive.
func gzipOf(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Verifies: FN-016.
func TestBuildRefusesWhenTheKernelDeniesTheSandbox_FN_016(t *testing.T) {
	s := newService(t, refusingSandbox{}, false, nil, nil)
	good := archive(t, map[string]string{"Cargo.toml": "[package]\nname = \"fee\"\n", "Cargo.lock": lockFile, "vendor/": "", "function.json": declaration})
	_, err := s.Build(t.Context(), connect.NewRequest(request(good)))
	if codeOf(err) != connect.CodeFailedPrecondition || !strings.Contains(err.Error(), "builder.sandbox") {
		t.Errorf("error = %v; a refused sandbox must refuse the build and name the setting", err)
	}
}

// syncBuffer is a log sink safe for concurrent use.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Verifies: FN-016.
func TestUnsandboxedBuildsWarnOnStartAndOnEveryBuild_FN_016(t *testing.T) {
	var out syncBuffer
	log := slog.New(slog.NewTextHandler(&out, nil))
	s := newService(t, builder.NoSandbox{}, true, log, nil)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Serve(ln); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := s.Build(t.Context(), connect.NewRequest(request(archive(t, map[string]string{"Cargo.toml": "x"})))); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(out.String(), "builder.sandbox is none"); n != 2 {
		t.Errorf("the warning was logged %d times for 2 builds; log:\n%s", n, out.String())
	}
	sandboxed := &syncBuffer{}
	quiet := newService(t, builder.NoSandbox{}, false, slog.New(slog.NewTextHandler(sandboxed, nil)), nil)
	if _, err := quiet.Build(t.Context(), connect.NewRequest(request(archive(t, map[string]string{"Cargo.toml": "x"})))); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sandboxed.String(), "builder.sandbox is none") {
		t.Error("a sandboxed role warned about the sandbox")
	}
}

// Verifies: FN-016.
func TestBuildsAreBoundedInNumber_FN_016(t *testing.T) {
	sb := &blockingSandbox{started: make(chan struct{}), release: make(chan struct{})}
	s := newService(t, sb, false, nil, nil)
	good := archive(t, map[string]string{"Cargo.toml": "[package]\nname = \"fee\"\n", "Cargo.lock": lockFile, "vendor/": "", "function.json": declaration})
	first := make(chan error, 1)
	go func() {
		_, err := s.Build(t.Context(), connect.NewRequest(request(good)))
		first <- err
	}()
	<-sb.started
	_, err := s.Build(t.Context(), connect.NewRequest(request(good)))
	if codeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("the second build: %v; want resource exhausted", err)
	}
	close(sb.release)
	if err := <-first; err == nil {
		t.Error("the blocked build should have ended with the sandbox's refusal")
	}
}

// Verifies: SEC-043.
func TestListenerRequiresAClientCertificate_SEC_043(t *testing.T) {
	_, err := New(t.Context(), Options{
		Listen: "127.0.0.1:0", TLS: &tls.Config{}, Limits: limits.Defaults(), Concurrency: 1, Clock: time.Now, Random: rand.Reader,
	})
	if err == nil {
		t.Fatal("a listener without client authentication was accepted")
	}
	ca := newPKI(t)
	serverTLS := &tls.Config{Certificates: []tls.Certificate{ca.issue(t, 2)}, ClientCAs: ca.pool, ClientAuth: tls.RequireAndVerifyClientCert}
	s := newService(t, builder.NoSandbox{}, false, nil, serverTLS)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Serve(tls.NewListener(ln, s.tls)); err != nil {
		t.Fatal(err)
	}
	call := func(cert *tls.Certificate) error {
		conf := &tls.Config{RootCAs: ca.pool, MinVersion: tls.VersionTLS13}
		if cert != nil {
			conf.Certificates = []tls.Certificate{*cert}
		}
		hc := &http.Client{Transport: &http.Transport{TLSClientConfig: conf, ForceAttemptHTTP2: true}, Timeout: 10 * time.Second}
		client := fnpbconnect.NewBuilderServiceClient(hc, "https://"+ln.Addr().String())
		_, err := client.Build(t.Context(), connect.NewRequest(&fnpb.BuildRequest{}))
		return err
	}
	clientCert := ca.issue(t, 3)
	if err := call(&clientCert); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a client with the internal identity: %v; want the role's own refusal of an empty request", err)
	}
	var ce *connect.Error
	if err := call(nil); err == nil || errors.As(err, &ce) && ce.Code() == connect.CodeInvalidArgument {
		t.Errorf("a client without a certificate reached the handler: %v", err)
	}
}

// Verifies: FN-004, FN-005, FN-031.
func TestResponseCarriesTheModuleTheSchemasTheProvenanceAndTheTests_FN_005(t *testing.T) {
	a := &builder.Artifact{
		Module: []byte("wasm"), ModuleSHA256: "ab", ABIVersion: 1,
		Functions: []builder.Function{{Name: "bank.transferFee", Placement: builder.PlacementServer, InputSchema: []byte(`{"a":1}`), OutputSchema: []byte(`{"b":2}`)}},
		Provenance: builder.Provenance{
			SourceSHA256: "x", SDKSHA256: "sdk", Toolchain: "rustc 1.97.0", WasmOpt: "version_132", BuildTime: "2026-10-13T08:30:00Z",
			BuilderIdentity: "builder-1", Dependencies: []builder.Dependency{{Name: "serde", Version: "1.0.229", Checksum: "c1"}},
		},
	}
	decl := &builder.Declaration{Document: []byte(`{"kind":"function"}`)}
	r, err := responseOf(a, decl, []builder.TestResult{{Name: "t", Passed: true}, {Name: "u", Message: "no"}}, "source-hash")
	if err != nil {
		t.Fatal(err)
	}
	p := r.GetProvenance()
	if string(r.GetModule()) != "wasm" || r.GetAbiVersion() != 1 || r.GetPlacement() != "server" || string(r.GetDeclaration()) != `{"kind":"function"}` ||
		r.GetFunctions()[0].GetOutputSchema() != `{"b":2}` || p.GetSdkSha256() != "sdk" || p.GetSourceSha256() != "source-hash" ||
		p.GetWasmOptVersion() != "version_132" || len(p.GetDependencies()) != 1 || len(r.GetTestResults()) != 2 || r.GetTestResults()[1].GetPassed() {
		t.Errorf("response = %v", r)
	}
	a.Functions = append(a.Functions, a.Functions[0])
	if _, err := responseOf(a, decl, nil, ""); codeOf(err) != connect.CodeInternal {
		t.Errorf("two functions: %v; the role builds one", err)
	}
}

// Verifies: LIM-001.
func TestBuildLimitsAreTheSmallerOfTheRequestAndTheRegistryMaximum_LIM_001(t *testing.T) {
	s := newService(t, builder.NoSandbox{}, false, nil, nil)
	timeoutMax, _ := limits.Lookup(limits.FunctionBuildTimeout)
	memoryMax, _ := limits.Lookup(limits.FunctionBuildMemory)
	cfg := s.toolConfig(&fnpb.BuildLimits{TimeoutMs: 1000, MemoryBytes: 1 << 20})
	if cfg.Limits.Wall != time.Second || cfg.Limits.Memory != 1<<20 {
		t.Errorf("limits = %+v; the request's smaller values apply", cfg.Limits)
	}
	cfg = s.toolConfig(&fnpb.BuildLimits{TimeoutMs: 1 << 31, MemoryBytes: 1 << 62})
	if cfg.Limits.Wall != time.Duration(timeoutMax.Max)*time.Millisecond || cfg.Limits.Memory != uint64(memoryMax.Max) { //nolint:gosec // G115: a registry maximum.
		t.Errorf("limits = %+v; the registry maximum caps the request", cfg.Limits)
	}
	if cfg.SourceBytes != maxSource() {
		t.Errorf("source bytes = %d", cfg.SourceBytes)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/buildclient"
	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/builderrole"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/signing"
)

// feeManifest is the standalone manifest of the fee example: the workspace
// form of sdk/fn/examples/fee, with the patch that `cargo vendor` needs while
// plux-fn is unpublished.
const feeManifest = `[package]
name = "fee"
version = "0.1.0"
edition = "2024"
publish = false

[dependencies]
plux-fn = "0.1"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
schemars = "1.2"

[patch.crates-io]
plux-fn = { path = %q }
`

// runIn runs a setup command, failing the test with its output.
func runIn(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...) //nolint:gosec // G204: the test names its own setup commands.
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// feeSource lays out the fee example as an uploadable crate: its sources,
// its document (the example's identifiers are made valid), the vendored
// dependencies and the lock file.
func feeSource(t *testing.T, sdk, examples string) string {
	t.Helper()
	proj := filepath.Join(t.TempDir(), "fee")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(proj, "src"), os.DirFS(filepath.Join(examples, "fee", "src"))); err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join(examples, "fee", "function.json")) //nolint:gosec // G304: a fixed path in the repository.
	if err != nil {
		t.Fatal(err)
	}
	doc = bytes.Replace(doc, []byte(`"schemaVersion": 1,`), []byte(`"schemaVersion": "1.0.0",`), 1)
	doc = bytes.Replace(doc, []byte("94662393-d109-4bcf-a454-818ab471e664"), []byte("0194f2a0-7b1c-7d3e-8a5f-3c4d5e6f7a8b"), 1)
	for name, data := range map[string][]byte{
		"function.json": doc, "Cargo.toml": []byte(fmt.Sprintf(feeManifest, sdk)),
	} {
		if err := os.WriteFile(filepath.Join(proj, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runIn(t, proj, "cargo", "vendor", "--quiet")
	return proj
}

// packSource packs a crate directory as the gzip-compressed tar archive the
// builder role takes.
func packSource(t *testing.T, dir string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Uid, h.Gid, h.Uname, h.Gname = 0, 0, "", ""
		if err := tw.WriteHeader(h); err != nil || !info.Mode().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: walked from the test's own directory.
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// startBuilderRole serves a builder role over the real toolchain, without a
// sandbox, on a loopback address, and returns its URL.
func startBuilderRole(t *testing.T, sdk, wasmOptDir string) string {
	t.Helper()
	role, err := builderrole.New(t.Context(), builderrole.Options{
		Listen: "127.0.0.1:0", TLS: &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert},
		Tools: builder.Config{
			Sandbox: builder.NoSandbox{}, Clock: time.Now, Identity: "e2e-builder", SDKDir: sdk,
			WasmOpt: builder.DefaultWasmOpt(wasmOptDir),
		},
		Unsandboxed: true, Limits: limits.Defaults(), Concurrency: 1, Clock: time.Now, Random: rand.Reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// The role's own tests cover the mutual-TLS listener; here the listener is
	// plain so the test needs no certificates.
	if err := role.Serve(ln); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = role.Stop(t.Context()) })
	return "http://" + ln.Addr().String()
}

// Verifies: FN-004, FN-005, FN-009, FN-016, FN-031.
// The fee example goes from a source archive through the worker, the builder
// role (real Rust toolchain and Binaryen), the schema build read on the
// function host, the stored test cases run against the module, and the
// worker's signature over the statement, to a deployed version whose
// signature verifies under the function-artifact public key.
func TestRustBuildEndToEnd_FN_016(t *testing.T) {
	if os.Getenv("PLUX_RUST_E2E") != "1" {
		t.Skip("set PLUX_RUST_E2E=1 to build with the real Rust toolchain and Binaryen")
	}
	root, err := filepath.Abs("../../../sdk/fn")
	if err != nil {
		t.Fatal(err)
	}
	proj := feeSource(t, filepath.Join(root, "plux-fn"), filepath.Join(root, "examples"))
	archive := packSource(t, proj)
	wasmOpt := filepath.Join(t.TempDir(), "binaryen")
	if err := os.MkdirAll(wasmOpt, 0o750); err != nil {
		t.Fatal(err)
	}
	runIn(t, wasmOpt, "bun", "add", "--exact", "binaryen@132.0.0")

	w := newWorld(t, nil, nil)
	backend, err := signing.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := buildclient.NewSigner(backend, buildclient.DefaultKeyRef)
	if err != nil {
		t.Fatal(err)
	}
	client, err := buildclient.NewClient(buildclient.Options{
		HTTP: &http.Client{Timeout: 30 * time.Minute}, URL: startBuilderRole(t, filepath.Join(root, "plux-fn"), wasmOpt),
		Objects: w.store, MaxResponseBytes: 80 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	w.builder.delegate, w.signer.delegate = client, signer

	ctx := t.Context()
	src, err := w.svc.UploadSource(ctx, w.developer, w.app, w.envs["development"], "bank.transferFee", bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.svc.RequestBuild(ctx, w.developer, src.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := w.svc.RunBuild(ctx, w.queue.last(t)); err != nil {
		t.Fatalf("RunBuild: %v", err)
	}
	t.Logf("source %d bytes; build job (cargo, schema build, wasm-opt, tests, signature) took %s", len(archive), time.Since(start).Round(time.Second))
	done, err := w.svc.GetBuild(ctx, w.developer, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != function.BuildSucceeded || !done.Signed || done.Placement != function.PlacementServer {
		t.Fatalf("build = %+v", done)
	}
	if len(done.TestResults) < 5 {
		t.Fatalf("test results = %+v", done.TestResults)
	}
	for _, r := range done.TestResults {
		if !r.Passed {
			t.Errorf("test %q failed: %s", r.Name, r.Message)
		}
	}
	if done.Provenance == nil || done.Provenance.SDKSHA256 == "" || done.Provenance.Toolchain == "" || len(done.Provenance.Dependencies) == 0 ||
		done.Provenance.BuilderIdentity != "e2e-builder" {
		t.Errorf("provenance = %+v", done.Provenance)
	}
	if !strings.Contains(w.declarationOf(t, b.ID), `"bank.transferFee"`) {
		t.Error("the declaration was not stored with the build")
	}

	v, err := w.svc.Deploy(ctx, w.developer, b.ID, "stable")
	if err != nil || !v.TestsPassed {
		t.Fatalf("Deploy: %v %+v", err, v)
	}
	verifyStatement(t, w, backend, v)
}

// verifyStatement checks, as fnrunner does, that the signature covers the
// statement and the statement names the module that is stored (FN-009).
func verifyStatement(t *testing.T, w *world, backend signing.Signer, v function.Version) {
	t.Helper()
	pub, keyID, err := backend.PublicKey(t.Context(), buildclient.DefaultKeyRef)
	if err != nil {
		t.Fatal(err)
	}
	if v.KeyID != keyID || !ed25519.Verify(pub, v.Statement, v.Signature) {
		t.Fatalf("the signature does not verify under the function-artifact key %s", keyID)
	}
	module, _, err := w.store.Get(t.Context(), v.ModuleKey)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		ModuleSHA256 string `json:"moduleSha256"`
		Provenance   struct {
			SDKSHA256 string `json:"sdkSha256"`
		} `json:"provenance"`
	}
	if err := json.Unmarshal(v.Statement, &st); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(module)
	if st.ModuleSHA256 != hex.EncodeToString(sum[:]) || st.Provenance.SDKSHA256 == "" {
		t.Errorf("statement = %s", v.Statement)
	}
}

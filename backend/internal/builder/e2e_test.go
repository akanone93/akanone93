// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
)

// e2eManifest is the standalone manifest of the loan example: the workspace
// form of sdk/fn/examples/loan, with the patch that `cargo vendor` needs while
// plux-fn is unpublished.
const e2eManifest = `[package]
name = "loan"
version = "0.1.0"
edition = "2024"
publish = false

[dependencies]
plux-fn = "0.1"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
schemars = "1.2"
rust_decimal = { version = "1.43", default-features = false, features = ["std", "serde-str", "maths"] }

[patch.crates-io]
plux-fn = { path = %q }
`

// exportChecker is a SchemaReader that checks the schema build exports
// plux_schemas and returns fixed schemas.
type exportChecker struct {
	t       *testing.T
	checked bool
}

func (e *exportChecker) Schemas(ctx context.Context, module []byte, count int) ([]FunctionSchemas, error) {
	e.checked = true
	assertExports(ctx, e.t, module, "plux_schemas")
	out := make([]FunctionSchemas, count)
	for i := range out {
		out[i] = FunctionSchemas{Input: []byte(`{"type":"object"}`), Output: []byte(`{"type":"object"}`)}
	}
	return out, nil
}

// assertExports compiles module with wazero and requires the exports.
func assertExports(ctx context.Context, t *testing.T, module []byte, extra ...string) {
	t.Helper()
	rt := wazero.NewRuntime(ctx)
	defer func() { _ = rt.Close(ctx) }()
	compiled, err := rt.CompileModule(ctx, module)
	if err != nil {
		t.Fatalf("compile module: %v", err)
	}
	funcs := compiled.ExportedFunctions()
	for _, name := range append([]string{"plux_abi_version", "plux_alloc", "plux_invoke"}, extra...) {
		if _, ok := funcs[name]; !ok {
			t.Errorf("module does not export %s", name)
		}
	}
	if _, ok := compiled.ExportedMemories()["memory"]; !ok {
		t.Error("module does not export memory")
	}
}

// runSetup runs a setup command, failing the test with its output.
func runSetup(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...) //nolint:gosec // G204: the test names its own setup commands.
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// TestBuildLoanExampleEndToEnd_FN_019 builds sdk/fn/examples/loan with the real
// toolchain and checks the module, the allowlist and reproducibility.
//
// Verifies: FN-001, FN-003, FN-016, FN-019.
func TestBuildLoanExampleEndToEnd_FN_019(t *testing.T) {
	if os.Getenv("PLUX_RUST_E2E") != "1" {
		t.Skip("set PLUX_RUST_E2E=1 to build with the real Rust toolchain and Binaryen")
	}
	sdk, err := filepath.Abs("../../../sdk/fn/plux-fn")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	proj := filepath.Join(work, "loan")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(proj, "src"), os.DirFS("../../../sdk/fn/examples/loan/src")); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(fmt.Sprintf(e2eManifest, sdk))
	if err := os.WriteFile(filepath.Join(proj, "Cargo.toml"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	runSetup(t, proj, "cargo", "vendor", "--quiet")
	bin := filepath.Join(work, "binaryen")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	runSetup(t, bin, "bun", "add", "--exact", "binaryen@132.0.0")

	reader := &exportChecker{t: t}
	cfg := Config{
		Sandbox: NoSandbox{}, Schemas: reader, Clock: time.Now, Identity: "e2e",
		SDKDir: sdk, WasmOpt: DefaultWasmOpt(bin),
		Limits:      Limits{Wall: 10 * time.Minute, Memory: 16 << 30, OutputBytes: 8 << 20},
		SourceBytes: 256 << 20,
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	src := []FunctionSource{{Name: "loan.calculateSchedule", Placement: PlacementDevice, Files: os.DirFS(proj)}}

	start := time.Now()
	a1, err := b.Build(t.Context(), src, Options{Verify: true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Logf("module %d bytes, build with verify took %s", len(a1.Module), time.Since(start).Round(time.Second))
	if !reader.checked {
		t.Error("schema reader was not called")
	}
	assertExports(t.Context(), t, a1.Module)
	a2, err := b.Build(t.Context(), src, Options{})
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if !bytes.Equal(a1.Module, a2.Module) || a1.Provenance.SourceSHA256 != a2.Provenance.SourceSHA256 {
		t.Error("two builds differ")
	}
	cfg.Sandbox = LinuxSandbox{}
	sandboxed, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a3, err := sandboxed.Build(t.Context(), src, Options{})
	switch {
	case errors.Is(err, ErrSandboxUnavailable):
		t.Logf("sandboxed build skipped: %v", err)
	case err != nil:
		t.Fatalf("sandboxed build: %v", err)
	case !bytes.Equal(a1.Module, a3.Module):
		t.Error("the sandboxed build differs from the unsandboxed one")
	}
	if a1.Provenance.Toolchain == "" || len(a1.Provenance.Dependencies) == 0 {
		t.Errorf("provenance incomplete: %+v", a1.Provenance)
	}
}

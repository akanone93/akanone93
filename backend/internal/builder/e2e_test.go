// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// e2eManifest is the standalone manifest of the loan example: the workspace
// form of sdk/fn/examples/loan, with the patch that Cargo needs while plux-fn is
// unpublished.
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
	runSetup(t, proj, "cargo", "fetch", "--quiet")
	lockData, err := os.ReadFile(filepath.Join(proj, "Cargo.lock"))
	if err != nil {
		t.Fatal(err)
	}
	crates, err := LocalCrates(cargoHome(t), lockData)
	if err != nil {
		t.Fatal(err)
	}
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
	src := []FunctionSource{{Name: "loan.calculateSchedule", Placement: PlacementDevice, Files: os.DirFS(proj), Crates: crates}}

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
	cfg.Sandbox = testSandbox(t)
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
	checkCachedBuilds(t, cfg, src, a1.Module)

	// Build log #159: serde's build script, changed and repacked, with
	// Cargo.lock naming the new archive, is refused before anything compiles.
	tampered, tamperedLock := tamperSerde(t, crates, lockData)
	files := os.DirFS(proj)
	_, err = b.Build(t.Context(), []FunctionSource{{
		Name: "loan.calculateSchedule", Placement: PlacementDevice,
		Files: overlayFile(t, files, "Cargo.lock", tamperedLock), Crates: tampered,
	}}, Options{})
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
}

// checkCachedBuilds proves the build cache of ADR-0107 with the real
// toolchain: a cached build makes the same module as one from scratch, and a
// changed source is compiled again even when every file of the cache claims
// to be newer than it, so a module is never made from an earlier build's
// compilation of another source.
func checkCachedBuilds(t *testing.T, cfg Config, src []FunctionSource, want []byte) {
	t.Helper()
	cache := t.TempDir()
	cfg.CacheDir, cfg.CacheBytes, cfg.CacheTotalBytes = cache, 4<<30, 8<<30
	const key = "org-e2e"
	if _, err := cfg.Sandbox.Run(t.Context(), inScratch(t, Command{Path: "/bin/true", Limits: Limits{Wall: 10 * time.Second}})); errors.Is(err, ErrSandboxUnavailable) {
		cfg.Sandbox = NoSandbox{}
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := b.Build(t.Context(), src, Options{CacheKey: key})
	if err != nil {
		t.Fatalf("cached build: %v", err)
	}
	if !bytes.Equal(cold.Module, want) {
		t.Error("the cached build differs from the build from scratch")
	}
	// A changed source whose cached compilation looks up to date: every
	// file of the cache is dated in the future.
	lib, err := fs.ReadFile(src[0].Files, "src/lib.rs")
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(lib, []byte("Decimal::from(12)"), []byte("Decimal::from(13)"), 1)
	if bytes.Equal(changed, lib) {
		t.Fatal("the example no longer divides by 12")
	}
	future := time.Now().Add(24 * time.Hour)
	if err := filepath.WalkDir(cache, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, future, future)
	}); err != nil {
		t.Fatal(err)
	}
	next := slices.Clone(src)
	next[0].Files = overlayFile(t, src[0].Files, "src/lib.rs", changed)
	start := time.Now()
	warm, err := b.Build(t.Context(), next, Options{CacheKey: key})
	if err != nil {
		t.Fatalf("cached build of the changed source: %v", err)
	}
	t.Logf("cached build of a changed source took %s", time.Since(start).Round(100*time.Millisecond))
	cfg.CacheDir = ""
	scratch, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := scratch.Build(t.Context(), next, Options{})
	if err != nil {
		t.Fatalf("build of the changed source from scratch: %v", err)
	}
	if bytes.Equal(warm.Module, want) || !bytes.Equal(warm.Module, fresh.Module) {
		t.Error("the cached build reused the earlier compilation of the function instead of compiling the changed source")
	}
}

// cargoHome is the Cargo home of the machine running the test.
func cargoHome(t *testing.T) string {
	t.Helper()
	if h := os.Getenv("CARGO_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".cargo")
}

// tamperSerde returns the crates with serde's archive repacked with a line
// added to its build script, and the lock file naming the repacked archive.
func tamperSerde(t *testing.T, crates [][]byte, lockData []byte) ([][]byte, []byte) {
	t.Helper()
	locked, err := LockedCrates(lockData)
	if err != nil {
		t.Fatal(err)
	}
	var serdeSum string
	for _, c := range locked {
		if c.Name == "serde" {
			serdeSum = c.Checksum
		}
	}
	out := make([][]byte, 0, len(crates))
	var repacked []byte
	for _, data := range crates {
		if CrateSHA256(data) != serdeSum {
			out = append(out, data)
			continue
		}
		files := map[string]string{}
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		tr := tar.NewReader(zr)
		top := ""
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			first, rel, _ := strings.Cut(h.Name, "/")
			top, files[rel] = first, string(b)
		}
		files["build.rs"] = strings.Replace(files["build.rs"], "fn main() {", "fn main() {\n    // changed\n", 1)
		repacked = crateArchive(top, files)
		out = append(out, repacked)
	}
	if repacked == nil {
		t.Fatal("serde is not among the crates")
	}
	return out, bytes.Replace(lockData, []byte(serdeSum), []byte(CrateSHA256(repacked)), 1)
}

// overlayFile returns fsys with one file replaced.
func overlayFile(t *testing.T, fsys fs.FS, name string, data []byte) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		out[p] = &fstest.MapFile{Data: b, Mode: 0o644}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out[name] = &fstest.MapFile{Data: data, Mode: 0o644}
	return out
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// fakeCargo stands in for cargo: it answers each subcommand from files next to
// the script, so a test controls what the "toolchain" does without Rust.
const fakeCargo = `#!/bin/sh
d=$(dirname "$0")
cmd=$1
echo "$*" >> "$d/log"
env > "$d/env-$cmd"
pwd > "$d/pwd-$cmd"
if [ -f "$d/fail-$cmd" ]; then
  cat "$d/fail-$cmd" >&2
  exit 101
fi
if [ -f "$d/sleep-$cmd" ]; then sleep 5; fi
case "$cmd" in
generate-lockfile) cp "$d/lock.out" Cargo.lock ;;
metadata) cat "$d/metadata.json" ;;
tree) cat "$d/tree.txt" ;;
build)
  t=""; prev=""
  for a in "$@"; do
    [ "$prev" = "--target-dir" ] && t=$a
    prev=$a
  done
  mkdir -p "$t/wasm32-wasip1/release"
  case " $* " in
  *plux-module/plux-schema*) printf 'schema-module' > "$t/wasm32-wasip1/release/plux_module.wasm" ;;
  *)
    n=0
    [ -f "$d/count" ] && n=$(cat "$d/count")
    n=$((n+1)); echo $n > "$d/count"
    if [ -f "$d/nondet" ]; then printf 'module-%s' "$n" > "$t/wasm32-wasip1/release/plux_module.wasm"
    else printf 'module' > "$t/wasm32-wasip1/release/plux_module.wasm"; fi ;;
  esac ;;
esac
`

// fakeRustc prints the version file.
const fakeRustc = `#!/bin/sh
d=$(dirname "$0")
if [ -f "$d/sleep-version" ]; then sleep "$(cat "$d/sleep-version")"; fi
cat "$d/rustc-version"
`

// fakeWasmOpt answers --version and wraps its input.
const fakeWasmOpt = `#!/bin/sh
d=$(dirname "$0")
echo "wasm-opt $*" >> "$d/log"
if [ "$1" = "--version" ]; then cat "$d/wasmopt-version"; exit 0; fi
if [ -f "$d/fail-wasm-opt" ]; then cat "$d/fail-wasm-opt" >&2; exit 1; fi
printf 'opt(%s)' "$(cat "$1")" > "$3"
`

// fakeReader is a SchemaReader that returns what the test configured.
type fakeReader struct {
	schemas []FunctionSchemas
	err     error
	module  []byte
	count   int
}

func (f *fakeReader) Schemas(_ context.Context, module []byte, count int) ([]FunctionSchemas, error) {
	f.module, f.count = module, count
	if f.err != nil {
		return nil, f.err
	}
	if f.schemas != nil {
		return f.schemas, nil
	}
	out := make([]FunctionSchemas, count)
	for i := range out {
		out[i] = FunctionSchemas{Input: []byte(`{"type":"object","a":1}`), Output: []byte(`{"b":2,"a":1}`)}
	}
	return out, nil
}

// fixture is a Builder wired to the fake tools.
type fixture struct {
	t      *testing.T
	dir    string
	sdk    string
	temp   string
	reader *fakeReader
	cfg    Config
}

// serdeLock is the one registry crate the fixtures depend on.
var serdeLock = lockPackage{Name: "serde", Version: "1.0.229", Source: cratesIOSource, Checksum: "c1"}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, dir: t.TempDir(), sdk: t.TempDir(), temp: t.TempDir(), reader: &fakeReader{}}
	for name, body := range map[string]string{"cargo": fakeCargo, "rustc": fakeRustc, "wasm-opt": fakeWasmOpt} {
		if err := os.WriteFile(filepath.Join(f.dir, name), []byte(body), 0o700); err != nil { //nolint:gosec // G306: the fake tools must be executable.
			t.Fatal(err)
		}
	}
	f.write("rustc-version", "rustc 1.97.0 (2d8144b78 2026-07-07)\n")
	f.write("wasmopt-version", "wasm-opt version 132 (version_132)\n")
	f.write("lock.out", lockText(serdeLock))
	f.write("metadata.json", `{"packages":[{"name":"serde","version":"1.0.229","targets":[{"kind":["custom-build"]}]},`+
		`{"name":"loan","version":"0.1.0","targets":[{"kind":["lib"]}]}]}`)
	f.write("tree.txt", "plux-module v0.1.0 (/x)\nserde v1.0.229\n")
	if err := os.WriteFile(filepath.Join(f.sdk, "Cargo.toml"), []byte("[package]\nname = \"plux-fn\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(f.sdk, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.sdk, "src", "lib.rs"), []byte("// sdk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.cfg = Config{
		Sandbox: NoSandbox{}, Schemas: f.reader, Identity: "builder-1", SDKDir: f.sdk, TempDir: f.temp,
		Cargo: filepath.Join(f.dir, "cargo"), Rustc: filepath.Join(f.dir, "rustc"),
		WasmOpt: []string{filepath.Join(f.dir, "wasm-opt")},
		Clock:   func() time.Time { return time.Date(2026, 10, 13, 9, 30, 0, 0, time.FixedZone("x", 3600)) },
		Limits:  Limits{Wall: 20 * time.Second, OutputBytes: 1 << 20}, SourceBytes: 1 << 20,
		Getenv: func(k string) string {
			return map[string]string{"PATH": "/usr/bin:/bin", "HOME": "/home/dev", "RUSTUP_TOOLCHAIN": "1.97.0", "SECRET": "x"}[k]
		},
	}
	return f
}

// write puts a control file next to the fake tools.
func (f *fixture) write(name, body string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, name), []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// read returns a control or log file, empty when it does not exist.
func (f *fixture) read(name string) string {
	b, _ := os.ReadFile(filepath.Join(f.dir, name))
	return string(b)
}

// builder constructs the Builder, failing the test on a configuration error.
func (f *fixture) builder() *Builder {
	f.t.Helper()
	b, err := New(f.cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

// build runs Build with the sources.
func (f *fixture) build(opts Options, srcs ...FunctionSource) (*Artifact, error) {
	f.t.Helper()
	return f.builder().Build(context.Background(), srcs, opts)
}

// lockText writes a lock file with the packages.
func lockText(pkgs ...lockPackage) string {
	var b strings.Builder
	b.WriteString("version = 4\n")
	for _, p := range pkgs {
		fmt.Fprintf(&b, "\n[[package]]\nname = %q\nversion = %q\n", p.Name, p.Version)
		if p.Source != "" {
			fmt.Fprintf(&b, "source = %q\n", p.Source)
		}
		if p.Checksum != "" {
			fmt.Fprintf(&b, "checksum = %q\n", p.Checksum)
		}
	}
	return b.String()
}

// fnSource returns a function source whose crate depends on deps.
func fnSource(name, pkg string, placement Placement, deps ...lockPackage) FunctionSource {
	fsys := fstest.MapFS{
		"Cargo.toml":      {Data: []byte("[package]\nname = \"" + pkg + "\"\nversion = \"0.1.0\"\n")},
		"src/lib.rs":      {Data: []byte("// " + name + "\n")},
		"Cargo.lock":      {Data: []byte(lockText(append([]lockPackage{{Name: pkg, Version: "0.1.0"}}, deps...)...))},
		"vendor/.gitkeep": {Data: []byte{}},
	}
	for _, d := range deps {
		dir := "vendor/" + d.Name + "/"
		fsys[dir+"Cargo.toml"] = &fstest.MapFile{Data: []byte("[package]\nname = \"" + d.Name + "\"\nversion = \"" + d.Version + "\"\n")}
		fsys[dir+".cargo-checksum.json"] = &fstest.MapFile{Data: []byte(`{"files":{},"package":"` + d.Checksum + `"}`)}
		fsys[dir+"src/lib.rs"] = &fstest.MapFile{Data: []byte("// " + d.Name + "\n")}
	}
	return FunctionSource{Name: name, Placement: placement, Files: fsys}
}

// wantCode fails the test unless err carries the Plux code.
func wantCode(t *testing.T, err error, code plxerr.Code) {
	t.Helper()
	got, ok := plxerr.CodeOf(err)
	if !ok || got != code {
		t.Fatalf("error = %v; want code %s", err, code)
	}
}

// unavailableSandbox always refuses.
type unavailableSandbox struct{}

func (unavailableSandbox) Run(context.Context, Command) (Result, error) {
	return Result{}, fmt.Errorf("%w: no namespaces", ErrSandboxUnavailable)
}

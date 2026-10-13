// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// twoFunctions are listed out of order on purpose: the module sorts them.
func twoFunctions() []FunctionSource {
	return []FunctionSource{
		fnSource("loan.schedule", "loan", PlacementServer, serdeLock),
		fnSource("fee.compute", "fee-calc", PlacementDevice, serdeLock),
	}
}

// Verifies: FN-005, FN-004, FN-001, FN-003.
func TestBuildProducesAnArtifactWithProvenance_FN_005(t *testing.T) {
	f := newFixture(t)
	a, err := f.build(Options{}, twoFunctions()...)
	if err != nil {
		t.Fatal(err)
	}
	if string(a.Module) != "opt(module)" || a.ModuleSHA256 != hexSum(a.Module) || a.ABIVersion != 1 {
		t.Errorf("module = %q hash %s abi %d", a.Module, a.ModuleSHA256, a.ABIVersion)
	}
	if len(a.Functions) != 2 || a.Functions[0].Name != "fee.compute" || a.Functions[0].Index != 0 ||
		a.Functions[0].Placement != PlacementDevice || a.Functions[1].Name != "loan.schedule" || a.Functions[1].Index != 1 {
		t.Errorf("functions = %+v", a.Functions)
	}
	if got := string(a.Functions[0].OutputSchema); got != `{"a":1,"b":2}` {
		t.Errorf("output schema = %s; want canonical JSON", got)
	}
	p := a.Provenance
	if p.Toolchain != "rustc 1.97.0 (2d8144b78 2026-07-07)" || p.WasmOpt != "version_132" ||
		p.BuildTime != "2026-10-13T08:30:00Z" || p.BuilderIdentity != "builder-1" {
		t.Errorf("provenance = %+v", p)
	}
	if len(p.Dependencies) != 1 || p.Dependencies[0] != (Dependency{Name: "serde", Version: "1.0.229", Checksum: "c1"}) {
		t.Errorf("dependencies = %+v", p.Dependencies)
	}
	if len(p.SourceSHA256) != 64 || len(p.SDKSHA256) != 64 {
		t.Errorf("hashes = %q %q", p.SourceSHA256, p.SDKSHA256)
	}
	if string(f.reader.module) != "schema-module" || f.reader.count != 2 {
		t.Errorf("schema reader got %q, %d", f.reader.module, f.reader.count)
	}
}

// Verifies: FN-003, FN-016.
func TestBuildRunsCargoOfflineLockedWithACleanEnvironment_FN_016(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PLUX_TEST_LEAK", "leaked")
	if _, err := f.build(Options{}, twoFunctions()...); err != nil {
		t.Fatal(err)
	}
	log := f.read("log")
	for _, want := range []string{
		"generate-lockfile --offline",
		"metadata --offline --locked --format-version 1",
		"tree --offline --locked -p plux-module --target wasm32-wasip1",
		"build --release --locked --offline --frozen --target wasm32-wasip1 -p plux-module --target-dir",
		"--features plux-module/plux-schema",
		"-Os --strip-debug --strip-producers --enable-bulk-memory",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("tool log lacks %q:\n%s", want, log)
		}
	}
	env := f.read("env-build")
	for _, want := range []string{
		"CARGO_NET_OFFLINE=true", "SOURCE_DATE_EPOCH=0", "PATH=/usr/bin:/bin",
		"RUSTUP_TOOLCHAIN=1.97.0", "RUSTUP_HOME=/home/dev/.rustup", "--remap-path-prefix=", "=/src", "=/cargo", "=/rustup",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("environment lacks %q:\n%s", want, env)
		}
	}
	for _, leak := range []string{"PLUX_TEST_LEAK", "SECRET=", "/home/dev\n"} {
		if strings.Contains(env, leak) {
			t.Errorf("environment leaks %q", leak)
		}
	}
	if entries, _ := os.ReadDir(f.temp); len(entries) != 0 {
		t.Errorf("scratch directories left behind: %d", len(entries))
	}
}

// Verifies: FN-019.
func TestBuildVerifyComparesTwoBuilds_FN_019(t *testing.T) {
	f := newFixture(t)
	if _, err := f.build(Options{Verify: true}, twoFunctions()...); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(f.read("log"), "--target-dir"); n != 3 {
		t.Errorf("cargo builds = %d; want the schema build and two artefact builds", n)
	}
	f.write("nondet", "1")
	_, err := f.build(Options{Verify: true}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildNotReproducible)
	if _, err := f.build(Options{}, twoFunctions()...); err != nil {
		t.Errorf("a build without Verify must not compare: %v", err)
	}
}

// Verifies: FN-003.
func TestBuildRejectsLockDrift_FN_003(t *testing.T) {
	cases := map[string]string{
		"another version":    lockText(lockPackage{Name: "serde", Version: "1.0.230", Source: cratesIOSource, Checksum: "c1"}),
		"another checksum":   lockText(lockPackage{Name: "serde", Version: "1.0.229", Source: cratesIOSource, Checksum: "c2"}),
		"an extra crate":     lockText(serdeLock, lockPackage{Name: "extra", Version: "1.0.0", Source: cratesIOSource, Checksum: "c3"}),
		"a git source":       lockText(lockPackage{Name: "serde", Version: "1.0.229", Source: "git+https://example.invalid/serde", Checksum: "c1"}),
		"an unparsable lock": "not a lock file",
	}
	for name, lock := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.write("lock.out", lock)
			_, err := f.build(Options{}, twoFunctions()...)
			if name == "an unparsable lock" {
				wantCode(t, err, plxerr.FunctionBuildFailed)
				return
			}
			wantCode(t, err, plxerr.FunctionDependencyNotVendored)
		})
	}
}

// Verifies: FN-003.
func TestBuildMapsResolutionFailuresToNotVendored_FN_003(t *testing.T) {
	f := newFixture(t)
	f.write("fail-generate-lockfile", "error: failed to select a version for the requirement `serde = \"^2\"`\n")
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionDependencyNotVendored)

	f = newFixture(t)
	f.write("fail-generate-lockfile", "error: could not parse manifest\n")
	_, err = f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildFailed)
}

// Verifies: FN-016.
func TestBuildRefusesCodeThatIsNotOnTheAllowlist_FN_016(t *testing.T) {
	f := newFixture(t)
	f.write("metadata.json", `{"packages":[{"name":"evil-build","version":"0.1.0","targets":[{"kind":["custom-build"]}]}]}`)
	f.write("tree.txt", "plux-module v0.1.0\nevil-build v0.1.0\n")
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
	if !strings.Contains(err.Error(), "evil-build 0.1.0") {
		t.Errorf("error does not name the crate: %v", err)
	}
	if strings.Contains(f.read("log"), "--target-dir") {
		t.Error("the build ran after the allowlist refused it")
	}
}

// Verifies: FN-001.
func TestBuildChecksThePinnedTools_FN_001(t *testing.T) {
	f := newFixture(t)
	f.write("rustc-version", "rustc 1.96.0 (abc 2026-01-01)\n")
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildFailed)
	if !strings.Contains(err.Error(), "1.96.0") {
		t.Errorf("error does not show the toolchain found: %v", err)
	}

	f = newFixture(t)
	f.write("wasmopt-version", "wasm-opt version 131 (version_131)\n")
	_, err = f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildFailed)

	f = newFixture(t)
	f.write("rustc-version", "rustc 1.97.01 (abc)\n")
	_, err = f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildFailed)
}

// Verifies: FN-016.
func TestBuildReportsLimitBreachesAsPLX7203_FN_016(t *testing.T) {
	t.Run("wall time", func(t *testing.T) {
		f := newFixture(t)
		f.cfg.Limits.Wall = 300 * time.Millisecond
		f.write("sleep-build", "1")
		_, err := f.build(Options{}, twoFunctions()...)
		wantCode(t, err, plxerr.FunctionBuildLimitExceeded)
	})
	t.Run("output", func(t *testing.T) {
		f := newFixture(t)
		f.cfg.Limits.OutputBytes = 64
		f.write("tree.txt", strings.Repeat("serde v1.0.229\n", 100))
		_, err := f.build(Options{}, twoFunctions()...)
		wantCode(t, err, plxerr.FunctionBuildLimitExceeded)
	})
	t.Run("memory", func(t *testing.T) {
		f := newFixture(t)
		f.write("fail-build", "memory allocation of 4294967296 bytes failed\n")
		_, err := f.build(Options{}, twoFunctions()...)
		wantCode(t, err, plxerr.FunctionBuildLimitExceeded)
	})
	t.Run("sources", func(t *testing.T) {
		f := newFixture(t)
		f.cfg.SourceBytes = 10
		_, err := f.build(Options{}, twoFunctions()...)
		wantCode(t, err, plxerr.FunctionBuildLimitExceeded)
	})
}

func TestFailureClassification(t *testing.T) {
	f := newFixture(t)
	r, cleanup, err := f.builder().newRun()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cases := []struct {
		name string
		res  Result
		code plxerr.Code
	}{
		{"cpu signal", Result{Signal: sigXCPU, ExitCode: -1}, plxerr.FunctionBuildLimitExceeded},
		{"timeout", Result{TimedOut: true}, plxerr.FunctionBuildLimitExceeded},
		{"plain failure", Result{ExitCode: 1, Stderr: []byte("error[E0308]: mismatched types")}, plxerr.FunctionBuildFailed},
		{"kill without limits", Result{Signal: sigKill, ExitCode: -1}, plxerr.FunctionBuildFailed},
	}
	for _, c := range cases {
		wantCode(t, r.failure(c.res, "step"), c.code)
	}
	r.b.cfg.Limits.CPUTime = time.Second
	wantCode(t, r.failure(Result{Signal: sigKill, ExitCode: -1}, "step"), plxerr.FunctionBuildLimitExceeded)
}

// Verifies: FN-016.
func TestBuildReturnsCompilerDiagnosticsWithRemappedPaths_FN_016(t *testing.T) {
	f := newFixture(t)
	f.write("fail-build", "error: boom at WORKSPACE/functions/loan/src/lib.rs\n")
	// The fake cannot know its workspace; make the script print it.
	script := strings.Replace(fakeCargo, `cat "$d/fail-$cmd" >&2`, `sed "s|WORKSPACE|$PWD|" "$d/fail-$cmd" >&2`, 1)
	if err := os.WriteFile(f.cfg.Cargo, []byte(script), 0o700); err != nil { //nolint:gosec // G306: the fake tool must be executable.
		t.Fatal(err)
	}
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildFailed)
	msg := err.Error()
	if !strings.Contains(msg, "/src/functions/loan/src/lib.rs") || strings.Contains(msg, f.temp) {
		t.Errorf("diagnostics not remapped: %s", msg)
	}
}

// Verifies: FN-016.
func TestDiagnosticsAreBoundedTo64KiB_FN_016(t *testing.T) {
	f := newFixture(t)
	r, cleanup, err := f.builder().newRun()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	big := strings.Repeat("é", 100*1024)
	got := r.diagnostics(Result{Stderr: []byte(big)})
	if len(got) > 64*1024 || len(got) < 64*1024-4 {
		t.Errorf("len = %d", len(got))
	}
	if r.diagnostics(Result{Stdout: []byte("from stdout")}) != "from stdout" {
		t.Error("stdout is the fallback when stderr is empty")
	}
}

// Verifies: FN-008.
func TestBuildReportsPlacementMismatchAsPLX7205_FN_008(t *testing.T) {
	f := newFixture(t)
	f.write("fail-build", "error: evaluation panicked: PLX-7205: the placement of `loan` differs from the one its function document declares\n")
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionDeclarationMismatch)
}

// Verifies: FN-004.
func TestBuildReportsSchemaFailuresAsPLX7204_FN_004(t *testing.T) {
	f := newFixture(t)
	f.reader.err = errors.New("module trapped")
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)

	f = newFixture(t)
	f.reader.schemas = []FunctionSchemas{{Input: []byte(`{}`), Output: []byte(`{}`)}}
	_, err = f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)

	f = newFixture(t)
	f.reader.schemas = []FunctionSchemas{{Input: []byte(`{`), Output: []byte(`{}`)}, {Input: []byte(`{}`), Output: []byte(`{}`)}}
	_, err = f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)

	f = newFixture(t)
	f.reader.schemas = []FunctionSchemas{{Input: []byte(`{}`), Output: []byte(`{`)}, {Input: []byte(`{}`), Output: []byte(`{}`)}}
	_, err = f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionSchemaDerivationFailed)
}

// Verifies: FN-016.
func TestBuildNeverFallsBackFromAnUnavailableSandbox_FN_016(t *testing.T) {
	f := newFixture(t)
	f.cfg.Sandbox = unavailableSandbox{}
	_, err := f.build(Options{}, twoFunctions()...)
	if !errors.Is(err, ErrSandboxUnavailable) {
		t.Fatalf("error = %v; want ErrSandboxUnavailable", err)
	}
	if f.read("log") != "" {
		t.Error("a tool ran without the sandbox")
	}
}

func TestBuildReportsWasmOptFailure(t *testing.T) {
	f := newFixture(t)
	f.write("fail-wasm-opt", "validation error\n")
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildFailed)
}

func TestBuildHonoursContextCancellation(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.builder().Build(ctx, twoFunctions(), Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v; want context.Canceled", err)
	}
}

func TestNewValidatesTheConfiguration(t *testing.T) {
	mutate := map[string]func(*Config){
		"sandbox":   func(c *Config) { c.Sandbox = nil },
		"schemas":   func(c *Config) { c.Schemas = nil },
		"clock":     func(c *Config) { c.Clock = nil },
		"sdk":       func(c *Config) { c.SDKDir = "" },
		"wasm-opt":  func(c *Config) { c.WasmOpt = nil },
		"limits":    func(c *Config) { c.Limits.Wall = 0 },
		"sources":   func(c *Config) { c.SourceBytes = 0 },
		"cargo":     func(c *Config) { c.Cargo = "/nonexistent/cargo" },
		"rustc":     func(c *Config) { c.Rustc = "/nonexistent/rustc" },
		"optimiser": func(c *Config) { c.WasmOpt = []string{"/nonexistent/wasm-opt"} },
	}
	for name, m := range mutate {
		f := newFixture(t)
		m(&f.cfg)
		if _, err := New(f.cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	f := newFixture(t)
	f.cfg.Getenv = nil
	f.cfg.Allowlist = Allowlist{}
	if _, err := New(f.cfg); err != nil {
		t.Errorf("defaults: %v", err)
	}
}

func TestBuildRejectsAScratchPathWithWhiteSpace(t *testing.T) {
	f := newFixture(t)
	f.cfg.TempDir = t.TempDir() + "/with space"
	if err := os.MkdirAll(f.cfg.TempDir, 0o750); err != nil {
		t.Fatal(err)
	}
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildFailed)
}

func TestBuildFailsWithoutAScratchDirectory(t *testing.T) {
	f := newFixture(t)
	f.cfg.TempDir = "/nonexistent/dir"
	if _, err := f.build(Options{}, twoFunctions()...); err == nil {
		t.Error("expected an error")
	}
}

func TestBuildFailsWithAnEmptySDK(t *testing.T) {
	f := newFixture(t)
	f.cfg.SDKDir = t.TempDir()
	_, err := f.build(Options{}, twoFunctions()...)
	wantCode(t, err, plxerr.FunctionBuildFailed)
}

// Verifies: FN-003.
func TestPrepareSourcesRejectsMalformedInput_FN_003(t *testing.T) {
	ok := fnSource("a.b", "a-b", PlacementDevice, serdeLock)
	withFile := func(name, body string) FunctionSource {
		s := fnSource("a.b", "a-b", PlacementDevice, serdeLock)
		s.Files.(fstest.MapFS)[name] = &fstest.MapFile{Data: []byte(body)}
		return s
	}
	without := func(name string) FunctionSource {
		s := fnSource("a.b", "a-b", PlacementDevice, serdeLock)
		delete(s.Files.(fstest.MapFS), name)
		return s
	}
	cases := map[string]struct {
		src  []FunctionSource
		code plxerr.Code
	}{
		"no functions":      {nil, plxerr.FunctionBuildFailed},
		"bad name":          {[]FunctionSource{{Name: "../x", Placement: PlacementDevice, Files: ok.Files}}, plxerr.FunctionBuildFailed},
		"bad placement":     {[]FunctionSource{{Name: "x", Placement: "cloud", Files: ok.Files}}, plxerr.FunctionBuildFailed},
		"no files":          {[]FunctionSource{{Name: "x", Placement: PlacementDevice}}, plxerr.FunctionBuildFailed},
		"duplicate name":    {[]FunctionSource{ok, ok}, plxerr.FunctionBuildFailed},
		"duplicate crate":   {[]FunctionSource{ok, {Name: "other", Placement: PlacementServer, Files: ok.Files}}, plxerr.FunctionBuildFailed},
		"no manifest":       {[]FunctionSource{without("Cargo.toml")}, plxerr.FunctionBuildFailed},
		"workspace table":   {[]FunctionSource{withFile("Cargo.toml", "[package]\nname = \"a\"\n[workspace]\n")}, plxerr.FunctionBuildFailed},
		"no package name":   {[]FunctionSource{withFile("Cargo.toml", "[dependencies]\n")}, plxerr.FunctionBuildFailed},
		"no lock":           {[]FunctionSource{without("Cargo.lock")}, plxerr.FunctionDependencyNotVendored},
		"empty lock":        {[]FunctionSource{withFile("Cargo.lock", "")}, plxerr.FunctionDependencyNotVendored},
		"git in lock":       {[]FunctionSource{withFile("Cargo.lock", lockText(lockPackage{Name: "x", Version: "1", Source: "git+https://x", Checksum: "c"}))}, plxerr.FunctionDependencyNotVendored},
		"no vendor":         {[]FunctionSource{withoutPrefix(ok, "vendor/")}, plxerr.FunctionDependencyNotVendored},
		"vendor no toml":    {[]FunctionSource{without("vendor/serde/Cargo.toml")}, plxerr.FunctionDependencyNotVendored},
		"vendor no version": {[]FunctionSource{withFile("vendor/serde/Cargo.toml", "[package]\nname = \"serde\"\n")}, plxerr.FunctionDependencyNotVendored},
		"vendor no sum":     {[]FunctionSource{without("vendor/serde/.cargo-checksum.json")}, plxerr.FunctionDependencyNotVendored},
		"vendor bad sum":    {[]FunctionSource{withFile("vendor/serde/.cargo-checksum.json", "{")}, plxerr.FunctionDependencyNotVendored},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := prepareSources(c.src)
			wantCode(t, err, c.code)
		})
	}
}

// withoutPrefix drops every file below a directory.
func withoutPrefix(s FunctionSource, prefix string) FunctionSource {
	out := fstest.MapFS{}
	for k, v := range s.Files.(fstest.MapFS) {
		if !strings.HasPrefix(k, prefix) {
			out[k] = v
		}
	}
	s.Files = out
	return s
}

func TestBuildRefusesSymlinksInSources(t *testing.T) {
	f := newFixture(t)
	s := fnSource("a.b", "a-b", PlacementDevice, serdeLock)
	s.Files.(fstest.MapFS)["src/link.rs"] = &fstest.MapFile{Data: []byte("/etc/passwd"), Mode: 0o777 | os.ModeSymlink}
	_, err := f.build(Options{}, s)
	wantCode(t, err, plxerr.FunctionBuildFailed)
}

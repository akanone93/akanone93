// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnhost"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// Environment variables and directories of the function commands.
const (
	sdkEnv     = "PLUX_SDK"
	wasmOptEnv = "PLUX_WASM_OPT"
	// binaryenPackage is the pinned Binaryen release, installed with Bun into
	// the user's cache when no wasm-opt is configured (plan P7 E10).
	binaryenPackage = "binaryen@132.0.0"
	// outputDir is where `plux fn build` writes the module, the schemas and
	// the provenance, relative to the function's crate.
	outputDir = "target/plux"
	// moduleName is the built module in outputDir.
	moduleName = "module.wasm"
)

// The wall time, output and source bounds of a development build; the server
// applies the registry's function.build* limits instead (LIM-001).
const (
	devBuildWall    = 15 * time.Minute
	devBuildMemory  = 16 << 30
	devBuildOutput  = 8 << 20
	devSourceBudget = 512 << 20
)

// functionNameRE is the dotted function name of the function document
// (FN-006).
var functionNameRE = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*(\.[a-z][a-zA-Z0-9]*)+$`)

// checkFunctionName refuses a name the function document would refuse.
func checkFunctionName(name string) error {
	if len(name) > 128 || !functionNameRE.MatchString(name) {
		return usageError(fmt.Sprintf("%q is not a function name: use dotted lower-camel-case words such as bank.transferFee", name))
	}
	return nil
}

// crateDir is where a function lives in a project.
func crateDir(project, name string) string { return filepath.Join(project, "functions", name) }

// resolveSDK finds the pinned SDK crate `plux-fn`: the flag, $PLUX_SDK, then
// the copy shipped next to the binary.
func resolveSDK(flagValue string) (string, error) {
	candidates := []string{flagValue, os.Getenv(sdkEnv)}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "sdk", "fn", "plux-fn"), filepath.Join(dir, "..", "share", "plux", "sdk", "fn", "plux-fn"))
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "Cargo.toml")); err == nil { //nolint:gosec // G703: the developer names the SDK directory.
			return abs, nil
		}
	}
	return "", usageError("no SDK: pass --sdk, or set " + sdkEnv + " to the directory of the plux-fn crate (sdk/fn/plux-fn)")
}

// resolveWasmOpt returns the command prefix that runs the pinned wasm-opt:
// $PLUX_WASM_OPT, else Binaryen installed with Bun in the user's cache.
func resolveWasmOpt(ctx context.Context, stderr io.Writer) ([]string, error) {
	if v := strings.Fields(os.Getenv(wasmOptEnv)); len(v) > 0 {
		return v, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("find the cache directory: %w", err)
	}
	dir := filepath.Join(cache, "plux", "binaryen")
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "binaryen", "bin", "wasm-opt")); err != nil {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
		_, _ = fmt.Fprintf(stderr, "installing %s into %s\n", binaryenPackage, dir)
		if err := runTool(ctx, dir, io.Discard, stderr, "bun", "add", "--exact", binaryenPackage); err != nil {
			return nil, fmt.Errorf("install Binaryen (needs Bun; or set %s to a wasm-opt command): %w", wasmOptEnv, err)
		}
	}
	return builder.DefaultWasmOpt(dir), nil
}

// runTool runs a developer tool in dir.
func runTool(ctx context.Context, dir string, stdout, stderr io.Writer, tool string, args ...string) error {
	return runToolEnv(ctx, dir, nil, stdout, stderr, tool, args...)
}

// runToolEnv runs a developer tool in dir with extra environment variables
// added to the CLI's own.
func runToolEnv(ctx context.Context, dir string, extra []string, stdout, stderr io.Writer, tool string, args ...string) error {
	cmd := exec.CommandContext(ctx, tool, args...) //nolint:gosec // G204: the developer's own toolchain.
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, stdout, stderr
	if len(extra) > 0 {
		cmd.Env = append(os.Environ(), extra...)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", tool, strings.Join(args, " "), err)
	}
	return nil
}

// sharedTargetDir is the Cargo build directory that every function of a
// project shares, so the native tests compile each dependency once per
// project rather than once per function. It is a dot-directory, which the
// project layout, plux push and plux dev all skip: every other directory
// under functions/ is a function.
func sharedTargetDir(project string) string { return filepath.Join(project, "functions", ".target") }

// cargoTargetEnv points Cargo at the project's shared build directory and
// makes sure version control ignores it. A CARGO_TARGET_DIR the developer
// set is kept.
func cargoTargetEnv(project string) ([]string, error) {
	if os.Getenv("CARGO_TARGET_DIR") != "" {
		return nil, nil
	}
	dir, err := filepath.Abs(sharedTargetDir(project))
	if err != nil {
		return nil, fmt.Errorf("find the shared build directory: %w", err)
	}
	if err := ignoreSharedTarget(project); err != nil {
		return nil, err
	}
	return []string{"CARGO_TARGET_DIR=" + dir}, nil
}

// targetIgnoreLine is the functions/.gitignore line for the shared build
// directory.
const targetIgnoreLine = "/.target"

// ignoreSharedTarget adds the shared build directory to functions/.gitignore,
// creating the file when there is none.
func ignoreSharedTarget(project string) error {
	p := filepath.Join(project, "functions", ".gitignore")
	data, err := os.ReadFile(p) //nolint:gosec // G304: the developer's own project.
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", p, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if l := strings.TrimSpace(line); l == targetIgnoreLine || l == targetIgnoreLine+"/" {
			return nil
		}
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	data = append(data, targetIgnoreLine+"\n"...)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil { //nolint:gosec // G703: the project directory the developer named.
		return fmt.Errorf("write %s: %w", p, err)
	}
	return nil
}

// sourceEntries are the parts of a crate the server builds from (FN-016); the
// dependencies travel as their published archives (crateSource).
var sourceEntries = []string{"Cargo.toml", "Cargo.lock", builder.DeclarationFile, "src"}

// collectSource reads the files of a crate that make up its source archive:
// the manifest, the lock file, src/ and function.json. A vendor/ directory
// in the crate is never read: the builder makes vendor/ itself.
func collectSource(dir string) (fstest.MapFS, error) {
	files := fstest.MapFS{}
	for _, entry := range sourceEntries {
		root := filepath.Join(dir, entry)
		info, err := os.Lstat(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", root, err)
		}
		if err := collectEntry(files, dir, root, info); err != nil {
			return nil, fmt.Errorf("collect %s: %w", entry, err)
		}
	}
	for _, need := range []string{"Cargo.toml", builder.DeclarationFile} {
		if _, ok := files[need]; !ok {
			return nil, usageError(fmt.Sprintf("%s has no %s: run plux fn new, or pass -C with the project directory", dir, need))
		}
	}
	return files, nil
}

// collectEntry adds a file, or every file under a directory, to files.
func collectEntry(files fstest.MapFS, dir, root string, info fs.FileInfo) error {
	add := func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file; a source holds only files and directories", p)
		}
		data, err := os.ReadFile(p) //nolint:gosec // G304: the developer's own crate.
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
		files[filepath.ToSlash(rel)] = &fstest.MapFile{Data: data, Mode: 0o644}
		return nil
	}
	if !info.IsDir() {
		return add(root, fs.FileInfoToDirEntry(info), nil)
	}
	return filepath.WalkDir(root, add) //nolint:wrapcheck // wrapped by the caller
}

// packArchive writes the gzip-compressed tar archive of a source. The entries
// are sorted, with fixed times and modes, so the same source gives the same
// archive and the same SHA-256 (FN-019).
func packArchive(files fstest.MapFS) ([]byte, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("pack the source: %w", err)
	}
	tw := tar.NewWriter(zw)
	for _, n := range names {
		data := files[n].Data
		if err := tw.WriteHeader(&tar.Header{Name: path.Clean(n), Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
			return nil, fmt.Errorf("pack %s: %w", n, err)
		}
		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("pack %s: %w", n, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("pack the source: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("pack the source: %w", err)
	}
	return buf.Bytes(), nil
}

// fetchCrate runs `cargo fetch` in the crate dir, so that Cargo.lock exists
// and Cargo's cache holds the archive of every dependency it names (FN-003).
func (e env) fetchCrate(ctx context.Context, dir string) error {
	if e.fetch != nil {
		return e.fetch(ctx, dir)
	}
	var errOut bytes.Buffer
	if err := runTool(ctx, dir, io.Discard, &errOut, "cargo", "fetch", "--quiet"); err != nil {
		_, _ = e.stderr.Write(errOut.Bytes())
		return err
	}
	return nil
}

// cargoHomeDir is Cargo's home on this machine: $CARGO_HOME, or ~/.cargo.
func (e env) cargoHomeDir() (string, error) {
	if e.cargoHome != "" {
		return e.cargoHome, nil
	}
	if h := os.Getenv("CARGO_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find Cargo's home: %w", err)
	}
	return filepath.Join(home, ".cargo"), nil
}

// crateSource reads the source of the crate in dir and the published crate
// archives of the dependencies its Cargo.lock names, from Cargo's cache,
// which every function and project on the machine shares, so no function
// keeps a copy of them (FN-003, ADR-0105). Each archive is the one whose
// SHA-256 Cargo.lock records; the builder checks them again.
func (e env) crateSource(ctx context.Context, dir string) (fstest.MapFS, [][]byte, error) {
	if err := e.fetchCrate(ctx, dir); err != nil {
		return nil, nil, err
	}
	files, err := collectSource(dir)
	if err != nil {
		return nil, nil, err
	}
	lock, ok := files["Cargo.lock"]
	if !ok {
		return nil, nil, fmt.Errorf("%s has no Cargo.lock after cargo fetch", dir)
	}
	home, err := e.cargoHomeDir()
	if err != nil {
		return nil, nil, err
	}
	crates, err := builder.LocalCrates(home, lock.Data)
	if err != nil {
		return nil, nil, err //nolint:wrapcheck // a Plux error that says what to do
	}
	e.dropOldVendor(dir)
	return files, crates, nil
}

// dropOldVendor removes the vendor/ directory that earlier versions of plux
// fn wrote into each crate and that `cargo vendor` rewrote on every run; the
// builder now makes vendor/ itself from the published archives. It is removed
// only when it holds nothing but crates written by `cargo vendor` (each with
// a .cargo-checksum.json); anything else is left alone.
func (e env) dropOldVendor(dir string) {
	vendor := filepath.Join(dir, "vendor")
	entries, err := os.ReadDir(vendor)
	if err != nil || len(entries) == 0 {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return
		}
		info, err := os.Lstat(filepath.Join(vendor, entry.Name(), ".cargo-checksum.json"))
		if err != nil || !info.Mode().IsRegular() {
			return
		}
	}
	if err := os.RemoveAll(vendor); err != nil {
		_, _ = fmt.Fprintf(e.stderr, "could not remove %s, which plux fn no longer uses: %v\n", vendor, err)
		return
	}
	_, _ = fmt.Fprintf(e.stderr, "removed %s: the dependencies are now sent as their published archives from Cargo's cache, not copied into each function\n", vendor)
}

// builtFunction is the result of a development build.
type builtFunction struct {
	Dir      string
	Decl     *builder.Declaration
	Artifact *builder.Artifact
}

// readDeclaration validates function.json against the function schema.
func readDeclaration(files fstest.MapFS, name string, set limits.Set) (*builder.Declaration, plxerr.Diagnostics, error) {
	v, err := schema.NewValidator()
	if err != nil {
		return nil, nil, fmt.Errorf("load the schemas: %w", err)
	}
	decl, diags := builder.ReadDeclaration(schema.NewLoader(v, schema.DefaultMigrator(), set), files, name, "")
	return decl, diags, nil
}

// newEngine returns a function host; with interpreter it runs modules in
// wazero's interpreter, the CLI's stand-in for the device engine (FN-050).
func newEngine(ctx context.Context, set limits.Set, interpreter bool) (*fnhost.Engine, error) {
	return fnhost.NewEngine(ctx, fnhost.Config{ //nolint:wrapcheck // reported as is
		Clock: time.Now, Random: rand.Reader, MaxMemoryBytes: uint64(set.Get(limits.FunctionMemory)), //nolint:gosec // G115: registry values are non-negative.
		Interpreter: interpreter,
	})
}

// localBuilder sets up the unsandboxed builder of the development commands:
// the SDK, the optimiser and the host that reads the schemas. closeFn
// releases the host.
func (e env) localBuilder(ctx context.Context, sdkFlag string, set limits.Set) (b *builder.Builder, closeFn func(), err error) {
	sdk, err := resolveSDK(sdkFlag)
	if err != nil {
		return nil, nil, err
	}
	wasmOpt, err := resolveWasmOpt(ctx, e.stderr)
	if err != nil {
		return nil, nil, err
	}
	engine, err := newEngine(ctx, set, false)
	if err != nil {
		return nil, nil, err
	}
	b, err = builder.New(builder.Config{
		Sandbox: builder.NoSandbox{}, Clock: time.Now, Identity: "plux-cli", SDKDir: sdk, WasmOpt: wasmOpt,
		Schemas: builder.NewWasmSchemaReader(engine, fnhost.Limits{
			Timeout: time.Duration(set.Get(limits.FunctionTimeout)) * time.Millisecond, MemoryBytes: uint64(set.Get(limits.FunctionMemory)), //nolint:gosec // G115: registry values are non-negative.
			OutputBytes: int(set.Get(limits.FunctionOutputSize)), LogBytes: int(set.Get(limits.FunctionLogBytes)),
		}),
		Limits: builder.Limits{Wall: devBuildWall, Memory: devBuildMemory, OutputBytes: devBuildOutput}, SourceBytes: devSourceBudget,
	})
	if err != nil {
		_ = engine.Close(context.WithoutCancel(ctx))
		return nil, nil, fmt.Errorf("set up the build: %w", err)
	}
	return b, func() { _ = engine.Close(context.WithoutCancel(ctx)) }, nil
}

// readCrate reads a function's source, the published archives of its
// dependencies and its validated declaration, as one builder source.
func (e env) readCrate(ctx context.Context, project, name string, set limits.Set) (builder.FunctionSource, *builder.Declaration, error) {
	if err := checkFunctionName(name); err != nil {
		return builder.FunctionSource{}, nil, err
	}
	files, crates, err := e.crateSource(ctx, crateDir(project, name))
	if err != nil {
		return builder.FunctionSource{}, nil, err
	}
	decl, diags, err := readDeclaration(files, name, set)
	if err != nil {
		return builder.FunctionSource{}, nil, err
	}
	if decl == nil {
		for _, d := range diags {
			_, _ = fmt.Fprintln(e.stderr, d.String())
		}
		return builder.FunctionSource{}, nil, errors.New("function.json is invalid")
	}
	src := decl.Source(files)
	src.Crates = crates
	return src, decl, nil
}

// buildFunction fetches, validates and builds a function without a sandbox,
// and writes the module, its schemas and the provenance to target/plux/.
// It is for development only: the server rebuilds in its sandbox (FN-016).
func (e env) buildFunction(ctx context.Context, project, name, sdkFlag string, verify bool) (*builtFunction, error) {
	if err := checkFunctionName(name); err != nil {
		return nil, err
	}
	set := limits.Defaults()
	b, closeBuilder, err := e.localBuilder(ctx, sdkFlag, set)
	if err != nil {
		return nil, err
	}
	defer closeBuilder()
	src, decl, err := e.readCrate(ctx, project, name, set)
	if err != nil {
		return nil, err
	}
	artifact, err := b.Build(ctx, []builder.FunctionSource{src}, builder.Options{Verify: verify})
	if err != nil {
		return nil, fmt.Errorf("build %s: %w", name, err)
	}
	out := builtDir(project, name)
	if err := writeBuildOutput(out, artifact); err != nil {
		return nil, err
	}
	return &builtFunction{Dir: out, Decl: decl, Artifact: artifact}, nil
}

// buildProvenance is provenance.json.
type buildProvenance struct {
	ModuleSHA256    string               `json:"moduleSha256"`
	SourceSHA256    string               `json:"sourceSha256"`
	SDKSHA256       string               `json:"sdkSha256"`
	Toolchain       string               `json:"toolchain"`
	WasmOptVersion  string               `json:"wasmOptVersion"`
	BuildTime       string               `json:"buildTime"`
	BuilderIdentity string               `json:"builderIdentity"`
	Sandboxed       bool                 `json:"sandboxed"`
	Dependencies    []builder.Dependency `json:"dependencies"`
}

// writeBuildOutput writes the module, the schemas and the provenance.
func writeBuildOutput(dir string, a *builder.Artifact) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	p := a.Provenance
	prov, err := json.MarshalIndent(buildProvenance{
		ModuleSHA256: a.ModuleSHA256, SourceSHA256: p.SourceSHA256, SDKSHA256: p.SDKSHA256, Toolchain: p.Toolchain,
		WasmOptVersion: p.WasmOpt, BuildTime: p.BuildTime, BuilderIdentity: p.BuilderIdentity, Dependencies: p.Dependencies,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the provenance: %w", err)
	}
	outputs := map[string][]byte{moduleName: a.Module, "provenance.json": append(prov, '\n')}
	if len(a.Functions) == 1 {
		outputs[inputSchemaFile], outputs[outputSchemaFile] = a.Functions[0].InputSchema, a.Functions[0].OutputSchema
	}
	for n, data := range outputs {
		if err := os.WriteFile(filepath.Join(dir, n), data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", n, err)
		}
	}
	return nil
}

// loadBuilt reads a built module and the declaration beside it.
func loadBuilt(project, name string) (module []byte, decl *builder.Declaration, err error) {
	if err := checkFunctionName(name); err != nil {
		return nil, nil, err
	}
	dir := crateDir(project, name)
	module, err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(outputDir), moduleName)) //nolint:gosec // G304: the developer's own build output.
	if err != nil {
		return nil, nil, usageError("no built module for " + name + ": run plux fn build " + name)
	}
	files, err := collectSource(dir)
	if err != nil {
		return nil, nil, err
	}
	d, diags, err := readDeclaration(files, name, limits.Defaults())
	if err != nil {
		return nil, nil, err
	}
	if d == nil {
		return nil, nil, fmt.Errorf("function.json is invalid: %s", diags[0].String())
	}
	return module, d, nil
}

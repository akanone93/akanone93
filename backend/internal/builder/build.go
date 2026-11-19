// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/jcs"
)

const (
	// wasmTarget is the compilation target of every function (FN-001).
	wasmTarget = "wasm32-wasip1"
	// moduleFile is the file name cargo gives the wrapper's cdylib.
	moduleFile = "plux_module.wasm"
	// diagnosticBytes bounds the compiler output returned in PLX-7200 (E7).
	diagnosticBytes = 64 * 1024
	// metadataOutputFactor scales Limits.OutputBytes for `cargo metadata`,
	// whose JSON is large by nature.
	metadataOutputFactor = 16
	// schemaDepth bounds the nesting of a derived JSON Schema.
	schemaDepth = 128
	// sigXCPU is the signal a process receives at its CPU-time limit.
	sigXCPU = 24
	// sigKill is the signal the hard CPU limit and the kernel's memory killer
	// end a process with; a sandbox's first process ignores SIGXCPU.
	sigKill = 9
)

// wasmOptFeatures are the WebAssembly features the Rust toolchain emits for
// wasm32-wasip1 and that wasm-opt must accept; the module's target-features
// section is stripped before the optimiser sees it.
var wasmOptFeatures = []string{
	"--enable-bulk-memory", "--enable-bulk-memory-opt", "--enable-sign-ext",
	"--enable-nontrapping-float-to-int", "--enable-mutable-globals",
}

// Config configures a Builder.
type Config struct {
	// Sandbox runs every child process; use [LinuxSandbox] on the server and
	// [NoSandbox] only where the caller has chosen to build unsandboxed.
	Sandbox Sandbox
	// Schemas reads the schemas out of the schema build (FN-004).
	Schemas SchemaReader
	// Clock supplies the build time of the provenance.
	Clock func() time.Time
	// Identity names this builder in the provenance.
	Identity string
	// SDKDir is the pinned SDK crate `plux-fn` (its Cargo.toml and src/).
	SDKDir string
	// TempDir is the parent of the scratch directories; empty means the
	// system's temporary directory.
	TempDir string
	// Cargo and Rustc are the tools; empty means "cargo" and "rustc" on PATH.
	Cargo, Rustc string
	// WasmOpt is the command prefix that runs the pinned wasm-opt, for example
	// [DefaultWasmOpt].
	WasmOpt []string
	// Allowlist replaces the embedded allowlist of build scripts and
	// procedural macros; nil means the embedded one.
	Allowlist Allowlist
	// Limits bound every child process; they come from the limits registry
	// (function.buildTimeout, function.buildMemory; LIM-001).
	Limits Limits
	// SourceBytes bounds the sources of one build (function.sourceArchiveSize).
	SourceBytes int64
	// CacheDir, when set, keeps the workspace, Cargo home and target
	// directory of builds there, at paths that do not change, so a build
	// reuses the crates.io dependencies an earlier one compiled; empty
	// builds each time from scratch. A sandboxed builder keeps one cache per
	// Options.CacheKey below CacheDir, which only that key's builds can see
	// or change (ADR-0106, ADR-0107); an unsandboxed development build with
	// no key uses CacheDir itself. Builds take a cache in turn; one that
	// finds it in use builds from scratch. The crates of the build's own
	// workspace — the functions, the module wrapper and the SDK — are
	// compiled again in every build, so a module is always compiled from its
	// own sources, and it is the same with or without the cache (FN-019);
	// Options.Verify builds its second copy from scratch to prove it.
	CacheDir string
	// CacheBytes bounds one cache: after a build that leaves its target
	// directory larger, the target directory is emptied
	// (function.buildCacheSize on a server, function.devBuildCacheSize on a
	// developer's machine).
	CacheBytes int64
	// CacheTotalBytes bounds the keyed caches together: after a build, the
	// caches used least recently are removed until the rest fit. A
	// sandboxed builder with a cache needs it.
	CacheTotalBytes int64
	// Getenv reads the builder's own environment; nil means os.Getenv.
	Getenv func(string) string
}

// Builder builds modules from Rust function sources. It is safe for concurrent
// use; every Build call works in its own scratch directory.
type Builder struct {
	cfg Config
	sdk fs.FS
	// readable are the directories a sandboxed tool may read and run from:
	// the toolchain and the tools (Command.Readable).
	readable []string
}

// New validates the configuration and returns a Builder.
func New(cfg Config) (*Builder, error) {
	switch {
	case cfg.Sandbox == nil:
		return nil, errors.New("builder.New: a sandbox is required")
	case cfg.Schemas == nil:
		return nil, errors.New("builder.New: a schema reader is required")
	case cfg.Clock == nil:
		return nil, errors.New("builder.New: a clock is required")
	case cfg.SDKDir == "":
		return nil, errors.New("builder.New: the SDK directory is required")
	case len(cfg.WasmOpt) == 0:
		return nil, errors.New("builder.New: the wasm-opt command is required")
	case cfg.Limits.Wall <= 0 || cfg.Limits.OutputBytes <= 0 || cfg.SourceBytes <= 0:
		return nil, errors.New("builder.New: the wall time, output and source limits are required (LIM-001)")
	case cfg.CacheDir != "" && cfg.CacheBytes <= 0:
		return nil, errors.New("builder.New: a cache needs its size limit (function.buildCacheSize)")
	}
	if _, unsandboxed := cfg.Sandbox.(NoSandbox); cfg.CacheDir != "" && !unsandboxed && cfg.CacheTotalBytes < cfg.CacheBytes {
		return nil, errors.New("builder.New: a sandboxed builder's caches need a total size limit of at least one cache's")
	}
	if cfg.Getenv == nil {
		cfg.Getenv = os.Getenv
	}
	var err error
	if cfg.Allowlist == nil {
		if cfg.Allowlist, err = DefaultAllowlist(); err != nil {
			return nil, err
		}
	}
	if cfg.Cargo, err = resolveTool(cfg.Cargo, "cargo"); err != nil {
		return nil, err
	}
	if cfg.Rustc, err = resolveTool(cfg.Rustc, "rustc"); err != nil {
		return nil, err
	}
	cfg.WasmOpt = append([]string{}, cfg.WasmOpt...)
	if cfg.WasmOpt[0], err = resolveTool(cfg.WasmOpt[0], ""); err != nil {
		return nil, err
	}
	return &Builder{cfg: cfg, sdk: os.DirFS(cfg.SDKDir), readable: toolDirs(cfg)}, nil
}

// toolDirs are the directories the tools of a build live in: those of cargo
// and rustc, of each part of the wasm-opt command and its parent (the Bun
// binary and the Binaryen package), and the toolchain directory.
func toolDirs(cfg Config) []string {
	var out []string
	add := func(p string) {
		if p != "" && filepath.IsAbs(p) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	add(filepath.Dir(cfg.Cargo))
	add(filepath.Dir(cfg.Rustc))
	for _, part := range cfg.WasmOpt {
		if filepath.IsAbs(part) {
			add(filepath.Dir(part))
			add(filepath.Dir(filepath.Dir(part)))
		}
	}
	add(rustupHome(cfg.Getenv))
	return out
}

// resolveTool returns the absolute path of a tool, looked up on PATH when it
// is a bare name.
func resolveTool(name, fallback string) (string, error) {
	if name == "" {
		name = fallback
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("builder.New: tool %q: %w", name, err)
	}
	return p, nil
}

// run is the state of one Build call.
type run struct {
	b       *Builder
	scratch string
	paths   envPaths
	env     []string
	// cachedTarget is the target directory in the cache when this build
	// holds the cache, and "" otherwise.
	cachedTarget string
	// cache is the cache directory this build holds, writable to its tools
	// beside the scratch directory, and "" otherwise.
	cache string
	// keyed is set when the cache is one organisation's (Options.CacheKey).
	keyed bool
}

// Build compiles the functions into one module and returns it with its
// schemas and provenance (FN-003, FN-004, FN-005, FN-016, FN-019). The module
// links the functions in the order of their names. Failures carry the codes
// PLX-7200 to PLX-7206; a sandbox the kernel refuses is [ErrSandboxUnavailable].
func (b *Builder) Build(ctx context.Context, srcs []FunctionSource, opts Options) (*Artifact, error) {
	funcs, err := prepareSources(srcs, b.cfg.SourceBytes)
	if err != nil {
		return nil, err
	}
	r, cleanup, err := b.newRun(opts.CacheKey, true)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	toolchain, err := r.checkRustc(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.checkWasmOpt(ctx); err != nil {
		return nil, err
	}
	stageRoot := r.paths.Workspace
	if r.cachedTarget != "" {
		stageRoot = filepath.Join(r.scratch, "ws")
	}
	staged, err := stageWorkspace(stageRoot, b.sdk, funcs, b.cfg.SourceBytes)
	if err != nil {
		return nil, err
	}
	if r.cachedTarget != "" {
		if err := syncTree(stageRoot, r.paths.Workspace); err != nil {
			return nil, fmt.Errorf("builder.Build: update the cached workspace: %w", err)
		}
	}
	deps, local, err := r.resolve(ctx, funcs)
	if err != nil {
		return nil, err
	}
	if err := r.checkBuildCode(ctx, deps); err != nil {
		return nil, err
	}
	if err := r.forgetLocalCrates(ctx, local); err != nil {
		return nil, err
	}
	schemas, err := r.deriveSchemas(ctx, funcs)
	if err != nil {
		return nil, err
	}
	module, err := r.artifact(ctx, opts.Verify)
	if err != nil {
		return nil, err
	}
	if r.cachedTarget != "" {
		pruneCache(r.cachedTarget, b.cfg.CacheBytes)
	}
	return &Artifact{
		Module:       module,
		ModuleSHA256: hexSum(module),
		ABIVersion:   ABIVersion,
		Functions:    describe(funcs, schemas),
		Provenance: Provenance{
			SourceSHA256:    staged.source,
			SDKSHA256:       staged.sdk,
			Dependencies:    deps,
			Toolchain:       toolchain,
			WasmOpt:         WasmOptVersion,
			BuildTime:       b.cfg.Clock().UTC().Format(time.RFC3339),
			BuilderIdentity: b.cfg.Identity,
		},
	}, nil
}

// describe lists the module's functions in index order.
func describe(funcs []preparedFunction, schemas []FunctionSchemas) []Function {
	out := make([]Function, len(funcs))
	for i, f := range funcs {
		out[i] = Function{
			Name: f.Name, Index: i, Placement: f.Placement,
			InputSchema: schemas[i].Input, OutputSchema: schemas[i].Output,
		}
	}
	return out
}

// newRun creates the scratch directory and the environment of a build. With
// cached, and a cache for key that is free, the workspace, Cargo home and home
// are the cache's, at paths that do not change between builds, so Cargo's
// fingerprints match and compiled dependencies are reused; the cache is held
// until the returned function runs, which also keeps the caches within
// CacheTotalBytes.
func (b *Builder) newRun(key string, cached bool) (*run, func(), error) {
	r, cleanup, err := b.newScratchRun()
	if err != nil || !cached {
		return r, cleanup, err
	}
	dir, keyed := b.cacheDir(key)
	if dir == "" {
		return r, cleanup, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return r, cleanup, nil //nolint:nilerr // the cache is an optimisation; the build goes on without it
	}
	unlock, held := tryLock(filepath.Join(dir, "lock"))
	if !held {
		return r, cleanup, nil
	}
	p := r.paths
	p.Workspace, p.CargoHome, p.Home = filepath.Join(dir, "ws"), filepath.Join(dir, "cargo"), filepath.Join(dir, "home")
	for _, d := range []string{p.Workspace, p.CargoHome, p.Home} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			unlock()
			return r, cleanup, nil //nolint:nilerr // as above
		}
	}
	// The lock file's time says when the cache was last used; the least
	// recently used caches are removed first.
	now := time.Now()
	_ = os.Chtimes(filepath.Join(dir, "lock"), now, now)
	r.paths, r.env, r.cachedTarget, r.cache, r.keyed = p, buildEnv(b.cfg.Getenv, p), filepath.Join(dir, sharedTarget), dir, keyed
	return r, func() {
		unlock()
		cleanup()
		if keyed {
			pruneCaches(filepath.Dir(dir), b.cfg.CacheTotalBytes)
		}
	}, nil
}

// cacheKeyPattern is what a cache key may be: it names a directory below
// CacheDir, so it can never name another one.
var cacheKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// cacheDir returns the cache directory of key and whether it is one of
// several keyed caches; "" means the build has no cache.
func (b *Builder) cacheDir(key string) (string, bool) {
	if b.cfg.CacheDir == "" {
		return "", false
	}
	root, err := filepath.Abs(b.cfg.CacheDir)
	if err != nil || strings.IndexFunc(root, unicode.IsSpace) >= 0 {
		// A path RUSTFLAGS cannot carry builds from scratch.
		return "", false
	}
	if key == "" {
		if _, unsandboxed := b.cfg.Sandbox.(NoSandbox); unsandboxed {
			return root, false
		}
		return "", false
	}
	if !cacheKeyPattern.MatchString(key) {
		return "", false
	}
	return filepath.Join(root, key), true
}

// newScratchRun creates a run whose directories are all in a new scratch
// directory.
func (b *Builder) newScratchRun() (*run, func(), error) {
	scratch, err := os.MkdirTemp(b.cfg.TempDir, "plux-build-")
	if err != nil {
		return nil, nil, fmt.Errorf("builder.Build: scratch directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(scratch) }
	if scratch, err = filepath.EvalSymlinks(scratch); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("builder.Build: scratch directory: %w", err)
	}
	if strings.IndexFunc(scratch, unicode.IsSpace) >= 0 {
		cleanup()
		return nil, nil, sourceError("the scratch directory %q contains white space; RUSTFLAGS cannot carry it", scratch)
	}
	p := envPaths{
		Workspace:  filepath.Join(scratch, "ws"),
		CargoHome:  filepath.Join(scratch, "cargo"),
		Home:       filepath.Join(scratch, "home"),
		RustupHome: rustupHome(b.cfg.Getenv),
	}
	for _, d := range []string{p.Workspace, p.CargoHome, p.Home, filepath.Join(scratch, "opt")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("builder.Build: %w", err)
		}
	}
	return &run{b: b, scratch: scratch, paths: p, env: buildEnv(b.cfg.Getenv, p)}, cleanup, nil
}

// exec runs one tool in the workspace with the configured limits.
func (r *run) exec(ctx context.Context, path string, args []string) (Result, error) {
	return r.execIn(ctx, r.paths.Workspace, path, args, r.b.cfg.Limits)
}

// execIn runs one tool in dir.
func (r *run) execIn(ctx context.Context, dir, path string, args []string, lim Limits) (Result, error) {
	res, err := r.b.cfg.Sandbox.Run(ctx, Command{
		Path: path, Args: args, Dir: dir, Env: r.env, Limits: lim, Writable: r.writable(), Readable: r.b.readable,
	})
	if err != nil {
		return res, fmt.Errorf("builder: run %s: %w", filepath.Base(path), err)
	}
	return res, nil
}

// writable are the directories the build's tools may change: its scratch
// directory, first, and the cache it holds.
func (r *run) writable() []string {
	if r.cache == "" {
		return []string{r.scratch}
	}
	return []string{r.scratch, r.cache}
}

// cargo runs cargo in the workspace and turns a failure into a Plux error.
func (r *run) cargo(ctx context.Context, step string, lim Limits, args ...string) (Result, error) {
	res, err := r.execIn(ctx, r.paths.Workspace, r.b.cfg.Cargo, args, lim)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 || res.TimedOut || res.OutputExceeded {
		return res, r.failure(res, step)
	}
	return res, nil
}

// diagnostics returns the tool's error output with every host path replaced
// by its stable name, cut to 64 KiB (E7).
func (r *run) diagnostics(res Result) string {
	text := string(res.Stderr)
	if text == "" {
		text = string(res.Stdout)
	}
	repl := strings.NewReplacer(
		r.paths.Workspace, "/src",
		r.paths.CargoHome, "/cargo",
		r.paths.Home, "/home",
		r.scratch, "/build",
	)
	text = repl.Replace(text)
	if r.paths.RustupHome != "" {
		text = strings.ReplaceAll(text, r.paths.RustupHome, "/rustup")
	}
	if len(text) > diagnosticBytes {
		text = strings.ToValidUTF8(text[:diagnosticBytes], "")
	}
	return text
}

// memoryMarkers appear when a process hits its address-space limit.
var memoryMarkers = []string{"memory allocation of", "out of memory", "Cannot allocate memory", "SIGXCPU"}

// resolutionMarkers appear when cargo cannot resolve a dependency from the
// vendored sources or its lock file would change.
var resolutionMarkers = []string{
	"failed to select a version", "no matching package", "needs to be updated",
	"failed to load source for dependency", "attempting to make an HTTP request", "failed to download",
	"checksum for", "failed to verify the checksum",
}

// failure classifies a failed tool run: a limit is PLX-7203, a placement
// mismatch PLX-7205, an unresolvable dependency PLX-7201, anything else
// PLX-7200 with the diagnostics.
func (r *run) failure(res Result, step string) error {
	text := r.diagnostics(res)
	switch {
	case res.TimedOut:
		return plxerr.New(plxerr.FunctionBuildLimitExceeded, "%s exceeded the build time limit", step)
	case res.OutputExceeded:
		return plxerr.New(plxerr.FunctionBuildLimitExceeded, "%s exceeded the output limit", step)
	case res.Signal == sigXCPU || r.killedByLimit(res) || containsAny(text, memoryMarkers):
		return plxerr.New(plxerr.FunctionBuildLimitExceeded, "%s exceeded the CPU or memory limit:\n%s", step, text)
	case strings.Contains(text, "PLX-7205"):
		return plxerr.New(plxerr.FunctionDeclarationMismatch, "%s: a function's placement differs from its document:\n%s", step, text)
	case containsAny(text, resolutionMarkers):
		return plxerr.New(plxerr.FunctionDependencyNotVendored, "%s: a dependency is not vendored or the lock file drifted (FN-003):\n%s", step, text)
	default:
		return plxerr.New(plxerr.FunctionBuildFailed, "%s failed:\n%s", step, text)
	}
}

// killedByLimit reports whether SIGKILL ended the tool while a CPU or memory
// limit was set and neither the wall time nor the output limit was the cause.
func (r *run) killedByLimit(res Result) bool {
	l := r.b.cfg.Limits
	return res.Signal == sigKill && (l.CPUTime > 0 || l.Memory > 0)
}

// containsAny reports whether text contains one of the markers.
func containsAny(text string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

// resolve generates the workspace lock file offline and checks it against the
// functions' own lock files (FN-003).
func (r *run) resolve(ctx context.Context, funcs []preparedFunction) ([]Dependency, []lockPackage, error) {
	if _, err := r.cargo(ctx, "cargo generate-lockfile", r.b.cfg.Limits, "generate-lockfile", "--offline"); err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(filepath.Join(r.paths.Workspace, "Cargo.lock"))
	if err != nil {
		return nil, nil, fmt.Errorf("builder.resolve: %w", err)
	}
	resolved, err := parseLock(data)
	if err != nil {
		return nil, nil, plxerr.Wrap(plxerr.FunctionBuildFailed, err, "reading the generated lock file")
	}
	if err := checkLockAgainst(funcs, resolved); err != nil {
		return nil, nil, err
	}
	var local []lockPackage
	for _, p := range resolved {
		if !p.registry() {
			local = append(local, p)
		}
	}
	return dependenciesOf(resolved), local, nil
}

// forgetLocalCrates removes from an organisation's cached target directory
// what earlier builds compiled of the workspace's own crates — the
// functions, the module wrapper and the SDK — so this build compiles them
// from its own sources whatever their files' times say, and the module the
// server signs is made from the source its provenance names. Only crates.io
// dependencies, made from archives verified against their checksums, are
// reused (ADR-0107). A developer's own cache (no key) keeps them, as Cargo
// does, for speed: its modules are not signed.
func (r *run) forgetLocalCrates(ctx context.Context, local []lockPackage) error {
	if r.cachedTarget == "" || !r.keyed || len(local) == 0 {
		return nil
	}
	args := []string{"clean", "--offline", "--locked", "--release", "--target", wasmTarget, "--target-dir", r.cachedTarget}
	for _, p := range local {
		args = append(args, "-p", p.Name+"@"+p.Version)
	}
	_, err := r.cargo(ctx, "cargo clean", r.b.cfg.Limits, args...)
	return err
}

// checkBuildCode applies the allowlist to the crates the build compiles
// (FN-016), with the checksums of the resolved dependencies.
func (r *run) checkBuildCode(ctx context.Context, deps []Dependency) error {
	lim := r.b.cfg.Limits
	lim.OutputBytes *= metadataOutputFactor
	meta, err := r.cargo(ctx, "cargo metadata", lim, "metadata", "--offline", "--locked", "--format-version", "1")
	if err != nil {
		return err
	}
	tree, err := r.cargo(ctx, "cargo tree", lim, "tree", "--offline", "--locked", "-p", moduleName,
		"--target", wasmTarget, "-e", "normal,build", "--prefix", "none", "--format", "{p}")
	if err != nil {
		return err
	}
	built, err := builtPackages(tree.Stdout)
	if err != nil {
		return plxerr.Wrap(plxerr.FunctionBuildFailed, err, "reading the dependency tree")
	}
	checksums := make(map[string]string, len(deps))
	for _, d := range deps {
		checksums[d.Name+" "+d.Version] = d.Checksum
	}
	return checkAllowlist(meta.Stdout, built, checksums, r.b.cfg.Allowlist)
}

// sharedTarget is the target directory the schema build and the artefact
// build of one Build share: the dependencies compiled for the schema build
// are reused by the artefact build, which recompiles only what the feature
// plux-schema changes. Nothing is shared between Build calls.
const sharedTarget = "target"

// compile builds the wrapper into a target directory and returns the raw
// module; with schema it enables the feature plux-schema (FN-004).
func (r *run) compile(ctx context.Context, name string, schema bool) ([]byte, error) {
	target := filepath.Join(r.scratch, name)
	if name == sharedTarget && r.cachedTarget != "" {
		target = r.cachedTarget
	}
	args := []string{
		"build", "--release", "--locked", "--offline", "--frozen",
		"--target", wasmTarget, "-p", moduleName, "--target-dir", target,
	}
	if schema {
		args = append(args, "--features", moduleName+"/plux-schema")
	}
	if _, err := r.cargo(ctx, "cargo build", r.b.cfg.Limits, args...); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(target, wasmTarget, "release", moduleFile)) //nolint:gosec // G304: the path is built from the builder's own scratch directory.
	if err != nil {
		return nil, plxerr.Wrap(plxerr.FunctionBuildFailed, err, "the build produced no module")
	}
	return raw, nil
}

// deriveSchemas builds the module with plux-schema, runs it through the
// SchemaReader and returns the canonical schema of every function (FN-004).
func (r *run) deriveSchemas(ctx context.Context, funcs []preparedFunction) ([]FunctionSchemas, error) {
	raw, err := r.compile(ctx, sharedTarget, true)
	if err != nil {
		return nil, err
	}
	got, err := r.b.cfg.Schemas.Schemas(ctx, raw, len(funcs))
	if err != nil {
		return nil, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, err, "reading the schemas of the module")
	}
	if len(got) != len(funcs) {
		return nil, plxerr.New(plxerr.FunctionSchemaDerivationFailed,
			"the module returned %d schema pairs for %d functions", len(got), len(funcs))
	}
	out := make([]FunctionSchemas, len(got))
	for i, g := range got {
		in, err := jcs.Canonicalize(g.Input, schemaDepth)
		if err != nil {
			return nil, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, err, "input schema of %q", funcs[i].Name)
		}
		o, err := jcs.Canonicalize(g.Output, schemaDepth)
		if err != nil {
			return nil, plxerr.Wrap(plxerr.FunctionSchemaDerivationFailed, err, "output schema of %q", funcs[i].Name)
		}
		out[i] = FunctionSchemas{Input: in, Output: o}
	}
	return out, nil
}

// artifact builds and optimises the module without the schema feature; with
// verify it does so twice into separate target directories and compares the
// bytes (FN-019, PLX-7206).
func (r *run) artifact(ctx context.Context, verify bool) ([]byte, error) {
	first, err := r.compileAndOptimise(ctx, sharedTarget)
	if err != nil || !verify {
		return first, err
	}
	// The second build starts from an empty target directory, so Verify also
	// proves that reusing the schema build's dependencies changed nothing.
	second, err := r.compileAndOptimise(ctx, "target-fresh")
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, plxerr.New(plxerr.FunctionBuildNotReproducible,
			"two builds of the same sources differ (%s and %s); the build is not reproducible (FN-019)",
			hexSum(first), hexSum(second))
	}
	return first, nil
}

// compileAndOptimise runs one artefact build followed by wasm-opt.
func (r *run) compileAndOptimise(ctx context.Context, name string) ([]byte, error) {
	raw, err := r.compile(ctx, name, false)
	if err != nil {
		return nil, err
	}
	return r.optimise(ctx, name, raw)
}

// optimise runs the pinned wasm-opt with -Os over a module (E7, E10).
func (r *run) optimise(ctx context.Context, name string, raw []byte) ([]byte, error) {
	dir := filepath.Join(r.scratch, "opt")
	in, out := name+".in.wasm", name+".out.wasm"
	if err := os.WriteFile(filepath.Join(dir, in), raw, 0o600); err != nil {
		return nil, fmt.Errorf("builder.optimise: %w", err)
	}
	prefix := r.b.cfg.WasmOpt
	args := append([]string{}, prefix[1:]...)
	args = append(args, in, "-o", out, "-Os", "--strip-debug", "--strip-producers")
	args = append(args, wasmOptFeatures...)
	res, err := r.execIn(ctx, dir, prefix[0], args, r.b.cfg.Limits)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 || res.TimedOut || res.OutputExceeded {
		return nil, r.failure(res, "wasm-opt")
	}
	opt, err := os.ReadFile(filepath.Join(dir, out)) //nolint:gosec // G304: the path is built from the builder's own scratch directory.
	if err != nil {
		return nil, plxerr.Wrap(plxerr.FunctionBuildFailed, err, "wasm-opt produced no module")
	}
	return opt, nil
}

// pruneCaches removes the keyed caches below root that were used least
// recently, until those left take at most total bytes. A cache in use is
// never removed.
func pruneCaches(root string, total int64) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type cache struct {
		dir  string
		used time.Time
		size int64
	}
	var caches []cache
	var sum int64
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".removed-") {
			_ = os.RemoveAll(filepath.Join(root, e.Name())) // left by a process that stopped
			continue
		}
		if !e.IsDir() || !cacheKeyPattern.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		info, err := os.Stat(filepath.Join(dir, "lock"))
		if err != nil {
			continue
		}
		size := dirSize(dir)
		caches = append(caches, cache{dir: dir, used: info.ModTime(), size: size})
		sum += size
	}
	slices.SortFunc(caches, func(a, b cache) int { return a.used.Compare(b.used) })
	for _, c := range caches {
		if sum <= total {
			return
		}
		unlock, held := tryLock(filepath.Join(c.dir, "lock"))
		if !held {
			continue
		}
		// Renamed first, under its lock, so a build that starts now finds
		// no cache and makes a new one rather than one being removed.
		trash := filepath.Join(root, ".removed-"+filepath.Base(c.dir))
		err := os.Rename(c.dir, trash)
		unlock()
		if err == nil {
			_ = os.RemoveAll(trash)
			sum -= c.size
		}
	}
}

// dirSize is the size of the regular files below dir; a file that vanishes
// meanwhile counts as nothing.
func dirSize(dir string) int64 {
	var size int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}

// pruneCache empties a cached target directory that has grown beyond most
// bytes, so the cache stays bounded (function.devBuildCacheSize); the next
// build fills it again with what it needs.
func pruneCache(target string, most int64) {
	if dirSize(target) > most {
		_ = os.RemoveAll(target)
	}
}

// syncTree makes dst hold exactly the files of src. A file whose bytes are
// already in dst is left as it is, so its modification time, which Cargo
// compares for path and vendored packages, does not change and what was
// compiled from it is reused; a changed file is written; everything dst holds
// beyond src is removed, so no crate of an earlier build is left in vendor/.
// Both trees are opened as roots, so no symbolic link can lead a write or a
// removal outside them.
func syncTree(src, dst string) error {
	from, err := os.OpenRoot(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = from.Close() }()
	to, err := os.OpenRoot(dst)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer func() { _ = to.Close() }()
	want, err := copyChanged(from, to)
	if err != nil {
		return err
	}
	return removeExtra(to, want)
}

// copyChanged writes into to every file of from that to does not already
// hold with the same bytes, and returns the paths of from.
func copyChanged(from, to *os.Root) (map[string]bool, error) {
	want := map[string]bool{".": true}
	err := fs.WalkDir(from.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		want[rel] = true
		if d.IsDir() {
			return ensureDir(to, rel)
		}
		data, err := from.ReadFile(rel)
		if err != nil {
			return err //nolint:wrapcheck // wrapped below
		}
		return writeIfChanged(to, rel, data)
	})
	if err != nil {
		return nil, fmt.Errorf("copy the workspace: %w", err)
	}
	return want, nil
}

// ensureDir makes rel a directory in root, replacing a file of that name.
func ensureDir(root *os.Root, rel string) error {
	if info, err := root.Lstat(rel); err == nil && !info.IsDir() {
		if err := root.RemoveAll(rel); err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
	}
	return root.MkdirAll(rel, 0o750) //nolint:wrapcheck // wrapped by the caller
}

// writeIfChanged writes data to rel in root unless a regular file with the
// same bytes is already there.
func writeIfChanged(root *os.Root, rel string, data []byte) error {
	if info, err := root.Lstat(rel); err == nil {
		if info.Mode().IsRegular() {
			if old, err := root.ReadFile(rel); err == nil && bytes.Equal(old, data) {
				return nil
			}
		}
		if err := root.RemoveAll(rel); err != nil {
			return err //nolint:wrapcheck // wrapped by the caller
		}
	}
	return root.WriteFile(rel, data, 0o600) //nolint:wrapcheck // wrapped by the caller
}

// removeExtra removes from root every path not in want.
func removeExtra(root *os.Root, want map[string]bool) error {
	var extra []string
	err := fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case want[rel]:
			return nil
		case d.IsDir():
			extra = append(extra, rel)
			return fs.SkipDir
		default:
			extra = append(extra, rel)
			return nil
		}
	})
	if err != nil {
		return fmt.Errorf("list the cached workspace: %w", err)
	}
	for _, rel := range extra {
		if err := root.RemoveAll(rel); err != nil {
			return fmt.Errorf("remove %s from the cached workspace: %w", rel, err)
		}
	}
	return nil
}

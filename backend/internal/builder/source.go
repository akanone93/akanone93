// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// functionName is the shape of a function document name that is safe as a
// directory name and as a sort key.
var functionName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,127}$`)

// vendorCrate is one crate directory of a function's vendor/ directory.
type vendorCrate struct {
	// topDir is the directory's name inside vendor/.
	topDir string
	// name and version come from the crate's own manifest.
	name, version string
	// identity is the crate archive checksum, or a digest of the checksum file
	// for a crate that has none; equal name and version must have equal
	// identities across functions (FN-003).
	identity string
}

// key is the merged directory name of the crate.
func (v vendorCrate) key() string { return v.name + "-" + v.version }

// preparedFunction is a function source that passed the structural checks.
type preparedFunction struct {
	FunctionSource
	// pkg is the cargo package name; ident is the crate identifier the wrapper
	// uses (hyphens become underscores).
	pkg, ident string
	// lock holds the packages of the function's own Cargo.lock.
	lock []lockPackage
	// vendor lists the crates of its vendor/ directory.
	vendor []vendorCrate
}

// dir is the directory of the crate inside the build workspace.
func (p preparedFunction) dir() string { return "functions/" + p.Name }

// sourceError is a PLX-7200 error about a malformed input.
func sourceError(format string, args ...any) error {
	return plxerr.New(plxerr.FunctionBuildFailed, format, args...)
}

// prepareSources validates every function source and orders them by name,
// which fixes each function's index in the module (E5).
func prepareSources(srcs []FunctionSource) ([]preparedFunction, error) {
	if len(srcs) == 0 {
		return nil, sourceError("a module needs at least one function")
	}
	out := make([]preparedFunction, 0, len(srcs))
	for _, s := range srcs {
		p, err := prepareSource(s)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	for i := 1; i < len(out); i++ {
		if out[i].Name == out[i-1].Name {
			return nil, sourceError("function %q is listed twice", out[i].Name)
		}
	}
	pkgs := map[string]string{}
	for _, p := range out {
		if other, dup := pkgs[p.pkg]; dup {
			return nil, sourceError("functions %q and %q use the same crate %q", other, p.Name, p.pkg)
		}
		pkgs[p.pkg] = p.Name
	}
	return out, nil
}

// prepareSource checks one function source: its identity, its manifest, its
// lock file (FN-003) and its vendor directory.
func prepareSource(s FunctionSource) (preparedFunction, error) {
	if !functionName.MatchString(s.Name) {
		return preparedFunction{}, sourceError("function name %q is not valid", s.Name)
	}
	if !s.Placement.Valid() {
		return preparedFunction{}, sourceError("function %q has placement %q; want device or server", s.Name, s.Placement)
	}
	if s.Files == nil {
		return preparedFunction{}, sourceError("function %q has no source files", s.Name)
	}
	manifest, err := fs.ReadFile(s.Files, "Cargo.toml")
	if err != nil {
		return preparedFunction{}, sourceError("function %q: reading Cargo.toml: %v", s.Name, err)
	}
	if declaresWorkspace(manifest) {
		return preparedFunction{}, sourceError("function %q: Cargo.toml declares a [workspace]; the builder owns the workspace", s.Name)
	}
	pkg := tomlStringFields(manifest, "package")["name"]
	if pkg == "" {
		return preparedFunction{}, sourceError("function %q: Cargo.toml has no package name", s.Name)
	}
	lock, err := readFunctionLock(s)
	if err != nil {
		return preparedFunction{}, err
	}
	vendor, err := readVendor(s)
	if err != nil {
		return preparedFunction{}, err
	}
	return preparedFunction{
		FunctionSource: s,
		pkg:            pkg,
		ident:          strings.ReplaceAll(pkg, "-", "_"),
		lock:           lock,
		vendor:         vendor,
	}, nil
}

// readFunctionLock reads Cargo.lock and checks that every registry package
// can be found in the vendored sources (FN-003, PLX-7201).
func readFunctionLock(s FunctionSource) ([]lockPackage, error) {
	data, err := fs.ReadFile(s.Files, "Cargo.lock")
	if err != nil {
		return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
			"function %q has no Cargo.lock; a lock file is required (FN-003)", s.Name)
	}
	lock, err := parseLock(data)
	if err != nil {
		return nil, plxerr.Wrap(plxerr.FunctionDependencyNotVendored, err, "function %q: Cargo.lock", s.Name)
	}
	for _, p := range lock {
		if !p.registry() {
			continue
		}
		if p.Source != cratesIOSource || p.Checksum == "" {
			return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
				"function %q: crate %s %s comes from %q; only vendored crates.io crates are accepted",
				s.Name, p.Name, p.Version, p.Source)
		}
	}
	return lock, nil
}

// checksumFile is the part of .cargo-checksum.json the builder reads.
type checksumFile struct {
	Package *string `json:"package"`
}

// readVendor lists the crates in vendor/ with their identity.
func readVendor(s FunctionSource) ([]vendorCrate, error) {
	entries, err := fs.ReadDir(s.Files, "vendor")
	if err != nil {
		return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
			"function %q has no vendor/ directory; run `cargo vendor` (FN-003)", s.Name)
	}
	var out []vendorCrate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c, err := readVendorCrate(s, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// readVendorCrate reads the identity of one vendored crate.
func readVendorCrate(s FunctionSource, top string) (vendorCrate, error) {
	manifest, err := fs.ReadFile(s.Files, "vendor/"+top+"/Cargo.toml")
	if err != nil {
		return vendorCrate{}, plxerr.New(plxerr.FunctionDependencyNotVendored,
			"function %q: vendored crate %q has no Cargo.toml", s.Name, top)
	}
	fields := tomlStringFields(manifest, "package")
	if fields["name"] == "" || fields["version"] == "" {
		return vendorCrate{}, plxerr.New(plxerr.FunctionDependencyNotVendored,
			"function %q: vendored crate %q has no name or version", s.Name, top)
	}
	raw, err := fs.ReadFile(s.Files, "vendor/"+top+"/.cargo-checksum.json")
	if err != nil {
		return vendorCrate{}, plxerr.New(plxerr.FunctionDependencyNotVendored,
			"function %q: vendored crate %q has no .cargo-checksum.json", s.Name, top)
	}
	var sum checksumFile
	if err := json.Unmarshal(raw, &sum); err != nil {
		return vendorCrate{}, plxerr.New(plxerr.FunctionDependencyNotVendored,
			"function %q: vendored crate %q has a malformed .cargo-checksum.json", s.Name, top)
	}
	identity := "files:" + hexSum(raw)
	if sum.Package != nil {
		identity = "package:" + *sum.Package
	}
	return vendorCrate{topDir: top, name: fields["name"], version: fields["version"], identity: identity}, nil
}

// mergeVendorPlan decides where each vendored crate goes in the merged vendor
// directory and fails when two functions vendor the same name and version with
// different checksums (FN-003, PLX-7201). owner maps a merged directory to the
// index of the function whose copy is written.
func mergeVendorPlan(funcs []preparedFunction) (map[string]int, error) {
	owner := map[string]int{}
	identity := map[string]string{}
	holder := map[string]string{}
	for i, f := range funcs {
		for _, c := range f.vendor {
			k := c.key()
			if prev, seen := identity[k]; seen {
				if prev != c.identity {
					return nil, plxerr.New(plxerr.FunctionDependencyNotVendored,
						"crate %s %s has different checksums in functions %q and %q",
						c.name, c.version, holder[k], f.Name)
				}
				continue
			}
			identity[k], holder[k], owner[k] = c.identity, f.Name, i
		}
	}
	return owner, nil
}

// checkLockAgainst verifies that the workspace's resolved registry crates are
// exactly versions and checksums the functions' own lock files recorded
// (FN-003): drift is PLX-7201.
func checkLockAgainst(funcs []preparedFunction, resolved []lockPackage) error {
	expected := map[string]string{}
	for _, f := range funcs {
		for _, p := range f.lock {
			if p.registry() {
				expected[p.key()] = p.Checksum
			}
		}
	}
	for _, p := range resolved {
		if !p.registry() {
			continue
		}
		if p.Source != cratesIOSource {
			return plxerr.New(plxerr.FunctionDependencyNotVendored,
				"crate %s %s resolves from %q, which is not vendored", p.Name, p.Version, p.Source)
		}
		want, ok := expected[p.key()]
		if !ok {
			return plxerr.New(plxerr.FunctionDependencyNotVendored,
				"crate %s %s is not in any function's Cargo.lock; the lock file has drifted", p.Name, p.Version)
		}
		if want != p.Checksum {
			return plxerr.New(plxerr.FunctionDependencyNotVendored,
				"crate %s %s has checksum %s; the function's Cargo.lock records %s", p.Name, p.Version, p.Checksum, want)
		}
	}
	return nil
}

// fileHash is the path and digest of one source file.
type fileHash struct {
	path string
	sum  string
}

// canonicalHash is the SHA-256 over the sorted list of path and SHA-256 of
// every file: each file contributes `path NUL hex(sha256) LF`, in bytewise
// path order. It does not depend on file modes, times or archive layout.
func canonicalHash(files []fileHash) string {
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.path))
		h.Write([]byte{0})
		h.Write([]byte(f.sum))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// hexSum is the lower-case hexadecimal SHA-256 of data.
func hexSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// refuseSpecial reports a directory entry that is neither a directory nor a
// regular file.
func refuseSpecial(name string, d fs.DirEntry) error {
	return sourceError("%s is a %s; only regular files are accepted", name, strings.ToLower(d.Type().String()))
}

// sizeBudget tracks the source bytes still allowed (the registry's
// function.sourceArchiveSize).
type sizeBudget struct{ remaining int64 }

// take uses n bytes of the budget and fails with PLX-7203 when it is exhausted.
func (b *sizeBudget) take(n int64) error {
	b.remaining -= n
	if b.remaining < 0 {
		return plxerr.New(plxerr.FunctionBuildLimitExceeded, "the sources exceed the source size limit")
	}
	return nil
}

// walkFiles calls visit with every regular file of fsys in lexical order.
func walkFiles(fsys fs.FS, budget *sizeBudget, visit func(rel string, data []byte) error) error {
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return refuseSpecial(p, d)
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", p, err)
		}
		if budget != nil {
			if err := budget.take(info.Size()); err != nil {
				return err
			}
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		return visit(p, data)
	})
	if err != nil {
		return fmt.Errorf("builder.walkFiles: %w", err)
	}
	return nil
}

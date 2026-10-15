// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// cratesIOSource is the source string a lock file gives registry crates. The
// builder replaces crates.io by the vendored directory, so no other registry
// and no git source can be resolved (FN-003).
const cratesIOSource = "registry+https://github.com/rust-lang/crates.io-index"

// stringField matches `key = "value"` on one line of a manifest or lock file.
var stringField = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=\s*"([^"]*)"\s*(#.*)?$`)

// tomlStringFields returns the string-valued keys of the named top-level table
// of a Cargo manifest, for example "package". It reads the line-oriented form
// cargo itself writes and the one developers write by hand; it is not a TOML
// parser and ignores nested tables, arrays and multi-line values.
func tomlStringFields(manifest []byte, table string) map[string]string {
	out := map[string]string{}
	in := false
	sc := bufio.NewScanner(bytes.NewReader(manifest))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			in = line == "["+table+"]"
			continue
		}
		if !in {
			continue
		}
		if m := stringField.FindStringSubmatch(line); m != nil {
			out[m[1]] = m[2]
		}
	}
	return out
}

// declaresWorkspace reports whether the manifest has a [workspace] table or a
// sub-table of it. A function crate must not: the builder owns the workspace.
func declaresWorkspace(manifest []byte) bool {
	sc := bufio.NewScanner(bytes.NewReader(manifest))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "[workspace]" || strings.HasPrefix(line, "[workspace.") {
			return true
		}
	}
	return false
}

// lockPackage is one [[package]] entry of a Cargo.lock.
type lockPackage struct {
	Name     string
	Version  string
	Source   string
	Checksum string
}

// registry reports whether the package comes from the registry; path and
// workspace packages have no source.
func (p lockPackage) registry() bool { return p.Source != "" }

// key identifies a package version.
func (p lockPackage) key() string { return p.Name + " " + p.Version }

// parseLock reads the packages of a Cargo.lock. It returns an error for a file
// that has none, which also covers an empty or unrelated file.
func parseLock(data []byte) ([]lockPackage, error) {
	var pkgs []lockPackage
	var cur *lockPackage
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "[[package]]" {
			pkgs = append(pkgs, lockPackage{})
			cur = &pkgs[len(pkgs)-1]
			continue
		}
		if strings.HasPrefix(line, "[") {
			cur = nil
			continue
		}
		if cur == nil {
			continue
		}
		m := stringField.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch m[1] {
		case "name":
			cur.Name = m[2]
		case "version":
			cur.Version = m[2]
		case "source":
			cur.Source = m[2]
		case "checksum":
			cur.Checksum = m[2]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("builder.parseLock: %w", err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("builder.parseLock: no packages in the lock file")
	}
	for _, p := range pkgs {
		if p.Name == "" || p.Version == "" {
			return nil, fmt.Errorf("builder.parseLock: a package entry lacks its name or version")
		}
	}
	return pkgs, nil
}

// dependenciesOf returns the registry packages of a lock as provenance
// dependencies, sorted by name, then version.
func dependenciesOf(pkgs []lockPackage) []Dependency {
	var deps []Dependency
	for _, p := range pkgs {
		if p.registry() {
			deps = append(deps, Dependency{Name: p.Name, Version: p.Version, Checksum: p.Checksum})
		}
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Name != deps[j].Name {
			return deps[i].Name < deps[j].Name
		}
		return deps[i].Version < deps[j].Version
	})
	return deps
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	_ "embed" // The allowlist is part of the binary.
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

//go:embed allowlist.json
var embeddedAllowlist []byte

// Allowlist names the crates, by exact version, whose build scripts and
// procedural macros may run in the builder (FN-016, ADR-0063). Extending the
// embedded list is a change to a security control and needs the maintainer.
type Allowlist map[string][]string

// DefaultAllowlist returns the allowlist embedded in the binary.
func DefaultAllowlist() (Allowlist, error) {
	return ParseAllowlist(embeddedAllowlist)
}

// ParseAllowlist reads an allowlist of the form {"crate": ["version", …]}.
func ParseAllowlist(data []byte) (Allowlist, error) {
	var a Allowlist
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("builder.ParseAllowlist: %w", err)
	}
	return a, nil
}

// allows reports whether the exact version of the crate is listed.
func (a Allowlist) allows(name, version string) bool {
	for _, v := range a[name] {
		if v == version {
			return true
		}
	}
	return false
}

// metadataDoc is the part of `cargo metadata` the allowlist check reads.
type metadataDoc struct {
	Packages []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Source  string `json:"source"`
		Targets []struct {
			Kind []string `json:"kind"`
		} `json:"targets"`
	} `json:"packages"`
}

// forEachCodeRunner calls visit for each package that whether a package has a build script or is a procedural
// has a build script or is a procedural macro, which makes the compiler run
// its code natively.
func (m metadataDoc) forEachCodeRunner(visit func(name, version string)) {
	for _, p := range m.Packages {
		for _, t := range p.Targets {
			if hasKind(t.Kind, "custom-build") || hasKind(t.Kind, "proc-macro") {
				visit(p.Name, p.Version)
				break
			}
		}
	}
}

// hasKind reports whether kinds contains kind.
func hasKind(kinds []string, kind string) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// builtPackages reads the output of `cargo tree --prefix none --format "{p}"`
// into the set of "name version" keys that are actually compiled.
func builtPackages(tree []byte) (map[string]bool, error) {
	set := map[string]bool{}
	for _, line := range strings.Split(string(tree), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 2 || !strings.HasPrefix(f[1], "v") {
			return nil, fmt.Errorf("builder.builtPackages: unexpected line %q", line)
		}
		set[f[0]+" "+strings.TrimPrefix(f[1], "v")] = true
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("builder.builtPackages: empty dependency tree")
	}
	return set, nil
}

// checkAllowlist fails with PLX-7202, naming the crate, when a package that is
// compiled has a build script or is a procedural macro and is not on the
// allowlist (FN-016). Packages in the metadata that the build does not compile,
// such as optional dependencies that no feature enables, are not considered.
func checkAllowlist(metadata []byte, built map[string]bool, allow Allowlist) error {
	var doc metadataDoc
	if err := json.Unmarshal(metadata, &doc); err != nil {
		return plxerr.Wrap(plxerr.FunctionBuildFailed, err, "reading cargo metadata")
	}
	var refused []string
	doc.forEachCodeRunner(func(name, version string) {
		if built[name+" "+version] && !allow.allows(name, version) {
			refused = append(refused, name+" "+version)
		}
	})
	if len(refused) == 0 {
		return nil
	}
	sort.Strings(refused)
	return plxerr.New(plxerr.FunctionBuildScriptRefused,
		"crate %s has a build script or is a procedural macro and is not on the allowlist (FN-016)", refused[0])
}

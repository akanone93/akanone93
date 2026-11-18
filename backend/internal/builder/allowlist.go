// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	_ "embed" // The allowlist is part of the binary.
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

//go:embed allowlist.json
var embeddedAllowlist []byte

// Allowlist names the crates whose build scripts and procedural macros may
// run in the builder, by exact version and the SHA-256 of the archive
// crates.io publishes for it: {"crate": {"version": "sha256", …}} (FN-016,
// ADR-0063, ADR-0105). The builder makes vendor/ only from archives whose
// SHA-256 is the checksum Cargo.lock records, so a pinned version is the
// published code and nothing else. Extending the embedded list is a change to
// a security control and needs the maintainer.
type Allowlist map[string]map[string]string

// DefaultAllowlist returns the allowlist embedded in the binary.
func DefaultAllowlist() (Allowlist, error) {
	return ParseAllowlist(embeddedAllowlist)
}

// ParseAllowlist reads an allowlist and refuses one whose checksums are not
// lower-case hex SHA-256 digests.
func ParseAllowlist(data []byte) (Allowlist, error) {
	var a Allowlist
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("builder.ParseAllowlist: %w", err)
	}
	for name, versions := range a {
		for version, sum := range versions {
			if !sha256Hex.MatchString(sum) {
				return nil, fmt.Errorf("builder.ParseAllowlist: %s %s: %q is not a SHA-256", name, version, sum)
			}
		}
	}
	return a, nil
}

// sha256Hex is a lower-case hex SHA-256 digest.
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// pin returns the checksum the allowlist pins for a crate version.
func (a Allowlist) pin(name, version string) (string, bool) {
	sum, ok := a[name][version]
	return sum, ok
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

// forEachCodeRunner calls visit for each package that has a build script or
// is a procedural macro, which makes the compiler run its code natively.
func (m metadataDoc) forEachCodeRunner(visit func(name, version, source string)) {
	for _, p := range m.Packages {
		for _, t := range p.Targets {
			if hasKind(t.Kind, "custom-build") || hasKind(t.Kind, "proc-macro") {
				visit(p.Name, p.Version, p.Source)
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
// compiled has a build script or is a procedural macro and is not the pinned
// crates.io archive of a version on the allowlist (FN-016, ADR-0105): it must
// come from crates.io, its version must be listed, and the checksum the build
// uses for it (checksums, keyed "name version") must be the pinned one. Path
// and workspace packages never run code at build time. Packages in the
// metadata that the build does not compile, such as optional dependencies that
// no feature enables, are not considered.
func checkAllowlist(metadata []byte, built map[string]bool, checksums map[string]string, allow Allowlist) error {
	var doc metadataDoc
	if err := json.Unmarshal(metadata, &doc); err != nil {
		return plxerr.Wrap(plxerr.FunctionBuildFailed, err, "reading cargo metadata")
	}
	refused := map[string]string{}
	doc.forEachCodeRunner(func(name, version, source string) {
		key := name + " " + version
		if !built[key] {
			return
		}
		pin, listed := allow.pin(name, version)
		switch {
		case source != cratesIOSource:
			refused[key] = "crate " + key + " has a build script or is a procedural macro and does not come from crates.io (FN-016)"
		case !listed:
			refused[key] = "crate " + key + " has a build script or is a procedural macro and is not on the allowlist (FN-016)"
		case checksums[key] != pin:
			refused[key] = "crate " + key + " has a build script or is a procedural macro and its archive is not the one crates.io publishes, which the allowlist pins (FN-016, ADR-0105)"
		}
	})
	if len(refused) == 0 {
		return nil
	}
	keys := make([]string, 0, len(refused))
	for k := range refused {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return plxerr.New(plxerr.FunctionBuildScriptRefused, "%s", refused[keys[0]])
}

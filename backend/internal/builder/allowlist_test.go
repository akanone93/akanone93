// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// metadataJSON is a cargo metadata document with a build script, a procedural
// macro and a plain library from crates.io, and a procedural macro from a path
// that borrows an allowlisted name.
const metadataJSON = `{"packages":[
 {"name":"serde","version":"1.0.229","source":"registry+https://github.com/rust-lang/crates.io-index","targets":[{"kind":["lib"]},{"kind":["custom-build"]}]},
 {"name":"serde_derive","version":"1.0.229","source":"registry+https://github.com/rust-lang/crates.io-index","targets":[{"kind":["proc-macro"]}]},
 {"name":"leftpad","version":"1.0.0","source":"registry+https://github.com/rust-lang/crates.io-index","targets":[{"kind":["lib"]}]},
 {"name":"borsh","version":"1.8.1","source":"registry+https://github.com/rust-lang/crates.io-index","targets":[{"kind":["custom-build"]}]},
 {"name":"sneaky","version":"2.0.0","source":"registry+https://github.com/rust-lang/crates.io-index","targets":[{"kind":["proc-macro"]}]},
 {"name":"quote","version":"1.0.47","source":null,"targets":[{"kind":["proc-macro"]}]}
]}`

func built(keys ...string) map[string]bool {
	m := map[string]bool{}
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// pinned returns the checksums of the allowlist for the keys, as a build that
// uses the published archives has them.
func pinned(t *testing.T, allow Allowlist, keys ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, k := range keys {
		name, version, _ := strings.Cut(k, " ")
		sum, ok := allow.pin(name, version)
		if !ok {
			t.Fatalf("%s is not pinned", k)
		}
		out[k] = sum
	}
	return out
}

// Verifies: FN-016.
func TestAllowlistRefusesUnlistedBuildCode_FN_016(t *testing.T) {
	allow, err := DefaultAllowlist()
	if err != nil {
		t.Fatal(err)
	}
	ok := built("serde 1.0.229", "serde_derive 1.0.229", "leftpad 1.0.0")
	sums := pinned(t, allow, "serde 1.0.229", "serde_derive 1.0.229")
	if err := checkAllowlist([]byte(metadataJSON), ok, sums, allow); err != nil {
		t.Errorf("allowed set refused: %v", err)
	}
	err = checkAllowlist([]byte(metadataJSON), built("serde 1.0.229", "sneaky 2.0.0", "borsh 1.8.1"), sums, allow)
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
	if !strings.Contains(err.Error(), "borsh 1.8.1") {
		t.Errorf("error names %v; want the first refused crate, sorted", err)
	}
}

// Verifies: FN-016.
func TestAllowlistChecksTheExactVersion_FN_016(t *testing.T) {
	allow := Allowlist{"serde": {"1.0.228": strings.Repeat("a", 64)}}
	err := checkAllowlist([]byte(metadataJSON), built("serde 1.0.229"), map[string]string{"serde 1.0.229": strings.Repeat("a", 64)}, allow)
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
}

// Verifies: FN-016.
// A listed name and version is not enough: the archive the build uses must be
// the one crates.io publishes, which the allowlist pins. This is the attack of
// build log #159: a modified copy of an allowlisted crate.
func TestAllowlistRefusesAModifiedArchiveOfAListedVersion_FN_016(t *testing.T) {
	allow, err := DefaultAllowlist()
	if err != nil {
		t.Fatal(err)
	}
	sums := pinned(t, allow, "serde 1.0.229")
	sums["serde 1.0.229"] = strings.Repeat("0", 64)
	err = checkAllowlist([]byte(metadataJSON), built("serde 1.0.229"), sums, allow)
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
	if !strings.Contains(err.Error(), "not the one crates.io publishes") {
		t.Errorf("error %v does not say the archive differs", err)
	}
	err = checkAllowlist([]byte(metadataJSON), built("serde 1.0.229"), map[string]string{}, allow)
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
}

// Verifies: FN-016.
// Build code from a path, here a procedural macro that takes the name and
// version of an allowlisted crate, is refused: only crates.io archives are
// pinned.
func TestAllowlistRefusesBuildCodeFromOutsideCratesIO_FN_016(t *testing.T) {
	allow, err := DefaultAllowlist()
	if err != nil {
		t.Fatal(err)
	}
	err = checkAllowlist([]byte(metadataJSON), built("quote 1.0.47"), pinned(t, allow, "quote 1.0.47"), allow)
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
	if !strings.Contains(err.Error(), "does not come from crates.io") {
		t.Errorf("error %v does not name the source", err)
	}
}

// Verifies: FN-016.
func TestAllowlistIgnoresPackagesTheBuildDoesNotCompile_FN_016(t *testing.T) {
	if err := checkAllowlist([]byte(metadataJSON), built("leftpad 1.0.0"), nil, Allowlist{}); err != nil {
		t.Errorf("an uncompiled build script was refused: %v", err)
	}
}

func TestAllowlistRejectsUnreadableMetadata(t *testing.T) {
	wantCode(t, checkAllowlist([]byte("{"), built("a 1"), nil, Allowlist{}), plxerr.FunctionBuildFailed)
}

// Verifies: FN-016.
// Every pin is the checksum sdk/fn/Cargo.lock records for that version, which
// Cargo took from the crates.io index; the list holds the crates the SDK needs.
func TestEmbeddedAllowlistIsPinnedToTheSDKLock_FN_016(t *testing.T) {
	allow, err := DefaultAllowlist()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"proc-macro2", "quote", "syn", "unicode-ident", "serde", "serde_core", "serde_derive",
		"serde_json", "schemars", "schemars_derive", "rust_decimal", "itoa", "memchr",
	} {
		if len(allow[name]) == 0 {
			t.Errorf("%s is missing from the allowlist", name)
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "sdk", "fn", "Cargo.lock"))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := parseLock(data)
	if err != nil {
		t.Fatal(err)
	}
	inLock := map[string]string{}
	for _, p := range lock {
		inLock[p.key()] = p.Checksum
	}
	for name, versions := range allow {
		for version, sum := range versions {
			if inLock[name+" "+version] != sum {
				t.Errorf("%s %s is pinned to %s; sdk/fn/Cargo.lock records %q", name, version, sum, inLock[name+" "+version])
			}
		}
	}
	for _, bad := range []string{`[`, `{"serde": ["1.0.229"]}`, `{"serde": {"1.0.229": "c1"}}`} {
		if _, err := ParseAllowlist([]byte(bad)); err == nil {
			t.Errorf("ParseAllowlist accepted %s", bad)
		}
	}
}

func TestBuiltPackagesParsesCargoTree(t *testing.T) {
	got, err := builtPackages([]byte("plux-module v0.1.0 (/ws/plux-module)\nserde v1.0.229\nserde v1.0.229 (*)\nsyn v2.0.119 (proc-macro)\n\n"))
	if err != nil || len(got) != 3 || !got["syn 2.0.119"] || !got["plux-module 0.1.0"] {
		t.Errorf("got %v, %v", got, err)
	}
	for _, bad := range []string{"", "garbage\n", "name only\n"} {
		if _, err := builtPackages([]byte(bad)); err == nil {
			t.Errorf("builtPackages(%q) accepted bad input", bad)
		}
	}
}

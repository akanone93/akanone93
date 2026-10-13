// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// metadataJSON is a cargo metadata document with a build script, a procedural
// macro and a plain library.
const metadataJSON = `{"packages":[
 {"name":"serde","version":"1.0.229","targets":[{"kind":["lib"]},{"kind":["custom-build"]}]},
 {"name":"serde_derive","version":"1.0.229","targets":[{"kind":["proc-macro"]}]},
 {"name":"leftpad","version":"1.0.0","targets":[{"kind":["lib"]}]},
 {"name":"borsh","version":"1.8.1","targets":[{"kind":["custom-build"]}]},
 {"name":"sneaky","version":"2.0.0","targets":[{"kind":["proc-macro"]}]}
]}`

func built(keys ...string) map[string]bool {
	m := map[string]bool{}
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// Verifies: FN-016.
func TestAllowlistRefusesUnlistedBuildCode_FN_016(t *testing.T) {
	allow, err := DefaultAllowlist()
	if err != nil {
		t.Fatal(err)
	}
	ok := built("serde 1.0.229", "serde_derive 1.0.229", "leftpad 1.0.0")
	if err := checkAllowlist([]byte(metadataJSON), ok, allow); err != nil {
		t.Errorf("allowed set refused: %v", err)
	}
	err = checkAllowlist([]byte(metadataJSON), built("serde 1.0.229", "sneaky 2.0.0", "borsh 1.8.1"), allow)
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
	if !strings.Contains(err.Error(), "borsh 1.8.1") {
		t.Errorf("error names %v; want the first refused crate, sorted", err)
	}
}

// Verifies: FN-016.
func TestAllowlistChecksTheExactVersion_FN_016(t *testing.T) {
	allow := Allowlist{"serde": {"1.0.228"}}
	err := checkAllowlist([]byte(metadataJSON), built("serde 1.0.229"), allow)
	wantCode(t, err, plxerr.FunctionBuildScriptRefused)
}

// Verifies: FN-016.
func TestAllowlistIgnoresPackagesTheBuildDoesNotCompile_FN_016(t *testing.T) {
	if err := checkAllowlist([]byte(metadataJSON), built("leftpad 1.0.0"), Allowlist{}); err != nil {
		t.Errorf("an uncompiled build script was refused: %v", err)
	}
}

func TestAllowlistRejectsUnreadableMetadata(t *testing.T) {
	wantCode(t, checkAllowlist([]byte("{"), built("a 1"), Allowlist{}), plxerr.FunctionBuildFailed)
}

func TestEmbeddedAllowlistIsSeededFromTheSDKLock(t *testing.T) {
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
	if _, err := ParseAllowlist([]byte("[")); err == nil {
		t.Error("ParseAllowlist accepted malformed JSON")
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

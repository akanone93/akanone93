// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"strings"
	"testing"
)

func TestTomlStringFieldsReadsOnlyTheNamedTable(t *testing.T) {
	manifest := []byte(`# comment
[package]
name = "loan-calc"   # trailing
version = "0.1.0"
edition.workspace = true

[dependencies]
name = "not-the-package"
`)
	got := tomlStringFields(manifest, "package")
	if got["name"] != "loan-calc" || got["version"] != "0.1.0" || len(got) != 2 {
		t.Errorf("fields = %v", got)
	}
}

func TestDeclaresWorkspace(t *testing.T) {
	for in, want := range map[string]bool{
		"[workspace]\n":                true,
		"  [workspace.dependencies]\n": true,
		"[package]\nname = \"x\"\n":    false,
		"# [workspace]\n":              false,
	} {
		if got := declaresWorkspace([]byte(in)); got != want {
			t.Errorf("declaresWorkspace(%q) = %v", in, got)
		}
	}
}

func TestParseLock(t *testing.T) {
	pkgs, err := parseLock([]byte(lockText(serdeLock, lockPackage{Name: "loan", Version: "0.1.0"}) + "\n[metadata]\nx = \"y\"\n"))
	if err != nil || len(pkgs) != 2 || pkgs[0] != serdeLock || pkgs[1].registry() {
		t.Fatalf("pkgs = %+v, err = %v", pkgs, err)
	}
	for _, bad := range []string{"", "version = 4\n", "[[package]]\nname = \"x\"\n"} {
		if _, err := parseLock([]byte(bad)); err == nil {
			t.Errorf("parseLock(%q) accepted a malformed lock", bad)
		}
	}
}

func TestDependenciesAreSorted(t *testing.T) {
	deps := dependenciesOf([]lockPackage{
		{Name: "zeta", Version: "1.0.0", Source: cratesIOSource, Checksum: "z"},
		{Name: "alpha", Version: "2.0.0", Source: cratesIOSource, Checksum: "a2"},
		{Name: "alpha", Version: "1.0.0", Source: cratesIOSource, Checksum: "a1"},
		{Name: "path-crate", Version: "0.1.0"},
	})
	var got []string
	for _, d := range deps {
		got = append(got, d.Name+"@"+d.Version)
	}
	if strings.Join(got, ",") != "alpha@1.0.0,alpha@2.0.0,zeta@1.0.0" {
		t.Errorf("deps = %v", got)
	}
}

// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"reflect"
	"sort"
	"testing"
)

// Verifies: FN-016, FN-019.
func TestBuildEnvIsCleanAndDeterministic_FN_019(t *testing.T) {
	parent := map[string]string{
		"PATH": "/usr/bin", "HOME": "/home/dev", "RUSTUP_HOME": "/opt/rustup",
		"RUSTUP_TOOLCHAIN": "1.97.0", "AWS_SECRET_ACCESS_KEY": "s3cret", "CARGO_REGISTRY_TOKEN": "t",
	}
	p := envPaths{Workspace: "/tmp/b/ws", CargoHome: "/tmp/b/cargo", Home: "/tmp/b/home", RustupHome: "/opt/rustup"}
	env := buildEnv(func(k string) string { return parent[k] }, p)
	want := []string{
		"CARGO_HOME=/tmp/b/cargo",
		"CARGO_INCREMENTAL=0",
		"CARGO_NET_OFFLINE=true",
		"CARGO_TERM_COLOR=never",
		"HOME=/tmp/b/home",
		"LC_ALL=C",
		"PATH=/usr/bin",
		"RUSTFLAGS=--remap-path-prefix=/tmp/b/ws=/src --remap-path-prefix=/tmp/b/cargo=/cargo --remap-path-prefix=/opt/rustup=/rustup",
		"RUSTUP_HOME=/opt/rustup",
		"RUSTUP_TOOLCHAIN=1.97.0",
		"SOURCE_DATE_EPOCH=0",
		"TZ=UTC",
	}
	if !sort.StringsAreSorted(env) || !reflect.DeepEqual(env, want) {
		t.Errorf("env =\n%q\nwant\n%q", env, want)
	}
}

func TestRustupHomeFallsBackToTheHomeDirectory(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := rustupHome(get(map[string]string{"HOME": "/h"})); got != "/h/.rustup" {
		t.Errorf("got %q", got)
	}
	if got := rustupHome(get(nil)); got != "" {
		t.Errorf("got %q", got)
	}
	env := buildEnv(get(map[string]string{"PATH": "/p"}), envPaths{Workspace: "/w", CargoHome: "/c", Home: "/h"})
	for _, e := range env {
		if e == "RUSTUP_HOME=" || e == "RUSTUP_TOOLCHAIN=" {
			t.Errorf("empty variable %q passed through", e)
		}
	}
}

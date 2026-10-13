// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"path/filepath"
	"sort"
)

// envPaths are the directories a build uses; they are remapped in the module
// and in the diagnostics so that no host path survives (FN-019).
type envPaths struct {
	// Workspace is the build workspace, remapped to /src.
	Workspace string
	// CargoHome is the empty, private cargo home, remapped to /cargo.
	CargoHome string
	// RustupHome is the toolchain directory, remapped to /rustup.
	RustupHome string
	// Home is the private home directory of the child.
	Home string
}

// rustupHome returns the toolchain directory: the one in the parent's
// environment, or rustup's default under the parent's home directory.
func rustupHome(getenv func(string) string) string {
	if v := getenv("RUSTUP_HOME"); v != "" {
		return v
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".rustup")
	}
	return ""
}

// remapFlags is the RUSTFLAGS value that keeps host paths out of the module
// (E7, FN-019).
func remapFlags(p envPaths) string {
	flags := "--remap-path-prefix=" + p.Workspace + "=/src" +
		" --remap-path-prefix=" + p.CargoHome + "=/cargo"
	if p.RustupHome != "" {
		flags += " --remap-path-prefix=" + p.RustupHome + "=/rustup"
	}
	return flags
}

// buildEnv returns the complete environment of every child: nothing is
// inherited except the search path and the toolchain location (FN-016, FN-019).
// The result is sorted, so it is the same for the same inputs.
func buildEnv(getenv func(string) string, p envPaths) []string {
	env := map[string]string{
		"PATH":              getenv("PATH"),
		"HOME":              p.Home,
		"CARGO_HOME":        p.CargoHome,
		"CARGO_NET_OFFLINE": "true",
		"CARGO_INCREMENTAL": "0",
		"CARGO_TERM_COLOR":  "never",
		"SOURCE_DATE_EPOCH": "0",
		"RUSTFLAGS":         remapFlags(p),
		"LC_ALL":            "C",
		"TZ":                "UTC",
	}
	if p.RustupHome != "" {
		env["RUSTUP_HOME"] = p.RustupHome
	}
	if t := getenv("RUSTUP_TOOLCHAIN"); t != "" {
		env["RUSTUP_TOOLCHAIN"] = t
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

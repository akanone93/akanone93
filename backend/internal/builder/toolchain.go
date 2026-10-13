// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The pinned tools (FN-001, plan P7 E7 and E10).
const (
	// RustVersion is the pinned Rust toolchain, as in sdk/fn/rust-toolchain.toml.
	RustVersion = "1.97.0"
	// WasmOptVersion is the pinned Binaryen release, as the provenance records it.
	WasmOptVersion = "version_132"
	// rustcPrefix is how `rustc --version` starts for the pinned toolchain.
	rustcPrefix = "rustc " + RustVersion + " "
	// wasmOptMarker is what `wasm-opt --version` prints for the pinned release.
	wasmOptMarker = "version 132"
)

// DefaultWasmOpt returns the command prefix that runs the pinned wasm-opt: the
// pinned Bun running the npm package binaryen@132.0.0 installed in dir (E10).
func DefaultWasmOpt(dir string) []string {
	return []string{"bun", dir + "/node_modules/binaryen/bin/wasm-opt"}
}

// checkRustc runs `rustc --version` and returns the full version line; any
// other toolchain than the pinned one is PLX-7200 (FN-001).
func (r *run) checkRustc(ctx context.Context) (string, error) {
	res, err := r.exec(ctx, r.b.cfg.Rustc, []string{"--version"})
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(res.Stdout))
	if res.ExitCode != 0 || !strings.HasPrefix(line+" ", rustcPrefix) {
		return "", plxerr.New(plxerr.FunctionBuildFailed,
			"the toolchain must be rustc %s; found %q", RustVersion, line)
	}
	return line, nil
}

// checkWasmOpt runs `wasm-opt --version` and requires the pinned release.
func (r *run) checkWasmOpt(ctx context.Context) error {
	prefix := r.b.cfg.WasmOpt
	res, err := r.exec(ctx, prefix[0], append(append([]string{}, prefix[1:]...), "--version"))
	if err != nil {
		return err
	}
	out := strings.TrimSpace(string(res.Stdout))
	if res.ExitCode != 0 || !strings.Contains(out, wasmOptMarker) {
		return plxerr.New(plxerr.FunctionBuildFailed,
			"wasm-opt must be Binaryen %s; found %q", WasmOptVersion, out)
	}
	return nil
}

#!/bin/sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Builds the ABI conformance probe (FN-015): ../probe.wasm with every
# import of plux_v1, and ../probe-device.wasm with the device feature,
# which omits the server-only imports. The build is deterministic for the
# pinned toolchain (Rust 1.97.0, target wasm32-wasip1): paths are remapped
# and the target directory is private. With --check the modules are rebuilt
# and compared with the committed ones.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
out=$(cd "$here/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

sysroot=$(rustc --print sysroot)
export RUSTFLAGS="--remap-path-prefix=$here=/probe --remap-path-prefix=$sysroot=/rustc --remap-path-prefix=${CARGO_HOME:-$HOME/.cargo}=/cargo"
export CARGO_TARGET_DIR="$work/target"
export SOURCE_DATE_EPOCH=0

build() { # name [cargo args...]
	name=$1
	shift
	cargo build --release --locked --offline --target wasm32-wasip1 \
		--manifest-path "$here/Cargo.toml" "$@"
	cp "$work/target/wasm32-wasip1/release/plux_fn_probe.wasm" "$work/$name"
}

build probe.wasm
build probe-device.wasm --features device

if [ "${1:-}" = "--check" ]; then
	cmp "$work/probe.wasm" "$out/probe.wasm"
	cmp "$work/probe-device.wasm" "$out/probe-device.wasm"
else
	cp "$work/probe.wasm" "$work/probe-device.wasm" "$out/"
fi

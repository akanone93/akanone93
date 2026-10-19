#!/bin/sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Builds the Dart host's fixtures: loan_module.wasm and fee_module.wasm from
# the SDK's example modules (sdk/fn), and probe.wasm from src/probe. The
# build is deterministic for the pinned toolchain (Rust 1.97.0, target
# wasm32-wasip1): paths are remapped and the target directory is private, so
# the bytes do not depend on where the repository or cargo's home are. With
# --check the modules are rebuilt and compared with the committed ones.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

export RUSTUP_TOOLCHAIN=1.97.0
sysroot=$(rustc --print sysroot)
export RUSTFLAGS="--remap-path-prefix=$root=/plux --remap-path-prefix=$sysroot=/rustc --remap-path-prefix=${CARGO_HOME:-$HOME/.cargo}=/cargo"
export CARGO_TARGET_DIR="$work/target"
export SOURCE_DATE_EPOCH=0
release="$work/target/wasm32-wasip1/release"

cargo build --release --locked --target wasm32-wasip1 \
	--manifest-path "$root/sdk/fn/Cargo.toml" -p loan-module -p fee-module
cp "$release/loan_module.wasm" "$release/fee_module.wasm" "$work/"
cargo build --release --locked --target wasm32-wasip1 \
	--manifest-path "$here/src/probe/Cargo.toml"
cp "$release/probe.wasm" "$work/"

for name in loan_module.wasm fee_module.wasm probe.wasm; do
	if [ "${1:-}" = "--check" ]; then
		cmp "$work/$name" "$here/$name"
	else
		cp "$work/$name" "$here/"
	fi
done

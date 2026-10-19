<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Test fixtures

The `.wasm` files are built, not written: `sh build.sh` rebuilds `loan_module.wasm` and
`fee_module.wasm` from the SDK's example modules (`sdk/fn`) and `probe.wasm` from `src/probe`
with Rust 1.97.0, paths remapped so the bytes do not depend on the machine; `sh build.sh --check`
rebuilds and compares them, as CI does in `make wasm-check`. `src/refusals.py` prints the hand-assembled
modules `refusals_test.dart` embeds.

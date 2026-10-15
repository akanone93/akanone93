<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Test fixtures

The `.wasm` files are built, not written: `loan_module.wasm` and `fee_module.wasm` with
`cd sdk/fn && cargo build --release --target wasm32-wasip1 -p loan-module -p fee-module`, and
`probe.wasm` with `cd src/probe && cargo +1.97.0 build --release --target wasm32-wasip1`; copy each
`.wasm` from its `target/wasm32-wasip1/release/` here. `src/refusals.py` prints the hand-assembled
modules `refusals_test.dart` embeds.

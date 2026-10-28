<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Functions

Plux Functions are Rust compiled to WebAssembly that run on the server or on the device. Authoring
and deploying are done from the command line (`plux fn new`, `build`, `test`, `deploy`; see the
[functions guide](../functions.md)); Studio is where you inspect and operate them.

## Find and inspect a function

1. Open **Functions** in the app's navigation and pick the environment. The list shows each
   function's latest version, placement (**the server** or **the device**), test results and alias.
2. Open a function. **Versions** lists each deployed version with its **Assurance**, whether it was
   **Signed by the builder**, the **Source hash**, **Module hash**, **Toolchain**, **Builder**,
   **Dependencies** and the **Input schema** and **Output schema**.
3. **Capabilities** shows what the version is **Granted**: network domains, key-value store, secrets
   it may read, functions it may call and user claims. A version that recorded no declaration is
   granted nothing. See [Function security](../function-security.md).
4. **Tests** shows the stored cases and their results. **Metrics** shows invocations, latency p50,
   p95 and p99, memory peak, cache hits and errors by kind. **Logs** shows log lines by time and level.

## Move an alias

Choose **Point alias at this version** to make the alias serve that version. The alias moves; the
versions stay.

## Edit Rust source

**Edit in the Rust editor** opens the uploaded source with a starter template, the crate's
**Dependencies**, **Run the tests** and the **Build output**. **Start the language server** gives
completion, hints and diagnostics; it runs in the function build sandbox, with a time limit, so
**Start the language server** again after it stops. You can read the source without being able to
change it. Deploy with `plux fn deploy`.

If functions fail in production: [Function outage](../../runbooks/function-outage.md),
[Function egress refusals](../../runbooks/function-egress-refusals.md) and
[Function builder failure](../../runbooks/function-builder-failure.md).

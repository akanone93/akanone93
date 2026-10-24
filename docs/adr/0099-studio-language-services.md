<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0099. Studio language services: Wasm validator and the Rust editor bridge

- **Status:** **Accepted** (maintainer, 2026-10-24; P11 plan M-9)
- **Date:** 2026-10-24
- **Requirements:** `STU-064`, `STU-110`, `FN-033`, `PXL-007`

## Context and problem

Studio shows diagnostics while the user types, in the JSON editor (`STU-064`), in PXL expressions
(`PXL-007`) and in the Rust editor for functions (`FN-033`, opened from `STU-110`). Two questions:
where does validation run, and how does a browser editor talk to rust-analyzer, which executes
build scripts and procedural macros, that is, arbitrary user code. See Studio decisions A4.7, A5.3
([the stack decisions](../plans/p11-studio-decisions.md)) and contract #6 and L-12 in
[the P11 plan](../plans/p11.md).

## Decision drivers

- One validator, so Studio and the server never disagree (security and correctness).
- Feedback within a keystroke, also on a slow or dropped connection.
- The server stays the authority on save and publish.
- Untrusted code from a user's crate must never run with credentials or network (L-4, `SEC-054`).
- Ordered edits over a transport that cannot be client-streamed from a browser.

## Considered options

1. Compile the Go validator and the PXL language service to WebAssembly, in one worker; run rust-analyzer in the build sandbox behind a Connect bridge.
2. Reimplement validation in TypeScript.
3. Validate on the server only.
4. Run rust-analyzer in the browser, or on the server without a sandbox.

## Decision

Chosen option: **1**.

### The Wasm module (A4.7, L-12)

- The real Go validator (`compiler.NewValidator`, `ValidatePage`) and the PXL language service are
  compiled with `GOOS=js GOARCH=wasm` into **one module in one Web Worker**, loaded only when an
  editor opens and then cached. It measured 11.8 MiB raw and 3.1 MiB gzip.
- `ValidatePage` runs **per edit** on the edited page and its graphs; **project-wide** validation
  runs once when a project opens. Edits go to the worker as one batched message.
- **The server validates again** on save and publish and remains the authority. A disagreement is
  a bug in one of them, found by the shared conformance vectors.
- **The importer and collection-migration planning join the module** (L-12), so previews of OpenAPI
  imports and migrations need no new server RPC.
- Speed-ups, if the prototype (S0.8) needs them, in this order: batched messages, splitting
  validation from bundle encoding, TinyGo, and only then moving project-wide checks to the server.

### The Rust editor bridge (A5.3; contract #6)

- **Transport.** LSP JSON-RPC over Connect: `LanguageServerService.Open` is a server stream per
  editor session (diagnostics, progress, responses) and `Send` is a unary, **sequenced** call
  (ordered edits; HTTP/2 multiplexing keeps keystroke latency low). The browser uses a standard LSP
  client with a small custom transport. Streams follow [ADR-0096](0096-studio-live-channels.md).
- **Sandbox.** rust-analyzer runs inside the function **build sandbox** of
  [ADR-0064](0064-function-isolation.md): **no database credentials or signing capability** (L-4),
  **build scripts and procedural macros off**, **no network**, CPU, memory and time limits, and
  **one sandbox per session**.
- **Limits.** `lsp.sessionsPerOrganization` caps concurrent sessions and `lsp.sessionMinutes` ends
  an idle or long session. Both come from the registry.

## Consequences

- **Positive:** diagnostics at typing speed and offline; one validator; user code stays contained.
- **Negative:** a large module to download (once, cached) and a slower first start; two language
  services in one build to keep in step with the compiler; sandboxes cost CPU and memory per
  session; without macros, some crates show reduced diagnostics.
- **Follow-up:** the prototype's measurements (S0.8) and conformance vectors for both validators;
  the limit keys; the sandbox's resource profile.

## Options in detail

### Option 2 — TypeScript reimplementation

Fast to start, but two implementations drift, and Studio would accept or reject what the server
decides otherwise. Rejected.

### Option 3 — server only

Latency on every keystroke and no feedback offline. Round trips remain for the authority checks.
Rejected for per-edit checks.

### Option 4 — in the browser, or unsandboxed

rust-analyzer in the browser is not practical at this size; on the server without the sandbox it
would run arbitrary code beside the services that hold credentials. Rejected.

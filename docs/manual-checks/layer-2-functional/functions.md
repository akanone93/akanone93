<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Functions

You write a Plux Function in Rust, test and build it locally, deploy it to the server's sandboxed
builder, call it from a screen on the server and on the device, see what the egress proxy refuses and
read its logs and metrics. Guides: [Functions](../../guides/functions.md),
[Function security](../../guides/function-security.md), [Studio functions](../../guides/studio/functions.md).

You need the Rust toolchain with the `wasm32-wasip1` target, and the stack started with the builder:
`COMPOSE_PROFILES=functions make compose-up`.

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-FUNCTION | Function `shop.fee` in R-PROJECT, deployed to `development` | L2-FN-001, L2-FN-006 |
| R-FN-DEVICE | Function `shop.preview`, placement `device` | L2-FN-010 |

### L2-FN-001 — Scaffold a function: create R-FUNCTION

- **Needs:** R-CLI, R-PROJECT
- **Do:** `cd ~/plux-checks/shop; plux fn new shop.fee --placement server; ls -R functions/shop.fee`
- **Expect:** `Cargo.toml`, `function.json` (with a stored test case), `src/lib.rs`, `.cargo/config.toml`, `.gitignore`.
- **Why:** one crate, one document; the document is the source of the name, placement and capabilities (FN-001, FN-004).
- **Look at:** `function.json` has a stable UUIDv7 `id`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-002 — Test natively and on the function host

- **Needs:** R-FUNCTION
- **Do:** `plux fn test shop.fee; echo $?`; change the expected output in `function.json` and run again.
- **Expect:** `cargo test`, then the stored case on the host: pass; the wrong expectation fails with exit 1 and shows the difference.
- **Why:** tests run before anything is deployed (FN-030).
- **Look at:** `--skip-native` skips only the first step.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-003 — Build locally and prove it is reproducible

- **Needs:** R-FUNCTION
- **Do:** `plux fn build shop.fee --verify`; `ls functions/shop.fee/target/plux`.
- **Expect:** a warning that this build has no sandbox; the module, schemas and provenance are written; `--verify` builds twice and the modules are identical.
- **Why:** reproducible, inspectable artifacts (FN-016, FN-019).
- **Look at:** `make rust-e2e` does the same with the server's builder role.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-004 — Invoke locally

- **Needs:** L2-FN-003
- **Do:** `plux fn invoke shop.fee --input '{"name":"Ada"}'`; then `--input '{"nme":1}'`.
- **Expect:** the greeting `Hello, Ada!`; the wrong input is rejected by the input schema, not by a crash.
- **Why:** typed schemas come from the Rust types (FN-005).
- **Look at:** the input and output schemas in `target/plux/`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-005 — Capabilities are denied by default

- **Needs:** R-FUNCTION
- **Do:** in `src/lib.rs` call an HTTP request to `https://example.com` without declaring it; build, invoke.
- **Expect:** the module gets a denial that the SDK reports as `PLX-7003`.
- **Why:** deny by default (FN-012, SEC-080).
- **Look at:** add `"capabilities":{"http":["example.com"]}` and see `PLX-7003` go away locally; the server's egress still decides in L2-FN-009.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-006 — Deploy to the server

- **Needs:** R-FUNCTION, the stack with the `functions` profile
- **Do:** `plux fn deploy shop.fee --env development --alias stable; plux fn versions shop.fee --env development`
- **Expect:** the server builds the source in the builder role, runs the stored tests and deploys version 1 with the alias `stable`.
- **Why:** only server builds are deployable, signed by the builder (FN-009, FN-016).
- **Look at:** `docker compose -f deploy/compose/compose.yaml logs plux-builder` shows the build; the builder has no database access.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-007 — Inspect the version in Studio

- **Needs:** L2-FN-006, R-STUDIO
- **Do:** in Studio open **Functions** of `shop`, `development`; open `shop.fee`; read **Versions**, **Capabilities**, **Tests**, **Metrics**, **Logs**.
- **Expect:** **Signed by the builder**, **Source hash**, **Module hash**, **Toolchain**, **Input schema**; **Capabilities** shows "Granted" nothing; the stored test passes.
- **Why:** provenance is visible (FN-006, STU-120).
- **Look at:** the module hash equals the one in `plux fn build`'s provenance only if toolchain and sources match.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-008 — Call it from a screen

- **Needs:** R-FUNCTION, R-SCREENS, R-DEVICE-SHOP (or R-SESSION)
- **Do:**
  1. In Studio add the function to the plugin's grants and an `invokeFunction` step to the button's action graph, input `{"name":"Ada"}`, result into state.
  2. Publish to `development` (as L2-STU-039).
  3. Tap the button on the device.
- **Expect:** the state shows `Hello, Ada!`; the compiler refuses an unknown name or wrong input type (`PLX-7031`, `PLX-7032`).
- **Why:** functions are typed steps in a graph (FN-020).
- **Look at:** `plux_function_invocations_total` in Prometheus; **Process flow** shows the call.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-009 — Egress allowlist

- **Needs:** R-FUNCTION
- **Do:**
  1. Declare `capabilities.http: ["example.com"]`, make the function call it, deploy, invoke.
  2. Make it call a host that is not declared; deploy and invoke again.
- **Expect:** the declared host works through the egress proxy (or fails only because the sandbox has no outside network); the undeclared one is refused (`PLX-7003` or `PLX-7010`).
- **Why:** the egress proxy is the only way out (FN-017, SEC-163).
- **Look at:** `docker compose … logs plux-egress` and `plux_egress_tunnels_total`; [egress runbook](../../runbooks/function-egress-refusals.md).
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-010 — A device function: create R-FN-DEVICE

- **Needs:** R-CLI, R-PROJECT
- **Do:** `plux fn new shop.preview --placement device; plux fn test shop.preview`; try adding `"http":["example.com"]` to its capabilities and validate.
- **Expect:** tests run natively, on the host and on the device-engine stand-in with the same result; the `http` declaration is a compile error (`PLX-7030`).
- **Why:** device functions have no network, secrets or claims (FN-052).
- **Look at:** `plux validate ~/plux-checks/shop` prints the code.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-011 — Run the device function on the device

- **Needs:** R-FN-DEVICE, R-DEVICE-SHOP with `plux_functions`
- **Do:** grant it to a plugin, call it from a button, publish, tap on the device; also try with the device in airplane mode.
- **Expect:** the result appears on the device with no network; an app without `plux_functions` fails the step with `PLX-5401`.
- **Why:** offline logic runs in the WAMR interpreter (FN-050, FN-052).
- **Look at:** the **Functions** tab of the device drawer shows placement `device`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-012 — Versions and aliases

- **Needs:** L2-FN-006
- **Do:** change the output text, `plux fn deploy shop.fee --env development`; `plux fn alias shop.fee stable 2 --env development`; `plux fn versions shop.fee --env development`.
- **Expect:** version 2 is new; `stable` moves to 2; version 1 stays. In Studio **Point alias at this version** moves it back.
- **Why:** versions are immutable, aliases move (FN-006).
- **Look at:** aliases are per environment.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-013 — Promote without rebuilding

- **Needs:** L2-FN-006, an environment `staging`
- **Do:** `plux fn promote shop.fee@2 --from development --to staging`; compare the module hash in Studio for both environments.
- **Expect:** same number, same module hash and signature.
- **Why:** what was tested is what runs (FN-006).
- **Look at:** a release pinning the function cannot be promoted to `staging` before the function is (try `plux release promote`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-014 — Logs

- **Needs:** L2-FN-008
- **Do:** make the function log a line; invoke it; `plux fn logs shop.fee --since 15m -n 50`; and with `--follow` while tapping the button.
- **Expect:** lines oldest first, then new lines as they arrive; the same lines in Studio **Logs**.
- **Why:** function output is observable (FN-032).
- **Look at:** log lines never contain secrets.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-015 — Limits

- **Needs:** R-FUNCTION
- **Do:** write an endless loop, deploy to `development`, invoke; then allocate beyond the memory limit.
- **Expect:** the call ends with `PLX-7001` (time) or `PLX-7002` (memory) in about the limit time; the runner stays healthy.
- **Why:** a function cannot take the server down (FN-011, LIM-001).
- **Look at:** `plux_function_duration_seconds` and the memory peak in Studio **Metrics**.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-016 — Production needs more

- **Needs:** L2-FN-006, a `production` environment
- **Do:** `plux fn deploy shop.fee --env production` with a function whose stored test fails.
- **Expect:** refused (`PLX-7020`); a passing function needs the production deploy permission, a second factor, and any approval policy (see governance).
- **Why:** production is guarded (FN-007, FN-031).
- **Look at:** the sign-in prompt for step-up.
- **Result:** ☐ pass ☐ fail — notes:

### L2-FN-017 — Function isolation

- **Needs:** the stack
- **Do:** `docker compose -f deploy/compose/compose.yaml exec plux-fnrunner env | grep -i -E 'database|secret|token|vault'`
- **Expect:** nothing; the runner has no database URL, key or token. `make rust-check` and `make wasm-check` pass.
- **Why:** layering rule L-4 (SEC-080, FN-010).
- **Look at:** the `functions` network is `internal` (no route out); egress is the only way.
- **Result:** ☐ pass ☐ fail — notes:

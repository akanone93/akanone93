<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Functions Guide

How to write, test, deploy and promote a Plux Function: Rust compiled to WebAssembly, run on
the server or on the device. Logic that declarative actions and PXL bindings cannot express (a
loan schedule, a fee engine, an eligibility check) goes here. The design is spec §16 and
[ADR-0011](../adr/0011-plux-functions-rust-wasm.md); the contract between a module and its host is
the [ABI reference](../functions/abi.md); how a build becomes a signed artifact is the [builds
guide](function-builds.md); what keeps a function inside its box is the [security
guide](function-security.md).

## 1. Choose a placement

A function declares exactly one placement (`FN-008`).

| Placement | Runs in | Use it for | Trust |
|---|---|---|---|
| `server` | `fnrunner`, on `wazero` | Results the backend relies on, secrets, calls to external APIs | Authoritative |
| `device` | WAMR, in interpreter mode, in the optional package `plux_functions` | Offline and low-latency logic: calculators, previews, validation, formatting | For display only. The server never accepts a device result as evidence (`FN-054`) |

The same code can serve both: a flow that needs a trusted answer calls a server function that
runs the same calculation the device used for its preview.

A device function sees only its input, `log`, the clock and the plugin's key-value store
(`FN-052`). It has no network, no secrets, no claims. An app that does not include
`plux_functions` fails a device-placed `invokeFunction` with `PLX-5401`. Under the `maximum`
security profile device placement is off by default (`deviceFunctions`, `FN-055`), and enabling it
needs a dedicated permission with step-up authentication.

## 2. Write a function

A function is one Rust crate in `functions/<name>/` of your project, beside its document
`function.json`. The name is dotted, like `loan.calculateSchedule`: lower camel case parts
separated by dots.

```sh
plux fn new bank.transferFee --placement server
```

creates:

```text
functions/bank.transferFee/
  Cargo.toml
  function.json
  src/lib.rs
  .cargo/config.toml     # points plux-fn at the SDK on this machine; not committed
  .gitignore
```

`--placement` is `server` (the default) or `device`. `--sdk dir` names the pinned `plux-fn` crate
when `$PLUX_SDK` and the copy next to the binary do not apply.

### The handler

A handler is a plain function from a context and a typed input to a typed output or a business
error. The context type fixes the placement.

```rust
use plux_fn::{BusinessError, ServerContext};
use rust_decimal::Decimal;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

#[derive(Deserialize, JsonSchema)]
pub struct Input {
    amount: Decimal,
    currency: String,
}

#[derive(Serialize, JsonSchema)]
pub struct Output {
    fee: Decimal,
}

fn handler(ctx: &ServerContext, input: Input) -> Result<Output, BusinessError> {
    if input.amount <= Decimal::ZERO {
        return Err(BusinessError::new("AMOUNT_INVALID", "bank.transferFee.amountInvalid")
            .with_param("min", 1));
    }
    ctx.info("pricing a transfer");
    Ok(Output { fee: price(&input) })
}

plux_fn::function!(handler);
```

- `plux_fn::function!(handler)` exports the handler from its crate. That is all you write: you do
  not write exports, memory management or a main function.
- The macro `plux_fn::module!` is not yours to call. The builder generates a wrapper crate,
  `plux-module`, that lists your crate and emits the ABI exports with a dispatch table. It checks
  that the placement your handler's context fixes equals the one in `function.json`; a mismatch
  fails the build (`PLX-7205`).
- A handler takes `&DeviceContext` or `&ServerContext`. A device handler cannot name `http`,
  `secret`, `claims` or `invoke`: the code does not compile (`FN-002`, `FN-008`).
- The input must implement `Deserialize` and `JsonSchema`; the output `Serialize` and
  `JsonSchema`. The build derives the JSON Schemas from these types and publishes them with
  the function (`FN-004`). Studio binds inputs and outputs with them, and the compiler checks
  every `invokeFunction` step against them (`PLX-7032`). You never write the schemas by hand.
- Numbers travel as JSON text. Use a decimal type such as `rust_decimal` for money.
- Any crate that compiles for `wasm32-wasip1` may be a dependency, with one condition: a crate
  that has a build script or is a procedural macro must be on the builder's allowlist, or the
  build fails with `PLX-7202`. See the [builds guide](function-builds.md#4-allowlisted-build-scripts-and-procedural-macros).

### The context

Both contexts offer:

| Call | Does |
|---|---|
| `ctx.debug/info/warn/error(message)`, `ctx.log(level, message)` | Writes a log line; lines past `function.logBytes` are dropped with one marker |
| `ctx.now()` | The host clock in Unix nanoseconds |
| `ctx.app_id()`, `ctx.environment_id()`, `ctx.assurance_level()` | Where and for whom the call runs |
| `ctx.env(name)` | A variable the document lists in `env`; `None` when unset |
| `ctx.kv()` with `get`, `put`, `delete`, `get_json`, `put_json` | The key-value store, when `capabilities.kv` is true |

A `ServerContext` adds:

| Call | Does |
|---|---|
| `ctx.http().send(&HttpRequest::get(url))` | Outbound HTTP to an allowed domain, through the egress proxy |
| `ctx.secret(name)` | A secret granted by name |
| `ctx.claims()` | The verified claims of the caller (`SEC-026`) |
| `ctx.invoke(name, &input)` | Another granted server function; the callee's business error comes back as `InvokeError::Business` |

A call the host refuses returns a `HostError` that matches the ABI status: `NotFound`,
`CapabilityDenied`, `LimitExceeded`, `EgressRefused`, `UpstreamFailed`, `InvalidArgument`,
`Unavailable`. `HostError::trap()` ends the invocation as a system error.

### Business errors and system errors

A function reports a failure it expected as a **business error** (`FN-022`): a stable `code`
that matches `^[A-Z][A-Z0-9_]*$`, a `message_key` the app localises, and optional parameters. It
reaches the action graph as `PLX-7100`, carrying your code, and the graph can branch on it.
Everything else is a **system error**: a timeout (`PLX-7001`), a limit, a trap (`PLX-7007`), a
refused capability (`PLX-7003`). Users see a system error generically; the cause stays in the
logs. The full list is in the [error reference](../reference/errors.md).

## 3. Declare the function

`function.json` is the single source of the name, placement, capabilities, limits and tests
(`FunctionDocument` in the [document schema](../reference/document-schema.md)). The scaffold
writes this:

```json
{
  "schemaVersion": "1.0.0",
  "kind": "function",
  "id": "0198…",
  "name": "bank.transferFee",
  "placement": "server",
  "capabilities": {},
  "tests": [
    {"name": "greets by name", "input": {"name": "Ada"}, "output": {"greeting": "Hello, Ada!"}}
  ]
}
```

A function that calls an API, reads a secret and keeps a counter adds what it needs:

```json
{
  "capabilities": {
    "http": ["rates.example.com", "*.partner.example.net"],
    "secrets": ["ratesApiKey"],
    "kv": true,
    "functions": ["bank.roundAmount"],
    "claims": true
  },
  "env": ["feeTable"],
  "limits": {"timeoutMs": 1500, "httpCalls": 3},
  "assurance": "AL2",
  "attestation": false,
  "sca": false
}
```

- **Capabilities are deny by default** (`FN-012`). Whatever `capabilities` does not list returns
  `-2` to the module, which the SDK reports as `PLX-7003`.
- A device function may declare `kv` and `functions` that are themselves device-placed. A
  declaration of `http`, `secrets` or `claims` on a device function is a compile error
  (`PLX-7030`).
- `limits` can only tighten the registry values for this function (`FN-011`, `LIM-002`). The
  registry maximum wins over a larger value.
- `assurance`, `attestation` and `sca` say what a device must prove to invoke the function: an
  assurance level, a fresh attestation, a strong-customer-authentication signature (`FN-020`,
  `SEC-027`). The server enforces them on every call, so they do not depend on the app being
  honest. Without `assurance` the setting `defaultFunctionAssurance` applies.
- `pure: true` with `cacheTtlSeconds` lets the server cache results by input hash (`FN-014`).
- The `id` is a UUIDv7 that never changes. Renaming a function keeps its id.

## 4. Test it

```sh
plux fn test bank.transferFee
```

runs, in order (`FN-030`):

1. `cargo test` natively. The SDK ships a mock host (`plux_fn::testing`) with `TestServer` and
   `TestDevice`, where you set secrets, claims, key-value contents and scripted HTTP responses
   and read back the log and the requests. `--skip-native` leaves this step out.
2. A build of the module, then every stored case of `function.json` on the function host.
3. For a device function, the same cases again on an interpreter that stands in for the device
   engine. A difference between the engines fails the run.

A stored case is an input and either the expected `output` or the expected business `error`
code; a server function may also give `claims`. At most `function.testCases` (100 by default)
cases are stored per function. The scaffold's `every_stored_case_replays` test runs the same
cases under `cargo test`, so CI that only runs `cargo test` still checks them.

`plux fn build <name>` builds locally and writes the module, the schemas and the provenance to
`functions/<name>/target/plux/`. It runs without the server's sandbox and prints a warning to
say so: it is for development. `--verify` builds twice and fails unless the two modules are
byte-identical (`FN-019`). The server rebuilds from your source on every deploy; a module built
on your machine is never uploaded.

`plux fn invoke <name> --input '{"amount": 100, "currency": "EUR"}'` runs the built module under
the limits and capabilities of the document, with an in-memory key-value store. `--input @file`
reads the input from a file, `--input @-` from standard input; `--claims` gives a server function
its caller's claims. Calling a deployed function from a device is the runtime's job.

All `plux fn` commands accept `-C dir` for the project directory and `--json` for a result on
standard output.

## 5. Deploy

```sh
plux fn deploy bank.transferFee --env development
```

packs the crate with its vendored dependencies, uploads the source, waits for the server to
build and test it, and deploys the version. `--alias name` points an alias at the new version in
the same step. `--env` defaults to the environment in `plux.json`, then `development`. Add
`--app id` to name the app when the project does not.

The server builds the source itself, in the `builder` role, and signs the result. Only server
builds can be deployed (`FN-009`). What that means for provenance and reproducibility is in the
[builds guide](function-builds.md).

Deploying to **production** needs a dedicated permission, `function.deploy.production`, with a
second factor: sign in again with `plux login` when the server asks (`FN-007`). The approval
engine replaces this stand-in in P9.

### Stored tests and production

A stored case that fails does not stop a deploy to a development or staging environment: the
version is deployed and marked as failing its tests, so you can look at it. In a **production**
environment a function with failing stored tests cannot be deployed (`FN-031`, `PLX-7020`),
and a release that pins such a version cannot be promoted there.

## 6. Versions and aliases

```sh
plux fn versions bank.transferFee
plux fn alias bank.transferFee stable 7
plux fn logs bank.transferFee --version 7 --since 15m -n 200
```

- A **version** is an integer, assigned at deploy and immutable (`FN-006`). Version numbers belong
  to the app: version 7 of `bank.transferFee` names one module, with one SHA-256, in every
  environment it reaches.
- An **alias** is a name that points at a version, matching `^[a-z][a-z0-9-]*$` (`stable`,
  `canary`). `plux fn alias <name> <alias> <version>` moves it. A plugin grant may name an alias.
- `plux fn versions` lists versions newest first with their aliases, `plux fn logs` shows what a
  function logged, oldest first. Add `--follow` to wait for new lines. Logs are kept for the
  retention the operator configures.

Aliases are separate per environment: moving `stable` in staging does not move it in production.

## 7. Promote between environments

A version reaches another environment only by promotion:

```sh
plux fn promote bank.transferFee@7 --from staging --to production
```

Promotion copies the version, not the source: the module in production is byte-identical to the
one that was tested in staging, with the same SHA-256 and signature. Nothing is rebuilt.

What promotion does and does not carry:

| Carried | Stays per environment |
|---|---|
| The version number, module, schemas, provenance and signature | Secrets (the environment's server-held values, [ADR-0061](../adr/0061-server-held-configuration.md)) |
| The declared capabilities, including the HTTP domains of `function.json` | Environment variables, such as an API's base URL |
| | The key-value store |
| | Limits |
| | Aliases |

The egress allowlist is part of the version: the domains `function.json` declares are the same in
every environment. A function that calls a sandbox API in development and the real one in
production declares both domains and reads the base URL from an environment variable. After
promoting, set what the target environment needs: the secrets the function names, its environment
variables, its limits. A function that reads a secret that the environment does not have gets
`-1` from `secret_get`.

**Releases and functions.** A release records the name, version, module SHA-256 and placement
of every function its plugins reference, so its behaviour is reproducible (`FN-006`). A device
invocation sends the version the release pinned, and the server refuses a version no active
release of the app pins. Consequently:

- Promoting a release to an environment (`plux release promote`) is **refused until every function
  version it pins has been promoted there** with `plux fn promote`. Promote the functions first,
  then the release.
- A release that refers to a function that does not exist is refused when it is created
  (`PLX-7031`).

The usual order is: deploy to development, test, `plux fn promote … --to staging`, release to
staging, `plux fn promote … --to production`, `plux release promote --env production`.

## 8. Call it from an app

A plugin lists the functions it may call in its `functions` grants (`FunctionGrant`): the dotted
`function` name, an optional `alias` and an optional `requiresAssurance`. An action graph then
calls the function with the `invokeFunction` step, whose input and output are typed by the
function's schemas ([actions guide](actions.md)):

```json
{"id": "fee", "action": "invokeFunction",
 "input": {"function": "bank.transferFee",
           "input": {"amount": {"$expr": "params.amount"}, "currency": "EUR"}},
 "onSuccess": "show", "onError": "failed"}
```

The compiler resolves the name and checks the types (`PLX-7031`, `PLX-7032`). A server function
runs through `FunctionService.Invoke` with DPoP and a valid token (`FN-020`). The caller may send
an idempotency key: a retry with the same key and input within 24 hours returns the first result
without running the function again, and the same key with other input is `PLX-7011` (`FN-021`).
Rate limits apply per function, per app and per device (`FN-024`, `PLX-7015`).

See the [actions reference](../reference/actions.md#invokefunction) for the action itself.

## 9. Limits

Every invocation is bounded by the limits registry, never by a number in code (`LIM-001`).
Defaults for a server function: 2 s wall time, 64 MiB of memory, 1 MiB of input and output, 10
outbound HTTP calls, nested calls three deep. A device function defaults to 500 ms and 32 MiB.
The table with every key, default and maximum is in the [security guide](function-security.md#5-limits).
Exceeding time, memory, input or output ends the call with its own code (`PLX-7001`, `7002`,
`7022`, `7004`), never with an unexplained trap. The count limits (HTTP calls, nested calls,
key-value use) answer the module with status `-3`, and the function decides what to do.

## 10. What to look at when it fails

| You see | It means | Look at |
|---|---|---|
| `PLX-7003` | The module used a capability `function.json` does not declare | `capabilities` |
| `PLX-7010` | The egress allowlist refused the destination | [Egress refusals runbook](../runbooks/function-egress-refusals.md) |
| `PLX-7007` | The module trapped: a panic, an out-of-bounds access, a malformed result | `plux fn logs`; the panic message is logged at error level |
| `PLX-7001`, `PLX-7002` | Time or memory limit | `limits` in `function.json`; the work itself |
| `PLX-7200`–`7206` | The build failed | [Builder failure runbook](../runbooks/function-builder-failure.md) |
| `PLX-7012`, `PLX-7013` | No such function or alias in this environment | `plux fn versions --env …`; was it promoted? |
| `PLX-8091` | The server could not reach `fnrunner` | [Function outage runbook](../runbooks/function-outage.md) |

<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Function ABI conformance vectors

Language-neutral fixtures for the host ABI of Plux Functions (`FN-015`,
ADR-0062). Every function host runs them: the Go host on `wazero`
(`backend/internal/fnhost`) and, from F5, the Dart device host on WAMR. A host
that passes them implements the ABI the same way as every other host.

| File | Contents |
|---|---|
| `probe.wasm` | The probe module with every `plux_v1` import. |
| `probe-device.wasm` | The probe built with the `device` feature: no server-only imports. |
| `probe/` | The Rust crate both are built from. `probe/src/lib.rs` documents the entries by index. `sh probe/build.sh` rebuilds them (Rust 1.97.0, `wasm32-wasip1`); `--check` rebuilds and compares. The build is deterministic. |
| `abi-v1.json` | The cases. |

## The runner's environment

A runner supplies exactly this, so outputs are comparable byte for byte:

- the clock reads 2023-11-14T22:13:20Z (1 700 000 000 s); `clock_now` is
  `1700000000000000000` and the WASI wall clock agrees;
- `random_get` reads the bytes `00 01 02 03 …`, one counter for the engine;
- the largest memory any call may be given is 512 MiB (`huge-initial-memory`
  is refused because it declares more);
- a *fake host* serves the capabilities of a case from its `host` object
  (below).

## Format

```text
{ "version": 1,
  "defaults": { …a case without name and expect… },
  "modules":  { "<name>": {"file": "probe.wasm"} | {"hex": "<module bytes>"} },
  "cases":    [ { "name", …, "expect": {…} } ] }
```

A case is `defaults` overlaid with the case's own keys. `limits`, `context`,
`env`, `grants` and `host` merge key by key; every other key replaces.

| Key | Meaning |
|---|---|
| `module` | A key of `modules`. |
| `placement` | `server` or `device`: the placement the module is compiled for. |
| `index` | The entry passed to `plux_invoke`. |
| `input` | The input, as the UTF-8 bytes of the string. |
| `depth` | The nesting level of the call (`0` is top level). |
| `limits` | `timeoutMs`, `memoryBytes`, `inputBytes`, `outputBytes` (the JSON payload), `logBytes`, `httpCalls`, `callDepth`. |
| `context` | `app`, `environment`, `assuranceLevel`, reported by `context()`. |
| `env` | The variables of `env_get`. |
| `grants` | `httpDomains` (exact hosts or `*.example.com`), `kv`, `secrets` (names), `functions` (names), `claims`. Absent means not granted. |
| `host.kv` | Initial key-value pairs (strings). |
| `host.secrets` | Secret name to value. |
| `host.claims` | The claims JSON, or `null` for none. |
| `host.http` | Responses by exact request URL: `url`, `status`, `headers`, `body` (text; the host base64-encodes it). A URL not listed fails as `upstream`. |
| `host.invoke` | Nested results by function name: `{"output": …}` or `{"business": {code, messageKey, params}}`. A name not listed fails as `notFound`. |
| `host.fail` | Makes one capability fail: `kvGet`, `kvPut`, `kvDelete`, `http`, `secret`, `invoke` to `notFound`, `limit`, `egressRefused`, `upstream` or `unavailable`, which the host maps to −1, −3, −4, −5 and −7. |

### Expectations

Exactly one of these describes the result; the other keys are checked when
present.

| Key | Meaning |
|---|---|
| `compileError` | Compiling the module for the placement fails with this `PLX-` number. |
| `errorCode` | The invocation fails with this `PLX-` number. |
| `tag` | The invocation ends in this result tag: `0` with `output` (JSON, compared as a value), `outputText` (exact bytes) or `outputLength`, or `1` with `business`. |
| `logs` | The log records in order: `level` (0 debug, 1 info, 2 warn, 3 error) and `message`. Standard output is info, standard error is error; the one truncation marker is warn `log output truncated`. |
| `logBytes`, `httpCalls` | The accepted log bytes and the `http_request` calls that reached the host. |
| `hostCalls` | The calls that reached the fake host (all capabilities): `0` proves a refusal never called it. |
| `invokeDepths` | The depths passed to the host for nested invocations. |

### Statuses seen by the probe

A negative status from a host call appears in the probe's output as
`{"status": N}`, or as the number `N` where a value was expected: `-1` not
found, `-2` capability denied, `-3` limit exceeded, `-4` egress refused, `-5`
upstream failed, `-6` invalid argument, `-7` unavailable. An `http_request`
whose URL is not an `http` or `https` URL with a host is `-6`, whatever the
grants; a granted capability whose host is not on the allowlist is `-4`; HTTP
without any allowed domain is `-2`.

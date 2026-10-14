<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Function Security and Limits

What keeps a Plux Function inside its box, what it can reach when you grant it something, what
a device must prove before it may call one, and the limits that bound every call. The design is
[ADR-0064](../adr/0064-function-isolation.md); the module contract is the [ABI
reference](../functions/abi.md); building is the [builds guide](function-builds.md). The general
security model is the [security guide](security.md).

## 1. The threat

A server function is code written by one customer and run by Plux for that customer's users, on
machines shared by many customers. A function must not read another tenant's data, exhaust the
runner, reach internal services or leak a secret. Plux assumes the module is hostile and builds
so that a compromised module reaches nothing valuable.

## 2. The roles

| Role | Does | Holds | Reaches |
|---|---|---|---|
| `api` | Authenticates the device, checks the call, hands it on | Database access | `fnrunner` over mutual TLS |
| `fnrunner` | Runs modules on `wazero` | **No database credentials, no signing keys** | `api` (host service) and `egress`, nothing else |
| `egress` | Forward proxy; the only route out of `fnrunner` | The function's allowlist for each request | The internet, minus private addresses |
| `builder` | Compiles uploaded source | **No database credentials, no signing keys** | Nothing: no network |
| `worker` | Runs builds through `builder`; asks `signing/` to sign | Database access | `builder`, `signing/` |

These rules are the layering rule L-4 (`FN-013`, `FN-016`). A test fails the build if the
`fnrunner` or `builder` packages import storage, signing or a database driver. Every link
between roles uses mutual TLS with certificates from the internal CA; a role without a valid client
certificate is refused (`SEC-043`). A network policy lets `fnrunner` reach only `api` and `egress`;
`egress` is the only role with a route to the internet.

## 3. Isolation of a call

- **A fresh instance for every call** (`FN-010`). Linear memory is new each time, so no state
  leaks between calls or between tenants. Only compiled code is shared.
- **Verify before load.** `fnrunner` compiles a module only after its SHA-256 equals the
  requested one and its signature verifies against a key from its own configuration. Anything
  else is `PLX-7019` ([builds guide](function-builds.md#6-provenance-and-the-signed-statement)).
- **A module is checked before any guest code runs**: exports, imports and the absence of a
  start section. An import outside the ABI is refused (`PLX-7018`).
- **Limits are enforced from outside the module** (section 5). A loop is stopped by a deadline
  that closes the instance, not by the module's cooperation.
- **The WASI shim is locked down**: no file system, no arguments, no environment, no sockets.
  Standard output and error become log lines. See the [ABI](../functions/abi.md#6-the-wasi-shim).
- **Host pointers are bounds-checked.** A bad pointer ends the call with `PLX-7007`; it never
  crashes the host.
- **Nothing sensitive leaves in logs or metrics.** Metric labels never carry an app, user,
  function or input. A log line carries identifiers and the `PLX` code, never input, output, claims
  or secrets (`SEC-092`).

## 4. Capabilities

Capabilities are **deny by default** (`FN-012`, `SEC-080`) and granted by the function's
document. A module that calls a host function it was not granted gets status `-2` and the host
never serves it.

| Capability | Granted by | What it allows |
|---|---|---|
| Outbound HTTP | `capabilities.http`: exact host names or `*.example.com` | Requests to those hosts through `egress`; at most `function.httpCalls` per call |
| Key-value store | `capabilities.kv` | The function's own namespace |
| Secrets | `capabilities.secrets`: names | The value of each named secret; a function cannot list names |
| Other functions | `capabilities.functions`: names | Nested calls to those functions, at most three deep |
| Claims | `capabilities.claims` | The verified claims of the caller |
| Environment values | `env`: names | `env_get` of those names |

A device function can be granted only `kv` and device-placed `functions`; its context type does
not offer the rest, and declaring them is `PLX-7030`.

**A secret and a domain together are a reviewed pair.** A function that may read a secret and
call a domain can send that secret to that domain. Grant them deliberately and review changes to
`capabilities` like code.

### Egress

`egress` is a small forward proxy built on the Go standard library (`FN-017`). Every request a
function makes, plain HTTP included, becomes a `CONNECT` tunnel over the mutual-TLS link, so
TLS to the destination stays end to end and `egress` never sees plaintext or holds a
certificate. For each tunnel it decides, in this order:

1. The port is one of the configured ports (`egress.ports`).
2. The target is a DNS name, never an IP literal.
3. The name matches the function's allowlist. `fnrunner` sends the allowlist with each request;
   `egress` keeps no per-tenant state.
4. **Every address the name resolves to** passes the gateway's SSRF rules: loopback, link-local,
   private, multicast and cloud-metadata addresses are refused, including IPv4-mapped IPv6
   forms.

It then dials the address it validated, never the name again, which closes the DNS-rebinding
window. `fnrunner` does not follow redirects, so a redirect cannot lead out of the allowlist; the
function sees the `3xx` answer. A refusal is `PLX-7010` and names the rule that refused, and
nothing about the destination.

Tunnels are bounded: an idle timeout, a maximum duration and a maximum byte count per direction.

## 5. Limits

All limits come from the registry (`LIM-001`, `FN-011`); none is hard-coded. A function's
document can tighten a limit for itself but not exceed the registry maximum. Operators set the
installation, organisation or app value in [limits](../reference/limits.md).

| Key | Default | Maximum | Over the limit |
|---|---|---|---|
| `function.timeout` | 2 s | 30 s | `PLX-7001` |
| `function.memory` | 64 MiB | 512 MiB | `PLX-7002` |
| `function.inputSize` | 1 MiB | 16 MiB | `PLX-7022` |
| `function.outputSize` | 1 MiB | 16 MiB | `PLX-7004` |
| `function.httpCalls` | 10 | 100 | Status `-3` to the module |
| `function.callDepth` | 3 | 3 | Status `-3` to the module; `PLX-7005` from the host service |
| `function.logBytes` | 64 KiB | 1 MiB | Log truncated once, with a marker |
| `function.moduleSize` | 16 MiB | 64 MiB | The build fails |
| `function.deviceTimeout` | 500 ms | 5 s | `PLX-7001` on the device |
| `function.deviceMemory` | 32 MiB | 128 MiB | `PLX-7002` on the device |
| `function.kvBytes` | 10 MiB | 1 GiB | Status `-3` to the module; `PLX-7016` |
| `function.kvKeys` | 10,000 | 1,000,000 | `PLX-7016` |
| `function.kvValueBytes` | 64 KiB | 1 MiB | `PLX-7016` |
| `function.idempotencyWindow` | 24 h | 7 days | Installation scope only; less than 24 h is refused |
| `function.invocationsPerMinute` | 600 | 100,000 | `PLX-7015`, per function |
| `function.invocationsPerMinutePerApp` | 6,000 | 1,000,000 | `PLX-7015` |
| `function.invocationsPerMinutePerDevice` | 60 | 10,000 | `PLX-7015` |
| `function.testCases` | 100 | 1,000 | `PLX-7203` |
| `function.sourceArchiveSize` | 64 MiB | 512 MiB | Upload refused |
| `function.buildTimeout` | 10 min | 60 min | `PLX-7203` |
| `function.buildMemory` | 4 GiB | 16 GiB | `PLX-7203` |
| `app.functions` | 100 | 1,000 | Functions one app may have |

The runner also bounds the invocations running at once, and the size of the response body a
function reads. When the runner is full it refuses the next call with `PLX-8040` instead of
queueing it.

## 6. What a device must prove

For a server function the device calls `FunctionService.Invoke` with DPoP and a valid access
token (`FN-020`). The server checks, cheapest first, before the module runs:

1. The input is at most `function.inputSize`, and the device is under its rate.
2. The version is pinned by an active release of the app, and the function is server-placed. A
   device-placed version is refused.
3. The device holds the **assurance level** the function declares (`assurance`), or the
   environment's `defaultFunctionAssurance` when it declares none (`PLX-6002`).
4. If the function declares `attestation: true`, **per-request platform evidence**: an App Attest
   assertion or a Play Integrity token that binds the function, the version, the input and the
   identifier of the request's DPoP proof (`SEC-003`, `SEC-004`). Evidence for another input or
   another request does not verify (`PLX-6001`).
5. The app and function rates, then the idempotency key (`FN-021`).
6. If the function declares `sca: true`, a **strong customer authentication** signature over a
   challenge that is bound to this function, version and input, single use and short lived
   (`SEC-027`, `SEC-028`). The user confirms what the function will do, and a signature for one
   input cannot be reused for another.

None of this depends on the app being honest. The checks run on the server, and the module sees
the result only as the verified `claims` and the `assuranceLevel` of `context()`.

**Device results are never evidence.** A function placed on the device runs on hardware the user
controls. The server never accepts its output for a decision (`FN-054`). A flow that needs a
trusted answer calls a server function, which may run the same code.

## 7. Secrets and the key-value store

- `secret_get` returns the environment's server-held values
  ([ADR-0061](../adr/0061-server-held-configuration.md)), granted to a function by name. The
  value is delivered to the module's memory for that call and is never logged or cached by
  `fnrunner`. Secrets are separate per environment and are not carried by promotion.
- The server key-value store is PostgreSQL with one namespace per app, environment and function,
  under row-level security like every tenant table (`FN-018`). `fnrunner` reaches it only through
  host calls served by `api`, which takes the tenant from the call's grant and never from the
  module.
- `fnrunner` reaches `api` with a **per-invocation grant**: an opaque token valid for at most 60
  seconds and scoped to one app, environment, function and version. A call without a valid grant,
  or outside its scope, is refused.

## 8. If the sandbox were escaped

An attacker inside the `fnrunner` process would hold:

- the internet through `egress`, limited to private-address refusals, because the allowlist is
  sent by the compromised caller;
- the grants of the invocations in flight, each valid for under 60 seconds and scoped to one
  function;
- the cached compiled modules, which are signed and not secret.

They would not hold database credentials, signing keys, or another tenant's grants. The blast
radius is the key-value data and secrets of the functions in flight at that moment. This is the
reason `fnrunner` has no more than that.

## 9. Operating notes

- The `fnrunner`, `egress` and `builder` roles require mutual TLS and refuse to start without
  it. `egress.allowLoopback` and `builder.sandbox: none` are for development: the first is
  refused in a production installation, the second logs a distinct warning.
- Rotate the function-artifact key through `signing/` and add the new public key to
  `fnrunner.artifactKeys` before the old one is removed, or modules signed with the old key stop
  loading (`PLX-7019`).
- Failures and what to do: the [function outage](../runbooks/function-outage.md), [egress
  refusals](../runbooks/function-egress-refusals.md) and [builder
  failure](../runbooks/function-builder-failure.md) runbooks.

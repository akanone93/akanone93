# 0064. Function isolation: runner, egress, key-value store and limits

- **Status:** Proposed
- **Date:** 2026-10-13
- **Requirements:** `FN-010`–`FN-013`, `FN-017`, `FN-018`, `FN-020`, `FN-021`, `FN-024`, `FN-054`, `SEC-043`

## Context and problem

Server functions are code written by app developers and executed by Plux for their users:
many tenants share the runner. A function must not read another tenant's data, must not
exhaust the runner, must not reach internal services, and must not leak secrets. The runner
(`fnrunner`) therefore holds no database credentials and no signing keys and reaches the
network only through an egress proxy (`FN-013`, layering L-4). This ADR fixes the services
between roles, the limits, the egress design, the key-value store and what an escape from the
sandbox would be worth.

## Decision drivers

- Isolation by construction: a compromised module should reach nothing valuable.
- Deny-by-default capabilities (`FN-012`).
- Limits from the registry, never hard-coded (`LIM-001`, `FN-011`).
- Tenant isolation as strong as every other tenant table (row-level security).
- Overhead within the budgets of `FN-023`.

## Considered options

1. Run functions inside `api` or `worker`.
2. A separate `fnrunner` role with a narrow, grant-authorised host service, plus an `egress`
   role (chosen).
3. One process per invocation or a microVM per tenant.

## Decision

Chosen option: **2**. Option 1 puts tenant code beside database credentials and keys.
Option 3 gives stronger isolation at a start-up cost the budgets exclude; fresh `wazero`
instances give the isolation that matters at about 0.25 ms.

**Internal services** (protos in `proto/plux/internal/fn/v1/`), all over mutual TLS with
certificates from the internal CA (`SEC-043`); a role without a valid client certificate is
refused.

- `api` → `fnrunner`: `FunctionRunnerService.Invoke` carries the artifact reference, the
  input, the limits, the granted capabilities, verified claims and a **per-invocation grant**:
  an opaque token valid for at most 60 seconds and scoped to app, environment, function and
  version.
- `fnrunner` → `api`: `FunctionHostService` serves modules (by SHA-256), the key-value store,
  secrets and nested invocations. Every call presents the grant; `api` checks scope and
  capability. `fnrunner` therefore never holds credentials, only the grants in flight.
- `fnrunner` → `egress` for all outbound HTTP.

**Limits** (registry keys of `FN-011`, enforced by `wazero` and the host):

- *Memory:* the module's memory is capped in pages (`function.memory`); growth beyond it
  fails the instance with `PLX-7002`.
- *Time:* a context deadline (`function.timeout`) with close-on-done, so a module in an
  endless loop is closed from outside the instance (`PLX-7001`); no reliance on the module's
  cooperation.
- *Output:* the host refuses an output above `function.outputSize` (`PLX-7004`), and input
  above `function.inputSize` (`PLX-7022`); log bytes are bounded by `function.logBytes`.
- *Counting:* the host counts `http_request` calls (`PLX-7006`), key-value operations and
  nested calls; depth above `function.callDepth` fails with `PLX-7005`.
- Rate limits and quotas apply per function, app and device (`FN-024`, `PLX-7015`).

**Egress** (`FN-017`, a Go standard-library forward proxy, no new dependency).

- Plain HTTP is forwarded; HTTPS uses `CONNECT` with TLS end to end between the module's
  host call and the destination, so `egress` never sees plaintext and holds no certificates.
- `fnrunner` sends the function's domain allowlist with each request; `egress` holds no
  per-tenant state. Not allowed: `PLX-7010`.
- After resolving the name, `egress` applies the gateway's SSRF rules: loopback,
  link-local, private, multicast and metadata addresses are refused, including IPv4-mapped
  IPv6 forms.
- **DNS rebinding:** `egress` dials the *resolved address it validated*, never the name
  again, and checks the redirect targets the same way (`fnrunner` does not follow redirects).
- Network policy allows `fnrunner` only `api` (internal) and `egress`; `egress` is the only
  role with a route to the internet.

**Key-value store** (`FN-018`). PostgreSQL, one namespace per app, environment and function,
under row-level security like every tenant table, bounded by `function.kvBytes`,
`function.kvKeys` and `function.kvValueBytes` (`PLX-7016`). `fnrunner` reaches it only
through `FunctionHostService`, which sets the tenant from the grant, never from the module.

**Secrets.** `secret_get` returns the environment's server-held values
([ADR-0061](0061-server-held-configuration.md)), granted to a function by name; a function
cannot list names. The value is delivered to the module's memory for that call and never
logged or cached by `fnrunner`.

**If the sandbox in `fnrunner` were escaped.** The attacker holds a process with: the
internet through `egress` (limited to its allowlist, but the allowlist is sent by the
compromised caller, so effectively the public internet minus private ranges); the grants of
the invocations in flight, each valid for under 60 seconds and scoped to one app,
environment and function; and the cached compiled modules, which are signed and not secret.
It holds no database credentials, no signing keys and no other tenant's grants; the grant
cannot reach another scope. Blast radius: the in-flight tenants' key-value data and secrets
for those functions.

**Threat model delta (STRIDE).**

| Threat | Vector | Mitigation |
|---|---|---|
| Spoofing | A forged invocation or a role impersonating `api` | Mutual TLS between roles; grants are opaque, short-lived and scoped; `Invoke` sits behind DPoP and token checks (`FN-020`) |
| Tampering | A modified module | `fnrunner` verifies the artifact signature and the SHA-256 before compiling ([ADR-0063](0063-function-builds-provenance.md)) |
| Tampering | A device claiming a result as evidence | Device results are never accepted as evidence (`FN-054`) |
| Information disclosure | Cross-tenant key-value read | Namespace from the grant; row-level security in PostgreSQL |
| Information disclosure | Secret exfiltration through egress | Secrets are granted by name per function; egress allowlist; secrets never logged; a function that may read a secret and reach a domain is a reviewed, declared pair |
| Information disclosure | SSRF to internal services | Private ranges refused after resolution; dial the validated address; no redirects |
| Denial of service | Memory, time, output or call-depth exhaustion | Registry limits enforced by the host; deadline closes the instance; per function, app and device rate limits |
| Denial of service | Log flooding | `function.logBytes` per invocation; log rate limit; truncated with a marker |
| Elevation of privilege | Escape from the wasm sandbox | No credentials in `fnrunner`; network policy; blast radius above |

## Consequences

- **Positive:** a compromised module has no path to the database, the keys or other
  tenants beyond the grants in flight; limits are data, not code; the proxy is small and
  testable.
- **Negative:** three more roles to deploy (`fnrunner`, `egress`, `builder`) and their
  certificates; an `api` hop for every key-value or secret call; the allowlist is only as
  good as `fnrunner`'s integrity, mitigated by the SSRF rules and network policy.
- **Follow-up:** isolation tests cover memory and time exhaustion, output floods, call-depth
  loops, attempts at filesystem, sockets and environment, cross-tenant key-value access and
  fuzzed inputs; Compose and the Helm chart gain the roles and network policies; the
  configuration's refusal of `fnrunner` is removed.

## Options in detail

### Run inside `api` or `worker`

Lowest latency, and the compromised module then sits beside database credentials and
signing access. Rejected: violates `FN-013`.

### Separate runner and egress roles

Chosen. A narrow host service means every privilege is a call that `api` authorises against
the grant.

### Process or microVM per tenant

Stronger boundary, but start-up cost of tens of milliseconds to seconds per call against
`FN-023`'s 1 ms budget and a large operational burden. Rejected; revisited if a deployment
needs hard multi-tenancy beyond wasm.

## Revision (2026-10-14, P7 F4)

`fnrunner` compiles a server module when the api role deploys or promotes its version
(`Prepare`), so "the first invocation after deploy" of `FN-023` is the first call after that
compile (maintainer, P7 plan §2.6 M9). A call that reaches a runner without the module compiled
(the compile failed, a restart without a compilation cache, another replica) compiles it
first; that cold path is measured and reported, not gated. Each instance's linear memory comes
from a bounded pool of buffers that are cleared before reuse, so the per-call allocation no
longer drives the garbage collector, and no byte crosses from one invocation to the next
(`FN-010`). The budgets are gated on the CI reference runner, as `CMP-050`'s are, the warm one
on the median of five rounds (M10).

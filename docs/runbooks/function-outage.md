# Function Outage Runbook

Server functions fail because the `fnrunner` role is down, unreachable or overloaded, or because
it refuses the modules it is given (`FN-009`, `FN-010`, `FN-011`, `FN-013`,
[ADR-0064](../adr/0064-function-isolation.md)). Background: the [functions
guide](../guides/functions.md) and the [security guide](../guides/function-security.md). Device
functions run on the phone and are not affected by this runbook.

## Symptoms

- Devices report `PLX-8091` (`UPSTREAM_UNAVAILABLE`) for `FunctionService.Invoke`: the `api`
  role could not reach `fnrunner`, or `fnrunner` answered as unavailable.
- Devices report `PLX-8040` (`RATE_LIMITED`) from `fnrunner` while no per-function rate limit is
  near: the runner has no free slot (`fnrunner.maxConcurrent`) and refuses instead of queueing.
- A rise of `PLX-7001` (`FUNCTION_TIMEOUT`) across many functions at once, not one.
- Every call to every function fails with `PLX-7019` (`FUNCTION_ARTIFACT_SIGNATURE_INVALID`) or
  `PLX-7018` (`FUNCTION_MODULE_INVALID`).
- Pages that call a function show their error state. A call that carries an idempotency key
  can be repeated safely; the first result is returned if the call had completed (`FN-021`).

## Signals

`api` and `fnrunner` count the same invocations under different names, so compare them.

| Signal | Query |
|---|---|
| Result mix as devices see it (api) | `sum by (result)(rate(plux_function_requests_total[5m]))` |
| Result mix at the runner | `sum by (result)(rate(plux_function_invocations_total[5m]))` |
| Share of failures at the api | `1 - sum(rate(plux_function_requests_total{result=~"ok\|business_error"}[5m])) / sum(rate(plux_function_requests_total[5m]))` |
| Latency at the api | `histogram_quantile(0.99, sum by (le)(rate(plux_function_request_seconds_bucket[5m])))` |
| Latency in the runner | `histogram_quantile(0.99, sum by (le)(rate(plux_function_duration_seconds_bucket[5m])))` |
| Memory peaks | `histogram_quantile(0.99, sum by (le)(rate(plux_function_memory_peak_bytes_bucket[5m])))` |
| Module cache | `sum by (result)(rate(plux_function_module_cache_total[5m]))`, with `result` of `hit`, `miss` and `refused` |

`result` is `ok`, `business_error`, or the lower-case reason of the error (`function_timeout`,
`function_trapped`, `rate_limited`, `upstream_unavailable`, `function_artifact_signature_invalid`).
No series carries an app, user, function or input. A gap between the two views, with the api
counting failures that the runner never saw, is a path problem, not a function problem.

Alert when the failure share exceeds 5% for 5 minutes, or the api p99 exceeds `function.timeout`
(2 s by default). Per-function metrics, invocations by kind and latency, are available through the
API and `plux fn logs`. The runner's own log lines carry identifiers and the `PLX` code, never
input, output, claims or secrets.

## Diagnosis

Work from the cheapest check.

1. **Is it one function or all?** One function with `PLX-7001`, `PLX-7002` or `PLX-7007` is that
   function's problem: read its logs with `plux fn logs <name> --since 15m` and see whether the
   last deploy or promotion changed it. Everything failing together points at the runner or the
   path to it.
2. **Is the role up?** Check the `fnrunner` process or pod and its restarts. A role that does not start logs why; a configuration check offline
   is `plux-server config validate`. The role refuses to start without mutual TLS, an `apiURL`,
   an `egressAddr`, at least one trusted artifact key and positive bounds.
3. **Can `api` reach it?** `PLX-8091` with the role up means the path: `functions.runnerURL` on
   `api`, the network policy between them, and the internal certificates. Both sides need a
   certificate from the internal CA (`SEC-043`); an expired certificate looks like an outage.
   Check expiry on both ends.
4. **Is the runner full?** `rate_limited` at the runner while the load is normal means
   `fnrunner.maxConcurrent` is too small for the traffic, or slow functions hold slots. Compare
   the runner's p99 duration against the mix: a few functions at the timeout hold the slots.
5. **Can the runner reach `api`?** The runner fetches modules, key-value data and secrets from
   `api` through the host service, authorised by a grant that is valid for at most 60 seconds.
   `fnrunner.apiURL` wrong, `api`'s internal listener down, or clock skew that expires grants
   at once, each makes every call fail while the process looks healthy.
6. **Is it refusing modules?** `refused` in the cache metric, with `PLX-7019`, means the runner
   rejected a module: the signature does not verify against any key in
   `fnrunner.artifactKeys`, or the bytes do not match the SHA-256 it requested. After a key
   rotation this is the first thing to check (below). `PLX-7018` or `PLX-7017` for modules
   that ran yesterday means the runner changed under them; compare the runner version with the
   one that ran before, and read [the ABI](../functions/abi.md#8-when-a-host-refuses-a-module).
7. **Is egress the problem?** Functions that call APIs fail while others work: see the [egress
   refusals runbook](function-egress-refusals.md).
8. **Resource pressure.** A runner near its memory limit, CPU throttled, or out of disk for
   `fnrunner.cacheDir` slows every call. The memory peak histogram shows which calls use the
   most; with `function.memory` at 64 MiB and `maxConcurrent` slots, the worst case is their
   product.

## Mitigation

- **Runner down:** restart it. The module cache is rebuilt from `api` on demand, or from
  `cacheDir` if set; the first call of each module is slower (`FN-023`: first call after deploy
  within 50 ms p99 on the reference deployment).
- **Runner overloaded:** add `fnrunner` replicas, or raise `fnrunner.maxConcurrent` within the
  host's CPU and memory. Do not raise `function.timeout` globally, which lets slow functions
  hold slots longer.
- **One function misbehaving:** tighten its `limits` in `function.json`, or move its alias back
  to the previous version with `plux fn alias <name> <alias> <version> --env <env>`.
  Versions are immutable, so the old one is unchanged.
- **Certificate expired:** replace the internal certificates on the role and restart; `api`
  needs no change if the CA is the same. If the CA changed, update `server.internalTLS` on every
  role.
- **`PLX-7019` after a key rotation:** add the new function-artifact public key to
  `fnrunner.artifactKeys` and restart the runner; keep the old key while versions signed with it
  remain in use. Never accept a key from the `api` role: the runner takes keys from its own
  configuration only.
- **`PLX-7019` without a rotation:** treat it as a possible tampering event. Do not add keys to
  make it go away. Compare the artifact's SHA-256 in the version record with the object store,
  find out who changed the runner's configuration, and escalate as a security
  incident ([key compromise](key-compromise.md)).
- **Device-side fallback:** a device function keeps working when the server is down. A
  server-only flow cannot be bypassed; do not move a function to the device as a workaround,
  because its result is for display only (`FN-054`).

## Recovery

1. Watch the failure share and the p99 fall, and `hit` outgrow `miss` in the cache metric.
2. Clients retry on their own where the action graph retries. Calls with an idempotency key
   do not run twice. Calls without one may have run before the failure was reported: for a
   function with side effects, check the target system before replaying anything by hand.
3. Review what changed in the window: recent deploys, promotions and alias moves, and any change
   to the `fnrunner` configuration.
4. Record the cause and what would have caught it earlier.

## Prevention

- Run at least two `fnrunner` replicas, apart from `api`, and size `maxConcurrent` against the
  `function.memory` you allow.
- Alert on the failure share, the p99, `rate_limited` and the module cache `refused` series.
- Track expiry of the internal certificates and rotate before it.
- Rotate the function-artifact key with an overlap: add the new public key to every runner,
  then sign with it, then remove the old one when no version signed with it is pinned.
- Keep `limits` in each `function.json` tight, so one function cannot occupy the runner.

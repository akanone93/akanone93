# Function Egress Refusals Runbook

Server functions fail their outbound HTTP calls because the `egress` role refuses the
destination, cannot reach it, or is full (`FN-012`, `FN-017`,
[ADR-0064](../adr/0064-function-isolation.md)). Background: the [security
guide](../guides/function-security.md#egress) and the [ABI](../functions/abi.md) for the
`http_request` statuses.

## Symptoms

- A function reports `PLX-7010` (`FUNCTION_EGRESS_REFUSED`), or its module sees status `-4`
  from `http_request` and returns its own business error.
- A function sees status `-5` (upstream failed): the destination did not answer, or the tunnel
  failed after it was admitted.
- A function sees status `-7` (unavailable) on every HTTP call: `egress` has too many tunnels
  open, or `fnrunner` cannot reach it.
- A function sees `-3` (limit exceeded): it made more calls than `function.httpCalls`, or a
  response body was larger than the runner's bound.
- A burst of refusals right after a deploy, a promotion, a DNS change at the destination or a
  change to `egress` configuration.

## Signals

| Signal | Query |
|---|---|
| Tunnels by result | `sum by (result)(rate(plux_egress_tunnels_total[5m]))` |
| Refusals only | `sum by (result)(rate(plux_egress_tunnels_total{result=~"refused_.*"}[5m]))` |
| Bytes carried | `sum by (direction)(rate(plux_egress_bytes_total[5m]))` |

`result` is one of `ok`, `refused_domain`, `refused_address`, `refused_port`,
`refused_ip_literal`, `bad_request`, `upstream_failed` and `limited`. Each refusal is also a log
line of the `egress` role at warning level (`egress refused`) with the function name, the
host, the result and the reason. The answer to the runner names the rule and nothing about the
destination. Alert on a sustained rise of any `refused_*` series, and on `limited` above zero.

## Diagnosis

The result tells you which rule refused. Work through them in the order `egress` applies them.

| `result` | Reason in the log | Cause | Fix |
|---|---|---|---|
| `refused_port` | the port is not allowed | The URL uses a port outside `egress.ports` | Use a standard port, or add the port to `egress.ports` if the destination is trusted |
| `refused_ip_literal` | the target must be a host name | The function called `https://203.0.113.5/…` or an IPv6 literal | Call the destination by name. IP literals are never allowed |
| `refused_domain` | the domain is not allowed | The host is not in the function's allowlist | See below |
| `refused_address` | the address is not allowed | The name resolves to a loopback, link-local, private, multicast or cloud-metadata address | See below |
| `bad_request` | various | `fnrunner` sent a malformed tunnel request: a missing function or allowlist header, or an invalid host | This is not the function's fault; check that `fnrunner` and `egress` run the same release |
| `upstream_failed` | various | The destination refused or timed out | The destination's problem; check it from the `egress` host |
| `limited` | too many tunnels | `egress.maxTunnels` is reached | Scale `egress`, or find the function holding tunnels open |

**`refused_domain`.** The host must match an entry of the function's `capabilities.http`: an exact
name, or `*.example.com`. A wildcard matches any host with at least one label before
`.example.com`, not `example.com` itself; matching ignores case, a trailing dot and the port.
Common mistakes:

- The apex is missing (`api.example.com` is listed but the function calls `example.com`).
- A wildcard was expected to match the apex.
- A redirect: `fnrunner` does not follow redirects, but the function may follow one itself to a
  host that is not on the list. The log line names the host that was refused, not the one the
  function started with.
- The allowlist is the version's: the HTTP domains its `function.json` declares, the same in
  every environment. A function that reads its base URL from an environment variable fails here
  when that variable names a host the version does not declare; deploy a version that declares it
  ([functions guide](../guides/functions.md#7-promote-between-environments)).
- A userinfo trick such as `https://allowed.example.com@evil.example.net/` is matched on the real
  host, `evil.example.net`, and refused.

**`refused_address`.** `egress` resolves the name and checks **every** address it returns
against the gateway's SSRF rules, then dials the address it validated. A name that resolves to a
private address is refused however it is allowlisted. This is correct for a public API and wrong
for a service in the customer's own network:

- If the name should be public, a split-horizon DNS record or a stale internal record is
  answering for it. Resolve it from the `egress` host, not from your laptop.
- If the destination really is in a private network, a function cannot reach it directly.
  Put the API behind the [gateway](../guides/gateway.md), which has `allowPrivateAddresses` per
  upstream with review, or expose it publicly with authentication. Do not set
  `egress.allowLoopback`: it is for tests, a production installation refuses it, and any other
  start with it set logs a warning.

**Status `-5` or `-7` with no `egress` log line** means the request never reached `egress`:
check `fnrunner.egressAddr`, the network policy that allows `fnrunner` to reach `egress`, and
the internal certificates (`SEC-043`). A certificate that has expired fails every call.

## Mitigation

- **Domain missing:** add it to the function's `capabilities.http`, deploy a new version
  (`plux fn deploy <name> --env <env>`), and promote it. Add only the hosts the function needs.
  Review the combination of secrets and domains the function holds: a function that can read a
  secret and call a domain can send the secret there.
- **Wrong port:** change the URL; widen `egress.ports` only for a destination you trust.
- **Resolves to a private address:** fix the DNS record, or route through the gateway.
- **`limited`:** raise `egress.maxTunnels` or add `egress` replicas. Tunnels also end by
  `egress.idleTimeout`, `egress.maxTunnelDuration` and `egress.maxTunnelBytes`; a function that
  streams large bodies hits them.
- **Destination down:** nothing to fix on the Plux side. A function should turn `-5` into a
  business error the app can show. Lower the function's `httpCalls` or `timeoutMs` so one slow
  destination does not hold runner slots (see the [function outage runbook](function-outage.md)).

## Recovery

1. Watch the refusal series fall and `ok` recover.
2. Calls that failed with `PLX-7010` made no request to the destination: a refusal happens
   before any bytes leave. They are safe to repeat. A call that failed with `-5` may have reached
   the destination; for a side-effecting API, check there first.
3. Record the change to the allowlist in the change ticket. Review it as you would a code change.

## Prevention

- Keep each function's allowlist to the exact hosts it needs; prefer exact names to wildcards.
- Test calls against the staging destination before promoting, and check the production
  allowlist as part of the promotion.
- Alert on `refused_address`: a name that starts resolving to a private address is either a
  misconfiguration or an attempt to reach the internal network.
- Never put internal hosts on an allowlist to make a call work; an allowlist entry does not
  override the private-address rules.

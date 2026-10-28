<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Studio BFF Runbook

How to configure, watch and repair the Studio backend for frontend (BFF, `studio/apps/bff`,
`STU-002`, [ADR-0014](../adr/0014-studio-stack.md)). The BFF is one Bun process that serves
Studio's built web application, forwards `/api` to the Plux Server, completes single sign-on
sign-ins, takes browser telemetry and CSP reports, and answers health checks. It holds no database,
object-store or signing credential and keeps no session: the session is the server's. Every
statement below is taken from the BFF's source; the file names are given so you can check them.

## Configuration

Every setting is an environment variable. Each can also be a key in a JSON object file named by
`PLUX_BFF_CONFIG`; a variable set in the environment wins over the file. A bad value stops the
process at start with exit code 2 and a line `plux-bff: <reason>` on standard error that never
contains the secret (`config.ts`, `main.ts`). Unsafe but accepted settings print
`plux-bff: warning: UNSAFE: …`.

| Variable (file key) | Default | Meaning |
|---|---|---|
| `PLUX_BFF_PUBLIC_BASE_URL` (`publicBaseUrl`) | required | The URL people open, `http(s)`. Only its origin is kept. `https` turns on `Secure` cookies and HSTS; plain `http` outside `localhost`, `127.0.0.1` and `[::1]` prints an UNSAFE warning. |
| `PLUX_BFF_API_UPSTREAM` (`apiUpstream`) | required | The Plux Server's API origin, `http(s)`. Credentials in the URL are dropped with an UNSAFE warning; use a network policy. Use `https` and make the BFF trust the server's CA (below). |
| `PLUX_BFF_STATE_SECRET` (`stateSecret`) | required | Key for the sign-in state cookie: base64 or base64url of at least 32 bytes (43 characters). Generate with `openssl rand -base64 32`. Every replica must hold the same value. |
| `PLUX_BFF_HOST` (`host`) | `127.0.0.1` | Address to listen on. The Studio image sets `0.0.0.0`. |
| `PLUX_BFF_PORT` (`port`) | `8081` | Port, 1 to 65535. The Studio image's health check and the chart's probes use 8081. |
| `PLUX_BFF_DRAIN_TIMEOUT` (`drainTimeout`) | `30s` | How long to wait for open streams after SIGTERM, as `500ms`, `30s` or `2m`. |
| `PLUX_BFF_MIN_SERVER` (`minServer`) | `0.0` | Oldest server version Studio supports, `major.minor`. `/readyz` fails when the server reports an older one. |
| `PLUX_BFF_DOCS_URL` (`docsUrl`) | empty | `https` URL of the documentation site, without credentials. Empty hides the documentation links in Studio's help menu. |
| `PLUX_BFF_VALKEY_URL` (`valkeyUrl`) | empty | Valkey/Redis URL for rate-limit counters shared by replicas. Empty counts in memory, per replica. |
| `PLUX_BFF_TRUSTED_PROXIES` (`trustedProxies`) | empty | Comma-separated CIDRs (or single addresses) of the load balancers whose `X-Forwarded-For` is honoured. None by default. |
| `PLUX_BFF_REQUESTS_PER_MINUTE` (`requestsPerMinute`) | 1200 | API requests per session per minute, and per client address, before the BFF answers 429. Registry key `bff.requestsPerMinute`. |
| `PLUX_BFF_STREAMS_PER_SESSION` (`streamsPerSession`) | 8 | Server streams open at once per session. Registry key `bff.streamsPerSession`. |
| `PLUX_BFF_MAX_STREAM_CONNECTIONS` (`maxStreamConnections`) | 1000 | Streams open at once in this process. |
| `PLUX_BFF_TELEMETRY_ENABLED` (`telemetryEnabled`) | `true` | `true` logs browser telemetry; any other value turns it off (`POST /telemetry` then answers 204 and logs nothing). |
| `PLUX_BFF_TELEMETRY_SAMPLE_RATE` (`telemetrySampleRate`) | 1 | Share of telemetry events kept, 0 to 1. |

Two more things shape a deployment. `NODE_EXTRA_CA_CERTS` points Bun at the CA that signed the
server's certificate; the Compose stack sets it to the development CA
(`deploy/compose/compose.yaml`). The telemetry rate limit, 120 events per session per minute
(registry key `bff.telemetryEventsPerMinute`), is not a setting; limits come from the registry
(`LIM-001`, [limits](../reference/limits.md)).

**Where the settings come from.** The Compose stack sets the upstream and public URL in
`deploy/compose/compose.yaml` and the secret in `deploy/compose/.secrets/studio.env`
(`init-secrets.sh`). The Helm chart sets them from `studio.publicBaseUrl`, `studio.docsUrl` and a
Secret with the key `stateSecret` (`studio.existingSecret`; unset, the chart generates one and keeps
it across upgrades), and runs `studio.replicas` replicas (default 2). `make studio-dev` is the
development setup.

## Health

| Endpoint | Answers | Use it for |
|---|---|---|
| `GET /livez` | `200 ok` while the process runs | Liveness; the Studio image's `HEALTHCHECK` and the chart's liveness and startup probes |
| `GET /readyz` | `200 ready`, or `503` with `draining`, `upstream not ready`, `upstream too old` or `upstream unreachable` | Readiness; the chart's readiness probe. It asks the server's `/readyz` (2 s timeout), then its `/version` against `minServer` |
| `GET /version` | `{"studio","minServer","docs"?}`, `Cache-Control: no-store` | What Studio shows in its "Studio was updated" and "server too old" notices |

`/readyz` turns `503 draining` the moment SIGTERM arrives, so a load balancer stops sending new
requests before the process exits.

## Sessions, CSRF and CSP

- **Sessions are the server's.** Sign-in (password, passkey, OIDC, SAML) happens through the API.
  The server sets the cookie `__Host-plux_session`; the BFF passes it through and strips every
  other cookie and header that is not on its allowlist (`proxy.ts`). Idle and absolute lifetimes
  are the server's limits; an ended session answers `PLX-6230` (revoked), `PLX-6231` (idle too
  long) or `PLX-6232` (past its absolute lifetime). End a session in Studio under Administration,
  *Sessions*.
- **CSRF.** A call made with the session cookie must carry the session's token in `X-CSRF-Token`;
  the server checks it, the BFF forwards the header. A mismatch is `PermissionDenied`. The BFF itself
  refuses cross-site requests to `POST /auth/oidc/state` (Fetch Metadata or `Origin`, JSON body
  only).
- **Sign-in state.** For OIDC, Studio stores a one-time state in `__Host-plux_oidc` (HttpOnly,
  `SameSite=Lax`, 10 minutes, HMAC-signed with `PLUX_BFF_STATE_SECRET`). `GET /auth/oidc/callback` and
  `POST /auth/saml/acs` (at most 256 KiB) complete the sign-in against the server and answer a fixed
  message on failure: *Sign-in failed. Return to Plux Studio and try again.*
- **CSP.** Every response carries a fresh-nonce policy with `default-src 'none'`, `script-src`
  nonce plus `'strict-dynamic'` and `'wasm-unsafe-eval'`, `connect-src 'self'`, `worker-src 'self'`,
  `frame-ancestors 'none'`, `require-trusted-types-for 'script'` and `trusted-types plux-worker
  'allow-duplicates'`, plus `nosniff`, `Referrer-Policy: no-referrer`, COOP and CORP `same-origin`,
  a closed `Permissions-Policy`, and, over https, HSTS for two years. Violations go to `POST
  /csp-report` (16 KiB, 60 per minute per address) and are counted, never stored or echoed
  (`security.ts`; [Studio ASVS evidence](../security/asvs-studio.md)).

## Logs and metrics

- **Logs** are lines on standard output and standard error; there is no access log. Start-up writes
  `plux-bff listening on http://<host>:<port>` and, on SIGTERM, `plux-bff draining`. Configuration
  problems go to standard error as `plux-bff: …`. Browser telemetry is one JSON line per event,
  `{"msg":"browser.event","name","kind","code"?,"durationMs"?,"route"?,"traceparent"?}`: names and
  codes only, no cookie, address or free text (`OBS-001`). A batch holds at most 50 events and 64 KiB;
  a batch that does not match the schema is refused whole with 400.
- **Metrics.** The BFF exports none. Watch the server's metrics ([observability](../guides/operations.md)),
  the probes above, and the proxy or ingress in front of the BFF for status codes. The counters of
  CSP reports and dropped telemetry events live inside the process (`BffApp.cspReports`,
  `telemetryDropped`) and are read by tests only.

## Shutting down and scaling

On SIGTERM the BFF stops accepting API calls (503 with `Retry-After: 5`), waits until no stream is
open or `PLUX_BFF_DRAIN_TIMEOUT` passes, and exits 0. Set the orchestrator's grace period longer
than the drain timeout. Replicas share nothing but the state secret and, if configured, the Valkey
counters; scale by adding replicas.

## Common failures

| Symptom | Likely cause | Action |
|---|---|---|
| The process exits at start with code 2 | A required value is missing (`PLUX_BFF_PUBLIC_BASE_URL`, `PLUX_BFF_API_UPSTREAM`, `PLUX_BFF_STATE_SECRET`), or a value is malformed | Read the `plux-bff:` line on standard error; fix the variable it names |
| `PLUX_BFF_STATE_SECRET` is refused as too short | Fewer than 32 bytes, or not base64 | `openssl rand -base64 32` |
| `/readyz` answers `503 upstream unreachable` | The server is down, the address is wrong, or the BFF does not trust the server's CA (a TLS error looks like an unreachable host) | `curl` the server's `/readyz` from the BFF's network; set `NODE_EXTRA_CA_CERTS`; see the [server runbook](../runbooks/server.md) |
| `/readyz` answers `503 upstream not ready` | The server answered but is not ready (PostgreSQL, object store or Valkey) | The [server runbook](../runbooks/server.md), "A dependency is down" |
| `/readyz` answers `503 upstream too old` | The server's version is older than `PLUX_BFF_MIN_SERVER` | Upgrade the server, or lower `minServer` only if the older server is supported |
| `/readyz` answers `503 draining` for a long time | The process received SIGTERM and is waiting for streams | Wait for the drain timeout; check the orchestrator is not restarting it in a loop |
| Studio loads but every call fails with 502 `upstream unavailable` | The server became unreachable after start | As for `upstream unreachable` |
| Everyone gets `429 rate limit exceeded` behind a load balancer | All requests share the balancer's address, so the per-address limit counts everyone together | Set `PLUX_BFF_TRUSTED_PROXIES` to the balancer's CIDRs so `X-Forwarded-For` is used |
| One person gets 429 with `Retry-After: 60` | More than `requestsPerMinute` calls in a minute from one session | Look for a runaway client or script; raise the limit only with a reason |
| `429 too many open streams` | More than `streamsPerSession` live streams, for example many tabs | Close tabs; raise `PLUX_BFF_STREAMS_PER_SESSION` if legitimate |
| Single sign-on returns to Studio with *Sign-in failed* | The state cookie expired (10 minutes) or was blocked, the callback landed on a replica with a different `PLUX_BFF_STATE_SECRET`, or the provider's answer was refused by the server | Give every replica the same secret; check the provider and `PLX-8091` in the [server runbook](../runbooks/server.md); try again from Studio's sign-in page |
| Sign-in works but the next call is `PermissionDenied` on the CSRF header | A tab holds a token from an earlier session | Reload the tab |
| Session cookies are not kept | `PUBLIC_BASE_URL` is plain `http` outside localhost; `__Host-` cookies need https | Serve Studio over https and set the https URL |
| Blank page; the browser console shows CSP or Trusted Types violations | A proxy or extension injects scripts or styles, or the page is served from a different origin than `PUBLIC_BASE_URL` | Serve Studio from its own origin without response rewriting; `/csp-report` counts violations |
| `GET /` answers `404 not found` | The web application was not built (`studio/apps/web/dist` is missing) | `make studio-build`, or use the Studio image |
| Telemetry is missing | `PLUX_BFF_TELEMETRY_ENABLED` is not `true`, the sample rate is below 1, or events were rejected (400 schema, 413 size) | Check the settings; read the response to `POST /telemetry` |
| Rate limits differ between replicas | `PLUX_BFF_VALKEY_URL` is unset, so each replica counts alone | Set it; if Valkey is down the BFF falls back to per-replica counting without failing |

## See also

[Run it locally](../guides/run-locally.md) §4, the [server runbook](../runbooks/server.md), the
[operations guide](../guides/operations.md), [Studio security evidence](../security/asvs-studio.md)
and [Studio development](../engineering/studio-development.md).

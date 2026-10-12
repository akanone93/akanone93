<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Gateway Guide

How a device reaches your API through the Plux server, how to register your API as an
*upstream*, what your API receives, and how to run the gateway. The reference is
[gateway.md](../reference/gateway.md); the design and wire formats are
[ADR-0056](../adr/0056-secure-gateway-and-sca.md).

## 1. Why a gateway

DPoP, attestation and SCA signatures are checked by the Plux server. Your API cannot check
them without reimplementing Plux. So a device sends its API calls to the Plux server, which
checks the device, forwards the call to your API, and adds a short-lived signed
**assertion** that says who is calling. Your API trusts one thing: that JWT.

```text
device ──DPoP + token──▶ Plux gateway ──mTLS / OAuth / key + Plux-Assertion──▶ your API
```

The cost is one more hop, and the Plux server in the request path. The gateway adds at most
5 ms p95 at 1,000 requests per second per replica, excluding your API's own time
(`NFR-025`; `go test -bench . ./internal/gateway`).

## 2. Choose a route per data source

A data source has a `route`: `plux` (the default) or `direct`.

```json
{"name": "accounts", "kind": "rest", "type": "list<Account>",
 "route": "plux",
 "config": {"baseUrl": "apiBaseUrl", "path": "/accounts", "select": "items", "auth": true},
 "mock": []}
```

| Route | What happens | Use it for |
|---|---|---|
| `plux` | The call goes to the gateway; the server forwards to the upstream registered under the data source's name. The bundle carries no base URL for it | Anything with user data or money. The only route that can enforce `requiresAssurance`, per-device limits and SCA |
| `direct` | The device calls your API, as before P6, within the plugin's `networkDomains` and optional pins | Public content, third-party APIs you do not control |

Rules:

- A transaction cannot be `direct`. The compiler refuses it (`PLX-6073`).
- Under `maximum` a `direct` source is refused (`allowDirectDataSources` is off): the compiler
  reports `PLX-6063` and the runtime sends nothing.
- Under `standard` and `strict`, a `direct` source that sets `requiresAssurance` gets the
  warning `PLX-1504` (`DIRECT_SOURCE_ASSURANCE_UNENFORCEABLE`): the server cannot enforce
  a level on a call it never sees.
- The runtime never falls back from one route to the other.
- WebSocket, SSE and file transfers of a `plux` source go through the streaming route (§7);
  see [data sources](../reference/data-sources.md) for what is supported on the device.

## 3. Register an upstream

An upstream belongs to one app and environment. Register it through
`SecurityAdminService`; Studio gets a view in P11. You need `app.manage` to change one and
`app.read` to read it. Every change is audited.

The upstream's **name** is the data source's `name`: a lower-case letter followed by up to 63
letters and digits.

```bash
curl -sS "$PLUX_SERVER/plux.v1.SecurityAdminService/CreateUpstream" \
  -H "Authorization: Bearer $PLUX_TOKEN" -H "X-Plux-Organization: $ORG" \
  -H 'Content-Type: application/json' -d '{
  "upstream": {
    "appId": "01d0c450-…", "environmentId": "01d0c450-…",
    "name": "accounts",
    "baseUrl": "https://api.acme.example/bank/v1",
    "oauthClientCredentials": {
      "tokenUrl": "https://idp.acme.example/oauth/token",
      "clientId": "plux-gateway",
      "clientSecretRef": "ACCOUNTS_CLIENT_SECRET",
      "scopes": ["accounts.read"]
    },
    "allowedMethods": ["GET", "POST"],
    "requiresAssurance": "AL2",
    "audience": "https://api.acme.example",
    "operations": [
      {"name": "list", "methods": ["GET"], "pathPrefix": "/accounts"},
      {"name": "transfer", "methods": ["POST"], "pathPrefix": "/transfers",
       "requiresAssurance": "AL3",
       "transaction": {"amount": "/amount", "currency": "/currency", "payee": "/to/iban"}}
    ]
  }}'
```

| Field | Meaning |
|---|---|
| `baseUrl` | `https`. `http` only for an upstream that allows private addresses. No credentials, query or fragment |
| authentication | One of `mtls` (`clientCertSecretRef`), `oauthClientCredentials` (`tokenUrl`, `clientId`, `clientSecretRef`, `scopes`), `apiKey` (`header`, `secretRef`) |
| `allowedMethods` | At least one |
| `requiresAssurance` | The lowest level any call needs. An operation may ask for more, never less |
| `operations` | If you list any, a call must name one; each narrows methods and path and carries its own policy |
| `operations[].transaction` | Makes the operation a transaction: JSON Pointers to the amount, currency and payee in the request body ([SCA](sca.md)) |
| `operations[].plugins` | Limits the operation to named plugins that declare the capability (`SEC-080`) |
| `allowPrivateAddresses` | Off by default. Needed when your API lives on a private network |
| `timeoutMs` | One call's bound; `0` uses `gateway.timeout` (10 s) |
| `audience` | The `aud` of the assertion; the base URL when empty |

Credentials are never in the upstream record. A `…Ref` names an **environment secret**
(`AppService.SetSecret`), sealed at rest (`SEC-106`). An mTLS secret holds the client
certificate and key. Rotate a credential by setting the secret again. A replica keeps the connections it built for
up to 5 minutes, so the change reaches every replica within that time; `UpdateUpstream` makes a
new version and forces new connections at once.

Use `GetUpstream`, `ListUpstreams`, `UpdateUpstream` and `DeleteUpstream` to manage the
rest. A call to an unregistered name is refused with `PLX-6060`.

## 4. What the gateway checks

In order, before it forwards anything:

1. the device's rate, `gateway.requestsPerMinutePerDevice` (600 by default): `PLX-6064`;
2. the registered upstream: `PLX-6060`;
3. the method and the path, which may not leave the base URL: `PLX-6061`;
4. the operation, its path and its plugins (`SEC-080`);
5. the assurance level of the upstream and the operation: `PLX-6002`;
6. the user token, when the source is `auth` (§6): `PLX-6016`;
7. for a transaction, the challenge, the dynamic linking and the signature:
   `PLX-6070` to `PLX-6072`.

Sizes and time are registry limits: `gateway.requestBytes` (1 MiB), `gateway.responseBytes`
(8 MiB), `gateway.timeout` (10 s). An operator may tighten them in `limits:` of
`plux-server.yaml`, never raise them past the hard maximum (`LIM-002`).

**SSRF.** The gateway reaches only registered upstreams. Private and loopback addresses are
refused unless the upstream allows them, checked again after DNS resolution and after
redirects. Link-local and cloud-metadata addresses are never reachable. Redirects are not
followed. Device credentials and DPoP headers are stripped before the call. Bodies are never
logged, traced or stored.

## 5. What your API receives

Your API receives the normal request plus:

```text
Plux-Assertion: eyJ0eXAiOiJwbHV4LWFzc2VydGlvbitqd3QiLC…
```

An ES256 JWT, `typ` `plux-assertion+jwt`, valid for 60 seconds. Verify it on every request:

1. Fetch the keys from `https://<your Plux server>/.well-known/plux/gateway-jwks/production`
   (`…/development` for non-production environments) and cache them by `kid`. Refresh when
   you see an unknown `kid`.
2. Check the signature, `typ`, `exp` and `iat`.
3. Check `iss` is your Plux server's public base URL and `aud` is the audience you registered.
4. Optionally bind it to the request: `rh` is SHA-256 over the method, the path with its
   query and the body hash, each preceded by its length as four big-endian bytes. The body
   hash is empty for a streamed body.
5. Accept nothing about the caller from anywhere else. The device sends no credentials of its
   own.

The same keys sign access tokens (`at+jwt`) and user tokens (`plux-user+jwt`). **Check `typ`.**
An access token is not an assertion.

| Claim | Meaning |
|---|---|
| `sub` | Device ID |
| `app`, `env`, `org` | App, environment, organisation |
| `al` | The device's assurance level |
| `usr` | `{sub, iss, claims}` of the verified end user, after a token exchange (§6) |
| `sca` | `{cid, sh}`: the challenge ID and the base64url SHA-256 of the signed summary, for a transaction the server verified |
| `jti`, `iat`, `exp`, `rh` | Uniqueness, time (60 s) and request hash |

For a transaction your API sees `sca` and nothing else about the signature. **Do not verify
device signatures.** Compare the amount, currency and payee in the request with your own
records if you want a second check; the gateway already guaranteed they equal what the user
confirmed.

## 6. Who the user is

Plux knows the device; your identity provider knows the user. For a data source with
`"auth": true`:

1. The host app's `PluxAuthDelegate` provides its token (`HST-010`).
2. The runtime exchanges it with the Plux server (RFC 8693, `TokenService.ExchangeUserToken`).
   The server verifies it against your identity provider's keys and issues a Plux user token
   bound to the device's key.
3. The device sends that token in the `Plux-User-Token` header on gateway calls and the
   assertion carries the verified user in `usr`.

Tell the server about your identity provider once per environment:

```bash
curl -sS "$PLUX_SERVER/plux.v1.SecurityAdminService/SetUserIdentityProvider" \
  -H "Authorization: Bearer $PLUX_TOKEN" -H 'Content-Type: application/json' -d '{
  "provider": {"appId": "…", "environmentId": "…",
    "issuer": "https://idp.acme.example", "jwksUrl": "https://idp.acme.example/jwks",
    "audiences": ["plux"], "forwardClaims": ["email", "roles"]}}'
```

The JWKS URL must be `https` and is fetched through the SSRF-safe client. Symmetric
algorithms are refused. `sub` is always forwarded; list other claims in `forwardClaims`.
A token the provider rejects is `PLX-6016`; the runtime refreshes it once through the
delegate.

## 7. Streams and transfers

`/gateway/v1/{upstream}/{path}` carries streaming bodies, server-sent events and WebSocket
upgrades, with `Authorization: DPoP <token>` and a proof for that method and path. The typed
fields travel as headers: `Plux-Operation`, `Plux-Plugin`, `Plux-User-Token`,
`Plux-Challenge-Id`, `Plux-Transaction-Signature`. Bodies stream through without being
buffered whole; only a transaction's body is read, bounded by `gateway.requestBytes`.

## 8. Operating the gateway

**Roles.** The `api` role serves the gateway. To scale it apart, run replicas with
`server.roles: [gateway]`. It needs the database, the cache and token signing, and nothing
else. A replica never holds a release-signing capability.

**Release check.** Creating a release compares your documents with the registry, because the
registry is the server's truth and the documents are what devices follow. Each mismatch is
an error at the data source in the document:

- a gateway source with no upstream of its name in the environment: `PLX-6060`;
- an upstream or operation that requires less assurance than the source's
  `requiresAssurance`: `PLX-6002`;
- a source calling operations the upstream does not register, when it registers any:
  `PLX-6061`;
- a `transaction` mapping that differs from the registered one, or a registered
  transaction the document lacks: `PLX-6072`.

Register the upstreams of an environment before you promote a release to it.

**Metrics.** A finished gateway call is recorded as an RPC of service `plux.gateway`, with
the upstream name as `method` and the HTTP status as `code`:

```promql
# 5xx share per upstream over 5 minutes
sum by (method) (rate(plux_rpc_requests_total{service="plux.gateway", code=~"5.."}[5m]))
  / sum by (method) (rate(plux_rpc_requests_total{service="plux.gateway"}[5m]))

# p95 added and upstream time, per upstream
histogram_quantile(0.95, sum by (le, method) (rate(plux_rpc_duration_seconds_bucket{service="plux.gateway"}[5m])))
```

Refusals before the upstream (`PLX-6060` to `PLX-6064`) appear in the Connect RPC metrics of
`plux.v1.GatewayService` instead. Failure handling is in the
[gateway upstream failure runbook](../runbooks/gateway-upstream-failure.md).

**Rotating the Plux assertion key.** The gateway signs with the token key of the
environment's class. A rotation adds a version to the published key set; upstreams that
cache by `kid` pick it up. A compromise is handled in the
[key compromise runbook](../runbooks/key-compromise.md).

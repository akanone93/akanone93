<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# The secure gateway

How a device reaches a customer's API through the server (`SEC-030`–`SEC-032`,
[ADR-0056](../adr/0056-secure-gateway-and-sca.md), which also defines the wire formats).

## Calls

| Path | For |
|---|---|
| `plux.v1.GatewayService/Call` | A request and response that fit in one message |
| `/gateway/v1/{upstream}/{path}` | Streaming bodies, server-sent events and WebSocket |
| `plux.v1.ScaService/CreateTransactionChallenge` | The challenge of a transaction |
| `plux.v1.TokenService/ExchangeUserToken` | The host's user token for the token the gateway forwards (RFC 8693) |
| `plux.v1.DeviceService/EnrolScaKey` | The device's SCA key, after registration |
| `plux.v1.ConfigService/GetLiveConfig` | The environment's `live` configuration values (`DAT-094`) |
| `/.well-known/plux/gateway-jwks/{production,development}` | The keys that verify `Plux-Assertion` |

All device calls carry `Authorization: DPoP` and a proof. The `api` role serves them, and the
`gateway` role serves them alone so that the gateway scales apart.

## What a call is checked against

In order: the device's rate (`gateway.requestsPerMinutePerDevice`, `PLX-6064`); the registered
upstream (`PLX-6060`); the method and the path, which may not leave the base URL (`PLX-6061`);
the operation, its path and its plugins (`SEC-080`); the assurance level of the upstream and
the operation (`PLX-6002`); the user token; and for a transaction the challenge, the
dynamic linking and the signature (`PLX-6070`–`PLX-6072`, `PLX-6073`). Request and response
sizes and the timeout come from `gateway.requestBytes`, `gateway.responseBytes` and
`gateway.timeout`; a challenge lives `sca.challengeLifetime`. Redirects are not followed.

## Administering upstreams

`SecurityAdminService` creates, reads, lists, updates and deletes upstreams (`app.manage` to
change, `app.read` to read; each change is audited) and sets the identity provider of an
environment's users. An upstream names its base URL (https; http only with private
addresses allowed), the methods it allows, its operations and one way to authenticate:
mutual TLS, OAuth 2.0 client credentials (the token is kept until it expires) or a key in a
header. Credentials are environment secrets, referenced by key and sealed at rest (`SEC-106`).

## Server-held values in a request

An operation may bind values the server holds into the request it forwards
(`DAT-091`, [ADR-0061](../adr/0061-server-held-configuration.md)). The values are an
environment's server-held configuration (`plux config`, `SecurityAdminService.SetConfigValue`);
the operation says where each goes, and the device never sends, chooses or sees it. Use this
for what must be current when it takes effect, such as a service provider's pool account.

| Location | The gateway |
|---|---|
| `json_pointer` | Sets that member of the JSON body (RFC 6901), creating the objects on the way. The body must be strict JSON with an object at its root; an empty body stands for `{}`. Numbers the device wrote keep their text. Only for operations that allow POST, PUT or PATCH. |
| `header` | Sets that request header. The names `Plux-*`, `Host`, `Cookie` and the framing headers are not allowed. |
| `path_param` | Inserts the value as one escaped segment directly after the operation's path prefix, in the order of the bindings; the operation needs a path prefix. A value that would need a separator or is a dot segment is refused. |
| `query_param` | Appends `name=value` to the query. |

A request that sets a bound location, a body member, a header or a query parameter, is refused
(`PLX-6065`). A value is bound as its type: an integer as a JSON number, a boolean as a JSON
boolean, a string or a decimal as a JSON string. Only `server-bound` values can be bound; one
that is not set, or is `live`, refuses the call (`PLX-8033`). Operations without bindings keep
streaming their bodies untouched; one with a JSON Pointer binding reads its body, up to
`gateway.requestBytes`.

### No stale read

Each replica remembers an environment's values with its **epoch**, which rises with every
change. A change writes its epoch to the cache store (`plux:config:version:<app>:<environment>`)
before it commits; every call that binds a value, or carries a configuration version, reads that
key and reloads from the database when it is not the epoch the replica holds. A key the cache
store lost is seeded from the database, and a replica reloads at least once a minute. If the
epoch cannot be read, the call is refused (`PLX-6069`): the gateway fails closed. A call that
needs no configuration still reads the key, for the header below, and goes on without it.

### Epoch and live version

Every response to a call that passed authorisation carries `Plux-Config-Epoch`, the
environment's epoch (`DAT-095`). `ConfigService.GetLiveConfig` returns the environment's `live`
values (never server-bound or sensitive ones) with a `version`, the epoch at which a live value
last changed, and the `epoch`. A device sends that version in `Plux-Config-Version` with every
call (`DAT-096`); if a value marked `strict` changed after it, the gateway refuses the call
before the upstream hears of it (`PLX-6067`). A call without the header is not checked.

### Transactions

A binding with `display` must sit at a JSON Pointer the operation's transaction maps (amount,
currency, payee or summary). `CreateTransactionChallenge` resolves it, merges it into the
summary as that member (the device's summary must not carry it), returns the merged canonical
summary in `transaction_summary_jcs`, which the SCA key signs, and the label and value in
`displayed` for the confirmation sheet (`DAT-093`). A sensitive value is never shown. When the
call arrives, the value is resolved again; if it differs from the one the user confirmed, the
call is refused without spending the challenge (`PLX-6066`) and the device asks for a new one.

## Releases are checked against the registry

The registry is the server's source of truth; the documents are what the device follows. So
creating a release compares the two (`SEC-028`, `SEC-030`): every data source that goes through
the gateway (any network source not declared `route: direct`) needs an upstream of its name in
the release's environment (`PLX-6060`); the upstream and the operations the source calls must
require at least the source's `requiresAssurance` (`PLX-6002`); the operations the source
declares must be registered if the upstream registers any (`PLX-6061`); and a source's
`transaction` mapping must equal the mapping of the registered transaction operations, and an
operation registered as a transaction must be the document's too (`PLX-6072`). Each mismatch is
an error at the source in the document.

## Measured

`go test -bench . ./internal/gateway` reports `authorise-p95-ms`, `added-p95-ms` and
`verify-p95-ms` for `NFR-025`, and `epoch-check-p95-ms`, `unbound-p95-ms` and `bound-p95-ms` for
the cost of server-held values (the epoch check is one cache-store read, budgeted at 0.2 ms p95).

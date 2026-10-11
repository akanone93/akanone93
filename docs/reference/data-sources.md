<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Data sources

REST and GraphQL data sources (`DAT-001`, ADR-0048), as the compiler checks them and the
runtime loads them from P5 R4. A source's `config` is validated for the kinds `rest` and
`graphql`; the other kinds keep their P1 behaviour until their milestone. A bundle that
declares a REST or GraphQL source requires the feature `data.v1`, first in runtime 0.3.0.

## Configuration

| Property | Kinds | Meaning |
|---|---|---|
| `baseUrl` | both, required | An app variable of type `string`; every environment gives it an HTTPS URL on a domain the plugin declares (`DAT-003`, `PLX-1170`, `PLX-1171`). The bundle carries the URL of every environment; the runtime uses `PluxConfig.environment`'s. |
| `method` | rest | `GET` (default) or `POST`. |
| `path` | rest (required), graphql | Starts with `/`; `{name}` placeholders take parameters. |
| `query` | graphql, required | The query document, sent with `POST`. |
| `params` | both | Literals or `$expr` bindings in the declaring scope: path placeholders, then the query (`GET`) or a JSON body (`POST`); GraphQL variables. |
| `headers` | both | Literal headers; credentials (`Authorization`, `Cookie`, API keys, tokens) are refused (`PLX-1174`). |
| `auth` | both | Sends the auth delegate's token; one refresh on `401`, then one retry (`HST-010`). |
| `select` | both | A dot path (`items`, `data.0.name`) to the value (`DAT-004`). |
| `responseType`, `transform` | both | Together: the selected value is read as `responseType` and `transform` (PXL over `response`) returns the source's type (`PLX-1172`). |
| `cache` | both | `policy` (`networkOnly`, `cacheFirst`, `networkFirst`, `staleWhileRevalidate`), `ttlSeconds` (required except for `networkOnly`), `key`, `encrypted` (`DAT-010`). Responses of `auth` sources are always encrypted at rest. |
| `pagination` | both | `style` (`cursor`, `page`, `offset`), `pageSize` ≤ `data.pageSize`, `sizeParam`, `cursorParam` and `nextCursor`, `pageParam` and `firstPage`, `offsetParam`, `hasMore` (`DAT-011`, `PLX-1173`). The source's type is a list. |
| `mocks` | both | `empty` (a value of the type) and `error` (`kind`, `status`, `message`); `success` is the source's `mock` (`DAT-080`). Only development bundles carry mocks; release bundles carry none. |
| `operations` | both | Named operations for `apiCall`: `method` and `path` (REST) or `query` (GraphQL), `input` (an object type), `output`, `select`, `headers`, `auth`. |

## At run time

`data.<name>` is `{value, loading, error, hasMore, status}`: a list binds `items` to
`data.<name>.value ?? []`, `status` to `data.<name>.status` and `hasMore` to
`data.<name>.hasMore`, and its `onEndReached` runs `refreshData` with `more: true`. A source
loads when a binding first reads it. `apiCall` runs `"<source>.<operation>"`; `refreshData`
reloads bypassing the cache.

Requests leave from the data isolate only after their host is checked against the plugin's
`networkDomains` over HTTPS; others fail with `PLX-5100` and are reported. Failures are typed
(`PLX-5101`–`5109`) and never carry bodies, query strings or headers. Sizes and time come
from `data.requestSize`, `data.responseSize`, `data.requestTimeout`; the cache keeps within
`data.cacheBytes` and `data.cacheEntries`. Each request records `api_call` with source,
operation, status, duration and bytes. Mock states are selected only in tests and
development builds, never in release builds, whose bundles carry no mocks.

Encrypted cache entries are sealed under a key made for the installation and kept by the
platform's secure storage, like the secure state store's; without one they are not cached
(`PLX-5109`). The `logout` action and `Plux.wipeData` remove every cached response, so the
next user never sees the previous user's; `logout` then calls the auth delegate's
`onLogout`. A source's loads and failures fire its `onLoaded` and `onFailed` triggers.

## Streams, the outbox and transfers

These complete the data layer in P5 R5 ([ADR-0048](../adr/0048-data-layer.md)).

**Streams** (`DAT-012`). A source of kind `websocket` or `sse`, or a GraphQL operation
that subscribes, delivers messages instead of one response. `baseUrl`, `path`, `params`,
`auth` and `select` work as above, over `wss` and `https` only. `subscribe` opens a stream
with its parameters, and `unsubscribe` closes it; a stream also closes with the scope that
opened it. `data.<name>.value` holds the latest message, mapped to the source's type, and
each message fires the source's `onMessage` trigger with it as `event`. A dropped
connection reconnects with capped exponential backoff and jitter (`data.streamBackoffMin`,
`data.streamBackoffMax`) and resubscribes; SSE resumes with `Last-Event-ID`. A message
over `data.streamMessageSize` closes the stream (`PLX-5111`). More than `data.streamsOpen`
open streams per plugin are refused (`PLX-5112`). A WebSocket refused with `401` or `403`
keeps retrying with capped backoff, since `dart:io` does not expose the status; SSE treats
a `4xx` as final (`PLX-5110`). WebSockets connect with the host's
`PluxConfig.webSocketClient` when it gives one.

**The outbox** (`DAT-020`). An operation marked `offlineCapable` that fails with `network`
or `timeout`, or runs while the host has said the network is down
(`Plux.setNetworkAvailable(false)`), is queued instead: its operation, resolved input and an
idempotency key, encrypted in the built-in store. No token is stored. The step succeeds
with a `null` output, and its optimistic state change stays. Entries replay in order,
one at a time per plugin: on app resume, after any successful request, on a backoff timer
(`data.outboxBackoffMin` to `data.outboxBackoffMax`) and when the host reports the network
back. Each attempt sends the same `Idempotency-Key` header, so a server can answer a
repeated mutation without applying it twice. A `2xx` fires the source's `onSynced`
trigger; `409` and `412` fire `onConflict` (`PLX-5122`); another `4xx` fires
`onSyncFailed` (`PLX-5123`); a `5xx`, `network` or `timeout` leaves the entry for the next
replay. A full outbox (`data.outboxEntries`, `data.outboxBytes`) refuses the mutation
(`PLX-5120`). Without the platform's secure storage there is no outbox (`PLX-5121`).
`Plux.wipeData` and `logout` clear it. Mark only operations that are safe to apply later:
the Plux Bank reference app keeps its transfers online.

**Transfers** (`DAT-031`). An upload streams a file, such as one `pickFile` returned, as a
multipart or raw body. A download streams to the app's private storage and returns its
path. Both report progress through `onProgress`, are cancelled with their run, and are
bounded by `data.uploadSize` and `data.downloadSize`: checked against the declared length
first, then counted during the transfer (`PLX-5130`; `PLX-5131` for a file that cannot be
read). A transfer is a step, so it is also bounded by `action.stepTimeout`; raise that
limit for large files.

## Routing, user identity and transactions

These are the device's side of the secure gateway ([ADR-0056](../adr/0056-secure-gateway-and-sca.md)).

**Route** (`SEC-030`). A source's `route` is `plux` (the default) or `direct`. A `plux` request
leaves the device for the Plux server (`GatewayService.Call`) with the device's access token
and a DPoP proof, made on the sync isolate; the server forwards it to the upstream it has
registered for the source, so the bundle carries no base URL for it and `networkDomains` do not
apply. The request names the upstream by the source's name and the operation by its name
(empty for a load). A `direct` request goes to the origin as described above. Where the
security profile refuses direct calls (`allowDirectDataSources`, off under `maximum`), a
`direct` source fails with `PLX-6063` at run time, is reported, and nothing is sent; the
runtime never falls back from one route to the other. The offline outbox replays through the
same route with the same `Idempotency-Key`. Streams (`websocket`, `sse`, GraphQL
subscriptions) and file transfers have no gateway route yet: a `plux` source that uses them
fails with `PLX-5108`, so declare them `direct` (where the profile allows it).

**User identity** (`SEC-026`). For a source or operation with `auth`, the gateway route does not
send the host's token to the upstream. The runtime hands the auth delegate's token to the
sync isolate, which exchanges it with the Plux server (RFC 8693, audience = the source's
name), keeps the Plux user token in memory until shortly before it expires, and sends it in the
`Plux-User-Token` header of the gateway call; the upstream sees the verified user in the
server's assertion. A token the identity provider rejects (`PLX-6016`) or a `401` is refreshed
once through the delegate.

**Transactions** (`SEC-027`, `SEC-028`, `SEC-032`). A source with a `transaction` mapping
treats every mutating operation as a transaction. Before it is sent the runtime reads the
amount, currency and payee from the **request body** by the mapping's JSON Pointers (from the
`variables` of a GraphQL request), asks the server for a single-use challenge bound to the
RFC 8785 canonical JSON of `{"amount","currency","payee"[,"summary"]}`, shows that text on a
confirmation sheet, asks for the user's biometric or device credential, and signs
`challenge ‖ summary` with the SCA key (ES256, so the signature is over
`SHA-256(challenge ‖ summary)`). The call carries `transaction_challenge_id` and
`transaction_signature`. A mapped field that is not in the body (a path placeholder took it)
fails the request before anything is asked (`PLX-6072`); a transaction on a `direct` source
fails with `PLX-6073`; a transaction is never queued in the outbox and never repeated after a
`401`. The `signTransaction` action (`transaction` = `"<source>.<operation>"`, `amount`,
`payee`) runs the same steps ahead of the `apiCall` and holds the confirmation for the call
that sends the same transaction: a different one fails with `PLX-6072`, and the
confirmation is spent either way. Its output is the challenge ID; the `declined` branch is
taken when the user declines.

The SCA key is made at registration when `PluxConfig.strongCustomerAuthentication` is set
(a device registered earlier registers again, once, the first time it signs). It needs a
screen lock; where it cannot be made the device registers without it and a transaction
fails with `PLX-6017`. On Android the host activity must be a `FlutterFragmentActivity`; on
iOS `Info.plist` needs `NSFaceIDUsageDescription`.

**Local database sources.** A source of kind `database` watches a query over a local collection
(`DB-006`): see the [database guide](../guides/database.md).

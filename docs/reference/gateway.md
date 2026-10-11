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

## Measured

`go test -bench . ./internal/gateway` reports `authorise-p95-ms`, `added-p95-ms` and
`verify-p95-ms` for `NFR-025`.

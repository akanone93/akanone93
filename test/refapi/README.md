<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# refapi

The reference backend of the reference apps (`DX-004`): the Plux Bank and Plux Express APIs
their end-to-end flows call. Standard library only; state lives in memory and is seeded the
same way on every start.

It serves **HTTPS and WSS only** — the runtime refuses cleartext data URLs. The certificate is
signed by a CA (ECDSA P-256) that `refapi` generates at start, valid for `localhost`,
`127.0.0.1` and `::1`; the CA's certificate (public) is written to `-ca-out`. The apps trust
it only in end-to-end mode, through `PluxConfig.httpClient` and `PluxConfig.webSocketClient`.

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `127.0.0.1:0` | Address to listen on |
| `-ca-out` | required | File the CA certificate (PEM) is written to |
| `-track-step` | `2s` | Pause between the status steps of an order's tracking stream |
| `-heartbeat` | `15s` | Interval of the notification stream's heartbeat comments |
| `-assertion-jwks` | none | URL of the Plux server's assertion keys, `/.well-known/plux/gateway-jwks/{production,development}`; the bank refuses every request without it |
| `-assertion-issuer` | none | The Plux server's public base URL: the `iss` of its assertions |
| `-assertion-audience` | `refapi` | The `aud` the upstreams were registered with |

Once it is listening it prints exactly one line, `refapi listening on https://HOST:PORT`, and
shuts down on SIGINT or SIGTERM.

## Plux Bank (`/bank`)

The bank is a **customer backend behind the Plux gateway** ([gateway guide](../../docs/guides/gateway.md)).
It trusts one thing about its caller: the `Plux-Assertion` the gateway adds (ES256, `typ`
`plux-assertion+jwt`, checked against the server's published keys for `iss`, `aud`, time and the
request hash `rh`). Without a valid assertion every bank endpoint answers `401 unauthorised`; a
bearer token or a device credential is never looked at. An endpoint then asks the assertion for the
verified user (`usr`, else `403 user_required`) and a transfer for the proof that the user
confirmed it (`sca`, else `403 sca_required`); the bank keeps the proof's challenge ID and summary
hash with the payment.

Users sign in at a **test identity provider** in the same process: `POST /bank/v1/login` checks
`demo` / PIN `246810` / code `123456` (the one-time password an SMS would carry) and returns an ES256 JWT (`iss` = `<url>/idp`, `aud` = `plux`, `sub` =
`user-demo`, `tier` = `standard`); its keys are at `GET /idp/jwks.json`. An environment registers it with
`SetUserIdentityProvider` (forwarding the `tier` claim, which `bank.transferFee` reads), and the gateway exchanges the host's token for the user it names.

| Endpoint | Needs | |
|---|---|---|
| `POST /bank/v1/login` | a device | Body `{username,pin,otp}`; `{"token","expiresIn"}`; `401` for other credentials |
| `GET /bank/v1/accounts` | a user | `{"items":[{id,name,iban,balanceCents,currency}]}` |
| `POST /bank/v1/transfers` | a confirmed user | Body `{fromAccountId,toIban,amount,currency,reference}`, the amount a decimal string (`"12.50"`); `201 {"id","status","balanceCents"}`; `422` for an unknown account, an amount that is not positive, over the balance or malformed, another currency, a malformed IBAN; idempotent on `Idempotency-Key` (else on the assertion's `jti`) |
| `POST /bank/graphql` | a user | The queries `accounts` and `transactions(accountId, first, after)` only — a fixture's server, not a GraphQL engine |
| `GET /bank/v1/notifications` | a user | Server-sent `transfer` events with `id:`; resumes from `Last-Event-ID`; heartbeat comments |

## Plux Express (`/express`, no auth)

| Endpoint | |
|---|---|
| `GET /express/v1/products?cursor=&limit=` | 40 seeded products, cursor pagination (`items`, `next`, `more`) |
| `GET /express/v1/products/{id}` | One product |
| `POST /express/v1/orders` | `201 {"id","status":"placed","totalCents"}` |
| `GET /express/v1/orders/{id}/track` | WebSocket: `placed`, `picked_up`, `on_the_way`, `delivered`, one per `-track-step`, then a normal close |
| `GET /express/v1/courier/jobs` | The courier's jobs |
| `POST /express/v1/courier/jobs/{id}/deliveries` | `201`; requires `Idempotency-Key`, so an outbox replay delivers once |
| `GET /express/v1/courier/jobs/{id}/deliveries` | The deliveries applied, which the flows read to prove it |

Errors are `{"error":{"code","message"}}`.

## In the end-to-end tests

`backend/internal/server/reference_e2e_integration_test.go` builds this module, starts it with
`-ca-out <tmp>/ca.pem -track-step 200ms` and the server's assertion flags, reads the listen line, and passes the CA (base64) and
the URL to the apps' flows as `PLUX_REFAPI_CA` and `PLUX_REFAPI_URL`. Run the module's own tests
with `go test -race ./...` from this directory.

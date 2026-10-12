<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Server-Held Configuration Guide

How to keep a value of your app current at the moment it takes effect, even on a device that
runs an old bundle, is offline or has a wrong clock. This is `DAT-090` to `DAT-097`; the design
is [ADR-0061](../adr/0061-server-held-configuration.md), the wire details are in the
[gateway reference](../reference/gateway.md#server-held-values-in-a-request).

## 1. Choose a tier per value

| Tier | Where the value lives | Use it for | Freshness |
|---|---|---|---|
| **server-bound** | On the server only. The gateway puts it into the request it forwards; the device never sends, chooses or sees it. | Values an API call needs: a pool or settlement account, a merchant ID, an API version. May be `sensitive`. | Current at the moment of the call (`DAT-092`). |
| **live** | On the server; the device fetches it over its DPoP-bound channel and holds it in memory only. | Values the device itself needs and that must be current: a transfer limit shown on a page, a feature switch. Never sensitive. | `strict` values are checked on every gateway call (`DAT-096`); others are refreshed when the epoch moves (`DAT-095`). |
| **bundled** | In the bundle, as an environment variable (`GOV-010`). | Values that may lag: colours, copy, URLs of public pages. | **May lag**: a device sees a change only after it syncs the new release. |

Rule of thumb: if a stale value would move money or break a contract, it is **server-bound**;
if a page must show the current value, it is **live** and `strict`; otherwise **bundled**.

## 2. Set values

```sh
plux config set poolAccount --tier server-bound --value-stdin --sensitive --env production
plux config set transferLimit --tier live --type integer --value 5000 --strict --env production
plux config list --env production
```

Changes need `app.manage` and are audited. A sensitive value is read from standard input,
sealed at rest and never shown again. `--expected-version` refuses a change when someone else
changed the value since you read it.

## 3. Bind server-bound values into an operation

An upstream operation declares where each value goes: a JSON Pointer into the body, a header,
a path parameter or a query parameter. A request from the device that already sets one of
those locations is refused (`PLX-6065`). For a transaction, mark the binding `display` and
give it a `label`: the value is resolved when the challenge is created, shown on the
confirmation sheet and signed with the amount and currency (`DAT-093`). The page leaves the
payee out of the request; the server fills it in.

If a displayed value changes between the confirmation and the call, the gateway refuses it
(`PLX-6066`) and the runtime asks the user to confirm once more, now with the new value.

## 4. Read live values on a page

The `readConfig` action returns the value of a key, typed: `string` for a string or a decimal,
`int` for an integer, `bool` for a boolean.

- A `strict` value is read from the server each time. Offline, or when the server cannot
  answer, the step fails with `PLX-6068`; handle it with `onError` (`DAT-097`).
- A value that is not `strict` falls back to the last value this process received. Nothing is
  written to disk, logged or traced (`DAT-094`).

Every gateway call names the live version it used (`Plux-Config-Version`). When a `strict`
value changed since, the gateway refuses with `PLX-6067`, the runtime refreshes and repeats
the call once with the same idempotency key, and the user sees nothing unless the repeat fails.

## 5. Failure modes

| Situation | What happens |
|---|---|
| The cache store that holds the epoch is down | Calls that bind a value or carry a version are refused (`PLX-6069`): the gateway fails closed. |
| A bound value is missing or is `live` | The call is refused (`PLX-8033`); fix the configuration. |
| The device is offline | `strict` live reads fail with `PLX-6068`; server-bound values need no device at all. |

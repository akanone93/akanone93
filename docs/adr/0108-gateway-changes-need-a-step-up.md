<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0108. Changes to the gateway need their own permission and a step-up

- **Status:** Accepted (maintainer, 2026-11-19, build log #167)
- **Date:** 2026-11-19
- **Requirements:** `SEC-103`, `DAT-090`, `SEC-026`, `SEC-031`, `GOV-002`

## Context and problem

The gateway forwards device calls to the customer's API with server-held credentials, believes
the user tokens of one identity provider per environment, and binds server-held configuration
values into requests and transactions, such as the payee account of a donation (`DAT-091`,
`DAT-093`). Creating or changing an upstream, the identity provider or a configuration value
needed only `app.manage`, which asks for no security key (`DAT-090` said so for values).

So a session without the security key — a stolen browser session, say — could:

- point an upstream that names a secret (an API key, a client secret, a client certificate) at
  a host of its choosing, and the gateway would send the secret there;
- trust another token issuer, and so act as any user towards the customer's API;
- replace a server-bound value, such as the payee IBAN shown in the payment confirmation.

Setting a secret itself needs `secrets.manage` and a step-up (`SEC-103`), so the gateway was
the weaker way to the same result. Today only Owner and Admin hold `app.manage`, but a custom
role could hold it without `secrets.manage`.

## Decision drivers

- Security first: changing where a secret goes is a secret change; no path around the step-up.
- Least privilege: a separate permission that custom roles grant on purpose, per environment.
- No change for reads: those who read the configuration keep `app.read`.

## Considered options

1. **A new permission `gateway.manage`**: high risk, step-up, environment scope (this
   decision).
2. **Reuse `secrets.manage`**: no new permission, but its name no longer says what it guards.
3. **Keep `app.manage`**, as `DAT-090` said.

## Decision

Chosen option: **1**.

- `gateway.manage` ("Register and change gateway upstreams, the identity provider whose user
  tokens an environment trusts, and server-held configuration values.") is in the catalogue
  with risk high, step-up and environment scope. The Owner holds it (as every permission); the
  Admin role gains it; no other predefined role has it.
- Creating, changing and deleting an upstream, setting and deleting the user identity provider,
  and setting and deleting a configuration value check `gateway.manage` on the environment
  concerned, which needs a WebAuthn step-up within five minutes (`SEC-103`). The check happens
  inside the transaction, against the environment the change names (`managedEnvironment` in
  `internal/gateway`).
- Reads (`GetUpstream`, `ListUpstreams`, `GetUserIdentityProvider`, `GetConfigValue`,
  `ListConfigValues`) keep `app.read`.
- CI tokens keep their rule: a workload holds no authenticator, so its granted scopes decide
  (`SRV-064`).

## Consequences

- **Positive:** the gateway's configuration is as protected as a secret; a custom role can let
  someone run the gateway of one environment (for example staging) without the app or the
  secrets.
- **Negative:** `plux config set` and the other changes ask for the security key; scripts use a
  CI token with the scope.
- **Verified by:** `TestGatewayChangesNeedGatewayManageAndAStepUp_SEC_103` (each of the seven
  changes refused with `app.manage` and `secrets.manage` alone, and with a step-up older than
  five minutes) and the authorization matrix (the RPCs require `gateway.manage`).

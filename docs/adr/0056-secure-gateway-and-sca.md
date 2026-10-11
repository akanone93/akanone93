# 0056. The secure gateway and strong customer authentication

- **Status:** Accepted (maintainer, 2026-10-07, P6 S0; P6 plan §2.1 B13, B17; §2.2 Q2)
- **Date:** 2026-10-07
- **Requirements:** `SEC-026`–`SEC-028`, `SEC-030`–`SEC-032`, `SEC-105`, `SEC-106`, `NFR-025`

## Context and problem

DPoP and assurance levels protect requests to the Plux server, but data sources call the
customer's APIs, which cannot verify DPoP, attestation or SCA signatures without
reimplementing Plux. The maintainer decided (B13) that signatures are checked only on the
Plux server, that transaction calls travel only through it, and that every other call is
configurable per data source: through the Plux gateway (default) or direct.

## Decision drivers

- The customer's backend trusts one thing: the Plux server's assertion.
- No SSRF; no bodies in logs; per-device rate limits from the registry.
- ≤ 5 ms p95 added at 1,000 req/s per replica (`NFR-025`).

## Considered options

1. **A gateway in `api`, routing to registered upstreams, adding a signed Plux assertion;
   SCA verified only there.**
2. An SDK for customer backends to verify DPoP and SCA themselves.
3. A separate gateway service.

## Decision

Chosen option: **1**.

- **Routing.** A data source's `route` is `plux` (default) or `direct`. Through the
  gateway, the runtime calls `GatewayService.Call` (unary) or the HTTP route
  `/gateway/v1/{upstream}/…` (streaming bodies, WebSocket, SSE) with DPoP. The gateway
  resolves the upstream registered for the app and environment (`PLX-6060`), applies the
  data source's `requiresAssurance`, the request and response size limits, the timeout,
  and the per-device rate (`gateway.*` limits, `PLX-6064`).
- **Destinations.** Only registered upstreams; the request path is joined to the base URL
  and normalised; private, loopback, link-local and metadata addresses are refused unless
  the upstream allows them (`SEC-105`'s client, re-validated after DNS and redirects;
  `PLX-6061`).
- **Upstream authentication.** mutual TLS, OAuth 2.0 client credentials (tokens cached
  until expiry), or a key held encrypted (`SEC-106`). Device credentials and DPoP headers
  are stripped.
- **Plux assertion.** A short-lived JWT (ES256, per-environment key via `signing/`) in a
  `Plux-Assertion` header: device ID, app, environment, assurance level, verified user
  (if any), `jti`, `iat`, `exp` (60 s), audience = upstream. The JWKS is published.
- **No bodies** are logged, traced or stored; metrics carry upstream, status and duration.
- **User identity (`SEC-026`).** `ExchangeUserToken` (RFC 8693) verifies the host's user
  token against the customer's IdP JWKS and issues a Plux user token bound to the device's
  `jkt`; the gateway forwards verified user claims in the assertion (functions in P7).
- **SCA (`SEC-027`, `SEC-028`, `SEC-032`).** A data-source operation with a `transaction`
  mapping requires route `plux`. Flow: `CreateTransactionChallenge` with the JCS summary
  (amount, currency, payee as displayed) → the device shows it, the user authenticates,
  the SCA key (user-authentication-required: Android `setUserAuthenticationRequired` with
  biometric or device credential via `androidx.biometric`; iOS `.biometryCurrentSet`)
  signs `SHA-256(challenge ‖ summary)` → `Call` with the challenge ID and signature. The
  server verifies the signature with the device's registered SCA key, the single-use
  challenge, and that the request's mapped fields equal the signed summary, then forwards
  with an assertion that carries `sca: {challengeId, summaryHash}`. A signature sent any
  other way is refused (`PLX-6073`); failures `PLX-6070`–`PLX-6072`.
- **Direct calls (Q2)** are allowed per `allowDirectDataSources`; the compiler warns that
  `requiresAssurance` cannot be enforced on them.

## Consequences

- **Positive:** customers integrate by trusting one JWT; SCA is verified in one place.
- **Negative:** the Plux server is in the request path (one hop, availability coupling);
  the maintainer accepts this cost.
- **Follow-up:** S8 with the `NFR-025` benchmark; checkpoint 2.

## Dependencies

- **`androidx.biometric`** (Apache-2.0): the system biometric prompt with
  `CryptoObject`, required to unlock a user-authentication-bound key.

## Options in detail

### Option 2: a verification SDK

Every customer stack would need it in its language; security fixes would depend on
customers updating it.

### Option 3: a separate service

Another deployable and another network hop; the `api` role already terminates DPoP.

## Revisions

### 2026-10-11, P6 S8: the wire formats

The decisions above stand. S8 fixed these details, which the device runtime and the
server share; this is the one place that defines them.

**Names.** `CallRequest.upstream` is the name of the data source, and an upstream is
registered under that name (lower-case letter, then up to 63 letters and digits).
`operation` is the operation's name, empty for a data source's load; the server then finds
the operation by method and path, so a path that belongs to a transaction cannot be
reached without its confirmation. The end user's token travels in the `Plux-User-Token`
request header, never in `CallRequest`.

**Transaction summary.** RFC 8785 canonical JSON, an object whose members are all
strings, with the keys `amount`, `currency`, `payee` and, if the data source maps one,
`summary`: sorted keys, no whitespace, for example
`{"amount":"125.50","currency":"EUR","payee":"DE89370400440532013000"}`. The device sends
exactly these bytes as `transaction_summary_jcs` and the server refuses a summary that is
not canonical or has other members. `amount` is a decimal number in a string.

**Signed message.** `challenge ‖ summary`: the 32 random bytes of the challenge followed by
the summary bytes. The SCA key signs it with ECDSA over P-256 and SHA-256 (ES256), and the
signature is 64 bytes, `r ‖ s`, 32 bytes each. Which device, upstream and operation the
challenge is for is bound when the server creates it (single use, lifetime
`sca.challengeLifetime`), not repeated in the message. This refines "signs
`SHA-256(challenge ‖ summary)`" above: the platform hashes the message, so the signed digest
is that hash.

**Dynamic linking.** The upstream's registered operation maps `amount`, `currency`,
`payee` (and `summary`) to JSON Pointers into the request body, which must be strict JSON
without duplicate member names. The gateway compares the amount as a decimal number
(`125.5` equals `"125.50"`) and the others as strings; any difference is `PLX-6072`.
The mapping the server enforces is registered with the upstream (`UpstreamOperation`);
the data source's `transaction` in the document carries the same pointers to the device.

**Assertion.** `Plux-Assertion` is an ES256 JWT, `typ` `plux-assertion+jwt`, signed with
the token key of the environment's class. Claims: `iss` (the installation's public base
URL), `aud` (the upstream's audience, default its base URL), `sub` (device ID), `app`,
`env`, `org`, `al`, `iat`, `exp` (60 s), `jti`, `rh` (SHA-256 over method, path with query
and body hash, each preceded by its length as four big-endian bytes; the body hash is empty
for a streamed body), `usr` (`sub`, `iss`, `claims`) after a token exchange, and `sca`
(`cid`, `sh`: the challenge ID and the unpadded base64url SHA-256 of the signed summary) for
a verified transaction. The same keys sign access tokens (`typ` `at+jwt`) and user tokens
(`plux-user+jwt`), so an upstream must check `typ`. The key sets are published at
`/.well-known/plux/gateway-jwks/production` and `/.well-known/plux/gateway-jwks/development`.

**SCA key enrolment.** In `RegisterAttestedDevice` (`sca_public_key_jwk`, `sca_evidence`) or
later in `DeviceService.EnrolScaKey`. Android: the Key Attestation chain of the SCA key
itself, which must be hardware-backed and name a user authentication type
(`noAuthRequired` absent). iOS: App Attest cannot attest a Secure Enclave key, so a
registration reuses its own App Attest evidence for the key, and a later enrolment brings an
attestation whose client data hash binds the key's thumbprint. A registration that reuses
its evidence leaves the SCA key bound to the attested request only; binding its thumbprint
into the registration's client data hash would close that (a device change).

**Streaming route.** `/gateway/v1/{upstream}/{path}` with the HTTP method the call needs,
`Authorization: DPoP <token>` and a proof whose `htm` and `htu` are that request's method
and path. The typed fields are the headers `Plux-Operation`, `Plux-Plugin`,
`Plux-User-Token`, `Plux-Challenge-Id` and `Plux-Transaction-Signature` (unpadded
base64url). Private and loopback addresses are reached only by an upstream that allows
them; link-local and metadata addresses never.

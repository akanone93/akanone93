<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Strong Customer Authentication Guide

How a user confirms a payment or another sensitive operation with their fingerprint, face or
device credential, how that confirmation is bound to the exact amount and payee, and how
your backend gets a verified result. This is `SEC-027`, `SEC-028` and `SEC-032`; the design
and wire formats are [ADR-0056](../adr/0056-secure-gateway-and-sca.md).

## 1. The model

- The device holds an **SCA key**: P-256, in secure hardware, usable only after the user
  authenticates. Every signature needs a fresh biometric or device credential.
- The user sees the **amount, currency and payee** exactly as they will be signed
  (*dynamic linking*, PSD2). The key signs those values together with a single-use challenge.
- Only the **Plux server** verifies the signature. A transaction call can travel only
  through the gateway, which verifies the signature, the challenge and that the request
  equals what was signed, and then forwards the call to your transaction service with a
  signed assertion. Your service never sees or checks a device signature.

## 2. Enrolment

Set `strongCustomerAuthentication: true` in `PluxConfig`. The runtime then creates the SCA key
at registration and registers it with its own platform evidence:

- Android: a Key Attestation chain of the SCA key itself. It must be hardware-backed and
  require user authentication.
- iOS: the registration's App Attest evidence, which covers the SCA key's thumbprint in its
  binding hash (ADR-0012, Revisions 2026-10-11).

Requirements on the host:

- the device has a screen lock; without one the key cannot be made, the device registers
  without it, and a transaction fails with `PLX-6017`;
- Android: the host activity extends `FlutterFragmentActivity`;
- iOS: `NSFaceIDUsageDescription` in `Info.plist`.

A device registered before the flag was set registers again, once, the first time it signs.
If the user changes their enrolled biometrics the key is invalidated; the runtime enrols a
new one (`DeviceService.EnrolScaKey`) with fresh evidence.

## 3. Mark a data source as a transaction

On the data source, give JSON Pointers (RFC 6901) to the fields in the **request body** that
carry the amount, currency and payee:

```json
{"name": "payments", "kind": "rest", "type": "TransferResult", "route": "plux",
 "transaction": {"amount": "/amount", "currency": "/currency",
                 "payee": "/to/iban", "summary": "/reference"},
 "config": {"baseUrl": "apiBaseUrl",
   "operations": {"transfer": {"method": "POST", "path": "/transfers", "auth": true,
                               "input": "TransferInput", "output": "TransferResult"}}},
 "mock": {"id": "tr-1", "status": "done"}}
```

- Every mutating operation of the source is then a transaction. Reads are not.
- `summary` is optional: a description the user also confirms.
- The source must use route `plux`; `direct` is a compile error (`PLX-6073`).
- Register the same mapping on the upstream operation (`operations[].transaction` in the
  [gateway guide](gateway.md)). The server enforces the registered mapping; creating a
  release fails with `PLX-6072` if the two differ.
- `amount` should be a string (`"125.50"`) or an integer. A field that is missing, or in the
  wrong type, fails before anything is shown (`PLX-6072`).

Then call the operation as any other. The runtime handles the rest. To confirm before a
longer flow, the action `signTransaction` (`transaction`: `"<source>.<operation>"`, `amount`,
`payee`) runs the same steps ahead of the `apiCall` and holds the confirmation for the call
that sends the same transaction; a different one fails with `PLX-6072`. Its branch
`declined` is taken when the user declines.

## 4. What happens, in order

1. The runtime reads amount, currency and payee from the request body.
2. It asks the server for a **challenge** (`ScaService.CreateTransactionChallenge`): 32 random
   bytes, single use, valid `sca.challengeLifetime` (5 minutes by default), bound to the
   device, the upstream and the operation.
3. It shows the **confirmation sheet** (§5).
4. The system **biometric prompt** appears (`androidx.biometric` on Android, `LAContext` on iOS;
   device credential is accepted as a fallback).
5. The SCA key signs `challenge ‖ summary` with ECDSA P-256 and SHA-256: a 64-byte `r ‖ s`
   signature.
6. The call goes through the gateway with `transaction_challenge_id` and
   `transaction_signature`.
7. The gateway verifies the signature with the device's registered SCA key, consumes the
   challenge, and compares the request's mapped fields with the signed summary: the amount as a
   decimal number (`125.5` equals `"125.50"`), the rest as strings. Any difference is
   `PLX-6072`.
8. Only then does it forward to your service with `Plux-Assertion` carrying `sca: {cid, sh}`.

A test pins steps 1 to 6 in this order. A transaction is never queued in the outbox, never
retried after a `401` (its confirmation is single use), and is not replayed offline.

## 5. What the user sees

A bottom sheet titled with the confirmation prompt:

```text
Amount      125.50
Currency    EUR
Payee       DE89370400440532013000
Description March rent           (only when the source maps a summary)

[ Confirm ]   Cancel
```

Each field is shown as it is, unformatted, because it is the text that is signed. Each row is
one unit for screen readers. Confirm opens the system prompt. Cancel, or dismissing the
sheet, or declining the prompt sends nothing; the call ends as cancelled.

## 6. The canonical payload

The *summary* is RFC 8785 canonical JSON of an object whose members are all strings, with
sorted keys and no whitespace:

```json
{"amount":"125.50","currency":"EUR","payee":"DE89370400440532013000"}
```

Keys: `amount`, `currency`, `payee` and, if mapped, `summary`. The device sends exactly these
bytes (`transaction_summary_jcs`); the server refuses a summary that is not canonical or
has other members. The signed message is the 32 challenge bytes followed by the summary
bytes. Which device, upstream and operation the challenge is for was bound when the server
created it, so it is not repeated in the message.

## 7. Errors

| Code | Meaning | What to do |
|---|---|---|
| `PLX-6070` | The signature is invalid | The user retries; check the key was not replaced |
| `PLX-6071` | The challenge is unknown, used or expired | Start again; challenges are single use |
| `PLX-6072` | The request is not what was signed (or the mapping differs) | Fix the document or the registered mapping |
| `PLX-6073` | A transaction signature arrived outside the gateway, or the source is `direct` | Use route `plux` |
| `PLX-6017` | No SCA key on this device | Needs a screen lock; the runtime tries to enrol again |

## 8. Your transaction service

- Accept calls only from the Plux gateway and verify `Plux-Assertion` ([gateway guide](gateway.md) §5).
- For a transaction, require `sca` in the assertion. No `sca`, no transfer.
- Use `jti` for idempotency; the assertion lives 60 seconds.
- Store `sca.cid` and `sca.sh` with the payment as evidence of the user's confirmation.

Plux Bank (`apps/`) is the worked example under `maximum`.

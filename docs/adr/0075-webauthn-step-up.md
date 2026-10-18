# 0075. WebAuthn step-up and the CLI's browser step-up

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `SEC-103`, `GOV-024`, `GOV-032`, `SEC-100`

## Context and problem

Some actions must prove that a human with a hardware-backed credential is present now: approving
a change, publishing to production, key operations, role changes, secret changes, break-glass,
licence and policy changes (`SEC-103`). P6 already registers WebAuthn credentials with an
in-house CBOR/COSE parser. P7 used a stand-in for the device-function step-up that the approval
engine now replaces. The CLI has no WebAuthn authenticator access. How does a step-up work in
Studio and from the CLI without weakening either?

## Decision drivers

- A recent user-verified assertion, not a long-lived flag.
- The CLI must not hold a step-up that another key or process can reuse.
- No new dependency; the existing WebAuthn code is reused.
- Security capabilities are available in every edition (`GOV-032`).

## Considered options

1. A WebAuthn assertion with user verification, recorded on the session; the CLI runs the ceremony in the browser and receives a short grant bound to its DPoP key.
2. Re-enter TOTP for sensitive actions.
3. Implement CTAP2/HID in the CLI to talk to authenticators directly.

## Decision

Chosen option: **1**.

- **Assertion.** A WebAuthn assertion with user verification within the last 5 minutes is
  recorded on the session as `stepUpAt`. Permissions marked `stepUp` in the catalogue
  (ADR-0073) require it. Without it the server answers `PLX-6200`; an old one gives
  `PLX-6201`; an assertion that does not verify gives `PLX-6202`.
- **CLI (maintainer decision Q15).** `plux` opens `https://<server>/stepup?request=<id>` in the
  browser. After the ceremony the server issues a 5-minute **step-up grant bound to the CLI's
  DPoP key thumbprint**. The CLI polls with its DPoP proof and presents the grant on the guarded
  request. A grant used with another key or after expiry is refused.
- **Tests** cover expiry, replay and a grant used with another key (plan §5.2).
- Approval decisions (ADR-0076) require step-up and are recorded as signed records, so the
  assertion is part of the evidence.

## Consequences

- **Positive:** hardware-backed presence for the dangerous actions in every edition; the CLI needs
  no authenticator code; a stolen grant is useless without the DPoP key.
- **Negative:** a CLI user needs a browser for these actions (headless CI must use a service
  principal that is not allowed those actions); a five-minute window is a policy choice that
  operators may want to tighten.
- **Follow-up:** whether the window length becomes a setting is decided in the milestone;
  step-up for dev sessions in P10.

## Options in detail

### Option 1 — session assertion and a DPoP-bound CLI grant

Reuses WebAuthn and DPoP (ADR-0012) already in the code. The grant's binding gives replay
resistance without new cryptography.

### Option 2 — TOTP re-entry

Phishable and not hardware-bound; `SEC-103` names WebAuthn. Rejected.

### Option 3 — CTAP2/HID in the CLI

Platform-specific, large, and a new attack surface in a tool that runs on developer machines.
The browser already solves it. Rejected.

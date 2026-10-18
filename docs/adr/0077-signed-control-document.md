# 0077. The signed control document, kill switches, mandatory updates and silent push

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `SYN-060`, `REL-060`, `REL-070`, `NFR-024`, `SEC-052`, `GOV-024`

## Context and problem

Each channel already carries an unsigned `Control` (plugin and app kill switch, mandatory update)
in `release/control.go` and `control.proto`. An unsigned kill switch is an attack surface: anyone
who can alter the response can disable a feature, or suppress one. P9 needs kill switches at every
scope (plugin, page, action graph, data source, function), mandatory updates and forced rollback
that reach online devices within 60 s at p95 (`NFR-024`), and that cannot be forged. It also
needs to be fast enough to use during an incident. The question: how is the control document
signed, refreshed, applied and kept safe when it cannot be refreshed?

## Decision drivers

- Verify before parse (`SEC-052`); a compromised transport or CDN cannot forge a switch.
- An online signing key is needed (changes must be immediate) but must be as limited as possible.
- A stale document must not silently re-enable a feature that was switched off.
- Propagation within 60 s p95 for online devices; the app keeps working offline.
- No push SDK in the runtime.

## Considered options

1. One FlatBuffers control document per app, environment and channel, signed by a new online `control` role of the update metadata, with expiry and anti-rollback.
2. Sign it with the existing `targets`/`snapshot`/`timestamp` online roles.
3. Keep it unsigned and rely on TLS and DPoP.

## Decision

Chosen option: **1** (maintainer decision Q9).

- **Document.** Kill switches (plugin, page, action graph, data source, function), each with an
  optional targeting rule; flag values per segment and running experiments with allocations
  (ADR-0079); a forced rollback (target release); a mandatory update (release ID and message key).
  Encoded in FlatBuffers, wrapped in the update-metadata envelope, signed by the `control` role
  (ADR-0004 Revision), with a version that only increases and an expiry (default 24 h,
  configurable).
- **Device.** Fetched on resume and every 5 minutes in the foreground (`SYN-060`) with
  `If-None-Match` and DPoP; verified before parsing; older versions are refused (`PLX-3102`),
  bad signatures give `PLX-3100`, an expired document `PLX-3101`. Switches apply without restart:
  pages and plugins show their fallback (`RT-022`), action graphs, data sources and functions
  fail with their documented error.
- **Fail closed for switches only.** When the document cannot be refreshed within its expiry,
  every kill switch it last carried stays in force. The app keeps working otherwise.
- **Mandatory update (`REL-070`).** The device activates the release before showing any Plux page
  again, behind a host-customisable `PluxConfig.updateScreen` (`PLX-3103`).
- **Forced rollback.** The device activates the named earlier release from its retained bundles;
  the anti-rollback rule allows a signed control-document rollback (`PLX-3104`) and the device
  reports `control_applied`.
- **Silent push (Q22).** `Plux.handleSilentPush(Map payload)` triggers an immediate check; the host
  passes the payload. The server sends data-only messages with no content through the operator's
  FCM HTTP v1 and APNs credentials, held in the secret store. Off by default and listed in the
  egress list (ADR-0082).
- **Measurement.** `bench-control` times a switch change to `control_applied` on 20 emulator
  instances, with and without push.

## Consequences

- **Positive:** switches cannot be forged or silently dropped; one signed object also carries flags
  and experiments; a leaked control key can disable features but cannot ship code.
- **Negative:** an online signing key to protect and rotate (runbook); fail-closed means a long
  outage keeps a switch on until the document is refreshed; a new device poll adds server load,
  addressed by `If-None-Match` and the sizing guide.
- **Follow-up:** `schema/control.fbs` and migrations 0032 to 0033 (E3); the key-rotation runbook;
  break-glass wiring from ADR-0076.

## Options in detail

### Option 1 — a dedicated `control` role

Separates what the online key may sign. Compromise is bounded by short expiry and by the root
key staying offline (ADR-0004).

### Option 2 — reuse the existing online roles

Fewer roles, but those keys also sign release metadata; widening their use widens their blast
radius. Rejected.

### Option 3 — unsigned

Cheapest, and it is today's behaviour; it fails `SEC-052`'s intent for anything that changes app
behaviour and cannot give an auditor evidence. Rejected.

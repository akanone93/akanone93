# 0078. Rollouts: bucketing, server-side targeting, schedules and health gates

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `REL-050`, `REL-051`, `REL-052`, `REL-053`, `ANL-001`

## Context and problem

Releases reach every device on a channel at once. Regulated organisations want a staged rollout
(1%, 10%, 50%), targeting by device and app attributes, schedules, and automatic pause or
rollback when a release makes things worse. Targeting inputs include user attributes, which are
personal data. The questions: how do devices get a stable position in a rollout, where is
targeting evaluated, who runs schedules in a multi-replica install, and what signals decide that
a release is unhealthy?

## Decision drivers

- Monotonic buckets: a device in at 10% stays in at 50%.
- Personal data stays off the wire where possible; region must not come from IP.
- Schedules run exactly once across replicas, and survive leader change.
- Health decisions rest on enough data and avoid rolling back healthy releases.
- No native crash SDK (maintainer decision Q13).

## Considered options

1. Hash bucketing, targeting on the server at manifest time, a leader-elected scheduler in `worker`, gates on runtime-reported events.
2. Evaluate targeting on the device from a signed document.
3. Use a third-party rollout service and crash SDK.

## Decision

Chosen option: **1** (maintainer decisions Q10, Q13).

- **Bucketing (`REL-050`).** `bucket = sha256(rolloutSalt || subject) mod 10000`. The subject is
  the user ID when `setUserContext` provided one (sent hashed), otherwise the device ID.
- **Targeting (`REL-051`).** Evaluated on the server when the device fetches its manifest. The
  request carries build, runtime version, OS and version, device class, locale, region (from the
  device's setting, not IP), channel, and hashed or allowed user attributes. Segments are
  documents (`PLX-3152` for an invalid rule); explicit allow and deny device lists serve testers.
  User attributes leave the device only hashed.
- **Schedules (`REL-052`).** Start, staged percentages with durations, end with auto-revert to
  the previous release. The scheduler runs in the `worker` role, is idempotent and leader-elected
  through PostgreSQL advisory locks. Overlapping schedules give `PLX-3153`.
- **Health gates (`REL-053`).** The runtime reports `session_start`, `session_end`, `fatal_error`
  (Plux errors) and uncaught Flutter errors (ADR-0034 Revision). Analytics (ADR-0080) computes
  crash-free sessions, Plux error rate and action, function and custom-metric rates per release.
  The gate compares the candidate with the previous release over a minimum sample and pauses
  (`PLX-3150`) or rolls back (`PLX-3151`) per the rollout's thresholds, notifying through
  ADR-0081. Pause comes before rollback by default.

## Consequences

- **Positive:** rollouts are deterministic and explainable; targeting rules and attributes never
  ship to devices; one scheduler at a time without extra infrastructure; gates need no new SDK.
- **Negative:** the manifest path now evaluates rules, so it needs latency budgets and caching;
  gate quality depends on telemetry consent and volume, so small apps get slow or no decisions;
  fatal native crashes that kill the process before reporting are invisible.
- **Follow-up:** minimum-sample and confidence-bound values are decided in the milestone;
  migrations 0034 to 0035 (E4); a runbook for stuck rollouts and health-gate false positives.

## Options in detail

### Option 1 — server-side, hash-bucketed

Matches how the manifest is already served per device, keeps rules private, and lets an
administrator see exactly which devices were eligible.

### Option 2 — device-side targeting

Good for flags (ADR-0079), which are evaluated from the control document, but for a rollout
the device would need the rules before it knows which release to fetch, and rules would be
visible to anyone with the app. Rejected for rollouts.

### Option 3 — third-party service and crash SDK

New dependencies and data flows to an external party, contrary to the egress and privacy rules
(ADR-0082). Rejected.

<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0092. Background sync

- **Status:** Accepted (maintainer, 2026-10-22; P10 plan D-9)
- **Date:** 2026-10-22
- **Requirements:** `SYN-014`, `SYN-004`, `SYN-012`, `SEC-052`, `LIM-001`

## Context and problem

The runtime syncs every plugin at app start (ADR-0021). A user who opens the app after a long
absence waits for the update or runs the last known good release. `SYN-014` asks for opt-in
background sync so updates are staged before the next start. It was deferred from P3 to P10 with
the other host tooling and needs native code on both platforms. The question: how does a headless
sync run, what may it do, and what does the host have to declare?

## Decision drivers

- Verify before load (`SEC-052`): a background task is not a shortcut around verification.
- Activation stays under the app's policy (`SYN-004`): a release that changes under a running or sleeping app is a surprise.
- Opt-in, battery-aware and best-effort: iOS gives no guarantee that a task runs.
- The interval comes from the limits registry (`sync.backgroundIntervalMinutes`).

## Considered options

1. Native schedulers (Android WorkManager, iOS `BGTaskScheduler`) that run the existing sync worker headless, stage a verified release and never activate it.
2. Foreground-only sync, as today.
3. Android `JobScheduler` and `AlarmManager` directly.
4. A third-party Flutter background-work plugin.
5. Silent push only (ADR-0077).

## Decision

Chosen option: **1**.

- **Opt-in.** `PluxConfig.backgroundSync` is `false` by default (Appendix H.2). The host enables
  it.
- **Android.** WorkManager (`androidx.work:work-runtime`, Apache-2.0) schedules periodic work with
  network and battery constraints, at the interval `sync.backgroundIntervalMinutes`. It is an
  AndroidX library, not part of the platform, so it needs the maintainer's approval as a
  dependency (AGENTS §7) recorded in `dependencies.md` before the Android scheduler merges.
- **iOS.** `BGAppRefreshTask` through `BGTaskScheduler`, a system framework with no dependency,
  using the task identifier the host declares in its `Info.plist`. It is best-effort: the system
  decides when, and whether, it runs.
- **What the task does.** It starts a headless Dart entry point that runs the existing sync
  worker: fetch metadata and manifest, download and patch, verify signatures and hashes, and
  **stage** the release. It never activates a release; activation follows `SYN-004` at the next
  start or safe point. Disk quota and garbage collection are those of `SYN-012`.
- **Errors.** `PLX-3500` background sync failed, `PLX-3501` background sync not configured
  (for example the identifier is missing from `Info.plist`), `PLX-3502` constraints unmet,
  `PLX-3503` staged release verification failed.
- **Tests.** Dart unit tests of the headless entry, Kotlin and Swift unit tests of the
  schedulers, and device tests in the emulator and simulator jobs.

## Consequences

- **Positive:** the app starts on the new release more often without the wait; verification and
  activation rules are unchanged.
- **Negative:** one new Android dependency; iOS runs are not guaranteed, so no flow may depend on
  them; two native schedulers to maintain; a host must declare an identifier on iOS.
- **Follow-up:** the `dependencies.md` row and approval for `androidx.work:work-runtime`; the
  limit key `sync.backgroundIntervalMinutes`; the sample `Info.plist` entry in the host guide.

## Options in detail

### Option 1 — native schedulers, stage only

Described above.

### Option 2 — foreground only

No new code, but it does not meet `SYN-014`.

### Option 3 — `JobScheduler` and `AlarmManager`

No dependency, but constraints, persistence across reboots and back-off are re-implemented and
re-tested on each Android version. Rejected.

### Option 4 — a third-party plugin

Adds a Dart dependency outside the allowlist and a native one beneath it, for the same work.
Rejected.

### Option 5 — silent push only

Complements this decision (the control document, ADR-0077) but needs a push provider and does not
cover the periodic case. Not a replacement.

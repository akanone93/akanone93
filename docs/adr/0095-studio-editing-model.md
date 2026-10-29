<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0095. Studio editing model: locks, autosave, undo, change sets, clipboard and canvas

- **Status:** **Accepted** (maintainer, 2026-10-24; P11 plan M-9)
- **Date:** 2026-10-24
- **Requirements:** `SRV-030`, `SRV-040`, `SRV-041`, `STU-004`, `STU-067`, `STU-069`, `STU-075`

## Context and problem

Studio is the first client that edits the same plugin from several browsers at once, for long
sessions, over networks that drop. The server already has revision-checked writes (`SRV-030`),
snapshots of every accepted write (`SRV-031`) and unary lock leases (`AcquireLock`, `RenewLock`,
`GetLock`). Four things are undecided: when one person may take a lock from another, how the
browser sends edits without losing or overwriting any, where undo lives, and how multi-document
edits and copy and paste stay safe. A fifth is where the editor-only layout of the app canvas is
stored. The functional decisions are F1 to F4, F7 and F8 in
[the Studio functional decisions](../plans/p11-studio-functional-decisions.md); the contracts are
numbered in [the P11 plan](../plans/p11.md) §4.3.

## Decision drivers

- Nobody's edits are lost or silently overwritten (`SRV-030`, `SRV-041`); a takeover is never a surprise.
- Rules that protect data are enforced by the server; the browser only displays them (security and correctness first).
- Sensitive fields never reach browser storage (`SEC-092`); undo must survive a reload and a second tab.
- History stays linear and auditable ([ADR-0015](0015-single-draft-with-snapshots-and-locks.md)).
- Limits come from the registry (`LIM-001`); none is hard-coded here.

## Considered options

1. Server-enforced locks and conflict replay, server-side undo on existing snapshots, atomic change sets.
2. Real-time collaborative editing (CRDT or operational transformation).
3. Browser-only undo in IndexedDB, with lock takeover decided by the client.

## Decision

Chosen option: **1**, because every guarantee sits where it can be tested and audited, and nothing
sensitive is persisted in the browser.

### Locks and takeover (L-1; contract #16)

- A lock is taken on the first edit, not on open, and renewed by a heartbeat from a worker.
  A lease lapses after two missed heartbeats (60 s). A lock is released after
  `studio.lockIdleReleaseMinutes` without an edit, after the autosave is flushed.
- **Takeover rule.** A user with `plugin.lock.override` may take a lock over only when the holder
  made no edit for `studio.lockTakeoverIdleMinutes` (default 5) or the lease lapsed. Any user with
  edit permission may always **request edit**, which notifies the holder.
- **The server enforces the rule.** Lock records carry the holder's last edit time; `AcquireLock`
  with `force` is refused while the holder is active. Until now `force` needed only the permission.
- Every takeover is audited, the holder's snapshot is kept, and the holder is told who took over
  and receives a link to the snapshot. Two tabs of one user share one lock.

### Edit pipeline (contracts #4, #25)

- Editor state is Zustand with Immer. Each edit yields a forward and an inverse JSON Patch;
  every patch carries `test` operations for the values it changes.
- One save is in flight at a time; edits made meanwhile merge and go with the confirmed revision.
  Autosave is debounced by `studio.autosaveDebounceMs`.
- A revision conflict fetches the draft and replays the pending patches; they apply only where the
  tested values are unchanged. After `studio.autosaveReplays` replays in a row, or on any clash,
  autosave pauses and the user chooses (D35: keep mine, take the server's, copy mine as JSON).
- On tab hide, unsent edits are flushed through the generated Connect client with a `keepalive`
  fetch via the BFF. While offline, edits queue in memory only, never on disk, and go in order on
  reconnect (F4).

### Undo on the server (contracts #4, #25)

- Every write carries an **editor-session ID**; the snapshot it creates stores that tag and the
  **inverse patch**. After a reload the browser rebuilds its stack from the session's entries, at
  most `studio.undoDepth` of them. Undo applies the inverse patch as a **new forward write**, so
  history stays linear. Nothing is stored in IndexedDB.
- Only document changes are undo steps; one stack per plugin and a separate one for app-level
  documents. Undo is disabled when read-only or after the lock is lost.

### Atomic change sets (contract #15)

A change set writes several documents and plugins in one transaction: each plugin's revision is
checked and every lock held, or nothing is written. It serves moving a screen with its links
(`STU-055`), cross-plugin undo steps and redirecting incoming links (`STU-070`).

### Clipboard (F7)

A versioned JSON envelope (`application/x-plux+json`, with a plain-text fallback) names the source
and the copied nodes and refers to dependencies by ID. Pasting creates new IDs and remaps
references. Pastes from outside are untrusted: parsed as data, never as HTML, validated by schema
and the Wasm validator (the server validates again), and capped by `studio.clipboardBytes`.
Sensitive values are never copied in clear.

### The `canvas` document (L-10; contract #5)

The position of each box and link on an app canvas, and of each screen on a plugin canvas, is an
editor-only `canvas` document kind per app and per plugin. It is **never compiled into bundles**,
so `CMP-002` is untouched, and it is versioned, locked and undone like any document.

## Consequences

- **Positive:** the takeover rule and the replay cap cannot be bypassed from a client; undo works
  across reloads, tabs and takeovers; no sensitive data in browser storage.
- **Negative:** each write stores an inverse patch (a few hundred bytes); change sets need a
  multi-plugin transaction in the server; a second source of lock time (heartbeat and edit) to test.
- **Follow-up:** limit keys and error codes in the registries (S0.0); the authorisation-matrix
  rows for the changed `force` path.

## Options in detail

### Option 2 — collaborative editing

Removes locks, but needs a merge model for a typed document graph with validation, signing and
deterministic compilation; the invariants of `SRV-030` and `CMP-002` become harder to state and
test. Rejected for now; locks with request-edit meet the requirement.

### Option 3 — undo in the browser

Puts sensitive fields on disk, breaks across tabs and devices, and diverges after a takeover
(`SRV-041`). Rejected.

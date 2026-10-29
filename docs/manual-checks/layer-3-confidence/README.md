<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 3 — Confidence (parked)

**Not written yet.** This layer is written after the AI generation (P12), USSD and super-app
mini-app (P16) phases, as part of the [cross-phase audit](../../plans/audit-phase.md), and
then worked through by the maintainer with Claude, check by check and file by file of the code.

## What it will be

The layer after which the maintainer can say *I know this system*: thousands of small checks,
scenario by scenario and implementation by implementation, each one a nameable feature or a
concise technical concept, in the form of the [README](../README.md), building on one another.

It covers every angle, not only the UI (which is covered completely, being the main surface):

- **Studio:** every screen, panel, dialog, shortcut, state (empty, loading, error, read-only,
  locked), permission, theme, locale (`en`, `de`, `ar` right to left) and keyboard path.
- **API and ConnectRPC:** every service and RPC, its permission, errors and their codes
  (Appendix F), pagination, idempotency, rate limits, the OpenAPI reference.
- **Streams:** WebSocket and server-sent events, dev sessions, resumption across replicas.
- **Security and keys:** every key the system holds (signing roles, function artifacts, device
  keys, DPoP, TLS, the licence), how many there are, where they live, how each is rotated,
  revoked and recovered; attestation, pinning, sessions, step-up, CSP and headers.
- **Server roles and workers:** `api`, `worker`, `builder`, `fnrunner`, `egress`, the render
  worker: what each runs, how to add or remove one, what happens when one dies, queues and
  jobs.
- **Logs, traces and metrics:** every log line a flow writes and when, trace spans, metrics,
  dashboards and alerts.
- **Compiler, bundles and sync:** document model, validation, PXL, FlatBuffers bundles,
  deltas, signatures, manifest, update and rollback on the device.
- **Runtime packages:** `plux_flutter` and every optional package; widgets, actions, state,
  data sources, local database, animation, routing, host API.
- **The canvas:** layout conformance, rendering, tiles, interaction, performance.
- **Functions:** SDK, builds, runners, egress, logs, limits.
- **Operations:** Compose, Helm, HA, backups and restore, air-gapped install, upgrades.
- **Enterprise:** roles, SSO, SCIM, approvals, break-glass, freezes, rollouts, experiments,
  analytics, audit export.
- Everything P12, USSD and P16 add.

Its outline and file layout are decided when it is written; the layer 1 and 2 files are its
starting point.

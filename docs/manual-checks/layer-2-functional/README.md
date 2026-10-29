<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Functional checks

A guided tour of every feature of Plux as it stands after P11. You sign in to Studio, build one new
app (`shop`) from nothing, publish it, put it on a device, then exercise operations and governance.
Do the files in this order; each reuses the resources the earlier ones created (each file lists
"Resources this file adds"). Setup first: [00-setup.md](../00-setup.md).

| # | File | What it covers |
|---|---|---|
| 1 | [studio.md](studio.md) | Sign in, shell, apps grid, new-app wizard, canvases, editors, data, localisation, design system, publish, releases, experiments, analytics, tests |
| 2 | [server-api.md](server-api.md) | The same system through `curl`: health, auth, organisations, apps, drafts and locks, publish, errors, OpenAPI |
| 3 | [cli.md](cli.md) | The `plux` command: login, export, validate, build, publish, create the host app, pull, tests |
| 4 | [runtime-sync.md](runtime-sync.md) | The app on the device: sync, deltas, offline, update and rollback, routing, mixed screens, state, data, database, animation |
| 5 | [dev-loop.md](dev-loop.md) | The Dev app, pairing, live push and the log, trace, state and network streams |
| 6 | [functions.md](functions.md) | Rust functions: build, deploy, run on server and device, egress, logs |
| 7 | [security.md](security.md) | Device keys, DPoP, attestation levels, pinning, sessions, step-up, signing, secure UI |
| 8 | [l10n-a11y.md](l10n-a11y.md) | Locales, right to left, calendars, translation exchange, accessibility |
| 9 | [governance.md](governance.md) | Roles, approvals, break-glass, freezes, rollouts, kill switch, audit |
| 10 | [operations.md](operations.md) | Compose stack, metrics, backups, Kubernetes, air-gap, upgrades |

Check IDs are `L2-<AREA>-<nnn>`; the form is in [../README.md](../README.md). Some checks are best
repeated after a later file (the Dev app streams after functions, analytics after the device runs);
they say so. Enterprise features (custom roles, freezes, SCIM, compliance reports) need a licence
file; without one the check records the refusal.

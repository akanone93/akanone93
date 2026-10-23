<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0093. The Dev app and the host kit

- **Status:** Accepted (maintainer, 2026-10-22; P10 plan D-6)
- **Date:** 2026-10-22
- **Requirements:** `DEV-001`, `DEV-040`, `DEV-003`, `SEC-052`, `SEC-056`

## Context and problem

ADR-0082 introduced `apps/plux_viewer`, a minimal host that signs in by device grant, receives
links, pins production root keys and opens reconstruction releases, "a minimal host that P10
grows into the Dev app". `apps/README.md` lists a separate `plux_dev/`. The two documents
disagree, and the Dev app has to trust the *development* root keys, which a viewer for audited
production reconstructions must never trust. The question: one app or two, and where does the
shared code live?

## Decision drivers

- One binary never trusts both root sets (`SEC-056`): the app that shows what an auditor needs to see must not accept dev bundles, and the Dev app must not accept production releases as if they were its own.
- The sign-in, link handling and root-pinning code is the same code twice, and that code is security-relevant.
- Host code (sign-in, links, pinning) does not belong in the runtime package `plux_flutter`, which every app ships and whose size is budgeted (`RT-061`).

## Considered options

1. Two apps sharing one new package: `apps/plux_dev` and `apps/plux_viewer`, with `packages/plux_host_kit`.
2. Rename the viewer into the Dev app and keep reconstruction as a mode.
3. Two apps with duplicated code.

## Decision

Chosen option: **1**, because it keeps the trust sets apart by construction and the shared code
in one reviewed place.

- **`packages/plux_host_kit`** (new, unpublished) holds sign-in (device grant or token), link
  handling, root-key pinning and the release opener, extracted from `apps/plux_viewer` without
  behaviour change. The roots are inputs; the kit pins whichever set the app hands it.
- **`apps/plux_viewer`** stays the reconstruction viewer and pins production roots. Its
  behaviour is unchanged; ADR-0082's sentence that P10 grows it into the Dev app is superseded by
  its Revision.
- **`apps/plux_dev`** (new) is a host on `plux_flutter`, `plux_devtools` and `plux_host_kit`,
  for Android and iOS, with every first-party optional package so that any plugin can be
  previewed (`DEV-001`), `plux_security` in its report-only development mode (ADR-0091), QR
  scanning through `plux_scanner`, pinned development roots (ADR-0087) and experiment previews
  (the P9 hand-off). It does not open reconstruction releases and does not trust production roots.
- **Documents.** `apps/README.md` and the viewer README name both apps; the CI and release paths
  list both.

## Consequences

- **Positive:** the trust boundary is the app boundary; security-relevant host code exists
  once; the viewer is not exposed to dev features.
- **Negative:** two apps to build, sign and publish; a new package to maintain; the extraction
  must not change the viewer's behaviour, which its existing tests guard.
- **Follow-up:** the extraction (D7); path rules in `ci/` for `apps/plux_dev` and
  `plux_host_kit`; coverage floors at the package defaults.

## Options in detail

### Option 1 — two apps and a host kit

Described above.

### Option 2 — one app, reconstruction as a mode

Least code, but one binary then holds both root sets and a mode switch decides which to trust,
exactly the weakness `SEC-056` exists to avoid. Rejected.

### Option 3 — duplicate the code

Two copies of sign-in and pinning drift apart. Rejected.

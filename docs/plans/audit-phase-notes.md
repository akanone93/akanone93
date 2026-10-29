<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Notes for the cross-phase audit phase

Items collected for the audit phase that follows P11 (it covers the requirements of P0–P11; its own plan
and milestones come after the maintainer's go-ahead). Each item says where it came from.

| # | Item | Source |
|---|---|---|
| A-1 | **Audit walkthrough: Studio-driven steps.** Studio is now how a regulated team approves release requests, requests changes and reads the audit trail; add a few steps to the scripted walkthrough (`test/walkthrough/`) that do this through Studio (approve a release request, export the audit evidence), so the signed evidence bundle covers the Studio path too. Needs the Studio image built in the kind job first: the kind values turn Studio off today (`deploy/helm/plux/ci/kind-values.yaml`). | Maintainer, 2026-10-29 |
| A-2 | **Faster database tests.** Every database test migrates a schema of its own; a template database (needs CREATEDB for the test role) would cut the Go race run, which now needs 45 minutes in CI (`.github/workflows/ci.yml`, Go test). | [p11-handoff/decisions.md](p11-handoff/decisions.md) 2026-10-28; CI rounds 2026-10-29 |
| A-3 | **Kind jobs with Studio.** The kind jobs (HA, restore drill, air-gapped install, walkthrough) run without Studio because its image is published only with a release; build it in those jobs (as the server image is) so the chart's Studio is installed and the air-gapped bundle carries it (DEP-004, DEP-040). | CI round 1, 2026-10-29 |
| A-4 | **Registry pulls in CI.** Jobs that pull images from ghcr.io failed twice on the registry (rate limit, connection timeout) in one night of runs; consider a pull-through cache or retries for the ZAP, CloudNativePG and kind images. | CI rounds 4–5, 2026-10-29 |

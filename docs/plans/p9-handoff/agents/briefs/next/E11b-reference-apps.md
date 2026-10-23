<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E11b — the reference apps' P9 scenarios (`DX-004` P9 parts)

Read: `docs/plans/p9.md` lines 666–672 (§5.13); `apps/plux_express` and `apps/plux_bank` (their README, their Plux projects — grep for where their JSON documents live — and their end-to-end flows: `mk/device.mk` `e2e-starter` shards `bank`, `express`); the P9 contracts you use: experiments/flags (`schema/json/{experiment,flag,segment}.schema.json`, the `Variant` node), rollouts (`plux rollout …`), the control document (`plux control …`), policies and approvals (`plux policies …`, `plux approvals …`), reconstruction (`plux audit reconstruct …`).

Deliver:
1. **Plux Express:** a scheduled campaign banner delivered through a rollout schedule; an A/B experiment on the catalogue banner (a `Variant` node, `experiment_exposure` recorded when the arm builds); a flag-gated courier feature (route guard on the flag).
2. **Plux Bank:** production releases under the `maximum` profile's default policy (two approvers from distinct teams for a release touching the secure transfer page); a kill-switch drill on the transfer page (fallback page shown, `control_applied` reported); the reconstruction scenario the walkthrough uses (a device's moment on the transfer page).
3. Seed/fixture scripts the walkthrough can call (`apps/<app>/tool/p9_*.sh` or Go test helpers — follow what the apps already use), documented in each app's README.
4. Tests: the compiler accepts both projects (goldens updated through the repo's `-update` flags), widget tests for the banner variant and the flag-gated route, and the e2e flows extended only where they run without devices here (device flows run in CI).
Done when: `make go-test` for the compiler packages you touched, `flutter test` in both apps, and the conformance/golden tests pass.

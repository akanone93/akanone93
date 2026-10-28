<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Verification at hand-off (2026-10-28, end of session 2)

## Last full gate that passed

On `feat/phase-11` at `b66f249b` (end of wave 11, pushed):

| Gate | Result |
|---|---|
| `make gen-check` | exit 0 |
| `make go-lint` | 0 issues |
| `make studio-check` | exit 0 — 3,477 tests, 0 fail |
| `make studio-e2e` | exit 0 — 247 passed (incl. the axe sweep and the keyboard flow) |
| full `make go-test` (pre-push hook) | ok |

## Since then: not gated (the maintainer stopped the gate at the hand-off)

The commits after `b66f249b` (statuses, plugin detail card, template parameters, apps grid and app
settings, the hand-off package, two catalogue fixes) have **not** been through the full gate. What
did run on them:

- by their agents: `bun test` per feature directory, `tsc`, `make studio-lint`, lingui extract,
  `make spec-lint`, `make trace` (all exit 0);
- by the lead: `make gen-check` 0, `make go-lint` 0, `make reuse-lint`, `make docs-site`,
  `make studio-i18n-check` 0 (after removing the obsolete `home.apps.more` message);
- the full Go tests of the pre-push hook on the hand-off push.

An earlier gate run during the packaging failed in `studio-check` on that obsolete catalogue entry
(fixed in `96cba2c9`); its `studio-e2e` failure was not examined (it ran while a second gate was
starting). **The next session runs `scripts/gate.sh` first** ([next.md](next.md) step 0).

## Never run in P11

`make check` (whole repository), CI (no pull request yet; M-6: once at the end), the Docker image
builds and `studio-image-repro`, `zap-studio`, the render worker in a container, the full Flutter
suite.

# 0070. Golden tests: harness, canonical renderer, server baselines, publish gate

- **Status:** Proposed
- **Date:** 2026-10-16
- **Requirements:** `TST-003`, `TST-005`, `A11Y-005`

## Context and problem

`TST-003` asks for visual regression tests per device frame, locale, theme and text scale,
compared with approved baselines, optionally blocking publication. The server cannot render
Flutter today (server-side rendering is P11, `STU-100`). `plux test` already writes a Flutter test
harness around the real runtime (ADR-0052).

## Decision drivers

- Render with the real runtime; deterministic images; a gate the server enforces.

## Considered options

1. A server renderer role now.
2. **`plux golden` on the developer's or CI's Flutter, baselines and the gate on the server.**
3. Baselines as project files with a CLI-only check.

## Decision

Chosen option: **2** (P8 plan D11, D12).

- `plux golden run|approve|status` compiles the project, writes the `plux test` harness, and renders
  each case with all data sources mocked and the clock frozen, using vendored Noto Sans, Noto
  Naskh Arabic and Noto Sans Ethiopic (OFL-1.1, pinned with SHA-256, never shipped in an app).
- **Canonical renderer:** Linux, compared pixel by pixel; other systems compare with a tolerance and
  mark the run non-canonical, which the gate never accepts.
- **Matrix:** frames from `schema/goldens/frames.json` (360 × 640 @2, Pixel 8 412 × 915 @2.625,
  iPhone 15 393 × 852 @3, tablet 820 × 1180 @2), themes, text scales 1.0 and 2.0, the app's locales
  and pseudo-locales; `goldens.matrix` narrows it; bounded by `golden.casesPerRun`.
- **Server:** `GoldenService` stores runs (bundle hashes, case IDs, image SHA-256s, differing images)
  in object storage under the plugin and rows under row-level security; `goldens.approve`
  (step-up) promotes images to baselines.
- **Gate:** with `goldens.blockPublish`, a publish whose plugin bundle hashes have no canonical run,
  or a run with unapproved differences, is refused (`PLX-3061`, `PLX-3062`). Compiler determinism
  (`CMP-002`) makes the local hashes the server's.
- **CI:** a Linux `goldens` job renders the starter's full matrix with the real runtime and the
  vendored fonts and requires identical images on every run, which keeps the canonical renderer
  deterministic (ADR-0043 Revision). The reference apps' suites compare against approved
  baselines, which only a server holds, so they run with `plux golden run --upload` against a
  real server (a developer's or the release pipeline's), not in CI (maintainer, 2026-10-18).

## Consequences

- **Positive:** visual regressions block publication now; P11's server renderer reuses the harness.
- **Negative:** developers need the Flutter SDK for golden runs (`PLX-9301`).
- **Follow-up:** L8; `TST-005` (P9) makes goldens mandatory under `maximum`.

# Golden Runs Runbook

A golden run is marked non-canonical, or its images differ although nothing changed
(`TST-003`, ADR-0070).

## Rules

- The **canonical renderer is Linux**, compared pixel by pixel. Other systems compare with a
  tolerance and mark the run **non-canonical**; the publish gate never accepts such a run
  (`PLX-3061`).
- Text uses the vendored Noto Sans, Noto Naskh Arabic and Noto Sans Ethiopic (OFL-1.1, pinned by
  SHA-256, never shipped in an app). Data sources are mocked and the clock is frozen.
- The matrix is the frames of `schema/goldens/frames.json` (360 × 640 @2, Pixel 8 412 × 915
  @2.625, iPhone 15 393 × 852 @3, tablet 820 × 1180 @2), themes, text scales 1.0 and 2.0, and the
  app's locales and pseudo-locales, narrowed by `goldens.matrix`.

## Diagnose

| Symptom | Cause | Fix |
|---|---|---|
| Run marked non-canonical | Run on macOS or Windows | Run on Linux, or in the CI `goldens` job |
| Every text image differs | Fonts are not the pinned ones (system fonts, or the vendored files altered) | Restore the vendored fonts and check their SHA-256 |
| Images differ in size | Frames differ from `frames.json`, or the matrix changed | Restore the frames; check `goldens.matrix` |
| Images differ after a Flutter upgrade | The rendering changed with the pinned SDK | Review, then approve with `plux golden approve` |
| Differences vary from run to run | A data source is not mocked in the case | Mock it; the harness freezes the clock |
| Run refused for size | Over `golden.casesPerRun`, `golden.imageBytes` or `golden.baselineBytesPerPlugin` | Narrow the matrix; see the limits registry |

## Fix

Rerun on the canonical renderer, review the differences, approve intended ones (`goldens.approve`,
step-up), and check with `plux golden status`. Never approve a non-canonical run's images to
silence a gate.

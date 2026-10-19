<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Golden Tests Guide

How to catch visual regressions before a release: `plux golden` renders every page with
the real runtime in a matrix of device frames, locales, themes and text scales, compares
the images with approved baselines, and can block a publish until the differences are
approved (`TST-003`). The design is [ADR-0070](../adr/0070-golden-tests.md). Declarative
scenario tests are in the [testing guide](testing.md).

## 1. Run

```text
plux golden run [-C dir] [--app id] [--plugin key] [--upload] [--flutter path]
plux golden approve --run id [--case id]... [-C dir] [--app id]
plux golden status [-C dir] [--app id] [--plugin key] [--json]
```

`plux golden run` compiles the project, generates a Flutter test project around the real
runtime (the same harness as `plux test`) and renders each case with every data source
mocked and the clock frozen. It compares the images with the approved baselines and writes
the ones that differ to `.plux/golden` (or `--out dir`). The exit code is 0 for no
differences and 1 for differences or a failure. It needs the Flutter SDK, found on the
`PATH` or with `--flutter`; without it the run fails with `PLX-9301`.

`--upload` records the run on the server: the bundle hashes, the case IDs, the SHA-256 of
each image and the images that differ. It prints the run's id.

## 2. The matrix

A run renders every page in each combination of:

- **Frames** from `schema/goldens/frames.json`: `small-360x640` (@2), `pixel-8`
  (412 × 915 @2.625), `iphone-15` (393 × 852 @3) and `tablet-820x1180` (@2).
- **Themes**: `light` and `dark`.
- **Text scales**: 1.0 and 2.0, so every page is checked at 200% text (`A11Y-005`).
- **Locales**: the app's `supportedLocales`, plus the pseudo-locales when
  `localisation.pseudoLocales` is on ([translation workflow](translation-workflow.md)).

Narrow the matrix in `app.json` (every field is optional):

```json
"goldens": {
  "blockPublish": true,
  "matrix": {"frames": ["small-360x640", "iphone-15"], "themes": ["light"],
             "textScales": [1, 2], "locales": ["en", "ar"]}
}
```

The number of cases in one run is bounded by `golden.casesPerRun`
([limits](../reference/limits.md)).

## 3. Fonts and the canonical renderer

Text is drawn with vendored Noto Sans, Noto Naskh Arabic and Noto Sans Ethiopic, pinned
by SHA-256 under `schema/goldens/fonts`, so a run does not depend on the machine's fonts.
They are test fixtures and never ship in an app. Use `--fonts dir` to point at another
copy.

Only Linux is canonical: its images are compared pixel by pixel. Another system compares
with a tolerance and marks the run non-canonical; the publish gate never accepts a
non-canonical run. Develop anywhere, but upload from Linux or CI.

## 4. Baselines and approval

A baseline is an approved image for a case. The first run has no baselines, so every case is a
difference. Review the images, then approve:

```text
plux golden run --upload          # prints the run id
plux golden approve --run <id>                 # every difference
plux golden approve --run <id> --case <case>   # one case; repeatable
```

Approval needs the `goldens.approve` permission and a second factor (step-up). It promotes the
run's images to baselines, stored in object storage under the plugin.

## 5. The publish gate

With `goldens.blockPublish`, a publish is refused when the plugin's bundle hashes have no
canonical run (`PLX-3061`) or the run has differences nobody approved (`PLX-3062`). Because
the compiler is deterministic (`CMP-002`), the hashes you render locally are the ones the
server sees. Check before you publish:

```text
plux golden status
```

It exits 0 when every plugin is approved and 1 when not.

## 6. CI

The repository's `Golden tests` job runs the golden tooling's tests on Linux
([CI](../engineering/ci.md)). To satisfy the gate from your own pipeline, run
`plux golden run --upload` on a Linux runner: only a canonical run counts, so a run from
another system cannot.

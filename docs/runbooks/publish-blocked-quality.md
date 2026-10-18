# Publish Blocked by Accessibility, Goldens, Profile or Pseudo-Locales

A publish or promotion is refused with one of `PLX-3060` to `PLX-3064`. Each is a gate working as
designed; this runbook reads the evidence and clears it the right way.

| Code | Refused because | Section |
|---|---|---|
| `PLX-3060` | An unwaived accessibility error exists (`A11Y-002`) | [Accessibility](#accessibility-plx-3060) |
| `PLX-3061` | No canonical golden run exists for the plugin version (`TST-003`) | [Goldens](#goldens-plx-3061-and-plx-3062) |
| `PLX-3062` | A golden image differs from its baseline and is not approved | [Goldens](#goldens-plx-3061-and-plx-3062) |
| `PLX-3063` | The release's compiled profile is looser than the environment's | [Profile floor](#profile-floor-plx-3063) |
| `PLX-3064` | The release carries `en-XA` or `ar-XB` and targets production | [Pseudo-locales](#pseudo-locales-plx-3064) |

## Accessibility (`PLX-3060`)

The security setting `accessibilityBlocksPublish` is off under `standard` and on under `strict`
and `maximum`. The compiler reports accessibility findings as errors under `strict` and
`maximum` and as warnings otherwise, by the `securityProfile` in `app.json` (ADR-0069).

1. Read the **accessibility report** of the release: `plux release report` or
   `ReleaseService.GetReport`; JSON and standalone HTML (`A11Y-008`). It lists every check run,
   every violation with its node path and severity, and every waiver with author and
   justification.
2. Fix the node: add an accessible name (`PLX-1401`), alternative text or `decorative`
   (`PLX-1402`), raise contrast (`PLX-1403`; colours that cannot be known before run time are
   `PLX-1409`), or enlarge the target (`PLX-1404`: 44 × 44, or 48 × 48 when the release targets
   Android).
3. **Waive** only with a reason: `a11yWaivers` on the node or page names one check and a
   justification of at least ten characters (`PLX-1410`). Under `maximum` the saving principal
   needs `a11y.waive` (`PLX-1411`); waivers are audited and bounded by `a11y.waiversPerPlugin`
   (default 50).
4. Lowering `accessibilityBlocksPublish` is a reviewed configuration change, not a fix.

## Goldens (`PLX-3061` and `PLX-3062`)

Applies where `goldens.blockPublish` is set (ADR-0070).

- `PLX-3061`: run `plux golden run` and upload; check with `plux golden status`. A run is matched
  by the plugin's bundle hashes (the compiler is deterministic, `CMP-002`), so any source change
  needs a new run. A non-canonical run does not count; see [Golden runs](golden-runs.md).
- `PLX-3062`: open the differing images (stored in object storage under the plugin). If the
  change is intended, `plux golden approve` (permission `goldens.approve`, step-up) promotes the
  images to baselines. If not, fix the rendering.
- Limits: `golden.casesPerRun` (500), `golden.imageBytes` (2097152) and
  `golden.baselineBytesPerPlugin` (268435456). `goldens.matrix` narrows the matrix.

## Profile floor (`PLX-3063`)

The release was compiled for a profile looser than the target environment's effective profile.
Releases are promoted without recompiling, so compile again with a `securityProfile` equal to or
stricter than the environment's and publish that release. A stricter release in a looser
environment is allowed (ADR-0053, Revision 2026-10-17). Lowering the environment's profile is a
reviewed configuration change.

## Pseudo-locales (`PLX-3064`)

`en-XA` and `ar-XB` are for development (`localisation.pseudoLocales`). Compiling for production
already refuses them (`PLX-1614`); promotion into a production environment refuses a release that
has them. Turn the setting off and publish again.

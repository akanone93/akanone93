<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Accessibility Guide

How Plux holds apps to WCAG 2.2 AA and EN 301 549: checks when a release is compiled,
profiles that decide how strict they are, waivers, a report per release, and the semantics
the runtime gives assistive technology (`A11Y-001` to `A11Y-008`). The design is
[ADR-0069](../adr/0069-accessibility-checks-and-report.md).

## 1. Compiler checks

The compiler checks every release. A value it cannot know is reported as not checked and
never guessed.

| Code | Check | What it finds |
|---|---|---|
| `PLX-1401` | `accessibleName` | An interactive node with no accessible name. |
| `PLX-1402` | `imageAlternative` | An image with neither alternative text nor `decorative`. |
| `PLX-1403` | `contrast` | Text or a UI component below 4.5:1, or 3:1 for text of 24 px or more, 18.66 px bold or more, and for component boundaries. Checked against the nearest opaque background in the light, dark and high-contrast token sets. |
| `PLX-1404` | `touchTarget` | A fixed target below 44 × 44, or 48 × 48 when the release targets Android. |
| `PLX-1405` | `fixedHeightText` | Text under a fixed-height ancestor. |
| `PLX-1406` | `focusOrder` | An invalid focus order. |
| `PLX-1407` | `headingLevel` | A skipped heading level. |
| `PLX-1408` | `liveRegion` | A live region with constant content. |
| `PLX-1409` | | Contrast that could not be checked (info). |

Give a node what the checks want with its semantics: a label, `focusOrder`, `group`,
`headingLevel`, `decorative` for images that carry no meaning.

## 2. Severity by profile

Findings are errors under the `strict` and `maximum` security profiles and warnings under
`standard` (`PLX-1405` and `PLX-1408` are always warnings). The profile is `app.json`'s
`securityProfile`, recorded in the bundle. The security setting `accessibilityBlocksPublish`
(off under `standard`, on under `strict` and `maximum`) refuses a publish with any
unwaived error: `PLX-3060`.

**The profile floor.** A release is promoted between environments without recompiling, so
its severity is fixed when it is compiled. Publishing or promoting a release into an
environment whose effective profile is stricter than the release's compiled profile is
refused (`PLX-3063`); a stricter release in a looser environment is allowed. Compile with
the profile of the strictest environment you will promote to, so a finding is never a
warning where the environment blocks on errors
([ADR-0053](../adr/0053-remote-security-configuration.md), revision 2026-10-17).

## 3. Waivers

When a check does not apply, waive it on the node or the page with `a11yWaivers`: one
entry per check, each with a justification of at least ten characters.

```json
{"type": "Image", "props": {"source": {"asset": "…"}},
 "a11yWaivers": {"imageAlternative": "Decorative flourish repeated beside the visible title text"}}
```

The checks that can be waived are `accessibleName`, `imageAlternative`, `contrast`,
`touchTarget`, `fixedHeightText`, `focusOrder`, `headingLevel` and `liveRegion`. A missing
or too-short justification is `PLX-1410`. Under `maximum`, the principal saving a waiver
needs the `a11y.waive` permission (`PLX-1411`), and the save is audited.

## 4. The report

Every release has an accessibility report: each check run, each violation with its node
path and severity, and each waiver with its author and justification (`A11Y-008`). It is
JSON (`schema/json/a11y-report.schema.json`) or standalone HTML rendered from it:

```text
plux release report --format html --out a11y.html <sequence>
plux release report --kind accessibility --format json -C app <sequence>
```

`--kind` is `accessibility` (the default) or `localisation`; `--channel` defaults to
`production` and `--env` selects an environment. The same report is served by the
release service's `GetReport`.

## 5. Runtime semantics

Every widget builder passes its role, label, value, state and actions to Flutter's
semantics (`A11Y-001`).

- `focusOrder` becomes an ordinal sort key in a focus-traversal group; `group` a semantics
  container; `headingLevel` the semantics heading level.
- Tap targets: Material's padded targets (48) and Cupertino's 44 pt.
- Reduce motion, bold text, high contrast and screen-reader state come from the platform
  and are available in PXL's `device` root; animations honour reduce motion
  ([animation guide](animation.md)).
- The `announce` action has the screen reader say a message without moving focus:
  `message` and `assertiveness` (`polite` by default). The message must already be
  translated ([reference](../reference/actions.md#announce)).
- The `setThemeMode` action changes the app's theme mode
  ([reference](../reference/actions.md#setthememode)); contrast is checked in the light,
  dark and high-contrast token sets.

## 6. Testing

Widget tests use a shared helper that asserts each widget's semantics and runs Flutter's
`androidTapTargetGuideline`, `iOSTapTargetGuideline`, `labeledTapTargetGuideline` and
`textContrastGuideline` in every golden variant, including 200% text scale and RTL
([golden tests](golden-tests.md)). The reference apps' device flows walk the platform
accessibility tree with UiAutomator and XCUITest. Live TalkBack and VoiceOver passes are
on the real-device checklist; they are manual and not run in CI.

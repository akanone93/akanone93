# 0069. Accessibility checks at publish, waivers, report and runtime semantics

- **Status:** Proposed
- **Date:** 2026-10-16
- **Requirements:** `A11Y-001`–`A11Y-008`, `QA-011`, `SEC-180`

## Context and problem

Plux targets WCAG 2.2 AA and EN 301 549. Until P8 the compiler checks one thing — an accessible
name, as a warning (`PLX-1401`) — and the runtime maps a few semantics fields. P8 needs checks at
publish that can block, waivers that auditors can read, a report per release, and correct
semantics and platform settings at run time.

## Decision drivers

- Catch defects at publish, before a device sees them (spec G6).
- Never guess: a value the compiler cannot know is reported as not checked.
- Profiles decide how strict (§15.12).

## Considered options

1. Runtime-only checks (Flutter's guidelines in tests).
2. **Compiler checks with profile-driven severity, waivers and a report, plus runtime semantics and
   test-time guidelines.**

## Decision

Chosen option: **2** (P8 plan D9, D10, D13).

- **Checks** (compiler `policy` stage): accessible names (`PLX-1401`); image alternative text or
  `decorative` (`PLX-1402`); contrast of text and UI components against the nearest opaque
  background in the light, dark and high-contrast token sets — 4.5:1, or 3:1 for text ≥ 24 px or
  ≥ 18.66 px bold and for component boundaries — with unknowns as `PLX-1409`; fixed touch targets
  below 44 × 44, or 48 × 48 when the release targets Android (`PLX-1404`); text under a
  fixed-height ancestor (`PLX-1405`, warning); focus order (`PLX-1406`), heading levels
  (`PLX-1407`), constant live regions (`PLX-1408`, warning).
- **Severity:** errors under `strict` and `maximum`, warnings otherwise, by the profile the
  release is compiled for (`app.json` `securityProfile`, recorded in the bundle). The security
  setting `accessibilityBlocksPublish` (`standard` off, `strict` and `maximum` on) refuses a
  publish on any unwaived error (`PLX-3060`). An environment never receives a release compiled
  for a looser profile than its own (`PLX-3063`, ADR-0053 Revision 2026-10-17), so a finding
  can never be a warning where the environment blocks on errors.
- **Waivers:** `a11yWaivers` on a node or page, one check each, with a justification of at least
  ten characters (`PLX-1410`); under `maximum` the saving principal needs `a11y.waive`
  (`PLX-1411`), audited.
- **Report:** per release, every check run, violation (node path, severity) and waiver (author,
  justification); JSON per `schema/json/a11y-report.schema.json` and standalone HTML rendered
  from it; served by `ReleaseService.GetReport` and `plux release report`.
- **Runtime:** every builder passes role, label, value, state and actions; `focusOrder` →
  `OrdinalSortKey` in a `FocusTraversalGroup`, `group` → a semantics container, `headingLevel` →
  `Semantics.headingLevel`; `announce` → `SemanticsService.sendAnnouncement` (`announce` is
  deprecated in Flutter 3.47.5); Material's padded tap targets and Cupertino's 44 pt; reduce
  motion, bold text, high contrast and screen-reader state from `MediaQuery`, also in PXL's
  `device` root.
- **Tests (`QA-011`):** a shared helper asserts each widget's semantics and Flutter's
  `androidTapTargetGuideline`, `iOSTapTargetGuideline`, `labeledTapTargetGuideline` and
  `textContrastGuideline` in every golden variant; the reference apps' device flows walk the
  platform accessibility tree with UiAutomator and XCUITest; live TalkBack and VoiceOver passes are
  on the real-device checklist.

## Consequences

- **Positive:** defects stop at publish under the stricter profiles; auditors get a report.
- **Negative:** contrast resolution needs the theme's role pairs for Material defaults.
- **Follow-up:** L6 (compiler and publish), L7 (runtime and tests); Studio overlays in P10/P11.

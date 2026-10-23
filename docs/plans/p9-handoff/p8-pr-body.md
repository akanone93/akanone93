## What

Phase 8 — Localisation and accessibility (spec §5.1, plan [docs/plans/p8.md](../p8.md)):

- **Locale data and formatting:** CLDR (pinned, `modern` coverage) shipped per app locale in the app bundle; MessageFormat subset; the PXL `format`, `l10n` and `calendar` groups; Gregorian, Ethiopian, Umm al-Qura Hijri and Persian calendars; Latin digits by default (ADR-0066, ADR-0067).
- **Runtime localisation:** locale controller, `setLocale` without restart, RTL, fallback chain with `l10n_missing`, Flutter's widget strings from the SDK's ARB files, per-locale assets.
- **Translation workflow:** translator role and permissions, statuses in the translations document, per-locale locks, approval-only releases, XLIFF/ARB/JSON/CSV import and export, pseudo-locales, `plux l10n extract|pseudo`, the localisation report (ADR-0068).
- **Accessibility:** WCAG 2.2 AA checks at publish, `accessibilityBlocksPublish`, waivers, JSON/HTML reports (ADR-0069); semantics across the catalogue; accessibility-tree passes on Android (UiAutomator) and iOS (XCUITest) for Plux Bank and Plux Express (`QA-011`).
- **Golden tests:** `plux golden` with server-held baselines and the optional publish gate (ADR-0070).
- **Catalogue catch-up:** every Appendix C widget tagged P3–P5 has a descriptor and builder (ADR-0071); optional packages `plux_components`, `plux_charts`, `plux_markdown`, `plux_player`, `plux_maps`, `plux_maps_google`, `plux_scanner`; `make catalogue-check` gates the catalogue.
- **Reference apps** in English, German, Arabic and Amharic; `plux_flutter` 0.5.0.
- Also: the P9 plan ([docs/plans/p9.md](../p9.md)) with the maintainer's decisions of 2026-10-18.

## Why

Implements `I18N-001`–`I18N-006`, `I18N-008`, `I18N-009`, `I18N-011`, `I18N-012`, `A11Y-001`–`A11Y-004`, `A11Y-006`–`A11Y-008`, `TST-003`, `QA-011`; advances `PXL-006`, `CMP-032`, `DX-004`. `I18N-007`, `I18N-010` and `A11Y-005` stay `WIP` for their Studio and Dev-app parts (P10/P11, plan D1).

## How it was verified

- `make check` with every gate green; `make trace` 0 violations; `make catalogue-check` 0 findings.
- `go test -race -count=40 -run 'TestPublishWithTheCLIAndSyncADevice$' ./internal/server` passed; `make e2e-starter` passed.
- Dispatched CI run 39143719161 on the final P8 code green in every job, including the device shards on Android 26, Android 35 and iOS.
- The CI rounds that made the new accessibility-tree passes run are recorded in the work log's 2026-10-18 note: androidTest dependencies held to androidx.test 1.3.0; the APK built for the emulator's ABI; the Android passes read `AccessibilityNodeInfo` and activate controls through `ACTION_CLICK`; the iOS pass closes the keyboard with Return; the Android 26 boot no longer restarts adb's server (which registered the emulator twice) and cold-boots a silent emulator once instead.

## Incompatible changes (before 0.0.1)

- The app bundle gains locale-data sections and `Meta.localisation`; the document schema gains translation statuses, waivers and semantics fields; calendar values are camelCase everywhere. No release exists yet, so no migration is required (`BND-000`, `SCH-000`).

## Checklist

- [x] `make check` passes locally
- [x] Docs, doc comments, component `AGENTS.md` and `docs/WORKLOG.md` updated
- [x] ADRs added (0066–0071, revisions to 0043, 0050, 0053)
- [x] New files carry SPDX headers (`make reuse-lint`)

**Implements requirements:**

- [x] Tests name the requirement IDs they verify (`QA-071`)
- [x] `Status` updated in `docs/requirements.md` and `make trace` passes (`QA-070`, `QA-072`)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

https://claude.ai/code/session_01HQqbAJE4rF2k3iVu2SV61R

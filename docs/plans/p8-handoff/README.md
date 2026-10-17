# P8 hand-off: where the first P8 session stopped (2026-10-17)

This folder lets a new session continue Phase 8 exactly where the first session stopped. Read it
after `AGENTS.md`, `docs/WORKLOG.md` (latest note) and `docs/plans/p8.md`. When everything listed
here is applied and merged, delete this folder in the same commit that closes the last item, so
no stale copy remains.

## 1. State of `feat/phase-8`

- Branch `feat/phase-8` (pushed). It holds everything merged so far, plus this folder. All Go
  tests (backend and tools) passed at `2595511` (the pre-push hook ran `make go-test`). Not run at
  the hand-off: `make check`, the Dart suites in full, e2e, CI.
- **Merged milestones:** L0 (contracts, spec 1.6.0, ADRs 0066–0071 *Proposed*), L1 (CLDR 48.2.3
  data, 104 locales, Go and Dart engines), L2 (MessageFormat, PXL `format`/`l10n` groups), L3
  (calendars: Gregorian, Ethiopian, Umm al-Qura table, Persian; PXL `calendar`), L5a (translator
  role, `translations.edit`/`approve`, additive `translation_locks` table, migration 0021), L5b
  (XLIFF 2.0/ARB/JSON/CSV exchange, `LocalisationService`, `plux l10n pull|push|export|import|status`),
  L6a/L6b (compiler accessibility checks, profile severity, waivers, a11y report JSON+HTML,
  `release_reports` table migration 0022, `accessibilityBlocksPublish` gate), L7 runtime part
  (semantics wrap, min target sizes, `announce`, `setThemeMode`), K1a–K1d, K2a, K2b, K3a
  (`plux_charts`, `plux_markdown`, `plux_player`, `SignaturePad`, `LottieView`/`RiveView`
  catalogue widgets), `PluxWidgetPackage` + `PluxConfig.widgetPackages`, `tools/cmd/catalogue`
  (`make catalogue-check`, report-only).
- `make catalogue-check` at the hand-off reports 14 findings: `setLocale` no handler (L4b adds
  it); `MapView`, `OrderTrackerMap` (K3b-maps patch), `QrScanner`, `BarcodeScanner`,
  `QrCodeView` (K3b-scan, not started) missing descriptors; and "no builder" for `AudioPlayer`,
  `VideoPlayer`, `LottieView`, `RiveView`, `MarkdownView`, `TermsViewer`, `PieChart`, `Sparkline`
  although their packages register builders. Check whether the tool only scans
  `plux_flutter`'s builders (fix the tool to read `PluxWidgetPackage.builders` of each package
  named in the descriptor's `package`) or whether those packages really miss the IDs. Fix it
  before flipping the check to failing in L10.

## 2. Unfinished tasks saved as patches (apply, finish, merge)

Six agents stopped at once on a usage limit. Their work-in-progress is saved here as
`git diff --binary` patches against `feat/phase-8` at `2595511`, gzip-compressed. Each applies
cleanly on its own (`git apply --check` passed for all five). The original briefs are in
[briefs.md](briefs.md).

Apply one per branch, for example:

```sh
git switch -c wip/l4a feat/phase-8
zcat docs/plans/p8-handoff/l4a.patch.gz | git apply --index -
make gen   # regenerates generated files the patch left out or that drifted
```

Overlaps between patches (resolve when the second one merges): `k3b-maps` and `l4b` both edit
`packages/plux_flutter/lib/src/render/renderer.dart`; `k3b-maps` and `l5c` both change the
generated `backend/internal/schema/registry/registry_gen.go` (regenerate, don't hand-merge).

| Patch | Task (brief) | State at the stop | What remains |
|---|---|---|---|
| `l4a.patch.gz` | **L4a** — locale data and Flutter widget strings in the app bundle, locale overrides, per-locale assets, CMP-032 font subsetting | Nearly done. `bundle.fbs` `Locale` gained `data`, `flutter`, `tag_resolved`; new `AssetLocaleFile`; `tools/cmd/flutterl10n` + `make flutter-l10n-data` wrote `schema/l10n/flutter/<locale>.json` from the pinned SDK's ARB files; Go embed copy in `backend/internal/l10n/flutterdata/` (+ REUSE annotation); `l10n/reduce.go` (per-app reduction); `compiler/localedata.go`, `assetlocale.go`, `fontsubset.go`; generated Go/Dart bundle code regenerated. `go build ./...` passes in backend and tools. Targeted tests: **one failure**, `TestInvalidLocaleOverridesAreRejected` (`localedata_test.go:180`: no PLX-1613 at the pointer ending `/localeOverrides/de_DE`). | Fix that test or the pointer; re-run `go test ./internal/compiler/... ./internal/l10n/... ./internal/bundle/...` and `tools` `./cmd/flutterl10n/...`. **Excluded from the patch to stay under the 500 KB file limit: all `schema/testdata/**` changes** (bundle `.pxb` files, `verifier/vectors.json`, `confidential/vectors.json`) — regenerate with the packages' `-update` flags (`go test ./internal/bundle/ ./internal/delta/ ./internal/plxtest/ ./internal/compiler/ -run . -update`, as done after earlier merges). Review that `flutterdata/` duplicating `schema/l10n/flutter/` is the same embed pattern the CLDR data uses. |
| `l4b.patch.gz` | **L4b** — runtime localisation: `LocaleController`, switching without restart, RTL, Plux's own Material/Cupertino/Widgets localizations from the `flutter` map, fallback + `l10n_missing`, `RuntimeStrings`, `setLocale` handler | Implementation in progress, **no tests yet**. New `l10n/locale_controller.dart`, `locale_host.dart`, `flutter_localizations.dart`, `runtime_strings.dart`; edits in `plux.dart`, `fallback.dart`, `router.dart`, `renderer.dart`, `values.dart`, builders `calendar`/`components_a`/`components_b`/`money`, `telemetry/events.dart`, `a11y_handlers.dart`. `dart analyze lib` shows only 5 infos (directive ordering, one tear-off). Agent was wiring "ValueResolver, host widget, imports" when it stopped. | Finish wiring (host widget above every page, `device.calendar`/`device.textDirection`, `DualCalendarDatePicker` default from `localisation.calendar`), then all of brief step 7 tests; run `flutter test test/l10n/ test/render/ test/actions/`. Code against L4a's bundle fields (merge L4a first). |
| `l5c.patch.gz` | **L5c** — compiler l10n checks, `translatable` marks (X1.9), pseudo-locales, localisation report, `plux l10n extract|pseudo` | Early. `translatable` carried through `tools/internal/registry/model.go`, the Go registry codegen and `schema/json/registry/value-type.schema.json`; marks applied to part of the descriptors (layer1 A–T subset, five layer2, 17 value types — **the agent was in the middle of applying its edit script, so coverage is partial and must be re-checked**); `backend/internal/l10n/pseudo/pseudo.go` and `l10n/mf/print.go` started. Builds; targeted tests pass. `make gen` not yet run for Dart/TS registries. | Finish the marks over every descriptor and value type (review the word list); `make gen`; PLX-1608, PLX-1609, PLX-1610 (`requireApprovedTranslations`), PLX-1614 + `en-XA`/`ar-XB`; the `localisation` report producer in `release/reports.go`; CLI `extract`/`pseudo`; tests. |
| `k3b-maps.patch.gz` | **K3b-maps** — `MapView` (197), `OrderTrackerMap` (198) in `plux_maps` (flutter_map 8.3.2) and `plux_maps_google` (google_maps_flutter 2.19.0); `NodeContext.remoteClient`/`remoteImage` seam (X1.2) | Descriptors and value types `LatLng`, `MapCamera`, `MapMarker`, `MapPolyline` written and generated (`ids.lock.json`, registries, `docs/reference/widgets.md`); seam added to `node_context.dart`, `plux_node.dart`, `renderer.dart`, `scope.dart`, with a test edit in `widget_package_test.dart`; packages created with pubspec/LICENSE/CHANGELOG only (**no `lib/` code, no README, no tests**); workspace `pubspec.yaml`/`pubspec.lock` and `mk/dart.mk` updated. Analyzer clean. | Builders (tile provider through `remoteClient`, attribution, markers as semantics, courier tween), Google provider, READMEs, tests per the brief. |
| `dart-fixes.patch.gz` | **Dart test fixes** after the wave-3 merges | Two agent commits (icon-font/widgets bundle expectations regenerated runtime 0.5.0; `min_target.dart` marks every `minTarget` descriptor) plus uncommitted work on `scrolling.dart` and its test (RefreshIndicator `onRefresh` not invoked — was debugging; Stepper hit-test; MaterialBanner label). | Finish scrolling (11 failures before), `cupertino_more_test.dart` (12, not started), `golden_bundles` (2), `telemetry/cost_test.dart` (1); then `flutter test` in `plux_charts`, `plux_markdown`, `plux_player`, `plux_media`, `plux_lottie`, `plux_rive`. Regenerate goldens only where the regenerated bundle explains the change. |
| — (no patch) | **K3b-scan** — `QrScanner` (199), `BarcodeScanner` (200), `QrCodeView` (201) in `plux_scanner`; P6 decision B21 (unbundled ML Kit + vendored ZXing-C++ fallback via FFI and a build hook) | Only the ZXing-C++ reader sources were vendored (2.4 MB, 199 files, no LICENSE/VERSION yet), so nothing is saved. Source used: PyPI sdist `zxing_cpp-2.3.0.tar.gz`, SHA-256 `3babedb67a4c15c9de2c2b4c42d70af83a6c85780c1b2d9803ac64c6ae69f14e` (the C++ core of ZXing-C++ 2.3.0); file list in [zxing-files.txt](zxing-files.txt) (copied to `packages/plux_scanner/third_party/zxing-cpp/core/<path>`, plus `src/libzueci/*.h`, `zueci.c`, `README.md`). | Start the brief from scratch. Prefer the upstream `zxing-cpp/zxing-cpp` `v2.3.0` tag if it is reachable (record tag, commit and archive SHA-256 in `VERSION`, add LICENSE and the REUSE annotation, and a pre-commit exception for vendored files as WAMR has). Per-ABI Android sizes need the NDK (not in the container) — report as not verified. |

## 3. Tasks not started (in order)

1. **X1 — cross-phase integration of the new widgets** (plan §2.8), after K3b merges:
   X1.1 widget → device API/package table beside `compiler/device.go` `deviceActions`, approvals,
   `DeviceUses`, `PLX-1233`, runtime guard (`device/guard.dart` `DeviceGuard.apiOf` covers actions
   only; add widgets: scanners/`SignaturePad`/`KycCameraCapture` camera, `FileUploader` files,
   players, maps + location); X1.2 every remote load through the domain check and the pinned
   client (images in `ImageGallery`, `ProductCard`, `PromoBanner`, `ProfileHeader`; Markdown links
   → `openUrl`, Markdown images; Lottie/Rive URLs need the domain check — a P5 gap; map tiles via
   the K3b-maps seam); X1.3 = A3 (accepted): network media only from declared HTTPS domains,
   document that platform players cannot pin, `pinnedMediaOnly` setting (on under `maximum`)
   downloads through the pinned client first; X1.4 references (`FileUploader.dataSource`/
   `operation`, `InfiniteList` sources, timeline/track names, `Hero` tags) resolved with
   PLX-1102-style diagnostics and graph edges; X1.5 generic event payload type args
   (`typecheck.go` `payload()`); X1.6 `sensitive` flag on events and props read by trace and
   telemetry redaction; X1.7 one shared overlay-scope helper + a test per overlay widget.
2. **Wave 5:** L8 golden tests (`plux golden`, `GoldenService`, server baselines, optional
   publish gate, vendored Noto fonts, Linux golden CI job — D11, D12); L7 accessibility-tree
   passes in the device flows (`QA-011` as reworded, D13) and the widgets-gallery guideline
   failures below; L9 reference apps (Plux Bank and Plux Express in `en`, `de-DE`, `ar`, `am-ET`;
   starter `en` + `ar`; agent-drafted `ar`/`am` marked `translated`, D15).
3. **Wave 6 / L10:** docs (guides, runbooks, references), threat-model delta, real-device
   checklist P8 section, `ci/affected.json` entries for every new path (the integration agent
   put `mk/l10n.mk` under `noJob` — revisit) and coverage floors for new packages
   (`coverage.json`, maintainer approval for new floors), flip `catalogue-check` to failing in
   `make check`, bump `plux_flutter` to 0.5.0 with changelog (X1.10; descriptors already say
   0.5.0, the package says 0.4.0, and packages depend on `^0.4.0`/`^0.5.0` inconsistently —
   check), requirement statuses in `docs/requirements.md` (plan §3), WORKLOG, central gates
   (`make check`, e2e ×40 with race, dispatched CI green), report.

## 4. Open review notes (decide or fix; several need the maintainer)

- **ADR-0069 conflict (maintainer):** the compiler takes accessibility severity from
  `app.json`'s `securityProfile`, while `accessibilityBlocksPublish` comes from the environment
  config; the ADR must say which wins. Waiver permission also reads the `app.json` profile.
- L5b: a non-approver saving `approved` is silently ignored — should it be refused?
- Calendar enum spelling `islamicUmalqura` vs CLDR `islamic-umalqura` — make consistent.
- `pxl` now imports `l10n` and `calendar` (binary size); about 6 MB of CLDR data embedded in the
  backend — measure and record.
- Unicode-3.0 licence annotation added for CLDR data (REUSE passes) — confirm with maintainer.
- `DataScope` has no `source` prop (binds through a status expression) — check against App. C.
- `minTarget` is not emitted by the Dart registry generator; `min_target.dart` keeps a hand list
  (the dart-fixes patch extends it) — generate it instead.
- L2 kept the old text formatter as a fallback; `format.phone` deferred; L3 picker month names
  stay English until L4b's data reaches them.
- Widgets gallery accessibility guidelines: Cupertino segmented control 28 px high vs Android's
  48 px tap target (decision pending), layout contrast failures, unlabeled tappables in the
  material/structure pages.
- After merging L4a, `schema/testdata` regeneration changes the widgets/benchmark bundles; the
  benchmark baseline was regenerated once before (`benchproject.go` Icon got a `semanticLabel`).

## 5. How the first session worked (keep doing this)

- **Agent playbook:** the lead plans, briefs, merges, resolves conflicts, runs `make gen` and
  every gate centrally; agents only develop. Briefs carry all context (files and line ranges
  to read) so agents do not explore; shared prefix and rules are in [briefs.md](briefs.md).
  Agents run in worktrees (`isolation: worktree`) that start from `main`, hence rule 0
  `git merge --ff-only feat/phase-8`. Each agent stages only its own paths and reports in ≤ 8
  lines. Custom agent types used: `coder` (Sonnet, medium effort), `coder-high` (Sonnet, high),
  `coder-light` (Haiku) — each with tools Read, Edit, Write, Bash, Grep, Glob and the body
  "You implement exactly the brief you are given, in your own worktree, with tests. Follow the
  brief's rules; read only what it names." They lived in `~/.claude/agents/` of the first
  container; recreate them there if the new container lacks them.
- **Merging:** [merge.sh](merge.sh) `<branch>` merges, keeps "ours" for generated files and
  unions list-like files ([union.py](union.py)), and prints `MANUAL <file>` for the rest; then
  `make gen`, the `-update` flags (`compiler TestWidgets`, bundle, delta, plxtest vectors), Go
  tests, push. Error codes are added in `backend/internal/plxerr` (`registry.go`,
  `definitions.go`) and generated with `go generate ./internal/plxerr/` — never edit
  `schema/errors.json` by hand. Registry allocations (widget IDs 109–207, enum/type ranges per
  workstream) are in plan §2.7; K3 enums take 155–174, types 76–84.
- **Hooks:** commit-msg limits the header to 72 characters; pre-push runs `make go-test` (all Go
  tests). Never `--no-verify`.

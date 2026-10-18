# 0066. Localisation engine: CLDR data per app locale in the bundle

- **Status:** Proposed
- **Date:** 2026-10-16
- **Requirements:** `I18N-001`, `I18N-002`, `I18N-004`, `I18N-005`, `I18N-006`, `I18N-011`, `I18N-012`, `PXL-006`, `CMP-032`, `ACT-020`

## Context and problem

P8 needs CLDR-correct plural rules, number, money, percent, compact and date formatting and ICU
MessageFormat, identical in the compiler (Go) and the runtime (Dart), for the launch locales
`en`, `de-DE`, `ar`, `am-ET` and any other CLDR locale without code changes. Until P8 the device
formats messages with an English-only plural rule (`render/messages.dart`), PXL's `format`,
`l10n` and `calendar` groups are declared and unimplemented, and Flutter's own widgets (date
pickers, text-field menus, back buttons) have no strings outside English unless the host ships
`flutter_localizations`. The core package is size-budgeted (`RT-061`).

## Decision drivers

- Go and Dart give byte-identical results (Studio previews, compile-time checks and the device
  agree; `PXL-007`, `QA-003`).
- A new locale is data, not code (`I18N-002`).
- An app pays only for its own locales (`RT-060`, `RT-061`).
- Nothing heavy on the UI isolate (L-6); deterministic artefacts (`CMP-002`).

## Considered options

1. `intl` on the device, `golang.org/x/text` in Go, `flutter_localizations` for widget strings.
2. ICU4X through FFI on the device.
3. **Our own engines over data generated from a pinned CLDR release, shipped per app locale in
   the app bundle.**

## Decision

Chosen option: **3** (maintainer, P8 plan D2–D4).

- **Data.** `make cldr-data` downloads the pinned `cldr-json` release (48.2.3 at L1; version and
  SHA-256 in `mk/l10n.mk`); `tools/cmd/cldrdata` (standard library) reduces it to
  `schema/l10n/cldr/<locale>.json` for every locale CLDR rates at `modern` coverage (104 in
  48.2.3): plural and ordinal rules, symbols per numbering system, decimal, percent, currency,
  accounting and compact patterns, currency symbols and digits, date and time patterns and
  skeletons, names per calendar, layout direction and parent locales. Files are canonical JSON
  and regenerate byte-identically. The source archive is not committed.
- **Flutter's widget strings.** `make flutter-l10n-data` derives `schema/l10n/flutter/<locale>.json`
  from the pinned Flutter SDK's `flutter_localizations` ARB files (read on disk, never a package
  dependency). `plux_flutter` implements `MaterialLocalizations`, `CupertinoLocalizations` and
  `WidgetsLocalizations` over that data, formatting dates and numbers with our engine. A locale
  Flutter has no strings for falls back along the CLDR parent chain to the app default.
- **Bundle.** The app bundle's `l10n` section of each supported locale carries `LocaleData` (only
  what the app uses: its currencies, the date skeletons its documents use, the calendars it
  declares) beside the locale's messages. Plugin bundles carry no messages. Messages are stored
  pre-parsed as a compact AST, so the device never parses MessageFormat.
- **MessageFormat.** One parser in Go (`backend/internal/l10n/mf`): `plural` with `offset` and
  `=n`, `selectordinal`, `select` (gender is a `select`), `number`, `date` and `time` arguments
  with CLDR styles and skeletons. Messages are checked against their key's `args` and the
  locale's plural categories (`PLX-1601`–`1605`). Evaluators in Go (tests, CLI previews,
  pseudo-locales) and Dart share vectors under `schema/testdata/l10n/`.
- **Resolution.** BCP 47 canonicalisation, the CLDR parent chain (`de-AT` → `de` → root), then the
  app's default locale; the compiler resolves each supported locale once.
- **Digits.** Latin by default; a decimal native numbering system (such as `arab`) opts in per app
  and locale (`localisation.numberingSystem`). Ge'ez numerals are out of scope.
- **Runtime.** A Riverpod `LocaleController` holds the effective locale (`PluxConfig.locale`,
  `Plux.setLocale`, the `setLocale` action, else the host's); a switch rebuilds Plux subtrees under
  new `Localizations` and `Directionality` without restarting the engine or losing page state.
  Missing keys fall back along the chain, report `PLX-4401` and the sampled `l10n_missing`
  telemetry event, and never show a key in release builds. The runtime's own strings (`ACT-020`
  messages, secure widgets, the fallback page) are a runtime catalogue shipped like the Flutter
  strings. Node `direction` overrides and `localeOverrides` (override layers, `BND-016`) apply
  per node; text fonts are subset by the scripts of the app's locales (`CMP-032`).

## Consequences

- **Positive:** one source of truth for Go and Dart; locales are data; an app ships only its
  locales; no `intl`, ICU or `flutter_localizations` in the core package.
- **Negative:** we own a formatting engine and a MessageFormat parser; CLDR upgrades are a pinned
  data change reviewed like a dependency.
- **Follow-up:** L1 (data and formatting), L2 (MessageFormat and PXL), L4 (runtime); measured
  bytes per locale recorded in `docs/benchmarks/`.

## Options in detail

### Option 1

Two independent implementations at different CLDR versions drift; `intl` has no Ethiopian or
Umm al-Qura calendar; `flutter_localizations` ships every locale's strings in every app; a new
locale may need a package upgrade.

### Option 2

ICU4X is correct and complete but adds a native library per ABI to the core package and a second
toolchain (Rust) to the app build, which ADR-0011 (P7 plan D10) reserves for Functions.

### Option 3

Costs an engine of our own, bounded by the vectors; matches the libphonenumber precedent
(ADR-0047).

## Revision (2026-10-18)

**The bundle carries only the currency names, territory names and date skeleton formats the app
uses** (maintainer, Q24). The decision above already said so; the compiler had shipped every one
per locale. It now trims them (`backend/internal/compiler/localeuse.go`, `l10n.Reduce`).

One conservative rule covers all three categories: every reference whose value is known at
compile time is kept, and when a reference is dynamic the whole category is kept.

- **Currencies.** Literal: the currency of a money constant or of `money(x, "XXX")` in PXL, a
  literal `currency`/`value`/`currencies` prop of `AmountInput` and `CurrencyPicker`, and
  `::currency/XXX` number skeletons in messages. Dynamic (keeps all): a money value read from
  state, a parameter or a data field, `money` with a computed currency, any declared type that
  mentions `money`, a computed currency prop, and a `CurrencyPicker` without a list. The
  digits of every currency are always kept. The app has no default-currency setting, so none
  is added.
- **Territories.** Only `CountryPicker` and `PhoneInput` name territories, and both list all of
  them; a project using neither carries none.
- **Skeletons.** The skeletons of message `date`/`time` arguments plus the ones
  `plux_flutter`'s Flutter localizations use (pinned by a test). For each, the available
  format the engine would pick is kept, so results are unchanged; the date and time styles are
  always kept.
- **Native widgets.** If the host's native catalogue declares slots, native Dart code may read
  any locale data, so nothing is trimmed.

The runtime already falls back to the ISO code for a currency name and the region code for a
territory name that the data lacks (`trimmed_data_test.dart`). Output stays sorted and
deterministic (`CMP-002`).

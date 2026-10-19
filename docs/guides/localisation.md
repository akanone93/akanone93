<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Localisation Guide

How a Plux app speaks more than one language: the locales it supports, translated messages
in ICU MessageFormat, locale-aware numbers, money and dates, right-to-left layout, and what
the device does when a message is missing (`I18N-001` to `I18N-012`). The design is
[ADR-0066](../adr/0066-localisation-engine.md). Calendars have their own
[guide](calendars.md); how translators work is in the
[translation workflow](translation-workflow.md).

## 1. Supported locales

`app.json` names the locales the app ships and the one it falls back to:

```json
"defaultLocale": "en",
"supportedLocales": ["en", "de-DE", "ar", "am-ET"]
```

The launch locales are `en`, `de-DE`, `ar` and `am-ET`, and any other locale CLDR rates at
`modern` coverage works without code changes: the formatting data is generated from a
pinned CLDR release (`make cldr-data`) and shipped in the app bundle for the app's own
locales only, so an app pays only for what it lists. A locale without data is refused when
the release is compiled (`PLX-1600`). The compiler and the runtime read the same data, so
Studio previews, compile-time checks and the device give identical results.

## 2. Translations

Text is not written into pages. An app has one keys document, `translations/keys.json`, and
one document per locale, `translations/<locale>.json`:

```json
{"schemaVersion": "1.0.0", "kind": "translationKeys", "id": "…", "keys": [
  {"id": "01b0c450-6c00-7000-8000-000000000065", "key": "tasks.count",
   "description": "Number of open tasks", "maxLength": 40,
   "args": [{"name": "count", "type": "int"}]}]}
```

```json
{"schemaVersion": "1.0.0", "kind": "translations", "id": "…", "locale": "de-DE",
 "messages": {"01b0c450-6c00-7000-8000-000000000065": "{count, plural, one {# Aufgabe} other {# Aufgaben}}"},
 "states": {"01b0c450-6c00-7000-8000-000000000065": "approved"}}
```

Messages are keyed by the key's identifier, so renaming a key never breaks a translation.
`states` records each message's review state ([workflow](translation-workflow.md)).

A text prop refers to a message with `$t`, with arguments when the message has them:

```json
{"type": "Text", "props": {"data": {"$t": "01b0c450-6c00-7000-8000-000000000065",
  "args": {"count": {"$expr": "state.openTasks"}}}}}
```

## 3. MessageFormat

Messages use ICU MessageFormat: `plural` with `offset` and `=n`, `selectordinal`, `select`
(gender is a `select`), and `number`, `date` and `time` arguments with CLDR styles and
skeletons:

```text
{count, plural, =0 {No tasks} one {# task} other {# tasks}}
{gender, select, female {She} male {He} other {They}} replied.
Balance: {amount, number, ::currency/EUR}
Due {due, date, ::yMMMd}
```

The compiler parses every message once and stores it as a compact tree, so the device
never parses MessageFormat. It checks each message against its key's `args` and the
locale's plural categories: a syntax error (`PLX-1601`), an undeclared argument
(`PLX-1602`), an argument of the wrong type (`PLX-1603`), a plural category the locale does
not have (`PLX-1604`) and a missing `other` branch (`PLX-1605`). A plural in German must
not use `few`; one in Arabic may.

## 4. Locale overrides and per-locale assets

Where a message is not enough, a node overrides props for one locale. Overrides apply as a
layer over the node's props (`I18N-012`):

```json
{"type": "Image", "props": {"width": 120},
 "localeOverrides": {"ar": {"width": 160}}}
```

An invalid override is refused (`PLX-1613`). A node's `direction` (`ltr`, `rtl` or
`inherit`) overrides the layout direction of its subtree. An image that contains text can
ship one file per locale: the asset index lists the variants of a file under `locales`, and
the device uses the one for the effective locale.

## 5. App settings

`app.json` carries the `localisation` object:

```json
"localisation": {
  "calendar": "gregorian",
  "secondaryCalendar": "islamicUmalqura",
  "numberingSystem": {"ar": "native"},
  "requireApprovedTranslations": false,
  "pseudoLocales": false
}
```

`calendar` and `secondaryCalendar` are described in the [calendars guide](calendars.md).
`numberingSystem` maps a locale to `latn` (Western digits, the default) or `native` (the
locale's own digits); a locale without one is refused (`PLX-1612`).
`requireApprovedTranslations` and `pseudoLocales` belong to the
[translation workflow](translation-workflow.md).

## 6. At run time

The effective locale is `PluxConfig.locale` if the host sets it, else the device's. It
changes with `Plux.setLocale(...)` from host code or the `setLocale` action from a page
([actions reference](../reference/actions.md#setlocale)); the locale must be one the app
supports. A switch rebuilds the Plux subtrees under new `Localizations` and
`Directionality` without restarting the engine or losing page state. Layouts mirror
automatically for right-to-left locales (`I18N-004`).

Flutter's own strings (date pickers, text-field menus, back buttons) and the runtime's own
messages come from the same locale data, so the host needs no `flutter_localizations`.

A message missing in the requested locale falls back along the CLDR parent chain
(`de-AT`, then `de`, then the app's `defaultLocale`). The device reports `PLX-4401` and a
sampled `l10n_missing` telemetry event with the key, the requested and the resolved locale
([telemetry](../reference/telemetry.md)). A release build never shows a key.

## 7. PXL

Expressions can format without a message. The `format` group (`pxl.format.v1`) has
`format.number`, `format.money`, `format.date`, `format.percent`, `format.compact`,
`format.phone` and `format.iban`; the `l10n` group (`pxl.l10n.v1`) has `t(key, args)` and
`plural(n, forms)`; the `calendar` group is in the [calendars guide](calendars.md). All
follow the effective locale. See the [PXL standard library](../reference/pxl-stdlib.md).

## 8. Fonts

Text fonts bundled with the app are subset by the scripts of the app's locales when the
release is compiled (`CMP-032`): an app that supports `en` and `de-DE` ships no Arabic or
Ge'ez glyphs. Adding a locale in a new script adds its glyphs at the next compile.

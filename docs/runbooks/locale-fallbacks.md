# Locale Fallback Runbook

A locale shows the app's default language, or text that looks like a key, instead of the
translation (`I18N-005`, `I18N-006`, ADR-0066).

## How resolution works

- The runtime follows the host's locale unless the app sets one (`PluxConfig.locale`,
  `Plux.setLocale`, the `setLocale` action). A switch needs no restart.
- A locale is canonicalised (BCP 47) and resolved along the CLDR parent chain (`de-AT` → `de` →
  root), then the app's default locale. The compiler resolves each supported locale once.
- A missing message falls back along that chain, reports `PLX-4401` and the sampled
  `l10n_missing` telemetry event, and never shows a key in release builds.
- `PLX-4400` means the app asked for a locale the current release does not contain; the locale
  was not changed.
- With `localisation.requireApprovedTranslations`, messages that are not `approved` are left
  out of the release and fall back (`PLX-1610`, info). The release is not refused.

## Signals

| Signal | Meaning |
|---|---|
| `l10n_missing` telemetry event | Devices fell back for a key |
| `PLX-4401` in development builds and the host app's `onError` | Same, on the device |
| `PLX-4400` | The requested locale is not in the release |
| The release's localisation report | Coverage per locale, missing, unapproved, too long, unused |

## Diagnose

1. **Read the report.** Fetch the localisation report of the release the device runs (served like
   the accessibility report). Find the locale and key: *missing* means no translation was
   saved; *unapproved* means it was left out by `requireApprovedTranslations`.
2. **A whole locale shows the default language.** Check that the release lists the locale
   (`PLX-4400`) and that the device's locale resolves to it along the parent chain. A locale
   with no data in the release falls back to the default.
3. **Flutter's own strings** (date pickers, `OK`, `Cancel`) in the default language: Flutter has
   no strings for that locale, so the CLDR parent chain applies (ADR-0066).
4. **Text looks like a key.** This must not happen in a release build. Check that the build is a
   release build, and report it as a defect with the key and the release.

## Fix

- Missing: translate the key in Studio, or import (see [Translation import](translation-import.md)),
  then publish a new release; translations ship in the app bundle.
- Left out as unapproved: have a user with `translations.approve` approve the keys, then publish.
- Locale absent: add it to the app's supported locales and publish.
- Devices keep the release they hold until their next sync; check adoption of the new release.

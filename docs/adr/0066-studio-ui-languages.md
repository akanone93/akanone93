# 0066. Studio's own UI ships in English, German and Arabic

- **Status:** Accepted
- **Date:** 2026-10-16
- **Requirements:** `STU-012`, `STU-014`, `I18N-002`

## Context and problem

`STU-012` required Studio's own UI to be localised "at least in English and German, with right-to-left support". Neither English nor German is written right to left, so the requirement asked for RTL support without a locale that would exercise it: RTL could only be tested with a pseudo-locale, and no real user could run Studio in an RTL language. The product's own launch locales already include Arabic to prove RTL on the device (`I18N-002`). This decision answers which locales Studio itself ships with.

## Decision drivers

- RTL support is proven by a real locale, not only by a pseudo-locale.
- Studio's locales match the product's RTL launch locale, so translators, fonts and tests are shared.
- Adding a locale stays data only, not code.
- The cost of a third catalogue is small while P11 has not started.

## Considered options

1. Keep English and German, and prove RTL with a pseudo-locale only.
2. Add Arabic as Studio's third locale (chosen).
3. Add Arabic and Amharic, matching all of `I18N-002`.

## Decision

Chosen option: **add Arabic** (maintainer, 2026-10-16). Studio ships in English (source), German and Arabic; Arabic proves right-to-left layout in the chrome. Pseudo-locales remain for tests ([studio-development.md](../engineering/studio-development.md) §7.7). `STU-012` is reworded in spec 1.5.2.

## Consequences

- **Positive:** RTL is tested and usable with real text from the first P11 screen; the same locale proves RTL on the device and in Studio.
- **Negative:** every Studio string needs an Arabic translation before a release; the UI typeface must cover Arabic script.
- **Follow-up:** the typeface and message-formatting library are chosen with the Studio stack's ADR before P11 code.

## Options in detail

### Keep English and German

Cheapest. RTL would rest on a mirrored pseudo-locale, which finds layout faults but not shaping, line-breaking or bidirectional text with real Arabic, and no user could use Studio in an RTL language.

### Add Arabic

One more catalogue and font coverage. Exercises real shaping, bidirectional text and mirrored layout; matches `I18N-002`.

### Add Arabic and Amharic

Would also prove Ethiopic script in the chrome. Amharic remains possible later as data; it is not needed to prove RTL.

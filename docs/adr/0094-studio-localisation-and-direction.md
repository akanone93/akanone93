<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0094. Studio localisation and direction

- **Status:** **Accepted** (maintainer, 2026-10-24; P11 plan M-9, O-7)
- **Date:** 2026-10-24
- **Requirements:** `STU-012`, `I18N-010`

## Context and problem

Studio's own interface must be localised and must run left-to-right and right-to-left
(`STU-012`); the language is a profile setting (`STU-014`). The Studio rule book cited the
decision as "ADR-0066". That number belongs to P8's localisation engine, which localises the
*apps users build* (spec §17); the plan reallocated Studio's decision to this number (L-3).
This ADR covers Studio's chrome only. The question: which library, which locales, and which rules
keep every screen correct in both directions without per-screen effort?

Sources: [p11-studio-decisions.md](../plans/p11-studio-decisions.md) (A4.13, A5.4) and the rule
book sections on localisation and direction; the phase plan is [p11.md](../plans/p11.md) (L-3,
L-11, M-1).

## Decision drivers

- ICU MessageFormat, the standard Plux apps use (`I18N-001`), with CLDR plurals through `Intl`.
- Catalogues compiled at build time: a small runtime, no parsing in the browser, CSP-friendly.
- Right-to-left proven by a real locale (`I18N-002`), not assumed.
- Rules a linter can enforce, so correctness does not depend on reviewer attention.
- Studio's strings manageable like an app's (`I18N-009`).

## Considered options

1. Lingui 6 with explicit message keys and build-time catalogues.
2. react-i18next.
3. FormatJS.

## Decision

Chosen option: **1**, because it is ICU-native, compiles catalogues ahead of time and supports
typed explicit keys.

### Library and catalogues

- **Lingui 6** with its Vite plugin. Messages are declared in `<feature>.messages.ts` with an
  explicit stable key (`<area>.<feature>.<name>`), the English source text and a description for
  translators. Strict extraction fails `make studio-check` on missing and unused keys.
- No literal UI text: labels, placeholders, `aria-label`, `title`, toasts, errors, empty states
  and document titles all come from the catalogue. A lint rule rejects string literals in JSX
  text and accessibility attributes. Product names and user content are the only exceptions.
- Catalogues import and export as XLIFF 2.0 and JSON (`I18N-009`), so Studio's translation uses
  the workflow Plux gives its users.

### Locales, negotiation and formatting

- **Shipped locales:** `en` (source), `de`, `ar`. Arabic proves right-to-left and is the
  product's RTL launch locale (`I18N-002`). Adding a locale is data only: a catalogue and an
  entry in the locale list.
- **Pseudo-locales** in development and tests: `en-XA` (accented, 40 % expanded) and `ar-XB`
  (mirrored right-to-left), matching `I18N-010`. The Playwright locale sweep over the component
  catalogue page runs in `en`, `en-XA` and `ar-XB`.
- **Negotiation:** the choice is stored in the user's profile on the server. Before sign-in the
  BFF negotiates from `Accept-Language`, and the sign-in page can override it. Switching needs
  no reload: the shell updates `<html lang>` and `<html dir>`, mounted strings re-render and
  focus stays put.
- **Formatting:** plurals, numbers, dates, relative times and lists use `Intl` with the active
  locale (CLDR). No concatenated sentences, no manual plural suffix.
- **Fallback:** `de-AT` → `de` → `en`. A missing translation is reported in development and in
  tests; a key is never shown to a user.
- User content (app, plugin, screen and person names) renders with `dir="auto"` or `<bdi>` so a
  name in another script does not reorder the surrounding text.

### Direction

- **Logical properties only.** CSS and Tailwind use `margin-inline-*`, `padding-inline-*`,
  `inset-inline-*`, `text-start`, and the `ms-`, `me-`, `ps-`, `pe-`, `start-`, `end-`, `rounded-s-`
  and `rounded-e-` utilities. Physical left and right properties and utilities are rejected by a
  lint rule from the first component.
- Direction is read from the direction provider (Base UI), never compared inline in components.
- In lists, tabs, menus and trees the "next" arrow key follows reading direction.
- Docked panels swap sides in RTL; the spec's left and right panels (`STU-061`, `STU-062`) mean
  the start and end panels.
- **Mirrored icons.** Phosphor carries no mirror metadata, so the icon catalogue marks every icon
  `mirrors: true | false`. Directional icons (back, forward, chevrons, undo and redo arrows,
  send, reply) mirror; non-directional ones (play, search, check, the Plux logo, clocks, media
  controls) never do.
- **The canvas does not mirror with the chrome.** Its coordinates, rulers, zoom and nudging stay
  physical; a device frame follows the *preview* locale (`STU-058`). Studio direction and
  preview direction are independent and never derived from each other.
- JSON, PXL, Rust, identifiers, URLs, versions and shortcuts render in LTR-isolated spans, and
  Monaco and CodeMirror are always LTR.

### Arabic fonts (M-1)

Studio's Latin interface fonts (Instrument Sans and Serif, DM Mono) cover Latin only. For Arabic
the maintainer approved **Noto Sans Arabic for the sans roles and Noto Naskh Arabic for the serif
roles**; the Latin mono is kept for code. All are OFL-1.1 and self-hosted.

### Translations (L-11)

Agents draft `de` and `ar` with every string. A native-speaker review of both is the
maintainer's, **before 0.0.1**, and is recorded in the work log when done.

## Consequences

- **Positive:** the rules are lint-enforced; RTL and expansion are exercised in every
  component state; Studio's strings use the product's own translation workflow.
- **Negative:** every string needs a key and a description; two pseudo-locales add test time;
  `de` and `ar` are agent-drafted until the maintainer's review.
- **Follow-up:** the `i18n` package and lint rules (S0.1); the locale sweep (S0.7); the native
  review before 0.0.1.
- **Supersession note:** where the rule book or older documents say "ADR-0066" for Studio's
  localisation, they mean this ADR; ADR-0066 stays P8's engine.

## Options in detail

### Option 2 — react-i18next

Its default message format is not ICU and runtime parsing grows the bundle; plural handling
differs from the format Plux apps use. Rejected.

### Option 3 — FormatJS

ICU-native, but a heavier runtime and no compile-time macros comparable to Lingui's. Rejected.

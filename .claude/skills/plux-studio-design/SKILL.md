---
name: plux-studio-design
description: Build or change any Plux Studio interface (studio/apps/web, studio/packages/ui, studio/packages/canvas) in the approved Porcelain design. Use before writing or reviewing markup, styles, components, icons or canvas visuals in studio/.
---

<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Plux Studio design

The approved design is defined in text and code, never in images:

1. `docs/engineering/studio-ui-language.md` — the UI language (U1–U19). Read U1, then only the
   sections your change touches.
2. `docs/engineering/studio-ui-reference.html` — the approved plugin canvas as markup. Search it for
   the pattern you need; do not open it in a browser or take screenshots (rule book §13.1).
3. `studio/packages/brand` — the tokens. `studio/packages/ui/CATALOGUE.md` — the components.

## Build

- Assemble screens from catalogue components. Read `CATALOGUE.md` before writing markup; use a
  component by name instead of re-writing its classes (rule book §5.1, §13.6).
- A pattern the catalogue lacks: add it to the reference page and the UI language first, then build
  it as a shared component with its catalogue entry and `*.cases.tsx`, then use it.
- Values come from tokens only: no hex, rgb, px, rem or shadow literals, no Tailwind arbitrary
  values (rule book §4.2).
- Typography: serif (`text-screen`, `text-title`, `text-section`) names things; sans for controls and
  sentences; mono (`text-data`, `text-tag`) for every number with a unit, identifier, path and code.
- Icons: the `Icon` component (Phosphor duotone) only; mark each new icon's `mirrors` value in the
  catalogue.
- Glass (`glass` utility) only on floating chrome; never glass on glass; on bare glass use only
  `text-primary` and `text-secondary-glass`.
- Violet only where U2.4 allows it. Status always as icon plus word or number.
- Information: prefer the U10 patterns (status pill, count, type tag, metric strip, meter, sparkline,
  live chip, summary card) over new ones.
- Motion: motion tokens only; one looping animation in the product (the live dot); everything
  honours reduced motion.

## Check before you finish

- [ ] Built from catalogue components; any new component has its entry and cases.
- [ ] No raw value, no foreign icon, no font family outside the tokens (`make studio-check`).
- [ ] Serif, sans and mono used in their roles (U7).
- [ ] Contrast pairs are tokens already measured in U2.5, or the token contrast test covers the new
      pair.
- [ ] Works in `en`, `en-XA`, `ar-XB`; logical properties only (rule book §8, §12.4).
- [ ] Verified textually: tests, scene-model assertions, the accessibility tree (rule book §13.2).
      No screenshots.
- [ ] At a milestone exit gate: the design gate of the UI language U18.6 ran and its findings are
      fixed or listed for the maintainer.

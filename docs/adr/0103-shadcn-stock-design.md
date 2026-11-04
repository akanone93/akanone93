<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# 0103. Studio uses shadcn/ui as it ships: Base UI, the base-nova style, its neutral theme, Inter and lucide

- **Status:** Accepted (maintainer, 2026-10-30)
- **Date:** 2026-10-30
- **Requirements:** `STU-001`, `STU-010`, `STU-011`, `STU-012`, `STU-014`, `STU-016`, `CI-007`

## Context and problem

`STU-001` and [ADR-0014](0014-studio-stack.md) chose shadcn/ui on Base UI, but Studio never used
shadcn's components: it built its own on Base UI in a custom visual language (Porcelain,
[studio-ui-language.md](../engineering/studio-ui-language.md)), with four typefaces, Phosphor
duotone icons, a Tailwind theme that removed every default, and a design skill with an approved
HTML reference page. On review (2026-10-30) the maintainer found the shared components
inconsistent and hard to use, the typography inconsistent, and the screens below the sign-in
picture unlike the rest. They decided that Studio uses shadcn's components, layouts and blocks
exactly as shadcn ships them, with its default font, and that the custom design skill and its
reference page are removed. The sign-in page's ring of sample apps stays as it is.

## Decision drivers

- Components that look and behave the same everywhere, from one maintained source.
- As few local changes to that source as possible, so it can be updated from upstream.
- Studio's own rules stay: localisation (`STU-012`), right-to-left layout, the contrast theme and
  WCAG 2.2 AA (`STU-011`), the light, dark and system themes (`STU-014`), deny-by-default
  dependencies (`CI-007`).
- No new network access at runtime and no new runtime dependency where shadcn's own tools make
  one avoidable.

## Considered options

1. **shadcn/ui as it ships** — the defaults of `shadcn init` for a Vite app on Base UI.
2. Keep Studio's components and restyle them in shadcn's look.
3. Another component library (Mantine, Chakra, MUI).

## Decision

Chosen option: **shadcn/ui as it ships**.

- **Base and style.** `base` (Base UI) with the `nova` style, shadcn's default; `rtl` on.
  Components live in `studio/packages/ui/src/components/` and are imported as
  `@plux/ui/components/<name>`. `components.json` in `studio/packages/ui` records the setup.
- **How they are added.** `ui.shadcn.com` is not reachable from every build environment, so a
  component is generated from the Base UI sources of `shadcn-ui/ui` with the shadcn CLI's own
  exported transforms (`createStyleMap`, `transformStyle`, `transformIcons`, `transformMenu`,
  `transformDirection`) in the order shadcn's registry build applies them; only import paths are
  rewritten. Where the CLI can reach the registry, `shadcn add` is used instead.
- **Theme.** shadcn's `zinc` base colour with its `indigo` theme (amended again by the maintainer on
  2026-11-01, who found the tinted theme "not industry standard/professional" and asked for more
  frosted glass, small solid colours instead of glows, and clear contrast between the sidebar, the
  content area, the header and the footer). The surfaces are neutral; the sidebar and the frame
  around the page are the darkest plane, the page a raised panel with a hairline edge, the cards a
  step above it, in light and dark alike. Colour comes in small solid marks: the primary, the five
  chart hues (indigo, violet, emerald, amber, rose) on figure tiles and plugin marks, the status
  colours on dots and pills; there are no washes of several hues and no coloured glows. What floats
  (the header, menus, popovers, selects, hover cards, dialogs, sheets and the canvas's panels) is
  frosted glass: a translucent fill, a 28-pixel blur with raised saturation, a hairline edge and a
  light inner highlight. Every text pair stays at WCAG AA, checked by `theme-css.test.ts`. The app
  canvas reads its scene colours from the same tokens, so it follows light, dark and contrast
  (amended by the maintainer on 2026-11-02, who found the app canvas "not a canvas at all", and
  again the next day): the canvas is a step off the page with a breath of cool blue
  (`--canvas-surface`, barely noticeable) and a quiet dot grid drawn by Plux Canvas (a dot every
  24 world units, the dots one CSS pixel in radius at every zoom, the next finer level fading in as
  the view zooms so the grid never jumps); a plugin box is a card resting on it with a soft, low
  shadow (a faint contact ring where the canvas draws without a GPU): a head with the plugin's
  solid mark (a primary tile with its initial), its name in bold on one line, its key and a
  release chip (`v3`, or `Draft`), a row of figures (screens, versions, links), and a tinted
  status strip along its foot with the badges as pills, each with a solid dot; a box over another
  hides it; text stays text at every zoom. The canvas's glass panels are lighter than the page's:
  a 62 % fill with a faint top-down sheen, a 24-pixel blur, a hairline and a bright top edge, and
  a creamy elevation of three soft, faint shadows instead of the third elevation; the bars share
  one height and round less than the side panels; the contrast theme, forced colours and reduced
  transparency make them solid.
  Earlier text: shadcn's `mist` base colour with its `indigo` theme (amended by the maintainer on
  2026-10-31, who found the stock `neutral` light theme flat: "a little bit of colour and contrasted
  whitish colours"). Studio tints the light page a cool off-white so white cards stand off it, gives
  hovers an indigo wash, uses the indigo ring and five Tailwind hues for the charts, and keeps every
  text pair at WCAG AA, checked by `theme-css.test.ts`. On top of the theme, `styles/surfaces.css`
  adds a soft wash of colour behind the pages, elevation shadows for cards and popups, and frosted
  glass (backdrop blur) for the header, menus, popovers, selects, hover cards and dialogs; the
  contrast theme, forced colours and reduced transparency get solid surfaces. It styles shadcn's
  `data-slot`s only, so the components stay shadcn's. Earlier text of this decision: shadcn's
  `neutral` theme and the `@theme inline` mapping `shadcn init` writes,
  generated by `@plux/brand`'s `theme-css.ts` into `theme.gen.css`, keyed to Studio's
  `data-theme` attribute. Tailwind's defaults stay in place. Studio adds only what shadcn has no
  token for: the contrast theme, the status colours, selection and arrow colours of the canvas,
  and the sample-app colours. Older Studio token names that do not clash with shadcn's remain as
  aliases of shadcn's variables, so screens not yet rebuilt follow its look.
- **Fonts.** Inter (`@fontsource-variable/inter`), shadcn's default; Noto Sans Arabic stays for
  Arabic, which Inter does not cover. Instrument Sans, Instrument Serif, DM Mono, Sora and Noto
  Naskh Arabic are removed. Studio's type roles for headings, body and labels follow shadcn's scale
  (`text-2xl` to `text-sm`), so screens not yet rebuilt read like the rebuilt ones.
- **Icons.** lucide (`lucide-react`), shadcn's icon library. The `Icon` component keeps Studio's
  icon names and maps them to lucide icons; the Phosphor generator and `make studio-gen-icons`
  are removed.
- **Helpers.** `cn` (shadcn-ui's class-name merger, which replaces clsx and tailwind-merge) and
  `class-variance-authority`. `shadcn/tailwind.css` is ejected into
  `apps/web/src/styles/shadcn.css` as `shadcn eject` does, so `shadcn` is not a runtime
  dependency; `tw-animate-css` is a build-time stylesheet.
- **Layouts and blocks.** The signed-in shell is shadcn's sidebar layout (blocks `dashboard-01`
  and `sidebar-07`); the sign-in page is block `login-02` with the ring in its picture half.
- **Studio's own components** (`Button`, `Field`, `Select`, `Menu`, `Dialog` and the rest of
  `@plux/ui`) keep their props and render shadcn's components, so every screen moves at once.
  `Select` and `Menu` are not modal and the select list opens below its trigger, because Base UI
  locks the page's scroll for a modal popup or a list laid over its trigger.
- **Local changes to shadcn's sources** are kept to the ones listed here: an unused React import
  removed from `scroll-area.tsx`, and an explicit empty child for the icon of `SelectTrigger`, so
  Base UI's default "▼" is not drawn inside the lucide icon.
- **Lint.** The generated components, `shadcn.css` and `use-mobile.ts` are excluded from Biome
  and from the physical-side check, as generated code is; shadcn's RTL transform already made
  them logical except for the `side` a caller names.
- **Motion.** `motion` (`motion/react`) animates the sign-in ring and the home page's entrance;
  it was added with the sign-in page and is recorded here.
- **Skills.** The official shadcn agent skill (`shadcn-ui/ui` `skills/shadcn`) is added under
  `.claude/skills/shadcn`; the `plux-studio-design` skill and
  `docs/engineering/studio-ui-reference.html` are removed.

## Consequences

- **Positive:** one component source, documented upstream; a design any React developer knows;
  fewer local rules to learn; future components come from the same registry.
- **Negative:** Studio looks like other shadcn products; the Porcelain sections of the UI
  language no longer describe the interface; shadcn's components use Tailwind arbitrary values,
  which Studio's lint forbids in its own code, so they are exempt.
- **Follow-up:** rebuild the remaining screens with shadcn's components and blocks; remove the
  older token aliases once no screen uses them; revise the UI language to describe the shadcn
  design or retire it.

## Options in detail

### shadcn/ui as it ships

Meets every driver: one maintained source, upstream updates, the rules of Studio kept through
the theme and the wrappers. Costs the custom look and needs the offline generation path.

### Keep Studio's components and restyle them

Keeps today's APIs and markup but leaves Studio maintaining every component and its accessibility
alone, which is what produced the inconsistency.

### Another component library

A styled library brings its own design language and runtime theming, against ADR-0014's choice of
components Studio owns as source.

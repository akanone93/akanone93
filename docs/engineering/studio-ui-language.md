<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# P11 Studio — UI Language (Style, Look and Feel)

- **Status:** Accepted by the maintainer on 2026-10-22. Five directions were mocked up on the plugin
  canvas (Ledger, Atelier, Porcelain, Signal, Halo); the maintainer chose **Porcelain** and asked for
  Atelier's typefaces, screen titles and intentional detail and Signal's compact, information-rich
  presentation to be brought into it without changing Porcelain's identity. This file replaces the
  version of 2026-10-20 (Electric Violet on frosted panels with Noto Sans and Lucide).
- **Reference page:** [studio-ui-reference.html](studio-ui-reference.html) — the approved plugin
  canvas as one self-contained HTML file. Open it in a browser. It is the visual source of truth for
  this file; where they disagree, this file wins and the page is corrected.
- **Home:** this file and the reference page beside it moved here from `docs/plans/` on 2026-10-24
  (P11 S0.0).
- **Read with:** [p11-studio-decisions.md](../plans/p11-studio-decisions.md) (stack),
  [studio-development.md](studio-development.md) (the rule book),
  [p11-studio-functional-decisions.md](../plans/p11-studio-functional-decisions.md),
  [p11-studio-routing.md](../plans/p11-studio-routing.md), and the agent skill drafted in
  [p11-studio-design-skill.md](../plans/p11-studio-design-skill.md).
- **Binding rules kept:** tokens only, no raw values (rule book §4.1–§4.2); computed contrast
  (§4.5); logical properties and RTL (§8); motion tokens with reduced motion (§4.7); brand tokens of
  Appendix J (`STU-016`); WCAG 2.2 AA (`STU-011`); 60 fps canvas (`STU-120`).
- *Measured* contrast ratios below were computed with the WCAG 2.2 formula used by
  `studio/packages/brand/src/contrast.ts`.

---

## U1. Principles

1. **Porcelain.** A cool, bright, quiet workspace. Content and the user's app are the colour.
2. **The canvas is the room; chrome floats in it.** Three capsules at the top, two panels and a
   drawer float over the canvas on frosted glass; the canvas runs underneath them, edge to edge.
3. **Soft and round.** Capsules, pills and generous radii; soft shadows tinted toward the brand
   violet; no hard boxes.
4. **Serif names, sans works, mono measures.** A serif names things (screens, panels, sections, the
   plugin); the sans carries every control and sentence; a monospace carries every number, code,
   path and identifier.
5. **Dense, not crowded.** Every surface carries live information — status under each screen,
   counts in the layer tree, metrics in the drawer — in small, quiet, aligned pieces.
6. **Colour is meaning.** Navy structures and reads; violet acts and selects; green, amber and red
   report status. Nothing is coloured for decoration.
7. **Calm motion.** Short, purposeful transitions; one looping animation in the whole product (the
   live-device pulse).
8. **Performance decides.** Every effect has a cheaper fallback that switches in before frame rate
   suffers (U3).

## U2. Colour

### U2.1 Proportions

| Share | Role | Colours |
|---|---|---|
| ~75 % | Dominant surfaces | White `#FFFFFF`, porcelain `#F2F4F9`, alabaster `#F7F8FB` (Appendix J `brand.surface`) |
| ~18 % | Structure and text | Deep Navy family from `brand.ink` |
| ~5 % | Accent | Electric Violet `brand.violet` `#5B3DF5` |
| as needed | Status | Green (success), amber (warning), red (danger) |

### U2.2 Palette — light theme

Brand tokens are Appendix J's; the rest are new primitive tokens of `@plux/brand`.

| Primitive token | Value | Use |
|---|---|---|
| `navy.900` | `#11142A` | Primary text, titles |
| `navy.700` | `#3C4262` | Secondary text and icons on solid surfaces |
| `navy.650` | `#30364F` | Secondary text on bare glass (U3) |
| `navy.500` | `#555C7C` | Tertiary text, captions, type tags — solid surfaces and section cards only |
| `navy.400` | `#7A81A3` | Arrows, disabled icons |
| `navy.alpha16` | `rgba(17,20,42,.16)` | Strong hairlines, hovered field borders, separators in capsules |
| `navy.alpha08` | `rgba(17,20,42,.08)` | Hairlines, field borders, table rules |
| `navy.alpha05` | `rgba(17,20,42,.05)` | Hover fill, tab tracks, search fields |
| `porcelain.100` | `#F2F4F9` | Canvas background |
| `brand.surface` | `#F7F8FB` | Page background (`shell`, `page` layouts) |
| `white` | `#FFFFFF` | Device frames, dialogs, inputs, selected tab, selected row |
| `brand.violet` | `#5B3DF5` | Primary action, selection, focus ring, active lanes |
| `violet.700` | `#4A2DE6` | Primary action hover; selected text on `violet.50` |
| `violet.50` | `#EEEAFE` | Data-binding chips, selected list items in pages |
| `violet.halo` | `rgba(91,61,245,.12)` | Halo around selection, the edited screen and focused fields |
| `green.700` | `#0B7A5A` | Success text and icons |
| `green.50` | `#E2F4EC` | Live-device chip fill |
| `amber.800` | `#8F5300` | Warning text and icons |
| `amber.50` | `#FFF1D6` | Warning pill fill, warning counts |
| `red.600` | `#D92D20` | Danger fills, destructive buttons |
| `red.700` | `#B42318` | Danger text |

### U2.3 Semantic tokens (what components use)

| Semantic token | Light | Dark |
|---|---|---|
| `surface.canvas` | `porcelain.100` | `#0B0D17` |
| `surface.app` | `brand.surface` | `#0B0D17` |
| `surface.raised` (frames, dialogs, inputs) | `white` | `#141726` |
| `surface.glass` | white at 66 % + blur (U3) | `#141726` at 80 % + blur |
| `surface.card` (section cards in panels) | white at 70 % + inset hairline | `#1A1E30` at 80 % |
| `surface.hover` | `navy.alpha05` | `rgba(255,255,255,.06)` |
| `surface.selected` | `white` + `elevation.1` | `#22263A` |
| `text.primary` | `navy.900` | `#F5F6FA` |
| `text.secondary` | `navy.700` | `#B4BAD0` |
| `text.secondaryOnGlass` | `navy.650` | `#B4BAD0` |
| `text.tertiary` (solid surfaces and cards only) | `navy.500` | `#A3AAC2` |
| `text.onAccent` | `white` | `white` |
| `border.hairline` | `navy.alpha08` | `rgba(255,255,255,.08)` |
| `border.strong` | `navy.alpha16` | `rgba(255,255,255,.16)` |
| `accent.primary` | `brand.violet` | `#8B74FF` |
| `accent.primaryHover` | `violet.700` | `#7257FF` |
| `accent.halo` | `violet.halo` | `rgba(139,116,255,.20)` |
| `focus.ring` | `brand.violet` | `#8B74FF` |
| `status.success` / `.surface` | `green.700` / `green.50` | `#38D9A9` / `rgba(56,217,169,.12)` |
| `status.warning` / `.surface` | `amber.800` / `amber.50` | `#FFC53D` / `rgba(255,197,61,.12)` |
| `status.danger` / `.surface` | `red.700` / `#FEECEB` | `#F97066` / `rgba(249,112,102,.12)` |
| `canvas.arrow` | `navy.400` | `#6B7390` |
| `canvas.arrowActive` | `brand.violet` | `#8B74FF` |

### U2.4 Where violet is allowed

The Publish button (the only solid violet button), selection on the canvas and in lists, the halo
of the screen being edited, focus rings, toggles that are on, data-binding chips, the lanes of a
hovered or selected arrow bundle, the `x` of the wordmark. Never for headings, body text on glass,
large fills or decoration. (AI actions will use it from P12.)

### U2.5 Measured contrast (light)

| Pair | Ratio | Allowed for |
|---|---|---|
| `navy.900` on white / on `porcelain.100` | 18.15 / 16.49 | all text |
| `navy.700` on white | 9.78 | all text on solid surfaces |
| `navy.500` on white / on `porcelain.100` | 6.55 / 5.95 | text on solid surfaces |
| `navy.500` on a section card over glass over black (worst case) | 5.20 | text in cards |
| `navy.900` / `navy.650` on bare glass over black (worst case) | 7.63 / 5.00 | text on bare glass |
| `navy.700` / `navy.500` on bare glass over black | 4.11 / 2.75 | **not allowed** on bare glass |
| `navy.400` on `porcelain.100` | 3.47 | arrows, icons, UI parts (3:1) — not text |
| white on `brand.violet` | 6.12 | button labels |
| `violet.700` on `violet.50` | 6.35 | binding chips, selected labels |
| `green.700` on `green.50` / on white | 4.66 / 5.32 | live chip, success text |
| `amber.800` on `amber.50` | 5.52 | warning pills and counts |

Status is never shown by colour alone: always an icon and a word or a number.

## U3. Glass — the frosted floating surface

| Property | Light | Dark |
|---|---|---|
| Fill | `rgba(255,255,255,.66)` | `rgba(20,23,38,.80)` |
| Backdrop | `blur(28px) saturate(180%)` | `blur(28px) saturate(140%)` |
| Inner edge | 1 px `rgba(255,255,255,.72)` | 1 px `rgba(255,255,255,.06)` |
| Outer edge | 1 px `rgba(17,20,42,.05)` | 1 px `rgba(0,0,0,.40)` |
| Shadow | `elevation.2` (U5) | `elevation.2` |

- **Used on:** the three top capsules, the left and right panels, the bottom drawer, menus,
  popovers (popovers at 88 % fill, 20 px blur), the command palette, toasts, the top bar of `page`
  layouts.
- **Never on:** canvas items, inputs, dialogs (solid white over a blurred scrim), anything stacked on
  another glass surface. Section cards inside panels are translucent white without a backdrop filter,
  not glass.
- **Text on glass:** only `text.primary` and `text.secondaryOnGlass` on bare glass; tertiary text
  lives inside section cards (U2.5). No violet text on glass.
- **Dialog scrim:** `rgba(17,20,42,.24)` + `blur(8px)`.
- **Performance rules:**
  1. Blur only on the floating chrome; its area is bounded by layout (panels ≤ 480 px wide, drawer
     ≤ 50 % height).
  2. Blur radius never exceeds 28 px; no animated blur radius.
  3. **Automatic solid fallback** (`surface.raised` at 96 %, no backdrop filter) when any of:
     `prefers-reduced-transparency`; the high-contrast theme; the main-thread canvas fallback
     (decisions A5.1); the canvas worker reports frames over budget during pan or zoom (switches for
     the rest of the session, visible in profile settings); the user setting "Reduce transparency".
  4. The S0 prototype measures `STU-120` with glass on and off on the reference laptop; if glass
     costs frames there, the solid fallback becomes the default and glass the option.

## U4. Shape

| Token | Radius | Use |
|---|---|---|
| `radius.xs` | 6 px | Keyboard hints, small tags |
| `radius.sm` | 10 px | Fields, binding chips, list items in pages |
| `radius.md` | 12 px | Layer-tree rows, menu items, small cards |
| `radius.lg` | 18 px | Section cards in panels, cards in pages |
| `radius.xl` | 24 px | Panels, drawer, dialogs, popovers (20 px) |
| `radius.full` | 9999 px | Capsules, buttons, tabs and tab tracks, search fields, pills, badges, avatars, toggles |

The spec's `STU-010` carries this wording since 1.10.0 (generously rounded corners: 18–24 px on
panels, cards and dialogs, full pills on controls, smaller radii only for small elements; O-12).

Device frames on the canvas use their device's real corner radius. Nested radii follow
`inner = outer − padding` so curves stay concentric.

## U5. Elevation

Shadows are tinted toward the brand violet, never neutral grey.

| Token | Light | Use |
|---|---|---|
| `elevation.1` | `0 1px 2px rgba(17,20,42,.08), 0 2px 8px -2px rgba(17,20,42,.10)` | Selected tab, selected row, count badges |
| `elevation.2` | `0 1px 1px rgba(30,24,90,.04), 0 16px 40px -8px rgba(48,34,150,.14)` | Capsules, panels, drawer, popovers |
| `elevation.frame` | `0 0 0 1px rgba(17,20,42,.05), 0 24px 48px -16px rgba(48,34,150,.20)` | Device frames ("paper" shadow) |
| `elevation.frameActive` | `elevation.frame` + `0 0 0 7px violet.halo` + violet 1 px edge | The screen being edited |
| `elevation.primary` | `0 6px 16px -6px rgba(91,61,245,.60)` | Publish button |
| `elevation.4` | `0 24px 64px rgba(17,20,42,.18), 0 4px 12px rgba(17,20,42,.08)` | Dialogs, overlays, command palette |

Dark theme: same geometry, black at double opacity, plus the 1 px outer edge.

## U6. Space and density

- Base unit 4 px; scale `space.0.5 … space.16` = 2, 4, 8, 12, 16, 20, 24, 32, 40, 48, 64 px.
- **Compact** (default) and **comfortable** density as token sets (rule book §4.4).

| Element | Compact | Comfortable |
|---|---|---|
| Capsule height / its buttons | 48 / 36 px | 52 / 40 px |
| Panel fields, tab track | 30 / 28 px (3 px track padding) | 36 / 32 px |
| Layer-tree row | 30 px | 36 px |
| Table row in the drawer | 30 px | 36 px |
| Section card padding | 12 × 14 px | 16 × 18 px |

- Floating chrome sits **16 px** from the viewport edge and from each other. Left panel 272 px
  (240–400), right panel 304 px (280–480), drawer 176 px tall by default.

## U7. Typography

Three open typefaces, one role each.

| Token | Typeface | Size / line | Use |
|---|---|---|---|
| `type.screen` | Instrument Serif | 26 / 28 | Screen names above device frames |
| `type.title` | Instrument Serif | 24 / 26 | Panel titles, page titles, dialog titles |
| `type.plugin` | Instrument Serif | 19 / 22 | The plugin or app name in the identity capsule |
| `type.section` | Instrument Serif | 17 / 20 | Section headings in panels and pages; popover headers |
| `type.body` | Instrument Sans 400 | 13 / 18 | Default text |
| `type.label` | Instrument Sans 500 | 12–13 / 16 | Buttons, tabs, field labels |
| `type.caption` | Instrument Sans 400–500 | 11.5–12 / 16 | Captions under titles, metadata |
| `type.data` | DM Mono 400–500 | 11 / 16 | Numbers with units, versions, revisions, paths, bindings, durations |
| `type.tag` | DM Mono 500 | 9.5–10.5 / 12 | Node type tags (`SCR`, `TXF`, `BTN`), counts, pills |

- **Serif rules:** the serif names things; it never labels a control, never sets a number, never
  runs below 17 px, never sets a sentence.
- **Mono rules:** every number shown with a unit, every identifier, every path and every code uses
  `type.data`; tabular numerals everywhere.
- **Sans:** sentence case everywhere; no uppercase labels; no letter-spacing changes except the
  wordmark (−3 %).
- **Wordmark:** Sora SemiBold per Appendix J (`plu` in ink, `x` in violet).
- **Licences:** Instrument Sans, Instrument Serif and DM Mono are OFL-1.1 (to verify on each file at
  import, legal review per decisions A1.2); self-hosted (rule book §11.5).
- **Arabic (decided, M-1 of the P11 plan):** none of the three covers Arabic. Noto Sans
  Arabic for the sans roles, Noto Naskh Arabic for the serif roles, the Latin mono kept for code
  (rule book §8.7). Arabic uses the same sizes, line heights +2 px at `type.body` and below.

## U8. Icons

- **Set:** Phosphor Icons (MIT), **duotone** weight: a 1.5-weight outline with a 20 % fill layer in
  the same colour. It matches the rounded shapes and gives the chrome its own character. Replaces
  Lucide (decisions A4.20).
- **Source:** the SVG assets of `@phosphor-icons/core` (2.1.1 checked in the session) through one
  `Icon` component of `@plux/ui` that inlines the paths; allowlist entry and ADR first
  (`dependencies.md`).
- **Sizes:** 17 px in capsules, 14 px in tabs, fields and tables, 12 px inside pills.
- **Colour:** `currentColor`; `text.secondary` at rest, `text.primary` on hover, `status.*` inside
  status pills, `accent.primary` only where violet is allowed (U2.4).
- **One set only:** no other icon set in Studio's chrome. The apps' own icons stay the Material
  Symbols and Cupertino sets the server embeds.
- **Mirroring:** the set carries no RTL metadata; the catalogue keeps the `mirrors` list (rule book
  §8.3).

## U9. Layout

### U9.1 Canvas layout (`canvas`, C1/C2)

- Canvas full-bleed on `surface.canvas` with a dot grid (`rgba(52,60,110,.12)`, 22 px, shown from
  50 % zoom).
- **Three capsules** along the top, each a glass pill 48 px high:
  1. **Identity** (start): wordmark · plugin name in `type.plugin` · version and draft state in
     `type.data` · switcher caret.
  2. **Tools** (centre): undo · redo | zoom (mono) · device frame · locale · theme | send to device.
  3. **Publish** (end): save state with revision (`Saved r214`) · presence avatars (the lock
     holder ringed green, viewers overlapping) · **Publish**.
- **Left panel** (start): serif title and caption (screen · node count) · pill tabs (Layers, Insert,
  Screens, State) · search pill with `/` hint · layer tree with type tags and warning counts ·
  summary card (screens · links · ghost screens).
- **Right panel** (end): serif title with the widget type in mono · pill tabs (Properties, Actions,
  JSON, A11y) · section cards with serif headings and label/value rows (70 px label column).
- **Bottom drawer** (between the panels): pill tabs with counts and the live-device chip on one row;
  the tab's content (Problems as a table); a **metrics strip** along the bottom — validation, bundle
  size meter, frame-time sparkline, last sync. Collapses to a 44 px capsule.
- Panels collapse to a 44 px icon rail; `[` and `]` toggle them; Tab cycles focus between chrome
  groups. Panels mirror in RTL; the canvas does not (rule book §8.5–§8.6).
- **Overlays** (V1–V9): a full-screen solid white sheet with `radius.xl` top corners rising over a
  dimmed, blurred canvas; serif title with breadcrumb and close.

### U9.2 Page layout (`shell`, `page`)

Every other page is built from the same parts, so it inherits the look:

- `surface.app` background; the same three-capsule top bar (identity · area tabs as a pill track ·
  search, notifications, avatar).
- Serif page title with a caption line; content in white section cards (`radius.lg`); key figures
  as **metric strips** and **status pills** (U10); lists as tables with mono numbers.
- Forms max 720 px wide; tables and grids full width; detail pages two columns (main + 320 px side
  card) from 1280 px.
- Apps grid cards: `radius.lg`, thumbnail 16:10, serif app name, team, environment pills, health pill;
  hover lifts 2 px.
- Dashboards (Analytics, Releases, Experiments, Functions): metric strip on top, charts in section
  cards, tables below; charts use the tokens of U2 (Recharts via the token CSS variables).

### U9.3 Breakpoints

| Width | Behaviour |
|---|---|
| < 768 px | Pages usable read-mostly; canvas shows "use a larger screen to edit" |
| 768–1279 px | One panel open at a time on the canvas; pages single column |
| 1280–1919 px | Default layout |
| ≥ 1920 px | Wider panels allowed; grids add columns |

## U10. Information patterns

The compact, information-rich presentation is built from these components only.

| Pattern | Look | Used for |
|---|---|---|
| Status pill | `radius.full`, 22 px, mono 10.5 px, icon + word or number; success in green, warning on amber fill, neutral on white | Health under each screen, release health, test results |
| Count | 16 px round badge, mono; warning counts on `amber.50` | Tabs, layer rows, notifications |
| Type tag | Mono 9.5 px, `text.tertiary`, fixed 26 px column | Node types in the layer tree and search results |
| Metric strip | One row at the foot of a panel, drawer or page section: label in sans, value in mono | Validation, bundle size, frame time, sync age |
| Meter | 72 × 6 px rounded track, success fill, grows in on load | Budgets: bundle size, limits usage |
| Sparkline | 84 × 18 px, 1.5 px violet line, draws in on load | Frame time, request latency, adoption |
| Live chip | Pill on `green.50`, pulsing dot, device · state · latency (`Pixel 9 · live · 41 ms`) | Paired devices, live streams |
| Summary card | Three equal cells, mono value over a sans caption | Panel footers (screens · links · ghost) |
| Presence | Overlapping avatars; the lock holder ringed in success green | Locks and viewers (F1) |
| Binding chip | Mono on `violet.50` | Data bindings, PXL expressions |

Rules: every number carries its unit; every status has an icon and a word; no chart without a label
and a value in text.

## U11. Components

| Component | Look |
|---|---|
| Primary button | Pill, solid `accent.primary`, white label, `elevation.primary`; hover adds the halo ring |
| Secondary button | Pill, transparent, `text.secondary`; hover `surface.hover` |
| Icon button | 36 px round, transparent; hover `surface.hover` |
| Destructive button | Pill, solid `red.600`; only inside confirm dialogs |
| Tabs | Pill track on `surface.hover` with 3 px padding; the selected tab is a white pill with `elevation.1` |
| Input, select | `radius.sm`, white, inset hairline; hover `border.strong`; focus 1.5 px violet + halo |
| Search | Pill on white at 72 %, inset hairline, keyboard hint at the end |
| Toggle | Pill; on = violet track |
| Section card | `surface.card`, `radius.lg`, serif heading with a caption or pill at its end |
| Tree row | `radius.md`, 30 px; selected = white pill with `elevation.1` and violet text |
| Menu, popover | Glass at 88 %, `radius.xl` (20 px), `elevation.2`, serif header where it names something |
| Tooltip | Solid `navy.900` at 94 %, white caption, `radius.md`, 400 ms delay |
| Dialog | Solid white, `radius.xl`, `elevation.4`, serif title; widths 400 / 560 / 800 px |
| Toast | Glass, `radius.full` when one line, status icon + text, bottom end, 5 s (errors stay) |
| Table | Rows separated by `border.hairline`, 30 px compact; mono for numbers and paths; hover `surface.hover` |
| Empty state | 48 px duotone icon, serif heading, one sentence, one primary action |
| Skeleton | `surface.hover` blocks with a slow shimmer (off under reduced motion) |
| Command palette | Glass sheet 640 px, `radius.xl`, `elevation.4`, centred at 20 % height |

## U12. Canvas visuals

| Element | Look |
|---|---|
| Screen title | Above the frame: name in `type.screen`, a caption below ("Entry screen", "From Sign in", "Editing · 3 ways in"); the edited screen's caption in violet with a haloed dot |
| Device frame | White, the device's radius, `elevation.frame`; lifts 2 px on hover |
| Edited screen | `elevation.frameActive` (violet edge and 7 px halo) |
| Screen health | A row of status pills under each frame: validation, accessibility findings, size |
| Selection | 2 px `accent.primary` outline with a 5 px halo; 9 px round handles, white fill; size pill below in mono on violet |
| Smart guides, distances | 1 px violet lines and mono measurement pills |
| Arrow | 2 px `canvas.arrow`, rounded caps and joins, open chevron head |
| Arrow bundle | One arrow with a mono count badge at its midpoint (`STU-044`) |
| Bundle hovered / selected | The lanes draw in, in `canvas.arrowActive`; the trunk and badge fade; a glass popover lists each link with its trigger and event (`Send money · Button · onTap`) |
| Plugin box (app canvas) | White card `radius.lg`, `elevation.frame`, serif name, version in mono, status pills |
| Native node | Dashed 1.5 px `navy.400` outline, native icon, no fill (`STU-033`) |
| Ghost screen | 40 % opacity, dashed `navy.400` border, plugin › screen caption, link count (`STU-052`) |
| Ghost jump highlight | Violet ring pulse, about 2 s, once (`STU-053`) |
| Lock (someone else) | Amber lock pill on the plugin box and a banner on the canvas (F1.6) |
| Low zoom | Titles and pills hidden, lanes merged, frames drawn from cached snapshots (`STU-048`, `STU-120`) |

## U13. Motion

| Token | Value | Use |
|---|---|---|
| `motion.fast` | 140 ms | Hover, press, tab change |
| `motion.base` | 180 ms | Popovers, lanes, menus |
| `motion.enter` | 420 ms | Chrome entering |
| `motion.bundle` | 220 ms | Arrow bundle expansion (`STU-044`: 150–200 ms for the fan-out; the draw-in finishes within 220 ms) |
| `ease.standard` | `cubic-bezier(0.2, 0, 0, 1)` | Everything |

| Micro-animation | Behaviour |
|---|---|
| Chrome enter | Capsules, then panels, then drawer: rise 10 px, scale from 98.5 %, fade; 40 ms apart |
| Press | Buttons and tabs scale to 96 % |
| Save | "Saving…" cross-fades to "Saved r214"; the cloud icon turns green |
| Selection | Outline and halo settle from 105 % |
| Frame hover | Lifts 2 px |
| Bundle | Lanes draw in along their path; popover rises 6 px |
| Meter, sparkline | Fill grows / line draws once on load |
| Live | The live-device dot pulses every 2.4 s — the only looping animation |
| Primary hover | A 5 px halo ring appears |

Never animate layout size on the canvas. `prefers-reduced-motion`: every animation becomes an
instant change.

## U14. Dark theme

Same structure and proportions: navy-black surfaces (`#0B0D17` canvas and app, `#141726` raised),
dark glass (U3), lighter violet `#8B74FF`, status colours from U2.3. Shadows stay subtle; edges come
from the 1 px outer border.

## U15. High-contrast theme

No glass and no transparency; solid surfaces; text ≥ 7:1; 2 px borders on inputs and cards; focus
ring 3 px; arrows 2 px `navy.900`. Used for the shell when chosen and for the canvas preview
(`STU-112`).

## U16. Brand in Studio

- Wordmark (Appendix J) at the start of the identity capsule, `plu` in ink with the violet `x`; dark
  theme uses the dark lockup.
- App icon as favicon and on the sign-in page; sign-in pages are porcelain with one centred white
  card and a serif title.

## U17. Tailwind mapping

The `@theme` block is generated from these tokens (rule book §4.2): colours (`bg-canvas`,
`bg-raised`, `bg-card`, `text-primary`, `text-secondary`, `text-secondary-glass`, `border-hairline`,
`bg-accent`, `ring-focus`, status utilities); radii `rounded-xs…xl/full`; shadows `shadow-e1`,
`shadow-e2`, `shadow-frame`, `shadow-primary`, `shadow-e4`; the spacing scale of U6; type utilities
`text-screen`, `text-title`, `text-section`, `text-body`, `text-data`, `text-tag`; motion
`duration-fast/base/enter`, `ease-standard`. Glass is one utility, `glass`, which applies U3 and
switches to the solid fallback automatically.

## U18. How the design is kept

The design reaches the code through parts an agent cannot route around, not through images:

1. **Tokens.** Every value in this file is a token in `@plux/brand`; Tailwind's theme and Plux Canvas
   are generated from it. Raw values are rejected by lint (rule book §4.2).
2. **Components.** Every element of U9–U12 is a shared component in `studio/packages/ui` with a
   `CATALOGUE.md` entry and `*.cases.tsx` (rule book §5.2). Screens are assembled from them; a
   second component doing the same job is rejected in review (rule book §5.1).
3. **Reference page.** [studio-ui-reference.html](studio-ui-reference.html) shows the approved
   result; a new pattern is added to it, as markup, before it is built.
4. **Agent skill.** Every Studio session loads the design skill
   ([p11-studio-design-skill.md](../plans/p11-studio-design-skill.md)), which routes the agent to the tokens,
   the catalogue and this file, and ends with its checklist.
5. **Lint.** Raw colours, sizes and shadows, icons from any set but Phosphor, and font families not
   from the tokens fail `make studio-check` (with the first component).
6. **Design gate.** At each P11 milestone exit gate (functional decisions F23), the automated
   AI-pattern detector runs over the catalogue page and the milestone's routes (proposed: Impeccable's
   deterministic rules, Apache-2.0, run without a language model; allowlist and ADR first), and the
   maintainer reviews the milestone's screens locally.
7. **Change here first.** A deviation from this file is proposed as a change to it (and to the
   reference page) before the code changes (`AGENTS.md` §3).

## U19. Relationship to earlier text

- Replaces the 2026-10-20 version of this file and rule book §4.3's description (decisions file,
  Part B); the decisions file is updated to point here.
- Studio follows the "floating" wording of `STU-060`–`STU-062` and Figure 21-2 **as written**: no
  specification change is needed for them. The radius wording of `STU-010` and the use column of
  Appendix J (arrows navy at rest and violet when hovered or selected; text, canvas and status
  colours from the derived scales) are in spec 1.10.0 (O-12, O-13).
- Fonts change from Noto Sans to Instrument Sans, Instrument Serif and DM Mono (decisions A3, A5.4,
  rule book §4.6); icons from Lucide to Phosphor (A3, A4.14, A4.20). Both need allowlist entries,
  the Studio stack ADR (ADR-0014) and the legal review of decisions A1.2.
- New primitive tokens (porcelain, navy steps and alphas, violet steps and halo, green, amber, red)
  are added to `@plux/brand`.

# P11 Studio — UI Language (Style, Look and Feel)

- **Status:** Written at the maintainer's request on 2026-10-20 from the maintainer's direction:
  a very whitish system, rounded everywhere, floating panels over the canvas with a very light
  frosted blur, Deep Navy for structure and Electric Violet as the rare accent. Intended home:
  `docs/engineering/studio-ui-language.md` (the file the rule book §4.3 links to).
- **Read with:** [p11-studio-decisions.md](p11-studio-decisions.md) (stack, rule book Part B),
  [p11-studio-functional-decisions.md](p11-studio-functional-decisions.md),
  [p11-studio-routing.md](p11-studio-routing.md).
- **Binding rules kept:** tokens only, no raw values (rule book §4.1–§4.2); computed contrast
  (§4.5); logical properties and RTL (§8); motion tokens with reduced motion (§4.7); brand tokens of
  Appendix J (`STU-016`); WCAG 2.2 AA (`STU-011`); 60 fps canvas (`STU-120`).
- *Measured* contrast ratios below were computed with the WCAG 2.2 formula used by
  `studio/packages/brand/src/contrast.ts`.

---

## U1. Principles

1. **White first.** Studio is a bright, quiet workspace; content and the user's app are the colour.
2. **The canvas is the room; chrome floats in it.** Panels, the action bar and the drawer float over
   the canvas on frosted white glass; the canvas runs underneath them, edge to edge.
3. **Soft and round.** Generous radii, soft shadows, no hard boxes.
4. **Colour is meaning.** Navy structures and reads; violet acts and selects; mint, amber and red
   report status. Nothing is coloured for decoration.
5. **Calm motion.** Short, purposeful transitions; nothing moves without a reason.
6. **Performance decides.** Every effect has a cheaper fallback that switches in before frame rate
   suffers.

## U2. Colour

### U2.1 Proportions

| Share | Role | Colours |
|---|---|---|
| ~75 % | Dominant surfaces | White `#FFFFFF`, alabaster `#F7F8FB` (Appendix J `brand.surface`) |
| ~18 % | Structure and text | Deep Navy family from `brand.ink` `#0F1222` |
| ~7 % | Accent | Electric Violet `brand.violet` `#5B3DF5` |
| as needed | Status | Mint (success), amber (warning), red (danger) |

### U2.2 Palette — light theme

Brand tokens are Appendix J's; the rest are new primitive tokens of `@plux/brand`.

| Primitive token | Hex | Use |
|---|---|---|
| `brand.ink` | `#0F1222` | Primary text, wordmark |
| `navy.800` | `#1E2A4A` | Strong structure: top bar text, headings in navigation |
| `navy.600` | `#475069` | Secondary text, icons |
| `navy.500` | `#6B7390` | Tertiary text on solid white only |
| `navy.400` | `#8B95AD` | Arrows, disabled icons, strong dividers |
| `navy.300` | `#C9CEDB` | Input borders |
| `navy.200` | `#E4E7EF` | Hairlines, dividers |
| `navy.100` | `#EEF0F6` | Hover fill, table row hover |
| `brand.surface` | `#F7F8FB` | App background, canvas background |
| `white` | `#FFFFFF` | Panels, cards, dialogs, inputs |
| `brand.violet` | `#5B3DF5` | Primary action, selection, focus ring |
| `violet.700` | `#4A2DE6` | Primary action pressed / hover |
| `brand.violet.subtle` | `#EEEAFE` | Selected rows, selected tab, lane labels |
| `brand.mint` | `#12B886` | Success fills and icons |
| `mint.700` | `#0B7A5A` | Success text |
| `brand.amber` | `#F59F00` | Warning fills and icons |
| `amber.700` | `#9A5B00` | Warning text |
| `red.600` | `#D92D20` | Danger fills, destructive buttons |
| `red.700` | `#B42318` | Danger text |
| `rose.500` | `#F0457A` | Canvas guides and distance measurement only |

### U2.3 Semantic tokens (what components use)

| Semantic token | Light | Dark |
|---|---|---|
| `surface.app` | `brand.surface` | `#0B0D17` |
| `surface.canvas` | `brand.surface` | `#0B0D17` |
| `surface.raised` (cards, dialogs, inputs) | `white` | `#141726` |
| `surface.glass` | white at 82 % + blur (U3) | `#141726` at 88 % + blur |
| `surface.hover` | `navy.100` | `#1C2033` |
| `surface.selected` | `brand.violet.subtle` | `#2A2250` |
| `text.primary` | `brand.ink` | `#F5F6FA` |
| `text.secondary` | `navy.600` | `#B4BAD0` |
| `text.tertiary` (solid surfaces only) | `navy.500` | `#A3AAC2` |
| `text.onAccent` | `white` | `white` |
| `border.hairline` | `navy.200` | `#262B40` |
| `border.input` | `navy.300` | `#3A4060` |
| `accent.primary` | `brand.violet` | `#8B74FF` |
| `accent.primaryPressed` | `violet.700` | `#7257FF` |
| `accent.selection` | `brand.violet` | `#8B74FF` |
| `focus.ring` | `brand.violet` | `#8B74FF` |
| `status.success` / `.text` | `brand.mint` / `mint.700` | `#38D9A9` / `#38D9A9` |
| `status.warning` / `.text` | `brand.amber` / `amber.700` | `#FFC53D` / `#FFC53D` |
| `status.danger` / `.text` | `red.600` / `red.700` | `#F97066` / `#F97066` |
| `canvas.arrow` | `navy.400` | `#6B7390` |
| `canvas.arrowActive` | `brand.violet` | `#8B74FF` |
| `canvas.guide` | `rose.500` | `#F76A93` |

### U2.4 Where violet is allowed (the 7 %)

Primary buttons (Publish, Apply, Create, Save), selection on the canvas and in lists, focus rings,
the active state of tabs, toggles and segmented controls, the hovered or selected arrow bundle,
active experiment states. Never for large fills, headings, body text on glass, or decoration.
(AI actions will use it from P12.)

### U2.5 Measured contrast (light)

| Pair | Ratio | Allowed for |
|---|---|---|
| `brand.ink` on white | 18.59 | all text |
| `brand.ink` on `brand.surface` | 17.50 | all text |
| `navy.800` on white | 14.14 | all text |
| `navy.600` on white | 8.02 | all text |
| `navy.500` on white | 4.69 | text on solid surfaces only |
| `navy.400` on white | 3.00 | icons, arrows, UI parts (3:1) — not text |
| white on `brand.violet` | 6.12 | button labels |
| `brand.violet` on `brand.violet.subtle` | 5.20 | selected labels |
| `mint.700` / `amber.700` / `red.700` on white | 5.32 / 5.43 / 6.57 | status text |
| `brand.mint` / `brand.amber` on white | 2.55 / 2.13 | fills and icons with a label only — never alone |
| Dark: white / `#B4BAD0` on `#141726`; `#8B74FF` on `#0B0D17` | 17.79 / 9.21; 5.57 | as light (other dark pairs, e.g. `#F97066`, are checked by the token contrast test) |

Status is never shown by colour alone: always an icon and a word.

## U3. Glass — the frosted floating surface

| Property | Light | Dark |
|---|---|---|
| Fill | `rgba(255,255,255,0.82)` | `rgba(20,23,38,0.88)` |
| Backdrop | `blur(24px) saturate(160%)` | `blur(24px) saturate(140%)` |
| Inner edge | 1 px `rgba(255,255,255,0.70)` inset | 1 px `rgba(255,255,255,0.06)` inset |
| Outer edge | 1 px `rgba(15,18,34,0.06)` | 1 px `rgba(0,0,0,0.40)` |
| Shadow | `elevation.2` (U5) | `elevation.2` |

- **Why these tints:** text must pass on the worst content underneath. *Measured*, light glass at
  82 % over pure black: `text.primary` 12.18, `text.secondary` 5.26 (both pass); dark glass at 88 %
  over pure white: `text.primary` 11.63, `text.secondary` 6.51. Violet text on glass would drop to
  4.01, so **no violet text on glass** (violet buttons are solid and fine).
- **Used on:** floating action bar, left and right panels, bottom drawer, menus, popovers, the
  command palette, toasts, the top bar of `page` layouts, sticky table headers.
- **Never on:** canvas items, cards inside pages, inputs, dialogs (dialogs are solid white over a
  blurred scrim), anything stacked on another glass surface (no glass on glass).
- **Dialog scrim:** `rgba(15,18,34,0.24)` + `blur(8px)`.
- **Performance rules (the maintainer's "only without negative performance impact"):**
  1. Blur is applied only to the few floating chrome elements; their total area is bounded by
     layout (panels ≤ 480 px wide, drawer ≤ 50 % height).
  2. Blur radius never exceeds 24 px; no animated blur radius.
  3. **Automatic solid fallback** (`surface.raised` at 96 %, no backdrop filter) when any of:
     `prefers-reduced-transparency`; the high-contrast theme; the main-thread canvas fallback
     (A5.1); the canvas worker reports frames over budget during pan or zoom (switches for the rest
     of the session, user-visible in profile settings); a user setting "Reduce transparency".
  4. The S0 prototype measures `STU-120` with glass on and off on the reference laptop; if glass
     costs frames there, the solid fallback becomes the default and glass the option.

## U4. Shape

| Token | Radius | Use |
|---|---|---|
| `radius.xs` | 6 px | Checkboxes, small chips, tags |
| `radius.sm` | 10 px | Buttons, inputs, selects, segmented controls |
| `radius.md` | 12 px | Menus, popovers, tooltips, toasts, small cards |
| `radius.lg` | 16 px | Panels, dialogs, app cards, plugin boxes, drawer |
| `radius.xl` | 20 px | Floating action bar, large overlay shells |
| `radius.full` | 9999 px | Pills, badges, avatars, count badges, toggles |

Device frames on the canvas use their device's real corner radius. Nested radii follow
`inner = outer − padding` so curves stay concentric.

## U5. Elevation

| Token | Light | Use |
|---|---|---|
| `elevation.0` | none | Flat content |
| `elevation.1` | `0 1px 2px rgba(15,18,34,0.06), 0 1px 1px rgba(15,18,34,0.04)` | Cards, inputs on hover |
| `elevation.2` | `0 4px 16px rgba(15,18,34,0.08), 0 1px 3px rgba(15,18,34,0.06)` | Floating panels, plugin boxes, device frames |
| `elevation.3` | `0 12px 32px rgba(15,18,34,0.12), 0 2px 6px rgba(15,18,34,0.06)` | Menus, popovers, dragged items |
| `elevation.4` | `0 24px 64px rgba(15,18,34,0.18), 0 4px 12px rgba(15,18,34,0.08)` | Dialogs, overlays, command palette |

Dark theme: same geometry, black at double opacity, plus the 1 px outer edge.

## U6. Space and density

- Base unit 4 px; scale `space.0.5 … space.16` = 2, 4, 8, 12, 16, 20, 24, 32, 40, 48, 64 px.
- **Compact** (default) and **comfortable** density as token sets (rule book §4.4): control heights
  28 / 32 px, row heights 28 / 36 px, panel padding 12 / 16 px.
- Floating chrome sits **12 px** from the viewport edge and from each other; content inside panels
  uses 12 px (compact) / 16 px (comfortable) padding.

## U7. Typography

| Token | Size / line | Weight | Use |
|---|---|---|---|
| `type.display` | 32 / 40 | 600 | Empty-state and onboarding titles |
| `type.title` | 24 / 32 | 600 | Page titles |
| `type.heading` | 18 / 26 | 600 | Section headings, dialog titles |
| `type.subheading` | 15 / 22 | 600 | Panel group titles |
| `type.body` | 14 / 20 | 400 | Default text |
| `type.bodyStrong` | 14 / 20 | 500 | Labels, emphasis |
| `type.small` | 13 / 18 | 400 | Panel fields, table cells (compact) |
| `type.caption` | 12 / 16 | 500 | Badges, metadata, canvas labels |
| `type.micro` | 11 / 14 | 600 | Count badges, keyboard hints (uppercase not used) |
| `type.code` | 13 / 20 | 400 | JSON, PXL, Rust, IDs |

- UI font: **Noto Sans** + **Noto Sans Arabic** (A5.4); code: **Noto Sans Mono** (OFL-1.1; to be
  added to the font list and its licence reviewed); wordmark: **Sora SemiBold** (Appendix J).
- Tabular numerals in tables, metrics and coordinates. No letter-spacing changes except the
  wordmark (−2 %).
- Arabic uses the same sizes; line heights +2 px for `type.body` and below.

## U8. Layout

### U8.1 Canvas layout (`canvas`, C1/C2)

- Canvas full-bleed on `surface.canvas` with a dot grid (`navy.400` at 12 % opacity, shown from
  50 % zoom).
- **Floating action bar:** top centre, glass, `radius.xl`, height 44 px, icon buttons with tooltips;
  Publish at its end as the one solid violet button.
- **Left panel** (start side): glass, `radius.lg`, default 280 px (240–400), top 68 px to bottom 12 px
  above the drawer.
- **Right panel** (end side): glass, `radius.lg`, default 320 px (280–480).
- **Bottom drawer:** glass, `radius.lg`, between the panels, collapsible to a 36 px tab strip,
  resizable up to 50 % height.
- Panels collapse to a 44 px icon rail; `[` and `]` toggle them; Tab cycles focus between chrome
  groups. Panels mirror in RTL; the canvas does not (rule book §8.5–§8.6).
- **Overlays** (V1–V9): a full-screen solid white sheet with `radius.xl` top corners sliding up over a
  dimmed, blurred canvas; title bar with breadcrumb and close.

### U8.2 Page layout (`shell`, `page`)

- `surface.app` background; glass top bar (56 px): wordmark, organisation switcher, app switcher (in
  apps), area tabs, search (⌘K), notifications, help, avatar.
- Content on white cards (`radius.lg`, `elevation.1`), 24 px gutters; forms max 720 px wide;
  tables and grids full width; detail pages two columns (main + 320 px side card) from 1280 px.
- Apps grid cards: `radius.lg`, image 16:10 (canvas thumbnail, uploaded image or monogram on an
  app-derived pastel), name, team, environment badges, health dot; hover lifts to `elevation.2`.

### U8.3 Breakpoints

| Width | Behaviour |
|---|---|
| < 768 px | Pages usable read-mostly; canvas shows "use a larger screen to edit" |
| 768–1279 px | One panel open at a time on the canvas; pages single column |
| 1280–1919 px | Default layout |
| ≥ 1920 px | Wider panels allowed; grids add columns |

## U9. Components

| Component | Look |
|---|---|
| Primary button | Solid `accent.primary`, white label, `radius.sm`, no shadow; pressed `accent.primaryPressed` |
| Secondary button | White, 1 px `border.input`, `text.primary` |
| Ghost / icon button | Transparent, `surface.hover` on hover, `navy.600` icons |
| Destructive button | Solid `red.600`, white label; used only inside confirm dialogs |
| Input, select | White, 1 px `border.input`, `radius.sm`; focus: 2 px `focus.ring` + 2 px white offset |
| Toggle | `radius.full`; on = violet track |
| Segmented control, tabs | Track `navy.100`, active segment white with `elevation.1`; panel tabs: active label `text.primary` with 2 px violet underline |
| Menu, popover | Glass, `radius.md`, `elevation.3`, 4 px inner padding, items 28/32 px |
| Tooltip | Solid `brand.ink` at 94 %, white `type.caption`, `radius.md`, 400 ms delay |
| Dialog | Solid white, `radius.lg`, `elevation.4`, scrim (U3); widths small 400, medium 560, large 800 px |
| Toast | Glass, `radius.md`, status icon + text, bottom end, 5 s (errors stay) |
| Table | White card; header sticky glass; rows separated by `border.hairline`; hover `surface.hover`; selected `surface.selected` |
| Badge | `radius.full`, `type.caption`; status badges use tinted fill (status at 12 %) + status text colour |
| Count badge (arrows) | White pill, `elevation.1`, `type.micro` |
| Empty state | Line icon 48 px `navy.400`, `type.heading`, one sentence, one primary action |
| Skeleton | `navy.100` blocks with a slow shimmer (off under reduced motion) |
| Command palette | Glass sheet 640 px, `radius.lg`, `elevation.4`, centred at 20 % height |

## U10. Canvas visuals

| Element | Look |
|---|---|
| Device frame | White screen, device radius, `elevation.2`, frame name above in `type.caption` `navy.600` |
| Overflow region | White at 50 % with 1 px dashed `navy.300` border, below or beside the frame (`STU-056`) |
| Selection | 1.5 px `accent.selection` outline; 8 px square handles, white fill, violet border |
| Hover | 1 px `accent.selection` at 50 % |
| Multi-select bounds | Dashed violet 1 px |
| Smart guides, distances | `canvas.guide` (rose) lines and measurement pills |
| Plugin box | White card `radius.lg`, `elevation.2`, header with icon, name, version; badges (unpublished, locked, errors, kill-switched) as status badges |
| Native node | Dashed 1.5 px `navy.400` outline, native icon, no fill (`STU-033`) |
| Ghost screen | 40 % opacity, dashed `navy.400` border, plugin › screen label, link count badge (`STU-052`) |
| Frame (group of plugins) | `navy.100` at 60 % fill, `radius.lg`, label top start |
| Arrow bundle | 1.5 px `canvas.arrow` cubic Bézier, rounded caps, count badge at midpoint |
| Bundle hovered / selected | Lanes in `canvas.arrowActive`, labels on `brand.violet.subtle` pills, other bundles dimmed to 30 % (`STU-044`) |
| Low zoom | Labels hidden, lanes merged, boxes drawn from cached snapshots (`STU-048`, `STU-120`) |
| Ghost jump highlight | Violet ring pulse, about 2 s (`STU-053`) |
| Lock (someone else) | Amber lock badge on the plugin box and banner on the canvas (F1.6) |
| Problems on canvas | Small status dot (danger/warning) at the node's top end; click opens Problems |

## U11. Motion

| Token | Value | Use |
|---|---|---|
| `motion.fast` | 120 ms | Hover, press, toggles |
| `motion.base` | 180 ms | Panels, menus, tabs, toasts |
| `motion.slow` | 240 ms | Dialogs, overlays |
| `motion.bundle` | 160 ms | Arrow bundle expansion (`STU-044`: 150–200 ms) |
| `ease.standard` | `cubic-bezier(0.2, 0, 0, 1)` | Most transitions |
| `ease.exit` | `cubic-bezier(0.4, 0, 1, 1)` | Leaving elements |

Panels slide 8 px + fade; overlays rise 16 px + fade; never animate layout size on the canvas.
`prefers-reduced-motion`: all become instant except a 1-frame opacity change.

## U12. Icons and imagery

- Lucide, 1.5 px stroke, 16 px in dense UI and panels, 20 px in toolbars, 24 px in empty states;
  colour `navy.600`, active `accent.primary`; RTL mirror list in the catalogue (rule book §8.3).
- App monograms: first character on a pastel derived from the app ID (lightness 92 %, chroma low),
  letter in `navy.800`.
- No stock photos or decorative illustrations; empty states use line icons.

## U13. Dark theme

Same structure and proportions: navy-black surfaces (`#0B0D17` app, `#141726` raised), dark glass
(U3), lighter violet `#8B74FF` for accents, status colours from Appendix J's dark column. Shadows
stay subtle; edges come from the 1 px outer border.

## U14. High-contrast theme

No glass and no transparency; solid surfaces; text ≥ 7:1; 2 px borders on inputs and cards; focus
ring 3 px; arrows 2 px `brand.ink`. Used for the shell when chosen and for the canvas preview
(`STU-112`).

## U15. Brand in Studio

- Wordmark (Appendix J) at the start of the top bar, 20 px high, Ink with the violet x; dark theme
  uses the dark lockup.
- App icon as favicon and on the sign-in page; sign-in pages are white with a single centred card.
- Clear space and minimum size as Appendix J.

## U16. Tailwind mapping

The `@theme` block is generated from these tokens (rule book §4.2): colours `bg-surface-app`,
`bg-surface-raised`, `bg-glass`, `text-primary`, `text-secondary`, `border-hairline`,
`bg-accent-primary`, `ring-focus`, status utilities; radii `rounded-xs…xl/full`; shadows
`shadow-e1…e4`; spacing scale U6; type utilities `text-title`, `text-body`, … ; motion
`duration-fast/base/slow`, `ease-standard/exit`. Glass is one utility, `glass`, which applies U3 and
switches to the solid fallback automatically.

## U17. Relationship to earlier text

- This file replaces the rule book's §4.3 description (decisions file, Part B): it was compact and
  docked with 4–6 px radii, hairlines instead of shadows and a light-blue accent; Studio is now
  whitish, rounded (radii 6–20 px), floating glass panels by default, violet accent.
- As a result, Studio follows `STU-010` and the "floating" wording of `STU-060`–`STU-062` and
  Figure 21-2 **as written**: the spec changes for those listed in the decisions file (C1 #15, C3)
  are no longer needed.
- Appendix J lists arrows among `brand.violet`'s uses; here arrows are navy at rest and violet when
  hovered or selected (keeps violet near 7 %). Appendix J's use column needs that wording change.
- New primitive tokens (navy, violet.700, mint.700, amber.700, red, rose) are added to
  `@plux/brand`; Noto Sans Mono joins the font list (legal review, decisions A1.2).
- The decisions file still carries the old §4.3 text until it is updated to point here.

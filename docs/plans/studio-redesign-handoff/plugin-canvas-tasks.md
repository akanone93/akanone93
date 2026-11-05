# Plugin canvas — research and feature tasks

The plugin canvas is where plugin content is built: the most important surface of Studio. **What matters most is how
each screen looks.** The goal: build one plugin, **All-in-one**, from scratch by dragging and dropping, with every
widget, component and template and everything the canvas offers; publish it; check it on a device.

Requirement IDs are from `docs/requirements.md`. Paths are relative to the repository root.

## 1. How we work

1. The maintainer names the next tasks; the agent implements them, one patch per task (`pc-NN.patch`, each on top of
   the previous one), pushed to the working branch first, with the `git apply` commands.
2. The maintainer applies them in order, follows each task's **Check**, and sends the feedback of all of them at once.
3. The agent fixes everything and hands over new patches, until approved.

**Done means:** the checks pass on `make dev`; unit tests (and an e2e test for a user flow); `make studio-lint
studio-typecheck studio-cover studio-build studio-size trace spec-lint reuse-lint` pass; strings in English, German and
Arabic; light, dark, RTL and 200 % text work; the docs that describe it change in the same patch.

**Decisions (maintainer, 2026-11-04):** `STU-005` and `STU-068` are `WIP` until PC-01 and PC-15/PC-30 land. The
Material Symbols (Apache-2.0) and Cupertino icons (MIT) fonts are bundled for the canvas (PC-03). Painting is reviewed by
eye against the device; no pixel gate (ADR-0013, amended).

## 2. What the research found

**Works:** layout of all 149 Layer 1 and 58 Layer 2 widgets, conformance-tested (`STU-005`); screens on one canvas,
arrows, ghost screens, native nodes (`STU-040`–`STU-056`); device presets and matrix, preview switches (`STU-057`,
`STU-058`); insert, drag, reparent, wrap, copy and paste (`STU-067`); Layers, inspector, the nine editors, console,
history (`STU-061`–`STU-069`); safety rules, components and templates (`STU-070`–`STU-086`); send to device, publish.

**Gaps** (each closed by a task): screens drawn as grey boxes (PC-01–PC-10); theme colours unused (PC-01); no text
styles, icons or images (PC-02–PC-04); the app bar is a stand-in (PC-12); dialogs and sheets never placed (PC-13); no
page settings (PC-14); nothing scrolls or switches (PC-15); no inline editing or handles (PC-16–PC-20); complex
properties have no editor (PC-21, PC-22); no editors for lists, forms, conditions or motion (PC-23–PC-30); quality and
problems not shown on the screens (PC-31–PC-34).

## 3. The tasks

Each: **See** (what changes on screen) · **Build** (what to add to All-in-one) · **Check** (the steps) · Spec ·
Needs.

### A. The look

- **PC-01 Paint and theme colours.** *See:* boxes in their colours, gradients, borders, corners, shadows; screens in
  the theme's Material colour scheme, recoloured by **Dark** and **Brand**. *Build:* a screen with a Container, a
  DecoratedBox, a Card and a ColoredBox. *Check:* set a fill, a gradient, corners and a shadow in the decoration
  editor; pick colours from the theme; switch Dark. `STU-005`, `THM-001`–`THM-003`.
- **PC-02 Text and true layout.** *See:* the type scale (display to label), the theme's typography tokens, sizes,
  weights, colours, alignment, ellipsis, rich text; screens laid out edge to edge as on the device (columns, rows,
  padding, containers, expanded). *Build:* a typography screen. *Check:* set a style preset; long text ellipsises at
  max lines; centre a text in a stretched column. `STU-005`, `STU-006`, `THM-004`.
- **PC-03 Icons.** *See:* Material Symbols and Cupertino icons, in their size and colour, in the colour of the
  component around them; an icon picker (buttons and bars draw their icons with PC-05 and PC-07). *Build:* a row of
  icons. *Check:* pick an icon by search; switch to Cupertino; change its size and colour. `THM-005`.
- **PC-04 Images.** *See:* asset images in every fit, with rounded corners; avatars with an image, initials or an
  icon; a web image's placeholder; an image picker. *Build:* drag an uploaded image onto a screen. *Check:* change the
  fit; pick another image; a web image shows its placeholder. `AST-002`.
- **PC-05 Buttons and controls.** *See:* Material 3 buttons, icon buttons, FABs, chips, switches, checkboxes, radios,
  sliders, progress, badges, in their states; labels in the component's text style and colour. *Build:* an actions
  screen. *Check:* a button without an action is disabled, as on the device — add one and it is enabled; set a
  checkbox, switch, radio or filter chip's value and it shows selected. `STU-005`.
- **PC-06 Fields and pickers.** *See:* text fields in every state (underline, filled, outlined; resting and floating
  label; hint, prefix, suffix, icons; helper and error), search bars. *Build:* a form screen. *Check:* set an error
  text and the field turns red; give it a value and the label floats. Steppers, date pickers and labelled dropdown
  menus follow with PC-09. `STU-005`, `STA-020`.
- **PC-07 Bars, surfaces and lists.** *See:* the page's own app bar (title, leading, actions), navigation bar and
  rail with the selected destination, tab bar with its indicator, drawers, bottom app bars, cards, list tiles, dividers.
  *Build:* a settings screen. *Check:* change the selected destination or tab; compare with the Dev app. Bars stay
  where the page puts them until PC-12 places the scaffold's parts. `STU-005`.
- **PC-08 Cupertino.** *See:* Cupertino widgets in iOS's colours (buttons, switches, sliders, text fields, activity
  indicators, navigation and tab bars, segmented controls); with **Cupertino** as the platform style in Preview,
  adaptive Material controls drawn the iOS way. *Build:* a Cupertino screen. *Check:* switch the platform style
  between Material and Cupertino; switch Dark. `WGT-011`.
- **PC-09 Layer 2 everyday components.** *See:* their content (money, card numbers, totals, list items, state texts)
  on their surfaces: gradient account and payment cards, product and ETA cards, promo banners, skeletons, underlined
  inputs. *Build:* a components screen. *Check:* compare with the Dev app; change an account card's name. `WGT-020`.
- **PC-10 Charts, media, maps, scanners, native slots.** *See:* charts from mock data, galleries, media posters, map
  and scanner previews, native slot screenshots. *Build:* a dashboard screen. *Check:* change the chart's mock data.
  `WGT-021`, `WGT-031`.

### B. Screens as device screens

- **PC-11 Device chrome.** Status bar, notch or island, home indicator, safe areas; the screen's name, route and kind
  above it. `STU-057`.
- **PC-12 Scaffold in place.** The page's own app bar, navigation bar, FAB, drawer; empty slots offer **Add**.
- **PC-13 Dialogs and sheets.** Dialog, bottom sheet and full-screen dialog pages over a dimmed screen. `SCH-022`.
- **PC-14 Page settings.** Route, kind, title, parameters with mocks, page state, data, transition, guards.
  `SCH-022`–`SCH-025`.
- **PC-15 Live surface.** Scroll lists, switch tabs and pages, open the drawer, in **Interact** mode. `STU-068`.

### C. Direct manipulation

- **PC-16 Feedback.** Hover outlines, names and sizes, padding and gaps, problem dots. `STU-067`.
- **PC-17 Inline text.** Double-click to edit a text in place; it becomes a translation key. `I18N-007`.
- **PC-18 Handles.** Resize, padding and gap handles snapping to tokens; nudging. `STU-067`.
- **PC-19 Context toolbar.** Wrap, convert, add sibling, align, duplicate, add event. `STU-067`.
- **PC-20 Drag and drop everywhere.** Into slots, stacks, other screens; assets as images. `STU-067`.

### D. Styling panel

- **PC-21 Style editors.** State colours, shapes, style copy and paste, token-first values. `STU-062`.
- **PC-22 Layout editors.** Constraints, flex, stack, wrap, size-class overrides, reset. `WGT-010`.

### E. Content, data and logic

- **PC-23 State and bindings.** Binding badges, values from mocks, mock state per source. `STA-001`, `DAT-080`.
- **PC-24 Lists from data.** Item templates, empty, loading, error and end states. `WGT-012`, `DAT-011`.
- **PC-25 Conditions.** `visible`, `If`, `Match`, `ForEach` branches on the canvas. `SCH-023`.
- **PC-26 Forms.** Fields, validators, error texts, form state preview. `STA-020`.
- **PC-27 Events and actions.** Event badges, quick actions, snackbars and dialogs. `ACT-001`–`ACT-003`.
- **PC-28 Navigation.** Typed parameters on arrows, transitions, arrange by flow. `NAV-001`, `NAV-010`.

### F. Motion

- **PC-29 Animations and transitions.** Implicit animations, enter and exit, hero, play preview. `ANI-001`–`ANI-004`.
- **PC-30 Timelines.** Keyframes, scrubbing, Lottie and Rive, scroll-linked effects. `ANI-002`, `ANI-005`, `ANI-006`.

### G. Quality

- **PC-31 Accessibility.** Contrast and touch-target overlays, focus order, semantics editor. `A11Y-001`–`A11Y-006`.
- **PC-32 Localisation.** Missing and overflowing text marked, plural samples, per-locale overrides. `I18N-*`.
- **PC-33 Responsive.** Size-class marks, adaptive navigation, foldables. `WGT-010`.
- **PC-34 Problems and budgets.** Problems pinned to nodes, cost per screen. `SCH-040`, `ANI-008`.

### H. Theme, assets, reuse

- **PC-35 Theme round trip.** Theme edits shown live, where-used, raw to token. `THM-001`.
- **PC-36 Assets.** Upload, preview, replace, usage, limits. `AST-001`, `AST-003`.
- **PC-37 Components and templates.** Isolation editing, instance badges, overrides. `STU-080`–`STU-086`.

### I. Ship

- **PC-38 Device round trip.** Send, highlight on device, compare canvas and device. `DEV-020`, `DEV-033`.
- **PC-39 All-in-one, published.** Every screen, sheet and dialog, navigation, forms, data, motion, three locales,
  light and dark, published and checked on the Dev app.

## 4. All-in-one

A plugin the maintainer builds from scratch with the canvas, one part per task: **Paint** (containers, cards,
decorations), **Type**, **Icons**, **Images**, **Actions**, **Form**, **Settings** (bars and lists), **Components**,
**Dashboard** (charts and media), then the screens, sheets, dialogs, flows, data and motion of the later tasks. Every
widget of the Insert pane and every template of the library appears in it at least once. PC-39 publishes it.

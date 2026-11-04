# Plugin canvas — research and feature tasks (2026-11-04)

The plugin canvas is where plugin content is built, so it is the most important surface of Studio. The maintainer
asked for deep research into everything it must do, and for a list of feature-based tasks, each one big enough for
one iteration and small enough to check when it is handed over. **What matters most is how each screen looks.** The
goal is to build one real plugin with every feature of the canvas, publish it, and check it on a device.

Requirement IDs are from `docs/requirements.md` (spec 1.14.0). Paths are relative to the repository root.

## 1. How we work through this list

1. The maintainer names the next task (normally the next one in order).
2. The agent implements it and hands over **one patch** (`pc-<NN>.patch`, on top of the previous one) with the
   `git apply` command, a short report, screenshots in light and dark, and the **check steps** of the task.
3. The maintainer applies it, runs `make dev`, follows the check steps, and gives feedback.
4. The agent fixes and hands over a new patch until the maintainer approves; then the next task starts.

**Every task is done when:** its check steps pass on `make dev`; its behaviour has unit tests (and an e2e test where
it is a user flow); `make studio-lint studio-typecheck studio-cover studio-build studio-size studio-e2e trace
spec-lint reuse-lint` pass (old failures named in the report); new strings are in English, German and Arabic; it
works in light, dark, RTL and at 200 % text; the docs that describe it are updated in the same patch (the canvas
guide, an ADR amendment where a decision changes, the work log).

**The Loans demo.** Each task adds a part of one plugin, built only with the canvas: by the last task the
maintainer has built it, published it and opened it on a device. The demo is described in §5; each task says what
it adds to it.

## 2. What the research found

### 2.1 What exists and works

- **Layout is complete and faithful.** `@plux/layout` ports Flutter's layout (boxes, flex, stack, slivers, text with
  SkParagraph) and builds every one of the 149 Layer 1 and 58 Layer 2 widget types; a conformance suite checks
  positions and sizes against Flutter goldens (`STU-005`, 326 scenes in `test/layout-conformance/goldens`).
- **The canvas itself:** all screens on one infinite canvas in device frames, dragging and snapping, bundled arrows,
  ghost screens and native nodes (`STU-040`–`STU-055`), the overflow region (`STU-056`), device presets and the
  device matrix (`STU-057`), the preview switches (`STU-058`), insert by click and drag, reparent, wrap and unwrap,
  copy and paste with ID remapping (`STU-067`), Layers with rename, hide and lock (`STU-061`), the inspector tabs
  (`STU-062`), the nine full-screen editors (`STU-063`–`STU-065`), the developer console (`STU-066`), undo and
  history (`STU-069`), the safety rules (`STU-070`–`STU-077`), components and templates (`STU-080`–`STU-086`), send
  to device and the publish flow (`STU-100`), and translated texts bound to design-time mocks (`DAT-080`).
- **The runtime** (`packages/plux_flutter` and the optional packages) renders all of it on the device, and the Dev
  app (`apps/plux_dev`) pairs with Studio.

### 2.2 What is missing (each gap is covered by a task)

| # | Gap | Evidence | Task |
|---|---|---|---|
| G1 | **Screens are drawn as grey boxes and black text.** The layout hands the canvas only `box`, `text` and `placeholder` rectangles; every box is the same grey rounded rectangle and every text has one size and colour. No colour, decoration, border, elevation, clipping or opacity is painted. | `packages/layout/src/widgets/screen.ts` (`PlacedKind`), `packages/canvas/src/screens.ts` (`boxItem`) | PC-01 |
| G2 | **Theme colours never reach the screens.** The theme gives the canvas only font family, font size, page padding and spacing; the dark preview changes the canvas, not the screens. | `apps/web/src/features/plugin-canvas/look.ts` (`resolveTheme`) | PC-01 |
| G3 | **No text styles:** size, weight, colour, type scale, alignment, overflow, rich text. | `screens.ts` draws every text at the theme size | PC-02 |
| G4 | **No icons:** no Material Symbols or Cupertino icon font is loaded; an `Icon` is a grey box. | no icon font under `studio/` | PC-03 |
| G5 | **No images:** `Image`, avatars and decoration images are grey boxes. | `screens.ts` | PC-04 |
| G6 | **Material and Cupertino components do not look like themselves** (buttons, fields, bars, cards, chips, switches…). | G1 | PC-05–PC-08 |
| G7 | **Layer 2 components and heavy widgets** (charts, maps, media, scanners) are boxes. | G1 | PC-09, PC-10 |
| G8 | **The app bar is not the page's own:** the screen draws a title bar of its own and skips the `appBar` slot. | `screen.ts` (`SKIPPED_SLOTS`) | PC-12 |
| G9 | **Dialogs, bottom sheets and full-screen dialogs are not on the canvas at all:** only `screen` pages are placed. | `plugin-model.ts` (`pageKind === "screen"` filter) | PC-13 |
| G10 | **No page settings on the canvas:** route, kind, parameters with mocks, page state, transitions and guards have no inspector of their own. | — | PC-14 |
| G11 | **The canvas is static:** lists and scroll views do not scroll, tabs and page views do not switch, the drawer does not open (`STU-068` exists in the engine only). | `packages/layout/src/sliver.test.ts` only | PC-15 |
| G12 | **No direct manipulation beyond select and drag:** no inline text editing, no resize, padding or gap handles, no contextual toolbar. | — | PC-16–PC-20 |
| G13 | **Complex properties have no real editor:** `TextStyle`, `BoxDecoration`, `ShapeBorder`, `BorderSide`, `WidgetStateColor`, `ImageSource`, `Curve`, `BoxConstraints` fall back to a generic editor. | `apps/web/src/features/panels/properties-model.ts` (`editorKind`) | PC-01–PC-04, PC-21, PC-22 |
| G14 | **No editors on the canvas for lists from data, conditions, forms, animations, timelines, hero and page transitions.** | no Studio code for item templates, form fields, keyframes, hero tags | PC-23–PC-30 |
| G15 | **Accessibility, localisation, responsive and problems are panels, not things seen on the screen.** | — | PC-31–PC-34 |

### 2.3 What the spec asks of the canvas (summary)

- **The look:** Flutter's layout model, styled from widget descriptors and theme tokens (`STU-005`), real fonts and
  shaping (`STU-006`), Material 3 and Cupertino from tokens (`THM-001`–`THM-005`), icons (`THM-005`), images
  (`AST-002`), Layer 2 components (`WGT-020`), native slots as placeholders or screenshots (`WGT-031`).
- **The screens:** device frames, presets and the matrix (`STU-057`), page kinds (`SCH-022`), overflow (`STU-056`),
  a live design surface (`STU-068`), preview switches (`STU-058`).
- **Editing:** selection, deep select, drag and drop with indicators, wrap and unwrap, copy and paste, smart guides,
  measurement, nudging, zoom (`STU-067`); properties generated from descriptors with responsive overrides,
  bindings, token and translation pickers (`STU-062`, `WGT-002`, `WGT-010`).
- **Content and logic:** state (`STA-001`–`STA-004`), forms (`STA-020`), data sources and mocks (`DAT-001`,
  `DAT-080`), lists from data (`WGT-012`, `DAT-011`), actions and triggers (`ACT-001`–`ACT-003`), navigation
  (`NAV-001`, `NAV-005`, `NAV-010`), animation (`ANI-001`–`ANI-007`).
- **Quality:** accessibility (`A11Y-001`–`A11Y-007`), localisation and RTL (`I18N-004`, `I18N-007`, `I18N-010`,
  `I18N-012`), responsive (`WGT-010`, `WGT-011`), validation as you type (`SCH-040`, `SCH-042`).
- **Ship:** send to device and live debugging (`DEV-020`, `DEV-033`), publish (`STU-100`).

## 3. Decisions the maintainer is asked for (not blocking the first task)

1. **Requirement status.** `STU-005` ("…styles them from widget descriptors and theme tokens") and `STU-068` (lists
   scroll, tabs switch, timelines scrub on the canvas) are `DONE`, but Studio shows neither. **Recommendation:** set
   them to `WIP` until PC-01 and PC-15/PC-30 land, so the status column stays true (AGENTS.md §3).
2. **Icon fonts in Studio** (PC-03): bundle Material Symbols (Apache-2.0) and the Cupertino icons font (MIT) for the
   canvas, as the device does. They are assets, not code; a row in `docs/engineering/dependencies.md` records them.
3. **Painting is reviewed by eye, not by pixels.** ADR-0013 compares positions and sizes, never pixels. The look tasks
   are checked against a **Widget gallery** (PC-01) next to the same screens on a device; no pixel gate is added.

## 4. The tasks

Each task: **You will see** (what changes on screen), **Scope**, **Spec**, **Needs** (earlier tasks), **Demo** (what
it adds to the Loans demo) and **Check** (the steps the maintainer follows).

### Part A — The look: screens drawn as the app will look

#### PC-01 Paint: colours, decoration, elevation and the theme's colours
- **You will see:** boxes take their real colours, gradients, borders, corner radii, shadows and opacity; the
  theme's colour scheme (light and dark, brand overlays) colours the screens; switching **Dark** in Preview changes
  the screens, not only the canvas.
- **Scope:** the layout reports what each box paints (fill or gradient, border, per-corner radius, shadow or
  elevation with the Material 3 surface tint, opacity, clip) for `Container`, `DecoratedBox`, `ColoredBox`, `Card`,
  `Material`, `ClipRRect`, `ClipOval`, `Opacity` and `PhysicalModel`; the canvas draws it (new scene item fields;
  rounded clips); the colour scheme is built from the theme tokens (`ColorScheme` roles) and resolved per light,
  dark and brand; a **Widget gallery** sample plugin (one screen per widget category, importable as a `.plux`
  archive) for reviewing every look task; editors for colour with token and opacity, and for `BoxDecoration` (fill,
  gradient, border, radius, shadow) in Properties.
- **Spec:** `STU-005`, `THM-001`, `THM-002`, `THM-003`, `STU-058`.
- **Needs:** —
- **Demo:** the Home screen's balance card (gradient, radius 20, soft shadow) and page background.
- **Check:** import the gallery; the Containers screen shows the colours, gradients, borders, radii and shadows of
  its props; toggle **Dark** and a brand overlay in Preview and the screens recolour; set a decoration on a widget
  in Properties and see it at once.

#### PC-02 Text: styles, type scale and rich text
- **You will see:** text in its real size, weight, colour, font and line height; headlines, body and labels from the
  theme's type scale; alignment, ellipsis and fade, rich text spans.
- **Scope:** `TextStyle` (size, weight, style, colour, letter and word spacing, height, decoration, shadows, font
  family per script) measured and drawn identically; `DefaultTextStyle` inheritance and the Material 3 type scale
  (display, headline, title, body, label) from tokens; `textAlign`, `maxLines`, `overflow`, `softWrap`; `RichText`,
  `Text.rich` and `SelectableText`; host and plugin fonts; a `TextStyle` editor with type-scale presets.
- **Spec:** `STU-005`, `STU-006`, `THM-001`, `THM-004`.
- **Needs:** PC-01.
- **Demo:** Home's greeting (headline), balance (display, tabular figures) and captions.
- **Check:** the gallery's Typography screen shows every type-scale role; long text ellipsises at `maxLines`;
  Arabic and Amharic render with their fonts; changing the style in Properties updates the text in place.

#### PC-03 Icons
- **You will see:** real Material Symbols (outlined, rounded, sharp; fill and weight) and Cupertino icons in icons,
  buttons, bars and tiles; custom SVG icon sets; an icon picker with search.
- **Scope:** icon fonts loaded by the canvas worker and drawn as glyphs with size, colour, fill, weight, grade and
  optical size; `Icon`, `IconButton`, `ImageIcon`, icons inside other widgets; the app's custom SVG icon sets
  (`plux_svgc`); an icon picker in Properties (search, style, recent).
- **Spec:** `THM-005`, `STU-062`.
- **Needs:** PC-01, decision §3.2.
- **Demo:** the quick-action icons on Home and the bell in the app bar.
- **Check:** the gallery's Icons screen; pick an icon for a button in Properties; Cupertino icons under the
  Cupertino switch.

#### PC-04 Images and media posters
- **You will see:** images from Assets and from the network (with their placeholder and error images), avatars,
  decoration images, SVGs, and the poster or first frame of video, Lottie and Rive.
- **Scope:** `Image` (asset, network, memory) with `fit`, `alignment`, `repeat`, rounded clips; `CircleAvatar`,
  `DecorationImage`, `FadeInImage`; network images previewed from the mock or a placeholder (`AST-002`);
  per-locale images (`I18N-012`); an image picker from Assets; drag an asset onto a screen to insert an image.
- **Spec:** `AST-002`, `I18N-012`, `ANI-005`.
- **Needs:** PC-01.
- **Demo:** the offer images on Home and the profile avatar.
- **Check:** upload an image in Assets, drag it onto a screen, change its fit; a network image shows its placeholder.

#### PC-05 Material 3: actions and selection controls
- **You will see:** filled, tonal, elevated, outlined and text buttons, icon buttons, FABs (small, regular, large,
  extended), chips (assist, filter, input, suggestion, choice), switches, checkboxes, radios, sliders, range
  sliders, segmented buttons, progress indicators and badges as Material 3 draws them, in their selected, disabled
  and error states.
- **Scope:** component tokens from the colour scheme and type scale; a **state** switch in Preview (enabled,
  pressed, disabled, selected, error) for every control; values from props or bound state.
- **Spec:** `STU-005`, `THM-001`, `STU-058`.
- **Needs:** PC-01, PC-02, PC-03.
- **Demo:** the FAB "Apply", the filter chips on Home, the switches on Profile.
- **Check:** the gallery's Actions and Selection screens next to the same screens on the Dev app.

#### PC-06 Material 3: text fields and pickers
- **You will see:** filled and outlined text fields with label, hint, helper, counter, prefix and suffix icons and
  the error state; dropdown menus, search bars, autocomplete; date and time pickers and steppers drawn as they are.
- **Scope:** `InputDecoration` in all states (empty, focused, filled, error, disabled); `DropdownMenu`, `SearchBar`,
  `SearchAnchor`, `Autocomplete`, `CalendarDatePicker`, `Stepper`; the keyboard type shown as a badge.
- **Spec:** `STU-005`, `STA-020`.
- **Needs:** PC-05.
- **Demo:** the fields of the Apply form.
- **Check:** the gallery's Inputs screen; toggle the error state in Preview.

#### PC-07 Material 3: bars, surfaces and lists
- **You will see:** app bars (small, medium, large, sliver), navigation bar, navigation rail, navigation drawer,
  bottom app bar, tab bar, cards (elevated, filled, outlined), list tiles (with switch, checkbox, radio), dividers,
  expansion tiles, data tables and material banners as they are drawn on the device.
- **Scope:** surfaces with elevation and tint, selected destination indicator, tab indicator, list tile leading and
  trailing slots, dense and three-line tiles.
- **Spec:** `STU-005`, `THM-001`.
- **Needs:** PC-05.
- **Demo:** the navigation bar on Home, the settings list on Profile.
- **Check:** the gallery's Navigation and Surfaces screens.

#### PC-08 Cupertino look and adaptive widgets
- **You will see:** with **Cupertino** in Preview, Cupertino widgets and every `adaptive` widget take the iOS look
  (switches, sliders, navigation bars, list sections, action sheets, alert dialogs, activity indicator, date
  picker), with SF Pro where the machine has it.
- **Scope:** Cupertino painting and the `adaptive` prop (`WGT-011`); the platform switch drives which variant is
  drawn.
- **Spec:** `WGT-011`, `STU-058`, `THM-001`.
- **Needs:** PC-05–PC-07.
- **Demo:** the demo viewed in Cupertino.
- **Check:** the gallery under Material and Cupertino side by side (device matrix).

#### PC-09 Layer 2 components: everyday
- **You will see:** the Layer 2 components built from Layer 1 widgets drawn as on the device: money text, price
  tags, amount, IBAN, phone, OTP and PIN inputs, masked text, pickers (country, currency, dual calendar),
  account, payment, product and live ETA cards, cart summary, transaction rows, profile header, promo banner,
  settings list, status timeline, onboarding pager, wizard form, rating, quantity stepper, empty, error and
  offline states, skeleton loader, terms viewer, markdown view, consent checkbox, biometric button, file uploader,
  signature pad, search field, filter bar, sort sheet, countdown.
- **Scope:** Layer 2 builders composed of the painted Layer 1 widgets; their mocks drive what they show.
- **Spec:** `WGT-020`, `DAT-080`, `I18N-011`.
- **Needs:** PC-01–PC-07.
- **Demo:** the transaction list and money text on Home, the amount and OTP inputs on Apply.
- **Check:** the gallery's Components screens next to the Dev app.

#### PC-10 Layer 2 components: charts, media, maps, scanners and native slots
- **You will see:** bar, line and pie charts and sparklines drawn from their mock data; image gallery and carousel;
  Lottie and Rive previews (first frame, playable in PC-30); video and audio players with poster and controls;
  map and order-tracker previews with markers; QR and barcode views; camera and scanner frames; native slots as
  labelled placeholders or the device's screenshot.
- **Scope:** canvas previews for the optional packages (`plux_charts`, `plux_maps`, `plux_media`, `plux_lottie`,
  `plux_rive`, `plux_scanner`); native slot screenshots from a paired device (`DEV-033`).
- **Spec:** `WGT-021`, `WGT-031`, `ANI-005`, `DEV-033`.
- **Needs:** PC-04, PC-09.
- **Demo:** the repayment chart on Offer details and the success animation.
- **Check:** the gallery's Media and Data screens.

### Part B — Screens as real device screens

#### PC-11 Device chrome, safe areas and the screen header
- **You will see:** each frame with the device's status bar (time, signal, battery), notch, island or punch hole,
  home indicator or navigation bar, rounded display corners, and the layout inset by the safe area; above each
  frame its name, the entry badge, its kind and route.
- **Scope:** chrome per preset and orientation (`STU-057`), safe-area insets fed to the layout (`SafeArea`,
  `MediaQuery` padding), light or dark status bar from the app bar's colour; a keyboard overlay preview for
  screens with a focused field.
- **Spec:** `STU-057`, `STU-050`.
- **Needs:** PC-01, PC-02.
- **Demo:** all demo screens in an iPhone and a Pixel frame.
- **Check:** switch device presets and orientation; content never sits under the notch.

#### PC-12 Scaffold anatomy in place
- **You will see:** the page's own app bar (title, leading, actions, colours) instead of the canvas's title bar; the
  bottom navigation bar, FAB in its location, drawer, persistent bottom sheet and a snackbar preview where the page
  has them; clicking a bar selects it and empty slots offer **Add app bar**, **Add FAB**, **Add navigation bar**.
- **Scope:** draw the `appBar` slot (remove the synthetic bar), every `Scaffold` and `CupertinoPageScaffold` slot,
  FAB locations, `extendBody`, drawer open preview; slot placeholders on the canvas for empty slots.
- **Spec:** `STU-005`, `STU-067`.
- **Needs:** PC-07, PC-11.
- **Demo:** Home's large app bar with avatar and bell, its navigation bar and FAB.
- **Check:** add and remove an app bar, a FAB and a navigation bar from the canvas.

#### PC-13 Dialogs, bottom sheets and full-screen dialogs
- **You will see:** pages of kind `dialog`, `bottomSheet` and `fullscreenDialog` on the canvas, each presented over
  a dimmed copy of the screen that opens it (or a neutral backdrop), with arrows into them; **New screen** offers
  the four kinds.
- **Scope:** place and draw the other page kinds (today filtered out); their size rules (dialog width, sheet
  heights and drag handle, full-screen dialog's close button); `openDialog` and `openBottomSheet` actions shown as
  arrows; changing a page's kind.
- **Spec:** `SCH-022`, `NAV-005`, `STU-050`, `STU-051`.
- **Needs:** PC-11.
- **Demo:** the Review bottom sheet and the Confirm dialog.
- **Check:** create one page of each kind; link a button to the sheet; the sheet shows over Apply.

#### PC-14 Page settings
- **You will see:** with a screen's frame selected, the inspector shows the page: route name, kind, title
  (translatable), typed parameters with defaults and design-time mock values, page state, page data sources,
  lifecycle triggers (enter, leave, resume), transition, guards and entry screen.
- **Scope:** a Page inspector built from the page schema; parameters' mocks drive what the canvas shows; route name
  rules and rename warnings (`SCH-025`, `STU-076`).
- **Spec:** `SCH-022`, `SCH-024`, `SCH-025`, `NAV-007`, `NAV-009`, `NAV-010`, `STU-076`.
- **Needs:** PC-13.
- **Demo:** Offer details takes `offerId` with a mock value.
- **Check:** add a parameter with a mock; the screen shows the mocked offer; rename the route and see the warning.

#### PC-15 A live design surface
- **You will see:** in **Interact** mode (a toggle on the top bar, or holding a key), lists and scroll views scroll
  inside their frames, tabs and page views switch, the drawer opens, expansion tiles expand, steppers step; the
  overflow region follows the scroll; nothing navigates or calls the network.
- **Scope:** scroll offsets, selected tab, page index, open drawer and expanded tiles held per frame as view state
  (not in the document); the layout runs at that state (`STU-068`).
- **Spec:** `STU-068`, `STU-056`.
- **Needs:** PC-07, PC-12.
- **Demo:** scroll Home's transaction list; switch the tabs on Profile.
- **Check:** scroll a long list in a frame; switch tabs; leaving Interact mode keeps the view.

### Part C — Direct manipulation

#### PC-16 Selection, hover and spacing feedback
- **You will see:** a hover outline with the widget's type; the selection with its name and size; padding, margins
  and gaps tinted on hover (as design tools do); distances to the neighbours; hidden and locked marks; a problem dot
  on nodes that have validation problems.
- **Scope:** overlay drawing on the canvas for hover, selection, spacing, measurement, problems; consistent with
  Layers.
- **Spec:** `STU-067`, `SCH-040`.
- **Needs:** PC-01.
- **Demo:** —
- **Check:** hover a padded column and see its padding and gaps; a node with an error shows a dot.

#### PC-17 Inline text editing
- **You will see:** double-click a text, a button label or a field's label to edit it on the canvas; Enter keeps,
  Escape cancels; the text becomes a translation key automatically; a bound text opens the PXL editor instead.
- **Scope:** an editor over the text with its style and width; extraction to keys (`I18N-007`); undo as one step.
- **Spec:** `STU-067`, `I18N-007`, `PXL-008`.
- **Needs:** PC-02.
- **Demo:** write all of Home's texts on the canvas.
- **Check:** edit a label in place; it appears in Translations with a new key.

#### PC-18 Resize, spacing and alignment handles
- **You will see:** handles that set width and height (or wrap in a `SizedBox`), drag the padding of a box, drag the
  gap of a row or column; values snap to the spacing tokens; arrow keys nudge (move among siblings, or change
  padding with a modifier); alignment buttons.
- **Scope:** handles mapped to the props Flutter has (no free positioning outside a `Stack`); token snapping;
  `Positioned` offsets inside a `Stack`.
- **Spec:** `STU-067`, `THM-001`.
- **Needs:** PC-16.
- **Demo:** size the balance card and space the quick actions.
- **Check:** drag a column's gap; resize a box; nudge a child up and down.

#### PC-19 Contextual toolbar and quick structure edits
- **You will see:** a small toolbar by the selection: wrap in Row, Column, Stack, Card, Padding or Center; convert
  (Row ↔ Column, button variant); add a sibling before or after (**+** on the edges); align and distribute;
  duplicate, delete, add an event.
- **Scope:** the toolbar's commands are registry commands with shortcuts (studio-development §5.7).
- **Spec:** `STU-067`.
- **Needs:** PC-16.
- **Demo:** build the quick-actions grid with the toolbar alone.
- **Check:** wrap two widgets in a Row and convert it to a Column.

#### PC-20 Drag and drop everywhere
- **You will see:** drop targets for slots (app bar title and actions, FAB, bottom bar, list tile leading and
  trailing), drops inside a `Stack` that create `Positioned`, drags between screens, assets dropped as images,
  components and templates dropped from Insert.
- **Scope:** insertion indicators per container kind (`STU-067`); slot rules from the descriptors.
- **Spec:** `STU-061`, `STU-067`, `STU-083`.
- **Needs:** PC-12, PC-18.
- **Demo:** drop the bell icon into the app bar's actions.
- **Check:** drag an icon into a list tile's trailing slot; drag a card from Home to Offer details.

### Part D — The styling panel

#### PC-21 Style editors
- **You will see:** colour per state (`WidgetStateColor`: pressed, hovered, disabled, selected), shapes
  (`ShapeBorder`, `BorderSide`), clip and opacity editors; every style value token-first with a **detach** to a raw
  value; copy and paste style between widgets.
- **Scope:** the editors the look tasks did not add; a style clipboard.
- **Spec:** `STU-062`, `THM-001`.
- **Needs:** PC-01–PC-05.
- **Demo:** the button colours per state on Apply.
- **Check:** set a pressed colour; copy the style of one card to another.

#### PC-22 Layout editors and size classes
- **You will see:** visual editors for constraints, flex (direction, main and cross alignment, size, spacing),
  `Expanded` and `Flexible`, `Stack` and `Positioned`, `Wrap`, durations and curves; each value can be set per size
  class (compact, medium, expanded) with a mark on overridden values; **Reset** to the default.
- **Scope:** generated from the descriptors (`WGT-002`); responsive overrides (`WGT-010`).
- **Spec:** `STU-062`, `WGT-002`, `WGT-010`.
- **Needs:** PC-18.
- **Demo:** the quick actions wrap to two rows on compact.
- **Check:** override a padding for **expanded** and see it in the device matrix.

### Part E — Content, data and logic

#### PC-23 State, bindings and mocks on the canvas
- **You will see:** a binding badge on bound props and nodes; bound values shown from state defaults and mocks;
  a mock state per data source (success, loading, empty, error) in Preview; **Create state from this field**.
- **Scope:** binding picker (state, parameters, data, flags, environment) with PXL autocompletion; computed state.
- **Spec:** `STA-001`–`STA-004`, `PXL-002`, `PXL-008`, `DAT-080`.
- **Needs:** PC-14.
- **Demo:** the balance bound to `account.balance`; the monthly payment computed from `amount` and `term`.
- **Check:** bind a text to state and change the state's default; switch a data source to **error**.

#### PC-24 Lists and grids from data
- **You will see:** a list or grid bound to a list or a data source shows its items from the mock; the first item is
  the template and editing it updates them all; the empty, loading, error and end-of-list states are slots you can
  see and edit by switching the mock state.
- **Scope:** item templates (`WGT-012`) for `ListView`, `GridView`, sliver lists and grids and `PageView`;
  pagination states (`DAT-011`).
- **Spec:** `WGT-012`, `DAT-011`, `DAT-080`.
- **Needs:** PC-23.
- **Demo:** Home's offers carousel and transactions list from the `offers` and `transactions` sources.
- **Check:** edit the item template; switch the mock to **empty** and edit the empty state.

#### PC-25 Conditional and repeated content
- **You will see:** nodes with a `visible` condition marked and shown or hidden from the mocks; **Show hidden
  branches** reveals them; `If`, `Match` and `ForEach` show each branch or item on the canvas.
- **Scope:** branch switcher on the canvas; conditions evaluated against the preview state.
- **Spec:** `SCH-023`, `PXL-001`.
- **Needs:** PC-23.
- **Demo:** the "No offers" banner shown only when the list is empty.
- **Check:** toggle the condition's state and see the branch change.

#### PC-26 Forms
- **You will see:** a form's fields with their validators, their error texts and the submit button's busy state,
  previewed by a **Form state** switch (pristine, invalid, submitting, submitted); validators edited per field.
- **Scope:** form state (`STA-020`), built-in validators, keyboard types, `submitForm`, `validateForm`, `resetForm`
  actions; Layer 2 inputs in forms.
- **Spec:** `STA-020`, `ACT-002`.
- **Needs:** PC-06, PC-23.
- **Demo:** the Apply form with amount, term, date of birth, phone and consent.
- **Check:** mark a field required and preview **invalid**; the error text shows.

#### PC-27 Events and actions from the canvas
- **You will see:** an event badge on widgets that have actions; **On tap → Navigate / Open sheet / Open dialog / Set
  state / Show snackbar / Call API / Call function** from the toolbar; the action graph opens for anything more;
  snackbars and toasts previewed.
- **Scope:** quick actions write normal action graphs (`ACT-001`); triggers and concurrency (`ACT-002`, `ACT-003`).
- **Spec:** `ACT-001`–`ACT-003`, `STU-046`, `STU-047`, `STU-065`.
- **Needs:** PC-13, PC-19.
- **Demo:** Apply's submit opens Review; Confirm's button shows a snackbar and goes to Success.
- **Check:** add an **On tap → Open sheet** in two clicks; the arrow appears.

#### PC-28 Navigation and flows
- **You will see:** typed parameters on arrows (and missing ones flagged), the transition of each route, pop with a
  result, tab switches, **Arrange by flow** that lays the screens out along the navigation, links to native routes
  and other plugins.
- **Scope:** navigate editors with parameter mapping (`NAV-001`), transitions (`NAV-010`), auto-layout
  (`STU-050`), loops (`STU-077`).
- **Spec:** `NAV-001`, `NAV-002`, `NAV-005`, `NAV-010`, `STU-050`, `STU-077`.
- **Needs:** PC-14, PC-27.
- **Demo:** Home → Offer details (`offerId`) → Apply → Review → Confirm → Success, Profile in the navigation bar, a
  link to the Cards plugin and to the native KYC route.
- **Check:** **Arrange by flow**; remove a required parameter and see the problem.

### Part F — Motion

#### PC-29 Implicit animations and transitions
- **You will see:** a **Play** button on a screen that runs its enter transitions, implicit animations, page
  transition and hero flights on the canvas; animation settings (duration, curve, delay) on animatable props.
- **Scope:** `ANI-001`, `ANI-003`, `ANI-004`, `NAV-010`; reduce-motion preview (`ANI-007`).
- **Spec:** `ANI-001`, `ANI-003`, `ANI-004`, `ANI-007`, `NAV-010`.
- **Needs:** PC-28.
- **Demo:** the offer image flies (hero) from Home to Offer details.
- **Check:** play the transition between two screens; turn on reduce motion.

#### PC-30 Timelines, keyframes and animated assets
- **You will see:** a timeline editor docked under the canvas with tracks and keyframes (transform, opacity, colour,
  size), stagger, repeat and reverse; scrubbing moves the screen; Lottie and Rive play, with Rive inputs bound to
  state; scroll-linked effects (collapsing header, parallax) preview while scrolling.
- **Scope:** `ANI-002`, `ANI-005`, `ANI-006`; `startAnimation` and `controlAnimation` actions.
- **Spec:** `ANI-002`, `ANI-005`, `ANI-006`, `STU-068`.
- **Needs:** PC-15, PC-29.
- **Demo:** the Success screen's confetti and check-mark animation; the collapsing header on Offer details.
- **Check:** add two keyframes and scrub; scroll Offer details and see the header collapse.

### Part G — Quality on the screen

#### PC-31 Accessibility on the canvas
- **You will see:** overlays for contrast failures (`A11Y-003`), small touch targets (`A11Y-004`), focus and reading
  order numbers, missing labels; a semantics editor (label, hint, role, heading, live region, group, focus order).
- **Scope:** checks run on the painted colours of PC-01; links to the problem and its fix.
- **Spec:** `A11Y-001`–`A11Y-006`, `WGT-013`.
- **Needs:** PC-01, PC-16.
- **Demo:** every demo control has a label and passes contrast in light and dark.
- **Check:** lower a button's contrast and see the overlay; reorder focus.

#### PC-32 Localisation and RTL on the canvas
- **You will see:** missing translations and truncated or overflowing text marked on the screens; ICU plurals
  previewed with sample values; per-locale overrides of props and images edited in place; pseudo-locale and RTL
  checks across the plugin.
- **Scope:** overflow detection from the layout; per-locale overrides (`I18N-012`); plural sample values.
- **Spec:** `I18N-001`, `I18N-004`, `I18N-006`, `I18N-010`, `I18N-012`.
- **Needs:** PC-02, PC-17.
- **Demo:** the demo in English, German and Arabic.
- **Check:** switch to the expanded pseudo-locale and see truncations marked; Arabic mirrors.

#### PC-33 Responsive layouts and the device matrix
- **You will see:** the size class of each frame, overridden values marked on the canvas, adaptive navigation
  (navigation bar on compact, rail on medium and expanded), orientation and foldables in the device matrix.
- **Scope:** `Responsive` widget editing; adaptive navigation patterns; matrix presets.
- **Spec:** `WGT-010`, `STU-057`.
- **Needs:** PC-22.
- **Demo:** Home on a tablet uses a navigation rail.
- **Check:** the device matrix with phone, foldable and tablet.

#### PC-34 Problems, performance and budgets on the screen
- **You will see:** problems pinned to their nodes and screens with quick fixes; each screen's node count, estimated
  cost and budget status on its frame; expensive animation warnings (`ANI-008`).
- **Scope:** the Problems drawer and the Performance tab linked to the canvas.
- **Spec:** `SCH-040`, `SCH-042`, `STU-062`, `ANI-008`.
- **Needs:** PC-16.
- **Demo:** the demo has no problems and every screen is within budget.
- **Check:** introduce a broken binding; follow the pin to the node and apply the quick fix.

### Part H — Theme, assets and reuse

#### PC-35 Theme round trip
- **You will see:** the theme editor's changes on the screens at once; brand overlays and high contrast in Preview;
  **Where used** for a token; creating a token from a raw value used on the canvas.
- **Scope:** live theme preview, token usage index, raw-to-token conversion.
- **Spec:** `THM-001`–`THM-003`, `STU-063`.
- **Needs:** PC-01–PC-05, PC-21.
- **Demo:** the bank's brand overlay.
- **Check:** change the primary colour in the theme editor; every screen updates.

#### PC-36 Assets workflow
- **You will see:** images, SVGs, fonts, Lottie and Rive files uploaded, previewed and dropped on screens; **Replace**
  keeps every reference; size limits warned at upload (`AST-003`); where each asset is used.
- **Scope:** Assets tab with previews, replace, usage, limits.
- **Spec:** `AST-001`, `AST-003`, `STU-061`.
- **Needs:** PC-04.
- **Demo:** the demo's images and animations.
- **Check:** replace an image used on two screens; both update.

#### PC-37 Components and templates on the canvas
- **You will see:** component instances outlined with a link badge; **Edit component** opens it in isolation on the
  canvas; overrides marked; **Update instances** shows which change; templates inserted with their parameters
  asked for.
- **Scope:** canvas presentation of `STU-080`–`STU-086`.
- **Spec:** `STU-080`–`STU-086`, `SCH-030`, `SCH-031`.
- **Needs:** PC-16, PC-20.
- **Demo:** the offer card as a component used on Home and Offer details.
- **Check:** extract a component, edit it in isolation and update both instances.

### Part I — Ship

#### PC-38 Device round trip
- **You will see:** **Send to device** opens the edited screen on the paired Dev app; **Highlight selection on
  device** marks the widget there; a side-by-side view of the canvas and the device's screenshot of the same screen.
- **Scope:** `DEV-020`, `DEV-033`; the compare view.
- **Spec:** `DEV-020`, `DEV-033`, `STU-066`.
- **Needs:** PC-11.
- **Demo:** compare every demo screen with the device.
- **Check:** pair the Dev app, send Home, compare.

#### PC-39 Publish and verify the Loans demo (acceptance)
- **You will see:** the Loans demo, built only with the canvas, published through the release flow and running on
  the Dev app: every screen, sheet and dialog, the navigation, the form, data from mocks and the live sources, the
  animations, three locales, light and dark.
- **Scope:** an acceptance checklist (§5) run by the maintainer; any gap found becomes a task.
- **Spec:** `STU-100`, `REL-*`, `SYN-*`.
- **Needs:** all.
- **Demo:** the whole demo.
- **Check:** the checklist of §5, signed off by the maintainer.

## 5. The Loans demo and its acceptance checklist

| Screen | Kind | Built with |
|---|---|---|
| Home | screen | large app bar with avatar and bell; balance card (gradient, display text, money); quick actions (icons, wrap); offers carousel from `offers`; transactions list from `transactions` with empty state; navigation bar (Home, Loans, Profile); FAB **Apply** |
| Offer details | screen | collapsing image header (hero from Home); chips; rich text; repayment chart; FAQ expansion tiles; sticky **Apply** bar; parameter `offerId` |
| Apply | screen | wizard form: amount input, term slider, date of birth, phone, OTP, consent; validation; submit with busy state |
| Review | bottomSheet | summary with money text and **Confirm** |
| Confirm | dialog | confirmation text and two actions |
| Success | fullscreenDialog | Lottie check mark and a confetti timeline; **Done** pops to Home |
| Profile | screen | profile header, settings list with switches (dark mode, notifications), language, logout |

**Acceptance (PC-39):** every screen matches the Dev app in light, dark and RTL (Arabic) and at 200 % text; every
arrow is a working navigation on the device; the form validates and submits; the mock states and the live data
sources both work; the animations play and respect reduce motion; no problem in Problems; within every budget; the
release is published and synced to the device.

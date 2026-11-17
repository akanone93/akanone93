<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Building the Apply Register app end to end (maintainer request, 2026-11-16)

The maintainer's build sheet (`ALL-IN-ONE-APP.md`, sections A–O) is built in Studio as a person would, on the
local stack, with a designed look, and every defect found on the way is logged below, root-caused and fixed
(small ones decided here, big ones put to the maintainer). Progress screenshots go to the maintainer as the build
advances.

## Order of work

1. Decisions of 2026-11-16 — done as each lands:
   - [x] Device compare: no light/dark check; the developer sets both sides alike.
   - [x] Translation catalogue check with `--overwrite` wherever it runs (`mk/studio.mk`, `studio/AGENTS.md`).
   - [x] Restrictions: pick the team or person instead of typing an identifier.
   - [x] Small walk findings: a not-found page inside the app frame; the copy after adding a key.
   - [x] Translation keys referenced only by their immutable identifier everywhere (investigate every use).
   - [x] Deleting a key removes its messages from every locale properly (investigate first).
2. `ar-api`: a stand-alone Go service with handlers only — in-memory data, a minimal JWT flow, REST, GraphQL,
   WebSocket and SSE as §A1 lists, no tests — delivered zipped (not in this repository).
3. The app, section by section (B → J), each screen designed (tokens, colours, radii, structure) and checked on
   the canvas in every preview mode; screenshots sent as it grows.

## Issues found

Numbered in the order found. State: `open`, `fixed (<commit>)`, `decision`.

| # | Where | Issue | State |
|---|---|---|---|
| 1 | Router | An unknown URL shows a bare "Page not found" outside the app frame, with no way back. | fixed (shell layout's not-found page with a way home) |
| 2 | Profile › Security keys | After adding a key the message says the session "counts as signed in with it", yet the next protected action still asks for a fresh confirmation. | fixed (copy: the key signs you in; protected actions ask for it again) |
| 3 | Administration › Restrictions | A restriction's target is a team or person identifier typed by hand. | fixed (one "Applies to" choice of the organisation, a team or a person, by name; the list shows names) |
| 4 | Plugin canvas › Insert | Inserting a widget into a selected Scaffold leaves the Scaffold selected; the new widget is neither selected nor visible (an empty ElevatedButton draws nothing). | fixed (the insert selected the new id against the node index from before the edit, which ignores an id it does not hold; the id now waits until the index has it, then is selected and brought into view; drops do the same) |
| 5 | Plugin canvas › Overview | The plugin editor's header counts 11 widgets while the Overview card counts 8. | open |
| 6 | Plugin canvas › link menu | Screens with the same title could not be told apart (fixed in 24a13bad: the route is added). | fixed (24a13bad) |
| 7 | Translation catalogues | `lingui extract` keeps the English text of a changed explicit-id message, so a changed string never shows and its translations go stale unnoticed. | fixed (`--overwrite` in the gate and the recipe) |
| 8 | PXL `t()` | `t()` named a key by its name, against I18N-007 (documents use `{"$t": id}`), so a rename would break it; and it never worked: neither the runtime nor the server gave PXL a translate function, so every `t()` failed when evaluated. | fixed (`t(keyId, args)`: the compiler resolves the identifier and reports a name or unknown id as PLX-1102; the runtime formats the key's message by identifier; spec table, stdlib reference and guide say so) |
| 9 | Studio › where used | Studio's key usage (Used in, the delete guard of STU-072) saw only `{"$t": id}`, so a key read by an expression could be deleted while in use. | fixed (expressions' `t("<id>")` count as uses) |
| 10 | Studio › expression editor | Nothing helped write `t()`; with identifiers it would be unusable. | fixed (inside `t("…")` the editor offers the keys by name and inserts the identifier; a chip shows each identifier's key name, "?" when unknown) |
| 11 | Localisation › keys | A key could not be renamed. | fixed (rename in place; only the name changes, as every reference is by identifier) |
| 12 | Localisation › delete | Deleting a key left its messages in the other locales' documents. Investigated: locks allow the draft holder (an app manager) to write every locale; undo is client-side, so a server-side cascade would make undo lose text. | fixed (one undo step removes the key's message and review state in every locale; undo restores all; releases still leave out any message without a key) |
| 13 | Design › tokens | A token's high-contrast value was saved as `$extensions.dev.plux.highContrast`; the compiler (A11Y-003) and the device read the group `contrast.high.<path>` (ADR-0032), so high-contrast values set in Studio never reached a phone or the contrast check. | fixed (written and read at `contrast.high.<path>`, typed as the token) |
| 14 | Design › brands | Brand overlays were saved as `brand.<name>`; the device reads `brands.<name>`. The canvas read both, so it showed brand colours a phone never would. | fixed (`brands.<name>`; the canvas reads only what the device reads) |
| 15 | Design › typography | Script fonts were saved as `typography.<script>` (`typography.latin`); the device reads `font.script.<ISO 15924>` (`font.script.Ethi`), and `typography.*` is where the text styles live. | fixed (`font.script.<code>`) |
| 16 | Design › typography | The families field dropped a typed comma at once, so a fallback could only be pasted. | fixed (kept as typed, saved on leaving the field or Enter) |
| 17 | Design › add token | A `number` or `fontWeight` token was saved as text (`"1.5"`), which the device does not convert. | fixed (stored as its type keeps it; lists for `fontFamily`; JSON for composites) |
| 18 | Design › import | An alias on a non-colour token (`{size.base}` for a `number`) was refused; a dark value was imported without checking it fits the type. | fixed |
| 19 | Design › import | Import took only pasted text, though the guide says it reads a file. | fixed (drop or choose a `.json` file; a file over the document size limit is refused before it is read) |
| 20 | Runtime › tokens | The device never followed DTCG aliases (`{color.primary}`), while the canvas and the contrast check did: a Figma theme, mostly aliases, looked right in Studio and fell back to defaults on a phone. | fixed (aliases followed through brand, contrast and mode, inside composites too, at most 8 deep and 64 lookups; the canvas does the same) |
| 21 | Compiler › theme | An alias naming no token or looping compiled silently. | fixed (PLX-1102 at the alias) |
| 22 | Canvas › brand preview | The layout theme looked the brand up as a separate theme document, not `brands.<name>` in the app's theme. | fixed |
| 23 | Canvas › fonts | The canvas ignored the script fonts, so Amharic text was drawn without the Ethiopic family the theme names. | fixed (`font.script.*` families are fallbacks, as on the device) |
| 24 | Design › remove token | Removing a token left its empty groups, against its own doc comment, and left its high-contrast and brand values orphaned. | fixed (one undo step removes all and prunes empty groups) |
| 25 | Design › icons | **Icons › Add icon** saves `icons.<set>.<name>` tokens that nothing reads: icons are Material Symbols or Cupertino by name, subset at publish, and custom icons are SVG assets shown with `Image` (ADR-0032, `docs/reference/icons.md`). | decision |
| 26 | Canvas › preview | The preview has no high-contrast switch, so high-contrast values cannot be checked in Studio (STU-058 does not ask for one). | open (suggestion) |
| 27 | Canvas › layout theme | The canvas layout reads top-level `fontFamily`, `fontSize`, `pagePadding` and `spacing` tokens that the device does not read. | open (to investigate against the layout conformance suite) |
| 28 | Every disabled button | Base UI marks a disabled button that stays focusable with `aria-disabled`, not `disabled`, so shadcn's `disabled:opacity-50` never matched: every disabled action in Studio looked enabled (measured: opacity 1 on Localisation's Approve with nothing selected). | fixed (the shared Button dims and shows a not-allowed cursor on `aria-disabled`) |
| 29 | Design, Localisation, Data, Analytics tabs | The area's first tab stayed highlighted on every other tab, so two tabs looked active. | fixed (the index tab is active on its own page only) |
| 30 | New app › Languages | 51 of 183 languages showed as bare codes (`aa (aa)`, `av (av)`) in Chrome, whose trimmed ICU data has no English name for them, and sorted to the top. | fixed (English CLDR names stand in where the browser has none) |
| 31 | New app › Languages | A regional locale read `German (Germany) (de-DE)`, brackets twice. | fixed (`German (Germany) · de-DE`) |
| 32 | New app › Theme | Each ready-made theme repeated its name under itself ("Violet / Violet, primary colour #5B3DF5"). | fixed ("Primary colour #5B3DF5") |
| 33 | Not-found page | The same sentence showed twice (caption and body). | fixed (the body says what to do) |
| 34 | Router | `/o/<org>/apps`, an address people type, was "Page not found". | fixed (goes to the organisation's home, which lists the apps) |
| 35 | Design › Typography | A script with no family said "Latin has no fallback font", as if it had a first choice. | fixed ("Latin text uses the device's fonts…") |
| 36 | Design › Tokens | The type tag is cut to "co…", the column headers are centred over start-aligned values, the table has no frame, and the add form's type select has no visible label. | fixed (a framed table with type tags, colour swatches and aligned headers; the add form is a labelled card) |
| 37 | Design › Tokens | There is no **Export tokens**, though the build sheet (C1) exports, edits and re-imports the file. | fixed (**Export tokens** downloads the theme as DTCG JSON) |
| 38 | Design › Import | The summary counts every valid token as "can be added", even one identical to the theme's; it does not say which are new, changed or unchanged. | fixed (New, Changed, Unchanged and Skipped against the theme; applying is off when nothing would change) |
| 39 | Design › Icons | The sheet expects a custom icon set made of SVG assets (C3); the tab takes a free-text "source" and writes tokens nothing reads (#25). | decision |
| 40 | Every app page | The breadcrumb reads "acme › Design": nothing on the page names the app being edited. | fixed (the breadcrumb names the app, linking to its canvas) |
| 41 | Localisation › Grid | Six solid primary buttons in two rows (Add a locale, Import, Export, Approve, Mark as needs review, Copy from source): no hierarchy between page actions and bulk actions. | fixed (page actions are outlined with icons; Approve is the one solid bulk action) |
| 42 | Localisation › Grid | The locale filter shows bare codes (`en`, `de-DE`) where the wizard showed names. | fixed (locales by name and tag) |
| 43 | Localisation › Grid | The filters have no visible labels and wrap unevenly (two selects, then the status select alone on the next row). | fixed (labelled filters on one row; the search takes the room left) |
| 44 | App settings | The key's hint is squeezed into a narrow column over three lines. | open |
| 45 | New app › Details | The Name field keeps a focus-like ring while the Description field has focus, so two fields look focused. | open |
| 46 | Plugin canvas | The "1 of 5000 nodes" line under the screen is half hidden behind the bottom bar. | open |
| 47 | Plugin canvas › Insert | Every category is shown twice, as tiles and again as the list below; a "More 0" tile leads nowhere; "Navigation13" runs the count into the name. | fixed for "More 0" and the cramped count; the tiles filter the sections below, so both stay (by design) |
| 48 | Runtime › tokens | A `cubicBezier` token converts to nothing on the device (`convertToken` returns null), though Studio offers the type and the sheet's motion easings use it. | open |
| 49 | Every dialog | A dialog had no height limit and shadcn's text area grows with its content: pasting a token file pushed **Add to the theme** below the window, out of reach (blocker for C1). | fixed (dialogs scroll inside past the window's height; text areas stop growing at 20rem) |
| 50 | Design › colour values | Colours were typed as text everywhere in the Design area (token values, add token, brand values), with no picker and no swatch in the table. | fixed (a colour picker beside the code, which also takes alpha and aliases; swatches in the table) |
| 51 | Design › copy | "Light: 1 colours", "acme: 1 overrides", "Latin: 1 fonts"; the import summary read "1 tokens can be added". | fixed (plural forms in English, German and Arabic) |
| 52 | Studio tests | The test harness compiled the English catalogue with a placeholder-only parser, so a plural message showed as raw ICU in every test and nothing could check plural copy. | fixed (the harness compiles `plural`/`select` as Lingui does) |
| 53 | Design › brand overlays | An overlay listed only "acme: 1 overrides": what it replaced could not be seen, changed or removed, more overrides were added only by creating the overlay again under the same name, and an overlay could not be removed. | fixed (a card per overlay listing its overrides, light and dark, each removable; **Add an override**; remove the overlay after a confirmation, undoable) |
| 54 | Design › Tokens | Composite values showed as raw JSON (a typography row ran to hundreds of characters), pushing Dark, High contrast and the actions off the page. | fixed (values read by type: `16px`, `150ms`, `Inter, Roboto`, `cubic-bezier(…)`, `family · size · weight · ×line height`; long ones cut with the whole value in the title) |
| 55 | Design › Tokens | 83 tokens in one undivided list, with no search. | fixed (a table per group, `color`, `typography`, `spacing`…, with counts, and a search by path or type) |
| 56 | Design › Import | Re-importing the theme's own file reported 34 tokens "changed": the server stores keys sorted, and the comparison was key-order sensitive. After **Add to the theme**, the dialog also said nothing would change. | fixed (canonical comparison; counts kept from when the file was read) |
| 57 | Design › Themes | Each mode was a run of swatches on the light page, sorted by name: the dark palette was never seen on a dark surface, and nothing said whether text on a colour could be read. | fixed (a preview per mode on its own surface: each colour and on-colour pair as a tile with sample text and its WCAG contrast, below-AA pairs marked; other colours as swatches) |
| 58 | Design › brand overlays | Eight overlays make a long page of tables; a compact grid of brand cards with their colours side by side would read better. | open (polish) |
| 59 | Tables | The "Actions" heading sits at the start of its column while the buttons sit at its end. | open (polish) |
| 60 | Plugin canvas › problems | The top bar showed 3 errors while the overview said 0, and every widget's problem mark blinked out after each edit: the overview and the marks read only a finished check, so they emptied while the next one ran. | fixed (the last finished check stands until the next one finishes, for this plugin only) |
| 61 | Plugin canvas › Insert | Long widget names were cut ("GestureDete…", "InteractiveVi…", "RefreshIndi…"). | fixed (names wrap at their words, two lines at most) |
| 62 | Plugin canvas › Insert | A new button, chip, list tile or app bar had no label and a Text no words, so most inserts drew nothing on the canvas. | fixed (starter content in Studio's language: "Button", "Chip", "List item", "Title", "Text"; a FAB starts with an icon and "Add"; a Dismissible with a tile to swipe) |
| 63 | Plugin canvas › Insert | Required numbers started at 0 where 0 hides or breaks the widget: an Opacity or AnimatedScale of 0 is invisible, an AspectRatio of 0 and a CupertinoPicker item extent of 0 are invalid. | fixed (opacity and scale 1, aspect ratio 1, item extent 32, header extents 56 and 120) |
| 64 | Plugin canvas › Insert | Into an empty screen a widget went straight into the Scaffold's body, drawn under the status bar and against the edges; once the body was taken, the next widget went into the app bar, the first empty slot. | fixed (a screen's first content goes into SafeArea › 16 px Padding › Column; chrome slots take only their own widgets — an AppBar, a FAB, a navigation bar, a drawer; with the body taken, the next widget goes on into the body's Column) |
| 65 | Widget registry | TextButton's `child` is required while ElevatedButton's, FilledButton's and OutlinedButton's are not, though Flutter requires it for all four; FloatingActionButton's `label` is required although only the extended FAB has one. | decision (registry contract change) |
| 66 | Plugin canvas | The first view centred the screen in the whole height, so the line under it ("1 of 5000 nodes") sat behind the console bar. | fixed (the screen is centred above the console bar, with room for its caption) |
| 67 | Plugin canvas › lock | Reopening a plugin right after closing its tab can find it read-only, locked by the same person ("Developer has been editing this plugin… Take over is possible from 10:12 PM"): the lock outlives the closed session until its lease lapses. A tab closed in a browser releases it on `pagehide`; an automated or crashed session does not. | open (consider letting the holder's own account take over at once) |
| 68 | Plugin canvas | An empty wrapper (Card, Container, Badge, BackdropFilter, Padding…) has no size, so once inserted it can be neither seen nor clicked on the canvas. | open (draw an outline-only placeholder for empty content slots, without changing the layout) |
| 69 | Canvas › CloseButton | A CloseButton draws a grey square without its "×". | open |
| 70 | Canvas › RadioListTile | The radio of a RadioListTile is not drawn; its space is kept. | open |
| 71 | Plugin canvas › Insert | 88 of 149 Layer 1 widgets are invalid the moment they are inserted: required props with no starting value (durations, padding, alignment, list props such as `items`, `destinations`, `tabs`), required slots left empty (`child` of Expanded, SafeArea, Positioned; `item` of ListView), and controls without a semantics label. Full list: [insert-validity.md](insert-validity.md). | fixed for all but what only the person can give (88 → 30 invalid; 24 of the 30 lack only a semantics label, left for the person to write; an Image needs its picture, a FormScope its form): valid first values by prop type — enums, padding, offsets, durations, colours, dates, two entries of a list of tabs, destinations, menu items, segments, columns or steps, typed list items — and starter children for required slots |
| 72 | Build sheet › G1 categories | The sheet binds a category grid's icons to data (`Icon {{ item.icon }}`), but an icon must be written literally: the server subsets the icon fonts to the names a bundle contains (PLX-1123). A data-driven icon grid cannot be built as specified. | decision (allow computed icons from a declared list of names that the subset includes, or keep icons literal and change the sheet) |
| 73 | Plugin canvas › JSON | With nothing selected, the inspector's **JSON** tab lights up but nothing opens; it should open the screen's (or the plugin's) document. | open |
| 74 | Plugin canvas › JSON | The inspector's JSON of a selection is read-only but drawn as a resizable text area, so it looks editable. | open |
| 75 | Plugin canvas › paste | Widget JSON pasted onto the canvas (from the JSON editor, docs or another tool) does nothing and says nothing. | open |
| 76 | CLI sign-in approval | The organisation select on **Approve a command-line sign-in** has no visible label. | open |
| 77 | CLI › export | `plux export` reports "Wrote 9 files to .." — the path `.` runs into the sentence's full stop. | open |
| 78 | New app | The wizard set `minRuntimeVersion` 0.1.0 while 146 registry entries need runtime 0.5.0: SearchBar, the animated widgets and others offered by **Insert** were errors (PLX-1119) in every new app. | fixed (a new app starts at the newest runtime its widgets need) |
| 79 | Canvas › slivers | A screen built on a CustomScrollView with a SliverAppBar that has a title, actions or a flexible space drew completely blank: the scene took a SliverAppBar's parts as several children and refused it, and the whole scroll view became an invisible placeholder (blocker for G1 `home`). | fixed (the SliverAppBar's header draws an AppBar of its leading, title and actions over its flexible space) |
| 80 | Canvas | When the scene cannot build a subtree it draws a placeholder with no paint and reports nothing, so a screen can go blank with no error, no warning and nothing in the console. | open (mark the failed subtree visibly and report why in Problems) |
| 81 | Plugin canvas › problems | The top bar counted 21 warnings while the Problems tab's badge said 23. | open |
| 82 | Canvas › scroll views | A screen whose scroll content is taller than the device draws the rest below the device frame, over the frame's caption, neither clipped nor making the frame taller. | open |
| 83 | Canvas › list data | A ListView whose `items` is a typed expression (a list literal) shows one item, with its `item.*` texts empty: the canvas does not evaluate the expression for the template. | open |
| 84 | Design system | A Text takes a typography token or a whole TextStyle, not a token with one field changed: white headline text on a gradient needs a new token or a hand-made style, losing the theme's type scale. | decision (a style that extends a token, `{"$token": …, "color": …}`, or a colour prop on Text) |
| 85 | Canvas › AppBar | An AppBar's (and a SliverAppBar's) `foregroundColor` is not applied on the canvas: the title and action icons keep the default colours. | open |

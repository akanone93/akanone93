# Studio redesign round 9 — research and plan (2026-11-04)

The maintainer applied round 8 and asked for: the open decisions settled, three fixes and a bonus on the canvases, the
full-screen editors redesigned, every other screen brought to the canvases' standard, and then the plugin canvas's
items (screens, arrows, ghosts). This file records the research and the plan. Requirement IDs are from
`docs/requirements.md`.

## 1. Decisions settled by research

| Question | What the documents and the code say | Plan |
|---|---|---|
| Layer rename, hide, lock (`STU-061`) | `STU-061` asks for them. Nothing implements them: no Studio code, no backend, no schema field. A node's schema refuses unknown properties **except** those prefixed `x-`, which "are preserved but ignored by the compiler" (`SCH-004`; `common.schema.json` node `patternProperties: {"^x-": true}`; `backend/internal/schema/load_test.go` already loads an `x-studio` object). Studio already keeps editor-only data this way (`x-position` on action-graph steps). | Store the three in the node's `x-studio` object: `name` (the layer's name), `hidden` (not drawn on the canvas; the device is untouched, its own switch is `visible`), `locked` (not picked on the canvas, not dragged in Layers, its properties read-only). No schema, compiler or bundle change. |
| Translated screen titles | Not documented. A page title is translatable (`SCH-022`, `{"$t": key}`); the canvas already resolves `$t` for the preview locale (`look.ts`). | The tree and the switcher name a screen by its title resolved in the preview's locale; the first text, then the key, only when the title is missing. |
| When the dev session starts | `STU-131`: a tap on the device selects the node in Studio; `NFR-030`: Studio measures autosave-to-device; contract #17 (`plans/p11.md`): the server pushes every accepted autosave to the app's sessions. Both Studio sides need the session and its event stream while the editor is open, not only while the console is. | Start the Studio dev session when the plugin canvas opens, for a person with `devsession.create`; the event stream runs while the editor is open (inspect, screenshots, hot-apply time); the console only shows it. |
| Merged left tabs (`STU-061` lists Screens, State, Data, Selection as tabs) | The maintainer keeps the merge (2026-11-04). | Reword `STU-061` (Layers holds the screens and the selection; Data holds state and data sources) with a Document Control row and an ADR-0103 amendment, as a changed `MUST` row needs. |

## 2. Fixes on the canvases

1. **Top bars**: the three panels at the top of each canvas share one height (the identity panel of the plugin canvas
   was taller than the others and nearly touched the dock); the docks start a full gap below them.
2. **Insert details**: the card that describes the widget under the pointer moved to the foot of the pane and covered
   the last tiles. It floats beside the hovered tile, outside the dock, and turns upwards near the window's foot.
3. **Bonus**: the inspector's header on the primary colour (as the left rail); both docks fold away to their rail;
   the inspector's **Editors** and its tabs fold separately.

## 3. Full-screen editors (`STU-063`)

Nine sheets: JSON, localisation, state, data sources, database schema, theme, action graph, Rust, history. Findings:
inputs stretch across the window, forms have no column or grouping, headers differ, actions sit in random places.

Plan: one structure for every sheet (`components/editor-sheet.tsx`):

- **Header bar**: the editor's icon on the primary colour, its name and what it edits, its status (valid, saving,
  read-only), the primary action at the end, Close.
- **Body**: a navigation column at the start where the editor has a list (state entries, sources, tables, locales,
  snapshots, files), the work area in the middle, and an optional details column at the end.
- **Forms**: grouped in titled sections, two columns on wide sheets, fields no wider than they need (`max-w` per
  field kind), helper text under fields.
- **Empty states** with an icon, a sentence and the action that fills them.

## 4. Every other screen

About 110 routes in 25 areas. Most share `components/page.tsx` (`PageFrame`, `PageHeader`, `DataTable`) and an area
frame; some areas build their own.

Plan:

- **Page system**: `PageHeader` gets the canvases' identity — an icon tile on the primary colour, a kicker naming the
  area, the title, the caption, and the actions at the end; area sub-navigation becomes a tab strip with icons;
  sections are cards with a header (icon, title, count, actions); forms keep a readable width; tables get the
  canvases' rows (hover, selected, compact counts).
- **Every area frame** moves onto that system (admin, approvals, releases, functions, security, data, localisation,
  design, analytics, experiments, tests, installation, library, publish, settings, profile, notifications).
- **A sweep of every route** in light and dark, fixing what each screen still gets wrong.

## 5. Plugin canvas items

After §2–§4: research the screens, the link arrows and the ghost screens (what the spec asks: `STU-040` to `STU-055`;
what the code does; what the screenshots show — text in screens overlaps at 100 %), then fix and enrich them.

## 6. Order of work

§1 → §2 → §3 → §4 → §5, each verified (tests, lint, screenshots in light and dark) before the next.

## 7. Status (2026-11-04)

All sections done and delivered as one patch. §4: one header card for every area page (`PageHeader`: the area's mark
and name from the sidebar's entry, caption, actions, area tabs), cards for tables, filter rows, forms and empty
states (`PageEmpty`), area tabs (`AreaTabs`) for Security, Releases and Experiments, analytics tabs and range in the
header, the remaining hand-made headings (library, publish, notifications, app and shell settings, Rust editor)
moved onto it, the header a `banner` landmark with organisation › area › page. §5: screen text overlapped because
the renderer laid a measured text out again at its exact floating-point width, where SkParagraph broke the line;
it now lays out at the width rounded up and with the measuring families (regression test in
`packages/canvas/src/screens.test.ts`); labelled boxes (buttons, fields, placeholders) draw their label; each
screen's name is drawn above its frame. The link arrows and ghost screens already meet `STU-040` to `STU-055` and
were left as they are.

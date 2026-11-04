# Canvas editors — research and plan (round 8, 2026-11-03)

The maintainer's review of round 7 asked for a research of the documents and the code behind both canvases, a
plan, and its implementation, without touching the canvas items (screens, arrows, ghosts, native nodes' drawing on
the plugin canvas). This file is that research and plan. Requirement IDs are from `docs/requirements.md`.

## 1. What the documents require

| Area | Requirement | Today | Gap |
|---|---|---|---|
| App canvas box | `STU-030`: name, icon, screens, draft/published and version, badges, **owner** | All but owner | Owner (owning team) not on the box |
| App canvas detail | `STU-031`: description, mini-map, links in/out, functions, bundle size, last published | Inspector card | — |
| App canvas moves | `STU-032`: drag, snap, guides, multi-select, auto-layout, zoom to fit, saved per app | Yes | Opening view fits (≈ 400 % with one plugin); the maintainer wants 100 % centred |
| Native nodes | `STU-033`: dashed outline, native icon | Tinted box | No dashed outline, no native icon |
| Plugin actions | `STU-034`: create, rename, duplicate, archive, configure from **context menu** and toolbar | Toolbar and inspector | No context menu |
| Frames | `STU-035` | Yes | — |
| Action bar | `STU-060`: undo/redo, zoom, frame, theme, locale, text scale, send to device, validation status (opens Problems), history, Publish | All, crowded | Counts jump the bar; zoom duplicated; Send and Publish unbalanced; screen picker unreadable |
| Left panel | `STU-061`: Insert (widgets, components, templates, search, drag); Layers (tree, drag to reorder and reparent, rename, hide, lock, multi-select); Screens; Assets; State; Data; Selection | Seven tabs | Tree unusable (no indentation, no click to expand); rename, hide, lock not storable (no schema field) |
| Right panel | `STU-062`: Properties, Actions, JSON, Code, Localisation, Accessibility, Performance, Docs | Yes | Editor tiles unlabelled; no plugin overview when nothing is selected |
| Heavy editors | `STU-063`: eight full-screen overlays from the right panel | Yes (round 7 frame) | — |
| Bottom drawer | `STU-066`: resizable, collapsible; Problems, Logs, Process flow, State, Network, Functions, Sync, Performance, Console; source device, filters, search, pause, export | All behind a "Devices" toggle in a small box | Not resizable, not visible by default, no live counts, cramped |
| Interactions | `STU-067`: zoom 10–400 %, fit screen and fit all | Yes | Zoom controls differ between canvases |
| Dev loop | `DEV-020`, `DEV-021`, `DEV-030`, `DEV-031`, `DEV-033` | Drawer, controls, inspect, screenshot | Controls hidden, no device status |

## 2. Root causes found in the code

1. **"[object Object]" and "debug" in the screen picker** — `plugin-model.ts` names a screen after its first Text
   widget, and falls back to `String(title)`, which prints an object title (a `{"$t": …}` translation reference) as
   "[object Object]". A screen is named by its title when it is text, otherwise by its key.
2. **The layer tree reads as a flat list** — rows indent by `var(--spacing-4)`, which is not defined, and the row's
   click selects, so a folder opens only from the keyboard.
3. **The bar jumps** — the problem counts render nothing while validation runs, then reappear.
4. **Dropdowns cut off** — a select's list is as wide as its trigger, and the trigger is narrow in two-column grids.
5. **The opening view** — both canvases fit the content on load, so one plugin opens at 400 %.
6. **No theme switch on the canvases** — the canvases fill the window without the shell's header.

## 3. Plan

### 3.1 Shared (both canvases and the whole app)

- **Opening view**: 100 % centred on the content; fit stays a button and a shortcut.
- **Zoom controls**: one component, the app canvas's (minus, percent that resets to 100 %, plus, fit icon), on both.
- **Top bar in three parts**: identity at the start (back, breadcrumb, mark), the tools in the centre, status at the
  end (what is selected, save state, problem counts with a fixed width, the theme switch).
- **Dropdowns never cut off**: a select's list is at least as wide as its trigger and as wide as its longest option
  (up to the window); applies to every select in Studio.
- **Custom scrollbars** everywhere: thin, rounded, in the theme's colours.
- **File drop zone** (`FileDrop` in `@plux/ui`): drag and drop or click to choose, with an icon, a title, a hint and
  the accepted types; replaces every bare file input (assets, new app, licence, library import, component import,
  OpenAPI import, localisation import).

### 3.2 Plugin canvas

- **Top bar**: start — back to the app canvas, app › plugin breadcrumb with the plugin's mark; centre — the screen
  switcher (a labelled, readable menu of the screens with the current one marked, new screen, rename, move, delete),
  undo, redo, Preview (wider popover, one column for the device frame), history, pair, **Send to device** (primary)
  beside **Publish** (secondary); end — status (selection, save state), problem counts that keep their width and
  open the Problems tab, the theme switch.
- **Left dock**:
  - a header that names the editor (Plux mark, "Plugin editor", the plugin's name and its screens);
  - a bold rail on the primary colour with icon and label per pane, the open pane raised in white;
  - **Insert**: search; a collapsible **Categories** section (closed at first) of category tiles with icons and
    counts; collapsible sections per kind (open at first); **Recently used**; tiles that animate as the search or
    the category filters them (the home page's list animation) and show details on hover; click or drag to insert;
  - **Layers** (Layers, Screens and Selection in one): screens as roots (title, entry mark), widgets nested with icon,
    type and a content preview (a Text's words, a button's label), indentation guides, click the caret to fold,
    expand and collapse all, a filter, the canvas selection highlighted and revealed, multi-select, drag to reorder
    and reparent; a selection card on top when something is selected (count, types, quick actions);
  - **Assets**: a drop zone, search, kind filters (images, vectors, fonts, animations, other) with counts, a grid of
    cards with thumbnails or kind icons, size and type, delete; uploads with progress, resume and dismiss;
  - **Data** (State and Data in one): State and Data sources as collapsible sections with counts, rich rows (name,
    type, computed or mocked), add, and a way into each designer; new icons (State is no longer the JSON icon).
- **Right inspector**: nothing selected — the plugin overview (screens, entry screen, state, data, problems, last
  save) with quick actions; selected — the identity header; the editors as labelled tiles; the tabs; property
  sections with a visible breakpoint field and calmer nested editors.
- **Bottom console** (`STU-066`): always present between the docks as a slim bar (live dot, devices, problem
  counts, the tabs with counts), expanding into a resizable drawer: tabs with icons and counts; source device,
  levels, search, pause and resume, export; device controls (open a screen on the device, highlight, screenshot,
  pair, send); rows with time, level and source.

### 3.3 App canvas

- **Top bar** in the same three parts: identity (back, organisation › app); tools (select, hand, frame, auto
  layout, trash) and **New plugin**; status (selection, problem totals over the plugins, theme switch).
- **Left dock** with the same header and rail: **Plugins** (search, status filters, rich rows with hue mark, key,
  screens and status), **Frames** (list, rename, resize), **Overview** (counts, entry plugin, unpublished,
  errors, last publish).
- **Inspector**: the selected plugin as today, in the plugin inspector's structure; nothing selected — the app
  overview.
- **Context menu** on a plugin (`STU-034`): open, settings, duplicate, make entry, move to, archive, delete.
- **Owner** on the plugin box (`STU-030`); **native nodes** with a dashed outline and a native mark (`STU-033`;
  Plux Canvas gains dashed strokes).

## 3.4 Status (2026-11-03)

Implemented as planned, with these differences:

- **Opening view**: 100 % centred in the room the docks leave; on the plugin canvas the screens' tops sit under the
  top bar when they are taller than that room (centring tall screens hid their tops).
- **Insert details**: one details card at the foot of the pane follows the pointer and the focus; a hover card per
  tile (about seventy) made every edit of the canvas slower.
- **Bottom console**: the dev session still starts only when the console opens; closed, the bar shows its name,
  whether it streams, the problem count and the autosave time.
- **App canvas inspector**: the selected plugin's detail panel stays; the app's overview is the dock's **Overview**
  tab.
- **Send and Publish**: side by side at the end of the toolbar; Publish stays the filled, primary button (`STU-060`)
  and Send to device the secondary one (maintainer, 2026-11-04: keep the specification, fix the colours).

## 4. Decisions for the maintainer

- **Layer rename, hide and lock** (`STU-061`): nodes have no editor-only name, hidden or locked field
  (`common.schema.json#/$defs/node`). Storing them needs a schema addition (stop-and-ask, AGENTS.md §7). Until
  decided, the tree shows a node's content preview in place of a name and offers neither hide nor lock.
- **Screen names**: a screen with a translated title shows its key; resolving the translation needs the keys
  document in the canvas.

## 5. Out of scope here

The plugin canvas's screens, arrows, ghost screens and native nodes (the maintainer's next go-ahead).

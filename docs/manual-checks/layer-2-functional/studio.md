<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Studio

You sign in to Studio, build a new app (`shop`) with a plugin, screens, actions, data, translations,
fonts and tests, then publish it and read what it does. Everything later in layer 2 reuses what
this file leaves behind. Guides: [Studio guides](../../guides/studio/README.md).

Labels in **bold** are the interface's own; if a label differs in your build, note it in the result.

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-STUDIO | Browser signed in to Studio as R-ADMIN | L2-STU-001 |
| R-USER2 | Second user `dev-two@plux.localhost`, role `developer`, password you chose | L2-STU-004 |
| R-APP | New app `shop` (languages en, de, ar, am; environments as the wizard offers) | L2-STU-006 |
| R-APP-COPY | App `starter-copy`, imported from a `.plux` archive | L2-STU-008 |
| R-PLUGIN | Plugin `catalog` in R-APP | L2-STU-010 |
| R-SCREENS | Screens `list` and `detail` in R-PLUGIN | L2-STU-013 |
| R-FONTS | Two uploaded font files registered as host fonts | L2-STU-035 |
| R-TEMPLATE | A template saved from the `list` screen | L2-STU-037 |
| R-RELEASE1 | First release of R-APP, published to `development` | L2-STU-039 |
| R-SCENARIO | A Plux Test scenario in R-APP | L2-STU-049 |

### L2-STU-001 — Sign in and create R-STUDIO

- **Needs:** R-STACK, R-ADMIN
- **Do:**
  1. Browse to <http://localhost:8081>. You land on `/signin`.
  2. Sign in as `dev@plux.localhost` with `PLUX_DEV_PASSWORD` from `deploy/compose/.secrets/dev.env`.
  3. Try a wrong password once first.
- **Expect:** the wrong password gives one generic error; the right one opens the organisation home of `acme`.
- **Why:** sign-in is the only way in; errors must not reveal which half was wrong (SEC-100, STU-001).
- **Look at:** browser dev tools, Application, Cookies: a session cookie that JavaScript cannot read (HttpOnly); `curl http://localhost:8081/readyz` still answers 200.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-002 — The shell: organisation, palette, shortcuts, theme

- **Needs:** R-STUDIO
- **Do:**
  1. Open the organisation menu in the top bar; note **New organisation** (installation administrators only).
  2. Press `Mod+K`, type "settings", Enter. Press `Escape`.
  3. Press `Shift+?`. Press `Alt+T` twice. Press `Alt+H`.
- **Expect:** the palette lists commands and apps and jumps to your choice; the shortcut list opens; the theme switches and switches back; `Alt+H` returns home.
- **Why:** the shell is the one place every area is reached from (STU-010).
- **Look at:** the URL changes with each jump (`/o/<org>/…`); nothing reloads the page.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-003 — Profile: language, right-to-left, density

- **Needs:** R-STUDIO
- **Do:**
  1. Press `Alt+P` (route `/me`). Set the theme to **High contrast**, then back to **Match system**.
  2. Change the language to German, then Arabic, then English.
  3. Open `/me/shortcuts`.
- **Expect:** the whole interface changes language without a reload; Arabic mirrors the layout right to left; the shortcuts page lists every shortcut.
- **Why:** Studio is localised and keyboard-complete (STU-012, STU-011).
- **Look at:** the `dir` attribute on the `<html>` element (`rtl` in Arabic).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-004 — Invite a second user and create R-USER2

- **Needs:** R-STUDIO
- **Do:**
  1. Go to **Administration**, **Members** (`/o/acme/admin/members`). Invite `dev-two@plux.localhost` with the role `developer`.
  2. Copy the invitation link (it is shown once). Open it in a private window (`/invite/<token>`) and set a password.
  3. Keep that private window signed in as R-USER2.
- **Expect:** the invitation is shown once and cannot be shown again; the new user lands on the organisation home; R-ADMIN's Members list shows the user as active.
- **Why:** people join by invitation, never by a shared password (GOV-001, SEC-100).
- **Look at:** **Administration**, **Audit log** (`/admin/audit`): entries for the invitation and its acceptance.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-005 — Apps grid: read a card, search, sort, pin

- **Needs:** R-STUDIO, R-APP-DEMO, R-APP-STARTER
- **Do:**
  1. On the organisation home switch between **Grid** and **List**.
  2. Type `demo` in **Search apps**; clear it. Sort by **Name**, then **Last activity**.
  3. Open **Actions for demo** and choose **Pin demo**; reload the page.
  4. Read a card: environments, last publish, **Active devices**, **Plugins**, health badge, **Limits usage**.
- **Expect:** the choice, the search and the pin survive a reload; the pinned app is first in every order.
- **Why:** the grid is the entry to every app (STU-040).
- **Look at:** the **Limits usage** meters: a badge appears from 80 percent of a limit.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-006 — New-app wizard, blank: create R-APP

- **Needs:** R-STUDIO
- **Do:**
  1. On the organisation home choose **Create app** (`/o/acme/apps/new`).
  2. **Start from**: **Blank app**. **Details**: name `Shop`, key `shop`. **Languages**: English (default), German, Arabic, Amharic.
  3. **Theme**: any. **Security**: leave the default. **Environments**: keep what is offered. Choose **Create app**.
- **Expect:** the **Done** step appears and you land on the new app's canvas with one plugin that has an empty entry screen. **From an AI prompt** is shown disabled.
- **Why:** an app starts from one guided path with safe defaults (STU-041, GOV-010).
- **Look at:** `curl -s http://localhost:8081/livez`; the server log line for `AppService.CreateApp`; the audit log entry.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-007 — Wizard from a template, then export a `.plux` archive

- **Needs:** R-STUDIO, R-APP-STARTER
- **Do:**
  1. Start **Create app** again, choose **From a template**, pick one, key `tmpl-try`, finish.
  2. Back on the home, open **Actions for starter**, choose **Export .plux archive**; save the file.
  3. Open **Actions for tmpl-try** and **Archive** it.
- **Expect:** the template copy opens with pages in it; a `.plux` file downloads; the archived app disappears from **Active** and shows under **Archived**.
- **Why:** apps are portable and nothing is deleted by accident (STU-041, GOV-031).
- **Look at:** `unzip -l <file>.plux`: documents in the Git layout.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-008 — Wizard from an archive: create R-APP-COPY

- **Needs:** R-STUDIO, the `.plux` file of L2-STU-007
- **Do:**
  1. **Create app**, **From a .plux archive**, choose the file. First try a file named `x.txt`.
  2. On **Details** set name `Starter copy`, key `starter-copy`.
- **Expect:** `x.txt` is refused ("The file name must end in .plux."); the archive is accepted; the copy opens with the same plugins as `starter`.
- **Why:** an archive is checked before anything is created (STU-041, LIM-001).
- **Look at:** the **Plugins** count on both cards is equal.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-009 — Duplicate, archive, unarchive

- **Needs:** R-APP-COPY
- **Do:**
  1. **Actions for starter-copy**, **Duplicate**, name it `Copy two`, key `copy-two`.
  2. **Archive** `copy-two`; choose **Archived** in the filter; **Unarchive** it.
- **Expect:** the duplicate opens at once; archived apps carry an **Archived** badge and no statistics; unarchiving returns it to **Active**.
- **Why:** lifecycle actions are reversible (GOV-031).
- **Look at:** **Administration**, **Audit log**: one entry per action.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-010 — App canvas: add the plugin R-PLUGIN

- **Needs:** R-STUDIO, R-APP
- **Do:**
  1. Open R-APP (`/o/acme/apps/shop`). Read the **App canvas**: a card per plugin.
  2. Choose **New plugin**, name `Catalog`, key `catalog`, save.
- **Expect:** a second card appears; the cards show screens, versions and links in and out.
- **Why:** an app is a set of plugins with visible links (STU-050).
- **Look at:** `curl -sk -H "Authorization: Bearer $PLUX_DEV_TOKEN" -H 'Content-Type: application/json' -d '{"appId":"<id>"}' https://localhost:8080/plux.v1.PluginService/ListPlugins` (see L2-SRV-002 for the variables).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-011 — Arrange the plugin graph

- **Needs:** R-APP, R-PLUGIN
- **Do:**
  1. Drag `catalog` to a new place. Use **Move to…** with X and Y.
  2. Select both cards, **Frame selection**, rename the frame.
  3. Choose **Arrange**, then **Zoom to fit**. Reload.
- **Expect:** positions, the frame and the arrangement persist after the reload.
- **Why:** layout is part of the app document, shared by every editor (STU-051).
- **Look at:** **JSON editor** of the app: the stored positions.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-012 — Plugin settings, archive, trash, restore

- **Needs:** R-APP, R-PLUGIN
- **Do:**
  1. Open **Plugin settings** of `catalog`: add a description and a tag; **Duplicate plugin** (key `catalog2`).
  2. **Archive plugin** `catalog2`, then **Restore plugin**.
  3. **Delete plugin** `catalog2` and find it in the **Trash**; restore it; delete it again.
- **Expect:** each action works; a deleted plugin stays in the **Trash** for 30 days and restores with its links.
- **Why:** deletion is recoverable (GOV-031).
- **Look at:** the audit log entries.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-013 — Plugin canvas and screens: create R-SCREENS

- **Needs:** R-APP, R-PLUGIN
- **Do:**
  1. Double-click `catalog`. Note the panels **Insert**, **Layers**, **Screens**, **Selection**, **Assets**.
  2. Choose **New screen**: `list` (make it the entry screen), then `detail`.
  3. **Rename screen** `detail` to `item`, then back to `detail`.
- **Expect:** screens appear in navigation order; the route follows the name; nothing is left in **Problems**.
- **Why:** routes are addresses other plugins and the host use.
- **Look at:** the **Problems** drawer is empty; the JSON editor shows the `routes` entry.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-014 — Insert widgets and edit properties

- **Needs:** R-SCREENS
- **Do:**
  1. On `list` open **Insert**, **Widgets**: add a column, a text, a button and an image.
  2. Select the text; in **Selection** edit its text, size and colour (choose a design token).
  3. Press `Mod+Z` three times, then `Mod+Shift+Z` three times.
- **Expect:** the canvas shows the changes at once; undo and redo walk back and forth through them.
- **Why:** direct manipulation with undo is the main editing loop (STU-052, STU-060).
- **Look at:** the saved state: reload and the edits are there (autosave).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-015 — Selection menu: wrap, unwrap, duplicate

- **Needs:** R-SCREENS
- **Do:**
  1. Select the button, open the selection menu: **Wrap selection in a column**, then **Unwrap selection**.
  2. **Duplicate selection**.
  3. Drag a widget from **Layers** to reorder it.
- **Expect:** the tree in **Layers** and the canvas agree after each step.
- **Why:** structure edits are first-class operations (STU-053).
- **Look at:** the **Layers** panel numbering.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-016 — Preview switches

- **Needs:** R-SCREENS
- **Do:**
  1. Switch the **Device frame**, then **Light** and **Dark**.
  2. Switch **Locale** to `ar` (right-to-left) and `de`; set a large **Text scale**.
  3. Switch **Material** and **Cupertino**; switch the data state through **Success**, **Loading**, **Empty**, **Error**.
- **Expect:** the screen redraws for each choice; `ar` mirrors the layout; the data states show the matching states.
- **Why:** you see the device result before publishing (STU-005, STU-054).
- **Look at:** compare the text wrap in `de` against `en`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-017 — Device matrix

- **Needs:** R-SCREENS
- **Do:** open the **Device matrix** of `list` and read the frames side by side.
- **Expect:** several device sizes are drawn at once from the same document.
- **Why:** layout differences between devices are found at design time (STU-055).
- **Look at:** a long text that wraps differently on the small frame.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-018 — Link screens

- **Needs:** R-SCREENS
- **Do:**
  1. Select the button, open **Actions**; choose the event (tap), **Target screen** `detail`, **Add link**.
  2. Drag the link handle of the image to `detail` to make a second link.
  3. Delete `detail` and read what Studio says; cancel.
- **Expect:** arrows appear between the screens; Studio refuses to delete a screen another screen navigates to.
- **Why:** navigation is explicit and checked (STU-056).
- **Look at:** the arrow from the button ends at `detail`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-019 — Action graph

- **Needs:** R-SCREENS
- **Do:**
  1. In **Actions**, open the **Action graph** for the button's tap.
  2. Add a built-in step (for example a navigation step), a state-set step and connect their ports.
  3. Add a deliberately unconnected step and read **Problems**.
- **Expect:** the graph draws; the unconnected step is reported with its path; removing it clears the problem.
- **Why:** declarative actions replace most code (ACT-001, STU-057).
- **Look at:** the **JSON editor**: the `actions` document that mirrors the graph.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-020 — State designer

- **Needs:** R-PLUGIN
- **Do:**
  1. Open the **State designer**. **Add state entry**: `cart.count`, number, default 0.
  2. Add a second entry that is stored in secure storage and tagged sensitive.
  3. Bind the text of `list` to `cart.count` with a PXL expression (next check).
- **Expect:** the entries are listed with their type and persistence; the validator accepts them.
- **Why:** state has a declared type and scope (STA-001, SCH-012).
- **Look at:** the sensitive entry carries a marker in the list and in JSON.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-021 — PXL expression editor

- **Needs:** R-SCREENS, state from L2-STU-020
- **Do:**
  1. Select the text. In **Selection**, change its text property from a fixed value to **PXL expression**, **Type expression**.
  2. Type `state.cart.count`, then a broken expression such as `state.cart.`.
- **Expect:** a valid expression is accepted and the canvas shows the value; the broken one shows a message while the validator checks it, and cannot be applied.
- **Why:** expressions are type-checked before publish (PXL-001).
- **Look at:** the same validator the server runs (**Problems**).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-022 — JSON editor with comparison

- **Needs:** R-SCREENS
- **Do:**
  1. Open the **JSON editor** for `list`; read the comparison against the last published version.
  2. Edit a text value and choose **Apply**.
  3. Break the JSON (delete a closing brace) and try **Apply**.
- **Expect:** the valid edit applies and shows in the canvas; the broken one is refused and nothing is written.
- **Why:** the JSON is the truth; the editor writes only valid documents (STU-058, SCH-001).
- **Look at:** the diff colouring; the canvas stays on the last valid state.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-023 — Problems drawer

- **Needs:** R-SCREENS
- **Do:**
  1. Open **Problems**. Set a required property to the wrong type in the JSON editor.
  2. Select the problem.
- **Expect:** an error with a `PLX-` code, a JSON path and a fix hint; selecting it selects the node on the canvas.
- **Why:** diagnostics point at the exact place (CMP-040).
- **Look at:** the code in [the error reference](../../reference/errors.md).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-024 — Editing locks between two people

- **Needs:** R-STUDIO, R-USER2 (private window), R-PLUGIN
- **Do:**
  1. As R-ADMIN open `catalog` and edit something.
  2. As R-USER2 open `catalog`: you can look, select and copy but not edit. Choose **Request edit**.
  3. As R-ADMIN stop editing; as R-USER2 choose **Take over**.
- **Expect:** R-USER2 is read-only while R-ADMIN holds the lock; after the hand-over R-USER2 edits and R-ADMIN becomes read-only.
- **Why:** one writer per plugin keeps drafts consistent (SRV-040).
- **Look at:** the server log of `PluginService` lock calls; the snapshot "lock taken over" in **History**.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-025 — Conflict handling

- **Needs:** R-PLUGIN, a second browser tab
- **Do:**
  1. Open `catalog` in two tabs as R-ADMIN. Change the same text in both, tab A first, then tab B.
  2. Read the conflict message in tab B.
- **Expect:** tab B offers **Keep mine**, **Take the server's** and **Copy mine as JSON**; each does what it says.
- **Why:** concurrent writes never silently overwrite (SRV-030).
- **Look at:** a revision mismatch (`if_revision`) in the server log.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-026 — History and restore

- **Needs:** R-PLUGIN
- **Do:** open **History**, pick two snapshots to compare, then **Restore** an older one.
- **Expect:** the list shows edit, lock-takeover and publish snapshots; the comparison shows the differing paths; restore returns the documents.
- **Why:** every state is recoverable (SRV-031).
- **Look at:** a new "restored" snapshot appears.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-027 — Data sources: import an API definition

- **Needs:** R-APP, R-PLUGIN, the file `backend/internal/importer/testdata/petstore31.yaml`
- **Do:**
  1. Open **Data** (`/o/acme/apps/shop/data`), choose plugin `catalog`.
  2. **Import an API definition**, choose the file, review the list, **Apply**.
  3. Open a source: read **Operations**, **Schemas**, **Base URL**. Choose **Through the Plux gateway**.
- **Expect:** sources appear only after **Apply**; a construct with no Plux type is reported and left out.
- **Why:** typed data comes from API definitions (DAT-002, DAT-003).
- **Look at:** `plux import openapi` gives the same fragment (L2-CLI-014).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-028 — Mocks and data states

- **Needs:** L2-STU-027
- **Do:** open an operation, **Edit the mock**, change a value, **Use this mock**; try invalid JSON. Switch the **Data state** on the canvas.
- **Expect:** the canvas shows the mock; invalid JSON is refused; the other states preview.
- **Why:** pages are designed without a live backend.
- **Look at:** the `mocks` part of the source in the JSON editor.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-029 — On-device collections

- **Needs:** R-PLUGIN
- **Do:**
  1. Open **Collections** (`/data/collections`); add collection `items` with key, primary key `id`, an index and fields; mark one field sensitive.
  2. Change the fields and read **Migration plans**; choose **Preview the migration**.
- **Expect:** the designer validates keys and indexes; the preview says what a migration does to existing data.
- **Why:** the local database is designed and migrated declaratively (DB-001, DB-005).
- **Look at:** the sensitive field's marker.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-030 — Localisation grid and heatmap

- **Needs:** R-APP, R-PLUGIN
- **Do:**
  1. Open **Localisation** (`/localisation`), the **Grid**. In the plugin canvas add keys `list.title` and `list.empty` in `en` (**Localisation table**, **Add key**).
  2. Back in the grid choose locale `de`, filter **Missing**; open **Heatmap** (`/localisation/heatmap`).
- **Expect:** the new keys show **Missing** for de, ar, am; the heatmap says "2 of N missing" for them.
- **Why:** gaps are visible before release (STU-070).
- **Look at:** the **Status** column values.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-031 — Translate, approve, import, export, add a locale

- **Needs:** L2-STU-030
- **Do:**
  1. Type German texts for both keys; select the rows, **Mark as needs review**, then **Approve**.
  2. **Export translations** as XLIFF; edit one text; **Import translations** it into `de`.
  3. **Add a locale** `fr`.
- **Expect:** Studio states how many messages change and asks to confirm; the import changes only that text; `fr` is added with every key missing.
- **Why:** translation is a reviewed workflow.
- **Look at:** the **Status** moves to *Approved*; only users with `translations.approve` see **Approve** enabled.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-032 — Move literal text to a key

- **Needs:** R-SCREENS
- **Do:** select the button on `list`, type a literal label, choose **Move to a translation key**, name it `list.buy`.
- **Expect:** the literal becomes a key reference and the text moves to the default locale.
- **Why:** no literal text ships untranslatable (I18N-001, STU-071).
- **Look at:** the new key in the grid.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-033 — Design tokens

- **Needs:** R-APP
- **Do:**
  1. Open **Design** (`/design`), **Tokens**. **Edit** a colour token's **Light**, **Dark**, **High contrast** values.
  2. **Add token** `color.accent2`.
  3. **Import tokens** with a small DTCG JSON file you write, containing one valid and one invalid token.
- **Expect:** `list` restyles at once; the import reports how many tokens can be added and why the other is skipped.
- **Why:** pages use tokens, never fixed values (THM-001).
- **Look at:** the preview in Dark and High contrast.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-034 — Themes and a brand overlay

- **Needs:** R-APP
- **Do:** open **Themes** (`/design/themes`), read the colours per mode, **Create a brand overlay** replacing one token.
- **Expect:** the overlay changes only that token for that brand.
- **Why:** one app serves several brands (THM-004).
- **Look at:** the overlay document in the JSON editor.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-035 — Typography and host fonts: create R-FONTS

- **Needs:** R-APP; two font files (`.ttf` or `.otf`), for example one Latin and one Arabic
- **Do:**
  1. Open **Assets** in the plugin canvas and upload both files.
  2. Open **Typography** (`/design/typography`). Set a font family list for Latin and Arabic with fallbacks.
  3. Under **Host app fonts** enter **Font family**, pick the **Font file**, **Add font**; repeat for the second.
- **Expect:** both fonts are listed; the canvas draws text in them.
- **Why:** measurements on the canvas must match the device (STU-005, CMP-032).
- **Look at:** the fonts in `host.fonts` of the app document.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-036 — Icons

- **Needs:** R-APP
- **Do:** open **Icons** (`/design/icons`); **Add icon** with a name and an asset source; use it from a widget.
- **Expect:** the icon appears in the icon list and on the canvas.
- **Why:** icon sets are part of the design system.
- **Look at:** the name resolves in the **Problems** drawer (no unknown-icon error).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-037 — Library: component and template, create R-TEMPLATE

- **Needs:** R-SCREENS
- **Do:**
  1. Select the column on `list`; extract a component; save the screen as a template.
  2. Open **Library** (`/library`); open the component (`/library/components/<id>`) and read **Versions** and **Instances across apps**.
  3. Insert the template on `detail`.
- **Expect:** the template lists what it uses (tokens, assets, components) as references; page-state bindings become parameters; the instance count increases.
- **Why:** reuse across apps without copying content (SCH-031, STU-080).
- **Look at:** the template's parameter list in step two of the dialog.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-038 — Shell settings

- **Needs:** R-APP
- **Do:**
  1. Open **Settings**, then shell settings (`/settings/shell`). Change **App name**.
  2. Read **Shell update required** and the table **What changed**.
- **Expect:** the page names each changed setting and why it needs a new store build.
- **Why:** what lives in the store binary is separated from what ships by release (GEN-004).
- **Look at:** the status word changes from "No project generated yet" after L2-CLI-012.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-039 — Publish with visual diff: create R-RELEASE1

- **Needs:** R-APP, R-SCREENS
- **Do:**
  1. Choose **Publish** (`/publish`). **Select plugins**: the app and `catalog`.
  2. **Freeze and build**; wait for *Built*. Read **Reports**: validation, **Size**, **Accessibility**.
  3. Read **Visual diffs** and **Changed documents**; write a **Changelog**.
  4. **Target**: environment `development`, its channel. Continue to **Release**.
- **Expect:** each step completes; the first publish has nothing to compare against; a release is created.
- **Why:** publishing is a pipeline of validate, compile, sign, store (SRV-050, REL-001).
- **Look at:** server log `publish` job lines; metric `plux_publish_jobs_total` in Prometheus.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-040 — Visual diff on a second publish

- **Needs:** R-RELEASE1
- **Do:** change the text colour of `list`, publish again to `development`.
- **Expect:** **Visual diffs** show before and after images of `list` only; **Changed documents** lists the changed JSON path.
- **Why:** reviewers see what changes, not just JSON.
- **Look at:** the diff image of the unchanged `detail` screen is absent.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-041 — Releases timeline and detail

- **Needs:** R-RELEASE1
- **Do:** open **Releases** (`/releases`), pick `development`, open the newest release.
- **Expect:** the list is newest first with channel and notes; the release shows **Content**, **Changelog**, **Bundle SHA-256**, **Adoption** and health.
- **Why:** a release is immutable and inspectable (REL-001).
- **Look at:** the hash equals the one `plux pull` prints later (L2-CLI-013).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-042 — Roll back from Studio

- **Needs:** R-RELEASE1 and the second publish
- **Do:** on the first release choose **Roll back to this release**; confirm.
- **Expect:** a new release (higher sequence) with the first release's content; the channel points to it.
- **Why:** rollback never goes back in sequence (REL-006).
- **Look at:** `plux release list --env development` shows the new sequence.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-043 — Approvals pages

- **Needs:** R-STUDIO
- **Do:** open **Approvals** (`/approvals`): **Inbox**, **History**, **Break-glass reviews**, **Policies**.
- **Expect:** all four pages load, empty or with the records of earlier checks.
- **Why:** the approval engine is reachable from the shell (GOV-020).
- **Look at:** the policy defaults written for the app's security profile.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-044 — Experiments: create from a selection

- **Needs:** R-SCREENS
- **Do:**
  1. Select the button on `list`; choose **Create experiment from selection**.
  2. Walk **Target**, **Variants** (control plus one), **Allocation** (50/50), **Targeting**, **Metrics**, **Schedule**.
  3. **Create experiment**. Try allocation 60/50 first.
- **Expect:** 60/50 is refused ("must add up to 100"); the experiment is a *Draft*; **Start experiment** moves it to *Running*; **Stop experiment** stops it.
- **Why:** experiments are first-class and bounded (LIM-001).
- **Look at:** `plux experiment list --env development`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-045 — Feature flags

- **Needs:** R-APP
- **Do:** open **Experiments**, **Feature flags** (`/experiments/flags`); add flag `new-checkout`, **Type** Boolean, **Default value** off; **Save flag**; **Edit**; **Delete**.
- **Expect:** the flag appears with its type and default and disappears on delete.
- **Why:** flags change behaviour without a release.
- **Look at:** `plux flags get --env development new-checkout` while it exists.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-046 — Analytics overview, usage and performance

- **Needs:** R-APP-STARTER, R-DEVICE (consent switched on in the app)
- **Do:** in the starter app tap around with consent on; in Studio open **Analytics** of `starter` (overview), **Usage**, **Performance**; set **From** and **To** to today.
- **Expect:** sessions and screen views appear (after the telemetry delay); **Usage** breaks views down by plugin then page; **Performance** shows render-time percentiles.
- **Why:** you see how the app is used from device events (ANL-001, STU-100).
- **Look at:** [Telemetry backlog](../../runbooks/telemetry-backlog.md) if nothing appears after five minutes.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-047 — Funnels

- **Needs:** L2-STU-046
- **Do:** open **Funnels**, **Create funnel** with two event names, one per line; open it; **Delete**.
- **Expect:** sessions per step and the share of the first step.
- **Why:** product questions are answered without third-party tools (ANL-010).
- **Look at:** step counts fall or stay level, never rise.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-048 — Errors and replays

- **Needs:** L2-STU-046
- **Do:**
  1. Open **Errors** (`/analytics/errors`); open a group if one exists and use **Open node** from a symbolicated path.
  2. Open **Replays** (`/analytics/replays`), open one, press **Play**, move the **Timeline**.
- **Expect:** an error goes to its widget on the canvas; a replay shows screens and state, with unrecorded values **Hidden**.
- **Why:** crash to widget, and structural replay without pixels (ANL-020, ANL-030).
- **Look at:** **State so far** next to the **Replay canvas**. Nothing listed means the app records no replays here; write that down.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-049 — Plux Test: write R-SCENARIO

- **Needs:** R-SCREENS
- **Do:**
  1. Give the button a **Test ID** `buy`. Open **Plux Test** (`/tests`), **New scenario** with **File** `buy-flow`, route of `list`.
  2. **Add step**: **Tap** `buy`. **Add expectation**: **Navigated to** `detail`. **Save scenario**.
- **Expect:** the scenario saves; a step without a test ID is refused.
- **Why:** behaviour is tested declaratively (TST-001, STU-110).
- **Look at:** `tests/buy-flow.scenario.yaml` in `plux export` (L2-CLI-010).
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-050 — Plux Test runs reported by the CLI

- **Needs:** R-SCENARIO, built `plux` (L2-CLI-001)
- **Do:** from the exported project run `plux test --report --app <id> --label "check run"`; open the run in **Plux Test**, **Runs**.
- **Expect:** a run with result and duration; the scenario's steps with a failed step's message if any.
- **Why:** CI results land beside the design (TST-004).
- **Look at:** exit code 0 of the command; `--junit` file if you add it.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-051 — Usage and limits

- **Needs:** R-APP
- **Do:** open app **Settings**; read **Limits usage** and **Plugin usage**; open **Administration**, **Limits** (`/admin/limits`).
- **Expect:** each limit shows what it uses, the limit in force and a meter; **Plugin usage** lists per-plugin usage (the data of `PluginService/ListPluginLimits`).
- **Why:** limits come from the registry and are visible (LIM-001, LIM-005).
- **Look at:** the plugins count equals the cards on the canvas.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-052 — Notifications

- **Needs:** R-STUDIO
- **Do:** click the bell (`/notifications`); mute one type.
- **Expect:** unread notifications are grouped by type; the mute is kept.
- **Why:** people hear about decisions that wait for them.
- **Look at:** a notification for the publish or invitation of earlier checks.
- **Result:** ☐ pass ☐ fail — notes:

### L2-STU-053 — Sign out

- **Needs:** R-STUDIO
- **Do:** sign out; press Back; open `/o/acme`.
- **Expect:** `/signed-out` shows; protected pages redirect to `/signin`. Sign in again to keep R-STUDIO.
- **Why:** sign-out ends the server session, not just the page (SEC-100).
- **Look at:** the old cookie is refused: reuse it with `curl -b` and expect 401.
- **Result:** ☐ pass ☐ fail — notes:

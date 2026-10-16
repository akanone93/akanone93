# Studio UI Language — "Daylight"

How Plux Studio looks and behaves: colour, shape, type, motion, and every area page by page. [studio-development.md](studio-development.md) says how Studio is built; this document says what it should feel like and why.

**In one line:** a calm, white workspace where the evidence is always one glance away.

Studio's users include banks, ministries and regulated companies. They do not buy excitement; they buy the feeling that nothing can go wrong unnoticed. Studio's creativity therefore goes into clarity, trust and flow, not decoration: a quiet, well-lit workspace, few colours, everything in its place, and a handful of careful details people notice on day three, not on first sight.

The direction follows `STU-010` (light and whitish, 12–20 px radii, floating panels with soft shadows, colour for meaning), `STU-011` (WCAG 2.2 AA), `STU-012` (English, German, Arabic, with RTL) and the brand tokens of spec Appendix J (`STU-016`).

---

## 1. Principles

1. **White is the material.** Studio is about 85% white and near-white. Colour is rare, so when it appears it means something.
2. **One accent, one meaning.** Plux Violet means *you*: your selection, your focus, the one primary action on the screen. Nothing else is violet.
3. **Evidence is never hidden.** Who changed it, who approved it, what is signed, which environment — always visible in one quiet line, and a click away in full.
4. **Calm by default, dense on demand.** Comfortable spacing for occasional users, compact density for power users, the same layout in both.
5. **No surprises.** Destructive and production actions look and feel different from everyday actions, every time, everywhere.

## 2. Colour

### Light (default)

| Token | Value | Use |
|---|---|---|
| `bg.app` | `#F7F8FB` | Page and canvas background (`brand.surface`) |
| `bg.panel` | `#FFFFFF` | Panels, tables, dialogs, cards |
| `bg.sunken` | `#EFF1F6` | Inputs at rest, table headers, code blocks |
| `text.primary` | `#0F1222` | Body text, 18.6:1 on white (`brand.ink`) |
| `text.secondary` | `#4F5568` | Labels and metadata, 7.4:1 |
| `text.tertiary` | `#80869A` | Placeholders, disabled text, and input borders, which must reach 3:1 (3.6:1) |
| `line.subtle` | `#E3E6ED` | Dividers and panel edges; decorative only |
| `accent` | `#5B3DF5` | Selection, focus ring, primary button, 6.1:1 on white (`brand.violet`) |
| `accent.subtle` | `#EEEAFE` | Selected rows, active tab (`brand.violet.subtle`) |

### Status

Each status is a soft fill or icon with dark text, never coloured text on white alone.

| Status | Icon and fill | Text on white | Meaning |
|---|---|---|---|
| Success | `#12B886` (`brand.mint`) | `#0A7A59`, 5.3:1 | Published, approved, healthy |
| Warning | `#F59F00` (`brand.amber`) | `#8F5B00`, 5.7:1 | Pending, unpublished changes, near a limit |
| Danger | `#D92D20` | `#C8281C` | Errors, blocked actions, production-destructive actions |
| Neutral | `text.secondary` | — | Everything else; Studio needs no fourth colour |

### Dark

The same structure on `#0B0D17` (`brand.surface` dark) with panels `#141726`, using Appendix J's dark values — text `#F5F6FA`, violet `#8B74FF`, mint `#38D9A9`, amber `#FFC53D` — and danger `#FF7066`. Every text pair reaches at least 5:1.

### Environment colour

Every screen shows which environment the user acts on, as a 2 px band along the top edge of the window and a label in the action bar:

| Environment | Band | Label |
|---|---|---|
| Development | none — the normal state | none |
| Staging | amber | `STAGING` |
| Production | violet | `PRODUCTION`, next to every action that reaches it |

"I didn't know I was in production" must be impossible.

## 3. Shape, depth and space

- **Radius:** 12 px for controls, 16 px for panels and cards, 20 px for large dialogs and overlays. Tables and their rows stay square inside their rounded container: data reads as data.
- **Depth:** three levels only.
  - *Flat* — pages and tables.
  - *Raised* — floating panels and cards, with a soft ink-tinted shadow (`0 1px 2px` at 4% and `0 8px 24px` at 6%).
  - *Overlay* — dialogs and full-screen editors, with a stronger shadow and the content behind dimmed to 40%.
  Nothing else casts a shadow.
- **Space:** a 4 px grid. Comfortable density: 44 px rows, 16 px panel padding. Compact density: 32 px rows, 12 px padding. Data pages cap at 1,440 px and centre on wide screens.
- **Lines over boxes.** Inside panels, sections are separated by space and one hairline, not by nested boxes.

## 4. Typography

- **One family** for the whole interface: a modern humanist sans with a matched Arabic design, so English, German and Arabic read as one product.
- **Mono** only for IDs, versions, hashes, keys, code, and numbers in tables (tabular figures).
- **Scale:** 12 medium (labels), 14 regular (body), 16 semibold (section titles), 20 semibold (page titles), 28 semibold (the rare large figure on a dashboard). No display serifs and no oversized hero text.
- **Sentence case** everywhere; uppercase only for environment labels and short status tags.
- **The wordmark** is Sora SemiBold (Appendix J), used in the top bar and on the sign-in page only.

## 5. Signature details

1. **The evidence line.** Under the title of every app, plugin, screen, release, function and policy:

   `Edited by Hanna Bekele · 12 min ago · Draft · 2 problems · Last published v14 by Abel, approved by Sara`

   Each part is a link. Clicking the line opens the **Evidence panel**: the full chain of changes, approvals with their WebAuthn time, the signing key's fingerprint and the audit entries for that object.

2. **Plain-language change summaries.** Before publishing or approving, Studio states the change in words, generated deterministically from the diff, not by AI:

   > 3 screens changed in Loans, 1 navigation link added, function `fee` updated to v15. No security-relevant changes.

   Security-relevant changes — secure pages, functions, permissions — get their own line with the danger icon.

3. **One pattern for production actions.** Every action that reaches production uses the same pattern: a button outlined in the environment colour, a confirmation that names the environment, the object and the impact, type-to-confirm (`STU-074`), and WebAuthn where required (`SEC-103`). Releases, the kill switch, rollback and deletion all use it, so the habit becomes "outlined, production-coloured button: slow down".

4. **Focus mode.** `F` on the canvas dims everything except the selection and what it connects to — incoming and outgoing links and the screens they lead to — so one flow stands out in a 100-screen plugin.

5. **Status is a word and an icon.** `● Published`, `◐ Changes not published`, `◷ Waiting for approval`, `✕ Failed` — readable in greyscale, in printed audit exports and by colour-blind users.

6. **Shortcuts in tooltips.** Every tooltip ends with its shortcut in a small key cap, so the keyboard is learnt without a manual.

7. **Undo where the change happened.** After a change, an "Undo" link appears for 8 seconds at the place of the change, not in a toast in a corner.

## 6. Motion

- 150–200 ms, ease-out; no bounce, no scaling pop-ins.
- Panels slide 8 px and fade; overlays rise 16 px and fade; arrow bundles expand smoothly (`STU-044`).
- Loading: static light-grey placeholder shapes, no shimmer. Long jobs show a thin progress bar with the step in words: *Validating · Compiling · Signing · Uploading*.
- With reduced motion, every transition is an instant change.

## 7. Areas, page by page

### Shell

- **Top bar** (56 px, white): the wordmark, the organisation switcher, global search (`⌘K`); at the end, notifications, help and the avatar.
- **Navigation rail** on the start side (64 px, icons with labels): Home, Apps, Releases, Approvals, Experiments, Functions, Localisation, Design system, Data, Analytics, Devices, Admin. It expands to 240 px on hover or when pinned.
- **The rail shows only what the user's role can use.** A Translator sees Localisation, Apps (read-only) and Approvals; an Auditor sees Releases, Approvals, Analytics and the audit log.

### Sign-in

One centred white card on `bg.app`: the logo, organisation SSO first, then account sign-in with MFA. The language switcher at the bottom names each language in its own script — English · Deutsch · العربية. A quiet line shows the installation name and, when self-hosted, its version and "Self-hosted · your data stays on your servers".

### Home

Two columns. **Needs your attention:** approvals waiting for the user, their drafts with problems, failed rollouts, devices about to expire — each with one action. **Recent work:** the last eight apps, plugins and screens opened, with thumbnails. The first-run checklist (`STU-015`) is a calm four-step card at the top, dismissed once done. No vanity charts: Home is for action.

### Apps grid (`STU-020`–`STU-025`)

Cards: the plugin-canvas snapshot on the top 60%; below it the name, team and description; environment chips with live versions (`Dev v18 · Staging v17 · Prod v15`); one health figure (`99.94% crash-free`) with a trend arrow; a limit warning only above 80% of a limit. The toolbar has search, filter chips, sort and a grid/list toggle; the list view is a sortable data table for people who manage forty apps.

### App canvas (`STU-030`–`STU-048`)

`bg.app` with a barely visible dot grid. Plugins are raised white cards (16 px radius) with icon, name, screen count, version and status; native routes are dashed cards tagged "Native". Arrows are 1.5 px in `text.tertiary` with the count in a small white pill; on hover a bundle expands into labelled lanes and turns violet, and everything unrelated fades to 30%. Frames (`STU-035`) are lightly tinted titled regions. A minimap and zoom controls sit in the bottom corner.

### Plugin canvas and screen editor (`STU-050`–`STU-069`)

- **Device frames** have thin light-grey bezels so the content dominates; the screen name sits above each frame, with problems as a small danger count.
- **Ghost screens** are 40% opacity with a dashed outline and an "Opens Cards › Card details" label.
- **Action bar** (floating at the top centre): undo and redo, zoom, device, theme, locale, text scale, send to device, problem count, history, AI assistant, and **Publish** — the only filled violet button on the screen.
- **Panels** float on both sides (16 px radius), with text-labelled tabs.
- **Properties** are grouped and collapsible; defaults show in `text.tertiary`, changed values in `text.primary` with a "changed from default" dot; token-bound values show the token name and a swatch.
- **Preview chip.** When the preview is not the default, a chip under the action bar says so — `Previewing: العربية · Dark · 200% · Error state ✕` — and resets it in one click.
- **Overlays** (JSON, Rust, action graph, localisation table, theme and the others) are full-screen white sheets with a title bar and "Back to canvas"; the canvas stays paused behind them (`STU-063`).
- **Bottom drawer** rests as a 32 px status strip with the device connection and problem count; opened, it shows Problems, Logs, Process flow, State, Network, Functions, Sync, Performance and Console. Process flow is a vertical timeline of action runs with step timings.

### Publish flow (`STU-100`)

A stepped dialog, steps on the start side and the current step beside them. The summary step leads with the plain-language change summary, then the validation, accessibility and budget reports as pass, warn and fail rows. The visual diff offers side by side and a before/after slider. The last step states exactly what will happen: *Release 1.6.0 will be submitted for approval to Production. Approvers: Release Manager, Compliance Officer.*

### Releases (`STU-101`)

One row per environment, each a timeline of releases as milestones with version, date, adoption and health. Rollout controls — pause, resume, advance, schedule — sit on the active release. The kill switch has its own separated card at the end, labelled in the danger colour and using the production-action pattern.

### Approvals (`STU-102`)

The inbox is a table — change, requested by, environment, risk, waiting time — sortable by urgency. The detail view leads with the plain-language summary, then the evidence (diff, reports, matched policies), approver slots showing who has signed and who is still needed, and Approve and Reject at the bottom; a rejection requires a comment. The policy simulator is a short form that answers in one sentence.

### Experiments (`STU-103`)

The list shows status, allocation and the primary metric with its confidence. The detail view shows variants side by side, with the result as a sentence above the chart: *Variant B is ahead by 2.1% (95% confidence).*

### Functions (`STU-110`)

A table of name, version, aliases, assurance level, placement (server or device), p99 latency and error rate. The Rust editor overlay places the test panel beside the code, with memory, time and fuel shown as usage bars from the last test run.

### Localisation (`STU-111`)

A spreadsheet-like grid: keys in rows, locales in columns, Arabic columns right-aligned and right-to-left. Each cell carries a status icon; missing cells have a light warning fill. Filters: missing only, needs review, by plugin. Each key shows its canvas screenshot, description and maximum length, with a live counter while typing.

### Design system (`STU-112`)

Tokens as swatch rows — name, value per theme, contrast ratio with a pass or fail mark. Typography as sample lines per script. Components as documentation pages with live examples, states and usage notes.

### Data (`STU-113`)

API definitions as tables per environment: endpoint, method, authentication, mock state. An OpenAPI import previews what it will create before it is confirmed.

### Analytics (`STU-114`)

A few large figures at the top — crash-free rate, active devices, adoption of the latest release, p95 render time — then charts in one main colour (`text.primary` at 80%), with violet only for the highlighted series and releases as thin labelled vertical markers. Errors link straight to their node in the editor.

### Devices and security (`STU-115`)

Tables of paired development devices and registered devices, with the assurance level as a labelled badge (`AL2 · Hardware key`). Pairing shows the QR code in a card with a countdown ("Expires in 1:42"). Keys and pins show fingerprints in mono, rotation dates and the next action ("Rotate before 12 Mar").

### Admin (`STU-116`)

Settings pages with a section list on the start side and one clear form per section with a visible save state. The audit log is a filterable table with CSV and signed exports and an "Integrity verified" indicator for its hash chain. Installation health is a status table of the server roles with uptime and version.

### Command palette (`STU-013`)

A centred white dialog with a large search field; results grouped as Screens, Plugins, Commands and Translation keys, each with its path and shortcut, recent items first. `?` opens the shortcut sheet as a two-column reference.

### Notifications, empty states and errors

- **Notifications:** a side panel with Unread and All tabs, grouped by day, each item linked to its object.
- **Empty states:** one clear sentence, one primary action and a link to the guide — *No plugins yet. An app starts with one plugin.* `[Create plugin]` · Learn about plugins. Small neutral line icons only; no illustrations.
- **Errors:** a plain explanation, what to do next, and the error code with "Copy details" for support.

### Settings and profile (`STU-014`)

One page: language with a live preview, theme (Light, Dark, System), density (Comfortable, Compact), shortcuts, and active sessions with "Sign out other sessions".

## 8. Right-to-left

- The chrome mirrors fully: rail, panels, tables, steppers, timelines and breadcrumbs.
- The canvas layout stays where the user placed it; only screen content previews right to left.
- IDs, versions, code and numbers stay left-to-right inside Arabic text.
- Arabic gets its own line height (about 10% taller), no letter-spacing, and is never truncated in the middle of a word.

## 9. Relation to the specification

- The direction follows `STU-010`, `STU-011`, `STU-012`, `STU-016` and Appendix J as written.
- The danger colour (`#D92D20` light, `#FF7066` dark) is not yet an Appendix J token; it becomes one before the first Studio component uses it.
- The interface typeface is chosen with the Studio stack decisions; until then this document names its requirements only.

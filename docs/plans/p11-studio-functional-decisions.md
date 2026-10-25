<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# P11 Studio — Functional, Flow and Operational Decisions

> **Status:** Decision record; the accepted plan is [p11.md](p11.md).

- **Status:** Decided by the maintainer in the preparation session (2026-10-18 to 2026-10-19),
  after the engineering decisions. Binding input for the P11 plan, its ADRs and every Studio
  session. Nothing here is implemented yet.
- **Read with:** [p11-studio-decisions.md](p11-studio-decisions.md) — engineering decisions and the
  Studio Development rule book (Part B there moved to
  [studio-development.md](../engineering/studio-development.md)). References like "A5.1" or "rule book §6.3"
  point to that file. The two files agree; where this file changed an earlier decision (F19), the
  earlier file was updated.
- **Standing rules from the maintainer apply to every item:** protect the server (bound all new
  server work), legal care, best performance first, no undeclared dependency.
- **Not yet applied:** no existing document was changed. Spec, ADR and `proto/` work is listed in
  Part Z, each to be made in the pull request that needs it, with the maintainer's approval where
  `AGENTS.md` §7 requires it.

---

## F0. Facts established in the session

- **Apps belong to the organisation, not to a person** (`GOV-001`). Access is granted per app to
  teams or users; anyone with an organisation-wide role (owner, admin, developer, viewer —
  ADR-0026) sees every app of the organisation. There is no private app; a user with only per-app
  grants sees only those apps.
- **"Screen editor"** (spec §21.7) is the plugin canvas with a screen selected for editing and the
  panels open — not a separate view. The plugin canvas shows all screens with their contents,
  overflow regions, ghost screens from other plugins and the arrows between them.
- **"Arrow bundle"** (spec §21.5): all links from one box to another in the same direction drawn as
  one arrow with a count badge; on hover it fans out into one *lane* per link (`STU-044`); a lane is
  one link (`STU-046`).
- **Approvals area is `STU-102`**; `STU-101` is the Releases area (an earlier slip in the session).
- The server already has: exclusive plugin locks with 30 s heartbeat and 2 min expiry, request,
  forced takeover with preserved snapshot, separate app-level lock (`SRV-040`–`SRV-042`,
  `plugin.proto`); snapshots of every write (`SRV-031`); durable idempotent publish jobs with
  progress streaming (`SRV-050`, `publish.proto`); releases, promotion, rollback, changelog,
  compatibility (`release.proto`); browser sessions and CSRF tokens (ADR-0026); `/livez` and
  `/readyz` (`SRV-007`). Approvals, rollout, kill switch, custom roles and step-up are P9 and come
  before P11.

## F1. Lock lifecycle (`SRV-040`–`SRV-042`, `STU-075`)

1. **Taken on the first edit**, not on open. The first edit waits for `AcquireLock` (one fast call);
   if it fails, the edit is undone and the plugin turns read-only. App-level documents (theme,
   locales, data sources, native catalogue) take the app lock the same way.
2. **Heartbeat from a worker** (background-tab timers are throttled). **Auto-release** after
   10 minutes without edits (autosave flushed first) and when the tab closes.
3. **Two tabs of the same user share one lock** via `BroadcastChannel`: one edits, the other shows
   "editing in another tab" — one heartbeat per person.
4. **Losing the lock** (failed renewal, expiry, takeover; seen on the events stream): switch to
   read-only at once, keep unsent changes in memory as "N unsaved changes", offer "re-take and
   apply" (only if the lock is free and the revision unchanged), "compare" or "discard". No new API
   (at most ~500 ms of edits are ever unsent).
5. **Takeover:** "Request edit" notifies the holder (release or keep). Users with
   `plugin.lock.override` see "Take over" with a confirmation showing the holder and their last
   activity. The previous holder is told who took it and gets a link to the preserved snapshot.
   *Decided (L-1 of the [P11 plan](p11.md)):* "Take over" is enabled only when the holder has made
   no edit for 5 minutes (`studio.lockTakeoverIdleMinutes`), or at once when the holder's lease has
   lapsed (no renewal for two missed heartbeats, 60 s); "Request edit" is always available. The
   server enforces the rule and audits every takeover.
6. **Read-only mode:** navigate, select, inspect, copy; editing controls disabled with a reason
   (not hidden); banner with holder, since when, "Request edit" (only for users with edit
   permission). Viewers see the holder's changes through revision events on the events stream;
   refetches are throttled.

## F2. Autosave conflicts (`SRV-030`)

- Meaning: a save says "these changes apply to revision N"; if the server is at N+1 it rejects the
  write so nothing is overwritten silently. Causes: Studio sending before the previous save is
  confirmed (most common); server-side writes into a held plugin (link updates from a rename or
  move elsewhere, restore, import, migration); a lost and retaken lock (F1).
- **Prevent:** one save in flight; edits meanwhile queue, merge and go with the confirmed revision.
- **On rejection:** fetch the latest draft and replay pending patches; every patch carries JSON
  Patch `test` operations, so it applies only where the values it changes are unchanged.
  - All apply → resend silently; canvas and undo stack move to the new revision.
  - A clash → pause autosave; "This plugin changed on the server" with a diff and: keep mine (apply
    what fits, list what was dropped), take the server's, or copy mine as JSON first.
- **Server protection:** at most 3 automatic replays in a row, then stop and ask.

## F3. Undo scope (`STU-069`)

- Only **document changes** are undo steps. View (pan, zoom), selection, panel state and preview
  switches are not; undo restores the selection of the edit it reverses and pans to show it.
- **One stack per plugin**, across its screens; undoing a change on another screen first moves
  there ("Undid: padding on Login"). **A separate stack for app-level documents** (`SRV-042`).
- **Grouping:** a drag is one step; typing in a field is one step until blur or ~1 s pause;
  multi-node actions (paste, wrap, extract to component) are one step.
- **Overlay editors:** Monaco/CodeMirror use their own undo while focused; each change they apply
  to the document is one step in the plugin stack.
- **Cross-plugin changes** (move a screen, rename updating links) are one step; undo needs both
  locks, otherwise it is blocked with the reason.
- A new edit clears redo; undo is disabled when read-only or after losing the lock; publishing does
  not clear the stack; depth from the limits registry (proposed default 200), which also caps the
  server-side rebuild (A5.5).

## F4. Offline and failure behaviour

- **Network loss:** banner "Offline — N changes waiting"; editing continues; changes queue in memory
  only (never on disk) and are sent in order on reconnect. Outages longer than the 2 min lock
  follow F1.4.
- **Server down or overloaded:** exponential backoff with jitter, at most every 30 s, honouring
  `Retry-After`; status chip "Reconnecting…"; streams resume from their cursor.
- **Save rejected for its content:** that patch is undone and the diagnostic shown in Problems
  (rule book §6.2). Revision conflicts follow F2.
- **Expired session:** the BFF answers 401 → "Signed out — sign in again" in a popup, so the tab
  keeps canvas, selection and queued changes, sent after sign-in; if sign-in fails, pending changes
  can be copied as JSON.
- **Always:** warn before closing a tab with unsent changes; errors are localised reasons, never
  raw messages (rule book §6.5).

## F5. Selection model (`STU-067`)

- **One selection per open canvas** in the area's Zustand store: an ordered set of node IDs, an
  *anchor* (last clicked) and a *scope* (the container you are inside). IDs, not positions: it
  survives re-layout; deleted nodes drop out. UI state — not saved, not an undo step (undo restores
  it). Only the anchor goes in the URL.
- **Shared everywhere:** the canvas worker, Layers tree, Properties panel and Selection tab read the
  same store. The tree expands and scrolls to the selection; the canvas pans to it only when the
  selection came from the tree, a panel or search, never from a canvas click. Hover is shared.
- **Mouse and keyboard:** click selects a direct child in the current scope; double-click or Enter
  goes into children; Esc selects the parent; Ctrl/⌘+click deep-selects the innermost node;
  Shift+click adds/removes; marquee selects touching nodes in scope; Ctrl/⌘+A selects all siblings;
  Tab/Shift+Tab move between siblings; arrows nudge.
- **Move, nudge and resize follow the parent** (maintainer, 2026-10-25): inside a `Stack`, move and
  nudge change the child's `Positioned` left/top (arrows 1 px, Shift+arrows 10 px); inside `Row`,
  `Column` or `Wrap`, move reorders (arrows step one sibling) and dragging reparents with the
  insertion indicator; resize sets width/height only where the node's descriptor has size
  properties (e.g. `SizedBox`, `Container`). Only schema fields are written; nothing else moves.
- **Multi-select:** Properties shows shared properties with "Mixed" where values differ; may span
  screens for copy and delete, not for reparenting; locked/hidden nodes only via the Layers tree.
- **What is selectable:** app canvas — plugin boxes, frames, arrows; plugin canvas — screens,
  overflow regions, ghost screens, arrows, and the nodes inside screens (inside component
  instances only overridable parts); an arrow bundle — a single lane (`STU-046`).
- **Accessibility:** selection changes announced via a live region; the accessible tree mirrors the
  selection; selection works in read-only mode.

## F6. Link creation and assisted fixes (`STU-040`, `STU-046`, `STU-047`)

- Arrows are always **derived from navigate actions** (`STU-040`), never drawn by hand.
- **Primary path (maintainer's proposal):** select the button → Actions tab → add "Navigate" →
  pick the target screen → map its parameters → apply; the arrow appears because the link exists.
- **Drag shortcut** (keeps `STU-047`, a MUST):
  - from a **selected widget's** handle to a screen: trigger fixed to that widget (solves "which of
    several buttons"); opens the same Actions panel with the target filled;
  - from a **screen's** handle: first asks which trigger (the screen's interactive widgets and
    events such as `onEnter`), then the same panel.
  - Nothing is created until the user applies in the panel.
- **Assisted fixes** (lane click `STU-046`, blocked delete `STU-070`): "open the action", "redirect
  target", "remove link" — each opens or edits the same navigate action: one editor, one
  validation path.

## F7. Copy and paste (`STU-067`, `STU-055`)

- **Format:** versioned JSON envelope under `web application/x-plux+json` (async Clipboard API) plus
  plain-text JSON fallback: source app/plugin/organisation, the copied nodes, and referenced
  dependencies (tokens, assets, components, state, translation keys) by ID. Sensitive values never
  copied in clear (rule book §11.4).
- **New UUIDv7 for every pasted node**, internal references remapped. Same app: links to existing
  screens and components stay. Other app or organisation: references missing there are listed.
- **Dependency report before applying**, per item: map to an existing token/component/asset, copy
  over (if allowed and within limits), or drop. Nothing silent.
- **Outside pastes are untrusted:** parsed as data only, never rendered as HTML; must pass the
  schema and the WebAssembly validator (server validates again); size capped by the registry; a
  paste from another organisation copies only references the user can read there.
- One undo step; refused in read-only mode.

## F8. Components and templates (§21.9, `STU-080`–`STU-086`)

**Templates — copied starting points, never linked.**
- **Create:** select nodes → "Save as template" in two steps (details; exposed parameters and
  visibility). Dependencies captured; page-state references become parameters, the rest flagged;
  thumbnail from the canvas snapshot.
- **Visibility:** `private` (author), `organization` (every member), `public` (whole installation;
  automated scan first, moderation optional) (`STU-081`).
- **Maintenance:** revision history; the author edits their own; organisation-wide ones also by
  holders of a proposed **`library.manage`** permission; templates of a departing author pass to the
  organisation admins.
- **Access:** Insert panel and library browser (`STU-086`) with search and filters; export/import as
  files.
- **Inserting into existing plugin content:** copied with new IDs; parameters asked; dependencies
  get the same mapping report as a paste (F7); clashes with existing state names, translation keys
  or tokens shown with rename or reuse; afterwards independent — template changes never change
  plugins it was inserted into (`STU-083`).

**Components — linked and versioned.**
- **Override model:** an instance stores component + version + overrides (prop values, slot fills);
  everything else comes from the component.
- **Which instances change:** editing creates a new draft version; before saving, Studio lists the
  instances that would change across all apps that can see it (`STU-084`).
- **Adopting a new version:** per app, explicitly, with a diff (`STU-085`); overrides kept where the
  prop still exists; overrides of removed props flagged.
- **Detach:** instance → plain nodes with new IDs; one undo step.
- The organisation's locked library lives in the Design system area (`STU-112`); only permitted
  roles publish component versions there.

## F9. Validation UX

- **When:** every edit, in the WebAssembly validator worker, for the edited page (`ValidatePage`);
  project-wide when a plugin opens and before publishing; the server again on save and publish.
- **Severities:** the compiler's own — error, warning, info.
- **Problems panel:** grouped by screen, with code and JSON path; click selects the node; the action
  bar shows counts per severity.
- **Publishing:** errors block (`SRV-051`); warnings need acknowledgement by the publisher or by
  policy; info never blocks.

## F10. Preview switches and responsive overrides (`STU-058`, `WGT-010`)

- Meaning: **preview switches** are canvas-wide view settings that change only how screens look
  while designing (theme, locale incl. RTL, text scale, Material/Cupertino, data mock state);
  **responsive overrides** are prop values for compact/medium/expanded size classes, saved in the
  document and shipped in the app.
- **Preview switches:** saved per user per app (profile), not in the document.
- **Responsive overrides:** in the document (`WGT-010`); the active device frame decides which size
  class an edit applies to; Properties marks overridden values.

## F11. Publish flow — a server-side state machine (`STU-100`)

The flow is a server-side **release request**; Studio's wizard only displays it. Every step's result
lives on the server, so closing the tab loses nothing. **One open request per app and channel**;
others see "Anna is publishing" and can view it.

| # | State | What happens | Exits |
|---|---|---|---|
| 1 | Select | Plugins with unpublished changes (draft vs last version); dependencies (new link targets, shared state; `REL-003`) added automatically with the reason | → 2 |
| 2 | Freeze & build | One `Publish` job per plugin at its current revision (`SRV-050`), idempotent with a key; editing may continue; banner offers "re-freeze" if the draft changes | failed → Failed (retry); → 3 |
| 3 | Reports | Validation, accessibility, budget reports from job diagnostics | any error → Blocked (links to Problems; fixing returns to 2); warnings acknowledged one by one or by policy (`SRV-051`) → 4 |
| 4 | Diffs | Server-side visual diff per changed screen on standard devices (queued, rate-limited job); JSON diff and "what changed" list in the browser | → 5 when the visual diff is done (approvers need it, `GOV-023`) |
| 5 | Changelog & notes | Auto changelog (`REL-081`), human notes, optional label | → 6 |
| 6 | Target | Environment and channel; built once, promoted unchanged (`REL-004`); compatibility and affected install base (`REL-080`) | → 7 |
| 7 | Approval | Policies evaluated (who must approve — the simulator); if needed, a request bound to the release's content hash (`GOV-022`) → Awaiting approval; Studio may close; a notification brings the user back | approved → 8; rejected → Rejected; content changed → approvals invalid, back to 2; expired → Expired; no policy → 8 |
| 8 | Rollout plan | Stages, schedule, health gates, mandatory flag, activation policy (`REL-050`–`REL-053`, `REL-070`), prefilled from app defaults; change freezes block production (`GOV-025`) | → 9 |
| 9 | Release | Confirm with WebAuthn step-up for production (`SEC-103`); promote and start rollout; hand over to Releases (`STU-101`) | → Released |

- **End states:** Released, Rejected, Expired, Cancelled. Failed and Blocked return to the step that
  failed.
- **Resume:** the Publish button, a notification or the Releases area reopens the request at its
  state; jobs keep running while the tab is closed; progress streams resume from their cursor.
- **Cancel:** `CancelPublishJob`; plugin versions already built stay (immutable, harmless).
- **Server protection:** idempotency keys; one open request per app and channel; queued,
  rate-limited visual-diff rendering; registry limits.
- **Contract:** reuse P9's approval request object if it exists; otherwise an additive `proto/`
  resource (Z3).

## F12. Approvals and WebAuthn step-up (`STU-102`, `SEC-103`)

- **Meaning:** *approvals* — some changes (production releases, function deploys, policy changes)
  need sign-off from other people per written policies (four-eyes, roles, distinct teams, stages,
  expiry; `GOV-020`–`GOV-023`). *WebAuthn step-up* — before a sensitive action, the user confirms
  again with a passkey, security key or biometric; valid 5 minutes (`SEC-103`).
- **Approvals area:** inbox; detail view with all evidence (visual and JSON diffs, reports, install
  base, changelog, risk); approve/reject with comment then step-up; history; break-glass reviews;
  policy editor with simulator.
- **Step-up:** one shared dialog (rule book §11.6). When the server answers "step-up required", the
  dialog runs the WebAuthn challenge through the BFF; the server marks the session stepped-up for
  5 minutes; Studio retries the action. Nothing beyond the browser's WebAuthn call touches
  credentials.

## F13. Safety rules (§21.8, `STU-070`–`STU-077`)

- **One "where used" index** built by the validator worker from the reference graph (`SCH-041`).
  Every delete checks it; blocked deletes (`STU-070`–`STU-072`) open a panel listing each use with
  navigation and assisted fixes (redirect incoming, remove outgoing) applied through the action
  editor as one undo step. The server enforces the same rules.
- **Entry points** (`STU-071`): delete disabled, with "choose another entry first".
- **Trash** (`GOV-031`): soft delete, 30 days, restorable per app from a Trash view with the old IDs
  (links return); permanent deletion admin-only.
- **Deprecation of published plugins** (`STU-073`): "Delete" marks *deprecated* and shows what still
  links to it; it leaves the next release only when nothing links to it; earlier releases keep it;
  then trash.
- **Production-destructive actions** (`STU-074`): one shared type-to-confirm dialog → approval or
  break-glass → step-up.
- **Renames** affecting deep links or host code warn and list affected generated APIs (`STU-076`);
  automatic redirect loops are validator errors and block publishing (`STU-077`).

## F14. Permissions in the UI — with restriction policies (maintainer requirement)

- **Existing (P9):** per-app grants to teams or users (`GOV-001`); predefined and custom roles from a
  documented permission catalogue (`GOV-002`); scoped by app and environment (`GOV-003`);
  deny-by-default, server-checked (`SEC-102`).
- **New: restriction policies (ceilings).** Rules that remove permissions whatever the grants say;
  deny always wins.
  - **Organisation level (owner):** switch features off org-wide (e.g. AI assistant, functions,
    production publishing).
  - **Team or person level (admins):** e.g. "Contractors: no publish, no functions", "Anna:
    Localisation only".
  - **Effective rights** = grants − every applicable restriction (organisation, team, person).
  - **No lockout:** restrictions can never remove the last owner's ability to manage members and
    policies. Every change to grants or restrictions is audited.
- **Every button maps to the catalogue:** each Studio command, menu item, panel and area declares the
  permission it needs in the command registry (rule book §5.7). The catalogue becomes fine-grained
  per feature (e.g. `studio.json.edit`, `studio.functions.edit`, `studio.ai.use`,
  `release.publish:<env>`). The admin **role builder** shows it as a tree (area → feature →
  action) — how "only this small set of buttons and menus" is configured.
- **"Explain access" view:** why a person can or cannot do X, naming the grant or restriction;
  doubles as a simulator before saving changes.
- **Showing it:** hide areas and menus the user can never use in any scope; disable with a tooltip
  reason when unavailable only here or now (other environment, someone's lock, "turned off by your
  organisation's policy"). The effective permission set per (organisation, app, environment) is
  loaded at sign-in, kept in the Query cache, refreshed via the events stream; the server still
  checks every call.
- **Enterprise (`ee/`) features:** shown only to owners and admins, labelled "Enterprise — requires a
  licence" (offline licence, `GOV-030`); hidden from others. Security features are never
  edition-gated (`GOV-032`).
- **Implementation dependency:** restriction policies and the per-feature catalogue are server
  work (RBAC and row-level security). *Decided (L-8):* P9 delivered them; the fallback is not needed.
  *Permissions (M-7, [p11.md](p11.md) §6.2):* each command maps to the server permission that
  guards its RPC, a new permission is added only with its server check, and there is no
  browser-only permission; the per-feature keys named above (e.g. `studio.json.edit`) are
  illustrative.

## F15. Search and the command palette (`STU-013`)

- **In the browser** (instant, no server load): commands from the registry; everything loaded for the
  open app (plugins, screens, nodes, translation keys, state names); fuzzy matching with
  `@leeoniya/ufuzzy` (1.0.19, MIT; allowlist entry needed), rebuilt as the document changes.
- **Server search API** for the rest of the organisation (other apps, component and template library,
  functions): names and keys only, never content; permission-filtered on the server (row-level
  security); debounced ~200 ms, capped results, rate-limited at the BFF.
- **Palette:** local results immediately, server results below as they arrive; recents first; prefixes
  narrow the scope (`>` commands, `#` screens).

## F16. Notifications

- **Types:** work (lock requested/taken over; publish finished/failed/blocked; shell update required;
  new component version); approvals (requested/approved/rejected/expired; break-glass alerts);
  operations (rollout paused by a health gate; kill switch used; limits nearly reached); account and
  security (role or access changed; new device paired; sessions revoked).
- **Storage:** server-side, one record per recipient with type and parameters; rendered in the
  reader's language at display time, never stored as text (O-18; typed parameters on `InboxItem`,
  contract #20 of the P11 plan).
- **Delivery:** live via the events stream; inbox list via Query.
- **Read state per user:** read/unread, "mark all read"; per-type mute in the profile, except approval
  and security notifications; repeats grouped ("3 publishes finished").
- **Retention:** 90 days default (limits registry); approval and security notifications link to their
  audited record; the audit log is the permanent source.
- **Server protection:** capped fan-out per event, grouping, unread count pushed on the stream (no
  polling). Email and other channels: an operational decision for later.

## F17. Onboarding and sample apps (`STU-015`)

- **Content:** the reference apps in the repository (`apps/plux_bank`, `apps/plux_express`,
  `apps/starter`) — their Plux documents in the Git layout (`SCH-006`) — packaged as `.plux` archives
  with each server release and embedded in the server image (no network; air-gapped works). CI
  already builds and tests these apps, so samples cannot go stale unnoticed.
- **Import:** an ordinary `.plux` import into the user's organisation with new IDs; the user's copy
  from then on.
- **Checklist** (create/import → pair a device → edit → publish): progress per user on the server,
  derived from real events, not ticked by hand; dismissible; reopened from Help.

## F18. Shell-update detection and regeneration (`GEN-004`, `GEN-001`)

- **Detection:** each app keeps the `ShellSpec` its project was last generated from (ADR-0024,
  `plux.shell.json`). Studio compares current shell settings (name, icons, splash, identifiers,
  version, permissions, optional packages, push) and shows "Shell update required" with exactly what
  changed and why each needs a store build.
- **Fix:** a regenerated project (zip, Git push or local directory; `GEN-003`) or a patch with only the
  changed files for teams who edited their project.
- **Generation:** the deterministic Go generator as a queued, rate-limited server job.
- **Publishing continues** meanwhile (`GEN-005`); only what needs the new shell waits — the compiler
  flags what is incompatible with host builds in use (`REL-080`).

## F19. BFF sessions

- **The BFF keeps no session state; it uses the server's sessions.** The server already issues
  `__Host-plux_session` (stored hashed in PostgreSQL) and CSRF tokens (ADR-0026). The server has no
  `/auth/*` routes (L-13 of the [P11 plan](p11.md)): the BFF proxies the IdentityService and
  FederationService RPCs, hosts the OIDC callback and the SAML assertion consumer, and passes the
  cookie through. No second store, no token in the
  browser (`SEC-101`). Valkey is not used for sessions (ADR-0007: Valkey holds only expendable state).
- **Sign-in** is the server's: password with TOTP or WebAuthn today; OIDC/SAML from P9 (`GOV-004`).
  `STU-002` currently says the BFF runs the OIDC flow and stores tokens — its wording changes (Z1).
- **Timeouts:** sliding idle expiry and an absolute limit, enforced by the server, adjustable by
  administrators (`GOV-005`); only user activity counts (not lock heartbeats or streams). *Decided
  (O-1):* the **30-minute idle** and 12-hour absolute defaults P9 set are kept, not the 2 hours
  first proposed; Studio's re-sign-in popup keeps the canvas and queued edits (F4).
- **Refresh:** the server extends the session on activity; no refresh tokens in the browser.
- **Revocation:** server-side and immediate (`GOV-005`); revoked sessions' streams are closed at once.
- **Several BFF instances:** stateless, so no sticky sessions; streams resume from their cursor on any
  instance.

## F20. Configuration and health (BFF)

- **Configuration:** same mechanism as the server (YAML + environment), typed, validated at start;
  unsafe values never default and always warn distinctly. Settings: public base URL, API upstream,
  rate limits, telemetry sampling and switch, maximum stream connections. The development catalogue
  page is always off in production builds.
- **Health:** `/livez` and `/readyz` like the server (`SRV-007`); `/readyz` checks the API upstream.
- **Graceful shutdown:** stop accepting; tell open streams "restarting" so clients resume elsewhere;
  drain within 30 s.
- **Rate limits** per session and per IP at the BFF; counters in memory on a single node or in Valkey
  with several instances (expendable state, fits ADR-0007); the API keeps its own limits.

## F21. Developer workflow

| Target | What it does |
|---|---|
| `make studio-dev` | `make dev` stack + BFF in watch mode + Vite dev server with hot reload; the BFF proxies to the stack |
| `make studio-check` | existing: frozen install, Biome, type-check, tests with coverage floors |
| `make studio-conformance` | `flutter_test` goldens, then the TypeScript layout engine compared against them |
| `make studio-e2e` | Playwright against the Compose stack with Studio, seeded from the sample apps |
| `make studio-bench` | CI performance benchmarks (relative baselines) |
| `make studio-build` | production build, Brotli files, notices |

- **Affected-only CI** (`ci/affected.json`, ADR-0043): `studio/**` → Studio job; `proto/**` →
  generated-client checks + Studio job; `schema/widgets/**` and the runtime's rendering code →
  conformance job; `deploy/compose/**` → end to end.
- **End-to-end environment:** the Compose stack (`DEP-002`) with the Studio container, on hosted
  runners with Playwright Chromium. Real-device runs stay manual.

## F22. Release build

- **Reproducible Vite build:** pinned Bun, frozen lockfile, `SOURCE_DATE_EPOCH`, content-hashed chunk
  names, no timestamps or environment values in output; CI builds twice and compares digests.
  `CI-006` covers Go binaries only today — extending it to Studio is a spec addition (Z1).
- **SBOM:** CycloneDX via Syft for the image (existing pipeline, `CI-001`), covering every package in
  `bun.lock`.
- **Provenance and signing:** the existing SLSA generator and cosign (`release.yml`) applied to the
  Studio image.
- **Notices:** third-party licences (CanvasKit/Skia, Monaco, CodeMirror, fonts, all packages) in the
  image under `/usr/share/doc`, as the server does.

## F23. P11 milestones — vertical slices, riskiest first

Each ends at the milestone exit gate used since P4: every gate passes with exit codes checked;
repeated end-to-end runs pass; a green dispatched CI run; open items resolved or put to the
maintainer. From S0, the gate also includes the **design gate** of the UI language
([studio-ui-language.md](../engineering/studio-ui-language.md) U18.6): the automated AI-pattern detector over
the catalogue page and the slice's routes, and the maintainer's local review of the slice's screens
against the reference page.

| # | Slice | Proves |
|---|---|---|
| S0 | Foundations: packages, generated clients, BFF (sessions, CSP, proxy), shell with routing, i18n, themes, command registry; CI jobs; the prototype measurements (decisions file C5) | the stack works; go/no-go on the CanvasKit worker |
| S1 | Plux Canvas core + layout port core (box, flex, stack, text) + conformance harness; a real plugin drawn read-only | the riskiest part works |
| S2 | Editing core: locks, edit pipeline, autosave and conflicts, undo, selection, Properties, Layers, Insert, validator worker and Problems | editing is safe |
| S3 | App canvas and arrows: plugin graph, bundled arrows, ghost screens, link creation, safety rules | the navigation design |
| S4 | Rest of the layout port (slivers, Material and Cupertino composites); conformance for all Layer 1 and Layer 2 widgets | `STU-005` complete |
| S5 | Overlays: JSON, action graph, inline PXL, Rust bridge, localisation, theme, state and data designers | the heavy editors |
| S6 | Components and templates, copy/paste, device frames and matrix, preview switches, send to device, bottom drawer | the full design loop with a device |
| S7 | Publish flow, Releases, Approvals, Experiments, step-up | shipping |
| S8 | Apps grid, onboarding, Functions, Analytics, Devices & security, Admin (permissions, restriction policies), notifications, search | completeness |
| S9 | Hardening: performance budgets on the reference laptop, accessibility audit, RTL sweep, security review | the targets hold |

**Exit demo** (spec P11 exit criterion): a complete app built in Studio alone from blank → previewed
on a paired device → approved by a second person with step-up → released with a staged rollout →
performance targets met on the reference laptop. Runs as a scripted end-to-end test in CI (without
the real device) plus one manual run with a real device, numbers recorded.

## F24. Scope decision

- **AI entry points are not in P11** (Option A, maintainer). `STU-060` (AI assistant button),
  `STU-024` (create an app "from an AI prompt") and `STU-055` (create a screen "from an AI prompt")
  name P12 features (`AI-010`); P11 leaves them out entirely (no disabled placeholders), P12 adds
  them, and the three requirements' wording says so (Z1). `AGENTS.md` §5: later-phase features are
  absent.

---

## Z. Follow-up work this file implies

### Z1. Spec wording (maintainer-approved; ADR for each `MUST` change; `make spec-lint` after each edit)

- `STU-002`: the server performs sign-in (OIDC/SAML from P9) and holds sessions; the BFF serves the
  sign-in pages and passes the session through (F19).
- `STU-060`, `STU-024`, `STU-055`: AI entry points arrive with P12 (F24).
- §21 area table: "Functions — Go editor" → "Rust editor" (Functions moved to Rust in spec 1.5.0,
  `FN-033`).
- `GOV-002` / `GOV-003` additions: restriction policies (ceilings) at organisation, team and person
  level; a per-feature permission catalogue covering every Studio command (F14).
- `CI-006`: extend reproducible release builds to the Studio image (F22).
- New: `library.manage` permission for organisation templates (F8).

### Z2. Defaults and policies needing the maintainer's approval

- Session timeouts: idle 30 min, absolute 12 h (F19, O-1; the 2 h first proposed was not kept) — security default.
- Lock auto-release after 10 min idle (F1.2); undo depth 200 (F3); notification retention 90 days
  (F16) — as limits-registry defaults.
- Takeover policy (F1.5): decided, L-1 (5 minutes without an edit, or a lapsed lease).

### Z3. `proto/` and contract additions (maintainer decision, `AGENTS.md` §7; additive)

In addition to the decisions file's C4 list:

1. Release request resource with its states (F11), unless P9's approval request object covers it.
2. Restriction policies and the effective-permissions / "explain access" API (F14). *Delivered by P9 (L-8).*
3. Organisation search API: names and keys, permission-filtered (F15). *Delivered by P9 (L-8).*
4. Notifications: list, read state, mute preferences, events-stream messages (F16).
5. Onboarding progress per user (F17).
6. Shell-spec comparison and project/patch generation job for Studio (F18).
7. Revision-changed and lock-changed events on the Studio events stream (F1, F2).

### Z4. Dependencies (allowlist + ADR)

`@leeoniya/ufuzzy` (F15). All others are in the decisions file's A3 list (fonts and icons as revised
on 2026-10-22). The design gate's detector (proposed: Impeccable, Apache-2.0, a development tool, not
shipped) needs an allowlist entry and the maintainer's approval as a CI gate change (`AGENTS.md` §7).

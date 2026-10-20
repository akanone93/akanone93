# P11 Studio — Routing

- **Status:** Accepted by the maintainer on 2026-10-20.
- **Read with:** [p11-studio-decisions.md](p11-studio-decisions.md) (engineering, rule book) and
  [p11-studio-functional-decisions.md](p11-studio-functional-decisions.md) (F1–F24).
- **Scope:** every Studio page, overlay, drawer, dialog and popover the router touches or opens, for
  all P11 areas (spec §21). Excluded by phase: AI entry points (P12, F24); sign-up, public gallery
  and multi-region (P15, `SAAS-002`, `SAAS-006`, `SAAS-008`).

---

## R1. Conventions

| Topic | Rule |
|---|---|
| Router | TanStack Router, file-based routes, typed params and search params (A4.9) |
| IDs in paths | Entities use their immutable UUIDv7 (`SCH-002`), optionally followed by a readable slug that is ignored when resolving: `/apps/0192f…-plux-bank`. Renames never break links (`STU-076`); Studio rewrites to the current slug with `replace` |
| Organisation | Every org-scoped path starts `/o/:org` (`:org` = organisation ID + slug). The router sends the org as `X-Plux-Organization` (ADR-0026). An org the user is not in → 404, never 403 |
| Environment | Per-environment pages take `?env=<key>` (default `development`; production shown last). Never in the path |
| Search params | Validated with Zod schemas; defaults omitted from the URL |
| History | Navigation between pages and opening an overlay: **push** (Back closes the overlay). Viewport, selection anchor, drawer tab, filters, sort: **replace** |
| Not in the URL | Preview switches (F10, per user per app in the profile); panel tabs (local per-user preference); Studio language (profile) |
| Guards | Auth (BFF 401 → `/signin?next=`); org membership (404); permission per route from the catalogue (F14: hidden in navigation, direct URL → in-shell 403 page with "Explain access"); enterprise (`ee/`) routes only with a licence (else "Enterprise — requires a licence" page for admins, 404 for others) |
| Loading | Every area is a lazy route chunk; loaders prefetch Query data; the canvas and overlays load on demand and are prefetched on intent (`STU-122`) |
| Layouts | `public` (sign-in family, no shell) · `shell` (org pages: top bar + area navigation) · `canvas` (app and plugin canvases: floating action bar, panels, drawer; `STU-060`–`STU-066`) · `page` (all other app pages, with app header and area tabs) |

## R2. Route tree

### R2.1 Public (layout `public`, no organisation)

| # | Path | Page | Notes |
|---|---|---|---|
| P1 | `/signin` | Sign in | password; `?next=`; `?popup=1` for the re-sign-in popup (F4); `?lang=` |
| P2 | `/signin/factor` | Second factor | TOTP or WebAuthn challenge (ADR-0026) |
| P3 | `/signin/sso` | SSO redirect | P9 OIDC/SAML (`GOV-004`); returns to `?next=` |
| P4 | `/invite/:token` | Accept invitation | set password, enrol second factor |
| P5 | `/setup` | First administrator | one-time; refused once an administrator exists (ADR-0026) |
| P6 | `/device` | Approve CLI sign-in | `plux login` device grant (ADR-0026) |
| P7 | `/signed-out` | Signed out | after sign-out or revocation |
| P8 | `/unsupported` | Unsupported browser | no WebGL2 (A5.8) |
| P9 | `*` | Not found | also used for unknown organisations |

### R2.2 Personal (layout `shell`, no organisation in the path)

| # | Path | Page |
|---|---|---|
| U1 | `/` | Redirect to the last used `/o/:org`; no organisation → "You are not in an organisation yet" |
| U2 | `/me` | Profile: name, avatar, theme, language, density (`STU-014`) |
| U3 | `/me/shortcuts` | Keyboard shortcuts (generated from the command registry) |
| U4 | `/me/security` | Password, second factors, passkeys |
| U5 | `/me/sessions` | Own sessions; revoke |
| U6 | `/me/tokens` | Personal access tokens |
| U7 | `/me/notifications` | Notification preferences (mute per type, F16) |

### R2.3 Organisation (layout `shell`, prefix `/o/:org`)

| # | Path | Page | Spec |
|---|---|---|---|
| O1 | `/` | Home: onboarding checklist, recent apps and plugins, pending approvals, notifications summary | `STU-015` |
| O2 | `/apps` | Apps grid/list `?view=grid\|list&q&team&tag&status&mine&sort` | `STU-020`–`STU-025` |
| O3 | `/apps/new` | New app wizard (route modal over O2) `?step=source\|details\|locales\|theme\|security\|environments\|done&source=blank\|template\|import&template=` | `STU-024`, `GEN-001` |
| O4 | `/library` | Components and templates `?kind&q&visibility&owner&category&tag` | `STU-086` |
| O5 | `/library/components/:id` | Component detail: versions, usage, instances across apps | `STU-084`, `STU-085` |
| O6 | `/library/templates/:id` | Template detail: parameters, revisions, visibility | `STU-080`–`STU-083` |
| O7 | `/approvals` | Inbox `?app&status&mine` | `STU-102` |
| O8 | `/approvals/:requestId` | Approval detail with all evidence | `GOV-023` |
| O9 | `/approvals/history` | Decided approvals | `STU-102` |
| O10 | `/approvals/break-glass` | Break-glass reviews | `GOV-024` |
| O11 | `/approvals/policies` | Policies list | `GOV-020` |
| O12 | `/approvals/policies/:policyId` | Policy editor + simulator panel | `GOV-020`, `GOV-021` |
| O13 | `/analytics` | Organisation overview | `ANL-020` |
| O14 | `/notifications` | Full notification inbox `?type&unread` | F16 |
| O15 | `/admin` | Organisation settings | `STU-116` |
| O16 | `/admin/members` | Members, invitations | `STU-116` |
| O17 | `/admin/members/:userId` | Member: grants, restrictions, sessions, "explain access" | F14, `GOV-005` |
| O18 | `/admin/teams` | Teams | `GOV-001` |
| O19 | `/admin/teams/:teamId` | Team: members, app grants, restrictions | F14 |
| O20 | `/admin/roles` | Predefined and custom roles | `GOV-002` |
| O21 | `/admin/roles/:roleId` | Role builder (catalogue tree area → feature → action) | F14 |
| O22 | `/admin/restrictions` | Restriction policies (org, team, person) | F14 |
| O23 | `/admin/access` | Explain access / simulator `?user&app&env&permission` | F14 |
| O24 | `/admin/sso` | SSO and SCIM | `GOV-004` |
| O25 | `/admin/tokens` | API and CI tokens, workload identities | `STU-116` |
| O26 | `/admin/sessions` | Active sessions; timeouts | `GOV-005` |
| O27 | `/admin/audit` | Audit log search and export `?from&to&actor&action&resource` | `STU-116` |
| O28 | `/admin/compliance` | Compliance reports | `GOV-026` |
| O29 | `/admin/retention` | Retention settings | `STU-116` |
| O30 | `/admin/license` | Licence | `GOV-030` |
| O31 | `/admin/limits` | Organisation limits and usage | `LIM-001`, `STU-025` |

### R2.4 Installation (layout `shell`, installation administrators only, prefix `/installation`)

| # | Path | Page |
|---|---|---|
| I1 | `/installation` | Health (`STU-116`) |
| I2 | `/installation/organisations` | Organisations |
| I3 | `/installation/storage` | Storage |
| I4 | `/installation/jobs` | Jobs |
| I5 | `/installation/backups` | Backups |
| I6 | `/installation/issuers` | Trusted CI issuers (ADR-0026) |
| I7 | `/installation/audit` | Installation audit chain |

### R2.5 App — canvases (layout `canvas`, prefix `/o/:org/apps/:app`)

| # | Path | Page | Spec |
|---|---|---|---|
| C1 | `/` | **App canvas** — plugin graph, native nodes, frames, bundled arrows | `STU-030`–`STU-048` |
| C2 | `/plugins/:plugin` | **Plugin canvas** — all screens, overflow regions, ghost screens, arrows; screen editing in place (§21.7) | `STU-050`–`STU-069` |

Canvas search params (C1 and C2):

| Param | Values | History |
|---|---|---|
| `x`, `y`, `z` | viewport centre and zoom (10–400 %) | replace |
| `screen` | screen ID being edited (C2) | push |
| `sel` | anchor node / box / lane ID (F5) | replace |
| `view` | `canvas` (default) \| `matrix` (device matrix of `screen`, `STU-057`) | push |
| `overlay` | see R3 | push |
| `target` | ID the overlay opens on (node, action graph, function, data source, collection, state entry) | push |
| `drawer` | `problems\|logs\|flow\|state\|network\|functions\|sync\|perf\|console` (`STU-066`) | replace |
| `device` | paired device ID feeding the drawer | replace |
| `highlight` | screen ID to pulse after a ghost-screen jump (`STU-053`); removed after 2 s | replace |

### R2.6 App — pages (layout `page`, prefix `/o/:org/apps/:app`)

| # | Path | Page | Spec |
|---|---|---|---|
| A1 | `/publish` | Start publishing: resumes the open release request or creates one | F11 |
| A2 | `/publish/:requestId` | Publish flow `?step=select\|build\|reports\|diffs\|changelog\|target\|approval\|rollout\|release` (step mirrors server state; read-only for others) | `STU-100`, F11 |
| A3 | `/releases` | Release timeline `?env&channel` | `STU-101` |
| A4 | `/releases/:releaseId` | Release: content, changelog, adoption, health vs previous, rollout controls, rollback | `STU-101`, `REL-050`–`REL-053` |
| A5 | `/releases/kill-switch` | Kill-switch panel `?env` | `REL-060` |
| A6 | `/experiments` | Experiments `?status` | `STU-103` |
| A7 | `/experiments/new` | New experiment wizard `?target=<node/page/prop>&step=target\|variants\|allocation\|targeting\|metrics\|schedule` | `ABT-001` |
| A8 | `/experiments/:expId` | Experiment: definition, live results, decision | `ABT-003` |
| A9 | `/experiments/flags` | Feature flags | `STU-103` |
| A10 | `/functions` | Functions list | `STU-110` |
| A11 | `/functions/:fnId` | Function: versions, aliases, assurance, capabilities, tests, metrics, logs `?tab&env` | `STU-110`, `FN-034`, `FN-035` |
| A12 | `/functions/:fnId/edit` | Rust editor (full page here; same editor as overlay `rust`) | `FN-033` |
| A13 | `/localisation` | Keys × locales grid `?plugin&locale&status&q` | `STU-111`, `I18N-008` |
| A14 | `/localisation/heatmap` | Missing-translation heatmap per plugin | `STU-111` |
| A15 | `/design` | Design system: tokens | `STU-112` |
| A16 | `/design/themes` | Themes (light, dark, high contrast) and brand overlays | `STU-112` |
| A17 | `/design/typography` | Typography per script | `STU-112`, `THM-004` |
| A18 | `/design/icons` | Icon sets | `STU-112` |
| A19 | `/design/library` | The organisation's locked component library as used by this app (adopt versions) | `STU-112`, `STU-085` |
| A20 | `/data` | API definitions `?env` | `STU-113` |
| A21 | `/data/sources/:sourceId` | Data source: operations, schemas, per-environment URLs, mocks | `DAT-002`, `DAT-080` |
| A22 | `/data/collections` | Local database collections | `STU-113` |
| A23 | `/data/collections/:collectionId` | Collection: schema, migration previews | `STU-113` |
| A24 | `/analytics` | App overview `?env&from&to` | `STU-114`, `ANL-020` |
| A25 | `/analytics/usage` | Plugin and page usage, drill-down `?plugin&screen` | `STU-114` |
| A26 | `/analytics/funnels` | Funnels | `ANL-020` |
| A27 | `/analytics/funnels/:funnelId` | Funnel | `ANL-020` |
| A28 | `/analytics/errors` | Errors and crashes by fingerprint | `ANL-040` |
| A29 | `/analytics/errors/:fingerprint` | Error: occurrences, symbolicated path, "open node" | `ANL-040` |
| A30 | `/analytics/performance` | Build time, jank percentiles | `ANL-020` |
| A31 | `/devices` | Paired dev devices and registered devices `?env&q` | `STU-115` |
| A32 | `/devices/:deviceId` | Device: platform, build, assurance, last attestation; revoke | `STU-115` |
| A33 | `/security` | Security profile and effective configuration per release | `STU-115`, `SEC-180` |
| A34 | `/security/keys` | Signing keys | `STU-115` |
| A35 | `/security/pins` | Certificate pins | `STU-115` |
| A36 | `/security/capabilities` | Capability grants | `STU-115` |
| A37 | `/settings` | App settings: name, description, icon, tags, owning team | `STU-023` |
| A38 | `/settings/shell` | Native shell settings; "shell update required"; project download | `GEN-004`, F18 |
| A39 | `/settings/environments` | Environments, channels, variables | `GOV-010` |
| A40 | `/settings/secrets` | Secrets per environment | `GOV-010` |
| A41 | `/settings/access` | App access: team and user grants | `GOV-001` |
| A42 | `/settings/limits` | Limits usage vs configured limits | `STU-025` |
| A43 | `/settings/native` | Native catalogue and host builds | `CLI-006` |
| A44 | `/trash` | Deleted plugins and pages; restore | `GOV-031`, F13 |

App navigation (area tabs in the `page` layout and a switcher in the `canvas` action bar): Canvas ·
Releases · Experiments · Functions · Localisation · Design · Data · Analytics · Devices · Security ·
Settings. The Publish button (action bar) opens A1.

### R2.7 Redirects

| From | To |
|---|---|
| `/` | last organisation, else U1 message |
| `/o/:org` with stale slug, `/apps/:app` with stale slug | canonical slug (`replace`) |
| `/o/:org/apps/:app/plugins` | `/o/:org/apps/:app` |
| `/o/:org/apps/:app/publish` | open request A2, else new request A2 |
| any guarded path while signed out | `/signin?next=<path>` |
| P1–P5 while signed in | `?next` or `/` |

## R3. Full-screen overlays (search param `overlay`, over C1/C2; canvas paused and kept, `STU-063`)

| # | `overlay` | Overlay | `target` | Spec |
|---|---|---|---|---|
| V1 | `json` | JSON editor (validate as you type, diff vs last published) | node/screen | `STU-064` |
| V2 | `actions` | Action graph editor | action graph | `STU-065` |
| V3 | `rust` | Rust function editor | function | `FN-033` |
| V4 | `l10n` | Localisation table for the plugin or selection | plugin/node | `STU-063` |
| V5 | `state` | State designer | state entry | `STU-063` |
| V6 | `data` | Data source designer (OpenAPI/GraphQL import) | data source | `STU-063` |
| V7 | `db` | DB schema designer | collection | `STU-063` |
| V8 | `theme` | Theme editor | theme | `STU-063` |
| V9 | `history` | History: snapshots, compare two, restore | plugin | `STU-069` |

## R4. Panels, drawer and popovers (not routes unless noted)

| # | Element | Where | Notes |
|---|---|---|---|
| E1 | Floating action bar | C1, C2 | undo/redo, zoom, device frame, theme, locale, text scale, send to device, validation status, history (V9), Publish (A1) (`STU-060`; AI assistant excluded, F24) |
| E2 | Left panel tabs | C2 | Insert, Layers, Screens, Assets, State, Data, Selection (`STU-061`) |
| E3 | Right panel tabs | C1, C2 | Properties, Actions, JSON, Code, Localisation, Accessibility, Performance, Docs (`STU-062`) |
| E4 | Bottom drawer | C1, C2 | 9 tabs, URL `drawer` + `device` (`STU-066`) |
| E5 | Plugin detail card | C1 | hover/focus popover (`STU-031`) |
| E6 | Lane menu | C1, C2 | open source action, open target, change trigger, redirect, remove (`STU-046`) |
| E7 | Trigger picker | C1, C2 | after a drag from a screen handle (F6) |
| E8 | Context menus | C1, C2, lists | plugin, screen, node, frame, arrow |
| E9 | Notifications popover | shell | latest + link to O14 |
| E10 | Organisation switcher | shell | popover |
| E11 | Help menu | shell | documentation links, shortcuts (D2), onboarding |
| E12 | Lock banner | C2 | holder, since when, Request edit (F1.6) |
| E13 | Status chips | shell | offline / reconnecting / N changes waiting (F4) |
| E14 | "Studio updated, reload" banner | shell | A5.9 |

## R5. Dialogs

### R5.1 Global (any page)

| # | Dialog | Size | Spec |
|---|---|---|---|
| D1 | Command palette (⌘K / Ctrl+K) | medium | `STU-013`, F15 |
| D2 | Shortcut sheet (`?`) | large | `STU-013` |
| D3 | WebAuthn step-up | small | `SEC-103`, F12 |
| D4 | Type-to-confirm (production-destructive) | small | `STU-074` |
| D5 | Break-glass justification | medium | `GOV-024` |
| D6 | Signed out — sign in again (opens P1 popup) | small | F4 |
| D7 | Unsaved changes on leave | small | F4 |
| D8 | Explain access (why is this disabled?) | medium | F14 |

### R5.2 Apps and library

| # | Dialog | Size |
|---|---|---|
| D9 | Import `.plux` | medium |
| D10 | Duplicate app | small |
| D11 | Export `.plux` | small |
| D12 | Archive app (confirm) | small |
| D13 | Project download (zip / Git / directory) | medium |
| D14 | Template visibility change + public scan result | medium |
| D15 | Import/export library file | medium |

### R5.3 Canvases (C1, C2)

| # | Dialog | Size | Spec |
|---|---|---|---|
| D16 | New plugin | small | `STU-034` |
| D17 | Plugin settings | medium | `STU-034` |
| D18 | Duplicate plugin | small | `STU-034` |
| D19 | Delete or deprecate plugin (blocking links + assisted fixes) | large | `STU-070`, `STU-073` |
| D20 | New frame / rename frame | small | `STU-035` |
| D21 | Leave and open ghost screen | small | `STU-053` |
| D22 | New screen (blank, from template) | medium | `STU-055` |
| D23 | Move screen to another plugin | medium | `STU-055` |
| D24 | Delete screen blocked (incoming links + fixes) | large | `STU-070` |
| D25 | Change entry plugin / entry screen | small | `STU-071` |
| D26 | Where used (blocked delete of component, state, key, source, collection, flow, function) | large | `STU-072` |
| D27 | Key change warning (affected generated APIs) | medium | `STU-076` |
| D28 | Save as template (2 steps) | medium | `STU-080` |
| D29 | Insert template: parameters + dependency mapping | large | `STU-083`, F8 |
| D30 | Extract to component | medium | `STU-084` |
| D31 | Component edit: instances that will change | large | `STU-084` |
| D32 | Adopt component version (diff, flagged overrides) | large | `STU-085` |
| D33 | Detach instance (confirm) | small | `STU-084` |
| D34 | Paste dependency report | large | F7 |
| D35 | Autosave conflict (diff, keep mine / take server's / copy) | large | F2 |
| D36 | Request edit / take over (confirm with holder and last activity) | small | F1.5 |
| D37 | Lock lost (re-take and apply / compare / discard) | medium | F1.4 |
| D38 | Send to device (pick paired device, parameters) | medium | `STU-060`, `DEV-011` |
| D39 | Pair a device (QR, ≤ 2 min token) | medium | `DEV-002` |
| D40 | Custom device frame | small | `STU-057` |
| D41 | Restore snapshot (confirm) | small | `STU-069` |

### R5.4 Publish, releases, approvals, experiments

| # | Dialog | Size | Spec |
|---|---|---|---|
| D42 | Acknowledge warnings | medium | `SRV-051` |
| D43 | Cancel release request | small | F11 |
| D44 | Rollout control (pause / resume / advance / schedule) | medium | `STU-101`, `REL-052` |
| D45 | Roll back (then D4 → approval or D5 → D3) | medium | `REL-006`, `STU-074` |
| D46 | Kill switch rule (target, environment, targeting) | large | `REL-060` |
| D47 | Approve / reject with comment (then D3) | medium | `STU-102` |
| D48 | Stop or ship experiment | medium | `ABT-003` |
| D49 | Feature flag create / edit | medium | `STU-103` |

### R5.5 Functions, localisation, design, data

| # | Dialog | Size | Spec |
|---|---|---|---|
| D50 | New function (starter template) | medium | `FN-033` |
| D51 | Deploy / promote function version (approval) | medium | `FN-007` |
| D52 | Edit alias | small | `STU-110` |
| D53 | Add locale | small | `STU-111` |
| D54 | Import translations (XLIFF, ARB, JSON, CSV) | medium | `I18N-009` |
| D55 | Export translations | small | `I18N-009` |
| D56 | Bulk action confirm | small | `STU-111` |
| D57 | Edit token / create brand overlay | medium | `STU-112` |
| D58 | Import design tokens (DTCG) | medium | `STU-112` |
| D59 | Import API definition (OpenAPI / GraphQL) | large | `DAT-002` |
| D60 | Edit mock | large | `DAT-080` |
| D61 | Collection migration preview | large | `STU-113` |

### R5.6 Devices, security, settings, admin, profile

| # | Dialog | Size | Spec |
|---|---|---|---|
| D62 | Revoke device | small | `STU-115` |
| D63 | Rotate or add signing key (then D3) | medium | `STU-115` |
| D64 | Edit certificate pin | medium | `STU-115` |
| D65 | Edit capability grant | medium | `STU-115` |
| D66 | Change security profile (then D3) | medium | `SEC-180` |
| D67 | Create environment / channel | small | `GOV-010` |
| D68 | Create or rotate secret (then D3) | medium | `GOV-010` |
| D69 | Grant app access to team or user | medium | `GOV-001` |
| D70 | Invite member | medium | `STU-116` |
| D71 | Create team | small | `GOV-001` |
| D72 | Create API/CI token (shown once) | medium | `STU-116` |
| D73 | Revoke session(s) | small | `GOV-005` |
| D74 | Export audit log | small | `STU-116` |
| D75 | Upload licence | small | `GOV-030` |
| D76 | Restore from trash / delete permanently (D4) | small | `GOV-031` |
| D77 | Change password | small | ADR-0026 |
| D78 | Add second factor (TOTP / passkey) | medium | ADR-0026 |
| D79 | Create personal access token (shown once) | medium | ADR-0026 |

## R6. Counts

| Kind | Count |
|---|---|
| Public pages (P1–P9) | 9 |
| Personal pages (U1–U7) | 7 |
| Organisation pages (O1–O31; O3 is a route modal) | 31 |
| Installation pages (I1–I7) | 7 |
| Canvases (C1–C2) | 2 |
| App pages (A1–A44) | 44 |
| **Routes in total** | **100** |
| Full-screen overlays (V1–V9) | 9 |
| Panels, drawer, popovers, banners (E1–E14) | 14 |
| Dialogs (D1–D79): small 29 · medium 37 · large 13 | 79 |

## R7. Deep links from notifications and other areas

| Source | Link |
|---|---|
| Lock requested / taken over | C2 `?screen` of the last edit |
| Publish finished / failed / blocked | A2 |
| Approval requested / decided / expired | O8 |
| Break-glass alert | O10 |
| Rollout paused by health gate | A4 |
| Kill switch used | A5 |
| Shell update required | A38 |
| New component version | A19 |
| Error in Analytics → "open node" | C2 `?screen&sel` |
| Problem in Problems drawer | C2 `?screen&sel` (+ `overlay` when inside an action graph) |
| Process-flow step | C2 `?screen&sel&overlay=actions&target` |
| Command palette result | the entity's canonical route |

## R8. Changes this implies to earlier decisions (applied when this file is accepted)

- **A4.9** (decisions file) said device frame, locale and theme live in the URL. Superseded by F10 and
  R1: preview switches live in the profile; the URL holds location (app, plugin, screen, anchor,
  viewport, overlay, drawer). A4.9 is reworded to match.

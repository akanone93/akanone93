<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# NEXT SESSION — start here (handoff of 2026-11-21)

> **TEMPORARY.** This file and the whole `handoff/` folder live only on the temporary branch
> **`claude/handoff-temp`**, pushed with `--no-verify` by the maintainer's explicit permission (the pre-push Go tests were
> not run on this commit). **Before anything is pushed to a permanent branch, delete `handoff/` (this file included)**,
> after moving what is worth keeping into real commits: the work-in-progress code and anything the maintainer wants
> kept. Then run the normal hooks; never use `--no-verify` again without a new explicit permission.

## 0. What this is

The previous session built the maintainer's **Apply Register** app (the "ALL-IN-ONE-APP" sheet, sections A–O) in Plux
Studio on a local stack. It fixed every platform defect it found on the way and logged each one in the build log. The
maintainer stopped the session to hand over. This file says what exists, what is done, what is left, in which order,
under which rules, and how to rebuild the live state, which is **not** in git.

Read, in this order:

1. `AGENTS.md`, then `CLAUDE.md` (the working agreement; binding).
2. This file, completely.
3. `docs/plans/handoff-studio-fixes/all-in-one-build.md`: the build log. It holds every defect found (#1–#176), its
   cause and its fix or decision. It is the source of truth for issue numbers.
4. `docs/WORKLOG.md` › Hand-off notes: the newest three notes (2026-11-19) cover the latest work in prose.
5. `handoff/ALL-IN-ONE-APP.md`: the maintainer's sheet (their copy, with their own ✔ / - marks).

## 1. The maintainer's standing rules (verbatim intent; they override convenience)

- **Security first, always. No reduction in security, ever.** The platform will run financial systems. Performance,
  efficiency and security problems are **blockers**: fix them, do not defer them silently.
- **Never print a secret** (tokens `plux_…`, invitation codes, one-time codes, passwords, keys) in a command's output,
  log or screenshot. Every browser script redacts `plux_[a-z]+_[A-Za-z0-9]+` and never prints alert or status text that
  may hold a one-time code. The previous session broke this rule twice: it revoked the printed CLI token and voided the
  printed invitation codes. Do not repeat it.
- **No `--3way` with `git apply`.**
- **Small decisions are yours; big ones go to the maintainer** (AGENTS.md §3 and §7: spec or ADR deviations, security
  defaults, dependencies, CI gates, schema and bundle formats). Ask with a short question, the options and a
  recommendation. The maintainer prefers **short, direct answers** (often "in a single sentence").
- **Committing and pushing are approved** for this work (the maintainer said so), on the working branch. The Stop hook
  asks for commit and push at the end of each turn.
- **Never edit the repository while a `git push` is running.** The pre-push hook runs the whole Go test suite (about
  15 minutes) and fails with "files were modified by this hook" if any file changes meanwhile.
- **Send progress screenshots** to the maintainer as the app grows (SendUserFile, rendered).
- Record every defect found in the build log (next free number: **#177**). Root-cause it, then fix it or put it to
  the maintainer.
- Quality bar: "high quality, high security", with readable code and docs that match the code (AGENTS.md §3, keeping
  documents and code in agreement).

## 2. Git state

| Branch | State |
|---|---|
| `claude/handoff-studio-fixes` | The working branch. All committed work is here, pushed, head **`ae1b0390`**. About 174 commits ahead of `main`. Continue here, or on the branch the session instructions name. |
| `claude/handoff-temp` | **Temporary.** `ae1b0390` plus one commit that adds `handoff/` (this payload). Do not merge it as is. |
| `main` | Protected; the maintainer merges. |

The last commits on `claude/handoff-studio-fixes`, newest first:

| Commit | What | Build log |
|---|---|---|
| `ae1b0390` | fix(studio): limit a restriction to an app and environment | #172 |
| `b304712f` | fix(studio): store token values by type and uploads by kind | #174, #175 |
| `9d0031a7` | feat(audit): list the audit log newest first | #171 |
| `63bf4c01` | fix(tenancy): void and renew invitations with the membership | #169 |
| `16460506` | fix(gateway): require gateway.manage and a step-up for changes | #167, ADR-0108 |
| `682762fd` | feat(studio): manage environments and secrets in administration | #165 |
| `9b5f9ff9` | perf(builder): cache dependencies per organisation on the server | #161, ADR-0107 |
| `929b8916` | fix(builder): isolate each build in a private view with Landlock | #164, ADR-0106 |
| `d6a25ffb` | perf(builder): share build output, cache dev builds, fix #157 #162 #163 | |
| `fe2915e6` | fix(functions): verify crate archives, store them once, pin allowlist | #159, #160, ADR-0105 |

Each of these passed its gates when it was made: the Go tests in the pre-push hook, the Studio tests, typecheck,
lint, catalogues, `golangci-lint`. The handoff commit was not tested.

## 3. Work in progress — NOT committed, NOT tested (`handoff/wip-uncommitted/`)

These are whole-file drafts of files under `studio/apps/web/src/` (copy each over its counterpart). They were based
on `ae1b0390`. They have not been run, type-checked, linted or extracted. Finish them first: they are small.

| Draft | Goes to | Purpose |
|---|---|---|
| `use-admin.ts` | `features/admin/use-admin.ts` | `useLicensed(feature)`: reads `LicenseService.GetLicenseState`. It returns false when the licence is absent, invalid or expired or lacks the feature, and undefined while unknown or when the reader may not read it (`organization.manage`, no step-up), so the server decides. |
| `parts.tsx` | `features/admin/parts.tsx` | `LicenceNote`: says a feature needs an enterprise licence, in place of its form, and links to `/o/$org/admin/license`. Also holds `CreateForm` `children` (already committed with #172). |
| `roles-pages.tsx` | `features/admin/roles-pages.tsx` | **#173**: Roles shows `LicenceNote` instead of **New role** when `custom-roles` is not licensed; the role builder locks a new or custom role then. |
| `compliance-page.tsx` | `features/admin/compliance-page.tsx` | **#173**: the report form becomes `LicenceNote` when `compliance-reports` is not licensed; earlier reports stay downloadable. |
| `access-page.tsx` | `features/admin/access-page.tsx` | **#177** (new, not yet in the build log): **Explain access** asked for raw user and app IDs and answered with grant and restriction IDs. It also hid the server's refusal behind "failed to load". The draft picks person, permission, app and environment by name; names the grants (role in org or app) and restrictions (patterns in app › env, with the reason); and shows refusals with `<Refusal>`. |
| `admin.messages.ts` | `features/admin/admin.messages.ts` | New messages for the above. `admin.accessUser` becomes id `admin.accessPerson` "Person"; `accessApp` and `accessEnv` are removed. |
| `admin.test.tsx` | `features/admin/admin.test.tsx` | Tests: O20 licence note (absent, without the feature, expired), the builder locked; O28 licence note; O23 names grants and restrictions; O23 asks by name and records the question. |
| `test-admin.ts`, `test-upstream.ts` | `src/` | Fakes: `license.features`; `explainAccess` records userId, appId and environment when set. |
| `admin.schema.ts` | `features/admin/admin.schema.ts` | Unchanged copy (for reference). |

To finish them:

1. Copy them in.
2. Run `cd studio && apps/web/node_modules/.bin/lingui extract --clean --overwrite`.
3. Add the **de** and **ar** translations of the new message IDs in `studio/apps/web/src/locales/{de,ar}/messages.po`
   (the strict compile needs them). Then run `lingui compile --strict` and delete the generated `messages.js`.
4. Run the tests from the **studio root**: `bun test apps/web/src/features/admin/admin.test.tsx
   apps/web/src/features/installation/installation.test.tsx apps/web/src/shell.test.tsx`. Running from
   `apps/web` loses the DOM preload.
5. Run `bun run typecheck` and `bunx biome ci .`.
6. Add the build log rows: #173 becomes fixed, and its wording must be corrected, because Studio has **no
   change-freeze page**; freezes are CLI and API only (`plux freeze`, `GovernanceService.PutFreeze`). Add #177 as a
   new row.
7. Commit.

Note: SSO and SCIM already show the licence refusal and hide the SCIM form (`sso-page.tsx`), so they need nothing.
Multi-stage approval policies (`multi-stage-approvals`) are not gated up front yet; consider them when doing §J2.

## 4. Sheet progress (ALL-IN-ONE-APP.md)

Legend: **done** = built and checked live; **part** = some items left (listed); **todo** = not started.

| Section | State | Notes and what is left |
|---|---|---|
| A1 `ar-api` | done | The fake Go service (REST, GraphQL, WebSocket, SSE, minimal JWT), source in `handoff/apply-register/ar-api/`. Runs on `127.0.0.1:8090`; every development upstream points there. |
| A2 server side | done | 25 gateway upstreams per environment (development → ar-api; staging and production → `https://api.apply-register.example/v1`); the user identity provider for staging and production; config values `feeTable`, `supportPhone`, `ratesEURUSD` (strict), `payeeIbanRelief` (server-bound); secrets `AR_GATEWAY_KEY` and `AR_PARTNER_KEY` in all 3 environments. Publishing items remain with J. |
| A3 host app | todo | Needs a first release (publish, the 0.0.1 rule in AGENTS.md §5). |
| A4 development apps | todo | |
| B1 organisation | part | Done: teams `platform`, `orgs`, `qa`; members Release Manager, Translator and Auditor (their invitation codes were voided after being printed by mistake, so none can sign in yet: re-invite them, without printing the codes); a CI token; session idle at the registry's 30 minutes; the restriction "Designers may not publish to production" on **team Orgs**: `release.publish, release.promote` in Apply Register › production. Left: approval policies (§J2); the change freeze (enterprise licence; CLI only); the custom translation-reviewer role (enterprise licence, #173); the audit export test (CEF, #170: Studio has no form to add a destination, use the API); retention defaults check. |
| B2 create app | part | The app exists (`apply-register` in organisation `acme`). The maintainer's marks: locale `am-ET` could not be chosen at creation; the icon remove-and-restore did not work for them. Card actions, settings and limits usage are unchecked. |
| B3 app document | part | `app.json` is complete in the project (see `handoff/apply-register/project/app.json`); the walk-through of the JSON editor is not ticked. |
| B4 environments, variables, secrets | done | development, staging, production; secrets set (names in the inventory). |
| B5 security profile | todo | Check against the sheet. |
| C1 tokens | done | `theme.json`. |
| C2 themes and brands | part | Brand colours and radius overrides done. Left: per-brand fonts and typography; the high-contrast accessibility check (#26: the canvas has no high-contrast switch); the brand preview switch. |
| C3 typography, host fonts, icons | part | Host fonts done (Inter, Noto Naskh Arabic, Noto Sans Ethiopic, IBM Plex Sans, JetBrains Mono, Source Serif 4; files in `handoff/apply-register/fonts/`). Icons are left: task #42, rebuild the icons tab (Material, Cupertino, SVG, external sets). |
| C4 assets | part | All assets uploaded (`handoff/apply-register/assets/`); the oversize refusal was verified; GIF and SVG accepted. Left: the meter turns amber at 80 %; Replace keeps uses; delete names its widget count; the webp variant list. New uploads now go to `fonts/`, `motion/`, `images/` (#175); earlier uploads keep their `images/…` names. |
| D1 components | done | `project/components/`. |
| D2 templates | todo | |
| E functions | done | 13 Rust functions in `project/functions/` (bank.eligibility, essay.stats, fees.quote, hooks.orgSync, partner.submit, pass.issue, pay.simulate, promo.check, reminders.daily, run.finishTime, slots.availability, slots.book, workshops.capacity), built in the sandboxed builder and deployed with aliases and triggers (see the inventory). |
| F1 plugins and frames | done | 17 plugins on the app canvas. |
| F2 links and arrows, F3 safety and lifecycle | todo | |
| G0–G17 plugins and screens | done | All 17 plugins (launch, explore, passes, account, verify, pay, partner, console, bank, campus, cityrun, devconf, clinic, relief, homes, arena, maintainers) built and validated; canvas defects found here are in the build log (many still open, see §6). |
| H localisation and accessibility | todo | Translations exist (`project/translations/`); the H checks are not done. |
| I tests | todo | |
| J publish, operate, ship | todo | J1 publish first (unblocks A3); J2 approvals (also B1). |
| K, L, M, N, O appendices | todo | Feature, widget, enum, action and platform checklists. |

## 5. Priorities, in order

1. **Finish §3** (#173 and #177 drafts), then log #177.
2. **Rebuild the live state** (§7) and check it: sign in, open the app canvas, a few plugins, the Administration pages.
3. **§J1 publish → first release 0.0.1**, which unblocks §A3 and §B1's policies (§J2). From the first release on, the
   bundle format, manifest, schema and API evolve additively only (AGENTS.md §5).
4. The rest of the sheet, in order: §A3, A4, B1 (rest), B2/B3/B5 checks, C2, C3 icons (#42), C4 checks, D2, F2, F3,
   H, I, J, then K–O.
5. The open platform issues (§6), security and performance first. Each found defect is a build-log row.
6. Android-side checks: screenshots of the built Dev app and runtime on a device or emulator.

## 6. Open issues (build log numbers; see the log for details)

- **Studio administration:** #166 (CLI `plux <group> --help` says unknown subcommand), #168 (policies under All apps
  show nothing), #170 (no form to add an audit export destination), #173 (drafted), #177 (drafted).
- **Tooling:** #176. A push's Go tests can exceed Go's 10-minute package timeout in `internal/devsession`, because
  every integration test migrates its own schema under one global migration lock. Fix proposal: migrate once into a
  template schema and copy it per test, or set explicit timeouts. This is a test-infrastructure change for the
  maintainer to decide.
- **Studio canvas fidelity** (open rows): #5, #26, #27, #44–#46, #58, #59, #67–#70, #73–#77, #80, #81, #116–#119,
  #122–#128, #138, #156.
- **Runtime, compiler and registry:** #48 (cubicBezier token is null on the device), #130, #131, #135–#137, #141,
  #148, #149.
- **Assets and functions:** #111.
- **Product gap to put to the maintainer (#178, to log):** the `.plux` app export (ADR-0102) carries documents and
  assets only. Environments, configuration values, gateway upstreams, the identity provider, secrets and function
  deployments do not move with an app, so rebuilding an app elsewhere needs the manual steps of §7.

## 7. Rebuilding the live state (the dev database is NOT in git)

What was live: one installation on the previous container (gone). Organisation **`acme`** (the dev organisation) holds
the app **`apply-register`** plus the dev samples `demo`, `starter` and `live-console`.
`handoff/apply-register/db-inventory.md` lists **every** setting read from that database: organisations, apps,
environments, teams, members, role grants, restrictions, secret names, environment variables, config values,
upstreams (with operations and bindings), identity providers, security profiles, plugins, assets, function versions,
aliases and triggers, releases and channels, policies, freezes, export destinations, webhooks, workload identities,
token names, retention and limits, shell settings. Secrets, hashes, sessions, keys and token values were never
exported.

What `handoff/apply-register/` holds:

| Folder | Content |
|---|---|
| `project/` | The CLI project (`plux.json`, `app.json`, `theme.json`, `native-catalogue.json`, `components/`, `translations/`, 17 `plugins/`, 13 `functions/` without build output). This is the app's source of truth. |
| `export/` | The app as exported by the server (document layout of ADR-0102's `project/`), same content as `project/` minus functions. |
| `bundles/` | Compiled app and plugin bundles (`.pxb` and source maps) from the earlier build. |
| `assets/` | Every uploaded asset (images, webp, svg, gif, Lottie, Rive). |
| `fonts/` | The six host font files. |
| `ar-api/` | The fake API service source and zip (`go run .` serves on :8090; see its README). |
| `db-inventory.md` | The settings inventory described above. |

Steps:

1. **Start a stack.** The documented way is `make setup`, then `make dev` (compose: server, builder, fnrunner,
   egress, Postgres, BFF; see `docs/guides/` and `deploy/compose/README.md`). The builder needs the sandbox settings
   of ADR-0106 (seccomp profile, unmasked `/proc`, AppArmor where present, Landlock).

   The previous session instead ran natively, in this order:
   - Postgres 16 cluster `/var/tmp/plux-pg/data` on 127.0.0.1:55432.
   - `plux-server serve -config plux-server.yaml` from `/var/tmp/plux-dev`, with `PLUX_DATABASE_URL` pointing at
     that cluster; HTTPS on 8443, and a small forwarder for 8080.
   - The BFF: `cd studio && bun apps/bff/src/main.ts` with `PLUX_BFF_API_UPSTREAM=https://localhost:8080`,
     `NODE_EXTRA_CA_CERTS=<dev CA>`, a 48-character `PLUX_BFF_STATE_SECRET` and
     `PLUX_BFF_PUBLIC_BASE_URL=http://localhost:8081`. It serves the **built** Studio
     (`cd studio/apps/web && bun run build` after any Studio change).
   - The line `127.0.0.1 plux-builder plux-fnrunner plux-egress` in `/etc/hosts`; it is lost on every restart.
2. **Sign in** as the dev owner (`dev@plux.localhost`, password from `make dev`'s `dev.env`) and register a security
   key. Step-up actions need it: secrets, gateway, restrictions, roles. Browser automation uses a virtual WebAuthn
   key; see the scripts' approach in the build log. The old key file was **not** carried over (it held a private
   key).
3. **Create the app** `apply-register` in the dev organisation and load the project. Either:
   - From `handoff/apply-register/project` with the CLI: `plux login`, set `plux.json`'s server, organization and
     app to the new ones, then `plux publish` / `plux import` (check `plux <cmd> -h`; #166 means a group's
     `--help` fails, use the command alone); or
   - Studio's **Import app** from a `.plux` archive made from `export/` (ADR-0102 layout:
     `plux-archive.json` + `project/<path>`).
4. **Upload the assets** (`assets/`) and **host fonts** (`fonts/`, then App › Design › host fonts: Inter, Noto Naskh
   Arabic, Noto Sans Ethiopic, IBM Plex Sans, JetBrains Mono, Source Serif 4).
5. **Organisation:** teams `platform`, `orgs`, `qa`; invite `release@`, `translator@`, `auditor@apply-register.example`
   with roles release-manager, translator and auditor (never print the invitation codes); create the restriction
   (team Orgs, `release.publish, release.promote`, In app Apply Register, In environment production, reason "Designers
   may not publish to production (§B1)").
6. **Environments** development, staging and production (created with the app). In Administration › Environments
   and secrets, set `AR_GATEWAY_KEY` and `AR_PARTNER_KEY` in each. The **values are not in the handoff**: for
   development use the values `ar-api` expects (see `ar-api/README.md` and `auth.go`); for staging and production any
   placeholder.
7. **Gateway:** recreate the upstreams, identity provider and config values exactly as `db-inventory.md` lists them
   (CLI `plux config …` and the gateway commands, or the API; all need `gateway.manage` and a step-up, ADR-0108).
8. **Functions:** `plux fn deploy` from `project/functions` (each needs the sandboxed builder). Then set aliases and
   triggers as the inventory lists.
9. **CI token** for the CLI (Administration › Tokens): shown once, never print it.
10. **Check:** the app canvas shows 17 plugins; Problems is clean apart from the known warnings; a function test run
    passes.

## 8. Known pitfalls

- After a container restart: start the Postgres cluster first; re-add the `/etc/hosts` line; rebuild Studio for the
  BFF after Studio changes.
- `make studio-i18n-check` fails when de or ar translations are missing (strict compile). Run Studio tests from
  `studio/`.
- `@plux/ui` buttons disable with `aria-disabled`, not `disabled`. Base UI selects need a full press sequence in
  tests (see `choose()` in `admin.test.tsx`).
- `schema/limits.json` and `auth/permissions.json` must stay sorted; regenerate with `make gen` (or `go generate` in
  `internal/auth`), never by hand.
- Custom roles, change freezes, compliance reports, SAML and SCIM need an enterprise licence; this community
  installation has none (`GetLicenseState` = absent).
- The pre-push hook takes about 15 minutes; do not touch files meanwhile.

## 9. Before pushing to a permanent branch

1. Move the finished WIP into real commits (with tests, catalogues and build-log rows).
2. **Delete `handoff/`**: this file, the app payload and the drafts. Keep the app payload only if the maintainer
   asks for it in a permanent place.
3. Add a WORKLOG hand-off note (keep ten; move the oldest to `docs/worklog-archive.md`).
4. Run the gates (`make check-changed`; `make check` for the whole repository) and push with hooks.

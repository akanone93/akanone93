<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Handoff: Studio fixes found while building "hello plux" (temporary)

Temporary hand-off note for the next session. Delete this folder once the work below is merged.

## What we are doing

The maintainer built a minimal host app ("hello plux") against a local `make dev` stack and hit a chain of
bugs in Studio and the dev stack. Each was root-caused, fixed with tests, and pushed. The goal of this
branch is to finish verifying every fix **live in a browser against a real server**, then hand back one
patch.

Branch history: the work started on `claude/dreamy-dirac-diipaj`; this hand-off branch
(`claude/handoff-studio-fixes`) carries all of it plus this note.

## Done (commits, oldest first)

| Commit | What it fixes | Verified |
|---|---|---|
| `b5e909ee` fix(dev): seed token | `make dev` could not publish: publish/promote need a WebAuthn step-up and nobody can register a security key. `plux-server seed` now also mints a 15-minute `seed` token (user-bound, counts as stepped up, migration `0067_seed_tokens.sql`); `mk/stack.mk` uses it for the two seed publishes and then deletes it from `dev.env`. | Unit + DB integration tests; live: seed + publish + promote ran against a real server. **The maintainer already applied this one as `dev-seed-stepup.patch`.** |
| `ce1b1d46` fix(studio): dev session environment | Studio called `DevSessionService/CreateSession` without an environment → 400 on every window focus; pairing, Send and the device tabs never connected. Now opens it in the app's `development` environment (else first non-production) and does not refetch on focus. | Unit tests; live: the developer console shows **Live**. |
| `c93c9921` fix(studio): assets under the app lock | Asset uploads/deletes (Assets panel, device screenshots, screen thumbnails) were sent with the tab's undo-tag ID instead of the app editing lock's session → `FinishAssetUpload` 400. Now sent under the app lock; thumbnails upload only while this tab already holds it. | Unit tests; **live not yet checked**. |
| `665720d5` fix(studio): Monaco features before first editor | Monaco 0.57 loaded its editor features lazily after the first editor, so their services were "UNKNOWN" and every editor threw. `json-editor/monaco-features.ts` loads them first (JSON and Rust editors). | Unit test that fails without the fix; production build; **live not yet checked**. |
| `b35be536` fix(studio): translations, all-or-nothing edits, toasts, CSP | See below. | Unit tests; **live not yet checked**. |
| `d5b49915` fix(studio): content slot first on insert; dev stack fixes; docs | Inserting into an empty Scaffold now fills `body` (content slot first: `body`, `child`), not `appBar` (`panels/model.ts` `freeSlot`). `make dev` stops and removes `dev.env` when seeding fails. `init-secrets.sh` adds `PLUX_ANALYTICS_READER_PASSWORD` to an older `.secrets/`. Guides (canvas, localisation, run-locally) and `WORKLOG.md` updated. | Unit tests (`panels/insert-slot.test.ts`); script tested on a fake old `.secrets/`; **live not yet checked**. |

### The last commit in detail

1. **Translations never saved (root cause, confirmed live).** Every save of the app's translation documents
   (keys, messages) and the theme was refused with `PLX-1005: an undo step needs an editor session`: those
   `EditorProvider`s sent an undo step ID without the tab's editor-session ID. Fix: `EditorProvider` tags every
   write with the tab's editor session by default (`editor/editor-provider.tsx`). The test server
   (`test-upstream.ts`) now enforces the same rule; without the fix the theme editor tests fail.
2. **Multi-document edits apply as a whole or not at all.** When the server refuses a save:
   - `EditSession` reports the refused undo steps and drops the rest of those steps in that document
     (`editor/edit-session.ts`);
   - `UndoManager.abandon()` rolls the steps back in every other document as `undo:<step>` writes and drops them
     from the stacks (`editor/undo-stack.ts`);
   - `EditorProvider` shows an error toast with the server's reason (`editor.save-refused`).
   - Key extraction (inline text edit, the PLX-1609 quick fix, the extract dialog) now waits until the key and
     its message are **stored** before the page refers to them (`EditSession.whenSaved`,
     `localisation/extract-key.ts`); a page can no longer point at a missing key.
   - Tests: `editor/atomic-steps.test.ts`, new cases in `localisation/extract-key.test.ts`.
3. **Toasts were never shown.** `<Toaster>` was never mounted, so every `notify()` in Studio was silent. It is
   now mounted in `app.tsx` (region label `notify.toasts`).
4. **CSP.** A live sweep of the routes found one remaining `style-src` violation: Base UI's hoisted `<style>`
   (selects, scroll areas) without the nonce. `@plux/ui` now exports `StyleNonce` (Base UI `CSPProvider` with the
   page nonce), mounted at the root. Sonner and Monaco were already handled in `vite.config.ts`.
5. Catalogues: `editor.save-refused` and `notify.toasts` added in en/ar/de; `lingui compile --strict` passes.

## The patch

`studio-fixes.patch` (sent to the maintainer separately) is
`git diff b5e909ee d5b49915 -- . ':!docs/plans/handoff-studio-fixes'`: every fix **after** the seed-token patch the
maintainer already applied, without this hand-off folder. Apply from the repo root:

```bash
git apply --3way ~/Downloads/studio-fixes.patch
```

Progress: every fix in the table is complete, unit-tested, documented and pushed. Not done here (at the
maintainer's request, to save time): the live browser pass and the full Studio suite (`cd studio && bun run test`,
which takes longer than 15 minutes here). The changed suites passed: editor, localisation, panels, plugin canvas,
design, devices, JSON and Rust editors; type check, `biome ci` and the strict catalogue compile passed.

## Remaining tasks

1. **Verify every fix live, in one pass** (the maintainer asked for one combined check after all fixes):
   - dev session: console shows Live, no `CreateSession` 400;
   - assets: upload an image in the Assets panel; thumbnails appear after an edit; no `FinishAssetUpload` 400;
   - Monaco: open the JSON editor and the Rust editor; no "depends on UNKNOWN service" errors;
   - translations: (a) double-click a literal text, type, Enter → a key appears in Localisation, the message in
     `en`, the canvas shows the text (not a UUID); (b) Localisation › Add key; (c) the PLX-1609 quick fix;
     (d) switch the preview locale to ar/de;
   - theme: change a colour token in the theme editor; it saves (it was refused by PLX-1005 too);
   - refusal: force one save to fail (for example revoke the lock or patch an invalid value) → a toast appears and
     every document of that edit is back to its previous state, on the server too;
   - CSP: sweep the routes (org home, app canvas, settings, publish, library, plugin canvas with JSON/Code/Preview
     open) and confirm zero `securitypolicyviolation` events.
2. Run the full Studio suite once: `cd studio && bun run test` (long; run it in the background).
3. Multi-step flows: edits of one action that share a `group` (paste style, typing bursts), the history restore,
   the library actions and key extraction share one undo step, so a refusal rolls all of them back. Editors without
   an undo manager (the app canvas's `EditorProvider`) are not covered: a refused save there undoes only its own
   document. Check moving a screen to another plugin and a rename that updates links, live.
4. Open issues, not fixed (bigger features, ask the maintainer first): no Studio page registers a security key
   (the WebAuthn registration API exists, nothing calls it), so nobody can step up in Studio; `plux login` points
   at `<server>/device`, which does not exist; Layers does not show which slot a child is in.
5. Update the `docs/WORKLOG.md` note of 2026-11-08 with what the live pass found, and delete this folder once
   merged.

## Running the stack here without Docker

The cloud container has no Docker. What worked (PostgreSQL 16 is installed):

```bash
S=/var/tmp/plux-pg; B=/usr/lib/postgresql/16/bin; mkdir -p $S && chown postgres $S
su postgres -c "$B/initdb -D $S/data -A trust -U postgres && $B/pg_ctl -D $S/data -o '-p 55432 -k $S -c listen_addresses=127.0.0.1' -l $S/log start"
# then, as postgres in db "plux": CREATE ROLE plux (NOSUPERUSER NOBYPASSRLS, a password you choose), ALTER SCHEMA public OWNER TO plux,
# and the plux_analytics_reader role, as deploy/compose/postgres-init.sh does.
```

`live/plux-server.yaml` is the server config used (filesystem storage, memory cache, file signing, plain HTTP on
`127.0.0.1:8080`); it takes the database URL from `PLUX_DATABASE_URL`
(`postgres://plux:<password>@127.0.0.1:55432/plux?sslmode=disable`). Then:

```bash
cd backend && go build -o /var/tmp/plux-dev/plux-server ./cmd/plux-server && go build -o /var/tmp/plux-dev/plux ./cmd/plux
cd /var/tmp/plux-dev && ./plux-server serve -config plux-server.yaml &
./plux-server seed -config plux-server.yaml -out - > dev.env
cd studio && bun run --filter @plux/web build
PLUX_BFF_API_UPSTREAM=http://localhost:8080 PLUX_BFF_STATE_SECRET="$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=')" \
  PLUX_BFF_PORT=8081 PLUX_BFF_PUBLIC_BASE_URL=http://localhost:8081 bun apps/bff/src/main.ts &
```

`live/*.cjs` are the Playwright drivers used (sign in, open the plugin canvas, edit a text in place, sweep CSP).
They read `/var/tmp/plux-dev/dev.env` and record console errors and failed `/api` responses with their bodies,
which is how the PLX-1005 refusal was found. Rebuild the web app (`vite build`) after source changes: the BFF
serves `apps/web/dist`.

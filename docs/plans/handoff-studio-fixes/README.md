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

## Hello-world pass (2026-11-09)

The maintainer's console on the plugin canvas, while assigning a translation to a `Text`, showed: an empty key
picker, a PXL `pxl.NewEnv` error, 139 `style-src` violations, a blocked asset image, `WatchEvents` 502s, a 401 and a
404. Reproduced live here (native stack, Playwright: new blank app → Center → Text → literal → key):

| Finding | Cause | Fix |
|---|---|---|
| The **Translation key** list under Properties › Data is empty, so a text cannot use an existing key | The pickers read only the plugin's documents; the keys (`translations/keys.json`) and the theme are app-level | `RightDock` takes `sourceDocuments`: the app's documents with the canvas's sessions over them (messages, theme, keys), so a key added on the canvas is offered at once (`panels/properties-model.ts` `projectSources` reads a session's `parsed` copy). Live: the list shows `home.title`, then `home.hello` right after extracting it; the canvas draws the message. |
| `scope: pxl.NewEnv: type "PluxAppState": declare either enum members or fields` on every canvas of an app with no state | The validator WASM returns the scopes as JSON with `fields,omitempty`, so an empty state type became `{}`; the language service read it back as neither enum nor object | `pxl.TypeSpec.Fields` is `omitzero`: an empty object type keeps `"fields": {}` (`backend/internal/pxl/env.go`; test `TestEmptyObjectTypeRoundTrips`, which fails with the old tag with the exact console message). |
| `DevSessionService/WatchEvents` 502 | The watch sends its headers only with its first event; Bun's fetch in the BFF gives up on a response without headers after about 5–6 minutes (measured: `TimeoutError` at 360 s) | The BFF proxies streams with Bun's `timeout: false` (`apps/bff/src/proxy.ts`); test "a stream is proxied without Bun's fetch timeout"; live: one stream held open for 7 minutes. |

Not fixed (not blocking the hello-world flow; ask the maintainer):

- **139 `style-src` violations**: none here with this branch (zero `securitypolicyviolation` events on org home, app,
  plugin canvas, Assets, Localisation view). They match the nonce-less `<style>` elements `StyleNonce` (`b35be536`)
  fixes; the maintainer's build most likely predates it.
- **Asset images blocked** (`img-src`): with `objectStorage.cdnBaseURL` set (compose: `https://localhost:8080/v1/objects`)
  asset variants carry absolute cross-origin URLs, which Studio's CSP (`img-src 'self' data: blob:`) and COEP
  (`require-corp`) both refuse. Only thumbnails and image previews are affected. Options: serve object URLs through
  the BFF (`/api/v1/objects/…`, same origin), or allow the object origin in `img-src` and send
  `Cross-Origin-Resource-Policy: cross-origin` from the object store.
- **401 `GetCurrentUser`** on the sign-in page: Studio asking whether a session exists; expected, harmless.
- **404**: not reproduced (the URL was not in the log); a missing CanvasKit gives a 404 on
  `/assets/canvaskit/canvaskit.js` when the web app is built without `scripts/stage-canvaskit.ts` (`make studio-build`
  stages it).

Verified on the device side without a device: `plux validate` and `plux build` of the exported draft (0 errors; the
app bundle carries the messages, the plugin bundle the key reference), and `flutter test
test/l10n/locale_runtime_test.dart` (7 passed).

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
4. Open issues: Layers does not show which slot a child is in. *Fixed 2026-11-09 (maintainer's go-ahead):* security
   keys are registered under **Profile › Security keys**; `plux login`'s `/device` page is Studio's
   (`features/auth/device-login-page.tsx`), never approves on arrival and asks the person to confirm they started
   the sign-in; `server.studioBaseURL` lets the WebAuthn ceremony run on Studio's own origin (dev: `:8081`) under
   the public URL's relying party; a session without a second factor cannot enrol a key on an account that has one.
5. Deferred fixes (after the hello-world check): `inline-text.test.ts` hangs (its sessions never save, and extraction
   now waits for the save, b35be536); asset images under `cdnBaseURL` are blocked by Studio's CSP and COEP; `make setup`
   on a fresh Ubuntu/WSL stops at pip (`No module named pip`) and at `install-flatc` (needs cmake): it should check for
   and install `pipx` and `cmake g++` itself (apt, with sudo) when they are missing. `make dev` fails with
   `permission denied … docker.sock` when the user is not in the `docker` group: setup should check Docker access
   and print the fix (`sudo usermod -aG docker $USER`, or Docker Desktop's WSL integration for the distribution).
   `make dev` skips seeding whenever `deploy/compose/.secrets/dev.env` exists, even when the Docker volumes are new
   (a reset Docker or WSL): the old token is not in the new database and `dev-starter` fails with PLX-8012. Seeding
   should check the token against the server (or record the volume it seeded) and seed again when it is stale.
   `plux init --app <key>` resolves the key for the files it writes, but its closing doctor check calls `GetApp` with
   the raw key and reports `FAIL app … PLX-1005` (`cmd/plux/cmd_project.go` doctor `app` step); it should use the
   resolved ID.
   On Windows with the pub cache on C: and the app on D:, the Android build fails in Kotlin's incremental compiler
   ("this and base files have different roots"); the host-app guide (and `plux init`) should add
   `kotlin.incremental=false` to `android/gradle.properties`.
   **Dropped refusal reasons (audit every area):** several Studio screens turned a server refusal into a generic
   message, so the way past it was never offered: the publish start page dropped PLX-6200 (fixed, `a4b683d2`), the
   build step showed only "Failed" without the job's diagnostics (fixed), and the publish steps still say "The step
   failed. Try again." for any other code (PLX-8090 included). Sweep every `catch`/`isError` in `apps/web/src` and
   pass the error to `Refusal`/`PageStatus` or show its code and message. Also: the publish start page's refusal reads
   "This page shows sensitive settings", which is wrong for an action; `RequestVisualDiff` answers 500 (PLX-8090,
   "job kind is not registered: releasereq.render") when no render worker is configured instead of saying so.
   **Problems misses app-level findings:** the canvas's live check (`compiler.Validator.ValidatePage`) keeps only
   diagnostics whose file is the open page, so translation findings (`PLX-1606` missing message, `PLX-1608` unused
   key, in `translations/*.json`) appear only when a publish compiles everything, where they fail the build
   (`PLX-8051`). Problems should show the app's and plugin's own findings too. The publish flow's builds also fail on
   any warning although the Reports step is where warnings are acknowledged (`SRV-051`): maintainer to choose between
   building with warnings allowed and acknowledging at Reports (recommended) or acknowledging before the build.
6. Update the `docs/WORKLOG.md` note of 2026-11-08 with what the live pass found, and delete this folder once
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
cd studio && bun scripts/stage-canvaskit.ts && bun run --filter @plux/web build   # needs `flutter precache --web` once
PLUX_BFF_API_UPSTREAM=http://localhost:8080 PLUX_BFF_STATE_SECRET="$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=')" \
  PLUX_BFF_PORT=8081 PLUX_BFF_PUBLIC_BASE_URL=http://localhost:8081 bun apps/bff/src/main.ts &
```

`live/*.cjs` are the Playwright drivers used (sign in, open the plugin canvas, edit a text in place, sweep CSP).
They read `/var/tmp/plux-dev/dev.env` and record console errors and failed `/api` responses with their bodies,
which is how the PLX-1005 refusal was found. Rebuild the web app (`vite build`) after source changes: the BFF
serves `apps/web/dist`.

# Studio redesign — hand-off (rounds 2–4, 2026-11-01)

Branch `handoff/studio-redesign-r4`, cut from `feat/studio-redesign`. It holds every change of the
Studio redesign rounds 2–4 as one commit, plus this folder. Continue on this branch, or merge it into
`feat/studio-redesign`.

## What was done

- **Round 2 (access and navigation)** — `patches/studio-access-nav-and-pages.patch`.
  - Permissions are organisation-scoped: the current user is cached per organisation; the root route
    sets the request organisation from the URL; preferences and notifications fall back to the home
    organisation (`lib/session.ts`, `lib/guards.ts`, `lib/use-current-user.ts`, `router.tsx`).
  - Fixed the query deadlock that left navigation changing only the URL (`fetchUser` vs `loadUser`).
  - Sidebar with Home, Approvals, Library, Administration and Installation areas on every page
    (`shell/app-sidebar.tsx`, `shell/nav-area.tsx`); notifications in a header popover.
  - Profile and shortcuts pages redesigned; Get started moved to a header dialog; command palette
    reworked; animated filtering; 403 and licence pages render inside the shell.
- **Round 3 (colour, cards, footer)** — `patches/studio-colour-cards-sidebar.patch`.
  - Compact app cards with a hover detail card, footer, link buttons, sidebar on installation pages.
- **Round 4 (this session)** — `patches/studio-glass-canvas-refusals.patch`.
  - Theme: shadcn `zinc` + `indigo` (`packages/brand/src/shadcn-theme.ts`), no washes or glows,
    small solid colour tiles, stronger frosted glass (`styles/surfaces.css`, `.glass`), the shell's
    planes a clear step apart (sidebar darkest, page raised with edge and shadow, footer band).
  - Refusals: `components/refusal.tsx` explains a refused read (PLX-6200 step-up with passkey
    confirmation, PLX-8011 second factor, PLX-8600 licence, permission, retry); wired through
    `PageFrame`/`PageStatus` and every area's `Frame` via an `error` prop. SSO and SCIM, Sessions
    and Compliance failed in dev for these reasons, not for missing permissions.
  - App cards: the whole card is the hover-card anchor; the detail card opens beside it and is a
    link to the app canvas.
  - App canvas rebuilt (`features/app-canvas/`): full-window scene with floating glass panels
    (`canvas-chrome.tsx`: app bar, toolbar with Select/Hand tools, Plugins list box, zoom controls,
    `PluginActions`), the detail panel as an inspector (`box-list.tsx` `DetailCard`), keyboard
    shortcuts (`use-canvas-keys.ts`), zoom state (`use-zoom.ts`), theme-aware scene colours read
    from CSS tokens (`scene-theme.ts`), a DOM marquee, selection drawn in the scene.
  - Root causes fixed on the app canvas: pointer positions were measured against the canvas element,
    which the canvas host enlarges and moves with a transform (now the page section's box); the
    wheel listener was passive, so Ctrl+wheel zoomed the browser page (now native, non-passive);
    a canvas element can be transferred to a worker once, so a theme change gets a new element
    (`key={theme.background}`); `browserSurface` has no overlay layer, so selection and marquee
    were invisible; resize is observed on the page, not the canvas (the host resizes the canvas).
  - Docs: ADR-0103 amended, spec 1.13.0 (`STU-010`, Appendix J, revision row),
    `studio-development.md` §4.1/§4.3, guides `canvas.md`, `apps.md`, `security-and-admin.md`.
  - Locales `en`, `de`, `ar` filled; `lingui compile --strict` passes.
  - Tests added: refusals (`admin.test.tsx`), panels and gestures (`app-canvas.test.tsx`).

## Verified (2026-11-01)

`make studio-lint`, `studio-bounds`, `studio-typecheck`, `reuse-lint`, `spec-lint` exit 0;
`bun test` of `features/app-canvas` (43 pass), `features/admin` (23 pass), `packages/brand`
(51 pass, WCAG AA pairs), `features/apps` and `features/home`; catalogues equal a fresh extract;
screenshots in `shots/` (light and dark home, hover card, canvas selected, dragged, panned,
SSO and Sessions refusals).

## Not verified

`make studio-cover` (full suite with floors, stopped early), `studio-build` size budget
(`studio-size`), `make studio-e2e`, `make trace`, `make check`. Run them first.

## Open items

1. The plugin canvas (`features/plugin-canvas/plugin-canvas-page.tsx`) measures pointer positions
   against the transformed canvas element too, and uses React's passive `onWheel`: the same zoom
   and hit bugs fixed on the app canvas.
2. Arrows on the app canvas were not seen against real documents (the mock had no navigate links).
3. Snap guides are not drawn in the browser (no overlay layer in `browserSurface`).
4. Stale-cookie 401 on sign-in: fix in `backend/internal/api/authn.go` `credential()` waits for the
   maintainer's approval (authentication change).
5. Dense editors still use native checkboxes (themed through `surfaces.css`).
6. `docs/WORKLOG.md` has a short note for this hand-off; the full rounds 3–4 notes are this file.

## How to run Studio and take screenshots

1. Bun 1.3.11 is the pinned version (`bun --version`); the container had it unpacked at a scratch
   path, any 1.3.11 works.
2. Build: `cd studio/apps/web && bun run build`.
3. Start the BFF against a dead upstream (the browser's API calls are answered by the in-memory
   upstream in the test):
   `cd studio && NODE_ENV=production PLUX_BFF_PUBLIC_BASE_URL=http://127.0.0.1:8081 PLUX_BFF_API_UPSTREAM=http://127.0.0.1:9 PLUX_BFF_STATE_SECRET=<value> bun run apps/bff/src/main.ts &`,
   with `<value>` the test state secret that `studio/e2e/playwright.config.ts` passes to its BFF
   (a test value, not a credential).
4. Copy `tools/zz-shots.*` into `studio/e2e/`, then `cd studio && bunx playwright test --config e2e/zz-shots.config.ts`
   (`SHOTS=canvas,home,refus` picks groups). Shots land in `/tmp/claude-0/shots/`. Delete the
   `zz-*` files before committing; Biome rejects them.

## Applying the patches elsewhere

Each patch is incremental on the one before (round 2, then 3, then 4) on top of `629f5ec0`. The
round-4 patch repeats the Dockerfile change of `dockerfile-gen-icons.patch`; if that one is already
applied, use `git apply --exclude=studio/Dockerfile studio-glass-canvas-refusals.patch`. This
branch already contains all of them.

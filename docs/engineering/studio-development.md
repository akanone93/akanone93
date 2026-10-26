<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Studio Development

How Plux Studio (`studio/`) is built: structure, design system, localisation and direction, state, canvas, testing, performance, security, and the rules that keep agents' context and token use small. It is binding for every contributor, human or agent. The specification says *what* Studio does (spec §21, `STU-001`–`STU-123`); [typescript-standards.md](typescript-standards.md) says how TypeScript is written; this document says how Studio is put together.

The stack is fixed by `STU-001` (with the wording changes of ADR-0014): Bun as runtime, package manager, test runner and BFF server; Vite on the Bun runtime as the build tool; strict TypeScript; React; TanStack Router, Query and Form; shadcn/ui on Base UI; Tailwind CSS v4; Monaco for full-screen editors and CodeMirror 6 for inline PXL fields; Connect-ES; Plux Canvas on CanvasKit. A package is installed only once it is on the [allowlist](dependencies.md).

Rules are numbered so a review can cite them ("studio-development §6.3"). A rule that no longer serves the project is changed here in a pull request, never ignored.

---

## 1. Principles

1. **The device is the truth; Studio is the instrument.** The canvas previews; the conformance-tested layout engine (`STU-005`) and the paired device decide.
2. **Every claim has a test or a measurement.** A UI that "looks right" is not evidence. Behaviour is proven by assertions on the DOM, the scene model or numbers; performance by benchmarks against `STU-120`–`STU-123`.
3. **One source per fact.** Tokens come from `@plux/brand`, strings from the message catalogue, commands from the command registry, contracts from `@plux/schema` and the generated Connect-ES clients. Nothing is copied.
4. **Direction- and language-agnostic by construction.** Every screen works in left-to-right and right-to-left and in any Studio locale from its first commit. Retrofitting is not allowed.
5. **Small, named, findable.** A feature is a folder with a known shape and a short README, so a reader — or an agent — finds the right file without exploring.
6. **Cheap to verify.** The test is the cheapest way to see a change working. Screenshots are the most expensive and are not used by agents without approval (§13).
7. **Protect the server.** Work that reaches the server is bounded: rate limits, registry limits (`LIM-001`), capped rebuilds, queued jobs. A feature that adds server work states how it is bounded.

## 2. Architecture

Studio lives entirely under `studio/`: its apps in `studio/apps/` and its TypeScript packages in `studio/packages/`. The root `packages/` holds the Dart and Flutter packages and is never used for Studio code.

| Path | Owns | May import |
|---|---|---|
| `studio/apps/web/` | The React single-page application: shell, routes, areas, panels, overlays, the development-only catalogue page | every `packages/*` |
| `studio/apps/bff/` | The stateless Hono backend-for-frontend on Bun: the OpenID Connect callback and SAML assertion consumer pages (the server has no `/auth/*` routes; sign-in is its IdentityService and FederationService RPCs, proxied like any other, L-13 of the P11 plan), pass-through of the server's session cookie and CSRF token, CSP nonces, API and stream proxy, rate limits, version reporting, the error/metrics endpoint (`STU-002`, `SEC-101`) | `packages/schema`, `packages/api`, `packages/i18n` (server-side locale negotiation) |
| `studio/packages/canvas/` | Plux Canvas: CanvasKit scene graph in the canvas worker, hit-testing, arrows, raster caches, the message interface (`STU-003`, §21.5) | `packages/layout`, `packages/brand`, `packages/schema` |
| `studio/packages/layout/` | The layout engine ported from Flutter, text through SkParagraph (`STU-005`, `STU-006`); no DOM | `packages/brand`, `packages/schema` |
| `studio/packages/validator/` | The Go validator compiled to WebAssembly and the PXL language service, run in a worker (`PXL-008`, `STU-064`) | nothing in `studio/` |
| `studio/packages/api/` | Generated Connect-ES clients (`*_pb.ts`) | nothing in `studio/` |
| `studio/packages/ui/` | Shared components on shadcn/ui and Base UI, the component catalogue and `*.cases.tsx` (§5) | `packages/brand`, `packages/i18n` |
| `studio/packages/i18n/` | Studio's own message catalogues, locale negotiation, direction, formatting (§7) | nothing in `studio/` |
| `studio/packages/brand/` | Brand tokens (spec Appendix J) and contrast utilities | nothing in `studio/` |
| `studio/packages/schema/` | Generated contracts: document types, limits, registries (`*.gen.ts`) | nothing in `studio/` |

2.1. **Imports flow downwards only.** `studio/packages/*` never import from `studio/apps/*`; `studio/packages/canvas` and `studio/packages/layout` never import React or `studio/packages/ui`. A policy check enforces the table with the first P11 package.

2.2. **Studio never reaches the database or object storage** (layering rule L-7). Every call goes browser → BFF → API through the generated Connect-ES clients; no hand-written `fetch` to the API. Loading Studio's own static files (WebAssembly, fonts, catalogues) from the installation is not an API call.

2.3. **The browser never holds a token** (`SEC-101`): no OAuth token, API token or secret in JavaScript, `localStorage`, `sessionStorage`, IndexedDB or URLs.

2.4. **Spec map.** Each area of spec §21 has exactly one home:

| Spec | Area | Home in `studio/apps/web/src/areas/` |
|---|---|---|
| §21.2 | Shell, command palette, profile settings | `shell/` |
| §21.3 | Apps grid | `apps-grid/` |
| §21.4–§21.5 | App canvas, bundled arrows | `app-canvas/` (scene in `packages/canvas`) |
| §21.6 | Plugin canvas | `plugin-canvas/` |
| §21.7 | Screen editor, panels, overlays, bottom drawer | `screen-editor/` |
| §21.8 | Safety rules | `safety/` (used by the areas above) |
| §21.9 | Components and templates | `library/` |
| §21.10 | Releases, approvals, experiments | `releases/`, `approvals/`, `experiments/` |
| §21.11 | Functions, localisation, design system, data, analytics, devices, admin | one folder each, named after the area |

2.5. **Threads.** The main thread runs React, the document store and undo, input capture and the accessible DOM tree. Plux Canvas (layout, CanvasKit, scene, hit-testing, caches) runs in one dedicated worker drawing through `transferControlToOffscreen()`. The validator and the PXL language service run in their own worker. Workers talk through typed message interfaces only; the canvas code also runs on the main thread as the fallback for browsers without worker WebGL.

2.6. **Live data.** Server → browser data (notifications, lock changes, publish progress, device streams) uses Connect server streaming through the BFF: one events stream per tab, one stream per paired device, HTTP/2, resumable from a cursor. Browser → server commands are unary RPCs. No WebSocket.

## 3. Feature folders

3.1. **One folder per feature, one shape.**

```
areas/<area>/<feature>/
  README.md            purpose, requirement IDs, public API, files — at most 40 lines
  index.ts             the only export point
  <Feature>.tsx        the root component, with its private sub-components
  <part>.tsx           a sub-component, once it is reused or its own unit
  use<Thing>.ts        hooks: data, commands, derived state
  <feature>.messages.ts  message descriptors for this feature (§7.4)
  <feature>.test.tsx   component tests (§12)
  model.ts             pure logic, no React, no I/O
  model.test.ts
```

3.2. **The README is the map.** It lists the requirement IDs the feature implements, what it exports, and one line per file. It is updated in the same change as the code; a stale README is a bug.

3.3. **Size guidelines.** These are recommendations, not gates. Code that belongs together may stay in one file; split when a part is reused, tested on its own, or the file becomes hard to navigate. A reviewer may ask for a split beyond these sizes, and the author says why the code stays together.

| Item | Recommended size |
|---|---|
| Source file (not tests, not `*.gen.ts`) | about 300 lines |
| Component function | about 120 lines, including JSX |
| Other functions | about 60 lines |
| Props of a component | about 8; more often means the component does two jobs or needs a grouped prop |
| JSX nesting inside one component | about 6 levels |

The one enforced limit is Biome's `noExcessiveCognitiveComplexity`, already an error in `studio/biome.json`.

3.4. **Names.** Components `PascalCase.tsx`; hooks `useCamelCase.ts`; pure modules `kebab-case.ts`; tests next to the file with `.test.ts(x)`; catalogue cases `<Component>.cases.tsx`. Names say what a thing is in the product's vocabulary (spec §35 glossary): `BundledArrow`, not `CurvyLine2`.

3.5. **No barrel chains.** Only a feature's or package's `index.ts` re-exports, and only its public API.

## 4. Design system

4.1. **Three token layers.** Brand tokens (`@plux/brand`, spec Appendix J) → semantic tokens (`surface.canvas`, `text.primary`, `accent.selection`, `status.warning`, `radius.panel`, `elevation.overlay`) → component tokens where a component needs its own. Components use semantic or component tokens, never brand tokens directly, so a theme or a brand overlay changes one layer. The same token source feeds Tailwind and Plux Canvas.

4.2. **No raw values.** No hex, `rgb()`, `hsl()`, pixel, `rem` or shadow literals in components, and no Tailwind arbitrary values (`bg-[#…]`, `p-[13px]`, `rounded-[…]`). The Tailwind v4 `@theme` is generated from the tokens and Tailwind's default values are reset, so only token-backed utilities exist. A lint rule enforces this with the first component. No runtime CSS-in-JS.

4.3. **The visual language is Porcelain** ([studio-ui-language.md](studio-ui-language.md), accepted by the maintainer 2026-10-22, with the reference page [studio-ui-reference.html](studio-ui-reference.html)): a cool, whitish workspace with a dark theme and a system setting; generous radii and pills; three floating glass capsules, floating panels and a floating drawer over the canvas; Electric Violet only for selection, focus and the primary action; colour only for meaning (selection, status, arrows); dense, information-rich presentation through a fixed set of patterns (status pills, metric strips, meters, sparklines). Studio follows `STU-010`, `STU-060`–`STU-062` and Figure 21-2 as written. A new colour use states its meaning in the token's name; a new pattern is added to the UI language and the reference page before it is built.

4.4. **Themes and density** (`STU-014`): light, dark and system; compact density by default and comfortable as the option, both as token sets (CSS variables), not conditional class names. A high-contrast theme exists for the canvas preview (`STU-112`) and is offered for the shell.

4.5. **Contrast is computed, not eyeballed.** Every text/background and UI-component token pair is checked with the `@plux/brand` contrast utilities in both themes: 4.5:1 for text, 3:1 for large text and components (`STU-011`). The check is a unit test over the token tables.

4.6. **Typography.** The wordmark uses Sora SemiBold (Appendix J). Instrument Serif names things (screens, panels, sections, plugins), Instrument Sans carries controls and sentences, DM Mono carries every number with a unit, identifier, path and code (OFL-1.1, self-hosted; UI language U7). Arabic uses the pairing the maintainer decides (UI language U7).

4.7. **Motion** comes from duration and easing tokens (arrow expansion 150–200 ms, `STU-044`). No animation library: canvas animations run in Plux Canvas; panels and dialogs use CSS transitions and Base UI state attributes. Every animation honours `prefers-reduced-motion` by reducing to an instant change.

## 5. Components

5.1. **Reuse before writing.** Before creating a component, search the catalogue (§5.2). A second component that does the job of an existing one is rejected in review.

5.2. **The catalogue.** `studio/packages/ui/CATALOGUE.md` lists every shared component: name, one-line purpose, props, variants, keyboard behaviour, whether its icons mirror in RTL, and when *not* to use it. Each shared component has a `<Component>.cases.tsx` with its states (loading, empty, error, long text), rendered by the development-only catalogue page. A new or changed shared component updates its entry and its cases in the same change; a policy check compares the entries with the package's exports.

5.3. **Components render; they do not decide.** Business rules — safety rules (§21.8), validation, permissions, link derivation — live in `model.ts` or hooks and are unit-tested without React. A component that contains an `if` on product rules is split.

5.4. **Every state is designed.** Each data-driven component handles loading, empty, error and success, and long text, in every Studio locale. The four states map to the canvas mock states of `STU-058`, so the same vocabulary is used in both places.

5.5. **Accessible by construction** (`STU-011`). Semantic elements first, Base UI primitives for composite widgets, a visible focus ring from tokens, an accessible name for every control, and labels from the message catalogue. No `div` with a click handler; no positive `tabindex`. Every drag has a keyboard alternative.

5.6. **Keyboard first.** Every action is reachable by keyboard. Arrow keys inside lists, menus and trees follow the reading direction (§8.4).

5.7. **Commands are registered once.** Every user-invocable action is a command in the command registry: ID, message key for its label, shortcut, availability predicate and handler. The command palette (`STU-013`, built on Base UI Dialog and Autocomplete), the shortcut sheet (`?`), menus and toolbars are generated from the registry; no shortcut is bound anywhere else. Each command maps to the server permission that guards its RPC; a command no permission covers gets a new permission with its server check in the same change, and there is no browser-only permission (M-7, plan [p11.md](../plans/p11.md) §6.2). A test checks for duplicate shortcuts per scope.

5.8. **No `dangerouslySetInnerHTML`**, no `innerHTML`, no `eval`-like APIs (§11).

## 6. State and data

6.1. **Two kinds of state** (`STU-004`). Server state lives in TanStack Query (through Connect Query), keyed by resource and document revision. Local UI state lives in component state or one small Zustand store per area, created inside the area's React provider. No module-level mutable state (AGENTS.md §8). Live streams write into the area's store, not into the Query cache.

6.2. **Edits are JSON Patch.** Document edits are applied optimistically and autosaved as JSON Patch (`SRV-030`) with a 500 ms debounce by default (`STU-004`); a rejected patch rolls back and shows the diagnostic. Pending edits flush when the tab is hidden, through the generated client with a `keepalive` fetch transport.

6.3. **Undo is a model, not a feature of each component** (`STU-069`): every edit goes through one edit pipeline (Zustand + Immer) that records the forward and the inverse patch. A component never mutates the document directly. The pipeline is the single stream that feeds autosave, undo, the validator worker and the canvas worker. Undo applies the inverse patch as a new write; after a reload the stack is rebuilt from the server's snapshots of the editing session, never from browser storage.

6.4. **Contracts are generated.** Types come from `@plux/schema` and the Connect-ES clients in `@plux/api`; a hand-written duplicate of a contract type is rejected.

6.5. **Errors are reasons, not strings** ([error-handling.md](error-handling.md)). The UI branches on the error reason and shows the localised message for that reason; it never parses `message`. Diagnostics show their code and JSON path and link to the node.

6.6. **Limits come from the registry** (`LIM-001`): editors read limits from `@plux/schema`'s generated limits; no size or count is hard-coded in the UI.

## 7. Studio localisation

Studio's own interface is localised and runs left-to-right and right-to-left (`STU-012`); the language is chosen in profile settings (`STU-014`). This section covers Studio's chrome. Localising the *apps users build* is spec §17 and is previewed on the canvas (§8.5).

7.1. **Locales.** Studio ships in English (`en`, the source locale), German (`de`) and Arabic (`ar`), which proves right-to-left layout (`STU-012`, ADR-0094); Arabic is also the product's RTL launch locale (`I18N-002`). Adding a locale is data only: a catalogue file and an entry in the locale list, no code change.

7.2. **No literal UI text.** Every user-visible string — labels, placeholders, `aria-label`, `title`, tooltips, toasts, errors, empty states, document titles — comes from the message catalogue. A lint rule rejects string literals in JSX text and accessibility attributes with the first component. Product names (Plux, Plux Canvas) and user content are the only exceptions.

7.3. **ICU MessageFormat.** Messages use ICU MessageFormat with plural and select forms (as `I18N-001` requires for apps), through Lingui; plural rules, numbers, dates, relative times and lists use `Intl` with the active locale. No string concatenation to build sentences; no manual plural `s`.

7.4. **Messages are declared next to their feature** in `<feature>.messages.ts`: explicit stable key (`<area>.<feature>.<name>`), English source text and a description for translators (Lingui message descriptors). Catalogues per locale are compiled at build time from the declarations and the translations; strict extraction makes unused and missing keys fail `make studio-check`.

7.5. **Switching language** happens without a reload: the shell updates `<html lang>` and `<html dir>`, every mounted string re-renders, and focus stays where it was. The choice is stored in the user's profile on the server; before sign-in, the BFF negotiates from `Accept-Language`, and the user can override it on the sign-in page.

7.6. **Fallback.** A missing translation falls back `de-AT` → `de` → `en` and is reported in development and in tests; a key is never shown to a user.

7.7. **Pseudo-locales.** Studio ships two pseudo-locales in development and tests: accented and 40% expanded (`en-XA`), and mirrored right-to-left (`ar-XB`), matching what `I18N-010` gives app authors. Behaviour tests and the Playwright locale sweep run in `en`, `en-XA` and `ar-XB` (§12.4).

7.8. **User content is isolated.** App, plugin, screen and person names can be in any script; they are rendered with `dir="auto"` or `<bdi>` so a Hebrew app name in an English list, or a Latin name in an Arabic list, does not reorder the surrounding text.

7.9. **Studio's strings are managed like an app's.** The catalogues import and export as XLIFF 2.0 and JSON (`I18N-009`), so Studio's own translation can use the translation workflow Plux gives its users.

## 8. Direction (LTR and RTL)

8.1. **Logical properties only.** CSS and Tailwind use logical properties: `margin-inline-start`, `padding-inline-end`, `inset-inline-start`, `text-align: start`, `border-start-start-radius`; in Tailwind `ms-*`, `me-*`, `ps-*`, `pe-*`, `start-*`, `end-*`, `text-start`, `text-end`, `rounded-s-*`, `rounded-e-*`. Physical left/right properties and utilities (`ml-*`, `mr-*`, `pl-*`, `pr-*`, `left-*`, `right-*`, `text-left`, `text-right`, `float-left`) are rejected by a lint rule with the first component.

8.2. **Direction is read, never assumed.** Code that needs the direction (popover placement, drawer side, drag gestures) reads it from the direction provider (Base UI); no `dir === "rtl" ? … : …` scattered through components.

8.3. **Icons.** Studio's chrome uses one icon set, Phosphor duotone, through the `Icon` component (UI language U8). Icons with a direction (back, forward, chevrons, undo and redo arrows, "send", reply) mirror in RTL; icons without one (play, search, check, the Plux logo, clocks, media controls) never mirror. The set carries no mirror metadata, so the catalogue marks each icon `mirrors: true | false`.

8.4. **Keyboard follows reading direction in the chrome.** In lists, tabs, menus and trees, the "next" arrow key is Right in LTR and Left in RTL.

8.5. **The canvas does not mirror with the chrome.** The infinite canvas is a spatial design surface: its coordinates, the positions of screens and plugin boxes, rulers, zoom and keyboard nudging stay physical whatever the Studio language. What a device frame *shows* follows the **preview locale** (`STU-058`): an Arabic Studio user previewing an English screen sees it left-to-right, and the reverse. Studio direction and preview direction are two independent settings and are never derived from each other.

8.6. **Panels mirror.** The docked left and right panels swap sides in RTL; the spec's "left panel" and "right panel" (`STU-061`, `STU-062`) mean *start* and *end* panels.

8.7. **Numbers, code and identifiers stay LTR.** JSON, PXL, Rust, IDs, URLs, version numbers and keyboard shortcuts are rendered in an LTR-isolated span inside RTL text. Monaco and CodeMirror editors are always LTR.

## 9. Plux Canvas

9.1. **Off React's render path and off the main thread** (`STU-003`). React renders panels and chrome; the canvas renders the scene in its own loop in the canvas worker (§2.5). A property edit reaches the scene only as JSON Patch from the edit pipeline (§6.3), never through React props.

9.2. **The scene is queryable.** The canvas exposes a read-only scene model — nodes, frames, bounds, arrows, lanes, anchors, selection, z-order — for tests and for accessibility, as an asynchronous snapshot on the main thread fed by the worker's back channel. Tests assert on the scene model; nothing reads pixels back from WebGL to decide whether a test passes.

9.3. **Layout is proven by conformance vectors** (`STU-005`): positions and sizes compared with headless Flutter within the defined tolerance, across device frames, locales and text scales. The canvas is never "fixed" by tweaking it until it looks like the device.

9.4. **Text metrics** use the device's font files and SkParagraph through CanvasKit, built from the Skia revision the pinned Flutter uses, so line breaks and metrics match the device; where metrics are still not guaranteed (for example a substituted font), the canvas says that the device preview is authoritative (`STU-006`). Apple's SF Pro is never served by Plux: it comes only from the user's own installation through the Local Font Access API, otherwise a substitute is used.

9.5. **The canvas is accessible.** The scene model drives an accessible tree (a parallel DOM of focusable items with names) so screen readers and keyboard users can select, move and inspect nodes (`STU-011`).

9.6. **Context loss is handled.** WebGL context loss is expected: the canvas rebuilds its GPU resources from the scene model without losing the selection or the view.

9.7. **Design surface, not a runtime** (`STU-068`). Actions that navigate, call networks or functions are never executed on the canvas.

## 10. Performance

10.1. **Budgets are requirements**: 60 fps pan and zoom with 100 screens or 200 plugins (`STU-120`, `STU-045`); property change on the canvas within 50 ms p95 (`STU-121`); Largest Contentful Paint ≤ 2.5 s and initial JavaScript < 1.5 MiB compressed (`STU-122`); overlay open ≤ 150 ms after load (`STU-123`).

10.2. **Each budget has a benchmark** in CI from the change that first affects it. CI runs without a GPU: it tracks GPU-independent costs (layout time, draw-call count, CPU time per frame) and fails on a regression over 10% against its baseline (`QA-007`). Frame-rate budgets are confirmed on the reference laptop (spec §30.1) before each release and recorded in `docs/benchmarks/`. A performance claim without a benchmark run in the same session is not made.

10.3. **Code splitting.** Areas, overlays, Monaco, CodeMirror, the validator and the canvas load on demand; the canvas is prefetched while the apps grid is open. A new dependency in the initial bundle states its compressed size in the pull request; the bundle-size check fails over budget.

10.4. **Overlays preserve the canvas** (`STU-063`): opening one pauses the canvas without unmounting or re-rendering it; closing restores zoom, scroll and selection exactly.

10.5. **Measure before memoising.** `memo`, `useMemo` and `useCallback` are added where a profile shows they pay for themselves, and the pull request says so.

10.6. **Static assets are cheap to serve.** Hashed files are cached for a year as `immutable` and served Brotli-precompressed from the build; `index.html` is `no-cache`; no service worker.

## 11. Security

11.1. **No credential reaches JavaScript** (`SEC-101`, `STU-002`): the server issues the `__Host-plux_session` cookie (`HttpOnly`, `Secure`, `SameSite=Strict`) and the CSRF token (ADR-0026); the stateless BFF passes them through and keeps no session store; CSRF token on state-changing requests.

11.2. **Strict CSP with nonces and Trusted Types.** No inline scripts or style elements without the nonce; inline `style` attributes are allowed (`style-src-attr 'unsafe-inline'`) because Monaco sets them, never style elements or sheets (ADR-0014 Revision 2026-10-26); `script-src` allows `'wasm-unsafe-eval'` (never `'unsafe-eval'`) for CanvasKit and the validator; `worker-src` allows Studio's own workers; Trusted Types are required for scripts and exactly one policy exists, `plux-worker`, which approves only Studio's own workers (the canvas, the validator and the lock heartbeat), each by an exact per-worker allow-list of its hashed same-origin asset (maintainer, 2026-10-25); no `default` policy; Monaco and workers are loaded in a way the policy allows; Base UI receives the nonce through its `CSPProvider`; Zod runs with `jitless: true`; no `eval`, `new Function` or string timers, in our code or a dependency's.

11.3. **Everything pasted, imported or dropped is untrusted** (security-practices §3): clipboard content (`STU-067`), `.plux` archives, templates, OpenAPI files, images and translation files are validated by the API before use and never rendered as HTML.

11.4. **Sensitive fields** (`SCH-012`) are masked in editors, excluded from logs, telemetry and error reports, and never copied into URLs, the clipboard or browser storage in clear.

11.5. **No third-party scripts, fonts or analytics at runtime.** Everything Studio loads is served by the installation (air-gapped installs, P9). Telemetry goes only to the installation's own BFF and logs; it is sampled, rate-limited and can be switched off by an administrator.

11.6. **Step-up and type-to-confirm** for destructive and production actions (`STU-074`, `SEC-103`) are implemented once, as shared components, never per feature.

11.7. **Language servers are sandboxed.** rust-analyzer runs in the function build sandbox with build scripts and procedural macros off, no network and resource limits, one sandbox per session.

## 12. Testing

12.1. **Levels.**

| Level | What it proves | Location | Runs |
|---|---|---|---|
| Unit | Pure logic: models, safety rules, arrow routing, layout maths, formatting | next to the code, `*.test.ts` (`bun test`) | `make studio-cover` |
| Component | A component's behaviour through the DOM: roles, names, states, keyboard | next to the code, `*.test.tsx` (`bun test` + happy-dom + Testing Library) | `make studio-cover` |
| Contract | Studio against the generated Connect-ES clients with an in-memory transport | next to the hook | `make studio-cover` |
| Layout conformance | The canvas engine against headless Flutter (`STU-005`), CanvasKit headless under `bun test` | `test/layout-conformance/` | CI |
| Locale sweep | Every catalogue component and feature root in `en`, `en-XA`, `ar-XB`: overflow, direction, missing strings (§12.4) | Playwright over the catalogue page | CI |
| End-to-end | A user flow in a real browser against the Compose stack | `test/e2e/studio/` (Playwright) | CI |
| Performance | The budgets of §10 | `test/bench/studio/` (Playwright traces) | CI |

12.2. **Assert on meaning, not appearance.** Queries use roles and accessible names ("the button named *Publish*"), never CSS classes, test IDs on everything, or DOM structure. A test that would pass with the wrong label is wrong.

12.3. **Requirement IDs in test names** in the bracket form ([testing.md](testing.md) §3): `test("blocks deleting a screen with incoming links [STU-070]", …)`.

12.4. **Every component is checked in three locales**: `en`, `en-XA` (expanded) and `ar-XB` (RTL). Behaviour tests run in all three. Text overflow (content size against box size) needs real layout, which happy-dom does not compute, so it is checked by the Playwright locale sweep over the catalogue page for every catalogue component and feature root. A check fails when text overflows its container, when a physical-direction style is applied, or when a string is missing from the catalogue.

12.5. **Accessibility checks are automated**: every component test also checks names, roles and focus order; Playwright runs axe in both themes and in RTL; end-to-end flows are run keyboard-only once per flow.

12.6. **Visual regression is CI-only.** Where a pixel comparison is the only possible oracle (the canvas renderer itself), CI compares images with a tolerance and reports the numbers. The images are artifacts for humans; agents read the numeric result, not the images (§13.1).

12.7. **Coverage floors** for `studio` are in `coverage.json` and are never lowered (`QA-001`).

## 13. Agent rules: context and token budget

Frontend work is where agents waste the most context: screenshots, page dumps from browser tools, and reading whole component trees. These rules keep Studio work cheap and verifiable. They apply to every agent session that touches `studio/`, and to humans directing agents.

13.1. **No screenshots without approval.** An agent does not take, request or read screenshots, screen recordings, rendered images of the UI, or image files (PNG, JPEG, WebP, GIF, AVIF) while working on Studio. An exception needs the maintainer's explicit approval **in the current session**, stating what the image is for; approval covers that one purpose only. Cases where asking is reasonable:
   - a canvas rendering defect that cannot be observed through the scene model (§9.2);
   - a CI visual-regression failure whose numbers do not explain it (§12.6).

   Even with approval: one image at the smallest size that shows the problem, cropped to the region, never a full-page capture. Visual review is the maintainer's: they pull the branch and run Studio locally.

13.2. **Verification is textual.** An agent proves a UI change with, in order of preference: unit tests, component tests (§12), scene-model assertions, end-to-end tests with role-based assertions, and the accessibility tree as text.

13.3. **Browser tools write to files.** Browser automation runs through the test runner or a command-line driver whose snapshots, traces and logs go to files under the session's scratch directory; the agent reads only the failure messages. Tools that stream full DOM snapshots or images into the conversation after every step are not used.

13.4. **Read narrowly.** Start from the spec map (§2.4) and the feature's README (§3.2); search before opening a file; open line ranges, not whole files, when the location is known; never read whole directories. Do not read `bun.lock`, `*.gen.ts`, `*_pb.ts`, coverage output, build output or `node_modules` unless the task is about them.

13.5. **Quiet commands.** Run `make` targets and tests with reporters that print failures only; on failure, read the first error before rerunning. Do not paste long logs back into the conversation.

13.6. **Reuse first, by catalogue.** Read `studio/packages/ui/CATALOGUE.md` before writing markup; a shared component is used by name instead of re-writing its classes.

13.7. **Design input as text.** Designs reach agents as text: tokens, the catalogue, the UI language, the reference page as HTML source, written specs, measurements and annotated wireframes in Markdown or SVG source. An image mock-up is converted to such text by a human before an agent works from it, or falls under §13.1. Every session that changes Studio's interface loads the design skill (`.claude/skills/plux-studio-design/`, drafted in [p11-studio-design-skill.md](../plans/p11-studio-design-skill.md)).

13.8. **One feature per session.** An agent session works on one feature folder and the shared packages it needs; work that spreads across areas is split into sessions, each starting from the work log.

## 14. Definition of done for a Studio change

In addition to [workflow.md](workflow.md) §6:

- [ ] `make studio-check` passes; any benchmark the change affects ran and is within budget.
- [ ] Tests name their requirement IDs; behaviour tests run in `en`, `en-XA` and `ar-XB`, and the locale sweep passes; strings exist in `en`, `de` and `ar`.
- [ ] No literal UI string, raw style value or physical-direction property was added.
- [ ] Every new string has a key, English text and a translator description; German and Arabic are provided or tracked.
- [ ] The feature README and, for shared components, the catalogue entry and cases are current.
- [ ] New commands are in the command registry with a shortcut-collision check.
- [ ] Keyboard-only use and screen-reader names are covered by tests.
- [ ] Any new server work is bounded (§1.7) and any new dependency is on the allowlist.
- [ ] The interface is built from catalogue components in the UI language; the design skill's checklist is complete.
- [ ] No screenshot was used, or the maintainer's approval and its purpose are noted in the pull request.

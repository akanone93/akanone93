<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Apply Register — build sheet

One app, seventeen plugins, 133 screens, every canvas feature. Organisations publish registration flows; people find an
organisation, register, verify, pay and follow their registration. Each organisation is its own plugin, built
differently on purpose, so that together the plugins use every widget, property option, action, data kind,
state kind, trigger, page kind, security control and Studio canvas feature Plux has today.

All organisations are **fictional demos** (replace them with real ones as they sign up). Names, logos and texts
here must never imitate a real company.


## Contents

- [0. How to read this sheet](#0-how-to-read-this-sheet)
- [1. The product in one page](#1-the-product-in-one-page)
- [A. Outside Studio](#a-outside-studio)
- [B. Organisation and app](#b-organisation-and-app)
- [C. Design system (app › Design)](#c-design-system-app--design)
- [D. Library — build once, reuse everywhere](#d-library--build-once-reuse-everywhere)
- [E. Functions (Rust → WebAssembly)](#e-functions-rust--webassembly)
- [F. App canvas](#f-app-canvas)
- [G. Plugins and screens](#g-plugins-and-screens)
  - [G0. For every plugin and every screen (do once per item; not repeated below)](#g0-for-every-plugin-and-every-screen-do-once-per-item-not-repeated-below)
  - [G1. launch — Launch (entry plugin, team platform)](#g1-launch--launch-entry-plugin-team-platform)
  - [G2. explore — Explore](#g2-explore--explore)
  - [G3. passes — Passes](#g3-passes--passes)
  - [G4. account — Account](#g4-account--account)
  - [G5. verify — Verify (reusable; typed in, typed out)](#g5-verify--verify-reusable-typed-in-typed-out)
  - [G6. pay — Pay (reusable wallet payment)](#g6-pay--pay-reusable-wallet-payment)
  - [G7. partner — "Bring your organisation"](#g7-partner--bring-your-organisation)
  - [G8. console — Organisation console (staff)](#g8-console--organisation-console-staff)
  - [G9. bank — Nordvia Bank (fintech · brand nordvia · Material · strictest)](#g9-bank--nordvia-bank-fintech--brand-nordvia--material--strictest)
  - [G10. campus — Aster University (4-page application · brand aster · GraphQL · Ethiopic)](#g10-campus--aster-university-4-page-application--brand-aster--graphql--ethiopic)
  - [G11. cityrun — Riverton City Run (map-first event · brand riverton · WebSocket)](#g11-cityrun--riverton-city-run-map-first-event--brand-riverton--websocket)
  - [G12. devconf — DevHarbor Conf (e-commerce · brand devharbor, dark by default)](#g12-devconf--devharbor-conf-e-commerce--brand-devharbor-dark-by-default)
  - [G13. clinic — Willow Clinic (all Cupertino · brand willow · sensitive · SSE)](#g13-clinic--willow-clinic-all-cupertino--brand-willow--sensitive--sse)
  - [G14. relief — Hands Together (NGO · brand hands · Arabic first, RTL)](#g14-relief--hands-together-ngo--brand-hands--arabic-first-rtl)
  - [G15. homes — Brickyard Homes (rental application · brand brickyard · tablet first)](#g15-homes--brickyard-homes-rental-application--brand-brickyard--tablet-first)
  - [G16. arena — Pulse Arena (one page · brand pulse)](#g16-arena--pulse-arena-one-page--brand-pulse)
  - [G17. maintainers — Maintainers (QA frame · published to production · listed in Account when flags.showMaintainers)](#g17-maintainers--maintainers-qa-frame--published-to-production--listed-in-account-when-flagsshowmaintainers)
- [H. Localisation and accessibility](#h-localisation-and-accessibility)
- [I. Tests](#i-tests)
- [J. Publish, operate, ship](#j-publish-operate-ship)
- [Appendix K — Studio feature checklist (both canvases and their editors)](#appendix-k--studio-feature-checklist-both-canvases-and-their-editors)
- [Appendix L — Every widget, every property](#appendix-l--every-widget-every-property)
- [Appendix M — Every enum value](#appendix-m--every-enum-value)
- [Appendix N — Every action](#appendix-n--every-action)
- [Appendix O — Platform options](#appendix-o--platform-options)

---

## 0. How to read this sheet

Tick each `- [ ]` as you finish it. Nested items are children of the item above.

| Notation | Meaning |
|---|---|
| `Widget` | Insert this widget (Insert pane or Add-after toolbar). A nested item is its child; `slot:` names a slot (`appBar:`, `body:`, `item:`, `then:` …) |
| `prop: value` | Inspector › Properties. Several are joined by ` · ` |
| `@color.primary` | Bind to a design token (token picker) |
| `t:home.title` | A translation key (literal → translation button). Messages are in §H |
| `{{ expr }}` | A PXL binding (literal → expression button) |
| `asset:hero.webp` | An uploaded asset (§C4) |
| `⚡onPressed [drop]: a → b` | Inspector › Actions: the event, its concurrency (default when omitted), then its steps in order |
| `→ route(p: v)` | `navigate` push; `→⟲` replace, `→⤒` clearAndPush, `→⤓ until` popUntil |
| `⇢ route(p: v) ⇒ x` | `openDialog` / `openBottomSheet` and wait for its result `x` |
| `↩ value` | `pop` with that result |
| `id:` | The node's Test ID |
| `a11y:` | Inspector › Accessibility |
| `motion:` | Properties › Motion |
| `M:` / `E:` | Overrides at the Medium / Expanded size class |
| `🧪` | An edge case to check on the canvas *and* on a device |
| `◇Name` | An instance of a shared component (§D1) |

Route names are app-wide, so they carry the plugin's prefix (`nb-`, `cp-`, …). Every screen header reads
**route** · kind · title · params · result · state · data · lifecycle · transition · guards · security.

Appendices K–O are the coverage net: the Studio feature list, every widget with every property, every
enum value, every action and every platform option. When a value is not used by a real screen, it is
placed on a **Maintainers** screen (§G17), so nothing ships untested.

---

## 1. The product in one page

**Who uses it.** Members of the public (search, register, follow), organisation staff (console, check-in),
and you, the maintainer (Maintainers plugin, onboarding new organisations through the Partner plugin).

**Main journey.** Welcome → interests → personalised Home → organisation sheet → the organisation's own plugin
→ shared Verify sheets (OTP, ID, selfie, signature, address, step-up) returning typed results → shared Pay
plugin returning a typed result → submit (server function) → pass with live status → push notification /
deep link back into the pass.

**Plugins** (17) and their frames on the app canvas:

| Frame | Plugin key | Name | Screens | Built to show |
|---|---|---|---|---|
| Discover | `launch` | Launch | 8 | Entry plugin, onboarding, tabbed home, inbox, not-found, fallback |
| Discover | `explore` | Explore | 7 | Search, filters, map, categories, org profile, QR join |
| Platform | `passes` | Passes | 7 | Local database, Hero, QR, live status, receipts |
| Platform | `account` | Account | 9 | Profile, settings, consent, security, sign-in gate, delete account |
| Platform | `verify` | Verify | 6 | Reusable pages with typed params and results; exported flows |
| Platform | `pay` | Pay | 2 | Reusable wallet payment: input in, result out (80 % success / 20 % error) |
| Platform | `partner` | Partner | 4 | "Bring your organisation" request to the maintainer |
| Platform | `console` | Console | 4 | Staff dashboard, charts, tables, check-in (role-guarded) |
| Organisations | `bank` | Nordvia Bank | 8 | Fintech: secure pages, SCA signing, KYC, maximum assurance |
| Organisations | `campus` | Aster University | 8 | 4-page application, drafts, uploads, GraphQL, Ethiopic calendar |
| Organisations | `cityrun` | Riverton City Run | 6 | Map-first event, WebSocket live tracking, timelines |
| Organisations | `devconf` | DevHarbor Conf | 8 | E-commerce: tickets, merch, cart, dark brand |
| Organisations | `clinic` | Willow Clinic | 6 | Fully Cupertino (iOS style), sensitive data, SSE queue |
| Organisations | `relief` | Hands Together | 5 | Arabic-first (RTL), charts, Rive, donations |
| Organisations | `homes` | Brickyard Homes | 5 | Tablet-first responsive, drag and drop, floor plans |
| Organisations | `arena` | Pulse Arena | 2 | The one-page registration |
| QA | `maintainers` | Maintainers | 38 | Every remaining option value, every page transition, labs |

**Shell.** Tabs `home` (Launch), `explore` (Explore), `passes` (Passes), `profile` (Account).

**Out of reach today** (phases not built): AI generation (P12), co-editing and comments (P14), mini-app mode
(P16), card-payment providers (`startPayment`, P13 — replaced by the Pay plugin). Two items are built but
still `WIP` in the spec: `secure` page screenshot blocking (`SEC-091`) and confidential bundles (`SEC-053`) —
they are set here and must be checked on devices.

---

## A. Outside Studio

### A1. `ar-api` — the Go microservice

One Go service behind the Plux gateway. It serves the four data kinds a Plux function cannot serve (REST,
GraphQL, WebSocket, SSE), the sign-in the host app needs, and the push sends. Domain below:
`api.<your-domain>`; all JSON; times ISO 8601 with offset; money as `{"amount":"12.50","currency":"EUR"}`.

- [ ] **REST** (`/v1`)
  - [ ] `GET /orgs?q&category&near=lat,lng&radiusKm&sort=relevance|closing|distance|name&cursor&limit` → `{items:[Org], next, more}`
  - [ ] `GET /orgs/{slug}` → `Org` · `GET /categories` → `[Category]` · `GET /orgs/{slug}/status` → `{open, closesAt, seatsLeft}`
  - [ ] `POST /registrations` (auth, honours `Idempotency-Key`) → `{id, status, passCode}`; `409` on a duplicate form for the same user
  - [ ] `GET /me/registrations?status&cursor` → paged `Registration` · `GET /me/registrations/{id}` · `DELETE /me/registrations/{id}` (withdraw)
  - [ ] `POST /uploads` (multipart, auth, max 10 MiB, `application/pdf,image/*`) → `{fileId}`
  - [ ] `POST /transfers` (auth; SCA transaction: body `{amount, currency, to:{iban}, reference}`) → `{id, status}`
  - [ ] Staff (role `staff`): `GET /staff/orgs/{slug}/stats` → `{daily:[n], byForm:[{label,value}], revenue}` · `GET /staff/orgs/{slug}/submissions?status&sort&cursor` · `PATCH /registrations/{id}` `{status}` · `POST /checkins` `{passCode}` → `{ok, name}`
  - [ ] Test switches for edge cases: header `X-AR-Fail: 500|timeout|401|409|slow` (non-production only)
- [ ] **GraphQL** (`/graphql`): `programmes(q, cursor): ProgrammePage`, `programme(id)`, `agenda(day: Int!): [Session]`, `speakers: [Speaker]`, `workshops: [Workshop]`
- [ ] **WebSocket** `/ws/races/{raceId}/runners/{bib}` → `{lat, lng, km, etaMinutes, status}` every 3 s (simulated runner on the route)
- [ ] **SSE** `/sse/inbox` (auth) → `Notice` · `/sse/clinics/{id}/queue?ticket` → `{position, waitMinutes, status}` · `/sse/registrations/{id}` → `{status, note}`
- [ ] **Auth** (`/auth`): `POST /otp/start {phone|email}` · `POST /otp/verify {code}` → `{accessToken, refreshToken, expiresIn}` · `POST /refresh` · `POST /logout` · `DELETE /account` · `GET /.well-known/jwks.json`. Access token claims: `sub`, `role` (`member|staff|maintainer`), `org` (staff's org slug), `region`. Test number `+15555550100`, code `000000` for App Review
- [ ] **Push**: on a status change, send FCM/APNs data `{"plux":{"route":"pass","params":{"id":"<id>"}}}` and an SSE `Notice`
- [ ] **Partner inbox**: `POST /partner-requests` (API key header `X-AR-Key`) → stores and e-mails you
- [ ] Seed data: the 8 organisations of §G9–G16 (slugs `bank campus cityrun devconf clinic relief homes arena`), 30 more "coming soon" orgs for paging, 2 races, 3 clinics, 40 programmes

### A2. Plux server side for `ar-api`

- [ ] Environments `development`, `staging`, `production` exist (§B4); per environment:
- [ ] Register upstreams (`SecurityAdminService/CreateUpstream`, name = data source name): `api` (REST, `allowedMethods` GET POST PATCH DELETE, `apiKey` header `X-AR-Gateway` → secret `AR_GATEWAY_KEY`), `graph` (POST `/graphql`), `live` (WebSocket + SSE), `transfers` (POST `/transfers`, `requiresAssurance: AL2`, `transaction {amount:/amount, currency:/currency, payee:/to/iban}`)
- [ ] Set environment secret `AR_GATEWAY_KEY` (`AppService.SetSecret`); function secret `AR_PARTNER_KEY`
- [ ] Trust `ar-api` tokens for user claims (token exchange, `SEC-026`): issuer `https://api.<your-domain>/auth`, JWKS URL above
- [ ] Server configuration values (`DAT-090`): `feeTable` (live, JSON string), `supportPhone` (live), `ratesEURUSD` (live, **strict**), `payeeIbanRelief` (server-bound, displayed in SCA sheet)

### A3. The host app (the store binary)

- [ ] Publish a first release (any content) to `development`, then `plux create apply-register --env production --channel production --name "Apply Register" --application-id com.<you>.applyregister --icon icon.png --splash '#4F46E5' --out ./apply-register-app`
- [ ] Add the optional packages the plugins use: `plux_components plux_charts plux_maps plux_location plux_scanner plux_media plux_player plux_markdown plux_lottie plux_rive plux_functions plux_security plux_db_drift` (and `plux_maps_google` only if you choose Google tiles)
- [ ] Add your push SDK (FCM) and `add_2_calendar`; Android activity extends `FlutterFragmentActivity` (SCA); iOS `NSFaceIDUsageDescription`, `NSCameraUsageDescription`, `NSPhotoLibraryUsageDescription`, `NSLocationWhenInUseUsageDescription`
- [ ] `PluxConfig` (one registration point, `lib/main.dart`):

```dart
// SignInParams, SignInResult, CalendarIn, NoParams: your own small classes with fromJson.
final startup = await Plux.initialize(PluxConfig(
  appId: '<app id>', endpoint: Uri.parse('https://plux.<your-domain>'),
  environment: const String.fromEnvironment('PLUX_ENV', defaultValue: 'production'),
  startup: StartupPolicy.useCacheThenSync(),
  activation: ActivationPolicy.atSafePoint,
  themeSource: PluxThemeSource.plux,                       // tokens decide the look
  consent: PluxConsent.necessaryOnly,                      // until Account › Privacy says otherwise
  strongCustomerAuthentication: true,                      // Nordvia's SCA (§G9)
  hostBuild: '1.0.0+1', navigatorKey: navigatorKey,
  nativeRoutes: {
    'sign-in': PluxNativeRoute<SignInParams, SignInResult>(params: SignInParams.fromJson,
        builder: (c, p) => SignInScreen(reason: p.reason)),        // embeds PluxView('ac-why-sign-in')
    'licenses': PluxNativeRoute<NoParams, void>(params: NoParams.fromJson,
        builder: (c, _) => const LicensePage()),
  },
  nativeSlots: {
    'CalendarButton': PluxNativeSlot((c, s) => CalendarButton(
        title: s['title']! as String, onAdded: () => s.emit('onAdded', true))),
    'HostVersionTile': PluxNativeSlot((c, s) => HostVersionTile(
        onLongPress: () => s.emit('onLongPress', null))),
  },
  nativeActions: {
    'addToCalendar': PluxNativeAction<CalendarIn, bool>(input: CalendarIn.fromJson, handler: addToCalendar),
    'openAppSettings': PluxNativeAction<NoParams, bool>(input: NoParams.fromJson, handler: (_) => openAppSettings()),
    'requestReview': PluxNativeAction<NoParams, bool>(input: NoParams.fromJson, handler: (_) => requestReview()),
  },
));
Plux.setAuthDelegate(ArAuthDelegate(api));                  // isAuthenticated, accessToken, refresh, onLogout
Plux.eventsNamed('brandRequested').listen((e) => Plux.setBrand(e.payload['brand'] as String?));
Plux.eventsNamed('consentChanged').listen((e) => Plux.setConsent(PluxConsent(
    analytics: e.payload['analytics'] as bool, experiments: e.payload['experiments'] as bool,
    replay: e.payload['replay'] as bool)));
Plux.eventsNamed('registrationSubmitted').listen((_) => maybeAskForReview());
Plux.state<int>('unreadCount').watch().listen(setAppIconBadge);
// after sign-in: Plux.setUserContext(PluxUser(id: pseudonymousId, attributes: {'role': role, 'orgSlug': org, 'region': region, 'segment': segment}));
//                Plux.sendEvent('userSignedIn', {'userId': pseudonymousId});
// links / push:  Plux.handleDeepLink(uri); Plux.handlePushPayload(message.data);
// home:          PluxShell('main')   ·   wrap in PluxDevtools in debug builds
```

- [ ] `ArAuthDelegate.onLogout` → `Plux.wipeData()` (all plugins), clear tokens
- [ ] `plux.yaml` lists `CalendarButton`, `HostVersionTile`; run `plux native scan` then `plux native sync -C <plux project>` for every build you ship
- [ ] `plux codegen` → `lib/plux/plux.g.dart`; use `PluxScreens.pass(id: …)` from the push handler, `PluxFlags`, `PluxAppState`
- [ ] Deep links: Android intent filters for `https://<your-domain>` (autoVerify) and scheme `applyreg`; iOS `applinks:<your-domain>` + `CFBundleURLTypes` `applyreg`; publish `assetlinks.json` and `apple-app-site-association`
- [ ] Debug builds: `PluxDevtools` overlay; release builds: none (checked by `SEC-074`)

### A4. Development apps

- [ ] **Plux Dev app** (`apps/plux_dev`): install on one Android and one iPhone; pair from the plugin canvas (**Pair a device**) for live preview, inspect, state edits, network simulation, the console (§I3)
- [ ] **Starter** (`apps/starter`): reference only for comparing host wiring; not shipped
- [ ] A Plux project folder (`plux.json`) for the CLI: `functions/` (§E), `scenarios/` (§I1), goldens

---

## B. Organisation and app

### B1. Organisation, people, governance

- [ ] **New organisation**: name `Apply Register`, key `apply-register` (key is permanent)
- [ ] Teams: `platform` (Launch, Explore, Passes, Account, Verify, Pay, Partner, Console, Maintainers), `orgs` (the 8 org plugins), `qa`
- [ ] Members/roles: you = Owner; a second account = Release Manager (four-eyes approvals); one Translator (`translations.edit`), one Reviewer (`translations.approve`), one Auditor (read-only)
- [ ] Restriction: Designers may not publish to `production`
- [ ] Approval policies (§J2), change freeze window (e.g. last day of month 18:00–24:00), break-glass role = Owner
- [ ] Admin: API token for CI (`plux` CLI), session idle timeout 30 min, audit export test (CEF), retention defaults

### B2. Create the app

- [ ] Apps grid › **Create app** › Blank: name `Apply Register`, key `apply-register`, description "Find an organisation, register, follow your registration."
- [ ] Default and supported locales: `en` (default), `de-DE`, `ar`, `am-ET`; theme (§C); security profile (§B5); environments (§B4)
- [ ] Icon: upload `asset:app-icon.png` (1024²). 🧪 remove it once: the card falls back to the monogram, then restore
- [ ] The default plugin Studio creates → rename to `launch` (§F1); its entry screen becomes `home`
- [ ] Card actions to try once on a scratch copy: **Duplicate**, **Export .plux archive**, **Archive**/**Unarchive**, **Pin**; grid ↔ list, search, team/tag filters, sort by health
- [ ] App settings: name/description edit; **Limits usage** shows release size, plugins, pages, assets, functions (§J6 checks the limits)

### B3. App document (Inspector › Editors › JSON editor on the app, or the matching Studio pages)

- [ ] `entryRoute: home` · `minRuntimeVersion`: current runtime · `requiredFeatures` policy `raise` (`requiredFeaturesPolicy`)
- [ ] `types` — `Org{slug,name,category,city,lat:double,lng:double,logo:string,cover:string,closesAt:dateTime?,seatsLeft:int?,fee:money?,verified:bool,tags:list<string>}` · `Category{key,label,icon}` · `Registration{id,orgSlug,orgName,formKey,status,submittedAt:dateTime,eventAt:dateTime?,passCode,amount:money?,pinned:bool}` · `Notice{id,title,body,route,params:map<string,string>,at:dateTime,read:bool}` · `PayRequest{amount:money,payee,reference,purpose}` · `PayResult{status,transactionId:string?,errorCode:string?,message:string?}` · `VerifyResult{verified:bool,value:string?}` · `Address{text,lat:double,lng:double}` · `Runner{lat:double,lng:double,km:double,etaMinutes:int,status}` · `Queue{position:int,waitMinutes:int,status}` · `Programme`, `Session`, `Speaker`, `Workshop`, `Product`, `Listing`, `Member` (fields as in §G)
- [ ] `variables` per environment (§B4): `apiBaseUrl`, `graphUrl`, `streamUrl`, `mapTiles`, `privacyUrl`, `termsVersion`
- [ ] `state` (app scope):
  - [ ] `onboarded:bool=false` persisted · `interests:list<string>=[]` persisted · `themePref:ThemeMode=system` persisted
  - [ ] `unreadCount:int=0` memory **exposed** · `activeBrand:string=""` memory exposed
  - [ ] `pinHash:string=""` **secure** sensitive · `lastOrg:string=""` session · `recentSearches:list<string>=[]` persisted
  - [ ] `hasInterests:bool` **computed** `{{ !isEmpty(app.interests) }}` · `isStaff:bool` computed `{{ user.role == "staff" || user.role == "maintainer" }}`
  - [ ] 🧪 in the second release change `interests` to `list<Interest>` with a `migration` from `list<string>` (§J5)
- [ ] `collections` (app-shared): `registrations` v1 — fields `id` (pk), `orgSlug`, `orgName`, `status`, `submittedAt:dateTime`, `eventAt:dateTime?`, `passCode`, `pinned:bool`, `rank:int`, `note:string?` · indexes `[status]`, `[pinned,rank]` · `notices` v1 — `id` pk, `title`, `body`, `route`, `read:bool`, `at:dateTime` · 🧪 v2 later: rename `note`→`memo` with a migration step (§J5)
- [ ] `dataSources` (app): `categories` static `list<Category>` (8 entries, inline) · `me` rest `GET /me` (`auth: true`, cache `networkFirst` TTL 300 s, encrypted)
- [ ] `hostEvents`: `brandRequested{brand:string?}` toHost · `consentChanged{analytics:bool,experiments:bool,replay:bool}` toHost · `registrationSubmitted{orgSlug,registrationId}` toHost · `userSignedIn{userId}` toPlux · `refreshAll{}` both
- [ ] `userContext`: `role` (`member|staff|maintainer`), `orgSlug`, `region`, `segment` (`beta|general`) — all non-sensitive
- [ ] `flags` (§J4): `showMaintainers:bool=true`, `maxWorkshops:int=2`, `feeMultiplier:double=1.0`, `heroCopy:string="Find your place"`, `mapClustering:bool=false`
- [ ] `capabilities` (approved for plugins): `deviceApis` camera photos files location contacts biometrics notifications clipboard share haptics (all 10) · `networkDomains` `api.<your-domain>`, `tiles.openstreetmap.org` · `networkPins` for `api.<your-domain>` (2 SPKI pins, `SEC-042`) · `functions` all of §E
- [ ] `navigation`: `notFound: not-found` · `shells: [{key: main, tabs: [home(t:tab.home, icon home, initialRoute home), explore(t:tab.explore, travel_explore, explore), passes(t:tab.passes, confirmation_number, passes), account(t:tab.account, person, profile)]}]` · `deepLinks: {hosts: [<your-domain>], schemes: [applyreg], routes: [{path: /o/{slug}, route: ex-org}, {path: /pass/{id}, route: pass}, {path: /join/{slug}, route: ex-org}]}`
- [ ] `push: {enabled: true, payloadKey: plux}`
- [ ] `triggers` (app): `onAppResume` → `refreshData me` · `onAppPause` → `trackEvent app_pause` · `onPushOpened` → `trackEvent push_open {route: event.route}` · `hostEvents.userSignedIn` → `setState app.lastOrg ""` → `refreshData me` · `hostEvents.refreshAll` → `sync` · `timers: [{name: badge, intervalMs: 60000, repeat: true}]` → `dbQuery notices where !read` → `setState app.unreadCount {{ size(steps.q.output) }}` · `onError` (app error handler) → `trackEvent unhandled_error` → `showSnackbar t:error.generic`
- [ ] `sync: {startup: useCacheThenSync, activation: atSafePoint}` · 🧪 one staging build with `blockUntilSynced` and `nextLaunch`
- [ ] `telemetry: {sampling: …}` (keep defaults; raise action-trace sampling in staging)
- [ ] `localisation: {calendar: gregorian, secondaryCalendar: ethiopic, numberingSystem: latn, requireApprovedTranslations: true (production), pseudoLocales: true (development)}`
- [ ] `goldens: {blockPublish: true, matrix: [light, dark, ar RTL, 200 % text, compact + expanded]}` (§I2)

### B4. Environments, variables, secrets

- [ ] `development` (dev signing keys, dev attestation provider allowed), `staging`, `production` (`GOV-010`)
- [ ] Variables per env: `apiBaseUrl` `https://api.<env>.<your-domain>/v1` · `graphUrl` `…/graphql` · `streamUrl` `wss://…` · `mapTiles` · `privacyUrl` · `termsVersion` `2026-11`
- [ ] Secrets per env (never in documents): `AR_GATEWAY_KEY`, `AR_PARTNER_KEY`, `PASS_SIGNING_KEY`
- [ ] 🧪 put a fake API key in a text prop once: publish must fail (`SEC-107`), then remove it

### B5. Security profile and settings (Devices & security › Security profile)

- [ ] `development` **standard**, `staging` **maximum** (test pass: device functions off by default, stricter RASP), `production` **standard** with these overrides:
  - [ ] `screenshotBlockingDefault false` · `inactivityLock true` · `inactivityLockTimeout 300` · `allowDirectDataSources false` · `allowSessionReplay true` (staging only) · `deviceFunctions true`
  - [ ] RASP: `raspRootHookingResponse warn` · `raspDebuggerResponse report` · `raspEmulatorResponse report` (degrade in staging) · `raspRepackagingResponse block` · `raspInstallerResponse report` · `raspOverlayResponse warn` · `raspAccessibilityResponse report` · `raspScreenCaptureResponse warn` · `raspScanInterval 600`
  - [ ] `confidentialBundles optional` · `confidentialMinAssurance AL2` · `defaultFunctionAssurance AL1` · `minAssuranceForSync AL0` · `encryptLocalStores false` (true in staging)
  - [ ] Leave token, DPoP and attestation timings at defaults; read them in the effective-configuration view
- [ ] Capabilities page: **Edit grants** = §B3 `capabilities`; 🧪 a plugin requesting one more domain blocks publish
- [ ] Devices: after the first device run, open its record (assurance, key storage, attestation); 🧪 **Revoke device** on a test phone, it re-registers
- [ ] Signing keys and certificate pins: read-only check (rotation state shown)

---

## C. Design system (app › Design)

### C1. Tokens (Design tokens tab; each has Light / Dark / High contrast)

- [ ] Colour roles: `color.primary #4F46E5 / #A5B4FC / #3730A3` · `color.onPrimary` · `color.primaryContainer` · `color.secondary #0EA5E9` · `color.tertiary #F59E0B` · `color.error #DC2626` · `color.surface #FFFFFF / #0B0B12` · `color.surfaceContainer` · `color.outline` · `color.outlineVariant` (fill every role the theming reference lists, so contrast checks run on all)
- [ ] Brand colours (no role): `color.success #16A34A` · `color.warning #D97706` · `color.info #2563EB` · `color.scrim #000000 @ 40 %` · `color.gradientA #6366F1` · `color.gradientB #EC4899`
- [ ] Typography roles: `typography.displayLarge` … `typography.labelSmall` (all 15), family `Inter`; `typography.headlineSmall` weight w700
- [ ] Spacing: `spacing.xxs 2 · xs 4 · sm 8 · md 12 · lg 16 · xl 24 · xxl 32 · xxxl 48`
- [ ] Radius: `radius.sm 6 · md 10 · lg 16 · xl 24 · pill 999 · card 16`
- [ ] Elevation: `elevation.0 0 · 1 1 · 2 3 · 3 6 · 4 8 · 5 12`
- [ ] Motion: `motion.duration.short 150 · medium 300 · long 600` · `motion.easing.standard fastOutSlowIn · emphasized overshoot · exit easeIn`
- [ ] Breakpoints: `breakpoint.medium 600 · breakpoint.expanded 840`
- [ ] Font per script: `font.script.Latn [Inter, Roboto]` · `font.script.Arab [Noto Naskh Arabic]` · `font.script.Ethi [Noto Sans Ethiopic]`
- [ ] **Import tokens**: export these as DTCG JSON, delete two, import the file back (two added, rest skipped with reasons)

### C2. Themes and brand overlays

- [ ] Themes tab: light, dark, high contrast reviewed for every role; 🧪 a deliberately low-contrast `color.outline` in high contrast is flagged by the accessibility report, then fixed
- [ ] **Create a brand overlay** per organisation (host selects with `Plux.setBrand`, requested by each org's entry `onEnter`):

| Brand | primary | secondary | surface (dark) | Shape and type |
|---|---|---|---|---|
| `nordvia` | `#0B3D91` | `#00A3E0` | `#06142B` | `radius.card 4`, `typography.*` family `IBM Plex Sans` |
| `aster` | `#7C2D12` | `#CA8A04` | `#1C1009` | serif headlines `Source Serif 4` |
| `riverton` | `#EA580C` | `#16A34A` | `#111827` | `radius.card 24`, bold display |
| `devharbor` | `#22D3EE` | `#A3E635` | `#020617` | dark by default, mono labels `JetBrains Mono` |
| `willow` | `#0F766E` | `#5EEAD4` | `#042F2E` | iOS look (Cupertino widgets) |
| `hands` | `#BE123C` | `#F59E0B` | `#1F0A10` | Arabic first, `Noto Naskh Arabic` |
| `brickyard` | `#78350F` | `#65A30D` | `#1A120B` | `radius.card 12` |
| `pulse` | `#DB2777` | `#7C3AED` | `#18061A` | pill buttons |

- [ ] 🧪 switch **Brand** under Preview on the plugin canvas for each; pages recolour; leaving an org resets (`brandRequested {brand: null}`)

### C3. Typography, host fonts, icons

- [ ] Typography tab: Latin, Arabic, Ethiopic families with fallbacks (above); 🧪 Amharic text shows no tofu on canvas and device
- [ ] **Host app fonts**: upload `Inter-Variable.ttf`, `NotoNaskhArabic.ttf`, `NotoSansEthiopic.ttf`, `IBMPlexSans.ttf`, `SourceSerif4.otf`, `JetBrainsMono.ttf`; **Add font** each with its family name
- [ ] Icons: Material Symbols and Cupertino sets on; **Add icon** a custom set `ar` (SVG assets `ar-logo`, `ar-pass`, `ar-verified`) — used by Launch and Passes

### C4. Assets (plugin canvas › Assets; upload once per plugin that uses them)

- [ ] Images: `app-icon.png`, `hero-1..3.webp`, `org-<slug>-logo.png` ×8, `org-<slug>-cover.webp` ×8, `floorplan-a.png`, `merch-*.webp` ×6, `listing-*.webp` ×9, `empty-passes.svg`, `offline.svg`, `pattern.png` (for `ImageRepeat`), `avatar-placeholder.png`
- [ ] Motion: `welcome.lottie`, `success.lottie`, `confetti.lottie`, `goal-meter.riv` (state machine `Meter`, input `progress`, trigger `celebrate`), `pitch-hero.riv`
- [ ] Media (by URL from `ar-api`, not assets): `pitch.mp4` (+ `pitch.vtt` captions), `clinic-intro.mp4`, `prep-instructions.mp3`
- [ ] 🧪 a file over `asset.fileSize` is refused before upload; a plugin over 80 % of `plugin.assetBytes` turns the meter amber; **Replace…** an image keeps its uses; deleting a used file names how many widgets lose it; a `.gif` and an `.svg` are accepted; an `image/webp` variant list shows

---

## D. Library — build once, reuse everywhere

**Rule for every screen:** use a library component first (`◇Name`); insert a raw widget only where the screen
tests that widget's own options (those screens say so, and Maintainers covers the rest). **Never pick a colour,
radius, elevation or font by hand**: components and screens bind tokens (§C), so a theme or brand change
restyles everything. A raw value appears only on Maintainers galleries, where showing the value is the point.
Reuse does not shrink the test surface: every widget and option inside a component is still listed in
Appendix L/M, and each component's own props, slots, events, versions and overrides are tested in
`mt-components`.

### D1. Components (select the nodes › **Extract to component**; edit in isolation; visibility **Organisation**; v1)

- [ ] **`BaseCard`** — the only card style. props `variant:string="raised"` (`flat` | `raised` | `outlined`), `tappable:bool=false`, `selected:bool=false`, `dense:bool=false`; slots `media`, `content`, `actions`; event `tap`; state `component.pressed:bool`
  - [ ] `Card` clipBehavior antiAlias · margin 0 · color `@color.surfaceContainerLow` · elevation `{{ props.variant == "raised" ? 2.0 : 0.0 }}` (values from `@elevation.0`/`@elevation.2`) · shape roundedRectangle radius `@radius.card` · side width `{{ props.selected ? 2.0 : props.variant == "outlined" ? 1.0 : 0.0 }}` colour `@color.outlineVariant` (selected: a second `If` layer draws the `@color.primary` outline)
    - [ ] `InkWell` borderRadius `@radius.card` · ⚡onTap: `condition {{ props.tappable }}` → `emitEvent tap` · ⚡onHighlightChanged `setState component.pressed {{ event }}` › `AnimatedScale` scale `{{ component.pressed && props.tappable ? 0.98 : 1.0 }}` duration 120 › `Column` crossAxisAlignment stretch › `Slot media` (fallback none); `Padding` `{{ props.dense ? 12.0 : 16.0 }}` (bind to `@spacing.md`/`@spacing.lg`) › `Slot content`; `Slot actions` (fallback none) in `Padding` h8 b8 › `Row` mainAxisAlignment end
  - [ ] Semantics: `group: true`; `button: {{ props.tappable }}`
- [ ] **`OrgCard`** (exported for `PluxView`) — built from `◇BaseCard` variant raised tappable. props `org:Org`, `compact:bool=false`, `showFee:bool=true`; slot `badge`; event `open{slug:string}`
  - [ ] media: `Stack` › `AspectRatio` 16/9 › `Image {{ props.org.cover }}` fit cover · loading `SkeletonLoader` · error `Icon broken_image`; `Positioned` top 8 end 8 › `Slot badge` (fallback `◇StatusChip` status `new`) · `visible {{ !props.compact }}`
  - [ ] content: `ListTile` contentPadding 0 · leading `CircleAvatar` backgroundImage `{{ props.org.logo }}` · title `{{ props.org.name }}` · subtitle `{{ props.org.city }}` · trailing `If {{ props.showFee && props.org.fee != null }}` › `MoneyText {{ props.org.fee }}`
  - [ ] ⚡ BaseCard `tap` → `emitEvent open {slug: {{ props.org.slug }}}`
- [ ] **`HeroHeader`** — props `image:ImageSource`, `title:string`, `subtitle:string?`, `heroTag:string?`; slot `badge`; for `SliverAppBar.flexibleSpace` and page tops
  - [ ] `Stack` fit expand › `Image {{ props.image }}` fit cover; `DecoratedBox` gradient linear `@color.scrim` → transparent (bottomCenter → topCenter); `Positioned` start 16 end 16 bottom 20 › `Column` [`Text {{ props.title }}` `@typography.headlineMedium` colour `@color.onPrimary` maxLines 2 overflow ellipsis, `If subtitle` › `Text` `@typography.bodyMedium`, `Slot badge`]
- [ ] **`SectionHeader`** — props `title:string`, `subtitle:string?`, `actionLabel:string?`; event `action`; `Padding` h16 t24 b8 › `Row` [`Expanded` › `Column` [`Text` `@typography.titleMedium` · a11y header level 2, `If subtitle` `Text` `@typography.bodySmall`], `If actionLabel` › `TextButton` ⚡ `emitEvent action`]
- [ ] **`StepHeader`** — props `step:int`, `of:int`, `title:string`; `Column` [`LinearProgressIndicator` value `{{ double(props.step) / double(props.of) }}` borderRadius `@radius.pill` minHeight 6, `Text {{ t("common.stepOf", {n: props.step, of: props.of}) }}` `@typography.labelMedium`, `Text {{ props.title }}` `@typography.titleLarge` · a11y header]
- [ ] **`FieldGroup`** — props `title:string`, `hint:string?`; slots `fields`, `trailing`; `◇BaseCard` variant outlined › content `Column` spacing `@spacing.sm` [`Text` title `@typography.titleSmall`, `If hint` `Text`, `Slot fields`, `Slot trailing`]. Form fields stay in the page and are placed into the slot (form state lives in the page)
- [ ] **`SubmitBar`** — props `label:string`, `busy:bool=false`, `enabled:bool=true`, `secondaryLabel:string?`; events `submit`, `secondary`; `SafeArea` top false › `Padding` `@spacing.lg` › `Row` [`If secondaryLabel` › `TextButton` ⚡ `emitEvent secondary`, `Expanded` › `FilledButton` minimumSize (∞, 52) · enabled `{{ props.enabled && !props.busy }}` ⚡onPressed [drop] `emitEvent submit` › `AnimatedSwitcher` (Text ↔ `CircularProgressIndicator` strokeWidth 2)]
- [ ] **`InfoRow`** — props `label:string`, `value:string`, `icon:IconData?`, `copyable:bool=false`, `ltr:bool=false`; event `copied`; `ListTile` dense · contentPadding 0 · leading `If icon` › `Icon` · title `Text label` `@typography.bodyMedium` · trailing `Row` mainAxisSize min [`SelectableText {{ props.value }}` (node `direction: ltr` when `props.ltr`), `If copyable` › `IconButton` content_copy ⚡ `copyToClipboard {{ props.value }}` → `emitEvent copied`]
- [ ] **`MoneyRow`** — props `label:string`, `amount:money`, `emphasis:bool=false`, `signed:bool=false`; `Row` [`Expanded` › `Text label`, `MoneyText` value `{{ props.amount }}` · signDisplay `{{ props.signed ? "always" : "auto" }}` (🧪 enum binding; else two `If` branches with literals) · style `@typography.titleLarge` when emphasis else `@typography.bodyLarge`]
- [ ] **`StatusChip`** — props `status:string`, `label:string?`; `Match` value `{{ props.status }}` cases `[new, draft, submitted, approved, rejected, done, error, live]` → each a `Chip` with literal tokens (`@color.secondaryContainer`, `@color.surfaceContainerHigh`, `@color.primaryContainer`, `@color.success`, `@color.errorContainer`, `@color.outlineVariant`, `@color.error`, `@color.tertiary`) and icon; label `{{ props.label ?? t("status." + props.status) }}`; otherwise `Chip` neutral
- [ ] **`EmptyHint`** — props `icon:IconData`, `title:string`, `message:string?`, `cta:string?`; event `cta`; `EmptyState` › action `If cta` › `FilledButton.tonal` ⚡ `emitEvent cta`
- [ ] **`ErrorBlock`** — props `title:string`, `message:string?`, `retryLabel:string?`, `icon:IconData=cloud_off`; event `retry`; `ErrorState` icon `{{ props.icon }}` · retryLabel `{{ props.retryLabel ?? t("common.retry") }}` · ⚡onRetry `emitEvent retry`
- [ ] **`ArWhySignIn`** (exported; the host's native sign-in screen shows it with `PluxView('ac-why-sign-in')`) — `Carousel` of 3 `◇BaseCard` variant flat with icon + text
- [ ] Versioning and reuse checks: publish `OrgCard` v2 (adds a `closesAt` chip in `badge`): Explore adopts v2, Launch stays on v1 (amber diamond); **Detach** one instance in `mt-components`; **Isolate**; **Open component** (versions, instances across apps); editing `BaseCard` lists every instance that will change before saving
- [ ] Library page: search, filters (visibility, owner, category, tag), previews, usage counts; **Export** `BaseCard` and `OrgCard` to files and **Import** them into a scratch app

### D2. Templates (select › **Save as template**, two steps; inserted as independent copies)

- [ ] `contact-block` — Private · Forms · params `requirePhone:bool`, `title:string` · `◇FieldGroup` with name, email (`TextFormField` keyboardType emailAddress) and `PhoneInput`
- [ ] `consent-block` — Organisation · params `documentVersion`, `linkLabel` · `ConsentCheckbox` + `TextButton` (opens terms)
- [ ] `upload-block` — Organisation · `◇FieldGroup` › `FileUploader` + hint `Text` · records its dependency on data source `api`
- [ ] `summary-card` — Organisation · params `title:string` · `◇BaseCard` variant outlined › `◇SectionHeader` + 3 `◇InfoRow` + `◇MoneyRow` emphasis
- [ ] `list-states` — Organisation · a `ListView` with its **empty** (`◇EmptyHint`), **loading** (`SkeletonLoader` lines 3 avatar) and **error** (`◇ErrorBlock` ⚡retry `refreshData`) views ready; param `source:string`
- [ ] `thank-you` — **Public** (passes the public scan: no secrets, private URLs or personal data) · `LottieView asset:success.lottie` + headline + `◇SubmitBar`
- [ ] Insert each once: parameters prompted, fresh IDs, independent afterwards; 🧪 a template that read `page.amount` lists it as a parameter; a page-state path inside a larger expression is flagged

---

## E. Functions (Rust → WebAssembly)

Each lives in `functions/<name>/` of the Plux project (`plux fn new <name> --placement server|device`).
Replace `src/lib.rs` with the code below and `capabilities` in `function.json` as listed. Common imports:
`use plux_fn::*; use rust_decimal::{Decimal, RoundingStrategy}; use schemars::JsonSchema; use serde::{Deserialize, Serialize};`
Dependencies: `rust_decimal`, `serde_json`, `sha2` (Cargo.toml).

- [ ] **`pay.simulate`** — device · capabilities `{}` · used by Pay (§G6)

```rust
#[derive(Deserialize, JsonSchema)] pub struct Input { amount: Decimal, currency: String, reference: String }
#[derive(Serialize, JsonSchema)] #[serde(rename_all = "camelCase")]
pub struct Output { status: String, transaction_id: Option<String>, error_code: Option<String> }

fn handler(ctx: &DeviceContext, i: Input) -> Result<Output, BusinessError> {
    if i.amount <= Decimal::ZERO { return Err(BusinessError::new("AMOUNT_INVALID", "pay.amountInvalid")); }
    let ms = ctx.now().unwrap_or_else(|e| e.trap()) / 1_000_000;
    if ms % 100 < 20 {                                    // one call in five fails
        let code = ["INSUFFICIENT_FUNDS", "DECLINED", "LIMIT_EXCEEDED", "WALLET_UNAVAILABLE"][(ms % 4) as usize];
        return Ok(Output { status: "error".into(), transaction_id: None, error_code: Some(code.into()) });
    }
    Ok(Output { status: "success".into(), transaction_id: Some(format!("W{ms}-{}", i.reference)), error_code: None })
}
plux_fn::function!(handler);
```

- [ ] **`bank.eligibility`** — device · `{}` · Nordvia (§G9)

```rust
#[derive(Deserialize, JsonSchema)] pub struct Input { monthly_income: Decimal, age: u32, employment: String }
#[derive(Serialize, JsonSchema)] pub struct Output { eligible: bool, score: u32, reasons: Vec<String> }
fn handler(_: &DeviceContext, i: Input) -> Result<Output, BusinessError> {
    if i.age < 18 { return Err(BusinessError::new("UNDERAGE", "bank.underage").with_param("min", 18)); }
    use rust_decimal::prelude::ToPrimitive;
    let mut score = 40 + (i.monthly_income / Decimal::from(100)).min(Decimal::from(40)).to_u32().unwrap_or(0);
    let mut reasons = vec![];
    if i.employment == "student" { score -= 10; reasons.push("bank.reason.student".into()); }
    Ok(Output { eligible: score >= 50, score, reasons })
}
plux_fn::function!(handler);
```

- [ ] **`run.finishTime`** — device · `{}` · City Run (§G11): `Input {distance_km: Decimal, pace_sec_per_km: u32}` → `Output {minutes: u32, label: String}`; `label = format!("{:02}:{:02}:{:02}", s/3600, s/60%60, s%60)`; business error `PACE_UNREALISTIC` when pace < 150
- [ ] **`essay.stats`** — device · `{"kv": true}` · Campus (§G10): `Input {text: String}` → `Output {words, sentences, reading_minutes, long_words}`; also `ctx.kv().put("lastWords", …)`; error `TOO_LONG` above 5,000 words
- [ ] **`fees.quote`** — server · `{"kv": false}` · `env: ["feeTable"]` · `pure: true, cacheTtlSeconds: 300` · Campus, City Run, DevHarbor, Arena (as a `function` data source)

```rust
#[derive(Deserialize, JsonSchema)] pub struct Input { org: String, tier: String, quantity: u32, currency: String }
#[derive(Serialize, JsonSchema)] pub struct Output { subtotal: Decimal, tax: Decimal, total: Decimal, currency: String }
fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    let raw = ctx.env("feeTable").unwrap_or_else(|e| e.trap()).unwrap_or_else(|| "{}".into());
    let table: serde_json::Value = serde_json::from_str(&raw).unwrap_or_default();
    let base: Decimal = table[&i.org][&i.tier].as_str().and_then(|s| s.parse().ok())
        .ok_or_else(|| BusinessError::new("UNKNOWN_TIER", "fees.unknownTier").with_param("tier", &i.tier))?;
    let subtotal = base * Decimal::from(i.quantity);
    let tax = (subtotal * Decimal::new(19, 2)).round_dp_with_strategy(2, RoundingStrategy::MidpointNearestEven);
    Ok(Output { subtotal, tax, total: subtotal + tax, currency: i.currency })
}
plux_fn::function!(handler);
```

- [ ] **`partner.submit`** — server · `{"http": ["api.<your-domain>"], "secrets": ["AR_PARTNER_KEY"], "kv": true}` · `env: ["arBase"]` · `limits: {"timeoutMs": 4000, "httpCalls": 1}` · Partner (§G7)

```rust
#[derive(Deserialize, Serialize, JsonSchema)] pub struct Input { org_name: String, email: String, category: String, pages: u32, needs: Vec<String>, plan: String, notes: Option<String> }
#[derive(Serialize, JsonSchema)] pub struct Output { reference: String }
fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    let key = format!("req:{}", i.email.to_lowercase());
    if ctx.kv().get(&key).unwrap_or_else(|e| e.trap()).is_some() {
        return Err(BusinessError::new("DUPLICATE_REQUEST", "partner.duplicate"));
    }
    let base = ctx.env("arBase").unwrap_or_else(|e| e.trap()).unwrap_or_default();
    let req = HttpRequest::post(&format!("{base}/partner-requests"), serde_json::to_vec(&i).unwrap_or_default())
        .header("Content-Type", "application/json")
        .header("X-AR-Key", &ctx.secret("AR_PARTNER_KEY").unwrap_or_else(|e| e.trap()));
    let res = ctx.http().send(&req).map_err(|_| BusinessError::new("UPSTREAM_DOWN", "partner.upstreamDown"))?;
    if res.status != 201 { return Err(BusinessError::new("UPSTREAM_REFUSED", "partner.refused").with_param("status", res.status)); }
    ctx.kv().put(&key, b"1").unwrap_or_else(|e| e.trap());
    Ok(Output { reference: String::from_utf8_lossy(&res.body).into_owned() })
}
plux_fn::function!(handler);
```

- [ ] **`slots.availability`** — server · `{"kv": true}` · Clinic: `Input {clinic, date}` → `Output {slots: Vec<Slot{time, free}>}` (09:00–16:30 every 30 min minus `kv` key `booked:<clinic>:<date>`); error `CLOSED` on Sundays
- [ ] **`slots.book`** — server · `{"kv": true, "claims": true}` · `assurance: "AL1"`: appends to the booked list; error `TAKEN` (🧪 book the same slot from two phones)
- [ ] **`workshops.capacity`** — server · `{"kv": true}` · DevHarbor: `Input {ids: Vec<String>}` → `Output {left: Vec<u32>}`; error `FULL{ id }` when any is 0
- [ ] **`promo.check`** — server · `{}` · DevHarbor async validator: codes `EARLY20` (−20 %, until `closesAt`), `TEAM5` (needs quantity ≥ 5); errors `PROMO_INVALID`, `PROMO_EXPIRED`, `PROMO_CONDITION`
- [ ] **`pass.issue`** — server · `{"secrets": ["PASS_SIGNING_KEY"], "claims": true}` · `assurance: "AL1"`: `code = hex(sha256(secret ‖ sub ‖ registrationId))[..12]`; error `NOT_OWNER` when the claims' `sub` differs
- [ ] **`reminders.daily`** — server · `{"http": ["api.<your-domain>"], "secrets": ["AR_PARTNER_KEY"]}` · run by a **schedule** (`FunctionTriggerService/CreateTrigger` expression `@daily`, timezone `Europe/Berlin`, alias `stable`): GET due reminders, POST pushes
- [ ] **`hooks.orgSync`** — server · `{"kv": true}` · inbound webhook `/hooks/<app>/hooks.orgSync` (Standard Webhooks signature): stores `{slug, open, closesAt}`; 🧪 a replayed message is refused (`PLX-7152`), a bad signature too (`PLX-7151`)

Per function:

- [ ] 2 stored test cases in `function.json` (one success, one business error); `plux fn test <name>` (native + module + device interpreter for device functions)
- [ ] `plux fn build <name> --verify` (byte-identical) · `plux fn invoke <name> --input '{…}'`
- [ ] `plux fn deploy <name> --env development --alias stable` → staging → production (production needs step-up; 🧪 a failing stored case blocks production, `PLX-7020`)
- [ ] Studio › Functions: versions (assurance, signed by builder, hashes, toolchain, schemas), **Capabilities** granted, **Tests** › Run, **Metrics** (p50/p95/p99, memory, cache hits for `fees.quote`), **Logs**, **Point alias at this version**, **Edit in the Rust editor** › **Start the language server**
- [ ] 🧪 under staging's `maximum` profile a device function fails until enabled per function (`FN-055`); a device app without `plux_functions` fails with `PLX-5401`; a `timeoutMs` exceeded shows a generic message and logs `PLX-7001`

---

## F. App canvas

### F1. Plugins and frames

- [ ] **New plugin** ×16 (the default plugin becomes `launch`): key, name, description, tags (`discover`, `platform`, `org`, `qa`), owning team (§B1), icon (`asset:org-<slug>-logo.png` for orgs, a monogram for the rest)
- [ ] **Plugin settings** on each: description, tags, team; 🧪 rename `explore` → `Explore & map` → back: links stay (ids)
- [ ] **Make entry plugin** on `launch`; 🧪 **Delete plugin** on `launch` is refused until another entry is chosen
- [ ] Place boxes: Discover frame (top-left), Platform frame (centre), Organisations frame (right, 2 rows of 4), QA frame (bottom); **Frame selection** for each group; rename and resize frames in the **Frames** panel
- [ ] Drag with snapping and guides; marquee-select the Organisations; **Arrange** (layered layout), then **Undo**; **Move to…** with exact X/Y for `maintainers`
- [ ] **Zoom out/in**, percent (resets to 100 %), **Zoom to fit** (Shift+1), Ctrl+0, Space/H/middle-button pan, pinch zoom, V back to Select, Escape clears
- [ ] Dock: **Plugins** search, filters **Unpublished**, **With errors**, **Archived**; row click selects and scrolls, double-click opens; **Frames**; **Overview** (counts, entry plugin); fold the dock and reopen it
- [ ] Detail card (hover/focus): description, mini-map, screens, versions, bundle size, links in/out, functions, last publisher; the box shows name, icon, screens, versions, links, owner, version/Draft, status (**Valid**, **N errors**, **Unpublished changes**, **+N**)
- [ ] Light/dark/contrast canvas theme switch

### F2. Links and arrows (they appear as you build §G; check them here)

- [ ] Bundled arrows with count badges: `explore → bank…arena` (8 links into 8 boxes), every org → `verify`, every paying org → `pay`, every org → `passes`, `passes → explore`, `launch → partner`, `account → native:sign-in`, `account → native:licenses`
- [ ] Opposite-direction bundles drawn parallel (`passes ⇄ explore`); sides chosen for shortest path with no flicker while dragging near equidistance; anchors spread on shared sides
- [ ] Hover/focus a bundle: it expands into labelled lanes ("Register → nb-welcome"), others dim; click a lane: **open the source widget**, **open the target**, **change the trigger**, **redirect the target**, **remove the link**
- [ ] Drag from a box's connection handle to another box: pick trigger widget + event; required params are filled (`maintainers → arena`)
- [ ] Native nodes `native:sign-in`, `native:licenses` (dashed, Native chip) only receive arrows
- [ ] Zoom out below the threshold: labels hide, lanes merge; 60 fps while dragging a box with ~40 links

### F3. Safety and lifecycle

- [ ] 🧪 **Delete plugin** on `verify`: blocked, lists incoming links; **Redirect incoming links** (to a scratch plugin) and **Remove outgoing links** offered — cancel
- [ ] **Duplicate plugin** `arena` → `arena-copy`: new ids, internal links remapped; then **Archive plugin**, **Restore plugin**; **Delete plugin** → **Trash** → restore (links come back) → delete again
- [ ] **Move to…** a screen between plugins (from the plugin canvas, §G17 `mt-move-me`) and see the arrow follow
- [ ] 🧪 open a plugin locked by your second account: read-only, holder shown, **Request edit**; **Take over** after they stop
- [ ] 🧪 in production (after the first prod release) delete/deprecate asks to type the key

---

## G. Plugins and screens

### G0. For every plugin and every screen (do once per item; not repeated below)

- [ ] Plugin document: description, tags, team, icon, `entryPage`, `fallbackPage` (`ln-offline` pattern: an `ErrorState` + retry `sync`), `capabilities` (subset of §B3), `types`, plugin `state`, `collections`, `dataSources`, `triggers`, flows
- [ ] **New screen** › kind (Page / Dialog / Bottom sheet / Full-screen dialog); rename in the header bar (click the title; Enter keeps, Escape reverts); 🧪 a rename that changes a deep-linked route shows the affected generated APIs first
- [ ] Page settings: title (`t:` key), description, route, key, entry, result type (dialogs/sheets), params with defaults and **mocks**, page state, forms, data sources (each with mocks: success, empty, error), lifecycle, transition, timeline, guards, `secure`, `requiresAssurance`
- [ ] Library first (§D): cards are `◇BaseCard`, section titles `◇SectionHeader`, label/value rows `◇InfoRow`/`◇MoneyRow`, statuses `◇StatusChip`, list states `◇EmptyHint`/`◇ErrorBlock` (or template `list-states`), page tops `◇HeroHeader`, primary actions `◇SubmitBar`; colours, radii, elevation and fonts only through tokens
- [ ] Every interactive node: `id:` (Test ID) and a semantics label (or visible text); images: `semanticLabel` or **Decorative**
- [ ] Problems tab clean (errors 0, warnings explained), cost under budget, Localisation view clean in all 4 locales, Accessibility view clean (focus order, targets ≥ 48/44, contrast)
- [ ] Preview each screen in: Light, Dark, a Brand, `ar` (RTL), `am-ET`, a pseudo-locale, text scale 2.0, Material and Cupertino, each data state (Success, Loading, Empty, Error) and per-source states; Device matrix (all frames, both orientations); a foldable for the screens marked `M:`/`E:`
- [ ] **Send to device** (Dev app) once per screen; **Highlight selection on device**; **Capture screenshot** › **Compare with the canvas** (side by side, overlay, difference)

### G1. `launch` — Launch (entry plugin, team platform)

Plugin: capabilities `deviceApis [notifications, haptics, location]`, `networkDomains [api]` · state `heroIndex:int=0` (memory) · dataSources `featured` rest `GET /orgs?sort=closing&limit=6` (`list<Org>`, cache **staleWhileRevalidate** TTL 600, auth false) · `nearby` rest `GET /orgs?near={lat},{lng}&radiusKm=10` (params lat, lng) · `inbox` **sse** `/sse/inbox` (`Notice`, auth) · `notices` **database** on app collection `notices` orderBy `at` descending · triggers `watch app.interests` → `refreshData featured` · `dataSources.inbox.onMessage` → `dbUpsert notices {{ event }}` → `setState app.unreadCount {{ app.unreadCount + 1 }}` → `haptic light` → `announce t:inbox.new polite` · fallbackPage `ln-offline`

- [ ] **`home`** · Page · `t:home.title` · page state `scrollY:double=0` · lifecycle onEnter: `emitHostEvent brandRequested {brand: null}` → `condition {{ !app.onboarded }}` → `⇢ ln-welcome ⇒ list<string>` → `setState app.interests {{ steps.w.output ?? [] }}` → `setState app.onboarded true`; else `kvGet whatsNewSeen` → `condition {{ steps.k.output != env.termsVersion }}` → `⇢ ln-whats-new`; onResume: `refreshData featured` · transition `fade` · timeline `heroFade` (driver **scroll**, extent 200: header opacity 1→0.2, image scale 1→1.15)
  - [ ] `Scaffold` extendBodyBehindAppBar: true · backgroundColor @color.surface
    - [ ] body: `RefreshIndicator` displacement 56 · ⚡onRefresh [restart]: `parallel [refreshData featured, refreshData nearby]`
      - [ ] `CustomScrollView` physics: bouncing · keyboardDismissBehavior: onDrag
        - [ ] `SliverAppBar` expandedHeight 260 · pinned · stretch · collapsedHeight 64 · ⚡onStretchTrigger: `refreshData featured`
          - [ ] flexibleSpace: `Stack` fit expand › `FadeTransition` timeline heroFade track 0 › `ScaleTransition` timeline heroFade track 1 › `◇HeroHeader` image `asset:hero-1.webp` · title `{{ flags.heroCopy }}` — **experiment `home-hero`** on this subtree (§J4)
          - [ ] actions: `IconButton` icon notifications · tooltip `t:home.inbox` › wrapped in `Badge` label `{{ app.unreadCount }}` · isLabelVisible `{{ app.unreadCount > 0 }}` · ⚡onPressed: `→ inbox`; `IconButton` translate · ⚡: `⇢ ln-language ⇒ string`
        - [ ] `SliverPadding` padding h16 v8 › `SliverToBoxAdapter` › `SearchBar` hintText `t:home.search` · leading Icon search · ⚡onTap: `switchTab explore`
        - [ ] `SliverToBoxAdapter` › `Carousel` autoplay true · intervalMs 6000 · indicators true · semanticLabel `t:home.featured` · ⚡onPageChanged: `setState plugin.heroIndex {{ event }}` · children: `ForEach` items `{{ data.featured.value ?? [] }}` › `◇OrgCard` org `{{ item }}` · ⚡open: `⇢ ex-peek(slug: {{ event.slug }}) ⇒ r` → `switch {{ steps.r.output.action }}` cases `[register, details]` → both: `→ ex-org(slug: {{ event.slug }})`
        - [ ] `SliverToBoxAdapter` › `If {{ !isEmpty(data.featured.value ?? []) }}` then `PromoBanner` title `t:home.closingSoon` · subtitle `{{ first(data.featured.value).name }}` · image `{{ first(data.featured.value).cover }}` · ctaLabel `t:common.open` · endsAt `{{ first(data.featured.value).closesAt }}` · ⚡onTap: `→ ex-org(slug: …)`; else `◇EmptyHint`
        - [ ] `SliverPersistentHeader` pinned · minExtent 48 · maxExtent 48 › `ColoredBox` @color.surface › `◇SectionHeader` title `t:home.categories` · actionLabel `t:common.all` · ⚡action `switchTab explore`
        - [ ] `SliverGrid` items `{{ data.categories.value }}` · crossAxisCount 4 · `M:` 6 · `E:` 8 · mainAxisSpacing 8 · crossAxisSpacing 8 · childAspectRatio 0.9 · item: `InkWell` borderRadius @radius.md · ⚡onTap: `→ ex-category(key: {{ item.key }})` › `Column` mainAxisSize min › `Icon {{ item.icon }}` size 28 color @color.primary; `Text {{ item.label }}` @typography.labelMedium textAlign center maxLines 1 overflow ellipsis
        - [ ] `SliverToBoxAdapter` › `Padding 16` › `◇BaseCard` variant raised › media: `SizedBox` height 180 › `MapView` semanticLabel `t:home.nearbyMap` · center `{{ {"lat": 52.52, "lng": 13.40} }}` · zoom 12 · interactive false · markers `{{ map(data.nearby.value ?? [], o, {"id": o.slug, "position": {"lat": o.lat, "lng": o.lng}, "title": o.name}) }}` · ⚡onMapTap: `switchTab explore`
        - [ ] `SliverToBoxAdapter` › `◇BaseCard` variant outlined · tappable · content `ListTile` leading Icon handshake · title `t:home.partner` · subtitle `t:home.partnerSub` · trailing Icon chevron_right · ⚡tap: `→ pt-pitch`
        - [ ] `SliverFillRemaining` hasScrollBody false › `OfflineBanner` message `t:common.offline` · visible `{{ data.featured.error != null }}`
  - [ ] 🧪 first run opens the welcome dialog once; airplane mode → offline banner, cached featured still shown (SWR); empty featured → EmptyHint; inbox badge counts up from SSE; scroll timeline in Interact mode
- [ ] **`ln-welcome`** · Full-screen dialog · result `list<string>` · transition `slideUp`
  - [ ] `Scaffold` › body `SafeArea` › `OnboardingPager` nextLabel `t:common.next` · doneLabel `t:welcome.start` · ⚡onPageChanged: `trackEvent onboarding_page {page: {{ event }}}` · ⚡onDone: `→ ln-interests`
    - [ ] page 1: `FractionallySizedBox` widthFactor 0.8 › `LottieView asset:welcome.lottie` repeat true · reduceMotion skip · height 240; `Text t:welcome.p1` headlineSmall
    - [ ] page 2: `Icon search` size 96 + `Text t:welcome.p2`; page 3: `Icon verified_user` + `Text t:welcome.p3`
  - [ ] persistentFooterButtons: `TextButton t:welcome.skip` · ⚡: `↩ []`
- [ ] **`ln-interests`** · Page · page state `picked:list<string>=[]` · transition `sharedAxis`
  - [ ] `AppBar` title `t:interests.title` · automaticallyImplyLeading true
  - [ ] body `SingleChildScrollView` padding 16 › `Wrap` spacing 8 · runSpacing 8 › `ForEach` items `{{ data.categories.value }}` › `FilterChip` label `{{ item.label }}` · avatar Icon `{{ item.icon }}` · selected `{{ item.key in page.picked }}` · showCheckmark true · ⚡onSelected: `condition {{ event }}` → `setState page.picked {{ page.picked + [item.key] }}` / else `setState page.picked {{ filter(page.picked, k, k != item.key) }}`
  - [ ] bottomNavigationBar: `◇SubmitBar` label `t:common.done` · enabled `{{ size(page.picked) > 0 }}` · ⚡submit: `setState app.interests {{ page.picked }}` → `setState app.onboarded true` → `→⤓ until home`
  - [ ] 🧪 nothing picked → button disabled (drawn disabled on canvas); 9+ chips wrap at 200 % text
- [ ] **`ln-language`** · Bottom sheet · result `string` · dismissible
  - [ ] `Column` mainAxisSize min › `ListTile` title `t:lang.title` style list; 4× `RadioListTile` value `en|de-DE|ar|am-ET` · groupValue `{{ device.locale }}` · title the language's own name · controlAffinity trailing · ⚡onChanged: `setLocale {{ event }}` → `↩ {{ event }}`
  - [ ] 🧪 switching to `ar` mirrors every page at runtime without restart; `am-ET` uses Ethiopic glyphs
- [ ] **`inbox`** · Page · data `notices` · lifecycle onEnter `subscribe inbox`; onLeave `unsubscribe inbox`
  - [ ] `AppBar` title `t:inbox.title` · actions `PopupMenuButton` items `[{value: read, label: t:inbox.markAll}, {value: clear, label: t:inbox.clear}]` · ⚡onSelected: `switch {{ event }}` → read: `forEach {{ data.notices.value }}` › `dbUpdate notices {{ item.id }} {read: true}`; clear: `⇢ ps-confirm(…destructive: true) ⇒ ok` → `forEach … dbDelete`
  - [ ] body `RefreshIndicator` triggerMode anywhere · ⚡onRefresh: `refreshData notices` › `ListView` items `{{ data.notices.value ?? [] }}` · padding v8 · empty: `◇EmptyHint icon notifications_off` · item: `Dismissible` direction endToStart · background `ColoredBox @color.error` › `Icon delete` · ⚡onDismissed: `dbDelete notices {{ item.id }}` → `showSnackbar t:inbox.deleted actionLabel t:common.undo` › `ListTile` title `{{ item.title }}` · subtitle `{{ item.body }}` maxLines 2 · trailing `Text {{ format.date(item.at) }}` · tileColor: unread rows `@color.primaryContainer`, read rows none (`visible`-style binding on `item.read`) · ⚡onTap: `dbUpdate … {read: true}` → `switch {{ item.route }}` cases `[pass, ex-org]` → `→ pass(id: {{ item.params.id }})` / `→ ex-org(slug: {{ item.params.slug }})`
- [ ] **`ln-whats-new`** · Dialog · ⚡ close `TextButton t:common.close` → `kvSet whatsNewSeen {{ env.termsVersion }}` → `↩`
  - [ ] `ConstrainedBox` maxHeight 420 › `SingleChildScrollView` › `MarkdownView` source `t:whatsNew.body` · ⚡onLink: `openUrl {{ event }}`
- [ ] **`not-found`** · Page (app `notFound`) › `EmptyState` icon search_off · title `t:nf.title` · message `t:nf.body` · action `FilledButton t:nf.home` → `→⤒ home`; 🧪 open `applyreg://nope` and `https://<your-domain>/p/no-such-route`
- [ ] **`ln-offline`** · Page (fallback of every plugin) › `ErrorState` icon cloud_off · title `t:fb.title` · message `t:fb.body` · retryLabel `t:common.retry` · ⚡onRetry: `sync` → `→⟲ home` (raw on purpose: the fallback must not depend on a component); 🧪 kill-switch `cityrun` (§J3): its pages show this

### G2. `explore` — Explore

Plugin: capabilities `deviceApis [location, camera]` · state `query:string=""` (**session**), `sort:string="relevance"` (persisted), `filters:Filters` (persisted), `view:string="list"` · dataSources `orgs` rest `GET /orgs` params q, category, sort, near, radiusKm · `select: items` · pagination **cursor** (pageSize 20, cursorParam cursor, nextCursor next, hasMore more) · cache **cacheFirst** TTL 120 · mocks success (20 orgs) / empty / error · `org` rest `GET /orgs/{slug}` cache **networkFirst** · `status` rest `GET /orgs/{slug}/status` cache **networkOnly**

- [ ] **`explore`** · Page (tab) · lifecycle onInit `refreshData orgs`
  - [ ] `Scaffold` › appBar `AppBar` title `t:explore.title` · bottom: `Padding` › `SearchAnchor` hintText `t:explore.search` · viewHintText `t:explore.searchHint` · isFullScreen true · suggestions `{{ app.recentSearches }}` · ⚡onSelected [debounce:300]: `setState plugin.query {{ event }}` → `setState app.recentSearches {{ slice(distinct([event] + app.recentSearches), 0, 8) }}` → `refreshData orgs`
  - [ ] body `Column` › `Row` › `Expanded` › `FilterBar` filters `{{ map(data.categories.value, c, {"id": c.key, "label": c.label, "selected": c.key == plugin.filters.category}) }}` · ⚡onToggle: `patchState plugin.filters {category: {{ event }}}` → `refreshData orgs`; `IconButton` tune · ⚡: `⇢ ex-filters(current: {{ plugin.filters }}) ⇒ Filters` → `setState plugin.filters {{ steps.f.output }}`; `SegmentedButton` segments list/map (icons) · selected `{{ [plugin.view] }}` · showSelectedIcon false · ⚡onSelectionChanged: `setState plugin.view {{ first(event) }}` → `condition {{ first(event) == "map" }}` → `→ ex-map`
    - [ ] `Expanded` › `Scrollbar` thumbVisibility true · interactive true › `InfiniteList` items `{{ data.orgs.value ?? [] }}` · status `{{ data.orgs.status }}` · hasMore `{{ data.orgs.hasMore }}` · padding 16 · ⚡onEndReached [drop]: `refreshData orgs more: true` · item: `◇OrgCard` org `{{ item }}` compact `{{ device.sizeClass == "compact" }}` · ⚡open: `→ ex-org(slug: {{ event.slug }})` · loading: `SkeletonLoader` lines 3 · avatar true · shimmer true · empty: `◇EmptyHint t:explore.none` · error: `◇ErrorBlock` title `t:explore.failed` · ⚡retry `refreshData orgs` (insert with template `list-states`)
  - [ ] floatingActionButton `FloatingActionButton` tooltip `t:explore.scan` › Icon qr_code_scanner · ⚡: `→ ex-scan`
  - [ ] 🧪 page 2 loads at the end; the error mock shows ErrorState; empty mock; slow network (Dev app 3G) shows skeletons; `Loading more` row on canvas (`Show › Loading more`)
- [ ] **`ex-filters`** · Full-screen dialog · params `current:Filters` (mock `{radiusKm: 10}`) · result `Filters` · page state copies `params.current` (onInit `setState page.f {{ params.current }}`)
  - [ ] `AppBar` leading `CloseButton` ⚡ `↩ null` · actions `TextButton t:common.reset` ⚡ `resetState page.f`
  - [ ] `ListView` › `ListTile t:filters.distance` + `RangeSlider` values `{{ {"start": 0, "end": page.f.radiusKm} }}` min 0 max 50 divisions 10 · labels `{{ {"start": "0 km", "end": string(page.f.radiusKm) + " km"} }}` ⚡onChangeEnd: `patchState page.f {radiusKm: {{ event.end }}}`; `SwitchListTile t:filters.openNow` value `{{ page.f.openNow }}`; `CheckboxListTile t:filters.verified` tristate false; `DropdownMenu` label `t:filters.sort` · dropdownMenuEntries relevance/closing/distance/name · initialSelection `{{ plugin.sort }}` · enableFilter true · width 280 · ⚡onSelected: `setState plugin.sort {{ event }}`; 3× `RadioListTile` fee any/free/paid · groupValue `{{ page.f.fee }}`; `Wrap` of `ChoiceChip` per category · selected `{{ page.f.category == item.key }}`
  - [ ] persistentFooterButtons `FilledButton t:filters.apply` ⚡ `↩ {{ page.f }}`
- [ ] **`ex-category`** · Page · params `key:string` (mock `education`) · page state `tab:int=0` · title `{{ first(filter(data.categories.value, c, c.key == params.key)).label }}`
  - [ ] `NestedScrollView` floatHeaderSlivers true › headerSlivers: `SliverAppBar` pinned floating snap · title · bottom: `TabBar` isScrollable true · tabs `{{ subcategories }}` · indicatorSize label · selectedIndex `{{ page.tab }}` ⚡onTap `setState page.tab`; body: `TabBarView` index `{{ page.tab }}` ⚡onIndexChanged `setState page.tab` › `GridView` items `{{ data.orgs.value }}` · maxCrossAxisExtent 220 · childAspectRatio 0.8 · `E:` maxCrossAxisExtent 300 · item `◇OrgCard`
- [ ] **`ex-map`** · Page · lifecycle onEnter: `requestPermission location ⇒ ok` → `condition {{ steps.p.output }}` → `getLocation accuracy balanced` (retry count 1 on timeout) → `setState page.center {{ {"lat": steps.l.output.latitude, "lng": steps.l.output.longitude} }}`; else `showSnackbar t:map.denied actionLabel t:common.settings` (⚡ action: `callNative openAppSettings`)
  - [ ] `Stack` › `MapView` semanticLabel `t:map.label` · center `{{ page.center }}` · zoom 13 · minZoom 3 · maxZoom 18 · markers `{{ map(data.orgs.value ?? [], o, {"id": o.slug, "position": {"lat": o.lat, "lng": o.lng}, "title": o.name}) }}` · showUserLocation true · tileUrlTemplate `{{ env.mapTiles }}` · attribution "© OpenStreetMap" · ⚡onMarkerTap: `⇢ ex-peek(slug: {{ event }})` · ⚡onCameraIdle [debounce:500]: `setState page.center {{ event.center }}` → `refreshData orgs`
    - [ ] `Positioned` top 16 start 16 end 16 › `SafeArea` › `◇BaseCard` variant raised · dense › content `SearchBar`; `Positioned` bottom 24 end 16 › `FloatingActionButton` mini true › `AnimatedRotation` turns `{{ page.locating ? 1.0 : 0.0 }}` duration 600 › Icon my_location
  - [ ] 🧪 permission denied → snackbar → app settings; GPS off → `getLocation` error routed to onError; RTL keeps the map un-mirrored but the controls mirrored
- [ ] **`ex-peek`** · Bottom sheet · params `slug:string` · result `PeekResult{action:string}` · data `org`, `status`
  - [ ] `BackdropFilter` filter sigmaX 12 sigmaY 12 tileMode clamp › `Container` decoration color @color.surface 85 % · borderRadius top @radius.xl › `Padding 16` › `Column` mainAxisSize min · spacing 12
    - [ ] `AnimatedOpacity` opacity `{{ data.status.value.open ? 1.0 : 0.5 }}` duration 200 › `Hero` tag `org-{{ params.slug }}` › `◇OrgCard` org `{{ data.org.value }}` showFee true
    - [ ] `Row` › `Chip` avatar Icon schedule · label `CountdownTimer endsAt {{ data.status.value.closesAt }} format dhms` (⚡onFinished: `refreshData status`); `Badge` label `{{ data.status.value.seatsLeft }}` › `Chip t:peek.seats`
    - [ ] `Row` › `Expanded` `FilledButton t:peek.register` · enabled `{{ data.status.value.open }}` ⚡ `↩ {action: "register"}`; `OutlinedButton t:peek.details` ⚡ `↩ {action: "details"}`; `IconButton` share ⚡ `share url: "https://<your-domain>/o/{{ params.slug }}"`
  - [ ] 🧪 closed org → Register disabled; swipe down returns null (dismissible) and Home ignores it
- [ ] **`ex-org`** · Page · params `slug:string` (deep link `/o/{slug}`, `/join/{slug}`) · data `org` · onEnter `setState app.lastOrg {{ params.slug }}` (session)
  - [ ] `DataScope` source `org` · loading `SkeletonLoader lines 6` · error `◇ErrorBlock` ⚡retry `refreshData org` · empty `◇EmptyHint` · success:
    - [ ] `CustomScrollView` › `SliverAppBar` expandedHeight 220 pinned · flexibleSpace `Hero` tag `org-{{ params.slug }}` › `◇HeroHeader` image `{{ data.org.value.cover }}` · title `{{ data.org.value.name }}` · subtitle `{{ data.org.value.city }}`; `SliverToBoxAdapter` › `Padding` › `Column` › `Row` [`CircleAvatar` radius 28 backgroundImage logo, `Text` name headlineSmall, `If {{ data.org.value.verified }}` then `Tooltip` message `t:org.verified` triggerMode tap › `Icon ar-verified`] · `MarkdownView` source `{{ data.org.value.description }}` · `RichText` textSpan `[{text: t:org.agree}, {text: t:org.terms, style: {fontWeight: w700, decoration: underline}}]` · `Wrap` of `InputChip` per tag (isEnabled false)
    - [ ] `Match` value `{{ params.slug }}` cases `[bank, campus, cityrun, devconf, clinic, relief, homes, arena]` branches: `FilledButton t:org.start` → `→ nb-welcome` / `→ cp-home` / `→ cr-race` / `→ dc-home` / `→ cl-welcome` / `→ rl-campaign` / `→ hm-listings` / `→ ar-join` · otherwise: `◇EmptyHint t:org.soon` + `FilledButton.tonal t:org.notify` → `apiCall api.notifyMe {slug}` (retry 2, backoff 200) → `showToast t:org.notified`
  - [ ] 🧪 `/o/bank` deep link opens here then into Nordvia; `/o/unknown` → Error mock; missing `slug` param → `PLX-4101` fallback, not a crash
- [ ] **`ex-scan`** · Page · lifecycle onEnter `requestPermission camera`
  - [ ] `Stack` › `QrScanner` cameraFacing back · torch `{{ page.torch }}` · overlayHint `t:scan.hint` · continuous false · ⚡onDetected: `condition {{ startsWith(event.value, "https://<your-domain>/o/") }}` → `openUrl {{ event.value }}` (the host routes the app link) else `haptic vibrate` → `showSnackbar t:scan.invalid`; ⚡onError: `showSnackbar t:scan.error`
  - [ ] `Positioned` bottom 32 › `Row` › `IconButton` flash ⚡ `setState page.torch {{ !page.torch }}`; `TextButton t:scan.system` ⚡ `scanCode formats [qrCode] ⇒ s` → `openUrl {{ steps.s.output.value }}`
  - [ ] 🧪 a non-app QR → snackbar; camera denied → message, no crash

### G3. `passes` — Passes

Plugin: capabilities `deviceApis [share, clipboard, notifications]`, `functions [pass.issue]`, `nativeRoutes []` · dataSources `active` **database** `registrations` filter `status in [submitted, approved]` orderBy `rank` · `drafts` database filter `status eq draft` · `past` database filter `status in [done, withdrawn]` orderBy `submittedAt` descending · `remote` rest `GET /me/registrations` auth · pagination **offset** (pageSize 25, offsetParam offset) · cache **networkFirst** encrypted · `regLive` **sse** `/sse/registrations/{id}` · triggers `onAppResume` [queue]: `apiCall api.remote` → `forEach {{ steps.r.output }}` › `dbUpsert registrations {{ item.id }} {{ item }}`; `dataSources.regLive.onMessage`: `dbUpdate registrations {{ event.id }} {status: {{ event.status }}}` → `announce {{ t("pass.status." + event.status) }} assertive`

- [ ] **`passes`** · Page (tab) · page state `tab:int=0`
  - [ ] `Scaffold` › appBar `AppBar` title `t:passes.title` · actions `IconButton sync` ⚡ `sync` → `refreshData remote` · bottom `TabBar` tabs `[t:passes.active, t:passes.drafts, t:passes.past]` · selectedIndex `{{ page.tab }}` · indicatorSize tab · ⚡onTap `setState page.tab {{ event }}`
  - [ ] body `TabBarView` index `{{ page.tab }}` · ⚡onIndexChanged `setState page.tab {{ event }}`
    - [ ] Active: `ReorderableListView` items `{{ data.active.value ?? [] }}` · buildDefaultDragHandles true · padding 8 · ⚡onReorder: `forEach {{ event.order }}` › `dbUpdate registrations {{ item }} {rank: {{ index }}}` · header `Text t:passes.dragHint` labelSmall · empty `◇EmptyHint t:passes.empty` (⚡cta `switchTab explore`) · item: `ListTile` leading `Hero` tag `pass-{{ item.id }}` › `Icon ar-pass` · title `{{ item.orgName }}` · subtitle `{{ format.date(item.eventAt) }}` · trailing `◇StatusChip` status `{{ item.status }}` · ⚡onTap `→ pass(id: {{ item.id }})` · ⚡onLongPress `dbUpdate … {pinned: {{ !item.pinned }}}` → `haptic selection`
    - [ ] Drafts: `ListView` items `{{ data.drafts.value ?? [] }}` · item `Dismissible` direction endToStart ⚡onDismissed: `⇢ ps-confirm(title: t:draft.discardQ, destructive: true) ⇒ ok` → `condition {{ steps.c.output }}` → `dbDelete registrations {{ item.id }}` · `ListTile` ⚡onTap `⇢ ps-draft(id: {{ item.id }}) ⇒ string` → `switch` resume/discard
    - [ ] Past: `ListView` items `{{ data.past.value ?? [] }}` · physics clamping · item `ListTile` enabled false when withdrawn · trailing `IconButton rate_review` ⚡ `⇢ ps-feedback(id) ⇒ int` → `showToast t:feedback.thanks`
  - [ ] 🧪 reorder survives restart (DB); drafts empty → EmptyHint; RTL swipe direction flips
- [ ] **`pass`** · Page · params `id:string` (mock `r-001`; deep link `/pass/{id}`; push target) · page state `reg:Registration?` · lifecycle onInit: `dbQuery registrations where {{ id == params.id }} limit 1 ⇒ q` → `setState page.reg {{ first(steps.q.output) }}` → `condition {{ page.reg.passCode == "" }}` → `invokeFunction pass.issue {registrationId: {{ params.id }}}` (retry count 2 on network, timeoutMs 4000) → `dbUpdate … {passCode: …}`; onEnter `subscribe regLive {id: {{ params.id }}}`; onLeave `unsubscribe regLive` · transition `platform` · `secure: false`
  - [ ] `Scaffold` › appBar `AppBar` title `{{ page.reg.orgName }}` · actions `MenuAnchor` items share/copy/withdraw · ⚡onSelected `switch`: share `share text {{ t("pass.shareText", {org: page.reg.orgName, code: page.reg.passCode}) }} url "https://<your-domain>/pass/{{ params.id }}"` · copy `copyToClipboard {{ page.reg.passCode }}` → `showSnackbar t:pass.copied` · withdraw `⇢ ps-confirm(destructive: true) ⇒ ok` → `apiCall api.withdraw {id} optimistic {page.reg.status: "withdrawn"}` (🧪 `X-AR-Fail: 500` rolls the chip back)
  - [ ] body `SingleChildScrollView` › `Column` spacing 16 · crossAxisAlignment center
    - [ ] `Hero` tag `pass-{{ params.id }}` › `Card` elevation @elevation.3 · shape roundedRectangle @radius.xl (raw on purpose: tests `Card` elevation/shape tokens) › `Padding 24` › `QrCodeView` data `{{ page.reg.passCode }}` · size 220 · errorCorrection quartile · quietZone 4 · semanticLabel `t:pass.qr`
    - [ ] `MaskedText` value `{{ page.reg.passCode }}` · mask custom · visibleDigits 4 · revealable true · revealLabel `t:pass.reveal`
    - [ ] `◇InfoRow` label `t:pass.code` · value `{{ page.reg.passCode }}` · copyable true · ltr true; `◇StatusChip` status `{{ page.reg.status }}`
    - [ ] `StatusTimeline` steps `{{ [{"label": t("st.sent"), "state": "done"}, {"label": t("st.review"), "state": page.reg.status == "submitted" ? "current" : "done"}, {"label": t("st.approved"), "state": page.reg.status == "approved" ? "done" : "pending"}] }}`
    - [ ] `Row` mainAxisAlignment spaceEvenly › `NativeSlot` name `CalendarButton` (prop title `{{ page.reg.orgName }}`; ⚡onAdded `showToast t:pass.added`) · `TextButton t:pass.calendar` ⚡ `callNative addToCalendar {title, start: {{ page.reg.eventAt }}} ⇒ ok` → `condition` → `showToast` · `OutlinedButton t:pass.live` ⚡ `→ ps-live(id)` · `OutlinedButton t:pass.receipt` ⚡ `→ ps-receipt(id)` · `TextButton t:pass.org` ⚡ `→ ex-org(slug: {{ page.reg.orgSlug }})`
  - [ ] 🧪 push opens this page cold (`handlePushPayload`); unknown id → EmptyHint; `pass.issue` business error `NOT_OWNER` routed to page `onError` → snackbar
- [ ] **`ps-live`** · Page · params `id` · data `regLive`
  - [ ] `Column` › `LiveEtaCard` title `t:live.title` · status `{{ data.regLive.value.status }}` · etaMinutes `{{ data.regLive.value.waitMinutes }}` · minutesLabel `t:common.min` · icon schedule; `AnimatedSwitcher` duration 300 › `Text {{ data.regLive.value.note }}` (key changes per message); `Sparkline` values `{{ page.history }}` semanticLabel `t:live.trend`; `LinearProgressIndicator` value null while `data.regLive.loading`
  - [ ] triggers on this page: `dataSources.regLive.onMessage` → `setState page.history {{ slice(page.history + [double(event.waitMinutes)], -20, 0) }}` (check `slice` args in the editor) · 🧪 kill the server: reconnect with backoff, resubscribe
- [ ] **`ps-receipt`** · Page · params `id`
  - [ ] `Table` columns 2 · columnWidths `[{kind: flex, value: 2}, {kind: intrinsic}]` · border `@color.outlineVariant 1` · defaultVerticalAlignment middle › rows: label `Text` / `MoneyText` value `{{ page.reg.amount }}` display code · signDisplay auto; `Divider` thickness 1; `◇MoneyRow` label `t:receipt.total` · amount `{{ page.reg.amount }}` · emphasis true; `TransactionListItem` title `t:receipt.fee` · amount `{{ page.reg.amount }}` · status completed · date `{{ page.reg.submittedAt }}`; `SelectableText` `{{ page.reg.id }}` style monospace; `ElevatedButton` (icon share) `t:receipt.share` ⚡ `share text …`
- [ ] **`ps-feedback`** · Bottom sheet · params `id` · result `int`
  - [ ] `Padding` (bottom inset) › `Column` › `RatingInput` max 5 · starLabel `t:common.star` · ⚡onChanged `setState page.rating {{ event }}`; `TextField` decoration outlined labelText `t:feedback.comment` · maxLines 4 · maxLength 280 · maxLengthEnforcement enforced · textInputAction newline; `FilledButton t:common.send` enabled `{{ page.rating > 0 }}` ⚡ `apiCall api.feedback {…}` → `↩ {{ page.rating }}`
- [ ] **`ps-confirm`** · Dialog · params `title:string`, `message:string?`, `destructive:bool=false` · result `bool` (shared by every plugin)
  - [ ] `Column` mainAxisSize min › `Text {{ params.title }}` headlineSmall; `If {{ params.message != null }}` then `Text`; `OverflowBar`-like `Row` end › `TextButton t:common.cancel` ⚡ `↩ false`; `FilledButton` style backgroundColor Default `{{ params.destructive ? @color.error : @color.primary }}` (bind each branch to its token; state colours: Default, Pressed, Focused, Disabled) ⚡ `↩ true`
- [ ] **`ps-draft`** · Dialog · params `id` · result `string` (`resume` | `discard`) · `ListTile`s with icons; ⚡ `↩ "resume"` / `↩ "discard"`

### G4. `account` — Account

Plugin: capabilities `deviceApis [camera, photos, biometrics, notifications, share]`, `nativeRoutes [sign-in, licenses]` · guard graph **`require-sign-in`** (`GuardResult`): `condition {{ user.authenticated }}` → `stop allow` / `stop {decision: redirect, route: ac-sign-in}` · plugin state `draftProfile:Profile` (session)

- [ ] **`profile`** · Page (tab) · guards `[require-sign-in]` · data app `me`
  - [ ] `CustomScrollView` › `SliverToBoxAdapter` › `ProfileHeader` name `{{ data.me.value.name }}` · subtitle `{{ format.phone(data.me.value.phone) }}` · avatar `{{ data.me.value.avatar }}` · radius 36 · actions: `IconButton edit` ⚡ `→ ac-edit`
  - [ ] `SliverToBoxAdapter` › `SettingsList` sections `[{title: t:set.general, count: 3}, {title: t:set.privacy, count: 2}, {title: t:set.about, count: 2}]` + `ListTile`s: Preferences → `ac-preferences`; Privacy → `ac-privacy`; Security → `ac-security`; About → `ac-about`; staff only (`visible {{ app.isStaff }}`): Console → `cs-dashboard`; `visible {{ flags.showMaintainers }}`: Maintainers → `mt-home`
  - [ ] `SliverToBoxAdapter` › `TextButton t:acc.signOut` ⚡ `⇢ ps-confirm ⇒ ok` → `logout` (host wipes)
- [ ] **`ac-sign-in`** · Page · params `reason:string="profile"` · lifecycle none
  - [ ] `Center` › `Column` › `Icon lock_person` 72; `Text t:signin.why`; `FilledButton t:signin.go` ⚡ `→ native:sign-in(reason: {{ params.reason }})` → `condition {{ user.authenticated }}` → `→⟲ profile`; `TextButton t:common.later` ⚡ `switchTab home`
  - [ ] 🧪 a guard redirect from `profile` lands here; signing in natively returns and replaces
- [ ] **`ac-edit`** · Page · form **`profile`**: fields `name` (required, length 2–60), `email` (required, email), `phone` (phone, region from device), `birth` date (required, dateRange 1900-01-01…today−13y), `country` (required) · page state `avatar:PickedFile?`
  - [ ] `FormScope` form profile › `ListView` padding 16 ›
    - [ ] `Stack` › `CircleAvatar` radius 48 · backgroundImage `{{ data.me.value.avatar }}` · foregroundImage `{{ page.avatar }}`; `Positioned` bottom 0 end 0 › `IconButton` style filled › Icon photo_camera ⚡ `⇢ ac-photo-source ⇒ string` → `switch` camera: `capturePhoto maxDimension 1024` / gallery: `pickImage multiple false maxDimension 1024` → `setState page.avatar {{ first(steps.p.output) }}`
    - [ ] `TextFormField` decoration filled labelText `t:f.name` · prefixIcon person · textCapitalization words · autofillHints [name] · value `{{ form.values.name }}` · forceErrorText `{{ form.touched.name ? form.errors.name : null }}` · autovalidateMode onUserInteraction · ⚡onChanged `setState page.profile.values.name {{ event }}`
    - [ ] `TextFormField` labelText `t:f.email` · keyboardType emailAddress · textInputAction next · autocorrect false · enableSuggestions false
    - [ ] `PhoneInput` label `t:f.phone` · region `{{ data.me.value.region }}` · invalidMessage `t:f.phoneBad`
    - [ ] `DualCalendarDatePicker` value `{{ form.values.birth }}` · firstDate 1900-01-01 · lastDate `{{ addDays(now, -4745) }}` · calendar `{{ device.calendar }}`-driven: gregorian · secondaryCalendar ethiopic (am-ET) / islamicUmalqura (ar) / persian (Maintainers)
    - [ ] `CountryPicker` value `{{ form.values.country }}` · searchHint `t:f.countrySearch`
  - [ ] bottomNavigationBar `◇SubmitBar` busy `{{ page.profile.status == "submitting" }}` · ⚡submit: `submitForm profile` (onError → `announce t:form.fix assertive`) → `apiCall api.updateMe {…} optimistic {…}` → `showSnackbar t:saved` → `pop`
  - [ ] 🧪 invalid phone for region DE; birthday too young; Ethiopic calendar shows both dates; avatar camera permission denied
- [ ] **`ac-photo-source`** · Bottom sheet · result `string` › 2 `ListTile`s ⚡ `↩ "camera"` / `↩ "gallery"`
- [ ] **`ac-preferences`** · Page
  - [ ] `ListView` › `ListTile t:pref.theme` + `SegmentedButton` segments system/light/dark (icons) · selected `{{ [app.themePref] }}` · ⚡onSelectionChanged `setState app.themePref {{ first(event) }}` → `setThemeMode {{ first(event) }}`
  - [ ] `ListTile t:pref.language` ⚡ `⇢ ln-language ⇒ string`
  - [ ] `SwitchListTile t:pref.notifications` value `{{ page.notif }}` · ⚡onChanged `requestPermission notifications ⇒ ok` → `setState page.notif {{ steps.p.output }}` · thumbIcon colours via `thumbColor` states (Selected, Disabled)
  - [ ] `ListTile t:pref.textSize` subtitle `{{ format.percent(device.textScale) }}` (read-only, explains system setting)
  - [ ] 🧪 theme switch recolours open pages; dark + high contrast; `system` follows the device
- [ ] **`ac-privacy`** · Page · page state `analytics:bool`, `experiments:bool`, `replay:bool` (persisted via `kvSet`)
  - [ ] `ConsentCheckbox` label `t:privacy.analytics` · documentVersion `{{ env.termsVersion }}` · linkLabel `t:privacy.read` · ⚡onChanged `setState page.analytics` · ⚡onLinkTap `openUrl {{ env.privacyUrl }}`; same for experiments, replay
  - [ ] `FilledButton t:privacy.save` ⚡ `kvSet consent {{ {"analytics": page.analytics, "experiments": page.experiments, "replay": page.replay} }}` → `emitHostEvent consentChanged {{ {"analytics": page.analytics, "experiments": page.experiments, "replay": page.replay} }}` → `showToast t:saved`
  - [ ] `ListTile t:privacy.export` ⚡ `apiCall api.export ⇒ x` → `share url: {{ steps.x.output.url }}`
  - [ ] `TermsViewer` source `t:legal.privacy` · version `{{ env.termsVersion }}` · acceptLabel `t:legal.accept` · scrollHint `t:legal.scroll` · ⚡onAccepted `trackEvent terms_accepted`
  - [ ] 🧪 withdraw analytics: buffered events of that category are deleted (Analytics shows none afterwards)
- [ ] **`ac-security`** · Page · **`secure: true`** · requiresAssurance AL1
  - [ ] `SwitchListTile t:sec.lock` ⚡onChanged `biometricAuth reason t:sec.reason allowDeviceCredential true ⇒ ok` → `condition` → `kvSet lockOn true`
  - [ ] `ListTile t:sec.pin` › `SecurePinPad` length 4 · randomizeLayout false · haptics true · ⚡onCompleted `setState app.pinHash {{ event }}` (stored **secure** + **sensitive**; the server never sees it) → `showToast t:sec.pinSet`
  - [ ] 🧪 screenshot blocked on this page (Android FLAG_SECURE; iOS cover); app switcher blurred; inactivity lock after 300 s asks biometrics; `app.pinHash` never appears in the Process flow trace
- [ ] **`ac-delete`** · Dialog · result `bool` · `TextField` labelText `t:del.typeKey` (must equal `DELETE`) · `FilledButton` enabled `{{ page.typed == "DELETE" }}` ⚡ `apiCall api.deleteAccount` → `logout` → `↩ true`
- [ ] **`ac-about`** · Page › `NativeSlot HostVersionTile` (⚡onLongPress → `setState page.taps {{ page.taps + 1 }}` → `condition {{ page.taps >= 5 }}` → `→ mt-home`); `ListTile t:about.licenses` ⚡ `→ native:licenses`; `ListTile t:about.rate` ⚡ `callNative requestReview`; `ListTile t:about.website` ⚡ `openUrl https://<your-domain>`

### G5. `verify` — Verify (reusable; typed in, typed out)

Plugin: capabilities `deviceApis [camera, location, biometrics]` · exported flows (graph `exported: true`, typed `inputs`/`output`):

- [ ] **flow `verifyPhone`** `(phone:string) → VerifyResult`: `apiCall api.otpStart {phone}` → `openBottomSheet vf-otp(phone) ⇒ r` → `stop {{ steps.r.output ?? {"verified": false} }}`
- [ ] **flow `captureIdentity`** `() → {document:string, selfie:string}`: `openDialog vf-id ⇒ d` → `condition {{ d.verified }}` → `openDialog vf-selfie ⇒ s` → `stop {…}`; else `stop error: "ID_CANCELLED"`
- [ ] **flow `stepUp`** `(reason:string) → bool`: `openDialog vf-step-up(reason) ⇒ ok` → `stop {{ steps.ok.output ?? false }}`

Screens:

- [ ] **`vf-otp`** · Bottom sheet · params `phone:string` · result `VerifyResult` · page state `left:int=60` · triggers timer `tick` intervalMs 1000 repeat → `condition {{ page.left > 0 }}` → `setState page.left {{ page.left - 1 }}`
  - [ ] `Padding` › `Column` › `Text t:otp.sent {phone: {{ format.phone(params.phone) }}}` (ICU arg); `OtpInput` length 6 · alphabet numeric · autofill true · ⚡onCompleted [drop]: `apiCall api.otpVerify {code: {{ event }}}` (onError → `haptic heavy` → `setState page.err true`) → `↩ {verified: true, value: {{ params.phone }}}`; `If {{ page.err }}` then `Text t:otp.wrong` color @color.error; `TextButton` enabled `{{ page.left == 0 }}` · child `Text {{ page.left > 0 ? t("otp.resendIn", {s: page.left}) : t("otp.resend") }}` ⚡ `apiCall api.otpStart` → `setState page.left 60`
  - [ ] 🧪 wrong code (server `000001`) → message + haptic; resend disabled until 0; SMS autofill on Android
- [ ] **`vf-id`** · Full-screen dialog · result `VerifyResult` · lifecycle onEnter `requestPermission camera`
  - [ ] `Stack` › `KycCameraCapture` mode document · ⚡onCaptured: `apiCall api.upload {file: {{ event }}} ⇒ u` → `↩ {verified: true, value: {{ steps.u.output.fileId }}}`; `Positioned` top › `SafeArea` › `Text t:kyc.doc` on scrim; `CloseButton` ⚡ `↩ {verified: false}`
- [ ] **`vf-selfie`** · Full-screen dialog · `KycCameraCapture` mode selfie (same as above)
- [ ] **`vf-signature`** · Bottom sheet · result `VerifyResult` · `SignaturePad` format svg · strokeWidth 2.5 · height 200 · clearLabel `t:sig.clear` · ⚡onChanged `setState page.svg {{ event }}`; `FilledButton` enabled `{{ page.svg != "" }}` ⚡ `↩ {verified: true, value: {{ page.svg }}}`
- [ ] **`vf-address`** · Full-screen dialog · result `Address` · page state `pin:LatLng`, `text:string`
  - [ ] `Column` › `Autocomplete` options `{{ page.suggestions }}` · hintText `t:addr.search` · optionsMaxHeight 240 · ⚡onSelected `setState page.text {{ event }}`; `Expanded` › `MapView` center `{{ page.pin }}` zoom 16 · markers `[{id: pin, position: {{ page.pin }}}]` · ⚡onMapTap `setState page.pin {{ event }}`; `FilledButton.icon` my_location ⚡ `getLocation accuracy high` → `setState page.pin …`; `FilledButton t:common.use` ⚡ `↩ {text, lat, lng}`
- [ ] **`vf-step-up`** · Dialog · params `reason:string` · result `bool` › `BiometricButton` label `t:stepup.confirm` · reason `{{ params.reason }}` · ⚡onSuccess `↩ true` · ⚡onFailure `↩ false`
- [ ] 🧪 on every caller's canvas these pages show as **ghost screens** (plugin name, screen, thumbnail, link count); clicking one asks "Leave … and open Verify › …?", opens Verify, pans to the screen and pulses it

### G6. `pay` — Pay (reusable wallet payment)

Contract: **input** `PayRequest {amount:money, payee:string, reference:string, purpose:string}` · **output** `PayResult {status: success|error|cancelled, transactionId?, errorCode?, message?}`. Callers use `openDialog pay(…) ⇒ PayResult` or the exported flow.

Plugin: capabilities `functions [pay.simulate]`, `deviceApis [haptics]` · flow **`payWithWallet`** (exported) `(req: PayRequest) → PayResult`: `openDialog pay(amount, payee, reference, purpose) dismissible false ⇒ r` → `stop {{ steps.r.output ?? {"status": "cancelled"} }}`

- [ ] **`pay`** · Full-screen dialog · params `amount:money` (mock `{"amount":"25.00","currency":"EUR"}`), `payee:string` (mock `Riverton City Run`), `reference:string` (mock `CR-2027-0042`), `purpose:string` · result `PayResult` · page state `step:string="confirm"` (`confirm|processing|success|error`), `result:PayResult?` · transition `slideUp` · `secure: true`
  - [ ] `Scaffold` › appBar `AppBar` leading `CloseButton` ⚡ `↩ {status: "cancelled"}` · title `t:pay.title` · automaticallyImplyLeading false
  - [ ] body `AnimatedSwitcher` duration 250 switchInCurve easeOut › `Match` value `{{ page.step }}` cases `[confirm, processing, success, error]`
    - [ ] confirm: `Column` padding 24 › `Text t:pay.youPay` labelLarge; `MoneyText` value `{{ params.amount }}` · display symbol · style @typography.displayMedium; `◇BaseCard` variant outlined › content `ListTile` leading Icon account_balance_wallet · title `t:pay.wallet` · subtitle `t:pay.walletSub`; `Divider`; `◇InfoRow` label `t:pay.to` value `{{ params.payee }}`; `◇InfoRow` label `t:pay.ref` value `{{ params.reference }}` copyable true ltr true; `◇InfoRow` label `t:pay.for` value `{{ params.purpose }}`; `Spacer`; `FilledButton t:pay.confirm {amount: {{ format.money(params.amount) }}}` ⚡onPressed [drop]: `⇢ pay-pin ⇒ ok` → `condition {{ steps.pin.output == true }}` → `setState page.step "processing"` → `delay 2000` → `invokeFunction pay.simulate {amount: {{ amount(params.amount) }}, currency: {{ currency(params.amount) }}, reference: {{ params.reference }}} ⇒ s` (onError → `setState page.result {status: "error", errorCode: {{ steps.s.error.code }}}` → `setState page.step "error"`) → `setState page.result {{ steps.s.output }}` → `setState page.step {{ steps.s.output.status }}` → `switch {{ page.step }}` → success: `haptic medium`; error: `haptic heavy`
    - [ ] processing: `Center` › `Column` › `CircularProgressIndicator` strokeWidth 4 · strokeCap round; `Text t:pay.processing`; a11y liveRegion true
    - [ ] success: `Column` › `LottieView asset:success.lottie` repeat false · ⚡onCompleted `announce t:pay.done`; `Text t:pay.success` headlineSmall color @color.success; `SelectableText {{ page.result.transactionId }}`; `Expanded` › `Align` alignment bottomCenter › `FilledButton t:common.done` ⚡ `↩ {{ page.result }}`
    - [ ] error: `Column` › `Icon error` 72 color @color.error; `Text {{ t("pay.err." + page.result.errorCode) }}`; `FilledButton t:common.done` ⚡ `↩ {{ page.result }}`; `TextButton t:pay.retry` ⚡ `setState page.step "confirm"`
  - [ ] 🧪 ~1 in 5 tries is an error with one of 4 codes; Done returns to the caller with the typed result; Close returns `cancelled`; Android back on processing does nothing harmful (dismissible false); screenshot blocked
- [ ] **`pay-pin`** · Bottom sheet · result `bool` · dismissible true
  - [ ] `Column` › `Text t:pay.pin` titleMedium; `SecurePinPad` length 6 · randomizeLayout true · haptics true · ⚡onCompleted: `↩ true` (any 6 digits); `TextButton t:common.cancel` ⚡ `↩ false`
- [ ] Callers (each returns to its own page; on canvas, `pay` appears as a ghost screen there): Campus fee, City Run entry, DevHarbor checkout, Clinic co-pay, Hands Together donation, Brickyard deposit, Pulse Arena membership

### G7. `partner` — "Bring your organisation"

Plugin: capabilities `functions [partner.submit]`, `deviceApis [photos]` · data none (function only)

- [ ] **`pt-pitch`** · Page · transition `scale`
  - [ ] `CustomScrollView` › `SliverToBoxAdapter` › `RiveView asset:pitch-hero.riv` artboard `Main` · stateMachine `Intro` · flags `{"dark": {{ device.darkMode }}}` · fit cover · reduceMotion skip · ⚡onLoaded `trackEvent pitch_seen`
  - [ ] `SliverToBoxAdapter` › `VideoPlayer` url `{{ env.apiBaseUrl }}/media/pitch.mp4` · captions `…/pitch.vtt` · aspectRatio 1.7777778 · autoplay false · ⚡onEnded `startAnimation ctaPulse`
  - [ ] `SliverToBoxAdapter` › `PageView` items `{{ page.testimonials }}` · padEnds true · pageSnapping true · ⚡onPageChanged `setState page.t {{ event }}` · item `Card` › `Text {{ item.quote }}` italic
  - [ ] `SliverToBoxAdapter` › `Flex` direction vertical · `M:` direction horizontal · spacing 12 › 3× `Flexible` fit tight › `◇BaseCard` variant outlined · tappable · selected `{{ page.plan == item }}` (plans: Starter, Growth, Custom; Growth `variant raised`) · ⚡tap `setState page.plan`
  - [ ] `SliverFillRemaining` › `ScaleTransition` timeline `ctaPulse` (repeatForever, reverse) › `FilledButton t:pt.start` ⚡ `→ pt-request`
- [ ] **`pt-request`** · Page · form **`req`**: `orgName` (required, length ≤ 80), `email` (required, email, **async**: plugin flow `checkEmail` = `apiCall api.partnerEmailFree {email} ⇒ r` → `stop {{ steps.r.output.free }}`, debounceMs 600), `category` (required), `size` (required), `pages` int (range 1–20), `needs` list<string>, `plan` (required), `notes` (length ≤ 500); page state `logo:PickedFile?`, `step:int=0`
  - [ ] `WizardForm` titles `[t:pt.org, t:pt.contact, t:pt.needs]` · currentStep `{{ page.step }}` · stepFields `["orgName,category", "email", "pages,needs,plan"]` · ⚡onNext `validateForm req` → `setState page.step +1` · ⚡onBack `setState page.step −1` · ⚡onSubmit `→ pt-review`
    - [ ] step 1: `TextFormField orgName`; `DropdownMenu` category (8 entries, leadingIcon each) · enableSearch true; `SegmentedButton` size small/medium/large · emptySelectionAllowed false; `OutlinedButton.icon` upload logo ⚡ `pickImage multiple false ⇒ f` → `setState page.logo {{ first(steps.f.output) }}`
    - [ ] step 2: `TextFormField email`; `PhoneInput`
    - [ ] step 3: `Slider` value `{{ form.values.pages }}` min 1 max 20 divisions 19 · label `{{ form.values.pages }}` · showValueIndicator onlyForDiscrete; `CheckboxListTile` ×6 needs (payments, uploads, maps, live, multi-page, multilingual); `SegmentedButton` plan starter/growth/custom · multiSelectionEnabled false; `TextField notes` maxLines 5 minLines 3
- [ ] **`pt-review`** · Page › `Column` › `ExpansionTile`s summarising each step (initiallyExpanded first) › each holds template `summary-card` (InfoRows of that step) · template `consent-block`; `◇SubmitBar` ⚡submit: `submitForm req` → `invokeFunction partner.submit {…} ⇒ r` (business `DUPLICATE_REQUEST` → `showSnackbar t:pt.duplicate`; `UPSTREAM_DOWN` → retry policy count 2 backoff 500 jitter on function) → `⇢ pt-sent(reference: {{ steps.r.output.reference }})` → `→⤒ home`
- [ ] **`pt-sent`** · Dialog · params `reference` › `LottieView asset:confetti.lottie` repeat false; `Text t:pt.sent`; `SelectableText {{ params.reference }}`; `TextButton t:common.share` ⚡ `share text`; `FilledButton t:common.done` ⚡ `↩`
- [ ] 🧪 submit twice with the same e-mail → `DUPLICATE_REQUEST`; airplane mode → function `network` error handled; you receive the e-mail from `ar-api`

### G8. `console` — Organisation console (staff)

Plugin: capabilities `deviceApis [camera, haptics, share]` · guard **`staff-only`**: `condition {{ user.role == "staff" || user.role == "maintainer" }}` → `stop allow` / `stop {decision: fallback}` · dataSources `stats` rest `GET /staff/orgs/{org}/stats` (param org = `{{ user.orgSlug }}`) · `subs` rest `GET /staff/orgs/{org}/submissions` pagination **page** (pageParam page, pageSize 50) · `check` rest operation `checkin` POST · plugin state `sort:int=0`, `asc:bool=false`

- [ ] **`cs-dashboard`** · Page · guards `[staff-only]` · lifecycle onEnter `refreshData stats`
  - [ ] `Scaffold` › drawer: `NavigationDrawer` destinations dashboard/submissions/check-in · selectedIndex 0 · header `ListTile` org name · ⚡onDestinationSelected `switch` → `→⟲ cs-submissions` / `→⟲ cs-checkin`
  - [ ] body `Responsive` › compact: `ListView` of cards; medium: `Row` [`NavigationRail` destinations … · labelType selected · groupAlignment −1 · leading FAB mini, `Expanded` grid 2 cols]; expanded: `Row` [`NavigationRail` extended true · minExtendedWidth 220, grid 3 cols]
    - [ ] Cards (each `◇BaseCard` variant flat with a `◇SectionHeader`): `LineChart` series `[{name: t:cs.daily, values: {{ data.stats.value.daily }}}]` · labels last 14 days · title `t:cs.registrations` · showTable false; `BarChart` series by form · showTable true (a11y table); `PieChart` slices `{{ data.stats.value.byForm }}` · summaryTemplate default; `◇BaseCard` › `Row` [`MoneyText` value revenue · display narrowSymbol · signDisplay exceptZero; `Sparkline` values daily]
  - [ ] bottomNavigationBar (compact only, `visible {{ device.sizeClass == "compact" }}`): `BottomNavigationBar` items 3 · currentIndex 0 · type fixed · showUnselectedLabels true · ⚡onTap `switch`
  - [ ] 🧪 a member (role `member`) opening this route gets the fallback; tablet frame shows the rail; foldable crease not crossing a chart
- [ ] **`cs-submissions`** · Page · guards `[staff-only]`
  - [ ] `Column` › `SearchField` hint `t:cs.search` · debounceMs 400 · suggestions `{{ page.recent }}` · ⚡onSearch `refreshData subs` · ⚡onSuggestionSelected same; `FilterBar` filters statuses (submitted, approved, rejected) · ⚡onToggle `refreshData subs`; `SortSheet` title `t:cs.sort` · options date/name/status · selected `{{ page.sortKey }}` · ⚡onSelected `setState page.sortKey {{ event }}` → `refreshData subs`
  - [ ] `Expanded` › `SingleChildScrollView` scrollDirection horizontal › `DataTable` columns `[{label: t:cs.name}, {label: t:cs.form}, {label: t:cs.date}, {label: t:cs.amount, numeric: true}, {label: t:cs.status}]` · rows `{{ map(data.subs.value, s, [s.name, s.form, format.date(s.at), format.money(s.amount), s.status]) }}` · sortColumnIndex `{{ plugin.sort }}` · sortAscending `{{ plugin.asc }}` · showCheckboxColumn true · headingRowColor (state colour) · dataRowMinHeight 44 · ⚡onSort `setState plugin.sort {{ event.column }}` → `setState plugin.asc {{ event.ascending }}`; ⚡onSelectChanged → `→ cs-submission(id)`; ⚡onSelectAll `setState page.all`
  - [ ] `persistentFooterButtons` `TextButton.icon t:cs.export` ⚡ `apiCall api.export ⇒ x` → `share url …`
- [ ] **`cs-submission`** · Page · params `id` · `ExpansionTile` per section (initiallyExpanded first · maintainState true · controlAffinity leading) · `Row` › `OutlinedButton t:cs.reject` ⚡ `patchState page.sub {status: "rejected"} optimistic true` → `apiCall api.setStatus` (🧪 `X-AR-Fail: 409` rolls back); `FilledButton t:cs.approve` same with approved → `showSnackbar t:cs.done actionLabel t:common.undo`
- [ ] **`cs-checkin`** · Page · guards `[staff-only]`
  - [ ] `Stack` › `BarcodeScanner` formats `[qrCode, code128, ean13, pdf417]` · continuous true · scanWindow `[0.1, 0.3, 0.9, 0.7]` · ⚡onDetected [throttle:1500]: `apiCall api.checkin {passCode: {{ event.value }}} ⇒ c` → `haptic medium` → `showToast {{ t("cs.welcome", {name: steps.c.output.name}) }}` (onError http 404 → `haptic vibrate` → `showSnackbar t:cs.unknown`)
  - [ ] `Positioned` bottom › `◇BaseCard` variant raised › content `Text {{ t("cs.count", {n: page.count}) }}` (ICU plural)
  - [ ] 🧪 the same code twice within 1.5 s is ignored (throttle); offline → error toast

### Organisation plugins — shared rules

- [ ] Entry page `onEnter`: `emitHostEvent brandRequested {brand: <brand>}` (§C2); `onLeave` of the last page back to Explore: `{brand: null}`
- [ ] Submitting: `apiCall api.register {orgSlug, formKey, answers}` (operation `offlineCapable: true` only for Campus) → `dbInsert registrations {…}` → `emitHostEvent registrationSubmitted {…}` → `trackEvent registration_submitted {org}` → `→⟲ pass(id)`
- [ ] Every org registers its own funnel (§J7): `org_open → form_start → form_step_n → pay_ok → submitted`

### G9. `bank` — Nordvia Bank (fintech · brand `nordvia` · Material · strictest)

Plugin: capabilities `deviceApis [camera, biometrics]`, `functions [bank.eligibility]` · dataSources `accounts` rest operation `open` POST `/accounts` (auth) · **`transfers`** rest, route `plux`, `requiresAssurance: AL2`, `transaction {amount: /amount, currency: /currency, payee: /to/iban, summary: /reference}`, operation `deposit` POST `/transfers` · plugin state `application:BankApp` (session), `taxId:string` (memory, **sensitive**) · approvals: four-eyes policy (§J2) · confidential bundle (`confidentialBundles optional` → this plugin requests it) · experiments here need approval (`ABT-007`)

- [ ] **`nb-welcome`** · Page · page state `product:string="everyday"`, `fade:CrossFadeState=showFirst`, `flipped:bool=false`, `rate:decimal?`, `rateError:bool=false` · onEnter: brand `nordvia` → `readConfig ratesEURUSD` (strict) ⇒ r → `setState page.rate` (onError: `setState page.rateError true`)
  - [ ] `Scaffold` › appBar `AppBar` backgroundColor @color.primary · foregroundColor @color.onPrimary · elevation 0 · scrolledUnderElevation 2 · centerTitle false · titleSpacing 0 · title `Row` [`Image asset:org-bank-logo.png` height 24, `Flexible` › `Text Nordvia` overflow ellipsis] · actions `IconButton help` ⚡ `openUrl …`
  - [ ] body `ListView` padding 16 ›
    - [ ] `AccountCard` name `t:nb.everyday` · maskedNumber "•••• 4821" · balance `{{ money("0.00", "EUR") }}` · flipped `{{ page.flipped }}` · backLabel `t:nb.cardBack` · ⚡onFlip `setState page.flipped {{ !page.flipped }}`
    - [ ] `SegmentedButton` segments everyday/plus (label + icon) · selected `{{ [page.product] }}` · ⚡onSelectionChanged `switch {{ first(event) }}` → everyday: `setState page.product "everyday"` → `setState page.fade "showFirst"`; plus: … `"showSecond"`
    - [ ] `AnimatedCrossFade` crossFadeState `{{ page.fade }}` · duration 300 · sizeCurve easeInOut › first `Text t:nb.everydayBody` / second `Text t:nb.plusBody`
    - [ ] `If {{ page.rateError }}` then `◇InfoRow` label `t:nb.rate` value `t:nb.rateOffline` else `◇InfoRow` label `t:nb.rate` value `{{ format.number(page.rate) }}` icon currency_exchange
    - [ ] `FilledButton t:nb.open` style minimumSize (∞, 52) · shape stadium ⚡ `→ nb-eligibility`; `TextButton t:nb.terms` ⚡ `⇢ nb-terms ⇒ bool`
  - [ ] 🧪 airplane mode: strict value fails closed (`PLX-6068`) and the page shows the offline text
- [ ] **`nb-eligibility`** · Page · form `elig`: `income` money (required, range 0–100000), `age` int (range 16–100), `employment` (required) · page state `reasons:list<string>=[]`, `underage:bool=false`
  - [ ] `◇StepHeader` step 1 of 5 · `MaterialBanner` visible `{{ page.underage }}` · content `t:nb.underage` · leading Icon info · actions `[TextButton t:common.ok ⚡ setState page.underage false]` · forceActionsBelow false · overflowAlignment end · ⚡onVisible `announce t:nb.underage assertive`
  - [ ] `AmountInput` label `t:nb.income` · currency EUR · min 0 · max 100000 · rangeMessage `t:nb.incomeRange` · ⚡onChanged `setState page.elig.values.income {{ event }}`
  - [ ] `Slider` value `{{ double(form.values.age) }}` min 16 max 100 divisions 84 · label `{{ form.values.age }}` · allowedInteraction tapAndSlide · secondaryTrackValue 18
  - [ ] 4× `RadioListTile` employed/self/student/retired · groupValue `{{ form.values.employment }}`
  - [ ] `◇SubmitBar` ⚡submit: `submitForm elig` → `invokeFunction bank.eligibility {monthly_income, age, employment} ⇒ e` (business `UNDERAGE` → `setState page.underage true`) → `condition {{ steps.e.output.eligible }}` → `→ nb-identity` / else `setState page.reasons {{ steps.e.output.reasons }}` (shown in an `AnimatedSize` list)
- [ ] **`nb-identity`** · Page · page state `step:int=0`, `phoneOk`, `idOk`, `addr:Address?`
  - [ ] `Stepper` type horizontal · currentStep `{{ page.step }}` · steps `[{title: t:nb.phone, state: {{ page.phoneOk ? "complete" : "editing" }}}, {title: t:nb.id, state: {{ page.idOk ? "complete" : page.step >= 1 ? "indexed" : "disabled" }}}, {title: t:nb.address, state: {{ page.addr != null ? "complete" : "indexed" }}}]` · connectorColor @color.primary · ⚡onStepTapped `setState page.step {{ event }}` · ⚡onStepContinue `switch {{ page.step }}` → 0: `callFlow verifyPhone {phone: {{ data.me.value.phone }}} ⇒ r` → `setState page.phoneOk {{ steps.r.output.verified }}`; 1: `callFlow captureIdentity ⇒ c` (error `ID_CANCELLED` → `showSnackbar`); 2: `⇢ vf-address ⇒ a` → `setState page.addr` · ⚡onStepCancel `setState page.step {{ max(0, page.step - 1) }}`
  - [ ] `FilledButton t:common.continue` enabled `{{ page.phoneOk && page.idOk && page.addr != null }}` ⚡ `→ nb-details`
  - [ ] 🧪 ghost screens of Verify's sheets show on this plugin's canvas; cancelling the ID dialog marks the step `error`
- [ ] **`nb-details`** · Page · **`secure: true`** · requiresAssurance **AL2** · form `details`: `taxId` (required, regex `^[0-9]{11}$`, **sensitive**), `iban` (required, iban), `nationality`, `dob` (dateRange ≥ 18 y), `funds` (required), `pep` bool
  - [ ] `FormScope` details › `ListView` › `SecureTextField` label `t:nb.taxId` · obscure true · maxLength 11 · ⚡onChanged `setState plugin.taxId {{ event }}`; `IbanInput` label `t:nb.iban` · invalidMessage `t:nb.ibanBad` · node `direction: ltr` (stays LTR in Arabic); `CountryPicker`; `DualCalendarDatePicker` calendar gregorian; `DropdownMenu` funds (salary, savings, business, other) · requestFocusOnTap true · `TextFormField` occupation; `SwitchListTile t:nb.pep` · subtitle `t:nb.pepHint`
  - [ ] 🧪 the canvas shows the **sensitive** mask; the device never logs `taxId`; AL1 device (emulator) is refused with the assurance message
- [ ] **`nb-pin`** · Page · `secure: true` · page state `first:string`, `stage:int=0`, `shake:Offset={dx:0,dy:0}`
  - [ ] `Column` › `Text {{ page.stage == 0 ? t("nb.pinChoose") : t("nb.pinRepeat") }}`; `AnimatedSlide` offset `{{ page.shake }}` · duration 80 · curve elasticOut › `SecurePinPad` length 6 · randomizeLayout true · haptics true · ⚡onCompleted: `switch {{ page.stage }}` → 0: `setState page.first {{ event }}` → `setState page.stage 1`; 1: `condition {{ event == page.first }}` → `apiCall accounts.setPin {pin: {{ event }}}` (input sensitive) → `→ nb-deposit` / else `haptic heavy` → `setState page.shake {dx: 0.05, dy: 0}` → `delay 160` → `setState page.shake {dx: 0, dy: 0}` → `setState page.stage 0`
- [ ] **`nb-deposit`** · Page · `secure: true` · requiresAssurance AL2 · page state `amount:money?`, `currency:string="EUR"`
  - [ ] `AmountInput` currency `{{ page.currency }}` · min 10 · max 5000 · rangeMessage `t:nb.depositRange`; `CurrencyPicker` value `{{ page.currency }}` · currencies `[EUR, USD, GBP, CHF]` · searchHint; `MoneyText` value `{{ page.amount }}` · display code · signDisplay never
  - [ ] `FilledButton t:nb.deposit` ⚡ [drop]: `signTransaction transaction "transfers.deposit" amount {{ page.amount }} payee "DE89370400440532013000"` (branch `declined` → `showSnackbar t:nb.declined`) → `apiCall transfers.deposit {amount: {{ amount(page.amount) }}, currency: {{ page.currency }}, to: {iban: "DE89370400440532013000"}, reference: "NV-OPEN"}` → `→ nb-done`
  - [ ] 🧪 the SCA sheet shows amount, currency, payee exactly; biometric cancel → declined; changing the amount after signing → `PLX-6072`; device without screen lock → `PLX-6017`; offline → never queued
- [ ] **`nb-terms`** · Full-screen dialog · result `bool` › `TermsViewer` source `t:legal.bank` · version `2026-11` · acceptLabel · scrollHint · ⚡onAccepted `↩ true`; `ConsentCheckbox` label `t:nb.marketing` · documentVersion `2026-11` · ⚡onAccepted `kvSet marketing true`
- [ ] **`nb-done`** · Page · transition `scale` · onEnter: `apiCall accounts.open {…} ⇒ a` → submit rule (shared) › `LottieView asset:success.lottie` repeat false · reverse false · `Text t:nb.welcome` · `FilledButton t:nb.toPass` ⚡ `→⟲ pass(id: {{ steps.a.output.id }})`

### G10. `campus` — Aster University (4-page application · brand `aster` · GraphQL · Ethiopic)

Plugin: dataSources `programmes` **graphql** `query Programmes($q:String,$cursor:String){programmes(q:$q,cursor:$cursor){items{id name level fee} next more}}` · pagination cursor · cache staleWhileRevalidate · `programme` graphql `programme(id)` · `fee` **function** `fees.quote` input `{org: "campus", tier: {{ plugin.app.level }}, quantity: 1, currency: "EUR"}` · operation `api.register` with **`offlineCapable: true`** · collections (private) `drafts` v1 `id` pk, `step:int`, `data:string`, `updatedAt:dateTime` · plugin state `app:CampusApp` (**persisted**, versioned) · triggers `onAppPause` (handler **detached**): `dbUpsert drafts "current" {id: "current", step: {{ plugin.app.step }}, data: {{ string(plugin.app) }}, updatedAt: {{ now }}}` (check the JSON-to-string function in the PXL editor) · `dataSources.api.onSynced` → `dbUpdate registrations … {status: "submitted"}` → `showToast t:cp.sent`; `onSyncFailed` → `dbUpdate … {status: "draft"}` → `showSnackbar t:cp.failed`; `onConflict` → `⇢ ps-confirm(title: t:cp.conflict) ⇒ ok`

- [ ] **`cp-home`** · Page · onEnter brand `aster` → `dbQuery drafts limit 1 ⇒ d` → `setState page.hasDraft {{ size(steps.d.output) > 0 }}`
  - [ ] `CustomScrollView` › `SliverAppBar` expandedHeight 200 · floating true · snap true · flexibleSpace `◇HeroHeader` image `asset:org-campus-cover.webp` · title `t:cp.title` (serif from the `aster` brand)
  - [ ] `SliverToBoxAdapter` › `If {{ page.hasDraft }}` then `◇BaseCard` variant flat · tappable › content `ListTile t:cp.continue` ⚡ `⇢ ps-draft ⇒ choice` → `switch` resume: `switch {{ plugin.app.step }}` → `→ cp-personal|cp-education|cp-documents|cp-essay`
  - [ ] `SliverToBoxAdapter` › `TextField` decoration outlined · prefixIcon search · hintText `t:cp.search` · textInputAction search · ⚡onChanged [debounce:400] `refreshData programmes` · ⚡onSubmitted same
  - [ ] `SliverList` items `{{ data.programmes.value ?? [] }}` · hasMore · status · ⚡onEndReached `refreshData programmes more:true` · item `ListTile` isThreeLine true · title name · subtitle `{{ t("cp.level." + item.level) }}` · trailing `MoneyText` · ⚡onTap `→ cp-programme(id)`
- [ ] **`cp-programme`** · Page · params `id` · page state `tab:int=0` · appBar `AppBar` leading `BackButton` color @color.primary
  - [ ] `Column` › `TabBar` tabs overview/requirements/fees · selectedIndex `{{ page.tab }}` · ⚡onTap; `Expanded` › `IndexedStack` index `{{ page.tab }}` · sizing expand ›
    - [ ] `MarkdownView` source `{{ data.programme.value.overview }}` · ⚡onLink `openUrl`
    - [ ] `ListView` of `CheckboxListTile` (read-only, `enabled: false`) per requirement
    - [ ] `Table` columns 3 · columnWidths `[{kind: fraction, value: 0.5}, {kind: flex, value: 1}, {kind: fixed, value: 96}]` · defaultVerticalAlignment baseline · textBaseline alphabetic › fee rows with `MoneyText` from `data.fee.value`
  - [ ] `FilledButton t:cp.apply` ⚡ `setState plugin.app {{ {"programmeId": params.id, "level": data.programme.value.level, "step": 1} }}` → `→ cp-personal`
- [ ] **`cp-personal`** (1/4) · Page · form `personal`: `given`, `family` (required, length ≤ 50), `dob` (required, dateRange), `gender` (required), `nationality`, `passport` (regex `^[A-Z0-9]{6,9}$`) · appBar action `TextButton t:cp.saveExit` ⚡ `dbUpsert drafts …` → `⇢ cp-saved ⇒ choice` → `switch` exit: `→⤓ until cp-home`
  - [ ] `◇StepHeader` 1/4 · `◇FieldGroup` title `t:cp.you` › `TextFormField` ×2 (textCapitalization words, autofillHints [givenName] / [familyName]); `DualCalendarDatePicker` calendar **ethiopic** · secondaryCalendar gregorian (in `am-ET`), gregorian + islamicUmalqura (in `ar`); 3× `RadioListTile` gender female/male/unspecified (message uses ICU `select`); `CountryPicker`; `TextFormField` passport · textCapitalization characters · node `direction: ltr`
  - [ ] `◇SubmitBar t:common.next` ⚡ `submitForm personal` → `patchState plugin.app {personal: …, step: 2}` → `→ cp-education`
- [ ] **`cp-education`** (2/4) · Page · page state `schools:list<School>=[]`, `choices:list<string>`
  - [ ] `ForEach` items `{{ page.schools }}` › `Dismissible` direction horizontal · secondaryBackground (other side colour) · ⚡onDismissed `setState page.schools {{ filter(page.schools, s, s.id != item.id) }}` › `◇FieldGroup` title `{{ t("cp.school", {n: index + 1}) }}` › fields `Column` [`TextFormField` school, `DropdownMenu` year (2000–2026) · menuHeight 240, `TextFormField` grade keyboardType decimal]
  - [ ] `OutlinedButton.icon t:cp.addSchool` enabled `{{ size(page.schools) < 5 }}` ⚡ `setState page.schools {{ page.schools + [{"id": string(now)}] }}`
  - [ ] `Text t:cp.rank` · `ReorderableListView` items `{{ page.choices }}` · shrinkWrap true · physics never · ⚡onReorder `setState page.choices {{ event.items }}`
  - [ ] 🧪 a 6th school is blocked; swipe either side removes; empty list shows the hint
- [ ] **`cp-documents`** (3/4) · Page · page state `docs:list<FileUploadResult>=[]`
  - [ ] `FileUploader` dataSource `api` · operation `upload` · accept `[application/pdf, image/jpeg, image/png]` · maxBytes 10485760 · label `t:cp.choose` · uploadLabel `t:cp.upload` · ⚡onUploaded `setState page.docs {{ page.docs + [event] }}` · ⚡onFailed `showSnackbar {{ event.message }}`
  - [ ] `OutlinedButton t:cp.pickMany` ⚡ `pickFile mimeTypes [application/pdf] multiple true ⇒ f` → `forEach {{ steps.f.output }}` › `apiCall api.upload {file: {{ item }}}` · `LinearProgressIndicator` value `{{ page.progress }}` (from the source's `onProgress` trigger → `setState page.progress {{ event.fraction }}`) · borderRadius 4 · minHeight 6
  - [ ] `ListView` items `{{ page.docs }}` · item `ListTile` leading Icon picture_as_pdf · title name · subtitle `{{ format.compact(item.sizeBytes) }}`
  - [ ] 🧪 an 11 MB file is refused with the size message; cancel mid-upload; offline upload fails cleanly (uploads are not queued)
- [ ] **`cp-essay`** (4/4) · Page · page state `text:string`, `stats:EssayStats?`, `preview:CrossFadeState=showFirst`
  - [ ] `SwitchListTile t:cp.preview` ⚡ `switch` → `setState page.preview "showSecond"|"showFirst"`
  - [ ] `AnimatedCrossFade` crossFadeState `{{ page.preview }}` › first: `SizedBox` height 320 › `TextField` expands true · maxLines null · minLines null · maxLength 30000 · keyboardType multiline · textInputAction newline · textAlignVertical top · ⚡onChanged [debounce:800] `setState page.text {{ event }}` → `invokeFunction essay.stats {text: {{ event }}} ⇒ s` → `setState page.stats` → `kvSet essayDraft {{ event }}`; second: `MarkdownView` source `{{ page.text }}`
  - [ ] `Text {{ t("cp.words", {n: page.stats.words}) }}` (ICU `plural` with `=0`, `one`, `other`, and `offset`) · `Text {{ t("cp.choiceOrdinal", {n: 3}) }}` (ICU `selectordinal`)
  - [ ] `◇SubmitBar t:common.next` enabled `{{ page.stats.words >= 250 }}` ⚡ `→ cp-review`
- [ ] **`cp-review`** · Page · data `fee`
  - [ ] `Stepper` type vertical · currentStep 0 · steps 4 with states (`complete` when filled, `error` when missing) · elevation 0 · ⚡onStepTapped `switch {{ event }}` → `→ cp-personal` … `cp-essay`
  - [ ] `◇MoneyRow` label `t:cp.subtotal` amount `{{ data.fee.value.subtotal }}` · `◇MoneyRow` label `t:cp.tax` amount `{{ data.fee.value.tax }}` · `◇MoneyRow` label `t:cp.fee` amount `{{ data.fee.value.total }}` emphasis true · template `consent-block`
  - [ ] `◇SubmitBar t:cp.payAndSubmit` ⚡submit: `callFlow payWithWallet {amount: {{ data.fee.value.total }}, payee: "Aster University", reference: {{ "AU-" + plugin.app.programmeId }}, purpose: t:cp.feePurpose} ⇒ p` → `switch {{ steps.p.output.status }}` → success: `apiCall api.register {…}` (queued offline) → `dbDelete drafts current` → submit rule; error: `showSnackbar {{ t("pay.err." + steps.p.output.errorCode) }}`; cancelled: nothing
  - [ ] 🧪 pay online, then go offline before submit: the registration is queued (outbox), shows `submitted` after reconnect; a `409` replay triggers `onConflict`
- [ ] **`cp-saved`** · Dialog · result `string` (`exit`|`stay`) › `Icon save` · `Text t:cp.savedBody` · `TextButton` ↩ "stay" · `FilledButton` ↩ "exit"

### G11. `cityrun` — Riverton City Run (map-first event · brand `riverton` · WebSocket)

Plugin: capabilities `deviceApis [location, share]`, `functions [run.finishTime, fees.quote]` · dataSources `race` **static** (inline JSON: `distances [5,10,21.1,42.2]`, `route` 60 LatLng points, `aid` 8 markers, `start/finish`, `startsAt`) · `runner` **websocket** `/ws/races/{raceId}/runners/{bib}` · `fee` function `fees.quote` · plugin state `bib:string` (persisted), `distance:double=10`, `wave:int=0` · **kill-switch target** (§J3) with fallback `ln-offline`

- [ ] **`cr-race`** · Page · onEnter brand `riverton` · timeline `intro` (scope page · autoplay · durationMs 1200 · staggerMs 80 over the distance chips · spring stiffness 180 damping 18 · reduceMotion shorten): tracks title offset (0,40)→(0,0), medal rotation 0→1 turn, chips opacity 0→1
  - [ ] `Timeline` timeline intro · autoplay true · repeat false › `Stack` › `Image asset:org-cityrun-cover.webp` fit cover · color @color.scrim · colorBlendMode multiply; `SlideTransition` timeline intro track 0 axis vertical › `Text t:cr.title` displayMedium; `RotationTransition` timeline intro track 1 › `Icon emoji_events` 64
  - [ ] `CountdownTimer` endsAt `{{ data.race.value.startsAt }}` · format dhms · ⚡onFinished `→ cr-live`
  - [ ] `Carousel` indicators true › `ForEach` distances › `◇BaseCard` tappable · selected `{{ plugin.distance == item }}` · ⚡tap `setState plugin.distance {{ item }}` · `Stack` progress bar › `AnimatedPositioned` start `{{ plugin.distance / 42.2 * 280 }}` duration 400 curve overshoot › `Icon directions_run`
  - [ ] `Row` › `FilledButton t:cr.register` ⚡ `→ cr-register`; `OutlinedButton t:cr.route` ⚡ `→ cr-route`
- [ ] **`cr-route`** · Page
  - [ ] `Column` › `Expanded flex 3` › `MapView` center start · zoom 13 · polylines `[{points: {{ data.race.value.route }}, color: "#EA580C", width: 5}]` · markers `{{ data.race.value.aid }}` · ⚡onMarkerTap `showSnackbar {{ first(filter(data.race.value.aid, a, a.id == event)).title }}`
  - [ ] `Expanded flex 2` › `LineChart` series `[{name: t:cr.elevation, values: {{ data.race.value.elevation }}}]` (add `elevation: list<double>` to the static source) · labels km · title `t:cr.elevation` · showTable true
- [ ] **`cr-register`** · Page · form `runner`: `distance` (required), `shirt` (required), `pace` int (range 150–600), `emergency` phone (required), `medical` (**sensitive**), `waiver` bool (custom rule `{{ value == true }}`) · page state `finish:string`
  - [ ] `SingleChildScrollView` padding 16 · keyboardDismissBehavior onDrag › `Column` spacing 16 ›
    - [ ] `Wrap` › `ForEach` items `{{ data.race.value.distances }}` › `ChoiceChip` label `{{ format.number(item) }} km` · selected `{{ form.values.distance == item }}` · ⚡onSelected `setState page.runner.values.distance {{ item }}`
    - [ ] `DropdownMenu` label `t:cr.shirt` · entries XS–XXL · enableFilter false · selectOnly true · closeBehavior self
    - [ ] `Slider` min 150 max 600 divisions 45 · showValueIndicator always · ⚡onChangeEnd: `invokeFunction run.finishTime {distance_km, pace_sec_per_km} ⇒ f` → `setState page.finish {{ steps.f.output.label }}` (business `PACE_UNREALISTIC` → `showToast t:cr.paceBad`) · `Text {{ t("cr.finish", {time: page.finish}) }}`
    - [ ] `ListTile t:cr.wave` trailing `Text {{ plugin.wave + 1 }}` ⚡ `⇢ cr-wave(current: {{ plugin.wave }}) ⇒ int` → `setState plugin.wave`
    - [ ] `PhoneInput` label `t:cr.emergency`; `TextFormField` medical · maxLines 3 · enableIMEPersonalizedLearning false · `CheckboxListTile t:cr.waiver` · isError `{{ page.runner.touched.waiver && !form.values.waiver }}`
  - [ ] `◇SubmitBar` ⚡submit: `submitForm runner` → `callFlow payWithWallet {amount: {{ data.fee.value.total }}, payee: "Riverton City Run", reference: …, purpose: t:cr.entry} ⇒ p` → `condition {{ steps.p.output.status == "success" }}` → `apiCall api.register` → `setState plugin.bib {{ steps.reg.output.passCode }}` → `→⟲ cr-bib`
- [ ] **`cr-wave`** · Bottom sheet · params `current:int` · result `int`
  - [ ] `SizedBox` height 220 › `CupertinoPicker` itemExtent 40 · selectedIndex `{{ params.current }}` · diameterRatio 1.1 · useMagnifier true · magnification 1.2 · squeeze 1.25 · looping false · offAxisFraction 0 · ⚡onSelectedItemChanged `setState page.sel {{ event }}` › 6 `Text`s; `FilledButton t:common.done` ⚡ `↩ {{ page.sel }}`
- [ ] **`cr-bib`** · Page · transition `sharedAxis`
  - [ ] `Hero` tag `bib` › `◇BaseCard` variant raised › content `Column` [`FittedBox` fit scaleDown › `Text {{ plugin.bib }}` displayLarge, `QrCodeView` data bib · errorCorrection high · foreground @color.primary · background @color.surface]; `OutlinedButton.icon share` ⚡ `share text …`; `FilledButton t:cr.live` ⚡ `→ cr-live`
- [ ] **`cr-live`** · Page · onEnter `subscribe runner {raceId: "rcr-2027", bib: {{ plugin.bib }}}`; onLeave `unsubscribe runner` · page state `follow:bool=true`
  - [ ] `Scaffold` floatingActionButtonLocation **centerDocked** · extendBody true
    - [ ] body `GestureDetector` behavior translucent · ⚡onDoubleTap `setState page.follow {{ !page.follow }}` › `OrderTrackerMap` semanticLabel `t:cr.liveMap` · origin start · destination finish · courier `{{ {"lat": data.runner.value.lat, "lng": data.runner.value.lng} }}` · route `{{ data.race.value.route }}` · follow `{{ page.follow }}` · etaLabel `{{ t("cr.eta", {m: data.runner.value.etaMinutes}) }}` · originLabel/destinationLabel/courierLabel
    - [ ] floatingActionButton `FloatingActionButton` shape circle › Icon share ⚡ `share url …`
    - [ ] bottomNavigationBar `BottomAppBar` notchMargin 6 · height 72 › `Row` › `LiveEtaCard` etaMinutes `{{ data.runner.value.etaMinutes }}` · status `{{ data.runner.value.status }}`; `Text {{ format.number(data.runner.value.km) }} km`
  - [ ] 🧪 runner messages every 3 s move the marker; drop Wi-Fi → reconnect/backoff; kill switch on `cityrun` → fallback page on every cityrun route

### G12. `devconf` — DevHarbor Conf (e-commerce · brand `devharbor`, dark by default)

Plugin: capabilities `functions [workshops.capacity, promo.check, fees.quote]` · dataSources `agenda` graphql `agenda(day)` · `speakers` graphql · `workshops` graphql · `products` rest `GET /orgs/devconf/products` cache cacheFirst TTL 3600 · `cartLines` database on `cart` · `quote` function `fees.quote` · collections (private) `cart` v1 `sku` pk, `title`, `unit:money`, `qty:int`, `kind` · plugin state `promo:string=""` (session), `day:int=0` · onEnter of `dc-home`: brand `devharbor` → `setThemeMode dark`, onLeave → `setThemeMode {{ app.themePref }}`

- [ ] **`dc-home`** · Page
  - [ ] `Scaffold` › drawer `Drawer` width 300 › `ListView` › `ListTile` style drawer × (Agenda, Tickets, Workshops, Merch, Cart) · selected · ⚡onTap `→ …`
  - [ ] appBar `AppBar` actions `IconButton` cart › `Badge` label `{{ size(data.cartLines.value) }}`
  - [ ] body `ListView` › `PromoBanner` title `t:dc.early` · ctaLabel `t:dc.buy` · endsAt `{{ dateTime("2027-03-01T00:00:00+01:00") }}` · ⚡onTap `→ dc-tickets`; `Text t:dc.speakers`; `Carousel` › `ForEach` speakers › `Column` [`CircleAvatar` radius 40 foregroundImage, `Text` name]; `GridView` crossAxisCount 2 shrinkWrap › 4 shortcut `Card`s with `InkWell`
  - [ ] floatingActionButton `FloatingActionButton` isExtended true · icon confirmation_number · label `t:dc.getTickets` · extendedTextStyle labelLarge ⚡ `→ dc-tickets`
- [ ] **`dc-agenda`** · Page
  - [ ] `CustomScrollView` › `SliverAppBar` pinned · bottom `TabBar` 3 days · ⚡onTap `setState plugin.day` → `refreshData agenda`; `SliverPersistentHeader` pinned minExtent 56 maxExtent 56 › `ListView` horizontal of track `FilterChip`s; `SliverList` items `{{ data.agenda.value }}` · item `ExpansionTile` title session · subtitle time · leading `IconButton` isSelected `{{ item.id in page.saved }}` · icon bookmark_border · selectedIcon bookmark · ⚡onPressed `kvSet saved …` › children `◇InfoRow` label `t:dc.speaker` value speaker name + `Text` abstract + `ActionChip` avatar Icon event · label `t:dc.addCal` · ⚡onPressed `callNative addToCalendar {…}`
- [ ] **`dc-tickets`** · Page · form `promo`: `code` (async validator flow `checkPromo` = `invokeFunction promo.check {code: {{ value }}, qty: …}`, debounceMs 500)
  - [ ] `ListView` › 3× `AnimatedContainer` duration 200 · decoration border `{{ page.sel == item.sku ? 2 : 0 }}` @color.primary · borderRadius @radius.lg › `ProductCard` image · title Student/Standard/VIP · price `{{ format.money(item.price) }}` · originalPrice (VIP only) · badge `t:dc.popular` (Standard) · addToCartLabel · ⚡onAddToCart `dbUpsert cart {sku, title, unit, qty: {{ page.qty }}, kind: "ticket"}` → `showSnackbar t:dc.added actionLabel t:dc.viewCart` · ⚡onTap `setState page.sel`
  - [ ] `Row` › `PriceTag` price · originalPrice · wasLabel `t:dc.was`; `QuantityStepper` value `{{ page.qty }}` min 1 max 10 step 1 · ⚡onChanged `setState page.qty`
  - [ ] `TextFormField` promo code · textCapitalization characters · suffixIcon (validating spinner via `{{ page.promo.validating }}`)
  - [ ] 🧪 `EARLY20` ok; `TEAM5` with qty 2 → `PROMO_CONDITION` text; expired code; typing fast calls once (debounce)
- [ ] **`dc-workshops`** · Page · lifecycle onEnter `invokeFunction workshops.capacity {ids} ⇒ c` → `setState page.left`
  - [ ] `ListView` › per workshop `CheckboxListTile` value `{{ item.id in page.picked }}` · enabled `{{ item.id in page.picked || size(page.picked) < flags.maxWorkshops }}` · secondary `Badge` label `{{ page.left[index] }}` · subtitle · ⚡onChanged `condition {{ event }}` → `setState page.picked {{ page.picked + [item.id] }}` / else `setState page.picked {{ filter(page.picked, p, p != item.id) }}`; `Text {{ t("dc.max", {n: flags.maxWorkshops}) }}`
  - [ ] `FilledButton` ⚡ `invokeFunction workshops.capacity` (business `FULL` → `showSnackbar {{ t("dc.full", {id: steps.c.error.params.id}) }}`) → `forEach page.picked` › `dbUpsert cart …`
  - [ ] 🧪 change flag `maxWorkshops` to 3 in Experiments › Feature flags: a third box enables without a release
- [ ] **`dc-merch`** · Page · page state `open:string?`
  - [ ] `SortSheet` title `t:dc.sort` · options price-asc/price-desc/new · selected `{{ page.sort }}` · ⚡onSelected `setState page.sort`
  - [ ] `GridView` items `{{ sortBy(data.products.value, p, p.price) }}` · crossAxisCount 2 · `M:` 3 · `E:` 4 · mainAxisExtent 280 · item `ProductCard` · ⚡onTap `setState page.open {{ item.sku }}`
  - [ ] `AnimatedSize` duration 300 › `If {{ page.open != null }}` then `ImageGallery` images `{{ first(filter(data.products.value, p, p.sku == page.open)).images }}` · initialPage 0 · indicators true · ⚡onPageChanged
- [ ] **`dc-cart`** · Page · data `cartLines`, `quote` (input quantity = total qty)
  - [ ] `ListView` items `{{ data.cartLines.value }}` · empty `◇EmptyHint t:dc.emptyCart` · item `Dismissible` ⚡onDismissed `dbDelete cart {{ item.sku }}` › `ListTile` title · trailing `QuantityStepper` ⚡onChanged `condition {{ event == 0 }}` → `dbDelete` / else `dbUpdate cart … {qty: {{ event }}}`
  - [ ] `CartSummary` lines `{{ map(data.cartLines.value, l, {"label": l.title + " ×" + string(l.qty), "amount": mul(l.unit, decimal(l.qty))}) }}` · subtotal `{{ data.quote.value.subtotal }}` · tax `{{ data.quote.value.tax }}` · total `{{ data.quote.value.total }}` · checkoutLabel `t:dc.checkout` · ⚡onCheckout `→ dc-checkout`; total wrapped in `AnimatedSwitcher`
  - [ ] 🧪 money stays exact (0.10 × 3); quote loading state on canvas; 0 quantity removes the line
- [ ] **`dc-checkout`** · Page · page state `method:string="wallet"`
  - [ ] insert template `contact-block` (requirePhone true); 2× `RadioListTile` wallet/invoice · groupValue `{{ page.method }}`
  - [ ] `◇SubmitBar` ⚡submit: `switch {{ page.method }}` → wallet: `callFlow payWithWallet {…} ⇒ p` → `condition success`; invoice: `apiCall api.invoice` → then `apiCall api.register` → `forEach {{ data.cartLines.value }}` › `dbDelete cart {{ item.sku }}` → `→⟲ dc-badge`
- [ ] **`dc-badge`** · Page › `QrCodeView` data `{{ page.reg.passCode }}` (onInit: `dbQuery registrations where {{ orgSlug == "devconf" }} orderBy submittedAt descending limit 1` → `setState page.reg`) · size 240; `OutlinedButton.icon t:dc.scanTicket` ⚡ `scanCode formats [code128, ean13, upcA] ⇒ s` → `apiCall api.claim {code: {{ steps.s.output.value }}}` → `showToast t:dc.claimed`; `TextButton t:common.share` ⚡ `share`

### G13. `clinic` — Willow Clinic (all Cupertino · brand `willow` · sensitive · SSE)

Preview this plugin with **Cupertino** platform style; every control below is Cupertino or `adaptive: true`.

Plugin: capabilities `functions [slots.availability, slots.book]` · dataSources `queue` **sse** `/sse/clinics/{id}/queue?ticket` · `slots` function `slots.availability` · plugin state `patient:Patient` (**secure**, **sensitive**), `ticket:string` (session) · security: pages `cl-patient`, `cl-symptoms` are `secure: true`

- [ ] **`cl-welcome`** · Page · transition `platform` (iOS slide)
  - [ ] `CupertinoNavigationBar` middle `Text t:cl.title` · previousPageTitle `t:common.back` · trailing `CupertinoButton` sizeStyle small › Icon info (cupertino set) · enableBackgroundFilterBlur true · automaticBackgroundVisibility true · transitionBetweenRoutes true
  - [ ] `CupertinoListSection` header `Text t:cl.services` · footer `Text t:cl.footer` · hasLeading true · separatorColor @color.outlineVariant › 4× `CupertinoListTile` leading Icon · title · subtitle · additionalInfo `Text t:cl.today` · trailing Icon chevron_forward (cupertino) · ⚡onTap `→ cl-patient`
  - [ ] `Row` › `ClipOval` › `Image asset:avatar-placeholder.png` width 56 height 56 fit cover; `Text t:cl.doctor`
  - [ ] `VideoPlayer` url `…/clinic-intro.mp4` · loop false · autoplay false · semanticLabel
  - [ ] `CupertinoButton` sizeStyle large · color @color.primary · borderRadius 12 · minimumSize (∞, 50) › `Text t:cl.register` ⚡ `→ cl-patient`
- [ ] **`cl-patient`** · Page · `secure: true` · form `patient`: `name` required, `dob` required, `insurer` required, `insuranceNo` (required, regex, **sensitive**), `insured` bool, `sex`, `weight` double (range 2–250)
  - [ ] `CupertinoTextField` ×3 · placeholder · prefix Icon · prefixMode always · clearButtonMode editing · suffixMode notEditing · keyboardType name/datetime/number · textInputAction next · padding 12 · decoration BoxDecoration border 1 @color.outline borderRadius 10
  - [ ] `CupertinoSwitch` value insured · activeTrackColor @color.primary · ⚡onChanged; `CupertinoSlidingSegmentedControl` values female/male/other · groupValue · thumbColor · proportionalWidth true; `CupertinoSlider` weight min 2 max 250 divisions 248 · ⚡onChangeEnd
  - [ ] `CupertinoButton` filled › `Text t:cl.chooseSlot` ⚡ `submitForm patient` → `setState plugin.patient …` → `⇢ cl-slot ⇒ Slot` → `setState page.slot` → `→ cl-symptoms`
- [ ] **`cl-slot`** · Bottom sheet · result `Slot` · page state `date:date`
  - [ ] `SizedBox` height 200 › `CupertinoDatePicker` mode date · minimumDate now · maximumDate +30 days · use24hFormat true · showDayOfWeek true · ⚡onDateTimeChanged [debounce:300] `setState page.date {{ event }}` → `refreshData slots`
  - [ ] `DataScope` source slots · loading `CupertinoActivityIndicator` radius 14 · error `Text {{ data.slots.error.code == "CLOSED" ? t("cl.closed") : t("error.generic") }}` · success `Wrap` › `CupertinoButton` per slot · color: `@color.primary` when `item.free`, none otherwise (the button is then disabled: no action) · disabledColor @color.outlineVariant · ⚡onPressed `↩ {{ item }}`
- [ ] **`cl-symptoms`** · Page · `secure: true`
  - [ ] 5× `Radio` adaptive true · useCupertinoCheckmarkStyle true (+ `Text`) · groupValue; 6× `Checkbox` adaptive true; `Switch` adaptive true · applyCupertinoTheme true
  - [ ] `Text t:cl.pain` · `RatingInput` max 10 · starLabel `t:cl.point`
  - [ ] `AudioPlayer` url `…/prep-instructions.mp3` · title `t:cl.prep` · playLabel/pauseLabel/positionLabel · ⚡onEnded `showToast t:cl.listened`
  - [ ] `CupertinoButton` ⚡ `⇢ cl-confirm ⇒ bool` → `condition` → submit rule
- [ ] **`cl-confirm`** · Dialog · result `bool`
  - [ ] `CupertinoContextMenu` actions `[{label: t:cl.addCal, icon: calendar_today}, {label: t:common.share, icon: share}, {label: t:cl.cancel, destructive: true}]` · enableHapticFeedback true · ⚡onAction `switch` (calendar → `callNative addToCalendar`, share → `share`, cancel → `↩ false`) › template `summary-card` (date, time, doctor, co-pay)
  - [ ] `CupertinoButton` filled ⚡ `invokeFunction slots.book {…}` (business `TAKEN` → `showSnackbar t:cl.taken` → `↩ false`) → `callFlow payWithWallet {amount: co-pay 10.00 EUR, …} ⇒ p` → `↩ {{ steps.p.output.status == "success" }}`
- [ ] **`cl-queue`** · Page · onEnter `subscribe queue {id: "willow-1", ticket: {{ plugin.ticket }}}` · page state `tab:int=0`
  - [ ] `CupertinoNavigationBar` · body `Match` value `{{ string(page.tab) }}` cases `[0, 1, 2]`: queue `Column` [`CupertinoActivityIndicator` animating `{{ data.queue.value.status == "waiting" }}`, `AnimatedSwitcher` › `Text {{ data.queue.value.position }}` displayLarge, `Text {{ t("cl.wait", {m: data.queue.value.waitMinutes}) }}`]; directions `MapView` (static clinic marker); help `CupertinoListSection`
  - [ ] bottom `CupertinoTabBar` items queue/directions/help · currentIndex `{{ page.tab }}` · activeColor @color.primary · ⚡onTap `setState page.tab {{ event }}`
  - [ ] 🧪 position counts down live; app in background then resume → stream resubscribes; `insuranceNo` is sent to `ar-api` but never appears in traces or logs

### G14. `relief` — Hands Together (NGO · brand `hands` · **Arabic first, RTL**)

Design and check this plugin in `ar` first, then `en`. Plugin: dataSources `campaign` rest `GET /orgs/relief/campaign` (`{goal, raised, monthly:[…], allocation:[…], donors:[…]}`) · `donations` rest `GET /me/donations` auth · **`reliefTransfer`** rest, transaction `{amount: /amount, currency: /currency, payee: /to/iban}`, operation `donate` POST `/transfers` (payee `payeeIbanRelief` is a **server-bound displayed** value, `DAT-093`)

- [ ] **`rl-campaign`** · Page · onEnter brand `hands`
  - [ ] `CustomScrollView` › `SliverAppBar` title `t:rl.title` (Arabic headline font) · actions `IconButton` share
  - [ ] `SliverToBoxAdapter` › `RiveView asset:goal-meter.riv` stateMachine `Meter` · inputs `{"progress": {{ double(data.campaign.value.raised) / double(data.campaign.value.goal) * 100 }}}` · triggers `{"celebrate": {{ data.campaign.value.raised >= data.campaign.value.goal ? 1 : 0 }}}` · fit contain · reduceMotion shorten · semanticLabel `t:rl.meter`
  - [ ] `SliverToBoxAdapter` › `◇MoneyRow` label `t:rl.raised` amount raised emphasis true · `◇MoneyRow` label `t:rl.goal` amount goal (Arabic number format)
  - [ ] `LineChart` monthly · `PieChart` slices allocation · showTable true · `Sparkline` donors
  - [ ] `ListTile` trailing `Transform` flipX true › `Icon arrow_back` (an icon that must *not* auto-mirror is flipped by hand; compare with an auto-mirrored `chevron_right`) ⚡ `→ rl-volunteer`
  - [ ] `FilledButton t:rl.donate` ⚡ `→ rl-donate`
  - [ ] 🧪 RTL: padding start/end swap, `Positioned start` follows, the `IbanInput` stays LTR; numbers on the canvas use Latin digits (device decides); charts read right to left in their tables
- [ ] **`rl-volunteer`** · Page · form `vol`: `skills` list (required, length ≥ 1), `from` date (dateRange today…+1 y), `hours` range
  - [ ] `Wrap` spacing 6 › `ForEach` page.skills › `InputChip` label · onDeleted `setState page.skills {{ filter(page.skills, k, k != item) }}` · deleteIcon Icon close · selected · ⚡onSelected; `TextField` hintText `t:rl.addSkill` · textInputAction done · ⚡onSubmitted `setState page.skills {{ page.skills + [trim(event)] }}`
  - [ ] `Table` columns 8 (day names + 7 checkbox columns) · border · defaultColumnWidth intrinsic › 3 rows (morning/afternoon/evening) of `Checkbox`
  - [ ] `RangeSlider` values hours 2–20 · divisions 18 · labels `{{ {"start": string(page.h.start), "end": string(page.h.end)} }}`
  - [ ] `DualCalendarDatePicker` calendar islamicUmalqura · secondaryCalendar gregorian
  - [ ] `◇SubmitBar` ⚡ `submitForm vol` → `apiCall api.volunteer` → `showSnackbar t:rl.thanksVol`
- [ ] **`rl-donate`** · Page · page state `preset:list<string>=["25"]`, `custom:money?`, `currency:string="EUR"`, `monthly:bool=false`
  - [ ] `SegmentedButton` segments 10/25/50/other · selected `{{ page.preset }}` · multiSelectionEnabled false · emptySelectionAllowed false · direction horizontal · `M:` direction vertical
  - [ ] `If {{ first(page.preset) == "other" }}` then `AmountInput` currency `{{ page.currency }}` min 1 max 10000 · `CurrencyPicker` currencies `[EUR, USD, SAR, AED, ETB]`
  - [ ] `PaymentCard` brand visa · maskedNumber "•••• 4242" · expiry "08/29" · holder `{{ data.me.value.name }}` · flipped `{{ page.flip }}` · ⚡onFlip (display of the wallet's saved card only; card data never enters Plux)
  - [ ] `SwitchListTile t:rl.monthly`
  - [ ] `FilledButton t:rl.payWallet` ⚡ `callFlow payWithWallet {amount: {{ first(page.preset) == "other" ? page.custom : money(first(page.preset), page.currency) }}, payee: "Hands Together", reference: "HT-…", purpose: t:rl.donation} ⇒ p` → `condition success` → `→ rl-thanks`
  - [ ] `OutlinedButton t:rl.payBank` ⚡ `signTransaction transaction "reliefTransfer.donate" amount … payee "Hands Together e.V."` → `apiCall reliefTransfer.donate {…}` → `→ rl-thanks` (🧪 the SCA sheet shows the server-held payee; changing it on the server between challenge and send → `PLX-6066` and a transparent re-challenge)
- [ ] **`rl-history`** · Page › `RefreshIndicator` › `ListView` items `{{ data.donations.value }}` · item `TransactionListItem` title · subtitle · amount · status pending/completed/failed/reversed (seed one of each) · statusLabel · date · ⚡onTap `→ ps-receipt(id)` · `MoneyText` total · signDisplay always
- [ ] **`rl-thanks`** · Page › `LottieView asset:confetti.lottie` · `Text t:rl.thanks` · `TextButton t:common.share` ⚡ `share` · `FilledButton t:rl.history` ⚡ `→⟲ rl-history`

### G15. `homes` — Brickyard Homes (rental application · brand `brickyard` · **tablet first**)

Design on the iPad and foldable frames first, then compact. Plugin: dataSources `listings` rest `GET /orgs/homes/listings` · plugin state `saved:list<string>` persisted, `prefs:list<string>=["light","quiet","transport"]`

- [ ] **`hm-listings`** · Page · onEnter brand `brickyard` · page state `nav:int=0`
  - [ ] `Scaffold` › bottomNavigationBar `NavigationBar` destinations Listings/Saved/Applications · selectedIndex `{{ page.nav }}` · labelBehavior onlyShowSelected · height 72 · indicatorColor @color.secondaryContainer · ⚡onDestinationSelected `setState page.nav` → then **Make navigation adaptive** (selection toolbar): rail from 600 dp, extended from 840 dp
  - [ ] body `Responsive` › compact: `ListView` of listing cards; medium: `GridView` crossAxisCount 2; expanded: `Row` [`Expanded flex 3` › `GridView` crossAxisCount 3, `VerticalDivider` width 1 thickness 1, `Expanded flex 2` › `MapView` markers listings]
  - [ ] Listing card: `◇BaseCard` variant raised · tappable · media `Stack` [`Image` fit cover · repeat noRepeat, `Positioned` top 8 end 8 › `IconButton` favourite isSelected `{{ item.id in plugin.saved }}`] · content `ListTile` title price `MoneyText` · subtitle address · `◇StatusChip` status `{{ item.available ? "live" : "done" }}` · ⚡tap `→ hm-listing(id)`
  - [ ] 🧪 Size classes view tags each frame; the foldable crease does not split a card image (marked if it does)
- [ ] **`hm-listing`** · Page · params `id`
  - [ ] `CustomScrollView` › `SliverAppBar` expandedHeight 280 · flexibleSpace `Hero` tag `home-{{ params.id }}` › `ImageGallery` images · indicators · semanticLabel
  - [ ] `SliverToBoxAdapter` › `Row` [`RatingBar` value 4.3 max 5 · ofLabel `t:common.of`, `Tooltip` message `t:hm.verified` · preferBelow false · waitDuration 300 › `Icon verified`]; `Wrap` of `Chip` amenities (avatar icon)
  - [ ] `SliverToBoxAdapter` › `AspectRatio` 4/3 › `ClipRRect` borderRadius @radius.lg › `InteractiveViewer` minScale 0.5 · maxScale 4 · boundaryMargin 40 · panAxis free · constrained true › `Image asset:floorplan-a.png` fit contain · ⚡onInteractionEnd `trackEvent floorplan_zoom`
  - [ ] floatingActionButton `FloatingActionButton.extended t:hm.apply` ⚡ `→ hm-apply(id)`; `persistentFooterButtons` `OutlinedButton t:hm.viewing` ⚡ `⇢ hm-viewing ⇒ dateTime` → `apiCall api.viewing`
- [ ] **`hm-apply`** · Page · params `id` · form `rent`: `income` money (required), `budget` range, `iban` (iban), `movein` date
  - [ ] `AmountInput` income; `RangeSlider` budget 500–3000 divisions 25 · labels; `IbanInput` (deposit return) · `CalendarDatePicker` initialDate · firstDate today · lastDate +180 d · ⚡onDateChanged · ⚡onDisplayedMonthChanged
  - [ ] Preference ranking (drag and drop): `Row` of 3 `DragTarget` accepts `string` · highlightColor @color.primaryContainer · ⚡onAccept `setState page.ranked {{ filter(page.ranked, r, r != event) + [event] }}` · ⚡onLeave › `Container` slot "1st/2nd/3rd" (raw `Container` decoration on purpose: dashed-look border + highlight); below `Wrap` of `Draggable` data `{{ item }}` · axis null · feedback `Material`-like `Card` elevated · childWhenDragging `Opacity 0.3` · maxSimultaneousDrags 1 · ⚡onDragStarted `haptic selection` · ⚡onDragEnd · ⚡onDragCompleted · ⚡onDraggableCanceled `showToast t:hm.dropHere`
  - [ ] `◇SubmitBar t:hm.payDeposit` ⚡ `submitForm rent` → `callFlow payWithWallet {amount: 1 month rent …}` → `apiCall api.register` → `→ hm-lease(id)`
  - [ ] 🧪 drag with the keyboard / screen reader: give each Draggable a semantics hint and a fallback `PopupMenuButton` "Move to 1st/2nd/3rd"
- [ ] **`hm-viewing`** · Dialog · result `dateTime` › `CalendarDatePicker` + `Wrap` of `ChoiceChip` times · `FilledButton` ↩ `{{ dateTime(…) }}`
- [ ] **`hm-lease`** · Page · params `id` › `TermsViewer` source `t:legal.lease` · version `L-2027-1` · ⚡onAccepted `⇢ vf-signature ⇒ VerifyResult` → `condition {{ steps.s.output.verified }}` → `apiCall api.sign {svg}` → submit rule

### G16. `arena` — Pulse Arena (one page · brand `pulse`)

Everything on **one page**: the "single page data" case, with every form validator.

- [ ] **`ar-join`** · Page · onEnter brand `pulse` · form `join` (validators run in order, stop at the first failure):
  - [ ] fields: `name` **required** + **length** 2–60 · `email` **email** · `phone` **phone** (region DE default) · `age` int **range** 6–99 · `memberCode` **regex** `^PA-[0-9]{4}$` + **async** (flow `codeFree` = `apiCall api.codeFree {code: {{ value }}}` → `stop {{ steps.f.output.free }}`, debounceMs 500) · `iban` **iban** (direct debit) · `start` date **dateRange** today…+60 d · `extra` decimal **decimalPrecision** maxIntegerDigits 4 maxScale 2 · `guardian` **custom** `{{ form.values.age >= 16 || value != "" }}` · `tier` required
  - [ ] page state `members:list<Member>=[]`
  - [ ] `Scaffold` › appBar `AppBar` shape (bottom radius 24) · backgroundColor @color.primary · toolbarHeight 72 · titleTextStyle headlineSmall
  - [ ] body `FormScope join` › `SingleChildScrollView` › `Column` spacing 16 ›
    - [ ] `SegmentedButton` tier basic/plus/family · selected `{{ [form.values.tier] }}`
    - [ ] `Match` value `{{ form.values.tier }}` cases `[basic, plus, family]` → basic: `Text t:ar.basic`; plus: `◇FieldGroup` with `extra` (`TextFormField keyboardType decimal`); family: `Column` › `ForEach` items `{{ page.members }}` › `Dismissible` › `Row` [`TextFormField` name, `TextFormField` age keyboardType number] + `TextButton.icon t:ar.addMember` ⚡ `setState page.members {{ page.members + [{"id": string(now)}] }}`
    - [ ] `TextFormField` name / email / `PhoneInput` / age (`keyboardType number`) / memberCode (`textCapitalization characters`, helperText `PA-1234`) / `IbanInput` / `DualCalendarDatePicker` start (gregorian only)
    - [ ] `If {{ form.values.age < 16 }}` then `TextFormField` guardian (labelText `t:ar.guardian`, autovalidateMode always)
    - [ ] `ConsentCheckbox` documentVersion `{{ env.termsVersion }}`
  - [ ] persistentFooterButtons `FilledButton t:ar.join` enabled `{{ form.valid && !form.validating }}` ⚡ [drop]: `submitForm join` → `callFlow payWithWallet {amount: {{ form.values.tier == "family" ? money("89.00", "EUR") : form.values.tier == "plus" ? money("49.00", "EUR") : money("29.00", "EUR") }}, payee: "Pulse Arena", reference: "PA-" + form.values.memberCode, purpose: t:ar.membership}` → `apiCall api.register` → `⇢ ar-welcome(code: …)` → `→⟲ pass(id)`
  - [ ] 🧪 run **every** validator to failure once (Preview **Errors** shows them on the initial values); double-tap Join submits once (`drop`); an `age` of 15 reveals the guardian field
- [ ] **`ar-welcome`** · Dialog · params `code` › `QrCodeView` data code · errorCorrection low · `FilledButton t:common.done` ↩

### G17. `maintainers` — Maintainers (QA frame · published to production · listed in Account when `flags.showMaintainers`)

The coverage net. Every screen is a real, polished page titled for maintainers ("Plux showcase"). Two patterns:

- **Gallery** — one small labelled sample per option value, in a `Wrap` (spacing 8) of `Card`s, each sample with a `Text` caption naming the value. Every value of every enum listed for the screen in **Appendix M** gets its own sample with a *literal* value.
- **Variant picker** — for page-level options that cannot repeat on one page (a `Scaffold`'s FAB location, `extendBody`, physics of the page's own scroll view): page state of the enum type (e.g. `fab:FloatingActionButtonLocation=endFloat`), one `ChoiceChip` per value that `setState`s the literal, and the prop bound to `{{ page.fab }}`. 🧪 If the compiler refuses an enum binding, fall back to a **Gallery** of small pages reached through `mt-pages`.

Plugin: capabilities = all 10 device APIs, every function of §E, native routes `sign-in`, `licenses` · guard none (public) · dataSources `cacheNet` rest `GET /orgs?limit=3` **networkOnly**, `cacheFirst` (cacheFirst TTL 60), `cacheNF` (networkFirst), `cacheSWR` (staleWhileRevalidate TTL 30), `direct` rest **route `direct`** (🧪 refused in production: `allowDirectDataSources false`), `liveStatic` static, `fn` function `fees.quote`, `gql` graphql `speakers`, `ws` websocket (race runner), `sse` sse (inbox), `db` database (`registrations` orderBy `submittedAt` descending limit 5)

- [ ] **`mt-home`** · Page · `GridView` maxCrossAxisExtent 180 of lab tiles (icon, name, count of options) → each lab; header `Card`: `readConfig supportPhone` (live, non-strict; 🧪 offline returns the last value in memory), active release, `flags.*` values, `device.*` readout (platform, OS, locale, direction, calendar, textScale, darkMode, sizeClass, assurance, reduceMotion, boldText, highContrast, screenReader), `user.*`
- [ ] **`mt-layout`** · Gallery: `Row`/`Column`/`Flex` (every `MainAxisAlignment`, `CrossAxisAlignment` incl. baseline with `TextBaseline` both, `MainAxisSize`, `VerticalDirection`, `TextDirection`, `spacing`), `Wrap` (every `WrapAlignment` for alignment and runAlignment, `WrapCrossAlignment`, `Axis`), `Stack` (`StackFit` ×3, `Clip` ×4, `alignment`), `IndexedStack` (`sizing`), `Positioned` (each edge), `Align`/`Center` (widthFactor, heightFactor), `Padding`, `SizedBox`, `ConstrainedBox`, `LimitedBox` (inside an unbounded `ListView`), `FractionallySizedBox`, `AspectRatio`, `FittedBox` (every `BoxFit`), `Baseline`, `Transform` (angle, scale, scaleX/Y, offset, origin, flipX/Y, filterQuality), `DecoratedBox` (`DecorationPosition` ×2), `ClipRRect`, `ClipOval`, `Opacity`, `ColoredBox`, `Expanded`/`Flexible` (`FlexFit` ×2, flex), `Spacer`, `SafeArea` (each side off, minimum, maintainBottomViewPadding)
- [ ] **`mt-paint`** · Gallery: `BoxDecoration` colour, border (all/each side, `BorderStyle` ×2, strokeAlign), borderRadius (all/each corner, elliptical `Radius` x/y), `BoxShadow` (every `BlurStyle`, spread, offset), `Gradient` (every `GradientKind` × every `TileMode`, stops, focal, start/end angle), `BoxShape` ×2, backgroundBlendMode; `Image` with `color` + every `BlendMode` (29 samples), every `FilterQuality`, every `ImageRepeat` (`asset:pattern.png`), cacheWidth/cacheHeight, gaplessPlayback, matchTextDirection, network image with loading/error slots; `ShapeBorder` every `ShapeKind` (eccentricity on circle); `BackdropFilter` blendMode + enabled false; `Card` borderOnForeground, semanticContainer
- [ ] **`mt-text`** · Gallery: `Text` every `TextAlign`, every `TextOverflow` (in a 120-px box, maxLines 1), `TextWidthBasis` ×2, softWrap false, locale, semanticsLabel/Identifier, selectionColor; `TextStyle` every `FontWeight`, `FontStyle` ×2, every `TextDecoration` × every `TextDecorationStyle`, decorationColor/thickness, letter/word spacing, height, `TextLeadingDistribution` ×2, backgroundColor, shadows, `TextBaseline` ×2, fontFamily fallback list, inherit false; `RichText` nested `TextSpan`s (style, locale, semanticsLabel, spellOut); `SelectableText` every `BoxHeightStyle` and `BoxWidthStyle`, cursor props, showCursor, enableInteractiveSelection false
- [ ] **`mt-inputs`** · Gallery: `TextField` with every `TextInputType`, every `TextInputAction` (13 each, one field per value; 🧪 check the keyboard on both platforms), every `TextCapitalization`, `SmartDashesType`/`SmartQuotesType` both, every `MaxLengthEnforcement`, every `TextAlignVertical`, `keyboardAppearance` dark/light, obscureText + obscuringCharacter, readOnly, enabled false, expands, minLines/maxLines, cursor (width, height, radius, colour, errorColour, opacity), selection styles, scrollPadding, hintLocales, stylusHandwritingEnabled, canRequestFocus false, onTapAlwaysCalled; `InputDecoration` every `InputBorderKind` (outline/underline/none) × enabled/focused/error/disabled borders, every `FloatingLabelBehavior`, every `FloatingLabelAlignment`, filled/fillColor, prefix/suffix (icon, text, widget), counter, helper, error, hint (maxLines, fade), isDense, contentPadding, constraints; `TextFormField` every `AutovalidateMode` + forceErrorText + onSaved; `DropdownMenu` every `DropdownMenuCloseBehavior`, enableFilter/enableSearch, expandedInsets, alignmentOffset, label/leading/trailing/selectedTrailingIcon, disabled entry; `CupertinoTextField` every `OverlayVisibilityMode` for prefix, suffix and clear button; `Autocomplete`; `SearchBar` + `SearchAnchor` (isFullScreen false)
- [ ] **`mt-buttons`** · Gallery: `ElevatedButton`, `FilledButton` (+ tonal), `OutlinedButton`, `TextButton` (isSemanticButton false once), `IconButton` (isSelected, selectedIcon, every `VisualDensity`, constraints, splashRadius), `FloatingActionButton` (mini, extended, every elevation prop), `BackButton`, `CloseButton`; one `ButtonStyle` with **every state** (Default, Pressed, Hovered, Focused, Disabled, Selected, Error, Dragged) for backgroundColor, foregroundColor, overlayColor, shadowColor, surfaceTintColor, elevation, padding, minimumSize, fixedSize, maximumSize, side, shape, textStyle, iconColor/size, `IconAlignment` start/end; every `MaterialTapTargetSize`; chips `Chip` (onDeleted, deleteIcon, avatar), `ActionChip`, `ChoiceChip`, `FilterChip`, `InputChip` with every prop incl. `color` per state; `Badge` (small/large size, offset, alignment, isLabelVisible false); `Tooltip` every `TooltipTriggerMode`, richMessage, durations, decoration; `SegmentedButton` direction vertical, multiSelectionEnabled, emptySelectionAllowed; `CupertinoButton` every `CupertinoButtonSize`, pressedOpacity, focusColor; **Variant picker** for the `Scaffold`'s `floatingActionButtonLocation` (all 19 values)
- [ ] **`mt-selection`** · Gallery: `Checkbox` (tristate null, isError, shape, side, every `VisualDensity`), `CheckboxListTile` (every `ListTileControlAffinity`, checkboxShape, checkboxScaleFactor, isThreeLine, dense), `Radio` (toggleable, innerRadius per state, backgroundColor per state), `RadioListTile` (radioScaleFactor), `Switch` (thumb images on/off, trackOutline per state, padding), `SwitchListTile`, `Slider` every `SliderInteraction`, every `ShowValueIndicator`, secondaryTrackValue, divisions, `RangeSlider` labels/overlay, `ListTile` every `ListTileStyle`, every `ListTileTitleAlignment`, minTileHeight, minLeadingWidth, horizontalTitleGap, selected/selectedTileColor, enabled false; `CupertinoSwitch`, `CupertinoSlider`, `CupertinoSlidingSegmentedControl` (disabledChildren, isMomentary)
- [ ] **`mt-scroll`** · Gallery + live: `ListView` (horizontal, reverse, itemExtent, prototypeItem, shrinkWrap, padding), `GridView` (crossAxisCount vs maxCrossAxisExtent, mainAxisExtent), `PageView` (vertical, padEnds false, physics page, allowImplicitScrolling), `CustomScrollView` (anchor 0.2, every `SliverPaintOrder`, center behaviour), `SliverPadding`, `SliverFillRemaining` (hasScrollBody false, fillOverscroll), `SliverList`/`SliverGrid` with semanticIndexOffset, `Scrollbar` every `ScrollbarOrientation`, thumb/track visibility; every `ScrollPhysics` (6 lists), every `ScrollViewKeyboardDismissBehavior`, every `DragStartBehavior`, every `HitTestBehavior`; every `ListStatus` via the list's **Show** override and via a bound status; **Variant picker** for the page's own `CustomScrollView.physics`
- [ ] **`mt-nav`** · Buttons for every `NavigationMode` (push, replace, popUntil `mt-home`, clearAndPush), `switchTab` each tab, `pop` with and without result, `openUrl` (allowlisted `https://<your-domain>` and a blocked domain 🧪 reported), a deep link to self, an unknown route (→ not-found), a missing param (→ `PLX-4101`), native `sign-in` with typed result and `licenses`; widgets `BottomNavigationBar` every `BottomNavigationBarType`, `NavigationBar` every `NavigationDestinationLabelBehavior`, `NavigationRail` every `NavigationRailLabelType` + extended, `TabBar` every `TabBarIndicatorSize` + isScrollable, `TabBarView` viewportFraction 0.9, `Drawer` + endDrawer (`Scaffold` drawerEdgeDragWidth, drawerScrimColor, drawerEnableOpenDragGesture false, endDrawerEnableOpenDragGesture, drawerBarrierDismissible false, onDrawerChanged), `NavigationDrawer` (header/footer), `MenuAnchor` (consumeOutsideTap, onOpen/onClose), `PopupMenuButton` (initialValue, onCanceled, onOpened), `AppBar` every prop (bottom `PreferredSize`-like `TabBar`, flexibleSpace, toolbarOpacity, bottomOpacity, leadingWidth, actionsPadding, forceMaterialTransparency, animateColor, primary false), `BottomAppBar`, `SliverAppBar` floating + snap + stretch
- [ ] **`mt-transitions`** · 10 buttons, each → a target page whose `routeOptions.transition` is one value: `platform`, `fade`, `slideLeft`, `slideRight`, `slideUp`, `slideDown`, `scale`, `sharedAxis`, `custom` (route timeline `mtRoute`, scope route), `none` — targets **`mt-t-platform` … `mt-t-none`** (10 small pages, each a `Text` of its kind and a back button); 🧪 Android predictive back on `platform` and `custom`
- [ ] **`mt-pages`** · Open **`mt-k-dialog`** (Dialog, dismissible false and true), **`mt-k-sheet`** (Bottom sheet, dismissible), **`mt-k-full`** (Full-screen dialog); each pops a typed result shown back on `mt-pages`; 🧪 a pop with a value of the wrong type is reported and returns none (`NAV-003`); a `navigate` push to a dialog route presents it as a dialog
- [ ] **`mt-motion`** · Gallery: every implicit widget — `AnimatedAlign`, `AnimatedContainer`, `AnimatedCrossFade` (every `CrossFadeState`), `AnimatedOpacity`, `AnimatedPadding`, `AnimatedPositioned`, `AnimatedRotation`, `AnimatedScale`, `AnimatedSize`, `AnimatedSlide`, `AnimatedSwitcher` each animating a toggle, with **every `Curve`** shown once (10 rows); node `animation`: implicit props (durationMs, delayMs, curve), **enter/exit** every `animTransitionKind` (fade, scale, slideUp/Down/Left/Right), **Play enter/Play exit** on canvas, `hero` tag, `reduceMotion` skip/shorten/ignore; timelines: `repeat` 3, `repeatForever`, `reverse`, `staggerMs` over a `ForEach`, `spring`, scope page and route, driver **scroll** and **drag** (a draggable handle drives a sheet's timeline), keyframes on transform, opacity, colour, size and a custom prop; `Timeline` widget autoplay false; actions `startAnimation` and `controlAnimation` with every `AnimationCommand` (play, pause, reverse, seek `position: 500`, stop); `FadeTransition`/`SlideTransition`/`ScaleTransition`/`RotationTransition` on tracks 0–3; `Hero` transitionOnUserGestures + reverseCurve; `LottieView` progress bound to a `Slider`, reverse, fit values; `RiveView` inputs, flags and triggers bound to state; 🧪 device Reduce Motion on: each declared behaviour holds; the compiler warns on an opacity over a large subtree (`ANI-008`) — keep one with a waiver note
- [ ] **`mt-gestures`** · A `GestureDetector` pad that logs every event group: tap (down/up/move/cancel), secondary and tertiary taps, double tap, long press (start/move/up/end/cancel), secondary/tertiary long press, vertical/horizontal drag, pan, scale (`trackpadScrollCausesScale`), force press; payload `GestureDetails` shown; `InkWell` every event incl. onHover, onHighlightChanged, onFocusChange (customBorder, radius); `Dismissible` every `DismissDirection` (7 tiles; resize/movement durations, crossAxisEndOffset); `Draggable` axis horizontal/vertical, rootOverlay; `InteractiveViewer` every `PanAxis`, scaleEnabled false, panEnabled false; `RefreshIndicator` every `RefreshIndicatorTriggerMode`, edgeOffset, onRefresh slow (2 s)
- [ ] **`mt-actions`** · One button per behaviour, each writes a log line into page state `log`:
  - [ ] control: `condition` (then/else), `switch` (cases + default), `forEach` over 5, `parallel` 3 lanes with `delay` 300/600/900, `stop` with result and with error, `callFlow` (local) and exported flows of Verify and Pay
  - [ ] robustness: `retry` `{count: 3, backoffMs: 200, maxBackoffMs: 2000, jitter: true, on: [network, timeout, http]}` against `X-AR-Fail: 500` then success; `timeoutMs 500` against `slow`; every error kind reached and routed step → page `onError` → plugin → app (`network`, `http` 401/404/409/500, `timeout`, `validation`, `function`, `permission` (deny camera), `cancelled` (leave mid-run), `custom` via `stop error`)
  - [ ] concurrency on a counter button: `parallel`, `drop`, `restart`, `queue`, `debounce:500`, `throttle:1000`; a `detached` handler that finishes after you leave
  - [ ] optimistic: `apiCall … optimistic {page.liked: true}` rolled back on `X-AR-Fail: 500`; `patchState … optimistic true` rolled back when a later step fails
  - [ ] feedback: `showToast` (duration 1000/4000), `showSnackbar` with action, every `HapticPattern`, `announce` every `Assertiveness`
  - [ ] device: `requestPermission` for each of the 10 device APIs; `getLocation` every `LocationAccuracy`; `pickImage` (multiple, maxDimension), `capturePhoto`, `pickFile` (mimeTypes, multiple); `scanCode` with **every `BarcodeFormat`** (13 printed test codes); `biometricAuth` allowDeviceCredential true and false; `copyToClipboard`; `share` text, url and file
  - [ ] app/host: `setThemeMode` system/light/dark; `setLocale` each locale; `sync`; `trackEvent` with props; `emitHostEvent refreshAll`; `callNative requestReview`, and an **unregistered** action (🧪 `PLX-4200`, safe failure); `logout` (test account only)
  - [ ] triggers: page timer (`intervalMs 2000`, repeat), a state watcher, lifecycle logs for `onInit`, `onEnter`, `onResume`, `onLeave`, `onDispose`
- [ ] **`mt-data`** · The four cache sources side by side with their TTL and last-fetched time (🧪 airplane mode: networkOnly fails, cacheFirst/SWR serve cache, networkFirst falls back); `direct` source (🧪 refused); `liveStatic`; `fn`; `gql`; `ws`/`sse` with `subscribe`/`unsubscribe` buttons; `db` with `dbQuery` (where, orderBy, descending, limit 2, offset 1), `dbInsert`, `dbUpdate`, `dbUpsert`, `dbDelete`; `kvSet`/`kvGet`/`kvRemove`; `DataScope` with explicit `status` and `isEmpty` overrides; `refreshData more`; outbox: 3 `offlineCapable` mutations queued offline replay **in order** with the same `Idempotency-Key`; per-source mock state (`Data: cacheNet` = Error, others Success); source triggers `onLoaded` and `onFailed` write to the log
- [ ] **`mt-state`** · One row per scope (`app`, `plugin`, `page`, `component` via a component instance, `run` via `setState run.x`), per persistence (`memory`, `session`, `persisted`, `secure` — 🧪 kill the app and reopen: which survive), a computed entry, a watched entry, an exposed entry the host shows in its own native badge, `resetState`, `patchState`; form `lab` with `validateForm`, `resetForm`, `submitForm` taking its **invalid** branch; a sensitive entry (🧪 absent from Process flow and Logs)
- [ ] **`mt-pxl`** · A list of `Text`s, each bound to one expression from every PXL group: strings (`len upper lower trim contains startsWith endsWith replace split join substring padLeft padRight matches`), numbers (`abs min max round floor ceil clamp int double`), decimal & money (`decimal money add sub mul div(scale, mode) round(scale, mode) currency amount isZero compare` with rounding modes `halfEven halfUp down up ceiling floor`, literal `12.50d`), dates (`date dateTime duration addDays addMonths diffDays startOfDay toCalendar fromCalendar weekday isBefore isAfter`), lists & maps (`size isEmpty first last at slice map filter any all sum sortBy distinct keys values has`), formatting (`format.number money date percent compact phone iban`), localisation (`t`, `plural`), logic (`coalesce ifNull isNull typeOf`), validation (`isEmail isPhone isIban isNumeric luhn`), operators (`?.`, `??`, `? :`, `in`, `%`, `!`), roots (`app plugin page params event props item index steps data form flags env device user now`); 🧪 evaluate each in the Dev app **Console** against live state; a 10,001-operation expression fails with its budget error
- [ ] **`mt-l10n`** · ICU samples: `plural` (`=0`, `one`, `few`, `many`, `other`, `offset:1`), `selectordinal`, `select` (gender), `number` (percent, currency), `date`/`time` styles; fallback chain (`de-AT` → `de` → `en`); a key with no `am-ET` message (🧪 Localisation view marks it; device shows the default and reports `l10n_missing`); a text overflowing in `de` and in the pseudo-locale; `localeOverrides` on a node (prop changes only in `ar`, then only in `de-DE`); a per-locale image; `Text` `locale` prop; node `direction` override; every `CalendarSystem` in `DualCalendarDatePicker`; `numberingSystem native` in staging (🧪 Arabic-Indic digits on device)
- [ ] **`mt-a11y`** · A widget per semantics field (`label`, `hint`, `value`, `header` + `headingLevel` 1–6, `button`, `liveRegion`, `excludeSemantics`, `focusOrder`, `group`, `decorative`); a 32×32 target and a low-contrast text, each with an `a11yWaivers` entry (code, justification) — 🧪 the accessibility report lists the waivers; `announce`; bindings to `device.reduceMotion`, `boldText`, `highContrast`, `screenReader`; a TalkBack and a VoiceOver walk-through script (§I4)
- [ ] **`mt-secure`** · `secure: true` · `requiresAssurance AL3` (🧪 most phones are AL2: the page's fallback shows, `degrade` rules apply); `copyToClipboard` is blocked here; `SecureTextField` (obscure false once), `MaskedText` every `MaskKind`, `SecurePinPad` randomize on/off; inactivity lock; RASP: run on an emulator (report), a rooted test phone (warn), a repackaged APK (block)
- [ ] **`mt-l2`** · Gallery of every Layer 2 component not already exercised with all its props: `PaymentCard` every `CardBrand`, `MoneyText` every `MoneyDisplay` × every `SignDisplay`, `CountdownTimer` every `CountdownFormat`, `TransactionListItem` every `TransactionStatus`, `QrCodeView` every `QrErrorCorrection`, `OtpInput` alphabet alphanumeric and length 4, `QrScanner`/`BarcodeScanner` `CameraFacing` front + torch + scanWindow, `StatusTimeline` every `TimelineStepState`, `KycCameraCapture` both modes, `SkeletonLoader` lines 1/5 avatar shimmer false, `OfflineBanner` visible false, `EmptyState`/`ErrorState` with and without action, `LiveEtaCard` all props, `ProfileHeader` actions slot, `SettingsList` sections, `OnboardingPager` currentPage 2, `WizardForm` stepFields, `ConsentCheckbox` value true, `TermsViewer` long text, `Carousel` autoplay off, `ImageGallery` initialPage 2, `RatingBar` 2.5, `RatingInput` max 3, `QuantityStepper` step 5, `PriceTag` without original, `ProductCard` without image, `CartSummary` without tax, `SearchField` suggestions, `FilterBar`, `SortSheet`, `InfiniteList` error, `SignaturePad` format png, `AudioPlayer` loop autoplay, `VideoPlayer` loop + captions, `LottieView` animate false, `RiveView` artboard, `MapView` tiles + attribution + non-interactive, `OrderTrackerMap` follow false, charts with `summaryTemplate` and `showTable`, `BiometricButton`, `DualCalendarDatePicker` without secondary
- [ ] **`mt-responsive`** · `Responsive` compact/medium/expanded; props with `M:`/`E:` overrides (padding, crossAxisCount, direction); `visible {{ device.sizeClass == "expanded" }}`; `If` on size class; landscape/portrait; foldable fold crossing marks; **Make navigation adaptive**; `SafeArea` per side under the device chrome (punch hole, notch, island)
- [ ] **`mt-components`** · Every library component with **every prop value**: `◇BaseCard` ×3 variants × tappable/selected/dense, each slot filled and empty; `◇OrgCard` default/overridden/compact, `badge` filled and fallback, `open` payload shown, v1 vs v2 adoption, a **detached** copy; `◇HeroHeader` with/without subtitle and badge; `◇SectionHeader` with/without action; `◇StepHeader` 1/4…4/4; `◇FieldGroup` with/without hint and trailing; `◇SubmitBar` busy/disabled/secondary; `◇InfoRow` icon/copyable/ltr; `◇MoneyRow` emphasis/signed; `◇StatusChip` every status + otherwise; `◇EmptyHint` with/without cta; `◇ErrorBlock` default and custom icon; every `emitEvent` wired to a log; **Isolate**; all 6 templates inserted; 🧪 change `@radius.card` in the theme: every card in the app changes
- [ ] **`mt-native`** · `NativeSlot CalendarButton` and `HostVersionTile` (props in, events out, layout constraints, a native exception contained `PLX-4205`); native routes with typed params/result; `emitHostEvent` and a `toPlux` event (`refreshAll`) received through `triggers.hostEvents`; exposed `app.unreadCount` changed from the host
- [ ] **`mt-move-me`** · A plain page used to test **Move to another plugin** (F3) and back

---

## H. Localisation and accessibility

### H1. Keys and messages (Localisation table on each plugin canvas; app › Localisation)

- [ ] Every `t:` key above exists with an `en` message; inline-edited texts become keys named after the page (`home.…`); 🧪 a literal text left in an app with 4 locales is marked by Localisation view
- [ ] Key groups: `common.*` (ok, cancel, done, next, back, retry, close, share, save, later, settings, open, undo, star, of, min, use, continue, send, reset), `tab.*`, `home.*`, `welcome.*`, `interests.*`, `lang.*`, `inbox.*`, `explore.*`, `filters.*`, `map.*`, `peek.*`, `org.*`, `scan.*`, `passes.*`, `pass.*`, `st.*`, `live.*`, `receipt.*`, `feedback.*`, `draft.*`, `acc.*`, `signin.*`, `f.*`, `pref.*`, `privacy.*`, `legal.*`, `sec.*`, `del.*`, `about.*`, `otp.*`, `kyc.*`, `sig.*`, `addr.*`, `stepup.*`, `pay.*` (incl. `pay.err.INSUFFICIENT_FUNDS|DECLINED|LIMIT_EXCEEDED|WALLET_UNAVAILABLE|AMOUNT_INVALID`), `pt.*`, `cs.*`, one prefix per org (`nb.*`, `cp.*`, `cr.*`, `dc.*`, `cl.*`, `rl.*`, `hm.*`, `ar.*`), function message keys (`bank.underage`, `fees.unknownTier`, `partner.duplicate`, …), `error.generic`, `nf.*`, `fb.*`, `whatsNew.body`
- [ ] ICU forms used in real screens: `otp.sent {phone}`, `otp.resendIn {s, plural, one {# second} other {# seconds}}`, `cp.words {n, plural, offset:1 =0 {No words} one {One word} other {# words}}`, `cp.choiceOrdinal {n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}`, gender `select` in `cp.personal`, `cs.count {n, plural, …}`, `cs.welcome {name}`, `cr.eta {m, number}`, `cl.wait {m, plural, …}`, `dc.max {n}`, `pay.confirm {amount}`, dates `{d, date, medium}`, times `{t, time, short}`
- [ ] Translate into `de-DE`, `ar`, `am-ET`: **Export translations** (XLIFF 2.0) → translate → **Import translations**; also try ARB, JSON and CSV once each; 🧪 a broken file is refused before anything changes
- [ ] Workflow: statuses `new` → `translated` → `reviewed` → `approved` (Translator edits, Reviewer approves); **Heatmap** shows 0 missing per plugin; production needs approved translations (`requireApprovedTranslations`)
- [ ] Screenshot context per key (from a paired device, **Capture screenshot**) for the 20 longest keys
- [ ] **Add a locale** `de-AT` (no messages) to test the fallback chain, then remove it

### H2. Accessibility

- [ ] Accessibility view on every screen: focus order numbered, targets ≥ 48 × 48 dp (Android) / 44 × 44 pt (iOS), contrast 4.5 : 1 (3 : 1 large) in light, dark and high contrast
- [ ] Inspector › Accessibility on: images (alt text or Decorative), headers (`header`, level), live regions (`pay` processing, `ps-live`, `cl-queue`), groups (cards), explicit `focusOrder` on `ar-join`
- [ ] Release accessibility report (JSON and HTML, §J1): violations 0, waivers only the two in `mt-a11y`
- [ ] Device runs: TalkBack (Android) and VoiceOver (iOS) through Home → Explore → an org → Verify → Pay → Pass; 200 % text on every org's first page; bold text; reduce motion; high contrast

---

## I. Tests

### I1. Scenarios (`plux test`, Studio › Plux Test)

- [ ] One scenario per plugin flow (≥ 17): e.g. `explore-search` (enter text `aster` in `explore.search`, wait for `orgcard-campus`, tap, expect **Navigated to** `ex-org`), `pay-success`/`pay-error` (mock `pay.simulate`), `arena-validators` (enter invalid values, expect each error text), `campus-offline-submit`, `bank-sca-declined`, `console-guard`
- [ ] **Create test from device session** once (from a Dev app session)
- [ ] `plux test --report --app <id> --label "release 1"`; Runs tab shows passed/failed/skipped

### I2. Goldens

- [ ] Golden matrix of §B3 for every screen; **approve** baselines; `goldens.blockPublish true` → 🧪 change a colour token and see publish blocked until approved

### I3. Live debugging (bottom console, Dev app paired)

- [ ] **Problems**, **Logs** (level filter, search, pause), **Process flow** (click a step → selects the node), **State** (edit `app.unreadCount` live), **Network** (simulate offline, 2G, 3G, high latency, packet loss), **Functions** (server logs of `partner.submit`), **Sync**, **Performance**, **Console** (PXL against device state); **Export trace**; fake the clock (CountdownTimer, `pay.simulate` outcome); trigger lifecycle events
- [ ] Inspect both ways: tap on the device selects the node; select a node highlights it on the device

### I4. Edge-case sweep (tick per device: one Android low-end, one Android flagship, iPhone, iPad)

- [ ] First launch offline (baseline release shows), then online sync; `blockUntilSynced` build waits; a release activates at a safe point, not under an open page
- [ ] Kill switch, mandatory release, rollback (§J3) seen on device
- [ ] Deep links (`/p/<route>`, `/o/<slug>`, `/pass/<id>`, `applyreg://…`), push open cold and warm, unknown route, bad params
- [ ] Permissions denied for each device API used, and later granted from settings
- [ ] Locale switch at runtime (4 + pseudo), RTL, 200 % text, dark, high contrast, reduce motion, screen readers
- [ ] Network: offline, flaky, slow; outbox replay order; strict live value fail-closed; SSE/WebSocket reconnect
- [ ] Security: device registration and assurance (AL1/AL2), revoked device, RASP responses, secure pages, sensitive values absent from traces/logs/analytics/replay, `logout` wipes data
- [ ] Persisted-state and collection migrations from release 1 to release 2 (§J5)
- [ ] Limits: a page over 1,000 nodes warns; a plugin's assets near the limit; a function over its timeout

---

## J. Publish, operate, ship

### J1. Publish flow (Publish in the top bar)

- [ ] **Select plugins** (a needed plugin is added for you) → **Freeze and build** (Queued/Running/Built; **Freeze again**) → **Reports** (validation, size vs previous, accessibility) → **Visual diffs** + **Changed documents** → **Changelog** → **Target** (environment, channel) → **Approval** → **Rollout plan** → **Release**
- [ ] Release 1 to `development` (everything), then `staging`, then `production`
- [ ] 🧪 a validation error, a budget overrun and an accessibility violation each block publish (`PLX` code shown); fix and publish

### J2. Approvals

- [ ] Policy `prod-four-eyes`: resource release, environment production, approvers 1 Release Manager **distinct from the author**; policy `bank-strict`: pages marked `secure` or flows with money (Bank, Pay) need 2 approvers from different teams; **simulate** the policy on a sample change first
- [ ] Inbox › open request (visual diff, JSON diff, reports, risk, install base) › **Approve** with WebAuthn step-up; 🧪 changing the release after approval invalidates it (hash-bound); a change freeze blocks a production publish; **break-glass** kill switch with justification, then review it

### J3. Rollouts, kill switch, rollback, mandatory

- [ ] Rollout plan: 1 % → 10 % → 50 % → 100 % with durations, scheduled start; targeting by `segment beta` first; health gate (crash-free, Plux error rate, action error rate, function error rate)
- [ ] Controls: **Pause**, **Resume**, **Advance to next stage**, **Change end**, **Abort rollout**
- [ ] **Kill switch** `cityrun` (Switch off → devices show `ln-offline` on its routes; Switch on)
- [ ] **Roll back to this release** (step-up); 🧪 devices never go back in sequence (anti-rollback: the rollback is a new sequence)
- [ ] One **mandatory** release (e.g. fix in Pay): devices update before showing any Plux page
- [ ] Confidential bundle for `bank` (`SEC-053`, WIP): an AL1 device cannot decrypt it and keeps the last release it could

### J4. Experiments and flags

- [ ] Experiment `home-hero` (subject: component subtree on `home`): control + 2 variants (`flags.heroCopy` text, hero image), allocation 34/33/33, holdout 5 %, targeting `region in [DE, ET, SA]`, primary metric `org_opened` (share of subjects), guardrail `unhandled_error`, subjects per variant 500; **Start**, read **Results** (interval, p-value, verdict; SRM warning), **Ship this variant**
- [ ] A second experiment on a **prop value** (`SubmitBar` label) in a mutually exclusive **layer** with the first; 🧪 an experiment on `bank` needs approval (`ABT-007`)
- [ ] Flags (§B3) with **values per segment** (`beta`: `maxWorkshops 3`, `mapClustering true`); change `showMaintainers` and see it without a release

### J5. Release 2 (migrations)

- [ ] `app.interests` → `list<Interest>` with `migration`; collection `registrations` v2 rename `note`→`memo` (acknowledge the warning); `OrgCard` v2 adopted by Explore only
- [ ] 🧪 a device on release 1 with data upgrades cleanly; a device two releases behind resets and reports

### J6. Analytics and limits

- [ ] Funnels per org (`org_open → form_start → pay_ok → submitted`); **Usage** by plugin → page; **Errors** → **Open node** jumps to the canvas; **Performance** percentiles; **Replays** (staging, replay consent on): play, timeline, State so far, sensitive values **Hidden**
- [ ] Limits usage at < 80 % (release size, plugins 17/200, pages, assets, functions); 🧪 temporarily lower a plugin limit in Admin to see **Near limit** / **At limit**

### J7. Store release

- [ ] `plux create` again from the production release (§A3) → host changes re-applied (keep them in your fork) → `flutter test integration_test/app_test.dart -d <device>`
- [ ] Android: `scripts/build_android.sh` (upload keystore), Play Console: app content (privacy policy URL, **data safety**: contact info, user IDs, photos/files, location, health info for Clinic; encryption in transit; deletion available in app), target audience 13+, permissions declarations (camera, location, photos), content rating, closed testing track → production
- [ ] iOS: `scripts/build_ios.sh`, App Store Connect: privacy nutrition labels (same data), usage strings, Sign in with your own account + **in-app account deletion** (`ac-delete`), demo account `+15555550100` / `000000` in review notes, export compliance (standard encryption), age rating, screenshots (6.9", 6.5", iPad 13")
- [ ] Review notes: "Content is delivered as signed declarative data rendered by the app's own native widgets; no executable code is downloaded." List what the Maintainers plugin is (a public showcase of the app's components)
- [ ] After approval: phased release on both stores; first production Plux release follows the same rollout (§J3)

---

## Appendix K — Studio feature checklist (both canvases and their editors)

Tick each once you have used it while building §F–§G (where to use it is in brackets).

### K1. Shared canvas behaviour

- [ ] Opens at 100 % centred beside the docks; zoom out/in, percent resets, Zoom to fit (Shift+1), Ctrl+0; wheel/two-finger pan; Ctrl/Cmd+wheel and pinch zoom about the pointer; Hand (H), Space, middle button; Select (V); Escape
- [ ] Top bar in three parts (identity · tools · status); saved state; error and warning counts open Problems; light/dark/contrast switch; command palette ⌘K/Ctrl+K (jump to plugin, screen, node, component, template, key, function, command); shortcut sheet `?`
- [ ] Docks fold and reopen (header button, rail tab), on both sides of both canvases; only the first folding section of each pane is open at first and Studio remembers your choices
- [ ] Drop zones (drag or click), custom scrollbars, dropdowns never cut off (Studio look checks)

### K2. App canvas (§F)

- [ ] Plugin box facts and badges; detail card on hover/focus; native nodes; frames (create, rename, resize); snapping, guides, marquee, multi-select, Arrange, Move to…; positions saved per app
- [ ] Context menu and toolbar: Open plugin (double-click), Plugin settings, Duplicate plugin, Make entry plugin, Move to…, Archive/Restore, Delete → Trash (30 days) → restore
- [ ] Bundled arrows: count badge, opposite bundles offset, shortest sides with hysteresis, anchor spreading, curves routed around boxes, hover/focus expansion with lane titles and dimming, lane click menu (open source widget, open target, change trigger, redirect, remove), drag from a connection handle, low-zoom simplification, live re-routing while dragging
- [ ] Dock panes: Plugins (search, Unpublished / With errors / Archived), Frames, Overview; safety rules (linked plugin not deletable, entry plugin, redirect incoming, remove outgoing, type-to-confirm in production, locks)

### K3. Plugin canvas — screens (§G)

- [ ] All screens in device frames with arrows in navigation order; **Arrange by flow**; drag screens by the header bar or body (snaps, Undo); marquee across screens (Alt: inside only)
- [ ] Screen header bar: kind icon, title (click to rename), Entry tag, route; quick actions (**Page settings**, **Show on every device**, **Zoom to screen**); ⋯ menu (**Rename**, **Duplicate**, **Make it the entry screen**, **Move to another plugin…**, **Delete…**); narrow frames drop the route then the quick actions
- [ ] **New screen** with **Kind** (Page, Dialog, Bottom sheet, Full-screen dialog) from a fitting layout; from a template; dialogs drawn over their opener with a scrim, sheets at the bottom
- [ ] Ghost screens (other plugins' pages: dashed, plugin name, thumbnail, link count) → confirmation → open, pan, pulse; native nodes
- [ ] Overflow region below a frame for long scroll content; device chrome (status bar, punch hole / notch / island, home indicator or buttons; **Device chrome: Hidden**); SafeArea and app bar under the status bar; status-bar icon colour follows the bar
- [ ] Scaffold in place: app bar, navigation bar, persistent footer, FAB at its location, bottom sheet, drawer; chips on a selected page to add **App bar**, **Navigation bar**, **Action button**, **Drawer**; **Show drawer**
- [ ] **Interact** mode (I): scroll lists, switch tabs, destinations, segments (bound page state switches what it drives), open/close the drawer, back/close, a button that opens a page brings it into view; network/function actions are named, not run
- [ ] Preview: **Device frame** (all presets: small Android 360×800, Pixel, Galaxy A, entry-level, iPhone SE, iPhone, Pro Max, iPad, foldable) and **Custom frame**, **Orientation**, **Theme**, **Brand**, **Locale** (RTL, pseudo), **Text scale**, **Material/Cupertino**, **Data state** (one for all or per source); **Device matrix** (current screen in every frame; Back to the canvas / Escape)
- [ ] Views in the tool bar: **Accessibility view**, **Localisation view**, **Size classes view** (with foldable crease marks)
- [ ] Per-screen cost under each frame (nodes vs budget, simultaneous timelines, problems); Performance tab lists all screens

### K4. Plugin canvas — editing

- [ ] Hover outline with name; click top-level, Ctrl/Cmd-click innermost, Shift-click toggle, double-click one level in, Esc one level out, Enter first child, Tab next sibling; selection name, size, 8 resize handles; arrow nudge 1 / Shift 10
- [ ] Drag to move: insertion line in rows/columns/wraps, green target container, free move inside a stack with guides and distances; drop into empty single-child widgets; onto another screen (one undo step)
- [ ] Right-click menus (widget, screen, canvas): copy, cut, paste, duplicate, wrap, unwrap, select parent/first child, zoom to selection (Shift+2), delete; copy/paste across screens, plugins and apps (id remapping, dependency report)
- [ ] Selection toolbar: **Wrap in**, **Convert to**, **Add after**, **Align in row or column**, **Duplicate**, **Add an action**, **Delete**; **Make navigation adaptive** on a navigation bar; **Create experiment from selection**; **Save as template**; **Extract to component**
- [ ] Spacing overlay (padding magenta, gaps violet) and handles (Alt both sides, Shift all four), token snapping (`spacing.*`), resize snapping to dimension tokens
- [ ] Inline text editing (double-click; Enter, Shift+Enter, Escape) creating translation keys
- [ ] Problem dots (red/amber) and problems under the selection; binding badges (teal, count) with **The canvas shows**; event badges (amber, count); instance diamonds (violet/amber/red)
- [ ] Lists from data (up to 24 copies, `item`/`index`), **Show** state override, **Add an empty/loading/error view**; conditions (**Visible when** Now true/false/device-only, draw From data/branch/Shown/Hidden); `If`/`Match`/`ForEach` drawn by design-time values
- [ ] Forms preview **Pristine / Errors / Valid / Submitting**; **Form field** binding in one step
- [ ] State colours per interaction state chip with the state layer drawn; **Copy style** / **Paste style**
- [ ] Snackbar drawn at the bottom above the navigation bar when its widget is selected
- [ ] Timelines strip: play, pause, scrub **Playhead**, tracks, keyframes (click, Shift+click remove), **Add keyframe**, settings (duration, repeats, Loop, Reverse on repeat, Play when the page shows, Driven by scroll); motion **Play enter / Play exit**; Hero partners outlined
- [ ] Locale preview with fallback chain, ICU formatting, plural sample, **Only in** this locale props
- [ ] Theme round trip: token where-used (select all users), edit value live, **Not a token** → **Use** / **Make a token**, **Unlink**
- [ ] Undo/redo (persisted per plugin), autosave, offline waiting, conflict dialog (**Keep mine**, **Take the server's**, **Copy mine as JSON**), History (compare two snapshots, Restore), locks (**Request edit**, **Take over**)

### K5. Left dock

- [ ] **Insert**: search, Categories (Layout, Content, Inputs, Interaction, Navigation, Motion, Platform, More), Recently used, hover details, drag or click; Layer 2, native slots, components, templates
- [ ] **Layers**: screens as roots, carets, Expand/Collapse all, Filter layers, drag to move/reparent, rename (F2/double-click), hide (Ctrl+Shift+H), lock (Ctrl+Shift+L), selection card (**Show on canvas**, **Clear selection**)
- [ ] **Assets**: drop/click upload, search and kind filter, usage counts or **Unused**, size meter vs `plugin.assetBytes`, details (preview, type, pixels, bytes, hash, variants, findings), **Select its uses**, **Replace…**, delete warning; drag an image onto an image/avatar to replace, elsewhere to add
- [ ] **Data**: app/plugin/page **State** (computed, forms) and **Data sources** (mocks, connections)

### K6. Right dock (Inspector)

- [ ] Header (widget, screen, id); nothing selected → **Plugin overview**
- [ ] **Properties**: sections Content, list, Layout (Auto layout: Direction, 9-point grid, Distribute, Stretch, Gap, Run gap, Size hug/fill; child sizing Hug/Fill (Share)/Fixed; stack pins with Set width/height; Constraints; **Reset layout**), Appearance, Theme card, Keyboard & input, Behaviour, Form field, Conditions, Motion, Other; section counts; **Filter**, **Set only**; **Size class** M/E overrides with markers and clear; reset by name; literal ↔ translation ↔ expression; segmented choices, 3×3 alignment, four-side padding, folding composite rows (decoration, text style, border, list) with summaries; colour picker (theme colours, tokens, custom + opacity); decoration editor (fill, gradient, corners/circle, border, elevation shadow); shape editor; number with dimension tokens; icon picker (Material + Cupertino search); image picker; PXL editor (completion, hover types, inline errors, signature help)
- [ ] **Page** (page settings) · **Editors** (JSON, translations, theme, state, data sources, database, action graph, Rust) open as full-screen overlays and return to the same zoom/selection · **Actions** (events, **Quick actions**: Open a dialog, Open a sheet, Show a message, Open a link, Go back, Set state, Submit a form; replace or run after; **What its events run**) · **JSON** · **Code** · **Localisation** · **Accessibility** · **Performance** · **Docs**
- [ ] Instance card: component, version, overrides, **Isolate**, **Open component**, **Detach**
- [ ] Lane parameters on an arrow: each declared param as expression/number/bool/"text", missing required ones named, **Opens with** transition

### K7. Full-screen editors

- [ ] **JSON editor**: validates as you type, inline diagnostics, diff against last published, **Apply** only when valid
- [ ] **Action graph editor**: nodes with typed ports, catalogue (built-in, host/custom, functions, flows), branches (`next`, `onSuccess`, `onError`, named), inline PXL, validation of every path; flows with typed inputs/outputs and **exported**
- [ ] **State designer** (single value, list, map; memory/session/persisted/secure; sensitive; computed; exposed), **Data source designer** (OpenAPI/GraphQL import, operations, schemas, base URL per env, gateway vs direct, mocks), **DB schema designer** (key, primary key, indexes, fields, sensitive, migration plans, **Preview the migration**), **Theme editor**, **Localisation table** (keys shown, locale, Add key, **Move to a translation key**), **Rust editor** (template, dependencies, Run the tests, build output, language server)

### K8. Bottom console and devices

- [ ] Slim bar → open; tabs Problems, Logs, Process flow, State, Network, Functions, Sync, Performance, Console; source device, filters, search, Pause, Export trace; resize by dragging or arrow keys
- [ ] **Pair a device** (QR, link, short code, New code), **Send to device** (route, language, theme, text scale), **On the device**, **Highlight selection on device**, **Capture screenshot**, **Compare with the canvas** (side by side, overlay with opacity, Difference)

### K9. Other Studio areas used by this app

- [ ] Apps grid (§B2) · App settings & limits · Releases (§J1, §J3) · Approvals (§J2) · Experiments & flags (§J4) · Functions (§E) · Localisation (§H1) · Design (§C) · Library (§D) · Data (§A2, §G) · Analytics (§J6) · Devices & security (§B5) · Plux Test (§I1) · Admin (§B1)

---

## Appendix L — Every widget, every property

Each widget: where the real screens use it, the Maintainers screen that shows **all** its properties, then one ☐ per property (set it to a non-default value at least once), event (wire it at least once) and slot (fill it at least once). Enum-typed properties take every value on the screen Appendix M names.

### L1. Layer 1

- [ ] **ActionChip** (P3, core) — used on `dc-agenda`, `mt-buttons` · all on `mt-buttons`
  ☐labelStyle ☐labelPadding ☐pressElevation ☐tooltip ☐side ☐shape ☐clipBehavior ☐autofocus ☐color ☐backgroundColor ☐disabledColor ☐padding ☐visualDensity ☐materialTapTargetSize ☐elevation ☐shadowColor ☐surfaceTintColor ☐avatarBoxConstraints ☐⚡onPressed ☐▢avatar ☐▢label
- [ ] **Align** (P3, core) — used on `pay`, `mt-layout` · all on `mt-layout`
  ☐alignment ☐widthFactor ☐heightFactor ☐▢child
- [ ] **AnimatedAlign** (P5, core) — used on `mt-motion` · all on `mt-motion`
  ☐alignment ☐heightFactor ☐widthFactor ☐curve ☐duration ☐⚡onEnd ☐▢child
- [ ] **AnimatedContainer** (P5, core) — used on `dc-tickets`, `mt-motion` · all on `mt-motion`
  ☐alignment ☐padding ☐color ☐decoration ☐foregroundDecoration ☐width ☐height ☐constraints ☐margin ☐transformAlignment ☐clipBehavior ☐curve ☐duration ☐⚡onEnd ☐▢child
- [ ] **AnimatedCrossFade** (P5, core) — used on `nb-welcome`, `cp-essay`, `mt-motion` · all on `mt-motion`
  ☐crossFadeState ☐firstCurve ☐secondCurve ☐sizeCurve ☐alignment ☐duration ☐reverseDuration ☐excludeBottomFocus ☐clipBehavior ☐⚡onEnd ☐▢firstChild ☐▢secondChild
- [ ] **AnimatedOpacity** (P5, core) — used on `ex-peek`, `mt-motion` · all on `mt-motion`
  ☐opacity ☐alwaysIncludeSemantics ☐curve ☐duration ☐⚡onEnd ☐▢child
- [ ] **AnimatedPadding** (P5, core) — used on `mt-motion` · all on `mt-motion`
  ☐padding ☐curve ☐duration ☐⚡onEnd ☐▢child
- [ ] **AnimatedPositioned** (P5, core) — used on `cr-race`, `mt-motion` · all on `mt-motion`
  ☐left ☐top ☐right ☐bottom ☐width ☐height ☐curve ☐duration ☐⚡onEnd ☐▢child
- [ ] **AnimatedRotation** (P5, core) — used on `ex-map`, `mt-motion` · all on `mt-motion`
  ☐turns ☐alignment ☐filterQuality ☐curve ☐duration ☐⚡onEnd ☐▢child
- [ ] **AnimatedScale** (P5, core) — used on `BaseCard`, `mt-motion` · all on `mt-motion`
  ☐scale ☐alignment ☐filterQuality ☐curve ☐duration ☐⚡onEnd ☐▢child
- [ ] **AnimatedSize** (P5, core) — used on `nb-eligibility`, `dc-merch`, `mt-motion` · all on `mt-motion`
  ☐alignment ☐curve ☐duration ☐reverseDuration ☐clipBehavior ☐⚡onEnd ☐▢child
- [ ] **AnimatedSlide** (P5, core) — used on `nb-pin`, `mt-motion` · all on `mt-motion`
  ☐offset ☐curve ☐duration ☐⚡onEnd ☐▢child
- [ ] **AnimatedSwitcher** (P5, core) — used on `SubmitBar`, `ps-live`, `pay`, `dc-cart`, `cl-queue`, `mt-motion` · all on `mt-motion`
  ☐duration ☐reverseDuration ☐switchInCurve ☐switchOutCurve ☐▢child
- [ ] **AppBar** (P3, core) — used on `ln-interests`, `inbox`, `explore`, `ex-filters`, `passes`, `pass` +6 · all on `mt-nav`
  ☐automaticallyImplyLeading ☐automaticallyImplyActions ☐elevation ☐scrolledUnderElevation ☐shadowColor ☐surfaceTintColor ☐shape ☐backgroundColor ☐foregroundColor ☐primary ☐centerTitle ☐excludeHeaderSemantics ☐titleSpacing ☐toolbarOpacity ☐bottomOpacity ☐toolbarHeight ☐leadingWidth ☐toolbarTextStyle ☐titleTextStyle ☐forceMaterialTransparency ☐useDefaultSemanticsOrder ☐clipBehavior ☐actionsPadding ☐animateColor ☐▢leading ☐▢title ☐▢actions ☐▢flexibleSpace ☐▢bottom
- [ ] **AspectRatio** (P3, core) — used on `OrgCard`, `hm-listing`, `mt-layout` · all on `mt-layout`
  ☐aspectRatio ☐▢child
- [ ] **Autocomplete** (P5, plux_components) — used on `vf-address`, `mt-inputs` · all on `mt-inputs`
  ☐options ☐hintText ☐optionsMaxHeight ☐initialValue ☐⚡onSelected
- [ ] **BackButton** (P3, core) — used on `cp-programme`, `mt-buttons` · all on `mt-buttons`
  ☐color ☐style ☐⚡onPressed
- [ ] **BackdropFilter** (P5, core) — used on `ex-peek`, `mt-paint` · all on `mt-paint`
  ☐filter ☐blendMode ☐enabled ☐▢child
- [ ] **Badge** (P3, core) — used on `home`, `ex-peek`, `dc-home`, `dc-workshops`, `mt-buttons` · all on `mt-buttons`
  ☐backgroundColor ☐textColor ☐smallSize ☐largeSize ☐textStyle ☐padding ☐alignment ☐offset ☐isLabelVisible ☐▢label ☐▢child
- [ ] **Baseline** (P3, core) — used on `mt-layout` · all on `mt-layout`
  ☐baseline ☐baselineType ☐▢child
- [ ] **BottomAppBar** (P3, core) — used on `cr-live`, `mt-nav` · all on `mt-nav`
  ☐color ☐elevation ☐clipBehavior ☐notchMargin ☐padding ☐surfaceTintColor ☐shadowColor ☐height ☐▢child
- [ ] **BottomNavigationBar** (P4, core) — used on `cs-dashboard`, `mt-nav` · all on `mt-nav`
  ☐items ☐currentIndex ☐elevation ☐type ☐backgroundColor ☐selectedItemColor ☐unselectedItemColor ☐iconSize ☐selectedFontSize ☐unselectedFontSize ☐showSelectedLabels ☐showUnselectedLabels ☐⚡onTap
- [ ] **CalendarDatePicker** (P5, plux_components) — used on `hm-apply`, `hm-viewing` · all on `mt-inputs`
  ☐initialDate ☐firstDate ☐lastDate ☐currentDate ☐⚡onDateChanged ☐⚡onDisplayedMonthChanged
- [ ] **Card** (P3, core) — used on `BaseCard`, `pass`, `pt-pitch`, `dc-home`, `hm-apply`, `maintainers (plugin)` +2 · all on `mt-paint`
  ☐color ☐shadowColor ☐surfaceTintColor ☐elevation ☐shape ☐borderOnForeground ☐margin ☐clipBehavior ☐semanticContainer ☐▢child
- [ ] **Center** (P3, core) — used on `ac-sign-in`, `pay`, `mt-layout` · all on `mt-layout`
  ☐widthFactor ☐heightFactor ☐▢child
- [ ] **Checkbox** (P3, core) — used on `cl-symptoms`, `rl-volunteer`, `mt-selection` · all on `mt-selection`
  ☐value ☐tristate ☐activeColor ☐fillColor ☐checkColor ☐focusColor ☐hoverColor ☐overlayColor ☐splashRadius ☐materialTapTargetSize ☐visualDensity ☐autofocus ☐shape ☐side ☐isError ☐semanticLabel ☐adaptive ☐⚡onChanged
- [ ] **CheckboxListTile** (P3, core) — used on `ex-filters`, `pt-request`, `cp-programme`, `cr-register`, `dc-workshops`, `mt-selection` · all on `mt-selection`
  ☐value ☐activeColor ☐fillColor ☐checkColor ☐hoverColor ☐overlayColor ☐splashRadius ☐materialTapTargetSize ☐visualDensity ☐autofocus ☐shape ☐side ☐isError ☐enabled ☐tileColor ☐isThreeLine ☐dense ☐selected ☐controlAffinity ☐contentPadding ☐tristate ☐checkboxShape ☐selectedTileColor ☐enableFeedback ☐horizontalTitleGap ☐minVerticalPadding ☐minLeadingWidth ☐minTileHeight ☐checkboxSemanticLabel ☐checkboxScaleFactor ☐titleAlignment ☐adaptive ☐⚡onChanged ☐⚡onFocusChange ☐▢title ☐▢subtitle ☐▢secondary
- [ ] **Chip** (P3, core) — used on `StatusChip`, `ex-peek`, `hm-listing`, `mt-buttons` · all on `mt-buttons`
  ☐labelStyle ☐labelPadding ☐deleteIconColor ☐deleteButtonTooltipMessage ☐side ☐shape ☐clipBehavior ☐autofocus ☐color ☐backgroundColor ☐padding ☐visualDensity ☐materialTapTargetSize ☐elevation ☐shadowColor ☐surfaceTintColor ☐avatarBoxConstraints ☐deleteIconBoxConstraints ☐⚡onDeleted ☐▢avatar ☐▢label ☐▢deleteIcon
- [ ] **ChoiceChip** (P3, core) — used on `ex-filters`, `cr-register`, `hm-viewing`, `maintainers (plugin)`, `mt-buttons` · all on `mt-buttons`
  ☐labelStyle ☐labelPadding ☐pressElevation ☐selected ☐selectedColor ☐disabledColor ☐tooltip ☐side ☐shape ☐clipBehavior ☐autofocus ☐color ☐backgroundColor ☐padding ☐visualDensity ☐materialTapTargetSize ☐elevation ☐shadowColor ☐surfaceTintColor ☐selectedShadowColor ☐showCheckmark ☐checkmarkColor ☐avatarBorder ☐avatarBoxConstraints ☐⚡onSelected ☐▢avatar ☐▢label
- [ ] **CircleAvatar** (P3, core) — used on `OrgCard`, `ex-org`, `ac-edit`, `dc-home` · all on `mt-paint`
  ☐backgroundColor ☐backgroundImage ☐foregroundImage ☐foregroundColor ☐radius ☐minRadius ☐maxRadius ☐⚡onBackgroundImageError ☐⚡onForegroundImageError ☐▢child
- [ ] **CircularProgressIndicator** (P3, core) — used on `SubmitBar`, `pay` · all on `mt-l2`
  ☐value ☐backgroundColor ☐color ☐strokeWidth ☐strokeAlign ☐semanticsLabel ☐semanticsValue ☐strokeCap ☐constraints ☐trackGap ☐padding ☐adaptive
- [ ] **ClipOval** (P3, core) — used on `cl-welcome`, `mt-layout` · all on `mt-layout`
  ☐clipBehavior ☐▢child
- [ ] **ClipRRect** (P3, core) — used on `hm-listing`, `mt-layout` · all on `mt-layout`
  ☐borderRadius ☐clipBehavior ☐▢child
- [ ] **CloseButton** (P3, core) — used on `ex-filters`, `vf-id`, `pay`, `mt-buttons` · all on `mt-buttons`
  ☐color ☐style ☐⚡onPressed
- [ ] **ColoredBox** (P3, core) — used on `home`, `inbox`, `mt-layout` · all on `mt-paint`
  ☐color ☐isAntiAlias ☐▢child
- [ ] **Column** (P3, core) — used on `BaseCard`, `HeroHeader`, `SectionHeader`, `StepHeader`, `FieldGroup`, `home` +25 · all on `mt-layout`
  ☐mainAxisAlignment ☐mainAxisSize ☐crossAxisAlignment ☐textDirection ☐verticalDirection ☐textBaseline ☐spacing
- [ ] **ConstrainedBox** (P3, core) — used on `ln-whats-new`, `mt-layout` · all on `mt-layout`
  ☐constraints ☐▢child
- [ ] **Container** (P3, core) — used on `ex-peek`, `hm-apply` · all on `mt-paint`
  ☐alignment ☐padding ☐color ☐isAntiAlias ☐decoration ☐foregroundDecoration ☐width ☐height ☐constraints ☐margin ☐transformAlignment ☐clipBehavior ☐▢child
- [ ] **CupertinoActivityIndicator** (P3, core) — used on `cl-slot`, `cl-queue` · all on `mt-l2`
  ☐color ☐animating ☐radius
- [ ] **CupertinoButton** (P3, core) — used on `cl-welcome`, `cl-patient`, `cl-slot`, `cl-symptoms`, `cl-confirm`, `mt-buttons` · all on `mt-buttons`
  ☐sizeStyle ☐padding ☐color ☐foregroundColor ☐disabledColor ☐minimumSize ☐pressedOpacity ☐borderRadius ☐alignment ☐focusColor ☐autofocus ☐⚡onFocusChange ☐⚡onLongPress ☐⚡onPressed ☐▢child
- [ ] **CupertinoContextMenu** (P5, plux_components) — used on `cl-confirm` · all on `mt-gestures`
  ☐actions ☐enableHapticFeedback ☐⚡onAction ☐▢child
- [ ] **CupertinoDatePicker** (P5, plux_components) — used on `cl-slot` · all on `mt-inputs`
  ☐mode ☐initialDateTime ☐minimumDate ☐maximumDate ☐minimumYear ☐maximumYear ☐minuteInterval ☐use24hFormat ☐backgroundColor ☐showDayOfWeek ☐showTimeSeparator ☐itemExtent ☐⚡onDateTimeChanged
- [ ] **CupertinoListSection** (P3, core) — used on `cl-welcome`, `cl-queue` · all on `mt-selection`
  ☐margin ☐backgroundColor ☐decoration ☐clipBehavior ☐dividerMargin ☐additionalDividerMargin ☐topMargin ☐hasLeading ☐separatorColor ☐▢header ☐▢footer ☐▢children
- [ ] **CupertinoListTile** (P3, core) — used on `cl-welcome` · all on `mt-selection`
  ☐backgroundColor ☐backgroundColorActivated ☐padding ☐leadingSize ☐leadingToTitle ☐⚡onTap ☐▢title ☐▢subtitle ☐▢additionalInfo ☐▢leading ☐▢trailing
- [ ] **CupertinoNavigationBar** (P4, core) — used on `cl-welcome`, `cl-queue` · all on `mt-nav`
  ☐automaticallyImplyLeading ☐automaticallyImplyMiddle ☐previousPageTitle ☐backgroundColor ☐automaticBackgroundVisibility ☐enableBackgroundFilterBlur ☐brightness ☐transitionBetweenRoutes ☐▢leading ☐▢middle ☐▢trailing
- [ ] **CupertinoPicker** (P5, plux_components) — used on `cr-wave` · all on `mt-inputs`
  ☐itemExtent ☐selectedIndex ☐diameterRatio ☐backgroundColor ☐offAxisFraction ☐useMagnifier ☐magnification ☐squeeze ☐looping ☐⚡onSelectedItemChanged
- [ ] **CupertinoSlider** (P3, core) — used on `cl-patient`, `mt-selection` · all on `mt-selection`
  ☐value ☐min ☐max ☐divisions ☐activeColor ☐thumbColor ☐⚡onChanged ☐⚡onChangeStart ☐⚡onChangeEnd
- [ ] **CupertinoSlidingSegmentedControl** (P3, core) — used on `cl-patient`, `mt-selection` · all on `mt-selection`
  ☐values ☐disabledChildren ☐groupValue ☐thumbColor ☐padding ☐backgroundColor ☐proportionalWidth ☐isMomentary ☐⚡onValueChanged ☐▢segments
- [ ] **CupertinoSwitch** (P3, core) — used on `cl-patient`, `mt-selection` · all on `mt-selection`
  ☐value ☐activeTrackColor ☐inactiveTrackColor ☐thumbColor ☐inactiveThumbColor ☐applyTheme ☐focusColor ☐onLabelColor ☐offLabelColor ☐activeThumbImage ☐inactiveThumbImage ☐trackOutlineColor ☐trackOutlineWidth ☐autofocus ☐dragStartBehavior ☐⚡onChanged ☐⚡onActiveThumbImageError ☐⚡onInactiveThumbImageError ☐⚡onFocusChange
- [ ] **CupertinoTabBar** (P4, core) — used on `cl-queue` · all on `mt-nav`
  ☐items ☐currentIndex ☐backgroundColor ☐activeColor ☐inactiveColor ☐iconSize ☐height ☐⚡onTap
- [ ] **CupertinoTextField** (P3, core) — used on `cl-patient`, `mt-inputs` · all on `mt-inputs`
  ☐value ☐decoration ☐padding ☐placeholder ☐placeholderStyle ☐prefixMode ☐suffixMode ☐crossAxisAlignment ☐clearButtonMode ☐clearButtonSemanticLabel ☐keyboardType ☐textInputAction ☐textCapitalization ☐style ☐textAlign ☐textAlignVertical ☐textDirection ☐readOnly ☐showCursor ☐autofocus ☐obscuringCharacter ☐obscureText ☐autocorrect ☐smartDashesType ☐smartQuotesType ☐enableSuggestions ☐maxLines ☐minLines ☐expands ☐maxLength ☐maxLengthEnforcement ☐enabled ☐cursorWidth ☐cursorHeight ☐cursorRadius ☐cursorOpacityAnimates ☐cursorColor ☐selectionHeightStyle ☐selectionWidthStyle ☐keyboardAppearance ☐scrollPadding ☐dragStartBehavior ☐enableInteractiveSelection ☐selectAllOnFocus ☐scrollPhysics ☐autofillHints ☐clipBehavior ☐stylusHandwritingEnabled ☐enableIMEPersonalizedLearning ☐enableInlinePrediction ☐⚡onChanged ☐⚡onEditingComplete ☐⚡onSubmitted ☐⚡onTapOutside ☐⚡onTapUpOutside ☐⚡onTap ☐▢prefix ☐▢suffix
- [ ] **CustomScrollView** (P3, core) — used on `home`, `ex-org`, `profile`, `pt-pitch`, `cp-home`, `dc-agenda` +3 · all on `mt-scroll`
  ☐scrollDirection ☐reverse ☐primary ☐physics ☐shrinkWrap ☐anchor ☐paintOrder ☐semanticChildCount ☐dragStartBehavior ☐keyboardDismissBehavior ☐clipBehavior ☐hitTestBehavior ☐▢slivers
- [ ] **DataScope** (P5, core) — used on `ex-org`, `cl-slot`, `mt-data` · all on `mt-data`
  ☐status ☐isEmpty ☐source ☐▢success ☐▢loading ☐▢empty ☐▢error
- [ ] **DataTable** (P5, plux_components) — used on `cs-submissions` · all on `mt-text`
  ☐columns ☐rows ☐selectedRows ☐sortColumnIndex ☐sortAscending ☐showCheckboxColumn ☐decoration ☐dataRowColor ☐dataRowMinHeight ☐dataRowMaxHeight ☐dataTextStyle ☐headingRowColor ☐headingRowHeight ☐headingTextStyle ☐horizontalMargin ☐columnSpacing ☐showBottomBorder ☐dividerThickness ☐checkboxHorizontalMargin ☐border ☐clipBehavior ☐⚡onSort ☐⚡onSelectChanged ☐⚡onSelectAll
- [ ] **DecoratedBox** (P3, core) — used on `HeroHeader`, `mt-layout` · all on `mt-paint`
  ☐decoration ☐position ☐▢child
- [ ] **Dismissible** (P5, core) — used on `inbox`, `passes`, `cp-education`, `dc-cart`, `ar-join`, `mt-gestures` · all on `mt-gestures`
  ☐direction ☐resizeDuration ☐movementDuration ☐crossAxisEndOffset ☐dragStartBehavior ☐behavior ☐⚡onDismissed ☐▢child ☐▢background ☐▢secondaryBackground
- [ ] **Divider** (P3, core) — used on `ps-receipt`, `pay` · all on `mt-layout`
  ☐height ☐thickness ☐indent ☐endIndent ☐color ☐radius
- [ ] **DragTarget** (P5, plux_components) — used on `hm-apply` · all on `mt-gestures`
  ☐accepts ☐highlightColor ☐hitTestBehavior ☐⚡onAccept ☐⚡onLeave ☐▢child
- [ ] **Draggable** (P5, plux_components) — used on `hm-apply`, `mt-gestures` · all on `mt-gestures`
  ☐data ☐axis ☐feedbackOffset ☐maxSimultaneousDrags ☐ignoringFeedbackSemantics ☐ignoringFeedbackPointer ☐rootOverlay ☐hitTestBehavior ☐⚡onDragStarted ☐⚡onDragEnd ☐⚡onDragCompleted ☐⚡onDraggableCanceled ☐▢child ☐▢feedback ☐▢childWhenDragging
- [ ] **Drawer** (P4, core) — used on `dc-home`, `mt-nav` · all on `mt-nav`
  ☐backgroundColor ☐elevation ☐shadowColor ☐surfaceTintColor ☐shape ☐width ☐semanticLabel ☐clipBehavior ☐▢child
- [ ] **DropdownMenu** (P3, core) — used on `ex-filters`, `pt-request`, `nb-details`, `cp-education`, `cr-register`, `mt-inputs` · all on `mt-inputs`
  ☐enabled ☐width ☐menuHeight ☐showTrailingIcon ☐hintText ☐helperText ☐errorText ☐enableFilter ☐enableSearch ☐keyboardType ☐textStyle ☐textAlign ☐initialSelection ☐requestFocusOnTap ☐selectOnly ☐expandedInsets ☐alignmentOffset ☐dropdownMenuEntries ☐closeBehavior ☐maxLines ☐textInputAction ☐cursorHeight ☐scrollPadding ☐⚡onSelected ☐▢leadingIcon ☐▢trailingIcon ☐▢label ☐▢selectedTrailingIcon
- [ ] **ElevatedButton** (P3, core) — used on `ps-receipt`, `mt-buttons` · all on `mt-buttons`
  ☐style ☐autofocus ☐clipBehavior ☐⚡onPressed ☐⚡onLongPress ☐⚡onHover ☐⚡onFocusChange ☐▢child
- [ ] **Expanded** (P3, core) — used on `SectionHeader`, `SubmitBar`, `MoneyRow`, `explore`, `ex-peek`, `vf-address` +7 · all on `mt-layout`
  ☐flex ☐▢child
- [ ] **ExpansionTile** (P3, core) — used on `pt-review`, `cs-submission`, `dc-agenda` · all on `mt-selection`
  ☐showTrailingIcon ☐initiallyExpanded ☐maintainState ☐tilePadding ☐expandedCrossAxisAlignment ☐expandedAlignment ☐childrenPadding ☐backgroundColor ☐collapsedBackgroundColor ☐textColor ☐collapsedTextColor ☐iconColor ☐collapsedIconColor ☐shape ☐collapsedShape ☐clipBehavior ☐controlAffinity ☐dense ☐splashColor ☐visualDensity ☐minTileHeight ☐enableFeedback ☐enabled ☐⚡onExpansionChanged ☐▢leading ☐▢title ☐▢subtitle ☐▢trailing ☐▢children
- [ ] **FadeTransition** (P5, core) — used on `home`, `mt-motion` · all on `mt-motion`
  ☐timeline ☐track ☐alwaysIncludeSemantics ☐▢child
- [ ] **FilledButton** (P3, core) — used on `SubmitBar`, `EmptyHint`, `not-found`, `ex-filters`, `ex-peek`, `ex-org` +28 · all on `mt-buttons`
  ☐style ☐autofocus ☐clipBehavior ☐⚡onPressed ☐⚡onLongPress ☐⚡onHover ☐⚡onFocusChange ☐▢child
- [ ] **FilterChip** (P3, core) — used on `ln-interests`, `dc-agenda`, `mt-buttons` · all on `mt-buttons`
  ☐labelStyle ☐labelPadding ☐selected ☐deleteIconColor ☐deleteButtonTooltipMessage ☐pressElevation ☐disabledColor ☐selectedColor ☐tooltip ☐side ☐shape ☐clipBehavior ☐autofocus ☐color ☐backgroundColor ☐padding ☐visualDensity ☐materialTapTargetSize ☐elevation ☐shadowColor ☐surfaceTintColor ☐selectedShadowColor ☐showCheckmark ☐checkmarkColor ☐avatarBorder ☐avatarBoxConstraints ☐deleteIconBoxConstraints ☐⚡onSelected ☐⚡onDeleted ☐▢avatar ☐▢label ☐▢deleteIcon
- [ ] **FittedBox** (P3, core) — used on `cr-bib`, `mt-layout` · all on `mt-layout`
  ☐fit ☐alignment ☐clipBehavior ☐▢child
- [ ] **Flex** (P3, core) — used on `pt-pitch`, `mt-layout` · all on `mt-layout`
  ☐direction ☐mainAxisAlignment ☐mainAxisSize ☐crossAxisAlignment ☐textDirection ☐verticalDirection ☐textBaseline ☐clipBehavior ☐spacing
- [ ] **Flexible** (P3, core) — used on `pt-pitch`, `nb-welcome`, `mt-layout` · all on `mt-layout`
  ☐flex ☐fit ☐▢child
- [ ] **FloatingActionButton** (P3, core) — used on `explore`, `ex-map`, `cr-live`, `dc-home`, `hm-listing`, `mt-buttons` · all on `mt-buttons`
  ☐tooltip ☐foregroundColor ☐backgroundColor ☐focusColor ☐hoverColor ☐splashColor ☐elevation ☐focusElevation ☐hoverElevation ☐highlightElevation ☐disabledElevation ☐mini ☐shape ☐clipBehavior ☐autofocus ☐materialTapTargetSize ☐isExtended ☐enableFeedback ☐extendedIconLabelSpacing ☐extendedPadding ☐extendedTextStyle ☐⚡onPressed ☐▢child ☐▢icon ☐▢label
- [ ] **ForEach** (P3, core) — used on `home`, `ln-interests`, `cp-education`, `cr-race`, `cr-register`, `dc-home` +3 · all on `mt-state`
  ☐items ☐▢item
- [ ] **FormScope** (P5, core) — used on `ac-edit`, `nb-details`, `ar-join` · all on `mt-state`
  ☐form ☐▢child
- [ ] **FractionallySizedBox** (P3, core) — used on `ln-welcome`, `mt-layout` · all on `mt-layout`
  ☐alignment ☐widthFactor ☐heightFactor ☐▢child
- [ ] **GestureDetector** (P3, core) — used on `cr-live`, `mt-gestures` · all on `mt-gestures`
  ☐behavior ☐excludeFromSemantics ☐dragStartBehavior ☐trackpadScrollCausesScale ☐trackpadScrollToScaleFactor ☐⚡onTapDown ☐⚡onTapUp ☐⚡onTap ☐⚡onTapMove ☐⚡onTapCancel ☐⚡onSecondaryTap ☐⚡onSecondaryTapDown ☐⚡onSecondaryTapUp ☐⚡onSecondaryTapCancel ☐⚡onTertiaryTapDown ☐⚡onTertiaryTapUp ☐⚡onTertiaryTapCancel ☐⚡onDoubleTapDown ☐⚡onDoubleTap ☐⚡onDoubleTapCancel ☐⚡onLongPressDown ☐⚡onLongPressCancel ☐⚡onLongPress ☐⚡onLongPressStart ☐⚡onLongPressMoveUpdate ☐⚡onLongPressUp ☐⚡onLongPressEnd ☐⚡onSecondaryLongPressDown ☐⚡onSecondaryLongPressCancel ☐⚡onSecondaryLongPress ☐⚡onSecondaryLongPressStart ☐⚡onSecondaryLongPressMoveUpdate ☐⚡onSecondaryLongPressUp ☐⚡onSecondaryLongPressEnd ☐⚡onTertiaryLongPressDown ☐⚡onTertiaryLongPressCancel ☐⚡onTertiaryLongPress ☐⚡onTertiaryLongPressStart ☐⚡onTertiaryLongPressMoveUpdate ☐⚡onTertiaryLongPressUp ☐⚡onTertiaryLongPressEnd ☐⚡onVerticalDragDown ☐⚡onVerticalDragStart ☐⚡onVerticalDragUpdate ☐⚡onVerticalDragEnd ☐⚡onVerticalDragCancel ☐⚡onHorizontalDragDown ☐⚡onHorizontalDragStart ☐⚡onHorizontalDragUpdate ☐⚡onHorizontalDragEnd ☐⚡onHorizontalDragCancel ☐⚡onForcePressStart ☐⚡onForcePressPeak ☐⚡onForcePressUpdate ☐⚡onForcePressEnd ☐⚡onPanDown ☐⚡onPanStart ☐⚡onPanUpdate ☐⚡onPanEnd ☐⚡onPanCancel ☐⚡onScaleStart ☐⚡onScaleUpdate ☐⚡onScaleEnd ☐▢child
- [ ] **GridView** (P3, core) — used on `ex-category`, `dc-home`, `dc-merch`, `hm-listings`, `mt-home`, `mt-scroll` · all on `mt-scroll`
  ☐items ☐scrollDirection ☐reverse ☐primary ☐physics ☐shrinkWrap ☐padding ☐addAutomaticKeepAlives ☐addRepaintBoundaries ☐addSemanticIndexes ☐semanticChildCount ☐dragStartBehavior ☐keyboardDismissBehavior ☐clipBehavior ☐hitTestBehavior ☐status ☐hasMore ☐crossAxisCount ☐maxCrossAxisExtent ☐mainAxisSpacing ☐crossAxisSpacing ☐childAspectRatio ☐mainAxisExtent ☐⚡onEndReached ☐▢item ☐▢empty ☐▢loading ☐▢error
- [ ] **Hero** (P5, core) — used on `ex-peek`, `ex-org`, `passes`, `pass`, `cr-bib`, `hm-listing` +1 · all on `mt-motion`
  ☐tag ☐transitionOnUserGestures ☐curve ☐reverseCurve ☐▢child
- [ ] **Icon** (P3, core) — used on `OrgCard`, `InfoRow`, `home`, `ln-welcome`, `ln-interests`, `inbox` +19 · all on `mt-text`
  ☐icon ☐size ☐fill ☐weight ☐grade ☐opticalSize ☐color ☐shadows ☐semanticLabel ☐textDirection ☐applyTextScaling ☐blendMode ☐fontWeight
- [ ] **IconButton** (P3, core) — used on `InfoRow`, `home`, `explore`, `ex-peek`, `ex-scan`, `passes` +8 · all on `mt-buttons`
  ☐iconSize ☐visualDensity ☐padding ☐alignment ☐splashRadius ☐color ☐focusColor ☐hoverColor ☐highlightColor ☐splashColor ☐disabledColor ☐autofocus ☐tooltip ☐enableFeedback ☐constraints ☐style ☐isSelected ☐⚡onPressed ☐⚡onHover ☐⚡onLongPress ☐▢selectedIcon ☐▢icon
- [ ] **If** (P3, core) — used on `BaseCard`, `OrgCard`, `HeroHeader`, `SectionHeader`, `FieldGroup`, `SubmitBar` +14 · all on `mt-state`
  ☐condition ☐▢then ☐▢else
- [ ] **Image** (P3, core) — used on `OrgCard`, `HeroHeader`, `nb-welcome`, `cr-race`, `cl-welcome`, `hm-listings` +2 · all on `mt-paint`
  ☐source ☐semanticLabel ☐excludeFromSemantics ☐scale ☐width ☐height ☐color ☐colorBlendMode ☐fit ☐alignment ☐repeat ☐matchTextDirection ☐gaplessPlayback ☐isAntiAlias ☐filterQuality ☐cacheWidth ☐cacheHeight ☐▢loading ☐▢error
- [ ] **IndexedStack** (P3, core) — used on `cp-programme`, `mt-layout` · all on `mt-layout`
  ☐alignment ☐textDirection ☐clipBehavior ☐sizing ☐index
- [ ] **InkWell** (P3, core) — used on `BaseCard`, `home`, `dc-home`, `mt-gestures` · all on `mt-gestures`
  ☐focusColor ☐hoverColor ☐highlightColor ☐overlayColor ☐splashColor ☐radius ☐borderRadius ☐customBorder ☐enableFeedback ☐excludeFromSemantics ☐canRequestFocus ☐autofocus ☐hoverDuration ☐⚡onTap ☐⚡onDoubleTap ☐⚡onLongPress ☐⚡onLongPressUp ☐⚡onTapDown ☐⚡onTapUp ☐⚡onTapCancel ☐⚡onSecondaryTap ☐⚡onSecondaryTapUp ☐⚡onSecondaryTapDown ☐⚡onSecondaryTapCancel ☐⚡onHighlightChanged ☐⚡onHover ☐⚡onFocusChange ☐▢child
- [ ] **InputChip** (P3, core) — used on `ex-org`, `rl-volunteer`, `mt-buttons` · all on `mt-buttons`
  ☐labelStyle ☐labelPadding ☐selected ☐isEnabled ☐deleteIconColor ☐deleteButtonTooltipMessage ☐pressElevation ☐disabledColor ☐selectedColor ☐tooltip ☐side ☐shape ☐clipBehavior ☐autofocus ☐color ☐backgroundColor ☐padding ☐visualDensity ☐materialTapTargetSize ☐elevation ☐shadowColor ☐surfaceTintColor ☐selectedShadowColor ☐showCheckmark ☐checkmarkColor ☐avatarBorder ☐avatarBoxConstraints ☐deleteIconBoxConstraints ☐⚡onSelected ☐⚡onDeleted ☐⚡onPressed ☐▢avatar ☐▢label ☐▢deleteIcon
- [ ] **InteractiveViewer** (P5, plux_components) — used on `hm-listing`, `mt-gestures` · all on `mt-gestures`
  ☐clipBehavior ☐panAxis ☐boundaryMargin ☐constrained ☐maxScale ☐minScale ☐interactionEndFrictionCoefficient ☐panEnabled ☐scaleEnabled ☐scaleFactor ☐alignment ☐trackpadScrollCausesScale ☐⚡onInteractionStart ☐⚡onInteractionEnd ☐▢child
- [ ] **LimitedBox** (P3, core) — used on `mt-layout` · all on `mt-layout`
  ☐maxWidth ☐maxHeight ☐▢child
- [ ] **LinearProgressIndicator** (P3, core) — used on `StepHeader`, `ps-live`, `cp-documents` · all on `mt-l2`
  ☐value ☐backgroundColor ☐color ☐minHeight ☐semanticsLabel ☐semanticsValue ☐borderRadius ☐stopIndicatorColor ☐stopIndicatorRadius ☐trackGap
- [ ] **ListTile** (P3, core) — used on `OrgCard`, `InfoRow`, `home`, `ln-language`, `inbox`, `ex-filters` +18 · all on `mt-selection`
  ☐isThreeLine ☐dense ☐visualDensity ☐shape ☐style ☐selectedColor ☐iconColor ☐textColor ☐titleTextStyle ☐subtitleTextStyle ☐leadingAndTrailingTextStyle ☐contentPadding ☐enabled ☐selected ☐focusColor ☐hoverColor ☐splashColor ☐autofocus ☐tileColor ☐selectedTileColor ☐enableFeedback ☐horizontalTitleGap ☐minVerticalPadding ☐minLeadingWidth ☐minTileHeight ☐titleAlignment ☐⚡onTap ☐⚡onLongPress ☐⚡onFocusChange ☐▢leading ☐▢title ☐▢subtitle ☐▢trailing
- [ ] **ListView** (P3, core) — used on `inbox`, `ex-filters`, `passes`, `ac-edit`, `ac-preferences`, `cs-dashboard` +13 · all on `mt-scroll`
  ☐items ☐scrollDirection ☐reverse ☐primary ☐physics ☐shrinkWrap ☐padding ☐itemExtent ☐addAutomaticKeepAlives ☐addRepaintBoundaries ☐addSemanticIndexes ☐semanticChildCount ☐dragStartBehavior ☐keyboardDismissBehavior ☐clipBehavior ☐hitTestBehavior ☐status ☐hasMore ☐⚡onEndReached ☐▢item ☐▢prototypeItem ☐▢empty ☐▢loading ☐▢error
- [ ] **Match** (P3, core) — used on `StatusChip`, `ex-org`, `pay`, `cl-queue`, `ar-join` · all on `mt-state`
  ☐value ☐cases ☐▢branches ☐▢otherwise
- [ ] **MaterialBanner** (P5, core) — used on `nb-eligibility` · all on `mt-l2`
  ☐contentTextStyle ☐elevation ☐backgroundColor ☐surfaceTintColor ☐shadowColor ☐dividerColor ☐padding ☐margin ☐leadingPadding ☐forceActionsBelow ☐overflowAlignment ☐minActionBarHeight ☐⚡onVisible ☐▢content ☐▢leading ☐▢actions
- [ ] **MenuAnchor** (P5, core) — used on `pass`, `mt-nav` · all on `mt-nav`
  ☐items ☐consumeOutsideTap ☐⚡onSelected ☐⚡onOpen ☐⚡onClose ☐▢child
- [ ] **NativeSlot** (P4, core) — used on `pass`, `ac-about`, `mt-native` · all on `mt-native`
  ☐name
- [ ] **NavigationBar** (P4, core) — used on `hm-listings`, `mt-nav` · all on `mt-nav`
  ☐destinations ☐selectedIndex ☐backgroundColor ☐elevation ☐shadowColor ☐surfaceTintColor ☐indicatorColor ☐height ☐labelBehavior ☐⚡onDestinationSelected
- [ ] **NavigationDrawer** (P4, core) — used on `cs-dashboard`, `mt-nav` · all on `mt-nav`
  ☐destinations ☐selectedIndex ☐backgroundColor ☐shadowColor ☐surfaceTintColor ☐elevation ☐indicatorColor ☐⚡onDestinationSelected ☐▢header ☐▢footer
- [ ] **NavigationRail** (P4, plux_components) — used on `cs-dashboard`, `mt-nav` · all on `mt-nav`
  ☐destinations ☐selectedIndex ☐extended ☐labelType ☐backgroundColor ☐elevation ☐groupAlignment ☐minWidth ☐minExtendedWidth ☐useIndicator ☐indicatorColor ☐⚡onDestinationSelected ☐▢leading ☐▢trailing
- [ ] **NestedScrollView** (P5, plux_components) — used on `ex-category` · all on `mt-scroll`
  ☐scrollDirection ☐reverse ☐physics ☐dragStartBehavior ☐floatHeaderSlivers ☐clipBehavior ☐hitTestBehavior ☐▢headerSlivers ☐▢body
- [ ] **Opacity** (P3, core) — used on `hm-apply`, `mt-layout` · all on `mt-paint`
  ☐opacity ☐alwaysIncludeSemantics ☐▢child
- [ ] **OutlinedButton** (P3, core) — used on `ex-peek`, `pass`, `pt-request`, `cs-submission`, `cp-education`, `cp-documents` +6 · all on `mt-buttons`
  ☐style ☐autofocus ☐clipBehavior ☐⚡onPressed ☐⚡onLongPress ☐⚡onHover ☐⚡onFocusChange ☐▢child
- [ ] **Padding** (P3, core) — used on `BaseCard`, `SectionHeader`, `SubmitBar`, `home`, `explore`, `ex-peek` +5 · all on `mt-layout`
  ☐padding ☐▢child
- [ ] **PageView** (P3, core) — used on `pt-pitch`, `mt-scroll` · all on `mt-scroll`
  ☐items ☐scrollDirection ☐reverse ☐physics ☐pageSnapping ☐dragStartBehavior ☐allowImplicitScrolling ☐clipBehavior ☐hitTestBehavior ☐padEnds ☐status ☐hasMore ☐⚡onPageChanged ☐⚡onEndReached ☐▢item ☐▢empty ☐▢loading ☐▢error
- [ ] **PopupMenuButton** (P5, core) — used on `inbox`, `hm-apply`, `mt-nav` · all on `mt-nav`
  ☐items ☐initialValue ☐tooltip ☐enabled ☐elevation ☐color ☐shadowColor ☐surfaceTintColor ☐iconSize ☐⚡onSelected ☐⚡onCanceled ☐⚡onOpened ☐▢child ☐▢icon
- [ ] **Positioned** (P3, core) — used on `OrgCard`, `HeroHeader`, `ex-map`, `ex-scan`, `ac-edit`, `vf-id` +4 · all on `mt-layout`
  ☐left ☐top ☐right ☐bottom ☐width ☐height ☐▢child
- [ ] **Radio** (P3, core) — used on `cl-symptoms`, `mt-selection` · all on `mt-selection`
  ☐value ☐groupValue ☐toggleable ☐activeColor ☐fillColor ☐focusColor ☐hoverColor ☐overlayColor ☐splashRadius ☐materialTapTargetSize ☐visualDensity ☐autofocus ☐enabled ☐backgroundColor ☐side ☐innerRadius ☐adaptive ☐useCupertinoCheckmarkStyle ☐⚡onChanged
- [ ] **RadioListTile** (P3, core) — used on `ln-language`, `ex-filters`, `nb-eligibility`, `cp-personal`, `dc-checkout`, `mt-selection` · all on `mt-selection`
  ☐value ☐groupValue ☐toggleable ☐activeColor ☐fillColor ☐hoverColor ☐overlayColor ☐splashRadius ☐materialTapTargetSize ☐isThreeLine ☐dense ☐selected ☐controlAffinity ☐autofocus ☐contentPadding ☐shape ☐tileColor ☐selectedTileColor ☐visualDensity ☐enableFeedback ☐horizontalTitleGap ☐minVerticalPadding ☐minLeadingWidth ☐minTileHeight ☐radioScaleFactor ☐titleAlignment ☐enabled ☐radioBackgroundColor ☐radioSide ☐radioInnerRadius ☐adaptive ☐useCupertinoCheckmarkStyle ☐⚡onChanged ☐⚡onFocusChange ☐▢title ☐▢subtitle ☐▢secondary
- [ ] **RangeSlider** (P3, core) — used on `ex-filters`, `rl-volunteer`, `hm-apply`, `mt-selection` · all on `mt-selection`
  ☐values ☐min ☐max ☐divisions ☐labels ☐activeColor ☐inactiveColor ☐overlayColor ☐padding ☐⚡onChanged ☐⚡onChangeStart ☐⚡onChangeEnd
- [ ] **RefreshIndicator** (P5, core) — used on `home`, `inbox`, `rl-history`, `mt-gestures` · all on `mt-gestures`
  ☐displacement ☐edgeOffset ☐color ☐backgroundColor ☐semanticsLabel ☐semanticsValue ☐strokeWidth ☐triggerMode ☐elevation ☐⚡onRefresh ☐▢child
- [ ] **ReorderableListView** (P5, plux_components) — used on `passes`, `cp-education` · all on `mt-scroll`
  ☐items ☐scrollDirection ☐reverse ☐physics ☐shrinkWrap ☐padding ☐buildDefaultDragHandles ☐itemExtent ☐anchor ☐dragStartBehavior ☐keyboardDismissBehavior ☐clipBehavior ☐autoScrollerVelocityScalar ☐status ☐⚡onReorder ☐⚡onReorderStart ☐⚡onReorderEnd ☐▢item ☐▢header ☐▢footer ☐▢empty ☐▢loading ☐▢error
- [ ] **Responsive** (P3, core) — used on `cs-dashboard`, `hm-listings`, `mt-responsive` · all on `mt-responsive`
  ☐▢compact ☐▢medium ☐▢expanded
- [ ] **RichText** (P3, core) — used on `ex-org`, `mt-text` · all on `mt-text`
  ☐textSpan ☐style ☐textAlign ☐textDirection ☐locale ☐softWrap ☐overflow ☐maxLines ☐semanticsLabel ☐semanticsIdentifier ☐textWidthBasis ☐selectionColor
- [ ] **RotationTransition** (P5, core) — used on `cr-race`, `mt-motion` · all on `mt-motion`
  ☐timeline ☐track ☐alignment ☐filterQuality ☐▢child
- [ ] **Row** (P3, core) — used on `BaseCard`, `SectionHeader`, `SubmitBar`, `InfoRow`, `MoneyRow`, `explore` +17 · all on `mt-layout`
  ☐mainAxisAlignment ☐mainAxisSize ☐crossAxisAlignment ☐textDirection ☐verticalDirection ☐textBaseline ☐spacing
- [ ] **SafeArea** (P3, core) — used on `SubmitBar`, `ln-welcome`, `ex-map`, `vf-id`, `mt-layout`, `mt-responsive` · all on `mt-layout`
  ☐left ☐top ☐right ☐bottom ☐minimum ☐maintainBottomViewPadding ☐▢child
- [ ] **Scaffold** (P3, core) — used on `home`, `ln-welcome`, `explore`, `passes`, `pass`, `pay` +9 · all on `mt-nav`
  ☐floatingActionButtonLocation ☐persistentFooterAlignment ☐persistentFooterDecoration ☐backgroundColor ☐resizeToAvoidBottomInset ☐primary ☐drawerDragStartBehavior ☐extendBody ☐drawerBarrierDismissible ☐extendBodyBehindAppBar ☐drawerScrimColor ☐drawerEdgeDragWidth ☐drawerEnableOpenDragGesture ☐endDrawerEnableOpenDragGesture ☐⚡onDrawerChanged ☐⚡onEndDrawerChanged ☐▢appBar ☐▢body ☐▢floatingActionButton ☐▢persistentFooterButtons ☐▢drawer ☐▢endDrawer ☐▢bottomNavigationBar ☐▢bottomSheet
- [ ] **ScaleTransition** (P5, core) — used on `home`, `pt-pitch`, `mt-motion` · all on `mt-motion`
  ☐timeline ☐track ☐alignment ☐filterQuality ☐▢child
- [ ] **Scrollbar** (P3, core) — used on `explore`, `mt-scroll` · all on `mt-scroll`
  ☐thumbVisibility ☐trackVisibility ☐thickness ☐radius ☐interactive ☐scrollbarOrientation ☐▢child
- [ ] **SearchAnchor** (P5, plux_components) — used on `explore`, `mt-inputs` · all on `mt-inputs`
  ☐suggestions ☐isFullScreen ☐viewHintText ☐hintText ☐⚡onSelected
- [ ] **SearchBar** (P5, plux_components) — used on `home`, `ex-map`, `mt-inputs` · all on `mt-inputs`
  ☐value ☐hintText ☐⚡onChanged ☐⚡onSubmitted ☐⚡onTap ☐▢leading ☐▢trailing
- [ ] **SegmentedButton** (P3, core) — used on `explore`, `ac-preferences`, `pt-request`, `nb-welcome`, `rl-donate`, `ar-join` +1 · all on `mt-buttons`
  ☐segments ☐selected ☐multiSelectionEnabled ☐emptySelectionAllowed ☐expandedInsets ☐style ☐showSelectedIcon ☐direction ☐⚡onSelectionChanged ☐▢selectedIcon
- [ ] **SelectableText** (P3, core) — used on `InfoRow`, `ps-receipt`, `pay`, `pt-sent`, `mt-text` · all on `mt-text`
  ☐data ☐style ☐textAlign ☐textDirection ☐showCursor ☐autofocus ☐minLines ☐maxLines ☐cursorWidth ☐cursorHeight ☐cursorRadius ☐cursorColor ☐selectionColor ☐selectionHeightStyle ☐selectionWidthStyle ☐dragStartBehavior ☐enableInteractiveSelection ☐scrollPhysics ☐semanticsLabel ☐textWidthBasis ☐⚡onTap
- [ ] **SingleChildScrollView** (P3, core) — used on `ln-interests`, `ln-whats-new`, `pass`, `cs-submissions`, `cr-register`, `ar-join` · all on `mt-scroll`
  ☐scrollDirection ☐reverse ☐padding ☐primary ☐physics ☐dragStartBehavior ☐clipBehavior ☐hitTestBehavior ☐keyboardDismissBehavior ☐▢child
- [ ] **SizedBox** (P3, core) — used on `home`, `cp-essay`, `cr-wave`, `cl-slot`, `mt-layout` · all on `mt-layout`
  ☐width ☐height ☐▢child
- [ ] **SlideTransition** (P5, core) — used on `cr-race`, `mt-motion` · all on `mt-motion`
  ☐timeline ☐track ☐axis ☐transformHitTests ☐textDirection ☐▢child
- [ ] **Slider** (P3, core) — used on `pt-request`, `nb-eligibility`, `cr-register`, `mt-selection`, `mt-motion` · all on `mt-selection`
  ☐value ☐secondaryTrackValue ☐min ☐max ☐divisions ☐label ☐activeColor ☐inactiveColor ☐secondaryActiveColor ☐thumbColor ☐overlayColor ☐autofocus ☐allowedInteraction ☐padding ☐showValueIndicator ☐adaptive ☐⚡onChanged ☐⚡onChangeStart ☐⚡onChangeEnd
- [ ] **SliverAppBar** (P3, core) — used on `HeroHeader`, `home`, `ex-category`, `ex-org`, `cp-home`, `dc-agenda` +3 · all on `mt-scroll`
  ☐automaticallyImplyLeading ☐automaticallyImplyActions ☐elevation ☐scrolledUnderElevation ☐shadowColor ☐surfaceTintColor ☐forceElevated ☐backgroundColor ☐foregroundColor ☐primary ☐centerTitle ☐excludeHeaderSemantics ☐titleSpacing ☐collapsedHeight ☐expandedHeight ☐floating ☐pinned ☐snap ☐stretch ☐stretchTriggerOffset ☐shape ☐toolbarHeight ☐leadingWidth ☐toolbarTextStyle ☐titleTextStyle ☐forceMaterialTransparency ☐useDefaultSemanticsOrder ☐clipBehavior ☐actionsPadding ☐⚡onStretchTrigger ☐▢leading ☐▢title ☐▢actions ☐▢flexibleSpace ☐▢bottom
- [ ] **SliverFillRemaining** (P3, core) — used on `home`, `pt-pitch`, `mt-scroll` · all on `mt-scroll`
  ☐hasScrollBody ☐fillOverscroll ☐▢child
- [ ] **SliverGrid** (P3, core) — used on `home`, `mt-scroll` · all on `mt-scroll`
  ☐items ☐addAutomaticKeepAlives ☐addRepaintBoundaries ☐addSemanticIndexes ☐semanticIndexOffset ☐status ☐hasMore ☐crossAxisCount ☐maxCrossAxisExtent ☐mainAxisSpacing ☐crossAxisSpacing ☐childAspectRatio ☐mainAxisExtent ☐⚡onEndReached ☐▢item ☐▢empty ☐▢loading ☐▢error
- [ ] **SliverList** (P3, core) — used on `cp-home`, `dc-agenda`, `mt-scroll` · all on `mt-scroll`
  ☐items ☐addAutomaticKeepAlives ☐addRepaintBoundaries ☐addSemanticIndexes ☐semanticIndexOffset ☐status ☐hasMore ☐⚡onEndReached ☐▢item ☐▢empty ☐▢loading ☐▢error
- [ ] **SliverPadding** (P3, core) — used on `home`, `mt-scroll` · all on `mt-scroll`
  ☐padding ☐▢sliver
- [ ] **SliverPersistentHeader** (P5, core) — used on `home`, `dc-agenda` · all on `mt-scroll`
  ☐pinned ☐floating ☐minExtent ☐maxExtent ☐▢child
- [ ] **SliverToBoxAdapter** (P3, core) — used on `home`, `ex-org`, `profile`, `pt-pitch`, `cp-home`, `rl-campaign` +1 · all on `mt-scroll`
  ☐▢child
- [ ] **Slot** (P3, core) — used on `BaseCard`, `OrgCard`, `HeroHeader`, `FieldGroup`, `cl-patient`, `cl-slot` · all on `mt-components`
  ☐name ☐▢fallback
- [ ] **Spacer** (P3, core) — used on `pay`, `mt-layout` · all on `mt-layout`
  ☐flex
- [ ] **Stack** (P3, core) — used on `OrgCard`, `HeroHeader`, `home`, `ex-map`, `ex-scan`, `ac-edit` +5 · all on `mt-layout`
  ☐alignment ☐textDirection ☐fit ☐clipBehavior
- [ ] **Stepper** (P5, plux_components) — used on `nb-identity`, `cp-review` · all on `mt-nav`
  ☐steps ☐type ☐currentStep ☐physics ☐elevation ☐margin ☐connectorColor ☐connectorThickness ☐stepIconHeight ☐stepIconWidth ☐stepIconMargin ☐clipBehavior ☐headerPadding ☐contentPadding ☐⚡onStepTapped ☐⚡onStepContinue ☐⚡onStepCancel
- [ ] **Switch** (P3, core) — used on `cl-symptoms`, `mt-selection` · all on `mt-selection`
  ☐value ☐activeThumbColor ☐activeTrackColor ☐inactiveThumbColor ☐inactiveTrackColor ☐activeThumbImage ☐inactiveThumbImage ☐thumbColor ☐trackColor ☐trackOutlineColor ☐trackOutlineWidth ☐materialTapTargetSize ☐dragStartBehavior ☐focusColor ☐hoverColor ☐overlayColor ☐splashRadius ☐autofocus ☐padding ☐adaptive ☐applyCupertinoTheme ☐⚡onChanged ☐⚡onActiveThumbImageError ☐⚡onInactiveThumbImageError ☐⚡onFocusChange
- [ ] **SwitchListTile** (P3, core) — used on `ex-filters`, `ac-preferences`, `ac-security`, `nb-details`, `cp-essay`, `rl-donate` +1 · all on `mt-selection`
  ☐value ☐activeThumbColor ☐activeTrackColor ☐inactiveThumbColor ☐inactiveTrackColor ☐activeThumbImage ☐inactiveThumbImage ☐thumbColor ☐trackColor ☐trackOutlineColor ☐materialTapTargetSize ☐dragStartBehavior ☐overlayColor ☐splashRadius ☐autofocus ☐tileColor ☐isThreeLine ☐dense ☐contentPadding ☐selected ☐controlAffinity ☐shape ☐selectedTileColor ☐visualDensity ☐enableFeedback ☐horizontalTitleGap ☐minVerticalPadding ☐minLeadingWidth ☐minTileHeight ☐hoverColor ☐adaptive ☐applyCupertinoTheme ☐⚡onChanged ☐⚡onActiveThumbImageError ☐⚡onInactiveThumbImageError ☐⚡onFocusChange ☐▢title ☐▢subtitle ☐▢secondary
- [ ] **TabBar** (P4, core) — used on `ex-category`, `passes`, `cp-programme`, `dc-agenda`, `mt-nav` · all on `mt-nav`
  ☐tabs ☐selectedIndex ☐isScrollable ☐indicatorColor ☐indicatorSize ☐dividerColor ☐labelColor ☐unselectedLabelColor ☐⚡onTap
- [ ] **TabBarView** (P4, core) — used on `ex-category`, `passes`, `mt-nav` · all on `mt-nav`
  ☐index ☐viewportFraction ☐⚡onIndexChanged
- [ ] **Table** (P5, plux_components) — used on `ps-receipt`, `cp-programme`, `rl-volunteer` · all on `mt-layout`
  ☐columns ☐columnWidths ☐defaultColumnWidth ☐textDirection ☐border ☐defaultVerticalAlignment ☐textBaseline
- [ ] **Text** (P3, core) — used on `HeroHeader`, `SectionHeader`, `StepHeader`, `FieldGroup`, `SubmitBar`, `InfoRow` +43 · all on `mt-text`
  ☐data ☐style ☐textAlign ☐textDirection ☐locale ☐softWrap ☐overflow ☐maxLines ☐semanticsLabel ☐semanticsIdentifier ☐textWidthBasis ☐selectionColor
- [ ] **TextButton** (P3, core) — used on `SectionHeader`, `SubmitBar`, `ln-welcome`, `ln-whats-new`, `ex-filters`, `ex-scan` +17 · all on `mt-buttons`
  ☐style ☐autofocus ☐clipBehavior ☐isSemanticButton ☐⚡onPressed ☐⚡onLongPress ☐⚡onHover ☐⚡onFocusChange ☐▢child
- [ ] **TextField** (P3, core) — used on `ps-feedback`, `ac-delete`, `pt-request`, `cp-home`, `cp-essay`, `rl-volunteer` +1 · all on `mt-inputs`
  ☐value ☐decoration ☐keyboardType ☐textInputAction ☐textCapitalization ☐style ☐textAlign ☐textAlignVertical ☐textDirection ☐readOnly ☐showCursor ☐autofocus ☐obscuringCharacter ☐obscureText ☐autocorrect ☐smartDashesType ☐smartQuotesType ☐enableSuggestions ☐maxLines ☐minLines ☐expands ☐maxLength ☐maxLengthEnforcement ☐enabled ☐ignorePointers ☐cursorWidth ☐cursorHeight ☐cursorRadius ☐cursorOpacityAnimates ☐cursorColor ☐cursorErrorColor ☐selectionHeightStyle ☐selectionWidthStyle ☐keyboardAppearance ☐scrollPadding ☐dragStartBehavior ☐enableInteractiveSelection ☐selectAllOnFocus ☐onTapAlwaysCalled ☐scrollPhysics ☐autofillHints ☐clipBehavior ☐stylusHandwritingEnabled ☐enableIMEPersonalizedLearning ☐enableInlinePrediction ☐canRequestFocus ☐hintLocales ☐⚡onChanged ☐⚡onEditingComplete ☐⚡onSubmitted ☐⚡onTap ☐⚡onTapOutside ☐⚡onTapUpOutside
- [ ] **TextFormField** (P3, core) — used on `ac-edit`, `pt-request`, `nb-details`, `cp-personal`, `cp-education`, `cr-register` +3 · all on `mt-inputs`
  ☐value ☐initialValue ☐forceErrorText ☐decoration ☐keyboardType ☐textCapitalization ☐textInputAction ☐style ☐textDirection ☐textAlign ☐textAlignVertical ☐autofocus ☐readOnly ☐showCursor ☐obscuringCharacter ☐obscureText ☐autocorrect ☐smartDashesType ☐smartQuotesType ☐enableSuggestions ☐maxLengthEnforcement ☐maxLines ☐minLines ☐expands ☐maxLength ☐onTapAlwaysCalled ☐enabled ☐ignorePointers ☐cursorWidth ☐cursorHeight ☐cursorRadius ☐cursorColor ☐cursorErrorColor ☐keyboardAppearance ☐scrollPadding ☐enableInteractiveSelection ☐selectAllOnFocus ☐scrollPhysics ☐autofillHints ☐autovalidateMode ☐enableIMEPersonalizedLearning ☐cursorOpacityAnimates ☐selectionHeightStyle ☐selectionWidthStyle ☐dragStartBehavior ☐clipBehavior ☐stylusHandwritingEnabled ☐canRequestFocus ☐hintLocales ☐⚡onChanged ☐⚡onTap ☐⚡onTapOutside ☐⚡onTapUpOutside ☐⚡onEditingComplete ☐⚡onFieldSubmitted ☐⚡onSaved
- [ ] **Timeline** (P5, core) — used on `cr-race`, `mt-motion` · all on `mt-motion`
  ☐timeline ☐autoplay ☐repeat ☐▢child
- [ ] **Tooltip** (P3, core) — used on `ex-org`, `hm-listing`, `mt-buttons` · all on `mt-buttons`
  ☐message ☐richMessage ☐constraints ☐padding ☐margin ☐verticalOffset ☐preferBelow ☐excludeFromSemantics ☐decoration ☐textStyle ☐textAlign ☐waitDuration ☐showDuration ☐exitDuration ☐enableTapToDismiss ☐triggerMode ☐enableFeedback ☐ignorePointer ☐⚡onTriggered ☐▢child
- [ ] **Transform** (P3, core) — used on `rl-campaign`, `mt-layout` · all on `mt-layout`
  ☐origin ☐alignment ☐transformHitTests ☐filterQuality ☐flipX ☐flipY ☐angle ☐scale ☐scaleX ☐scaleY ☐offset ☐▢child
- [ ] **VerticalDivider** (P3, core) — used on `hm-listings` · all on `mt-layout`
  ☐width ☐thickness ☐indent ☐endIndent ☐color ☐radius
- [ ] **Wrap** (P3, core) — used on `ln-interests`, `ex-filters`, `ex-org`, `cr-register`, `cl-slot`, `rl-volunteer` +5 · all on `mt-layout`
  ☐direction ☐alignment ☐spacing ☐runAlignment ☐runSpacing ☐crossAxisAlignment ☐textDirection ☐verticalDirection ☐clipBehavior

### L2. Layer 2

- [ ] **AccountCard** (P5, plux_components) — used on `nb-welcome` · all on `mt-l2`
  ☐name ☐maskedNumber ☐balance ☐flipped ☐backLabel ☐⚡onFlip
- [ ] **AmountInput** (P5, plux_components) — used on `nb-eligibility`, `nb-deposit`, `rl-donate`, `hm-apply` · all on `mt-l2`
  ☐label ☐currency ☐value ☐min ☐max ☐rangeMessage ☐⚡onChanged
- [ ] **AudioPlayer** (P5, plux_player) — used on `cl-symptoms`, `mt-l2` · all on `mt-l2`
  ☐url ☐title ☐autoplay ☐loop ☐playLabel ☐pauseLabel ☐positionLabel ☐⚡onEnded ☐⚡onError
- [ ] **BarChart** (P5, plux_charts) — used on `cs-dashboard` · all on `mt-l2`
  ☐series ☐labels ☐title ☐summaryTemplate ☐showTable
- [ ] **BarcodeScanner** (P5, plux_scanner) — used on `cs-checkin`, `mt-l2` · all on `mt-l2`
  ☐formats ☐torch ☐cameraFacing ☐scanWindow ☐continuous ☐overlayHint ☐semanticLabel ☐⚡onDetected ☐⚡onError
- [ ] **BiometricButton** (P6, core) — used on `vf-step-up`, `mt-l2` · all on `mt-l2`
  ☐label ☐reason ☐⚡onSuccess ☐⚡onFailure
- [ ] **Carousel** (P5, plux_components) — used on `ArWhySignIn`, `home`, `cr-race`, `dc-home`, `mt-l2` · all on `mt-l2`
  ☐autoplay ☐intervalMs ☐indicators ☐semanticLabel ☐⚡onPageChanged
- [ ] **CartSummary** (P5, plux_components) — used on `dc-cart`, `mt-l2` · all on `mt-l2`
  ☐lines ☐subtotal ☐tax ☐total ☐subtotalLabel ☐taxLabel ☐totalLabel ☐checkoutLabel ☐⚡onCheckout ☐▢checkout
- [ ] **ConsentCheckbox** (P5, plux_components) — used on `ac-privacy`, `nb-terms`, `ar-join`, `mt-l2` · all on `mt-l2`
  ☐label ☐documentVersion ☐linkLabel ☐value ☐⚡onChanged ☐⚡onAccepted ☐⚡onLinkTap
- [ ] **CountdownTimer** (P5, plux_components) — used on `ex-peek`, `cr-race`, `mt-l2` · all on `mt-l2`
  ☐endsAt ☐format ☐⚡onFinished
- [ ] **CountryPicker** (P5, plux_components) — used on `ac-edit`, `nb-details`, `cp-personal` · all on `mt-l2`
  ☐value ☐searchHint ☐⚡onSelected
- [ ] **CurrencyPicker** (P5, plux_components) — used on `nb-deposit`, `rl-donate` · all on `mt-l2`
  ☐value ☐currencies ☐searchHint ☐⚡onSelected
- [ ] **DualCalendarDatePicker** (P8, core) — used on `ac-edit`, `nb-details`, `cp-personal`, `rl-volunteer`, `ar-join`, `mt-l10n` +1 · all on `mt-l2`
  ☐value ☐firstDate ☐lastDate ☐calendar ☐secondaryCalendar ☐⚡onChanged
- [ ] **EmptyState** (P3, core) — used on `EmptyHint`, `not-found`, `mt-l2` · all on `mt-l2`
  ☐icon ☐title ☐message ☐▢action
- [ ] **ErrorState** (P3, core) — used on `ErrorBlock`, `ln-offline`, `explore`, `mt-l2` · all on `mt-l2`
  ☐icon ☐title ☐message ☐retryLabel ☐⚡onRetry ☐▢action
- [ ] **FileUploader** (P5, plux_components) — used on `cp-documents` · all on `mt-l2`
  ☐dataSource ☐operation ☐accept ☐maxBytes ☐label ☐uploadLabel ☐⚡onUploaded ☐⚡onFailed
- [ ] **FilterBar** (P5, plux_components) — used on `explore`, `cs-submissions`, `mt-l2` · all on `mt-l2`
  ☐filters ☐⚡onToggle
- [ ] **IbanInput** (P5, plux_components) — used on `nb-details`, `rl-campaign`, `hm-apply`, `ar-join` · all on `mt-l2`
  ☐label ☐value ☐invalidMessage ☐⚡onChanged
- [ ] **ImageGallery** (P5, plux_components) — used on `dc-merch`, `hm-listing`, `mt-l2` · all on `mt-l2`
  ☐images ☐initialPage ☐indicators ☐semanticLabel ☐⚡onPageChanged
- [ ] **InfiniteList** (P5, plux_components) — used on `explore`, `mt-l2` · all on `mt-l2`
  ☐items ☐status ☐hasMore ☐padding ☐⚡onEndReached ☐▢item ☐▢empty ☐▢loading ☐▢error
- [ ] **KycCameraCapture** (P6, core) — used on `vf-id`, `vf-selfie`, `mt-l2` · all on `mt-l2`
  ☐mode ☐⚡onCaptured
- [ ] **LineChart** (P5, plux_charts) — used on `cs-dashboard`, `cr-route`, `rl-campaign` · all on `mt-l2`
  ☐series ☐labels ☐title ☐summaryTemplate ☐showTable
- [ ] **LiveEtaCard** (P5, plux_components) — used on `ps-live`, `cr-live`, `mt-l2` · all on `mt-l2`
  ☐title ☐etaMinutes ☐status ☐minutesLabel ☐icon
- [ ] **LottieView** (P5, plux_lottie) — used on `ln-welcome`, `pay`, `pt-sent`, `nb-done`, `rl-thanks`, `mt-motion` +1 · all on `mt-l2`
  ☐url ☐animate ☐repeat ☐reverse ☐progress ☐fit ☐width ☐height ☐reduceMotion ☐semanticLabel ☐⚡onCompleted
- [ ] **MapView** (P5, plux_maps) — used on `home`, `ex-map`, `vf-address`, `cr-route`, `cl-queue`, `hm-listings` +1 · all on `mt-l2`
  ☐semanticLabel ☐center ☐zoom ☐minZoom ☐maxZoom ☐markers ☐polylines ☐tileUrlTemplate ☐attribution ☐showUserLocation ☐interactive ☐⚡onMarkerTap ☐⚡onMapTap ☐⚡onCameraIdle
- [ ] **MarkdownView** (P5, plux_markdown) — used on `ln-whats-new`, `ex-org`, `cp-programme`, `cp-essay` · all on `mt-l2`
  ☐source ☐⚡onLink
- [ ] **MaskedText** (P5, plux_components) — used on `pass`, `mt-secure` · all on `mt-l2`
  ☐value ☐mask ☐visibleDigits ☐revealable ☐revealLabel ☐⚡onReveal
- [ ] **MoneyText** (P5, plux_components) — used on `OrgCard`, `MoneyRow`, `ps-receipt`, `pay`, `cs-dashboard`, `nb-deposit` +5 · all on `mt-l2`
  ☐value ☐display ☐signDisplay ☐style
- [ ] **OfflineBanner** (P3, core) — used on `home`, `mt-l2` · all on `mt-l2`
  ☐message ☐visible
- [ ] **OnboardingPager** (P5, plux_components) — used on `ln-welcome`, `mt-l2` · all on `mt-l2`
  ☐currentPage ☐nextLabel ☐doneLabel ☐⚡onPageChanged ☐⚡onDone
- [ ] **OrderTrackerMap** (P5, plux_maps) — used on `cr-live`, `mt-l2` · all on `mt-l2`
  ☐semanticLabel ☐origin ☐destination ☐courier ☐route ☐etaLabel ☐follow ☐tileUrlTemplate ☐attribution ☐originLabel ☐destinationLabel ☐courierLabel
- [ ] **OtpInput** (P6, core) — used on `vf-otp`, `mt-l2` · all on `mt-l2`
  ☐length ☐alphabet ☐autofill ☐⚡onCompleted
- [ ] **PaymentCard** (P5, plux_components) — used on `rl-donate`, `mt-l2` · all on `mt-l2`
  ☐brand ☐maskedNumber ☐expiry ☐holder ☐flipped ☐backLabel ☐⚡onFlip
- [ ] **PhoneInput** (P5, plux_components) — used on `ac-edit`, `pt-request`, `cr-register`, `ar-join` · all on `mt-l2`
  ☐label ☐value ☐region ☐invalidMessage ☐⚡onChanged
- [ ] **PieChart** (P5, plux_charts) — used on `cs-dashboard`, `rl-campaign` · all on `mt-l2`
  ☐slices ☐title ☐summaryTemplate ☐showTable
- [ ] **PriceTag** (P5, plux_components) — used on `dc-tickets`, `mt-l2` · all on `mt-l2`
  ☐price ☐originalPrice ☐wasLabel
- [ ] **ProductCard** (P5, plux_components) — used on `dc-tickets`, `dc-merch`, `mt-l2` · all on `mt-l2`
  ☐image ☐title ☐price ☐originalPrice ☐badge ☐addToCartLabel ☐⚡onTap ☐⚡onAddToCart
- [ ] **ProfileHeader** (P5, plux_components) — used on `profile`, `mt-l2` · all on `mt-l2`
  ☐name ☐subtitle ☐avatar ☐radius ☐▢actions
- [ ] **PromoBanner** (P5, plux_components) — used on `home`, `dc-home` · all on `mt-l2`
  ☐title ☐subtitle ☐image ☐ctaLabel ☐endsAt ☐⚡onTap
- [ ] **QrCodeView** (P5, plux_scanner) — used on `pass`, `cr-bib`, `dc-badge`, `ar-welcome`, `mt-l2` · all on `mt-l2`
  ☐data ☐size ☐errorCorrection ☐foreground ☐background ☐quietZone ☐semanticLabel
- [ ] **QrScanner** (P5, plux_scanner) — used on `ex-scan`, `mt-l2` · all on `mt-l2`
  ☐torch ☐cameraFacing ☐scanWindow ☐continuous ☐overlayHint ☐semanticLabel ☐⚡onDetected ☐⚡onError
- [ ] **QuantityStepper** (P5, plux_components) — used on `dc-tickets`, `dc-cart`, `mt-l2` · all on `mt-l2`
  ☐value ☐min ☐max ☐step ☐decreaseLabel ☐increaseLabel ☐⚡onChanged
- [ ] **RatingBar** (P5, plux_components) — used on `hm-listing`, `mt-l2` · all on `mt-l2`
  ☐value ☐max ☐ofLabel ☐semanticLabel
- [ ] **RatingInput** (P5, plux_components) — used on `ps-feedback`, `cl-symptoms`, `mt-l2` · all on `mt-l2`
  ☐value ☐max ☐starLabel ☐⚡onChanged
- [ ] **RiveView** (P5, plux_rive) — used on `pt-pitch`, `rl-campaign`, `mt-motion`, `mt-l2` · all on `mt-l2`
  ☐url ☐artboard ☐stateMachine ☐inputs ☐flags ☐triggers ☐fit ☐reduceMotion ☐semanticLabel ☐⚡onLoaded ☐⚡onFailed
- [ ] **SearchField** (P5, plux_components) — used on `cs-submissions`, `mt-l2` · all on `mt-l2`
  ☐value ☐hint ☐debounceMs ☐suggestions ☐⚡onSearch ☐⚡onSuggestionSelected
- [ ] **SecurePinPad** (P6, core) — used on `ac-security`, `pay-pin`, `nb-pin`, `mt-secure` · all on `mt-l2`
  ☐length ☐randomizeLayout ☐haptics ☐⚡onCompleted
- [ ] **SecureTextField** (P6, core) — used on `nb-details`, `mt-secure` · all on `mt-l2`
  ☐value ☐label ☐obscure ☐maxLength ☐⚡onChanged ☐⚡onSubmitted
- [ ] **SettingsList** (P5, plux_components) — used on `profile`, `mt-l2` · all on `mt-l2`
  ☐sections
- [ ] **SignaturePad** (P5, plux_media) — used on `vf-signature`, `mt-l2` · all on `mt-l2`
  ☐format ☐strokeWidth ☐height ☐clearLabel ☐semanticLabel ☐⚡onChanged
- [ ] **SkeletonLoader** (P3, core) — used on `OrgCard`, `explore`, `ex-org`, `mt-l2` · all on `mt-l2`
  ☐lines ☐avatar ☐shimmer
- [ ] **SortSheet** (P5, plux_components) — used on `cs-submissions`, `dc-merch`, `mt-l2` · all on `mt-l2`
  ☐title ☐options ☐selected ☐⚡onSelected
- [ ] **Sparkline** (P5, plux_charts) — used on `ps-live`, `cs-dashboard`, `rl-campaign` · all on `mt-l2`
  ☐values ☐semanticLabel ☐summaryTemplate
- [ ] **StatusTimeline** (P5, plux_components) — used on `pass`, `mt-l2` · all on `mt-l2`
  ☐steps
- [ ] **TermsViewer** (P5, plux_markdown) — used on `ac-privacy`, `nb-terms`, `hm-lease`, `mt-l2` · all on `mt-l2`
  ☐source ☐version ☐acceptLabel ☐scrollHint ☐⚡onAccepted ☐⚡onLink
- [ ] **TransactionListItem** (P5, plux_components) — used on `ps-receipt`, `rl-history`, `mt-l2` · all on `mt-l2`
  ☐title ☐subtitle ☐amount ☐status ☐date ☐statusLabel ☐⚡onTap
- [ ] **VideoPlayer** (P5, plux_player) — used on `pt-pitch`, `cl-welcome`, `mt-l2` · all on `mt-l2`
  ☐url ☐captions ☐autoplay ☐loop ☐aspectRatio ☐semanticLabel ☐playLabel ☐pauseLabel ☐captionsLabel ☐positionLabel ☐⚡onEnded ☐⚡onError
- [ ] **WizardForm** (P5, plux_components) — used on `pt-request`, `mt-l2` · all on `mt-l2`
  ☐titles ☐currentStep ☐stepFields ☐backLabel ☐nextLabel ☐submitLabel ☐⚡onNext ☐⚡onBack ☐⚡onSubmit

## Appendix M — Every enum value

One ☐ per value. Real screens use some values; the Maintainers screen named shows **every** value as its own sample (or through a variant picker). Users: the widgets/types/actions that take the enum.

- [ ] **AnimationCommand** on `mt-motion` · users: controlAnimation
  ☐play ☐pause ☐reverse ☐seek ☐stop
- [ ] **Assertiveness** on `mt-actions` · users: announce
  ☐polite ☐assertive
- [ ] **AutovalidateMode** on `mt-inputs` · users: TextFormField
  ☐disabled ☐always ☐onUserInteraction ☐onUnfocus ☐onUserInteractionIfError
- [ ] **Axis** on `mt-layout` · users: CustomScrollView, Draggable, Flex, GridView, ListView, NestedScrollView, PageView, ReorderableListView, SegmentedButton, SingleChildScrollView, SlideT
  ☐horizontal ☐vertical
- [ ] **BarcodeFormat** on `mt-actions` · users: ScanResult, scanCode
  ☐qrCode ☐aztec ☐dataMatrix ☐pdf417 ☐code39 ☐code93 ☐code128 ☐codabar ☐ean8 ☐ean13 ☐itf ☐upcA ☐upcE
- [ ] **BlendMode** on `mt-paint` · users: BackdropFilter, BoxDecoration, Icon, Image
  ☐clear ☐src ☐dst ☐srcOver ☐dstOver ☐srcIn ☐dstIn ☐srcOut ☐dstOut ☐srcATop ☐dstATop ☐xor ☐plus ☐modulate ☐screen ☐overlay ☐darken ☐lighten ☐colorDodge ☐colorBurn ☐hardLight ☐softLight ☐difference ☐exclusion ☐multiply ☐hue ☐saturation ☐color ☐luminosity
- [ ] **BlurStyle** on `mt-paint` · users: BoxShadow
  ☐normal ☐solid ☐outer ☐inner
- [ ] **BorderStyle** on `mt-paint` · users: BorderSide
  ☐none ☐solid
- [ ] **BottomNavigationBarType** on `mt-nav` · users: BottomNavigationBar
  ☐fixed ☐shifting
- [ ] **BoxFit** on `mt-layout` · users: FittedBox, Image
  ☐fill ☐contain ☐cover ☐fitWidth ☐fitHeight ☐none ☐scaleDown
- [ ] **BoxHeightStyle** on `mt-text` · users: CupertinoTextField, SelectableText, TextField, TextFormField
  ☐tight ☐max ☐includeLineSpacingMiddle ☐includeLineSpacingTop ☐includeLineSpacingBottom ☐strut
- [ ] **BoxShape** on `mt-paint` · users: BoxDecoration
  ☐rectangle ☐circle
- [ ] **BoxWidthStyle** on `mt-text` · users: CupertinoTextField, SelectableText, TextField, TextFormField
  ☐tight ☐max
- [ ] **Brightness** on `mt-inputs` · users: CupertinoNavigationBar, CupertinoTextField, TextField, TextFormField
  ☐dark ☐light
- [ ] **CalendarSystem** on `mt-l10n` · users: DualCalendarDatePicker
  ☐gregorian ☐ethiopic ☐islamicUmalqura ☐persian
- [ ] **CameraFacing** on `mt-l2` · users: BarcodeScanner, QrScanner
  ☐back ☐front
- [ ] **CardBrand** on `mt-l2` · users: PaymentCard
  ☐visa ☐mastercard ☐amex ☐discover ☐other
- [ ] **Clip** on `mt-layout` · users: ActionChip, AnimatedContainer, AnimatedCrossFade, AnimatedSize, AppBar, BottomAppBar, Card, Chip, ChoiceChip, ClipOval, ClipRRect, Container, Cupertin
  ☐none ☐hardEdge ☐antiAlias ☐antiAliasWithSaveLayer
- [ ] **ColumnWidthKind** on `mt-layout` · users: ColumnWidth
  ☐flex ☐fixed ☐fraction ☐intrinsic
- [ ] **CountdownFormat** on `mt-l2` · users: CountdownTimer
  ☐hms ☐dhms ☐compact
- [ ] **CrossAxisAlignment** on `mt-layout` · users: Column, CupertinoTextField, ExpansionTile, Flex, Row
  ☐start ☐end ☐center ☐stretch ☐baseline
- [ ] **CrossFadeState** on `mt-motion` · users: AnimatedCrossFade
  ☐showFirst ☐showSecond
- [ ] **CupertinoButtonSize** on `mt-buttons` · users: CupertinoButton
  ☐small ☐medium ☐large
- [ ] **CupertinoDatePickerMode** on `mt-inputs` · users: CupertinoDatePicker
  ☐time ☐date ☐dateAndTime ☐monthYear
- [ ] **Curve** on `mt-motion` · users: AnimatedAlign, AnimatedContainer, AnimatedCrossFade, AnimatedOpacity, AnimatedPadding, AnimatedPositioned, AnimatedRotation, AnimatedScale, AnimatedSi
  ☐linear ☐easeIn ☐easeOut ☐easeInOut ☐fastOutSlowIn ☐decelerate ☐bounceIn ☐bounceOut ☐elasticOut ☐overshoot
- [ ] **DecorationPosition** on `mt-layout` · users: DecoratedBox
  ☐background ☐foreground
- [ ] **DismissDirection** on `mt-gestures` · users: Dismissible
  ☐vertical ☐horizontal ☐endToStart ☐startToEnd ☐up ☐down ☐none
- [ ] **DragStartBehavior** on `mt-scroll` · users: CupertinoSwitch, CupertinoTextField, CustomScrollView, Dismissible, GestureDetector, GridView, ListView, NestedScrollView, PageView, ReorderableListVi
  ☐down ☐start
- [ ] **DropdownMenuCloseBehavior** on `mt-inputs` · users: DropdownMenu
  ☐all ☐self ☐none
- [ ] **FilterQuality** on `mt-paint` · users: AnimatedRotation, AnimatedScale, Image, RotationTransition, ScaleTransition, Transform
  ☐none ☐low ☐medium ☐high
- [ ] **FlexFit** on `mt-layout` · users: Flexible
  ☐tight ☐loose
- [ ] **FloatingActionButtonLocation** on `mt-buttons` · users: Scaffold
  ☐startTop ☐miniStartTop ☐centerTop ☐miniCenterTop ☐endTop ☐miniEndTop ☐startFloat ☐miniStartFloat ☐centerFloat ☐miniCenterFloat ☐endFloat ☐miniEndFloat ☐startDocked ☐miniStartDocked ☐centerDocked ☐miniCenterDocked ☐endDocked ☐miniEndDocked ☐endContained
- [ ] **FloatingLabelAlignment** on `mt-inputs` · users: InputDecoration
  ☐start ☐center
- [ ] **FloatingLabelBehavior** on `mt-inputs` · users: InputDecoration
  ☐never ☐auto ☐always
- [ ] **FontStyle** on `mt-text` · users: TextStyle
  ☐normal ☐italic
- [ ] **FontWeight** on `mt-text` · users: Icon, TextStyle
  ☐w100 ☐w200 ☐w300 ☐w400 ☐w500 ☐w600 ☐w700 ☐w800 ☐w900
- [ ] **GradientKind** on `mt-paint` · users: Gradient
  ☐linear ☐radial ☐sweep
- [ ] **GuardDecision** on `mt-nav` · users: GuardResult
  ☐allow ☐redirect ☐fallback
- [ ] **HapticPattern** on `mt-actions` · users: haptic
  ☐light ☐medium ☐heavy ☐selection ☐vibrate
- [ ] **HitTestBehavior** on `mt-scroll` · users: CustomScrollView, Dismissible, DragTarget, Draggable, GestureDetector, GridView, ListView, NestedScrollView, PageView, SingleChildScrollView
  ☐deferToChild ☐opaque ☐translucent
- [ ] **IconAlignment** on `mt-buttons` · users: ButtonStyle
  ☐start ☐end
- [ ] **IconSet** on `mt-text` · users: IconData
  ☐material ☐cupertino
- [ ] **ImageRepeat** on `mt-paint` · users: Image
  ☐repeat ☐repeatX ☐repeatY ☐noRepeat
- [ ] **InputBorderKind** on `mt-inputs` · users: InputBorder
  ☐outline ☐underline ☐none
- [ ] **KycCaptureMode** on `mt-l2` · users: KycCameraCapture
  ☐document ☐selfie
- [ ] **ListStatus** on `mt-scroll` · users: DataScope, GridView, InfiniteList, ListView, PageView, ReorderableListView, SliverGrid, SliverList
  ☐ready ☐loading ☐error
- [ ] **ListTileControlAffinity** on `mt-selection` · users: CheckboxListTile, ExpansionTile, RadioListTile, SwitchListTile
  ☐leading ☐trailing ☐platform
- [ ] **ListTileStyle** on `mt-selection` · users: ListTile
  ☐list ☐drawer
- [ ] **ListTileTitleAlignment** on `mt-selection` · users: CheckboxListTile, ListTile, RadioListTile
  ☐threeLine ☐titleHeight ☐top ☐center ☐bottom
- [ ] **LocationAccuracy** on `mt-actions` · users: getLocation
  ☐low ☐balanced ☐high
- [ ] **MainAxisAlignment** on `mt-layout` · users: Column, Flex, Row
  ☐start ☐end ☐center ☐spaceBetween ☐spaceAround ☐spaceEvenly
- [ ] **MainAxisSize** on `mt-layout` · users: Column, Flex, Row
  ☐min ☐max
- [ ] **MaskKind** on `mt-secure` · users: MaskedText
  ☐card ☐account ☐phone ☐custom
- [ ] **MaterialTapTargetSize** on `mt-buttons` · users: ActionChip, ButtonStyle, Checkbox, CheckboxListTile, Chip, ChoiceChip, FilterChip, FloatingActionButton, InputChip, Radio, RadioListTile, Switch, Swit
  ☐padded ☐shrinkWrap
- [ ] **MaxLengthEnforcement** on `mt-inputs` · users: CupertinoTextField, TextField, TextFormField
  ☐none ☐enforced ☐truncateAfterCompositionEnds
- [ ] **MoneyDisplay** on `mt-l2` · users: MoneyText
  ☐symbol ☐narrowSymbol ☐code
- [ ] **NavigationDestinationLabelBehavior** on `mt-nav` · users: NavigationBar
  ☐alwaysShow ☐alwaysHide ☐onlyShowSelected
- [ ] **NavigationMode** on `mt-nav` · users: navigate
  ☐push ☐replace ☐popUntil ☐clearAndPush
- [ ] **NavigationRailLabelType** on `mt-nav` · users: NavigationRail
  ☐none ☐selected ☐all
- [ ] **OtpAlphabet** on `mt-l2` · users: OtpInput
  ☐numeric ☐alphanumeric
- [ ] **OverflowBarAlignment** on `mt-l2` · users: MaterialBanner
  ☐start ☐end ☐center
- [ ] **OverlayVisibilityMode** on `mt-inputs` · users: CupertinoTextField
  ☐never ☐editing ☐notEditing ☐always
- [ ] **PanAxis** on `mt-gestures` · users: InteractiveViewer
  ☐free ☐horizontal ☐vertical ☐aligned
- [ ] **QrErrorCorrection** on `mt-l2` · users: QrCodeView
  ☐low ☐medium ☐quartile ☐high
- [ ] **RefreshIndicatorTriggerMode** on `mt-gestures` · users: RefreshIndicator
  ☐anywhere ☐onEdge
- [ ] **ScrollPhysics** on `mt-scroll` · users: CupertinoTextField, CustomScrollView, GridView, ListView, NestedScrollView, PageView, ReorderableListView, SelectableText, SingleChildScrollView, Step
  ☐platform ☐bouncing ☐clamping ☐never ☐always ☐page
- [ ] **ScrollViewKeyboardDismissBehavior** on `mt-scroll` · users: CustomScrollView, GridView, ListView, ReorderableListView, SingleChildScrollView
  ☐manual ☐onDrag
- [ ] **ScrollbarOrientation** on `mt-scroll` · users: Scrollbar
  ☐left ☐right ☐top ☐bottom
- [ ] **ShapeKind** on `mt-paint` · users: ShapeBorder
  ☐roundedRectangle ☐stadium ☐circle ☐beveledRectangle ☐continuousRectangle
- [ ] **ShowValueIndicator** on `mt-selection` · users: Slider
  ☐onlyForDiscrete ☐onlyForContinuous ☐always ☐onDrag ☐alwaysVisible ☐never
- [ ] **SignDisplay** on `mt-l2` · users: MoneyText
  ☐auto ☐always ☐never ☐exceptZero
- [ ] **SliderInteraction** on `mt-selection` · users: Slider
  ☐tapAndSlide ☐tapOnly ☐slideOnly ☐slideThumb
- [ ] **SliverPaintOrder** on `mt-scroll` · users: CustomScrollView
  ☐firstIsTop ☐lastIsTop
- [ ] **SmartDashesType** on `mt-inputs` · users: CupertinoTextField, TextField, TextFormField
  ☐disabled ☐enabled
- [ ] **SmartQuotesType** on `mt-inputs` · users: CupertinoTextField, TextField, TextFormField
  ☐disabled ☐enabled
- [ ] **StackFit** on `mt-layout` · users: IndexedStack, Stack
  ☐loose ☐expand ☐passthrough
- [ ] **StepState** on `mt-nav` · users: StepItem
  ☐indexed ☐editing ☐complete ☐disabled ☐error
- [ ] **StepperType** on `mt-nav` · users: Stepper
  ☐vertical ☐horizontal
- [ ] **StrokeCap** on `mt-l2` · users: CircularProgressIndicator
  ☐butt ☐round ☐square
- [ ] **TabBarIndicatorSize** on `mt-nav` · users: TabBar
  ☐tab ☐label
- [ ] **TableCellVerticalAlignment** on `mt-layout` · users: Table
  ☐top ☐middle ☐bottom ☐baseline ☐fill ☐intrinsicHeight
- [ ] **TextAlign** on `mt-text` · users: CupertinoTextField, DropdownMenu, RichText, SelectableText, Text, TextField, TextFormField, Tooltip
  ☐left ☐right ☐center ☐justify ☐start ☐end
- [ ] **TextAlignVertical** on `mt-inputs` · users: CupertinoTextField, TextField, TextFormField
  ☐top ☐center ☐bottom
- [ ] **TextBaseline** on `mt-text` · users: Baseline, Column, Flex, Row, Table, TextStyle
  ☐alphabetic ☐ideographic
- [ ] **TextCapitalization** on `mt-inputs` · users: CupertinoTextField, TextField, TextFormField
  ☐words ☐sentences ☐characters ☐none
- [ ] **TextDecoration** on `mt-text` · users: TextStyle
  ☐none ☐underline ☐overline ☐lineThrough
- [ ] **TextDecorationStyle** on `mt-text` · users: TextStyle
  ☐solid ☐double ☐dotted ☐dashed ☐wavy
- [ ] **TextDirection** on `mt-layout` · users: Column, CupertinoTextField, Flex, Icon, IndexedStack, InputDecoration, RichText, Row, SelectableText, SlideTransition, Stack, Table, Text, TextField,
  ☐rtl ☐ltr
- [ ] **TextInputAction** on `mt-inputs` · users: CupertinoTextField, DropdownMenu, TextField, TextFormField
  ☐none ☐unspecified ☐done ☐go ☐search ☐send ☐next ☐previous ☐continueAction ☐join ☐route ☐emergencyCall ☐newline
- [ ] **TextInputType** on `mt-inputs` · users: CupertinoTextField, DropdownMenu, TextField, TextFormField
  ☐text ☐multiline ☐number ☐decimal ☐signedNumber ☐phone ☐datetime ☐emailAddress ☐url ☐visiblePassword ☐name ☐streetAddress ☐none
- [ ] **TextLeadingDistribution** on `mt-text` · users: TextStyle
  ☐proportional ☐even
- [ ] **TextOverflow** on `mt-text` · users: RichText, Text, TextStyle
  ☐clip ☐fade ☐ellipsis ☐visible
- [ ] **TextWidthBasis** on `mt-text` · users: RichText, SelectableText, Text
  ☐parent ☐longestLine
- [ ] **ThemeMode** on `mt-actions` · users: setThemeMode
  ☐system ☐light ☐dark
- [ ] **TileMode** on `mt-paint` · users: BlurFilter, Gradient
  ☐clamp ☐repeated ☐mirror ☐decal
- [ ] **TimelineStepState** on `mt-l2` · users: TimelineStep
  ☐done ☐current ☐pending ☐failed
- [ ] **TooltipTriggerMode** on `mt-buttons` · users: Tooltip
  ☐manual ☐longPress ☐tap
- [ ] **TransactionStatus** on `mt-l2` · users: TransactionListItem
  ☐pending ☐completed ☐failed ☐reversed
- [ ] **VerticalDirection** on `mt-layout` · users: Column, Flex, Row, Wrap
  ☐up ☐down
- [ ] **VisualDensity** on `mt-buttons` · users: ActionChip, ButtonStyle, Checkbox, CheckboxListTile, Chip, ChoiceChip, ExpansionTile, FilterChip, IconButton, InputChip, InputDecoration, ListTile, Ra
  ☐standard ☐comfortable ☐compact ☐adaptivePlatformDensity
- [ ] **WrapAlignment** on `mt-layout` · users: Wrap
  ☐start ☐end ☐center ☐spaceBetween ☐spaceAround ☐spaceEvenly
- [ ] **WrapCrossAlignment** on `mt-layout` · users: Wrap
  ☐start ☐end ☐center

## Appendix N — Every action

One ☐ per action and per input. `used on` lists the screens of this sheet that run it.

- [ ] **announce** (P8) — used on `launch (plugin)`, `passes (plugin)`, `ac-edit`, `pay`, `nb-eligibility`, `mt-actions`, `mt-a11y`
  ☐message ☐assertiveness
- [ ] **apiCall** (P5) → `O` — used on `ex-org`, `passes (plugin)`, `pass`, `ps-feedback`, `ac-edit`, `ac-privacy`, `ac-delete`, `verify (plugin)` +21
  ☐operation ☐input ☐optimistic
- [ ] **biometricAuth** (P6) → `bool` — used on `ac-security`, `mt-actions`
  ☐reason ☐allowDeviceCredential
- [ ] **callFlow** (P5) → `O` — used on `nb-identity`, `cp-review`, `cr-register`, `dc-checkout`, `cl-confirm`, `rl-donate`, `hm-apply`, `ar-join` +1
  ☐flow ☐input
- [ ] **callNative** (P4) → `O` — used on `ex-map`, `pass`, `ac-about`, `dc-agenda`, `cl-confirm`, `mt-actions`
  ☐action ☐input
- [ ] **capturePhoto** (P5) → `PickedFile` — used on `ac-edit`, `mt-actions`
  ☐maxDimension
- [ ] **condition** (P4) — used on `BaseCard`, `home`, `ln-interests`, `explore`, `ex-map`, `ex-scan`, `passes`, `pass` +18
  ☐when
- [ ] **controlAnimation** (P5) — used on `mt-motion`
  ☐animation ☐command ☐position
- [ ] **copyToClipboard** (P5) — used on `InfoRow`, `pass`, `mt-actions`, `mt-secure`
  ☐text
- [ ] **dbDelete** (P5) — used on `inbox`, `passes`, `cp-review`, `dc-cart`, `dc-checkout`, `mt-data`
  ☐collection ☐key
- [ ] **dbInsert** (P5) → `string` — used on `mt-data`
  ☐collection ☐record
- [ ] **dbQuery** (P5) → `list<T>` — used on `pass`, `cp-home`, `dc-badge`, `mt-data`
  ☐collection ☐where ☐orderBy ☐descending ☐limit ☐offset
- [ ] **dbUpdate** (P5) — used on `inbox`, `passes (plugin)`, `passes`, `pass`, `campus (plugin)`, `dc-cart`, `mt-data`
  ☐collection ☐key ☐patch
- [ ] **dbUpsert** (P5) — used on `launch (plugin)`, `passes (plugin)`, `campus (plugin)`, `cp-personal`, `dc-tickets`, `dc-workshops`, `mt-data`
  ☐collection ☐key ☐record
- [ ] **delay** (P5) — used on `pay`, `nb-pin`, `mt-actions`
  ☐duration
- [ ] **emitEvent** (P5) — used on `BaseCard`, `OrgCard`, `SectionHeader`, `SubmitBar`, `InfoRow`, `EmptyHint`, `ErrorBlock`, `mt-components`
  ☐event ☐payload
- [ ] **emitHostEvent** (P4) — used on `home`, `ac-privacy`, `mt-actions`, `mt-native`
  ☐event ☐payload
- [ ] **forEach** (P5) — used on `inbox`, `passes (plugin)`, `passes`, `cp-documents`, `dc-workshops`, `dc-checkout`, `mt-actions`
  ☐items
- [ ] **getLocation** (P5) → `GeoLocation` — used on `ex-map`, `vf-address`, `mt-actions`
  ☐accuracy
- [ ] **haptic** (P5) — used on `launch (plugin)`, `ex-scan`, `passes`, `vf-otp`, `pay`, `cs-checkin`, `nb-pin`, `hm-apply`
  ☐pattern
- [ ] **invokeFunction** (P7) → `O` — used on `pass`, `pay`, `pt-review`, `nb-eligibility`, `cp-essay`, `cr-register`, `dc-tickets`, `dc-workshops` +1
  ☐function ☐input
- [ ] **kvGet** (P5) → `T?` — used on `home`, `mt-data`
  ☐key
- [ ] **kvRemove** (P5) — used on `mt-data`
  ☐key
- [ ] **kvSet** (P5) — used on `ln-whats-new`, `ac-privacy`, `ac-security`, `nb-terms`, `cp-essay`, `dc-agenda`, `mt-data`
  ☐key ☐value
- [ ] **logout** (P5) — used on `profile`, `ac-delete`, `mt-actions`
- [ ] **navigate** (P4) — used on `BaseCard`, `OrgCard`, `HeroHeader`, `InfoRow`, `StatusChip`, `launch (plugin)`, `home`, `ln-welcome` +87
  ☐route ☐params ☐mode ☐until
- [ ] **openBottomSheet** (P4) → `R?` — used on `home`, `inbox`, `explore`, `ex-map`, `passes`, `pass`, `profile`, `ac-edit` +14
  ☐route ☐params ☐dismissible
- [ ] **openDialog** (P4) → `R?` — used on `home`, `inbox`, `explore`, `ex-map`, `passes`, `pass`, `profile`, `ac-edit` +14
  ☐route ☐params ☐dismissible
- [ ] **openUrl** (P5) — used on `ln-whats-new`, `ex-scan`, `ac-privacy`, `ac-about`, `nb-welcome`, `cp-programme`, `mt-nav`
  ☐url
- [ ] **parallel** (P5) — used on `home`, `mt-actions`
  ☐lanes
- [ ] **patchState** (P5) — used on `explore`, `ex-filters`, `cs-submission`, `cp-personal`, `mt-actions`, `mt-state`
  ☐path ☐patch ☐optimistic
- [ ] **pickFile** (P5) → `list<PickedFile>` — used on `cp-documents`, `mt-actions`
  ☐mimeTypes ☐multiple
- [ ] **pickImage** (P5) → `list<PickedFile>` — used on `ac-edit`, `pt-request`, `mt-actions`
  ☐multiple ☐maxDimension
- [ ] **pop** (P4) — used on `ln-welcome`, `ln-language`, `ln-whats-new`, `ex-filters`, `ex-peek`, `ps-feedback`, `ps-confirm`, `ps-draft` +17
  ☐result
- [ ] **readConfig** (P6) → `T` — used on `nb-welcome`, `mt-home`
  ☐key
- [ ] **refreshData** (P5) — used on `launch (plugin)`, `home`, `inbox`, `explore`, `ex-map`, `ex-peek`, `ex-org`, `passes` +6
  ☐source ☐more
- [ ] **requestPermission** (P5) → `bool` — used on `ex-map`, `ex-scan`, `ac-preferences`, `vf-id`, `mt-actions`
  ☐permission
- [ ] **resetForm** (P5) — used on `mt-state`
  ☐form
- [ ] **resetState** (P5) — used on `ex-filters`, `mt-state`
  ☐path
- [ ] **scanCode** (P5) → `ScanResult` — used on `ex-scan`, `dc-badge`, `mt-actions`
  ☐formats
- [ ] **setLocale** (P8) — used on `ln-language`, `mt-actions`
  ☐locale
- [ ] **setState** (P5) — used on `BaseCard`, `launch (plugin)`, `home`, `ln-interests`, `explore`, `ex-filters`, `ex-category`, `ex-map` +46
  ☐path ☐value
- [ ] **setThemeMode** (P8) — used on `ac-preferences`, `devconf (plugin)`, `mt-actions`
  ☐mode
- [ ] **share** (P5) — used on `ex-peek`, `passes (plugin)`, `pass`, `ps-receipt`, `account (plugin)`, `ac-privacy`, `pt-sent`, `console (plugin)` +9
  ☐text ☐url ☐file
- [ ] **showSnackbar** (P5) — used on `inbox`, `ex-map`, `ex-scan`, `pass`, `ac-edit`, `pt-review`, `cs-submission`, `cs-checkin` +11
  ☐message ☐actionLabel ☐duration
- [ ] **showToast** (P5) — used on `ex-org`, `passes`, `pass`, `ac-privacy`, `ac-security`, `cs-checkin`, `campus (plugin)`, `cr-register` +4
  ☐message ☐duration
- [ ] **signTransaction** (P6) → `string` — used on `nb-deposit`, `rl-donate`
  ☐transaction ☐amount ☐payee
- [ ] **startAnimation** (P5) — used on `pt-pitch`, `mt-motion`
  ☐animation
- [ ] **startPayment** (P13) — not built yet; the **Pay** plugin (§G6) stands in. Re-check when P13 lands
- [ ] **stop** (P4) — used on `account (plugin)`, `verify (plugin)`, `pay (plugin)`, `pt-request`, `console (plugin)`, `ar-join`, `mt-motion`, `mt-actions`
  ☐result ☐error
- [ ] **submitForm** (P5) → `T` — used on `ac-edit`, `pt-review`, `nb-eligibility`, `cp-personal`, `cr-register`, `cl-patient`, `rl-volunteer`, `hm-apply` +2
  ☐form
- [ ] **subscribe** (P5) — used on `inbox`, `pass`, `cr-live`, `cl-queue`, `mt-data`
  ☐stream ☐params
- [ ] **switch** (P5) — used on `home`, `inbox`, `ln-offline`, `passes`, `pass`, `ac-edit`, `ac-preferences`, `pay` +13
  ☐value ☐cases
- [ ] **switchTab** (P4) — used on `home`, `passes`, `ac-sign-in`, `mt-nav`
  ☐tab
- [ ] **sync** (P5) — used on `ln-offline`, `passes`, `mt-actions`
- [ ] **trackEvent** (P5) — used on `ln-welcome`, `ac-privacy`, `pt-pitch`, `hm-listing`, `mt-actions`
  ☐name ☐props
- [ ] **unsubscribe** (P5) — used on `inbox`, `pass`, `cr-live`, `mt-data`
  ☐stream
- [ ] **validateForm** (P5) → `bool` — used on `pt-request`, `mt-state`
  ☐form

## Appendix O — Platform options

One ☐ per value; where it is exercised follows the list.

- [ ] **Data source kinds** — rest `explore`, graphql `campus`, websocket `cityrun`, sse `launch`/`clinic`, function `campus`/`devconf`, database `passes`, static `cityrun`/app `categories`
  ☐rest ☐graphql ☐websocket ☐sse ☐function ☐database ☐static
- [ ] **Data source route** — plux everywhere; direct on `mt-data` (refused in production)
  ☐plux ☐direct
- [ ] **Cache policies** — `mt-data`; also `explore`, `launch`, `passes`
  ☐networkOnly ☐cacheFirst ☐networkFirst ☐staleWhileRevalidate ☐TTL ☐encrypted
- [ ] **Pagination styles** — cursor `explore`/`campus`, page `console`, offset `passes`
  ☐cursor ☐page ☐offset
- [ ] **Data source triggers** — `mt-data`, `launch`, `passes`, `campus`
  ☐onLoaded ☐onFailed ☐onMessage ☐onProgress ☐onSynced ☐onSyncFailed ☐onConflict
- [ ] **Operation flags** — `account`, `campus`, `bank`, `relief`, `mt-actions`
  ☐auth ☐offlineCapable ☐transaction ☐requiresAssurance ☐optimistic
- [ ] **State scopes** — §B3, plugins, pages, `OrgCard`, `mt-state`
  ☐app ☐plugin ☐page ☐component ☐run
- [ ] **Persistence** — §B3 app state, `explore`, `campus`, `clinic`, `mt-state`
  ☐memory ☐session ☐persisted ☐secure
- [ ] **State flags** — §B3, `bank`, `clinic`, `mt-state`, §J5
  ☐sensitive ☐exposed ☐computed ☐migration ☐watch
- [ ] **Triggers** — §B3, `home`, `vf-otp`, `mt-actions`
  ☐widget events ☐onInit ☐onEnter ☐onResume ☐onLeave ☐onDispose ☐timers ☐watch ☐onAppResume ☐onAppPause ☐onPushOpened ☐hostEvents ☐dataSources ☐onError
- [ ] **Concurrency** — `mt-actions`; real: debounce `explore`, throttle `cs-checkin`, restart `home`, queue `passes`, detached `campus`
  ☐parallel ☐drop ☐restart ☐queue ☐debounce:<ms> ☐throttle:<ms> ☐detached
- [ ] **Step options** — `mt-actions`, `pass`, `pt-review`
  ☐next ☐onSuccess ☐onError ☐branches ☐retry.count ☐retry.backoffMs ☐retry.maxBackoffMs ☐retry.jitter ☐retry.on ☐timeoutMs
- [ ] **Error kinds** — `mt-actions` (each one routed)
  ☐network ☐http ☐timeout ☐validation ☐function ☐permission ☐cancelled ☐custom
- [ ] **Form validators** — `ar-join` (all), `ac-edit`, `nb-details`, `dc-tickets` (async)
  ☐required ☐length ☐range ☐regex ☐email ☐phone ☐iban ☐dateRange ☐decimalPrecision ☐custom ☐async
- [ ] **Form state** — `ac-edit`, `ar-join`, `cr-register`
  ☐values ☐errors ☐dirty ☐touched ☐status ☐valid ☐validating
- [ ] **Page kinds** — every plugin; `mt-pages`
  ☐screen ☐dialog ☐bottomSheet ☐fullscreenDialog
- [ ] **Page transitions** — `mt-transitions` (+ real pages)
  ☐platform ☐fade ☐slideLeft ☐slideRight ☐slideUp ☐slideDown ☐scale ☐sharedAxis ☐custom ☐none
- [ ] **Node enter/exit** — `mt-motion`
  ☐fade ☐scale ☐slideUp ☐slideDown ☐slideLeft ☐slideRight
- [ ] **Curves (node/timeline)** — `mt-motion`
  ☐linear ☐easeIn ☐easeOut ☐easeInOut ☐fastOutSlowIn ☐decelerate ☐bounceIn ☐bounceOut ☐elasticOut ☐overshoot
- [ ] **Reduce motion** — `mt-motion`, `cr-race`
  ☐skip ☐shorten ☐ignore
- [ ] **Timeline drivers / scope** — `home` (scroll), `mt-motion` (drag, route)
  ☐scroll ☐drag ☐page ☐route
- [ ] **Guard decisions** — `account` (redirect), `console` (fallback)
  ☐allow ☐redirect ☐fallback
- [ ] **Navigation modes** — `mt-nav`; real in `ln-interests`, `pass`
  ☐push ☐replace ☐popUntil ☐clearAndPush
- [ ] **Device APIs** — §B3 grants; `mt-actions` requests each
  ☐camera ☐photos ☐files ☐location ☐contacts ☐biometrics ☐notifications ☐clipboard ☐share ☐haptics
- [ ] **Host event directions** — §B3 hostEvents
  ☐toHost ☐toPlux ☐both
- [ ] **Template visibility** — §D2
  ☐private ☐organization ☐public
- [ ] **Flag types** — §B3, §J4
  ☐bool ☐int ☐double ☐string ☐json (Studio)
- [ ] **Calendars** — `cp-personal`, `rl-volunteer`, `ac-edit`, `mt-l10n`
  ☐gregorian ☐ethiopic ☐islamicUmalqura ☐persian
- [ ] **Numbering systems** — §B3 (latn), staging (native)
  ☐latn ☐native
- [ ] **Startup / activation** — §B3, §I4
  ☐useCacheThenSync ☐blockUntilSynced ☐immediate ☐atSafePoint ☐nextLaunch ☐forced (host config)
- [ ] **Required-features policy** — §B3
  ☐reject ☐raise
- [ ] **Security profiles** — §B5
  ☐standard ☐strict ☐maximum
- [ ] **Assurance levels** — `ac-security` AL1, `nb-details` AL2, `mt-secure` AL3
  ☐AL0 ☐AL1 ☐AL2 ☐AL3
- [ ] **RASP responses** — §B5, `mt-secure`
  ☐report ☐warn ☐degrade ☐block
- [ ] **Media types** — §C4 assets
  ☐image/png ☐image/jpeg ☐image/webp ☐image/gif ☐image/svg+xml ☐font/ttf ☐font/otf ☐application/lottie+zip ☐application/riv
- [ ] **ICU MessageFormat** — §H1, `mt-l10n`
  ☐plural (=n, offset) ☐selectordinal ☐select ☐number ☐date ☐time
- [ ] **Locales** — §B2, §H1
  ☐en ☐de-DE ☐ar (RTL) ☐am-ET ☐pseudo (accented, expanded, RTL) ☐fallback de-AT
- [ ] **Theme modes / sources** — §C, `ac-preferences`
  ☐system ☐light ☐dark ☐high contrast ☐brand overlay ☐themeSource plux
- [ ] **Security settings (read each in the effective configuration; set those in §B5)** — §B5
  ☐accessTokenLifetime ☐accessibilityBlocksPublish ☐allowDirectDataSources ☐allowSessionReplay ☐allowSoftwareKeys ☐androidDeviceVerdictAL2 ☐androidRefreshRequiresIntegrity ☐attestationOutageGrace ☐confidentialBundles ☐confidentialMinAssurance ☐defaultFunctionAssurance ☐devSessionRequireStepUp ☐deviceFunctions ☐dpopIatWindow ☐dpopIatWindowFallback ☐dpopNonceRotation ☐encryptLocalStores ☐inactivityLock ☐inactivityLockTimeout ☐minAssuranceForSync ☐pinnedMediaOnly ☐raspAccessibilityResponse ☐raspDebuggerResponse ☐raspEmulatorResponse ☐raspInstallerResponse ☐raspOverlayResponse ☐raspRepackagingResponse ☐raspRootHookingResponse ☐raspScanInterval ☐raspScreenCaptureResponse ☐reattestationInterval ☐registrationChallengeTtl ☐replayCacheFallback ☐requiredTests ☐riskMetricMaxAL3 ☐screenshotBlockingDefault ☐tls12Allowed

<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Next steps after `claude/handoff-studio-fixes` (written 2026-11-12)

The single list of what remains: the agreed design for the Dev app experience in host apps (Part 1) and every open
fix, check and follow-up from this branch (Part 2). Start a new session here: read
`AGENTS.md`, `docs/WORKLOG.md`, this file and `README.md` in this folder, then work Part 1 M1 first.

Branch: `claude/handoff-studio-fixes` (the commit that added this file). The maintainer applies changes as patches
(`git apply <file>.patch`, no `--3way`) and asked for short replies.

# Part 1. The Dev app experience inside any host app's debug build

Status: **design agreed by the maintainer on 2026-11-12** (decisions in §7). Nothing is implemented yet.
Requirements: DEV-002, DEV-040, DEV-050, SEC-074 (amends ADR-0104; needs a new ADR).

## 1. Problem

Live editing from Studio works only in the generic **Plux Dev app** (`apps/plux_dev`). An app whose plugins call
native code, or are called from it, cannot be tested there:

- Native routes (`sign-in`, `licenses`), native slots (`CalendarButton`, `HostVersionTile`) and native actions
  (`addToCalendar`, `openAppSettings`, `requestReview`) exist only in the host app's own code.
- Calls from the host into plugins (`handleDeepLink`, `handlePushPayload`, `sendEvent`, `Plux.state`,
  `setUserContext`) need the real host. So do the app's identity (push, links, attestation) and its own packages.

DEV-040 already requires that a developer "MUST be able to pair their own host app in debug builds with
`plux_devtools`, with the same capabilities as the Dev app". Today a host debug build can only pair with a pasted
link or code (`PluxDevtools` › Session). Phone sign-in, joining with the number, the status pill, the dev sheet and
automatic rejoin all live in `apps/plux_dev`, so DEV-040's "same capabilities" is not met.

## 2. Options considered

| Option | Verdict | Why |
|---|---|---|
| Dev kit as a `dev_dependency` of the host | Rejected | Android drops dev-dependency plugins from release builds (Flutter 3.29, flutter#161343). iOS and macOS do not: flutter#163874 is open, and the latest maintainer comment (July 2025) says "always include dev_dependencies (even on release builds) for now". Any native code in the kit would ship on iOS. |
| Two app shells over a shared `app_core` package | Workable, not chosen | Safe by construction, but app-level native settings (Info.plist, entitlements, manifest, Firebase, signing, `MainActivity`) are duplicated and drift. The dev shell is a different app identity, so push, links and attestation differ. Existing projects need a migration. |
| Stand-ins for native calls in the Dev app | Rejected as the main path | It never exercises the real native code. |
| **Dev experience as a pure-Dart widget in `plux_devtools`, used by the host's own debug build** | **Chosen** | No new native code. It uses the runtime's existing native parts (keystore, secure store). Release builds exclude it by compile-time constant, already proven by `make release-inspect`. It runs the real native code with the real identity, with no second shell. |

## 3. Facts the design rests on

- `packages/plux_devtools` has no `android/` or `ios/` folder and no `flutter: plugin:` section: pure Dart.
- `kPluxDevtools = kDebugMode || bool.fromEnvironment('plux.devtools')`
  (`packages/plux_flutter/lib/src/devtools_api/dev_session.dart`). Release builds without the define drop every
  branch it guards. `kDevCanary` and `test/release/probes.txt` let `make release-inspect` (starter and Plux Bank,
  Android and iOS) fail when dev code survives.
- Phone sign-in and joining (`DevPhone`, `DevSignInLink`, `DevChannel`) live in the runtime
  (`packages/plux_flutter/lib/src/devtools_api/`). Their native needs are the runtime's own: `PlatformDeviceKeys`
  and `PlatformSecretStore`, present in every Plux app.
- The only native dependency the Dev app adds is `plux_scanner` (camera). The kit must not depend on it.
- Release apps are built with the normal `flutter build apk|appbundle|ipa`. `plux build` only compiles the project's
  documents into bundles.

## 4. Solution

### 4.1 `PluxDevHost`, a widget in `plux_devtools`

Move from `apps/plux_dev` into `packages/plux_devtools/lib/src/host/`:

| From `apps/plux_dev/lib/src/` | To `plux_devtools` | Notes |
|---|---|---|
| `connect_screen.dart` | `host/connect_sheet.dart` | Sign in: the link (§4.2), paste, or type a code (§4.3) |
| `ui/number_picker.dart` | `host/number_picker.dart` | Unchanged behaviour |
| `ui/status_pill.dart` | `host/status_pill.dart` | Floating, see-through, top end |
| `ui/dev_sheet.dart` | `host/dev_sheet.dart` | Connection, Sync, Release, Problems tabs |
| join and rejoin logic in `dev_app.dart` (`_open`, `_rejoin`, `_follow`) | `host/dev_link_controller.dart` | Joins **this** app's Studio session; rejoins on its own |
| `phone.dart` (`RuntimePhones`) | `host/phones.dart` | Over `DevPhone` |
| `ui/theme.dart` | `host/theme.dart` | Self-contained, never touches the host's theme |

API (host side, one place in `main.dart`):

```dart
runApp(MaterialApp(
  navigatorKey: navigatorKey,
  builder: (context, child) => kPluxDevtools
      ? PluxDevHost(navigatorKey: navigatorKey, child: child!)   // where sheets and the picker open
      : child!,
  …,
));
```

Scanner, detected (no `scan` parameter): when the host registered `plux_scanner`
(`PluxConfig.devicePackages: [PluxScanner(navigatorKey: …)]`), the runtime holds a `PluxCodeScanner` service
(interface in `packages/plux_flutter/lib/src/device/types.dart`, provided by `plux_scanner`'s
`services => {PluxCodeScanner: …}`). `PluxDevHost` asks the runtime for that service and offers **Scan** when it is
there. It never imports `plux_scanner`, so `plux_devtools` stays pure Dart. Add the runtime getter if none exists, and
check how a device package's service is reached outside a plugin's declared device APIs: the dev host is not a
plugin, so the `camera` API approval (SEC-080) must not be bypassed for plugins.

Behaviour:

1. Not signed in: the pill says **Connect**, and the connect sheet offers, in order, **Scan** (when detected),
   opening Studio's QR code with the phone's camera (§4.2), pasting the link, or typing the code (§4.3).
2. Signed in: it joins Studio's session of the app the runtime runs (`PluxConfig.appId`), with the number picker
   when Studio's session is new. Then it pairs (`PluxDevSession.pair`) on the running runtime, with no restart.
3. Rejoin: when the link ends, it joins again on its own: at once, every 15 s while Studio is closed, and on
   resume. **Join again** does the same on request (already built in `dev_app.dart`).
4. Environment: dev sessions exist only in non-production environments. If the runtime runs another environment than
   the join's `environment`, the sheet says so and **offers to restart in that environment** (decided): on
   confirmation it disposes the runtime and starts it again with the join's environment (as the Dev app does in
   `_launch`). It never switches without asking.
5. The Dev app becomes a thin host of `PluxDevHost` plus its app list and runtime start/stop.

Release safety: the call site and the widget are both behind `kPluxDevtools`. `PluxDevHost` adds no plugin
dependency to `plux_devtools`; a CI check fails if `plux_devtools/pubspec.yaml` gains a dependency with native code.

### 4.2 Opening Studio's QR code with the phone's camera

Studio's QR code holds the sign-in link. The system camera (Android and iOS) reads it and opens the app the link
belongs to; the host forwards it to `PluxDevHost.handleLink(uri)` (or `Plux.handleDeepLink`, which hands sign-in
links to the dev host in `kPluxDevtools` builds and ignores them otherwise).

Two link forms; the server picks one per app:

| Form | When | Security |
|---|---|---|
| **App Link / Universal Link** `https://<server>/dev/signin?grant=…` | The Plux server has a public HTTPS domain and the app registered its debug build | Verified by the OS: only the registered app receives it |
| **Custom scheme** `plux-dev-<appKey>://signin?server=…&grant=…` | Local setups (localhost, LAN, own CA), where OS verification cannot work | Another app could claim the scheme; the number match and Studio naming the phone limit the damage |

Debug builds only:
- **Android:** `android/app/src/debug/AndroidManifest.xml` (merged into debug builds only). It holds the intent
  filter for the custom scheme, and for the App Link (`autoVerify="true"`, host `<server>`, `pathPrefix="/dev/"`).
- **iOS:** the URL type and `applinks:<server>?mode=developer` live in the Debug configuration only, through
  `Runner/RunnerDebug.entitlements` (`CODE_SIGN_ENTITLEMENTS[config=Debug]`) and a Debug-only `CFBundleURLTypes`
  entry (`INFOPLIST_FILE[config=Debug] = Runner/Info-Debug.plist`).
- **Server:** serves `/.well-known/assetlinks.json` and `/.well-known/apple-app-site-association`. They are generated
  from the debug builds an app admin registered (Android package and SHA-256 signing certificate; iOS team ID and
  bundle ID), restricted to `/dev/*`. They are registered under **app settings › Dev builds** (decided); registering
  needs `app.manage` and is audited.
- **CLI:** `plux dev init` writes the debug-only manifest, plist and entitlements and the `PluxDevHost` call.
  `plux doctor` fails when a release manifest or plist names a dev link.

### 4.3 Typing a short code

**Studio** (`features/profile/phones.tsx`): **Type a code instead** reveals an 8-character code (decided), shown as `XXXX-XXXX`
in an unambiguous 32-letter alphabet (~40 bits). It sits under the QR code with the countdown and "Never read or
send this code to anyone". The QR code stays the default.

**Server** (`proto/plux/v1/devphone.proto`, `backend/internal/auth/phones.go`):
- `CreatePhoneSignInRequest.short_code = true`: the response adds `short_code`, stored as a hash.
- `ClaimPhoneSignInRequest` takes `short_code` in place of `grant`.
- Same lifetime as the grant (≤ 2 min), single use, constant-time comparison.
- Attempt limits per user and per IP from the limits registry (new keys `devPhone.shortCodeAttemptsPerUser` and
  `devPhone.shortCodeAttemptsPerIp`, LIM-001).
- An audit entry and a Studio alert on failed attempts.
- The number match, the security-key step-up and Studio naming the phone are unchanged.

**Server address for typed codes:** a host debug build already knows its server (`PluxConfig.endpoint`). The
generic Dev app takes it from a build define, with an editable field as fallback.

### 4.4 Order of sign-in methods

1. **Scan**, when the host registered `plux_scanner` (detected).
2. The phone's camera with a verified App Link, or the custom scheme on local setups.
3. Paste the link.
4. Type the short code.

## 5. Milestones

| # | Work | Done when |
|---|---|---|
| M1 | Release safety first: add a `PluxDevHost` stub to the starter's and Plux Bank's debug path; extend `probes.txt`; finish SEC-074 / DEV-050 | `make release-inspect PLATFORM=android` and `PLATFORM=ios` pass, and fail on a leaky fixture |
| M2 | Move the experience into `plux_devtools` (§4.1); Dev app becomes a thin host | Dev app tests (34) pass unchanged in behaviour; new widget tests for sign-in, join, rejoin and the sheet; Plux Bank's debug build joins Studio live |
| M3 | Camera link with the custom scheme; `plux dev init` / `plux doctor` | The debug build opens from the system camera; release manifest and plist hold no dev link (doctor test) |
| M4 | Short code (§4.3): server, Studio, widget | Server tests for format, lifetime, single use, limits, constant-time compare and audit; Studio and widget tests; live check |
| M5 | App Links (§4.2): registration UI, well-known files | Endpoint tests; verified link on a public test domain |
| M6 | Docs | New ADR "Dev experience in host debug builds" (records the options of §2); ADR-0104 amended; DEV-002 amended (camera link, short code); DEV-040 note; guides (`run-locally`, `studio/security-and-admin`); `make spec-lint` |
| M7 | `plux create` produces a ready host project: `PluxDevHost` wired in behind `kPluxDevtools`, the debug-only links of §4.2 (Android `src/debug/AndroidManifest.xml`, iOS Debug-only URL type and entitlements), `PLUX_ENV=development` for debug runs, and a README with the three commands (create, init, run) | A new project from `plux create` runs in debug, signs in to Studio and goes live with no manual step; its release build passes `make release-inspect`-equivalent checks |

## 6. Security summary

- **Release builds:** no dev code by compile-time constant, proven by the release inspection on Android and iOS
  (M1, before anything else).
- **Native code:** the kit adds none, so flutter#163874 does not apply.
- **Sign-in:** step-up, single-use grants, the number match and phone naming are unchanged. A short code has ~40 bits,
  2 minutes and attempt limits. The only new exposure is a code read out over a call, mitigated by the warning,
  QR-by-default and failed-attempt alerts.
- **Links:** App Links are verified by the OS. The custom scheme is local-only and app-specific, and the number match
  still guards it.

## 7. Decisions (maintainer, 2026-11-12)

1. The widget is `PluxDevHost`.
2. The short code has 8 characters (~40 bits).
3. Debug builds are registered for App Links under **app settings › Dev builds**.
4. On an environment mismatch the sheet offers to restart in the session's environment.
5. The scanner is detected through the runtime's `PluxCodeScanner` service; hosts pass nothing.
6. Fallback if Part 1 fails its M1 release check: two app shells over a shared `app_core` package (see §2), with a CI
   guard on the release shell's dependency tree, a generator plus a drift check for native settings, and a dev app
   identity. Re-decide with the maintainer before taking it.

## 8. The two ways to preview, and the Dev app's role (maintainer, 2026-11-12)

- **Main path for developers: the host app's own debug build with `PluxDevHost`.** Everything is the release
  behaviour — real native routes, slots and actions in both directions, real push, real links, real sign-in, the
  app's own identity — plus the live link (phone sign-in, join with the number, the pill, the sheet with diagnostics)
  that brings plugin edits live. `PluxDevHost` adds nothing else: no simulated pushes, test users or stand-ins.
- **The Dev app stays as it is**, for people without a Flutter toolchain (designers, managers, QA, reviewers),
  for prototyping before a host exists, and for seeing every app of every organisation in one place. Its behaviour
  does not change; after M2 it shares `PluxDevHost`'s code for the sign-in, join, pill and sheet.

# Part 2. Every other open item from this branch

## 2.1 Bugs to fix (found 2026-11-12)

**A. The phone draws an empty footer bar.** On a page whose Scaffold sets `persistentFooterDecoration` but has no
`persistentFooterButtons`, the runtime passes Flutter an empty list (`NodeContext.slotList` returns `[]`), so Flutter
draws a footer bar that Studio's canvas does not. Fix: the render code generator passes `null` for an empty optional
list slot (`make gen`), plus a runtime test.

**B. The JSON editor reports PLX-1020 on valid documents.** The JSON tab checks every file as a page
(`json-editor-host.tsx` passes `validatePage`), so `plugin.json` and `canvas.json` get "this location holds page
documents". Fix: check each file by its kind (page, plugin, canvas), plus Studio tests.

**C. The comparison assumes Android.** A frame matched to a device is a Material frame (`customFrame` in
`studio/apps/web/src/features/preview/frames.ts`); an iPhone's screenshot is then compared with Android chrome and
defaults. Fix: add `platform` to `ScreenshotEvent` (additive), have the runtime fill it, and have **Match the canvas to
the device** set the preview platform too.

**D. The screenshot's dark mode is the phone's setting.** `dark` comes from `MediaQuery.platformBrightness`; a host that
fixed a mode with `Plux.setThemeMode` draws otherwise. Fix: report the mode the runtime resolved (the same logic as
`render/renderer.dart`'s `dark` switch) instead.

## 2.2 Checks still to run on the maintainer's machine and a real phone

From hand-off items 7–10 in `README.md`, none of which is checked on a real phone yet:
- **Phone sign-in:** the keystore key, the camera scan, the phone's name in **Profile › Phones**, the app opening at its
  entry screen, the status pill, **Leave**, and `make dev`'s `adb reverse` watcher over USB and wireless debugging
  (from WSL it uses Windows' `adb.exe`).
- **Rejoin:** close Studio's tab, reopen the app canvas; the phone joins again by itself and shows the number when
  Studio's session is new. **Join again** works on request; a declined number waits.
- **Studio console:** goes **Live** on the maintainer's machine. If it ever stays on "Connecting…" or "Reconnecting…"
  for more than ~20 s, record the `WatchEvents` requests in the browser's Network tab (Pending, 200 or 429). The root
  cause on that machine was never reproduced here; the long-poll fallback covers buffering networks only.
- **Theme and compare:** pages draw in the app's own theme (no `#5B3DF5` bar, readable text). **Match the canvas to the
  device**, then Difference, shows only real differences.
- **Docker images:** after applying the patches, `make dev` must rebuild the server and Studio images (the browser
  needs the new Studio bundle; hard-reload Studio).

## 2.3 Earlier items still open (README "Remaining tasks")

1. **One combined live pass** of the earlier fixes (README task 1): dev session, asset uploads and thumbnails, the
   Monaco editors, translations (a–d), theme token save, a forced refusal rolling back every document, and the CSP sweep.
2. **Multi-step undo** (README task 3): moving a screen to another plugin and a rename that updates links, live.
   Editors without an undo manager (the app canvas's `EditorProvider`) undo only their own document on refusal.
3. **Layers does not name the slot** a child is in.
4. **Reports:** the app bundle's row says "Size not reported".
5. **Deleting a translation key** leaves its messages in the other locales (reported as `PLX-1608` until edited).
6. **Asset images through the BFF** (`/api/v1/objects/…`): unit-tested, not checked live.
7. **Work log:** update the `docs/WORKLOG.md` note of 2026-11-08 with the live pass's findings, and delete this folder
   once the branch is merged.

## 2.4 Smaller follow-ups

- **App canvas console:** the app canvas shows the connection pill and the join prompt only; the full console (logs,
  device controls) stays in plugins. Decide whether the app canvas gets the console too.
- **Release inspection** (SEC-074, DEV-050, both WIP): finish it for Android and iOS and run it in CI. Part 1 M1
  depends on it.
- **Dev app and app shells:** the Dev app opens an app at its entry screen; an app that defines a tab shell
  (`PluxShell`) is not shown in its shell there. Decide whether the Dev app should open the shell.
- **Dev app and native items:** pages that call the host's own native routes, slots or actions cannot run them in the
  Dev app (it has no host code); they are tested in the host's debug build with `PluxDevHost`.
- **`plux_devtools` overlay:** the dark panel (`PluxDevtools`) stays for hosts that use it; once `PluxDevHost` exists,
  decide whether `PluxDevtools` is folded into it or kept as the minimal overlay.

# Order of work

1. Part 1 M1 (release safety) → M2 (`PluxDevHost`) → M3 (camera link) → M4 (short code) → M5 (App Links) →
   M7 (`plux create` template) → M6 (docs).
2. Part 2.1 bugs A–D, each with tests, in any gap.
3. Part 2.2 checks with the maintainer as each patch lands.

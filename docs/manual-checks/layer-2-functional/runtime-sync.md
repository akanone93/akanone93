<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Device runtime and sync

You run the generated `shop` app on the device and watch the runtime do its work: sync at start,
deltas, offline, update and rollback, kill switch, then rendering, routing, mixed screens, the host
API, actions, state, data, the local database and animation. The starter app (R-DEVICE) shows the
host features; the `shop` app shows your own release. Guides: [host app](../../guides/host-app.md),
[routing](../../guides/routing.md), [runtime runbook](../../runbooks/runtime.md).

On Android run `adb reverse tcp:8080 tcp:8080` and install the development CA on the emulator once
(see "Development TLS" in `deploy/compose/README.md`).

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-DEVICE-SHOP | The generated `shop` app (R-SHOPAPP) running on R-DEVICE, on the `development` channel | L2-RUN-001 |
| R-RELEASE2 | A second release of R-APP with a visible text change | L2-RUN-005 |

### L2-RUN-001 — Run the generated app: create R-DEVICE-SHOP

- **Needs:** R-SHOPAPP, R-DEVICE
- **Do:** `cd ~/plux-checks/shop-app; flutter run -d <device>`
- **Expect:** the `shop` entry screen (`list`) appears at once, before any network call, from the embedded release.
- **Why:** a first launch renders offline from the baseline (SYN-007, RT-001).
- **Look at:** `docker compose … logs plux-server | grep manifest` shows the sync request a moment later.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-002 — Sync at app start

- **Needs:** R-DEVICE-SHOP
- **Do:** kill the app and start it again with the stack running.
- **Expect:** the runtime fetches the signed manifest and finds nothing new; the screen is unchanged and no flicker occurs.
- **Why:** every plugin is synced at start (SYN-001).
- **Look at:** `plux_manifest_requests_total` rises by one in `/metrics`; the log shows a `sync_result` event.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-003 — Device registers and gets a token

- **Needs:** R-DEVICE-SHOP, R-STUDIO
- **Do:** open **Devices** (under *Security*) of `shop` in Studio.
- **Expect:** your device is listed with platform, build, key storage (software on an emulator), assurance level and last attestation.
- **Why:** every request is bound to a device key (SEC-001, SEC-020).
- **Look at:** on an emulator the level is `AL0` or `AL1` and the key says `software`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-004 — Manual sync and the sync tile

- **Needs:** R-DEVICE (starter)
- **Do:** tap the sync tile in the starter app.
- **Expect:** the tile shows syncing, then the result; the release number shown matches Studio's **Releases**.
- **Why:** hosts can sync on demand (SYN-002).
- **Look at:** the devtools overlay shows the active release and sync status.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-005 — Publish an update: create R-RELEASE2

- **Needs:** R-DEVICE-SHOP, R-RELEASE1
- **Do:**
  1. In Studio change a visible text on `list`, publish to `development` (promote to its channel).
  2. On the device, restart the app (press `R` in the `flutter run` terminal).
- **Expect:** the old text first, then the new one after the sync; the page never changes under your finger while you look at it.
- **Why:** a release activates when no Plux page is shown, or at next start (SYN-004).
- **Look at:** the sync result in the log; `PluxSyncTile` / overlay shows the new sequence.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-006 — Deltas

- **Needs:** R-RELEASE2
- **Do:** note the downloaded size in the devtools overlay or server log for the update of L2-RUN-005; then change one letter, publish, sync again.
- **Expect:** the update downloads a delta far smaller than the full bundle; the result is byte-identical to a full download.
- **Why:** updates cost bytes proportional to the change (NFR-020).
- **Look at:** `GET /v1/objects/deltas/…` requests in the server log; `make bench-sync` for the measured numbers.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-007 — Offline

- **Needs:** R-DEVICE-SHOP
- **Do:** `make compose-down`; kill and start the app; use it; `make compose-up`.
- **Expect:** the app renders its current release offline; when the server returns the next sync succeeds.
- **Why:** devices work without a server (SYN-005, G5).
- **Look at:** the log shows `PLX-3050` for the failed sync, not a crash.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-008 — Tampered bundle is refused

- **Needs:** R-DEVICE-SHOP, R-CLI
- **Do:** in a throwaway copy of the generated project replace a byte of `assets/plux/*.pxb`, run it with the stack down.
- **Expect:** the runtime refuses the bundle and falls back or shows the plugin's fallback; it never renders it.
- **Why:** verify before load (SEC-052, SYN-010).
- **Look at:** `PLX-3040`–`PLX-3045` in the log.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-009 — Roll back from Studio

- **Needs:** R-RELEASE2
- **Do:** in Studio **Releases** choose **Roll back to this release** on R-RELEASE1; sync the device.
- **Expect:** a new, higher sequence with the old content; the device shows the old text.
- **Why:** rollback is a new release, devices never go backwards in sequence (REL-006).
- **Look at:** the sequence on the device is higher than before the rollback.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-010 — Last known good

- **Needs:** R-DEVICE-SHOP
- **Do:** publish a release whose page throws at start (for example a required binding that fails), sync, and restart the app three times.
- **Expect:** after repeated failures the runtime returns to the previous release and keeps it.
- **Why:** a bad release heals itself on the device (SYN-006).
- **Look at:** `PLX-3020` in `error` events; the runbook says what to do next.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-011 — Kill switch on a device

- **Needs:** R-DEVICE-SHOP, R-STUDIO
- **Do:** in Studio **Releases**, **Kill switch** (`/releases/kill-switch`): enter plugin key `catalog`, **Switch off**; sync the device; then **Switch on**.
- **Expect:** the plugin's routes show its fallback page without a new release; switching on restores it.
- **Why:** an emergency stop without a release (REL-030, RT-022).
- **Look at:** `PLX-4020` warnings while it is on; the control document version (`plux control get --env development`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-012 — Rendering and theme

- **Needs:** R-DEVICE (starter)
- **Do:** flip **Dark theme**; open **Open welcome**.
- **Expect:** the page restyles through design tokens; text, icon and image render natively.
- **Why:** pages are native widgets using tokens (RT-010, THM-001).
- **Look at:** no WebView is involved (use Flutter's inspector).
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-013 — Error boundary

- **Needs:** R-DEVICE-SHOP
- **Do:** publish a page with a widget that fails (unknown binding in a child); open it.
- **Expect:** the failing part shows its fallback panel; the rest of the page and the app keep working.
- **Why:** one broken node does not take the app down (RT-020).
- **Look at:** `PLX-4001` with a node path in the log.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-014 — Routing: open a page by name

- **Needs:** R-DEVICE (starter)
- **Do:** tap **Open welcome**, then **Places**, then a place; use the system back.
- **Expect:** pages push and pop like native screens, parameters arrive typed.
- **Why:** routes are app-wide names.
- **Look at:** an unknown route shows the not-found page and `PLX-4100`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-015 — Plugin to plugin and plugin to native

- **Needs:** R-DEVICE (starter)
- **Do:** on a place page use the control that opens the native `profile` screen; go back.
- **Expect:** the plugin navigates into a native screen and returns; the result type is honoured.
- **Why:** plugins and the host navigate each other's screens.
- **Look at:** **Process flow** (L2-DEV-009) shows the `navigate` step.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-016 — Guards and sign-in

- **Needs:** R-DEVICE (starter)
- **Do:** with **Signed in** off tap **Account**; switch **Signed in** on and tap again.
- **Expect:** signed out you are sent to the sign-in page; signed in you reach the account page.
- **Why:** guards are declared in the document and evaluated on the device.
- **Look at:** the app's auth delegate decides `user.authenticated`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-017 — Deep links and notifications

- **Needs:** R-DEVICE (starter)
- **Do:** tap **Open a link** (it opens `plux-starter://places/Lighthouse`) and **Open a notification**; on Android also `adb shell am start -a android.intent.action.VIEW -d plux-starter://places/Lighthouse`.
- **Expect:** the place page opens with `Lighthouse`; a link no pattern maps is handed back to the host (`PLX-4103`).
- **Why:** links map to pages by the app document.
- **Look at:** the linked page shows the typed parameter.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-018 — Mixed screens: Plux inside native

- **Needs:** R-DEVICE (starter)
- **Do:** tap **Mixed screen**; press the native button that writes the counter.
- **Expect:** both `PluxView` badges update from the shared counter state.
- **Why:** native screens embed plugin components with shared state (ADR-0023).
- **Look at:** state `counter` in the device drawer **State** tab.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-019 — Mixed screens: native inside Plux

- **Needs:** R-DEVICE (starter)
- **Do:** open **Places**; pick on the app's own map card.
- **Expect:** the native `MapCard` sits in the plugin page and its pick runs the page's `navigate` step.
- **Why:** native slots inside plugin pages (WGT-030).
- **Look at:** `plux native scan` lists `MapCard` in the catalogue.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-020 — Host API: native action and plugin event

- **Needs:** R-DEVICE (starter)
- **Do:** on a place page run the share action; go back home.
- **Expect:** the native action `sharePlace` runs; the home screen's **Open a link** subtitle shows the last shared place.
- **Why:** hosts register actions and receive plugin events without touching existing code (HST-020, HST-021).
- **Look at:** `Plux.events` in the host code (`lib/src/starter_app.dart`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-021 — Consent

- **Needs:** R-DEVICE (starter)
- **Do:** switch **Share usage analytics** off, tap around, on, tap around.
- **Expect:** with consent off no behavioural events leave the device (error and sync events still do); turning it off deletes buffered events.
- **Why:** consent gates analytics (ANL-002, SEC-090).
- **Look at:** Studio **Analytics** counts only the consented period.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-022 — Actions: sequence, branch, error path

- **Needs:** R-DEVICE-SHOP, R-SCREENS
- **Do:** build a graph in Studio: set state, branch on a condition, call a failing step with an `onError`; publish; tap.
- **Expect:** the branch taken follows the state; the failing step takes `onError`; the run is traced.
- **Why:** declarative actions with defined error paths (ACT-001, ACT-020).
- **Look at:** `action_run` events with `result: failed` carry the graph and step, never values.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-023 — Action limits

- **Needs:** R-DEVICE-SHOP
- **Do:** publish a graph that loops without end; tap it.
- **Expect:** the run ends at the step or run limit (`PLX-5001`/`PLX-5002`), the UI stays responsive.
- **Why:** actions are bounded (LIM-001).
- **Look at:** the limit values in the limits registry.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-024 — State: scope, bindings, persistence

- **Needs:** R-DEVICE-SHOP, state `cart.count`
- **Do:** bind a text to `cart.count`, a button that increments it, mark it persistent; tap, kill, restart.
- **Expect:** only the bound widget rebuilds on change; the value survives restart; a sensitive entry in secure storage is not in plain files.
- **Why:** fine-grained rebuilds and typed persistence (STA-001, STA-020).
- **Look at:** the **State** tab shows the value; the rebuild-counters overlay (L2-DEV-015).
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-025 — Forms and validators

- **Needs:** R-DEVICE (starter) or a form page in R-APP
- **Do:** build a form with a required field and a number range; submit empty, then out of range, then valid.
- **Expect:** inline errors at the right fields; submit enabled only when valid.
- **Why:** forms validate on the device.
- **Look at:** the error text comes from translations (switch language).
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-026 — Data source with cache

- **Needs:** R-DEVICE-SHOP, the data source of L2-STU-027 pointed at a reachable API (or `plux mock`)
- **Do:** bind a list to the source; load; go offline; reload.
- **Expect:** data appears; offline shows cached data or the source's error state; no crash.
- **Why:** typed data with caching and offline reads (DAT-010, DAT-020).
- **Look at:** `PLX-5101` offline; `PLX-5100` for a domain the plugin did not declare.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-027 — Offline outbox

- **Needs:** L2-RUN-026, a write operation marked offline-capable
- **Do:** go offline; trigger the write three times; go online.
- **Expect:** writes queue and replay in order once online.
- **Why:** mutations survive a lost connection (DAT-030).
- **Look at:** `PLX-5120` if the outbox fills.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-028 — Live data

- **Needs:** a stream source (SSE or WebSocket) from `plux mock` or your own server
- **Do:** bind a banner to the stream; send events.
- **Expect:** the banner updates live; a refused stream reconnects with back-off.
- **Why:** live sources are first-class.
- **Look at:** `PLX-5110` on 401/403.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-029 — Local database

- **Needs:** R-DEVICE-SHOP, collection `items` (L2-STU-029), the `plux_db_drift` package
- **Do:** write rows from an action; watch a query bound to a list; kill and restart; change the collection and publish.
- **Expect:** the list updates on write; data survives restart; the migration runs on update.
- **Why:** local persistence with watched queries and migrations (DB-001, DB-007).
- **Look at:** `make bench-db` measures write-to-update time.
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-030 — Animation and reduce motion

- **Needs:** R-DEVICE-SHOP
- **Do:** add an implicit animation (opacity) and a page transition in Studio; publish; run; then switch the system's reduce-motion setting on.
- **Expect:** animations play smoothly; with reduce motion they are replaced by instant changes.
- **Why:** motion respects accessibility (ANI-001).
- **Look at:** no jank in the performance tab (L2-DEV-016).
- **Result:** ☐ pass ☐ fail — notes:

### L2-RUN-031 — Disk quota and corrupt store

- **Needs:** R-DEVICE-SHOP
- **Do:** clear the app's storage in system settings; start it offline.
- **Expect:** the runtime starts from the embedded baseline; no crash.
- **Why:** the baseline is the floor (SYN-007).
- **Look at:** the runtime runbook row "A device lost its store".
- **Result:** ☐ pass ☐ fail — notes:

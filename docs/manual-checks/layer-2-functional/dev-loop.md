<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Dev app and live debugging

You install the Plux Dev app, pair it from Studio and from the CLI, watch edits reach the device in
well under a second, and read the streams a device sends back: logs, action traces, state, network,
function calls, sync and performance. Guides: [quick start](../../guides/quick-start.md),
[Dev app README](../../../apps/plux_dev/README.md).

A dev session never touches a release and is refused for a production environment (`PLX-9203`).

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-DEVAPP | The Plux Dev app installed on R-DEVICE | L2-DEV-001 |
| R-SESSION | A dev session for R-APP in `development`, device paired | L2-DEV-003 |

### L2-DEV-001 — Build and install the Dev app: create R-DEVAPP

- **Needs:** R-STACK, R-DEVICE, R-CLI
- **Do:**
  1. `plux pull -C ~/plux-checks/shop --env development --channel development -o /tmp/baseline` and keep the `keys.json` it writes.
  2. `make dev-app-build DEV_APP_TARGET=apk DEV_APP_KEYS=/tmp/baseline/keys.json`
  3. Install the APK on the emulator or phone and open it.
- **Expect:** the app starts and asks you to sign in; built without the keys it says "No development roots configured".
- **Why:** the Dev app pins development roots only (DEV-001, DEV-050, SEC-056).
- **Look at:** `adb shell pm list packages | grep dev.plux.dev`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-002 — Sign in on the device and choose the app

- **Needs:** R-DEVAPP
- **Do:** in the Dev app sign in with a device code (approve it in Studio), choose app `shop`.
- **Expect:** the Dev app shows the pairing status "not paired", with **Scan QR** and **Paste link**.
- **Why:** the same device grant as the CLI (CLI-002, DEV-002).
- **Look at:** Studio **Administration**, **Sessions**: a session for client `plux-dev`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-003 — Pair from Studio: create R-SESSION

- **Needs:** R-DEVAPP, R-STUDIO
- **Do:**
  1. In Studio open **Devices** of `shop` and choose **Pair a device**.
  2. Scan the QR code in the Dev app (or paste the link, or type the **Short code**).
  3. Let a code expire once and choose **New code**.
- **Expect:** the device shows the `shop` development draft; the page lists the paired device; an expired code is refused.
- **Why:** pairing is single-use and short-lived (DEV-003, SEC-020).
- **Look at:** the pairing link starts `plux-dev://pair?server=…&token=…`; use it twice, the second fails.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-004 — Live push of a Studio edit

- **Needs:** R-SESSION
- **Do:** in Studio edit the text of the `list` screen and watch the device; note the autosave-to-device time on the canvas.
- **Expect:** the device shows the change without a restart, in well under a second on a local network.
- **Why:** edit-to-device latency is a requirement (NFR-030, DEV-010).
- **Look at:** `make bench-devloop` prints the measured latency of 200 edits.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-005 — Live push from the CLI

- **Needs:** R-PROJECT, R-DEVAPP
- **Do:**
  1. `cd ~/plux-checks/shop; plux dev --env development` and pair the device with the printed link.
  2. Edit `translations/en.json` and save.
- **Expect:** the CLI prints `pushed 1 document in …ms` and `device …: applied`; the device updates.
- **Why:** developers without Studio get the same loop (DEV-010, DEV-011).
- **Look at:** Ctrl-C ends the session; the device returns to its release.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-006 — A bad edit is not sent

- **Needs:** L2-DEV-005
- **Do:** with `plux dev` running, make a page invalid and save.
- **Expect:** diagnostics with the JSON path are printed; nothing is sent; the device keeps the last good page; fixing the file sends it.
- **Why:** a broken draft never reaches a device (CMP-040).
- **Look at:** the device screen does not flicker.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-007 — Send a screen with a chosen language, theme and scale

- **Needs:** R-SESSION
- **Do:** in Studio **Devices**, **Send to device**: route `detail`, language `ar`, theme dark, text scale 1.5.
- **Expect:** the device opens `detail` mirrored, dark, larger.
- **Why:** reproduce a state on a real device (DEV-011).
- **Look at:** `plux dev open <route> --locale ar --theme dark --text-scale 1.5` does the same.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-008 — Device drawer: logs and problems

- **Needs:** R-SESSION
- **Do:** open the device drawer tabs **Problems**, **Logs**; filter by level; **Pause**; resume.
- **Expect:** lines appear as the device acts; the filter and pause work.
- **Why:** streaming logs replace `adb logcat` (DEV-020, DEV-021).
- **Look at:** the same lines in `plux dev logs`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-009 — Process flow: action traces

- **Needs:** R-SESSION, a button with an action (L2-STU-019)
- **Do:** tap the button on the device; read **Process flow** in Studio; then `plux dev logs --filter action`.
- **Expect:** the run with trigger, steps indented, timings and the node path that started it; a failing step shows its error code.
- **Why:** you see what ran, and why (ACT-030).
- **Look at:** sensitive values are not shown (ACT-031).
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-010 — State stream and setting state

- **Needs:** R-SESSION, state entry `cart.count`
- **Do:** read **State**; `plux dev set app cart.count 5`; try `plux dev set app <sensitive path> '"x"'`.
- **Expect:** the state tab shows the old and new value; the device UI follows; the sensitive path is refused (`PLX-9206`).
- **Why:** state is inspectable and drivable, secrets are not (SCH-012).
- **Look at:** the `PLX-9206` text.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-011 — Network stream and profiles

- **Needs:** R-SESSION, a data source (L2-STU-027)
- **Do:** open the **Network** tab; `plux dev network offline`; trigger a load; `plux dev network 3g`; `plux dev network good_wifi`.
- **Expect:** requests with status and timing; offline shows the source's error state; the profiles change timings; `good_wifi` restores.
- **Why:** test bad networks without leaving the desk (DEV-031).
- **Look at:** profile names: good_wifi, offline, 2g, 3g, high_latency, packet_loss.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-012 — Mocks

- **Needs:** R-SESSION, a mocked source
- **Do:** `plux dev mock <source> error`, then `empty`, then `success`.
- **Expect:** the screen shows the matching state each time.
- **Why:** design every data state on a real device (DEV-031).
- **Look at:** the **Data state** switch in Studio shows the same states.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-013 — Clock and lifecycle

- **Needs:** R-SESSION
- **Do:** `plux dev clock set 2030-01-01T00:00:00Z`; `plux dev clock advance 90m`; `plux dev clock reset`; `plux dev lifecycle pause`, `resume`, `memory-pressure`.
- **Expect:** dates shown on the device follow the fake clock; the app survives pause, resume and memory pressure.
- **Why:** time- and lifecycle-dependent behaviour is testable (DEV-031).
- **Look at:** each command prints `command <id> sent to <devices>` and each device's status.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-014 — Evaluate PXL live

- **Needs:** R-SESSION
- **Do:** `plux dev pxl 'state.cart.count + 1'`.
- **Expect:** the result of the expression on the live state.
- **Why:** expressions are debugged against real data (DEV-032).
- **Look at:** a type error prints a `PLX-` code, not a crash.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-015 — Screenshot, inspect, overlays

- **Needs:** R-SESSION
- **Do:** `plux dev screenshot --out /tmp/s.png`; `plux dev inspect <node-path>`; `plux dev overlay touch-targets on`; `off`.
- **Expect:** a PNG of the device; the node highlighted; the overlay appears and disappears. Studio's **Capture screenshot** and **Highlight selection on device** do the same.
- **Why:** on-device inspect mode (DEV-033, DEV-034).
- **Look at:** overlays: contrast, frame-chart, rebuild-counters, touch-targets.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-016 — Function calls, sync and performance streams

- **Needs:** R-SESSION; best after L2-FN-008
- **Do:** open **Functions**, **Sync**, **Performance** in the drawer; trigger a sync (pull to the sync tile) and a function call.
- **Expect:** function calls with the server's own log lines merged; sync steps; frame timings.
- **Why:** one place for every device signal.
- **Look at:** `plux dev logs --filter function_call`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-017 — Export a trace and read it offline

- **Needs:** R-SESSION
- **Do:** `plux dev trace export --out /tmp/bug.trace`; stop the stack; `plux dev trace show --filter action /tmp/bug.trace`; start the stack.
- **Expect:** the same events are readable offline; values marked sensitive are redacted.
- **Why:** bug reports carry evidence (DEV-021, SEC-092).
- **Look at:** grep the file for a password-like value: none.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-018 — Simulated security detections

- **Needs:** R-SESSION (an app using `plux_security`)
- **Do:** `plux dev security-detect root`; then `debugger`.
- **Expect:** the app responds as its security profile says (warn, degrade or block); the answer lists the device status.
- **Why:** try the app's reaction without rooting a phone (DEV-031, SEC-070).
- **Look at:** the assurance level in Studio **Devices** after the detection.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-019 — Devtools are absent from release builds

- **Needs:** Flutter, repository checkout
- **Do:** `make release-inspect-selftest`; and, with the Android SDK, `make release-inspect PLATFORM=android`.
- **Expect:** the self-test passes on a leaky and a clean fixture; the real inspection finds no devtools code, dev-only logging or source maps.
- **Why:** debugging tools never ship (DEV-050, SEC-074).
- **Look at:** the inspection's list of checked patterns.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-020 — Production environment refused

- **Needs:** R-CLI
- **Do:** `plux dev --env production`.
- **Expect:** refused with `PLX-9203`; no session is created.
- **Why:** live push can never reach production.
- **Look at:** the message names the environment.
- **Result:** ☐ pass ☐ fail — notes:

### L2-DEV-021 — Unpair and end

- **Needs:** R-SESSION
- **Do:** revoke the paired device in Studio **Devices**; edit a text.
- **Expect:** the device stops receiving edits and returns to its release.
- **Why:** pairing is revocable (DEV-003).
- **Look at:** **Audit log** shows the revocation.
- **Result:** ☐ pass ☐ fail — notes:

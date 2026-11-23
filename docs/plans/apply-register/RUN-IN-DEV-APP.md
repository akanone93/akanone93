<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Apply Register on your phone, live from Studio

Run the imported app in the **Plux Dev app** (`apps/plux_dev`) and watch every Studio edit arrive on the
phone as you make it. It assumes the stack, Studio and the app of [RECREATE-LOCALLY.md](RECREATE-LOCALLY.md)
are running. Reference: [Dev app README](../../../apps/plux_dev/README.md), [run-locally guide](../../guides/run-locally.md) §6–§7.

## 1. What you need once

- Flutter at the version pinned in the `Makefile` (`FLUTTER_VERSION`), and an Android phone with USB debugging (or an emulator).
No keys and no release are needed: the Dev app is built with none. When the phone is connected it pins your
organisation's development root, and every app opens with the development key that root certifies
([ADR-0109](../../adr/0109-organisation-development-root-for-the-dev-app.md)).

## 2. Build the Dev app

From the repository root:

```bash
make dev-app-defines DEV_APP_CA=deploy/compose/.secrets/ca/ca.crt
```

```bash
cd apps/plux_dev && flutter run --debug --dart-define-from-file=dev-defines.json
```

`dev-defines.json` holds the stack's certificate authority; it is ignored by git. With the checkout on a
Windows drive (`D:\…` in Windows, `/mnt/d/…` in WSL), run `make dev-app-defines` in WSL (where `make` and
the stack's `ca.crt` are) and `flutter run` in Windows: both see the same folder, so nothing needs copying.

## 3. Let the phone reach the stack

The development certificate is valid for `localhost` only, so the phone reaches the server through adb:

```bash
adb reverse tcp:8080 tcp:8080
```

`make dev` does this for you when it sees the phone; run it yourself if adb lives on another system (Windows
with the stack in WSL), and again after every reconnect of the cable.

## 4. Connect the phone, once

In Studio: **Profile › Phones › Connect a phone** (your security key first). In the Dev app: scan the code,
then tap the number Studio shows. The code also carries your organisation's development root, which the phone
keeps. A phone connected with an older Studio asks to be connected again.

## 5. The live loop

1. In Studio, open a plugin of Apply Register (app canvas › double-click a plugin).
2. In the Dev app, tap **Apply Register** (marked live): the first time in that Studio session, tap the number
   Studio shows. The app opens at its entry screen.
3. Edit in Studio: every saved change reaches the phone (the canvas shows the autosave-to-device time).
   **Send** in the plugin canvas opens a chosen screen with a language, theme and text scale.
4. The Dev app's floating status button shows Live, Connecting, Reconnecting or Offline, with Reconnect,
   diagnostics and Leave. Studio's **Developer console** shows the phone's logs, state, network and sync.

## 6. If it does not connect

| You see | Do |
|---|---|
| "Connect this phone again from Studio" | §4 again: the phone was connected before it kept your organisation's development root |
| "… was not opened: its key isn't certified …" | the app's key does not come from your organisation's root (another server, or a root replaced in the signing backend): connect the phone again (§4) |
| A certificate error | `DEV_APP_CA` must be the stack's `ca.crt`; rebuild after `make dev` created a new one |
| The app never goes Live | `adb reverse tcp:8080 tcp:8080` again; the phone must reach `https://localhost:8080` |
| The app is not in the list | open one of its plugins in Studio first; the list shows apps open in Studio |
| Data screens show errors | the local `ar-api` (`GOWORK=off go run .` in the kit's `ar-api`) must be running |

# Quick Start: From `flutter create` to a Plux Page on a Device in Ten Minutes

The shortest path from nothing to a published Plux page on your own device, and then the
loop you will live in: edit a page, save, see it on the device. It is the path the
requirements `HST-034` and `DX-001` describe. The time budget of each step is on the
step; they add up to ten minutes on a machine with the prerequisites in place. The
steps that need no device run, timed, with `make quickstart-dry-run`; whether people
really manage in ten minutes is decided by recorded usability sessions, not by that
script. Background on the concepts: [concepts](concepts.md).

| Step | Budget |
|---|---|
| 1. Create the app | 1 min |
| 2. Check it builds | 2 min |
| 3. Start a dev session and pair the device | 2 min |
| 4. Run the app and see the starter page | 2 min |
| 5. Edit a page and watch it change | 1 min |
| 6. Publish and promote | 2 min |

## Prerequisites

- Flutter at the version the repository pins (3.47 or newer), and a device, emulator or
  simulator. Check with `flutter doctor`.
- The `plux` CLI on your `PATH`, signed in to your Plux Server: `plux login --server
  https://plux.example.com`. A project with an app and its first release is described in
  [Publish a page and see it on a device](first-release.md); on your own machine,
  [Run it locally](run-locally.md) starts the whole stack with `make dev`.
- Until `plux_flutter` and the router adapters are published, a checkout of this repository
  to point the project at (`--plux-path`, below).

## 1. Create the app (1 minute)

Either start from your own app, or let Plux generate one.

**Generate one.** `plux create` pulls the app's release and writes a ready-to-build Flutter
project with the runtime wired in, the root keys embedded and the release as its baseline.
The `--router` flag picks how the app navigates: `none` (the default), `go_router` or
`auto_route`.

```bash
plux create my-app --router go_router --out my_app --plux-path /path/to/plux3/packages/plux_flutter
cd my_app
```

**Use your own.** In an app made with `flutter create`, `plux init` adds the runtime and
the router adapter your app already uses, writes `plux.yaml` and the options with your
environment's root keys, and wires `main.dart`; `plux pull` writes the baseline:

```bash
flutter create my_app && cd my_app
plux init --server https://plux.example.com --org my-org --app my-app --env development
plux pull --env development --channel development
```

The [host app guide](host-app.md) explains what each file does and what the host controls.

## 2. Check that it builds (2 minutes)

```bash
flutter pub get
flutter analyze
```

Both must pass before anything runs on a device; if they do not, the diagnostics name the
file. The first `flutter pub get` fetches packages and is the slowest step of the guide on a
cold machine.

## 3. Start a dev session and pair the device (2 minutes)

In the directory of your Plux project (the pages, actions and translations; the
[starter project](../../schema/testdata/documents/starter) is a small one to copy), start a
dev session:

```bash
plux dev
```

`plux dev` opens a session for your non-production environment, prints a pairing link
(`plux-dev://pair?server=…&token=…`) and then watches the directory. The link carries a
single-use token: show it only to the device you are pairing. Leave the command running;
Ctrl-C ends the session.

Pair the device in one of two ways:

- **The Plux Dev app.** Install it on the device ([apps/plux_dev](../../apps/plux_dev/README.md)),
  sign in, choose the app, and either scan a QR code made from the link or paste the link.
  The Dev app has every first-party optional package built in, so it previews any plugin
  without a build of your own.
- **Your own debug build.** Add `plux_devtools` to your app, wrap its content in
  `PluxDevtools(session: PluxDevSession(devRootKeys: …), …)`, open the overlay, choose
  *session* and paste the link. This runs your own host code, your native widgets and your
  own navigation against the draft. Pin the **development** root keys, never the
  production ones; in a release build every part of this is absent (`DEV-050`). Details:
  [`plux_devtools`](../../packages/plux_devtools/README.md).

If the link expires before you use it, `plux dev` prints a new one.

## 4. Run the app and see the starter page (2 minutes)

```bash
flutter run
```

The app starts on its embedded baseline release, so a page is on screen before any network
call. Once paired, the session's draft replaces it on the device: the page you are about to
edit is the one you see.

## 5. Edit a page and watch it change (1 minute)

Change a text in the project, for example in `translations/en.json`, and save. `plux dev`
validates and compiles the change, sends the bundle to every paired device, and prints what
each device did with it:

```text
pushed 1 document in 112ms (bundle 3fa91c04)
  translations/en.json
  device d1: applied, pages home
```

The device applies the bundle without a restart. A change that does not compile is not sent;
its diagnostics are printed with the JSON path of the problem, and the device keeps the last
good page.

## Debug what you see

While the session runs, the other end of the loop is open in a second terminal:

```bash
plux dev logs                      # every event of the session, one line each
plux dev logs --filter action      # action runs, with their steps and the node that started them
plux dev logs --filter function_call
plux dev trace export --out bug.trace
plux dev trace show bug.trace      # read the same events later, offline, without a server
```

An action that fails shows the graph, the node path, the failing step and its error code;
a state change shows the path with its value before and after; a server function call
shows the server's own log lines merged into it. You can also drive the device:
`plux dev open <route>`, `plux dev set`, `plux dev mock`, `plux dev network <profile>` and
`plux dev screenshot --out screen.png` ([CLI reference](../reference/cli.md)).

## 6. Publish and promote (2 minutes)

A dev session never touches a release. When the page is right:

```bash
plux validate .
plux publish --promote development
```

Devices on that channel pick the release up at their next sync, verify it, and show it at
the next time no Plux page is on screen ([Publish a page and see it on a device](first-release.md)
walks through what the device does). To embed the new release in your app as its baseline,
run `plux pull` again.

## Check the scripted part

The steps that need no device and no human can be timed on your machine:

```bash
make quickstart-dry-run
```

It generates the starter with `plux create --router go_router`, runs `flutter pub get` and
`flutter analyze` on it, and validates and builds the starter Plux project with `plux
validate` and `plux build`. It prints the timings of the steps as JSON and fails when
the scripted part takes more than five minutes. It does not replace the recorded usability
sessions, which decide whether `HST-034` and `DX-001` are met.

## What to try next

- Break the page on purpose and read the diagnostic the compiler gives.
- Make an action fail, and find it with `plux dev logs --filter action`.
- Read the [host app guide](host-app.md) for what the host controls, and the
  [routing guide](routing.md) for how pages and the router adapters fit together.

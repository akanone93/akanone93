# Run It Locally

Everything you can run on your machine, in one place. Each section says what it is for and
the command to run; the linked pages have the details. Every task is a `make` target, run
from the repository root, and `make help` lists them all.

## 1. Set up once

- Install Go (the toolchain in `backend/go.mod`), Flutter 3.47, Bun 1.3, Python 3, GNU Make 4
  or later, and Docker.
- On Windows, use Git Bash with GNU Make 4 (`winget install ezwinports.make`); GnuWin32's
  make 3.81 is refused. On macOS, `brew install make` and run `gmake`, or put its `gnubin`
  first on your `PATH`.
- Install the pinned tools and git hooks: `make setup`.
- Only for running tests: start the test database with `make test-db` (Docker), and export
  the `PLUX_TEST_DATABASE_URL` it prints. Each test creates and migrates its own schema in
  it. Remove it with `make test-db-down`. Running the stack and the app (§2) does not need
  it: the stack's server migrates its own database on start.

## 2. The whole loop: server, app, publish, update

Start here. It shows everything Phase 3 delivers.

1. Start an Android emulator, an iOS simulator or a phone.
2. Run `make dev`. It starts the server stack, seeds the starter app, pulls its first
   release into the app and runs the app with hot reload (`r` reload, `R` restart,
   `q` quit).
3. In the app: a native screen with a Plux page inside it, a sync tile, **Open welcome**
   (the same page full screen), consent and theme switches, and the devtools overlay.
4. Change the page, publish it and see the device update: steps 3–5 of
   [Publish a page and see it on a device](first-release.md).
5. Stop the stack (`make compose-down`) and restart the app: it still renders the release,
   offline.

Also: `make dev-starter` points the app at the stack again and pulls a fresh baseline;
`make dev-app` runs only the app.

## 3. The server stack on its own

- Start and stop it: `make compose-up`, `make compose-down`. Seed the sample apps:
  `make compose-seed`.
- Addresses: API <http://localhost:8080>, Grafana <http://localhost:3000>, Prometheus
  <http://localhost:9090>; object storage (SeaweedFS) is reachable only inside the Compose
  network.
- Seeded credentials: `deploy/compose/.secrets/dev.env`. More:
  [deploy/compose](../../deploy/compose/README.md).

## 4. The whole stack with Studio

This is the full loop on one machine: PostgreSQL, Valkey, object storage and `plux-server`,
Studio's backend for frontend (BFF) and web app, a device, and a release. Docker must be
running. Each step names the file the command comes from.

1. **Install once.** `make setup` (pinned tools and git hooks), then `make studio-install`
   (`bun install --frozen-lockfile` in `studio/`). Bun 1.3 is required (`studio/package.json`).
2. **Start the stack and seed it.**

   ```bash
   make compose-seed
   ```

   It generates credentials into `deploy/compose/.secrets/` on first run, builds the server and
   Studio images and starts every service of `deploy/compose/compose.yaml`: `plux-server`
   (API on <https://localhost:8080>, TLS under a development CA), PostgreSQL, SeaweedFS,
   Valkey, the function roles, OpenTelemetry, Prometheus, Grafana and `studio` (the BFF on
   <http://localhost:8081>). The server migrates its own database on start
   (`migrateOnStart: true` in `deploy/compose/plux-server.yaml`; `plux-server migrate` runs the
   migrations on their own). On first run the seed step (`plux-server seed`) then creates an
   administrator `dev@plux.localhost`, the organisation `acme`, the apps `demo` (the loan
   calculator, promoted to `staging`) and `starter`, and writes the IDs, the password and a
   command-line token to `deploy/compose/.secrets/dev.env` (`PLUX_DEV_ORGANIZATION`,
   `PLUX_DEV_APP`, `PLUX_DEV_STARTER`, `PLUX_DEV_PASSWORD`, `PLUX_DEV_TOKEN`).
   `make compose-up` starts the same stack without seeding; `make dev` does both and adds hot
   reload (§2).
3. **Open Studio and sign in.** Browse to <http://localhost:8081>, sign in with
   `dev@plux.localhost` and the password from `dev.env`. The BFF serves the web app and
   forwards `/api` to the server; the session cookie and CSRF token are the server's. Probe
   it with `curl http://localhost:8081/livez` (the process) and `/readyz` (also asks the
   server); the [BFF runbook](../operations/studio-bff.md) lists every setting and failure.
4. **Work on Studio itself (optional).** For development with hot reload, stop the container
   that holds port 8081 and start the dev servers:

   ```bash
   docker compose -f deploy/compose/compose.yaml stop studio
   make studio-validator   # once: the validator WebAssembly module and its loader
   make studio-dev
   ```

   `make studio-dev` runs the BFF (`bun --watch apps/bff/src/main.ts`) and Vite together, with a
   fresh `PLUX_BFF_STATE_SECRET`, `PLUX_BFF_API_UPSTREAM=https://localhost:8080`,
   `PLUX_BFF_PUBLIC_BASE_URL=http://127.0.0.1:5173` and the development CA
   (`deploy/compose/.secrets/ca/ca.crt`) in `NODE_EXTRA_CA_CERTS`. Open
   <http://127.0.0.1:5173>. Vite forwards `/api`, `/auth`, `/version`, `/telemetry` and
   `/csp-report` to the BFF on `127.0.0.1:8081` (`PLUX_STUDIO_DEV_BFF` changes the target), so
   the dev app passes through the same session and CSRF rules as production.
5. **Organisations, apps and plugins.** The seed made the organisation and the apps, and you can
   make more in Studio ([canvas guide](../guides/studio/canvas.md)). An installation
   administrator (the seeded user is one) chooses **New organisation** in the top bar, or
   **Create organisation** on the empty page of someone who belongs to none. Anyone with the
   `app.manage` permission chooses **Create app** on the organisation home and lands on the new
   app's canvas, where **New plugin** adds its first plugin. Without Studio, the API has
   `OrganizationService.CreateOrganization` and `AppService.CreateApp`, and on an empty database
   `plux-server seed -org <key> -app <key> -starter <key>` makes new ones.
6. **Connect a device.** Two apps connect to the stack:
   - The **starter app** (`apps/starter`): `make dev-app` (or `make dev`) writes its defines,
     pulls the `staging` baseline and runs it with hot reload. On an Android emulator or a
     phone on USB it runs `adb reverse tcp:8080 tcp:8080` first.
   - The **Plux Dev app** (`apps/plux_dev`), which previews unpublished drafts: build it with
     `make dev-app-build DEV_APP_TARGET=apk DEV_APP_KEYS=<keys.json>`, where `keys.json` is the
     development root keys of the installation as `plux pull` writes them
     ([Dev app README](../../apps/plux_dev/README.md)). In the app, sign in with a device
     code, choose the app, and pair.
7. **The device loop.** In Studio, open **Devices** of the app and choose **Pair a device**:
   scan the QR code with the Dev app, or paste the link or the short code. Then every saved
   edit reaches the device (the canvas shows the autosave-to-device time), **Send to device**
   opens a screen with a chosen language, theme and text scale, and the device drawer shows
   logs, state, network and sync ([devices guide](../guides/studio/security-and-admin.md)). The
   command-line equivalent is `plux dev` ([quick start](quick-start.md)).
8. **Publish a release.** In Studio, **Publish** walks the steps *Select plugins*, *Freeze and
   build*, *Reports*, *Visual diffs*, *Changelog*, *Target*, *Approval*, *Rollout plan* and
   *Release* ([publishing guide](../guides/studio/publishing.md)). From the command line:

   ```bash
   . deploy/compose/.secrets/dev.env
   cd backend && PLUX_TOKEN=$PLUX_DEV_TOKEN \
     go run -exec "env SSL_CERT_FILE=../deploy/compose/.secrets/ca/ca.crt" ./cmd/plux publish \
     -C <project> --server https://localhost:8080 --org "$PLUX_DEV_ORGANIZATION" \
     --app "$PLUX_DEV_APP" --env staging --promote staging
   ```

   Then watch the release on the **Releases** page and on the device
   ([first release](first-release.md)).
9. **Stop.** `make compose-down` keeps the volumes; delete them with
   `docker compose -f deploy/compose/compose.yaml down -v` and remove `deploy/compose/.secrets/`
   to start from nothing.

## 5. The CLI

From `backend/`:

```bash
go run ./cmd/plux validate <project>        # compile locally; errors carry their JSON path
go run ./cmd/plux build <project> --json    # inspect the compiled bundles
go run ./cmd/plux publish …                 # also: keys, pull, release
```

`make build` puts the `plux` and `plux-server` binaries in `bin/`.

## 6. Tests and quality gates

- Everything CI runs: `make check`. By area: `make go-check`, `make dart-check`,
  `make studio-check`, `make repo-check`.
- Unit tests only: `make test`.
- Go: `make go-test-race`, `make go-cover` (with coverage floors),
  `make go-fuzz FUZZTIME=30s`, `make go-determinism`.
- Dart: `make dart-test`, `make dart-cover` (three packages at a time; `DART_JOBS=1` for one),
  or `flutter test` in a package.
- Against the real stack (Docker): `make compose-test`.

How tests are organised: [testing](../engineering/testing.md).

## 7. End-to-end

- The starter app's flows against a server built from source, on this machine (about
  30 s): `make e2e-starter`.
- Old and new runtimes against old and new servers: `make compat`.
- On an emulator or simulator, as CI runs them: `make e2e-android`, `make e2e-ios` (slow;
  they create their own device). Details: [test/e2e](../../test/e2e/README.md).

## 8. Benchmarks and size

- Rendering and start-up, in profile mode: `make bench-runtime` (under xvfb on Linux).
- Against another commit, failing beyond 10%: `make bench-runtime-ab BASE=main`.
- On your own phone, for the reference-device numbers:
  [test/bench/runtime](../../test/bench/runtime/README.md).
- Sync on a simulated slow network: `make bench-sync`.
- What the runtime adds to an app: `make size-android` (Android SDK), `make size-ios`
  (Xcode). History: [size journey](../benchmarks/size.md).
- That a release build holds no dev tooling, dev-only logging or source maps: `make release-inspect`
  (`PLATFORM=android` or `ios`; [test/release](../../test/release/README.md)).

## 9. Documentation site

Build it, checking every internal link: `make docs-site`. Preview it:
`cd site && bun run preview`.

## When something fails

- **A download fails while building the server image** (`make dev`, `make compose-up`):
  the build fetches pinned sources and retries; a network that keeps resetting connections
  (VPN, proxy) still stops it. Run the command again.
- **The app cannot reach the server.** On Android (emulator or a phone on USB),
  `make dev-app` forwards port 8080 with `adb reverse`; check that `adb` is on your path.
  On an iPhone, point the app at your machine:
  `make dev-starter DEV_ENDPOINT=http://<your machine's address>:8080`.
- **Studio does not load, or `make studio-dev` says the port is in use.** The Compose `studio`
  container holds `127.0.0.1:8081`; stop it as in §4 step 4. Other BFF failures:
  [BFF runbook](../operations/studio-bff.md).
- **What can go wrong on a device:** [runtime runbook](../runbooks/runtime.md).

<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Command line

You build the `plux` command, sign in, export the app from Studio as a project, validate, build,
publish, promote and roll back it, generate the host app for a device and exercise the rest of the
commands. Everything runs from a shell; nothing prompts. Reference: [CLI reference](../../reference/cli.md).

Run `plux <command> -h` at any time for the flags. Flags come before positional arguments.

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-CLI | `bin/plux` built and on `PATH`, signed in to the stack | L2-CLI-001, L2-CLI-002 |
| R-PROJECT | The Git-layout project of R-APP in `~/plux-checks/shop` | L2-CLI-010 |
| R-SHOPAPP | The generated Flutter project `~/plux-checks/shop-app` | L2-CLI-012 |

### L2-CLI-001 — Build the CLI: create R-CLI

- **Needs:** R-STACK
- **Do:**
  1. `make build`
  2. `export PATH=$PWD/bin:$PATH; plux version; plux help`
  3. `source <(plux completion bash)` (or your shell); type `plux va` and press Tab.
- **Expect:** `version` prints version, commit, date and platform; `help` lists the commands; Tab completes `validate`.
- **Why:** one binary, reproducible (CLI-001, CI-006).
- **Look at:** `make go-reproducible` builds twice and compares.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-002 — Log in with the device grant

- **Needs:** R-CLI, R-STUDIO
- **Do:**
  1. `export SSL_CERT_FILE=$PWD/deploy/compose/.secrets/ca/ca.crt`
  2. `plux login --server https://localhost:8080`
  3. Open the printed URL in Studio, check the code, approve.
- **Expect:** the CLI prints a code and URL and waits; after approval it says it stored the token (OS credential store or a user-only file).
- **Why:** no password ever touches the CLI (CLI-002, ADR-0028).
- **Look at:** with no keyring the file `<user config dir>/plux/credentials.json` has mode 0600.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-003 — Who am I

- **Needs:** R-CLI
- **Do:** `plux whoami --server https://localhost:8080`; then `plux whoami --org acme --server https://localhost:8080`.
- **Expect:** your user and organisations; with `--org`, your permissions there.
- **Why:** you see which authority a command runs with (CLI-002).
- **Look at:** the permission list matches the role of R-ADMIN (`owner`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-004 — Where the server, organisation and app come from

- **Needs:** R-CLI
- **Do:** run `plux whoami` with `PLUX_SERVER` set; then in a directory with a `plux.json` naming the server; then with `--server` overriding both.
- **Expect:** flag wins over variable, variable over `plux.json`.
- **Why:** predictable configuration for scripts (CLI-003).
- **Look at:** the server in the first line of each output.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-005 — Doctor

- **Needs:** R-CLI, R-PROJECT (or any project dir)
- **Do:** `plux doctor -C ~/plux-checks/shop`; then stop the stack (`make compose-down`) and run it again; start it again.
- **Expect:** a checklist (project, configuration, `/readyz`, credential, sign-in, app) that fails at the right line when the server is down.
- **Why:** first-line diagnosis for developers (DX-003).
- **Look at:** exit code 4 when the server cannot be reached.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-006 — Validate a good project

- **Needs:** R-CLI, repository checkout
- **Do:** `plux validate schema/testdata/documents/starter; echo $?`
- **Expect:** no errors, exit code 0, works with no server.
- **Why:** CI can check a change offline (CLI-005).
- **Look at:** run it with the stack stopped: same result.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-007 — Validate a broken project, JSON output

- **Needs:** R-CLI
- **Do:** `cp -r schema/testdata/documents/starter /tmp/bad`; change a required property's type in a page; `plux validate --json /tmp/bad; echo $?`
- **Expect:** exit code 1; JSON with `ok:false`, a `diagnostics` list with `code`, `file`, `path`, `message`, `fix`.
- **Why:** machine-readable diagnostics for CI and editors (CMP-040, CLI-005).
- **Look at:** the text form prints `file#pointer[start:end]: severity PLX-nnnn message`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-008 — Build bundles

- **Needs:** R-CLI
- **Do:** `plux build --json -o /tmp/out schema/testdata/documents/starter`; `ls -l /tmp/out/app /tmp/out/plugins`; add `--dev` once.
- **Expect:** `app/<key>.pxb` and `plugins/<key>.pxb`, with size and hash in the JSON; a release build also writes `.sourcemap`; `--dev` marks bundles as development.
- **Why:** bundles are the unit of delivery (CMP-001, CMP-041).
- **Look at:** `development` field in the JSON.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-009 — Same input, same bytes

- **Needs:** L2-CLI-008
- **Do:** build twice to `/tmp/out1` and `/tmp/out2`; `diff -r /tmp/out1 /tmp/out2`; then `make go-determinism`.
- **Expect:** no difference; the make target passes.
- **Why:** reproducible artifacts (CMP-002).
- **Look at:** bundle hashes in both JSON outputs are equal.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-010 — Export R-APP as a project: create R-PROJECT

- **Needs:** R-CLI, R-APP
- **Do:**
  1. `mkdir -p ~/plux-checks/shop; cd ~/plux-checks/shop`
  2. `plux init --server https://localhost:8080 --org acme --app shop --env development`
  3. `plux export -o .`; `ls -R | head -40`
- **Expect:** `plux.json` plus app, plugin, page, translation and test documents as you made them in Studio (`catalog`, `list`, `detail`, locales, the `buy-flow` scenario).
- **Why:** Studio and Git are two views of one project (CLI-004, SCH-042).
- **Look at:** `git init && git add -A && git commit` shows a readable tree.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-011 — Diff and import

- **Needs:** R-PROJECT
- **Do:**
  1. `plux diff -C ~/plux-checks/shop; echo $?`
  2. Edit a text in `translations/en.json`; `plux diff` again.
  3. `plux import -C ~/plux-checks/shop`; reload Studio.
- **Expect:** exit 0 then 1 listing the changed file; after import Studio shows the new text.
- **Why:** edit in Studio or in Git, same drafts (CLI-004).
- **Look at:** Studio **History**: a snapshot for the import.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-012 — Generate the host app: create R-SHOPAPP

- **Needs:** R-PROJECT, R-RELEASE1 promoted to a development channel, Flutter
- **Do:**
  1. `plux create shop --env development --channel development --name "Shop" --application-id com.example.shop --out ~/plux-checks/shop-app -C ~/plux-checks/shop --plux-path <repo>/packages/plux_flutter`
  2. `cd ~/plux-checks/shop-app; ls; flutter pub get; flutter analyze`
- **Expect:** a Flutter project with Android and iOS folders, `lib/main.dart`, `assets/plux/`, `plux.shell.json`, `integration_test/app_test.dart` and build scripts; analyze passes.
- **Why:** a no-code app still needs a store binary, generated (GEN-001, GEN-002).
- **Look at:** run the command twice into two folders and `diff -r`: identical (GEN-003).
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-013 — Pull a baseline

- **Needs:** R-CLI, R-RELEASE1
- **Do:** `plux pull -C ~/plux-checks/shop --env development --channel development -o /tmp/baseline; ls -R /tmp/baseline`
- **Expect:** bundles per plugin and a signed manifest; the printed hash matches **Bundle SHA-256** in Studio.
- **Why:** a fresh install starts offline from a baseline (SYN-007, CLI-004).
- **Look at:** the sequence number equals the Studio release.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-014 — Import an API definition and mock it

- **Needs:** R-CLI
- **Do:**
  1. `plux import openapi backend/internal/importer/testdata/petstore31.yaml -o /tmp/pets.json`
  2. `plux mock backend/internal/importer/testdata/petstore31.yaml --addr 127.0.0.1:9099 &`
  3. `curl -s http://127.0.0.1:9099/` plus a path from the file; stop the mock.
- **Expect:** a data-source fragment as canonical JSON (the same as Studio's import); the mock answers from the examples on loopback only.
- **Why:** typed data from API definitions, mock without a backend (DAT-002).
- **Look at:** run the import twice: identical bytes.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-015 — Publish and promote

- **Needs:** R-PROJECT
- **Do:** change a text, then `plux publish -C ~/plux-checks/shop --env development --promote development --notes "cli check" ; echo $?`
- **Expect:** the project is imported, compiled, signed; a release is created and the channel points at it; the command waits for the signed manifest and prints one JSON-like summary.
- **Why:** publish is one command for CI (CLI-004, REL-001).
- **Look at:** Studio **Releases** shows the release with the notes.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-016 — Release list, promote, rollback

- **Needs:** L2-CLI-015
- **Do:**
  1. `plux release list -C ~/plux-checks/shop --env development`
  2. `plux release rollback -C ~/plux-checks/shop --env development --notes "undo" <earlier sequence>`
  3. `plux release promote -C ~/plux-checks/shop --env development --channel development <sequence>`
- **Expect:** rollback makes a new, higher sequence with older content; promote points the channel at a chosen release and waits for the manifest.
- **Why:** rollback never moves sequences backwards (REL-006).
- **Look at:** the `PLX-3003` runbook text for what a device does with an older sequence.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-017 — Run the tests

- **Needs:** R-PROJECT, R-SCENARIO, Flutter
- **Do:** `plux test -C ~/plux-checks/shop --junit /tmp/junit.xml; echo $?`
- **Expect:** `buy-flow` runs headlessly and passes; a JUnit file is written; a changed expectation makes it fail with exit 1.
- **Why:** declarative tests run on the real runtime (TST-002).
- **Look at:** `/tmp/junit.xml` has one test case per scenario.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-018 — Public keys

- **Needs:** R-CLI
- **Do:** `plux keys -C ~/plux-checks/shop --env development`
- **Expect:** the public keys the environment's manifests and bundles are signed with.
- **Why:** hosts embed the roots they trust (SEC-051).
- **Look at:** the same keys are embedded in `lib/plux/plux_options.g.dart` of R-SHOPAPP.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-019 — Add Plux to your own Flutter app

- **Needs:** R-CLI, Flutter
- **Do:** `flutter create /tmp/own && cd /tmp/own && plux init --server https://localhost:8080 --org acme --app shop --env development --project ~/plux-checks/shop`; run it twice.
- **Expect:** `pubspec.yaml` gains `plux_flutter`; `plux.yaml` and `lib/plux/plux_options.g.dart` appear; `main.dart` is wired or the lines to add are printed; the second run changes nothing.
- **Why:** adoption without rewriting the host (HST-001, CLI-004).
- **Look at:** `git diff` after the second run is empty.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-020 — Native catalogue

- **Needs:** R-SHOPAPP
- **Do:** in R-SHOPAPP `plux native scan`; then `plux native sync -C ~/plux-checks/shop`.
- **Expect:** `plux.catalogue.json` is written without changing app code; sync uploads it; repeating changes nothing.
- **Why:** plugins can use host widgets and routes safely (CLI-006, WGT-032).
- **Look at:** the catalogue keeps its id when the code is unchanged.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-021 — Typed Dart API

- **Needs:** R-PROJECT, R-SHOPAPP
- **Do:** `plux codegen --host ~/plux-checks/shop-app ~/plux-checks/shop`; `cd ~/plux-checks/shop-app && flutter analyze`.
- **Expect:** `lib/plux/plux.g.dart` with typed routes and parameters; analyze passes.
- **Why:** the host calls pages with compile-time types (HST-030).
- **Look at:** rename a route and regenerate: the host fails to compile where it used the old one.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-022 — Server-held configuration

- **Needs:** R-CLI
- **Do:**
  1. `plux config set theme.greeting --tier live --type string --value hello --env development -C ~/plux-checks/shop`
  2. `plux config get theme.greeting -C ~/plux-checks/shop --env development`; `list`; `delete`
  3. Repeat with `--sensitive` and `get`.
- **Expect:** values listed with tier, type, version; a sensitive value is shown without its content.
- **Why:** secrets and live values never ride in bundles (SEC-092, ADR-0061).
- **Look at:** the version rises with each `set`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-023 — Localisation commands

- **Needs:** R-PROJECT
- **Do:** `plux l10n status`; `plux l10n pseudo`; `plux l10n export --locale de --format xliff --out /tmp/de.xlf`; `plux l10n import --locale de --dry-run /tmp/de.xlf`.
- **Expect:** keys per locale by state; pseudo-locales `en-XA`, `ar-XB` written; export and a dry-run import that changes nothing.
- **Why:** translators work in their tools.
- **Look at:** the dry run's report of changed keys.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-024 — Exit codes

- **Needs:** R-CLI
- **Do:** `plux nonsense; echo $?`; `plux validate; echo $?`; `PLUX_TOKEN=bad plux whoami; echo $?`; `plux whoami --server https://localhost:1; echo $?`
- **Expect:** 2 (unknown command), 2 (missing argument), 3 (credential refused), 4 (unreachable).
- **Why:** scripts rely on exit codes (CLI-007).
- **Look at:** the table in the CLI reference.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-025 — No prompts

- **Needs:** R-CLI
- **Do:** run `plux publish </dev/null` in a project and `plux login` with `timeout 5`.
- **Expect:** no command asks a question; `login` prints its code and waits.
- **Why:** every command can run unattended (CLI-007).
- **Look at:** the exit status after the timeout.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-026 — Logout

- **Needs:** R-CLI
- **Do:** `plux logout --server https://localhost:8080; plux whoami --server https://localhost:8080; echo $?`; then `plux login` again.
- **Expect:** exit 3 after logout; login works again.
- **Why:** credentials can be forgotten (CLI-002).
- **Look at:** the stored token is gone from the credential store or file.
- **Result:** ☐ pass ☐ fail — notes:

### L2-CLI-027 — Command groups exist

- **Needs:** R-CLI
- **Do:** run `plux <group> -h` for `fn`, `dev`, `rollout`, `control`, `flags`, `experiment`, `policies`, `approvals`, `breakglass`, `freeze`, `roles`, `grants`, `teams`, `restrictions`, `access`, `sessions`, `tokens`, `compliance`, `audit`, `analytics`, `security`, `golden`, `dsr`, `notifications`, `license`.
- **Expect:** each prints a usage text that names its subcommands (exit status 0 or 2), never "unknown command"; later files exercise them.
- **Why:** the CLI covers the operations surface (CLI-001).
- **Look at:** `plux help` lists the same groups.
- **Result:** ☐ pass ☐ fail — notes:

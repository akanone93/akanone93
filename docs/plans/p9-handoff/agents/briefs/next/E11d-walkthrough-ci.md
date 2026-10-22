<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E11d — make the walkthrough runnable in CI (`SEC-103`, `GOV-030`, `GOV-033`, X1)

`make walkthrough-kind` (`test/walkthrough/walkthrough-kind.sh`, header lines 18–35) needs two hooks and an enterprise server with a licence, none of which exist. Deliver, in this order, committing each:
1. **WebAuthn driver** `tools/cmd/webauthn-driver` (standard library only, like all of `tools/`): a software authenticator (ES256, `none` attestation, the minimal CBOR it needs) that (a) signs a person in through the walkthrough's OIDC test provider, enrols itself as the person's second factor and prints an access token — the `WALKTHROUGH_TOKEN_CMD` contract; (b) completes the step-up ceremony at `$WALKTHROUGH_STEPUP_URL` — the `WALKTHROUGH_STEPUP_CMD` contract. Read the server side first: `backend/internal/auth/webauthn.go`, `stepup.go`, `stepup_cli.go`, `backend/internal/api/stepup.go`. Keys live in memory or a 0600 file under the run's temp dir; never print or log them. Unit-test it against `httptest` fakes of those endpoints.
2. **Test licence key:** `backend/edition/licensefile/keys.go` trusts only `productionKeys()` (empty). Add one build-time variable (set with `-ldflags -X`, empty by default) holding an extra base64 public key; when set, `TrustedKeys` includes it and the server logs a distinct warning at start ("this build trusts a test licence key") and `plux license` shows it (secure defaults: an unsafe setting always warns). Test both.
3. **Enterprise image:** a `server-ee` target in `backend/Dockerfile` building `ee/backend/cmd/plux-server-ee` like the `server` target, with a `TEST_LICENSE_KEY` build arg feeding the variable.
4. **Chart:** a `license.existingSecret` value that mounts the licence file read-only and points the server's licence path at it (find the config key in `backend/internal/config`); schema, README row, `make helm-lint`.
5. **Script:** `walkthrough-kind.sh` generates a key pair per run in its temp dir (`ee/backend/license` `GenerateKey`/sign — use or add a tiny `go run` helper), builds the ee image with the public key, signs a licence with every feature, creates the secret, deletes the private key, and defaults the two hooks to the driver. Keep `--dry-run` printing the plan.
6. **CI:** a `walkthrough` job in `.github/workflows/ci.yml` and `ci/affected.json` like `airgap` (daily, manual, path-selected on `test/walkthrough/**`, `tools/cmd/webauthn-driver/**`, `tools/internal/walkthrough/**`, the chart and the Dockerfile; `skipOnPush`), moving `test/walkthrough/**` out of `noJob`; update `tools/internal/affected/repo_test.go`'s list of push-skipped jobs; add the job to `ci-ok`.
Done when: `cd tools && go vet ./... && go test -count=1 ./...`, `cd backend && go test -count=1 ./edition/... ./cmd/plux/`, `make helm-lint`, `ZIZMOR_FLAGS=--offline make workflows-lint` and `make walkthrough-plan` pass.

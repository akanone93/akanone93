<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E11e — apply the maintainer's walkthrough decisions (2026-10-22)

Local branch `wip/p9-walkthrough-ci` holds one commit (`9489ff3f`, the `walkthrough` CI job); merge it after rule 0. Then:
1. **Licence (decision A):** no test signing key in code. The licence comes from the GitHub Actions secret `PLUX_TEST_LICENSE` (signed offline with the production key, which the maintainer adds to `productionKeys()` later). `walkthrough-kind.sh` takes it from `WALKTHROUGH_LICENSE_FILE`, creates the Secret and installs with `license.existingSecret`; the CI job writes the secret to a 0600 temp file and skips the walkthrough with a clear notice (job still succeeds) when the secret is empty. The ee image: add a `server-ee` target in `backend/Dockerfile` building `ee/backend/cmd/plux-server-ee` like `server` (no extra build args).
2. **HTTPS (decision A):** the run terminates TLS with its own CA so `publicBaseURL`, `PLUX_SERVER` and DPoP `htu` are all `https` (follow how `e2e-kind.sh` or the compose stack issues dev certificates); never relax the server's https check.
3. **Token scopes (decision A):** grant roles at organisation scope and issue every person's token after the grants; update `docs/compliance/walkthrough.md` to match.
Done when: `cd tools && go vet ./... && go test -count=1 ./...`, `make helm-lint`, `ZIZMOR_FLAGS=--offline make workflows-lint`, `make walkthrough-plan` and `cd tools && go test -count=1 ./internal/affected/` pass.

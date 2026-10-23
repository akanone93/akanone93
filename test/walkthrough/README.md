# walkthrough

The scripted regulated-industry audit walkthrough, the exit criterion of phase 9 (spec §5.1; plan §5.12, X1 and X5). `make walkthrough-kind` installs the Helm chart on `kind` with two replicas of every role and drives it with the `plux` CLI and the synthetic device fleet (`plux-load`) through nine steps; each step writes evidence, and the run ends with a signed evidence bundle. [`docs/compliance/walkthrough.md`](../../docs/compliance/walkthrough.md) is the auditors' version of the same steps.

| Piece | Where |
|---|---|
| The cluster, the chart, the seed, the port-forward, the hooks | [`walkthrough-kind.sh`](walkthrough-kind.sh) |
| The step runner, the steps, the evidence store and bundle signing | [`tools/internal/walkthrough`](../../tools/internal/walkthrough) |
| The command (`run`, `verify`) | [`tools/cmd/walkthrough`](../../tools/cmd/walkthrough) |
| The project the steps publish | [`schema/testdata/documents/plux_bank`](../../schema/testdata/documents/plux_bank), copied for the run and never changed in place |

```bash
make walkthrough-plan                         # the plan of every step; starts nothing
test/walkthrough/walkthrough-kind.sh --dry-run  # the same
make walkthrough-kind                         # the run (Docker, kind, helm, kubectl)
go run ./tools/cmd/walkthrough verify -dir walkthrough-out   # an auditor's check of the bundle
```

## What the environment must supply

* An **enterprise server image and a signed licence** (custom roles, SCIM, federation, step-up are enterprise features, `GOV-033`). `make walkthrough-kind` builds the image from the `server-ee` target; `WALKTHROUGH_LICENSE_FILE` names the licence, which the script stores as a Secret and installs with `license.existingSecret`. No licence signing key is in the repository: CI takes the licence from the secret `PLUX_TEST_LICENSE` and skips the walkthrough, with a notice, when it is empty. The api terminates TLS with a CA made for the run, so every URL is `https`.
* **A Transit signer for the run.** The file signing backend never signs for a production environment (`SEC-056`), and step 2 promotes to one, so the script runs an in-memory OpenBao dev server (`openbao/openbao`, MPL-2.0, the Vault Transit API) in the namespace and installs with `signing.backend: vault`, a token made for the run, and an egress rule from the api, the worker and the migration Job to it. It is gone with the cluster.
* Optionally `WALKTHROUGH_TOKEN_CMD`, which prints an access token for `$WALKTHROUGH_ACTOR` after that person signed in through the test provider and answered their second factor, and `WALKTHROUGH_STEPUP_CMD`, which completes the WebAuthn step-up at `$WALKTHROUGH_STEPUP_URL`. They default to `tools/cmd/webauthn-driver`, a software authenticator built for the run (ES256, `none` attestation, keys in a mode-0700 directory of the run).
* Optionally `WALKTHROUGH_VIEWER_CMD`, which opens a reconstruction in the viewer host on an emulator (step 7b, CI only).
* Optionally `WALKTHROUGH_SKIP`, the steps to leave out of a quicker, partial run (for example `03-staged-rollout`); see *Partial runs* in [the walkthrough document](../../docs/compliance/walkthrough.md).

## Evidence

`walkthrough-out/evidence/<step>/NN-<command>.txt` holds every command (arguments, exit code, output) and every call (request and response, with secret-bearing fields replaced); `evidence/run/transcript.txt` is the transcript. `evidence-bundle.json` is a DSSE-style envelope over a statement that lists every file with its SHA-256 and every step with its outcome, signed with an Ed25519 key generated for the run; `evidence-bundle.pub` is its public half. The private key is never written. `walkthrough verify` checks the signature first, then re-hashes the files.

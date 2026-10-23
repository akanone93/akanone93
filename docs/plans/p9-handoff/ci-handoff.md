# P9 CI hand-off (2026-10-23)

This note hands the work of getting P9's CI green, and merging it, to a new session. It gives the state, the method we use, what was done and what is left. Read it after `AGENTS.md` and `docs/WORKLOG.md` (session protocol, `AGENTS.md` §2).

## 1. Where things are

| Item | State |
|---|---|
| Pull request | [nightCode42/plux3#42](https://github.com/nightCode42/plux3/pull/42), `feat/phase-9` → `main`, **draft** until CI is green |
| `feat/phase-9` | head `25ca2ad4` (the PR's branch) |
| `ci/p9-failing` | head `196cf732`: the same fixes. Earlier sessions iterated here with a walkthrough-only CI selection; manual partial runs (§3) replace that |
| `docs/p9-ci-handoff` | this note on top of `feat/phase-9`; merge or cherry-pick it into `feat/phase-9` |
| Last full PR run | [39892738554](https://github.com/nightCode42/plux3/actions/runs/39892738554) on `a69ad784`: **every job green except Audit walkthrough (kind)** (step 4, fixed since). The PR runs on `25c93586` and `25ca2ad4` were cancelled by hand |
| Last partial walkthrough | [39908253174](https://github.com/nightCode42/plux3/actions/runs/39908253174) on `25ca2ad4`, run with `walkthrough-skip=03-staged-rollout`. Steps 1, 2 and 4 passed and step 3 ran only its setup. **Step 5 failed** (§4.1). Steps 6–9 have never run in CI |
| Scheduled check-in | `trig_01Tp9hHvjZ84WytvUqN66od8` belongs to the old session; delete or ignore it |

## 2. How the maintainer wants this done

1. **Root-cause before fixing.** Find the cause in the logs, then state it in one sentence. Fix the cause, not the symptom. Never retry a failure away (`AGENTS.md` §5).
2. **Smallest reliable fix.** Validate it locally first: the package's tests against PostgreSQL, `golangci-lint run` on the touched packages, and `make gen` for generated code.
3. **Ask before deviating from a document.** Use the format of `AGENTS.md` §3: the conflict, option A, option B, a recommendation. The same applies to any security control. This session asked about each of these, and the maintainer chose every time (§5).
4. **No sub-agents** for CI work. Read the logs yourself.
5. **Low token use.** Keep answers short. Answer in one sentence when the maintainer asks for that.
6. **Manual CI runs, iterated on the failing part.** Do not wait an hour for a full run each time (§3). Run the full PR CI only at the end.
7. **Commits and pushes.** The maintainer asked this session to commit and push to `feat/phase-9` and `ci/p9-failing`. Commit messages follow Conventional Commits and carry the attribution lines.

## 3. How to run and read CI

**Manual runs** use `workflow_dispatch` on `ci.yml` with `ref: feat/phase-9`:

- `only: walkthrough` runs only the walkthrough job, beside the repository jobs. **CI OK** then fails by design, because the run is not a full one.
- `walkthrough-skip: 03-staged-rollout` leaves out step 3's 33-minute rollout. Step 3 runs only its setup instead: it publishes the baseline and the candidate releases. A partial run reaches step 4 in about 10 minutes.
- Steps 1 and 2 have no setup but take about one minute together. Do not skip them: every later step needs the users and releases they create.
- Skipping step 4 breaks step 7, which needs the device and time recorded in step 4.
- A partial run's statement lists the steps it left out, and `verify` prints `partial run: …`. A partial run is not audit evidence (`docs/compliance/walkthrough.md`, *Partial runs*).
- Locally: `WALKTHROUGH_SKIP=03-staged-rollout make walkthrough-kind`, which needs Docker and kind. This container has neither.
- A manual run does not wake the session: no PR event arrives for a `workflow_dispatch` run. Poll with `actions_list` → `list_workflow_runs` (`event: workflow_dispatch`), or schedule a check-in about 15 minutes after dispatch.
- PR runs do wake the session through `subscribe_pr_activity` on #42.

**Logs:**

- `get_job_logs` with `tail_lines: 2600` and `return_content: true` is too large to return, so it saves the log to a file. Extract the text with `json.load(f)["logs_content"]`.
- Find the failure with:
  ```
  grep -nE "Z    (FAILED|passed|setup)|== [0-9]|✗|partial"
  ```
- A job's log is not readable while the job is running (HTTP 404).
- The artifact blob host is blocked here (proxy 403), so evidence bundles cannot be downloaded. Read the transcript lines in the job log instead.
- `list_workflow_jobs` returns more than the tool can show and is saved to a file. Its JSON is `{"jobs": {"jobs": [...]}}`. Print the name, status and conclusion of each job whose conclusion is not `success` or `skipped`.

**The local container:**

- The pre-push hook runs `make go-test` over every module, which takes more than 10 minutes. Run `git push` in the background and send its output to a file. Do not filter the output: an earlier filter hid the failure.
- The Go build cache filled the disk once (about 11 GB). That also stopped PostgreSQL, and every integration test then failed with "connection refused".
  - To recover, check `df -h /`, run `go clean -cache` and `rm -rf /tmp/go-build*`, then `pg_ctlcluster 16 main start`.
  - Check the database with `pg_isready -h 127.0.0.1`.
  - `PLUX_TEST_DATABASE_URL` is already exported.
- There are no Swift or Xcode tools here. iOS UI-test changes are checked only by CI.

## 4. What is left to finish P9

### 4.1 Walkthrough step 5 — failing now (root-caused, not fixed)

Run 39908253174 failed with:

```
plux control mandatory --env staging --app … --release 4
This action needs a step-up. Open https://…/stepup?request=… and confirm with your security key.
plux control: failed_precondition: PLX-8033: release 4 is newer than the current release 3 of channel production; promote it first
```

The step-up was completed by the driver; the server then refused the change.

**Cause.** `stepMandatoryAndRollback` (`tools/internal/walkthrough/steps_release.go`) makes `newestRelease` mandatory. That is release 4, the rollout candidate from step 3.
- Step 3 only publishes the candidate. Its setup does not promote it.
- In a full run, the rollout is rolled back.
- Either way, the fleet's channel still holds the baseline, release 3.
- The server refuses to make mandatory a release newer than the channel's current one.
- A full run would fail the same way.

**Options** (choose; check with the maintainer if neither fits the document):
- **A.** Promote the newest release to the fleet's environment first, then make it mandatory and force the rollback to the baseline. It is a non-production environment, so no approval is needed. Example: `plux release promote --app … --env staging <newest>`, as step 2 uses it in `steps_access.go`.
- **B.** Make the channel's current release (the baseline) mandatory and drop the forced rollback. This changes what step 5 shows, so `docs/compliance/walkthrough.md` §5 would need an update.
- **Recommendation: A.** It keeps step 5 as documented.

**Tests.** Update the step tests in `steps_test.go`, then iterate with partial runs (§3).

### 4.2 Steps 6–9 — never run in CI

| Step | What to watch |
|---|---|
| 6, audit export and compliance report | The syslog sink on the kind gateway. `-audit-key-cmd` comes from the kind script |
| 7, what did the user see | Needs step 4's device and time. Step 7b is optional and needs `WALKTHROUGH_VIEWER_CMD`, which CI does not set |
| 8, rolling upgrade | `-upgrade-cmd` runs `walkthrough-kind.sh upgrade`; the fleet runs for `-upgrade-window`, 8 minutes |
| 9, data-subject request | Export and erasure |

Each step's CLI flags were checked against `backend/cmd/plux` in an earlier session, so failures are likelier at the server or in the cluster. The method:
1. Fix one failure.
2. Run a partial run that keeps skipping step 3.
3. Repeat until the walkthrough passes.

### 4.3 Then

1. Run the **full PR CI** on `feat/phase-9`. Pushing a commit triggers it; otherwise ask the maintainer to re-run. All jobs must be green, CI OK included.
   - If an Android shard fails while downloading the NDK or CMake, before any test runs, that is infrastructure: re-run that job once.
2. **Mark PR #42 ready for review** (`update_pull_request`, `draft: false`). Update its body (`docs/plans/p9-handoff/p9-pr-body.md`) with the fixes in §5 and the follow-ups in §4.4.
3. **Update `docs/WORKLOG.md`.** Add a hand-off note, keep ten notes and move older ones to the archive. Report in the format of `AGENTS.md` §9.

### 4.4 Follow-ups to record (not blocking CI)

**Manual device check.** Kill switch propagation within 60 s (`NFR-024`), on a real device. The maintainer does this check; see `docs/compliance/walkthrough.md`, *Manual checks*.

**Requester attribution.** The capability-set approval gate (`backend/internal/release/capabilities_gate.go`) does not set `RequesterUser`. It has only the actor, so a token-made request may let its own owner approve it. This is the same flaw class as §5 items 1 and 17. Fix it, or put it to the maintainer.

**Health-gate design note.** The rollout health gate ignores the first rollup bucket after a rollout's `StartAt`, because its query clamps `From` to `StartAt` and buckets are five minutes long.

**Older open items** (session-3 note in `docs/WORKLOG.md`):
- hand-off 2 §9 items 6–7: limit defaults, the ADR-0066 locale rule, the `ee/LICENSE` review;
- obsolete branches `wip/p9-*`, `wip/p9-handoff`, `claude/tender-mendel-b4bh9u`;
- `GOV-020` and `ANL-040` stay `WIP`;
- `DEP-004` and `DEP-010` can move to `DONE` once the kind jobs (HA, restore drill, air-gapped, walkthrough) are green: check their rows;
- `NFR-040` has no check.

## 5. Done in this CI effort (on `feat/phase-9`)

Each fix was root-caused from the CI log, and its cause is in the commit message.

| # | Commit | Fix |
|---|---|---|
| 1 | `4f705220` | **Approval four-eyes bypass.** The token ID was recorded as the requester. `Artifact.RequesterUser` now records the person (GOV-021). Maintainer-approved |
| 2 | `09f62c33` | A promotion's risk and changes are read against the release the channel holds, not the previous release (PLX-8033). Maintainer-approved |
| 3 | `05a2dddd` | The walkthrough signs with a Vault Transit signer: an OpenBao dev pod, because the file signer cannot sign production (SEC-056). Maintainer chose OpenBao; `dependencies.md` row added |
| 4 | `7962e4cd` | The walkthrough bootstraps the installation as an operator does: invitation, WebAuthn, organisation, app, token (`webauthn-driver bootstrap`). Maintainer chose this over relaxing the seed guard |
| 5 | `692054c3` | The CLI releases the editing locks an import took (PLX-8020) |
| 6 | `15f28811` | Merged `origin/main`. A merge conflict had blocked the PR's CI |
| 7 | `dad11937` | Dependency review allows three licences, checked by hand. Maintainer-approved |
| 8 | `82370e07` | The walkthrough trusts the loopback proxy, so each synthetic device has its own address (rate limit, PLX-8040) |
| 9 | `518a1e7d` | Rollout stages are 6 minutes, long enough for the health gate's 5-minute rollups |
| 10 | `6820d8f4` | **Root cause of the gate never tripping.** The load generator now attributes events to the release each device runs |
| 11 | `ef587fd4` | A closing rollout waits for its lock (`FOR UPDATE`), not `SKIP LOCKED`. The race reproduced 1 time in 40 before the fix, 0 in 80 after |
| 12 | `4ffc0437` | Synthetic devices renew their 5-minute access token |
| 13 | `0aa03467`, `81a5e769` | Test schemas are dropped one table at a time. The lock table filled ("out of shared memory") |
| 14 | `d200c18d`, `9f999c02` | iOS UI tests tap again when a tap is lost while Dart holds the main thread (bank PIN field; add-to-app "Open welcome") |
| 15 | `b6f8f4fe`, `a69ad784` | Step 4 names the killed page by document ID (PLX-1005), and freezes **production**, because a freeze holds back production changes only (GOV-025). Maintainer chose this; walkthrough doc updated |
| 16 | `25c93586` | **Propagation is a manual check, as the maintainer decided.** The kind stack has no push provider and devices poll every 5 minutes, so steps 4 and 5 record the count instead of failing. Also adds partial runs (`-skip`, `WALKTHROUGH_SKIP`, the `walkthrough-skip` dispatch input) |
| 17 | `25ca2ad4` | **Break-glass four-eyes bypass.** The invoker could review their own action through a token. `invoked_by_user` was added to the unreleased migration 0048, and the review refuses that person (GOV-024). Maintainer-approved; regression test `TestReview_TokenInvokerRefused_GOV_024` |

Earlier commits on the branch, such as the step-up of API calls, the identity-provider private ranges and the air-gapped fixes, come from the sessions before; see `git log origin/main..feat/phase-9`.

## 6. Reference

- Walkthrough code:
  - steps: `tools/internal/walkthrough/steps*.go`
  - runner and `-skip`: `runner.go`
  - fleet: `fleet.go`
  - CLI driver and step-up watcher: `command.go`
  - CLI: `tools/cmd/walkthrough/main.go`
- Kind script: `test/walkthrough/walkthrough-kind.sh`. It sets up the OpenBao signer, bootstrap, `trustedProxies` and `WALKTHROUGH_SKIP`.
- Walkthrough document: `docs/compliance/walkthrough.md` (steps, *Manual checks*, *Partial runs*). Setup: `test/walkthrough/README.md`.
- CI: the `.github/workflows/ci.yml` walkthrough job (timeout 90 min), `ci/affected.json` for job selection, and `mk/helm.mk` (`walkthrough-kind`).
- Load generator: `backend/internal/loadgen`. Pacing: control every 5 minutes, manifest every 10 minutes. `-speedup` scales the pacing; the walkthrough does not use it.

# The audit walkthrough

This is the auditors' version of the scripted walkthrough that closes phase 9 (spec §5.1, plan §5.12). It says what is done on a self-hosted, highly available installation, which evidence each step leaves, and what that evidence proves. The same steps are executable: `make walkthrough-kind` runs them against the Helm chart on `kind` (two replicas of every role, PostgreSQL in three instances with CloudNativePG, Valkey with Sentinel), `make walkthrough-plan` prints them, and [`test/walkthrough`](../../test/walkthrough/README.md) holds the code.

## Reading the evidence

A run writes a directory:

```
evidence/<step>/NN-<command>.txt   one file per command or call: arguments, exit code, output
evidence/run/transcript.txt        the transcript, with times
evidence-bundle.json               the signed statement
evidence-bundle.pub                the public key of the run
```

* **What a file shows.** A command file begins with the actor and the arguments (`bob$ plux approvals approve …`), then the exit code, standard output and standard error. A call file shows the request and the response of a ConnectRPC or SCIM call. Fields whose names contain *secret*, *token*, *password* or *credential* are replaced with `[redacted]`; credentials travel in the environment of the child process and never appear in an argument, a transcript or a file.
* **What the bundle proves.** `evidence-bundle.json` is a DSSE-style envelope. Its payload is a statement listing every step with its requirement IDs, its outcome and every evidence file with its SHA-256, plus the SHA-256 of the transcript. It is signed with an Ed25519 key generated for the run and discarded with the process. `go run ./tools/cmd/walkthrough verify -dir <directory>` checks the signature before it reads the statement, then re-hashes every file; any changed, missing or added-in-place file fails it. The bundle proves that these files are the files of this run and were not edited afterwards. It does not prove who ran it: the public key beside the bundle is only as trustworthy as the way it reached you. Pin it from the CI job's log or artifact, not from the directory under review (`verify -key`).
* **A failed run is evidence too.** The runner stops at the first failed step, marks the rest skipped and still signs the statement with `passed: false`; `verify` then exits non-zero.
* **People.** `admin` is the installation owner, bootstrapped as an operator does (`plux-server bootstrap`, then the invitation accepted and a security key enrolled). `alice` (team *payments*) authors releases. `bob` (*compliance*) and `carol` (*risk*) approve. `dave` (*contractors*) is a restricted contractor. All four are provisioned through SCIM in step 1; none exists before the run. Their e-mail addresses end in `walkthrough.example`.
* **The fleet.** Devices are synthetic clients of the real device protocol: each registers with development evidence (the attestation development provider is on for this non-production stack only), presents DPoP proofs, fetches manifests and control documents and sends telemetry. They identify nobody. They run in a non-production environment (default `staging`), because development evidence is refused in production; the approval, freeze and break-glass controls are exercised on the same installation and by the same code paths.

## Steps

### 1. Sign-in, roles, provisioning and a restriction (`01-sso-roles-scim`)

*Requirements:* `SEC-100`, `GOV-004`, `GOV-002`, `GOV-006`, `GOV-007`.

An OIDC identity provider is created that points at a test provider run for this walkthrough (discovery, key set and token endpoint over TLS with a CA made for the run, PKCE required); the installation trusts that CA and that one address for identity providers through `auth.federation`, which it otherwise refuses (`SEC-105`). A SCIM token is issued; four people and four teams are provisioned through SCIM. Alice signs in through the provider: the server's sign-in URL, an authorisation code, the server's completion. A custom role, `release-approver` (approve, read; nothing else), is created and roles are granted to the teams at organisation scope; the four people's tokens are issued only after the grants. A restriction removes `release.publish` and `release.promote` in production from the contractors team, whose role grants them.

*Evidence:* `…federation-createidentityprovider.txt`, `…scim-users.txt`, `…scim-groups.txt`, `…identity-startoidclogin.txt`, `…identity-completeoidclogin.txt`, `…roles-create-…`, `…grants-create-…`, `…restrictions-create-…`, `…access-explain-…`.

*Proves:* people can be provisioned and can sign in only through the organisation's identity provider; the sign-in ends in a session or in a second-factor challenge, never in a token without one (`SEC-100`); a custom role carries exactly the permissions it was given (`GOV-002`); a restriction is a ceiling that wins over a grant, and `plux access explain` names the restriction by its identifier as the reason (`GOV-006`, `GOV-007`).

### 2. A release touching a secure page (`02-secure-release-approval`)

*Requirements:* `GOV-020`, `GOV-021`, `GOV-022`, `GOV-023`, `SEC-103`.

The policy *secure-transfer-release* (production, touches a secure page: two approvers, distinct from the author, from distinct teams) is written. Alice publishes a release of the Plux Bank project, whose transfer page is `secure`, and tries to promote it to production: refused for want of approval, with an approval request opened. Alice cannot approve her own change. Bob approves; one approval is not enough. The content is then changed (the title of the secure page); Alice publishes again. The promotion is refused again and a new request is opened: Bob's approval does not carry to the new content. Bob and Carol, from distinct teams, approve the new request, each with a WebAuthn step-up, and the promotion succeeds.

*Evidence:* the refusals with their `PLX-8001` and `PLX-8003` codes, each approval, the second request, the final promotion.

*Proves:* an approval is bound to the content it approved (`GOV-022`); the author cannot approve (`GOV-021`); two approvers from distinct teams are required (`GOV-021`); the deciding actions need a step-up (`SEC-103`).

### 3. A staged rollout and its health gate (`03-staged-rollout`)

*Requirements:* `REL-050`, `REL-052`, `REL-053`.

A baseline release is promoted to the fleet's environment and the fleet starts. A candidate release is rolled out in stages 1% → 10% → 50% → 100% with a health gate on the Plux error rate whose action is to roll back. When the rollout has reached 10%, a second fleet starts whose devices report errors, but only the devices that run the candidate. The gate trips, the rollout is rolled back and names the release that reverted it.

*Evidence:* the rollout as created (stages and gate), the rollout's state at each poll (state and share), the reports of both fleets.

*Proves:* the rollout widens by deterministic buckets (`REL-050`) in the scheduled stages (`REL-052`); a regression confined to the candidate trips the gate and reverts the channel without a person (`REL-053`).

### 4. A kill switch through break-glass during a change freeze (`04-kill-switch-break-glass`)

*Requirements:* `REL-060`, `GOV-024`, `GOV-025`, `NFR-024`, `SYN-060`.

A change freeze opens on the fleet's environment. A kill switch on the transfer page without break-glass is refused. The same kill switch with `--break-glass` and a written justification goes through. With the fleet online, the control document advances and at least 95% of the devices apply it within the propagation bound (60 s by default). The emergency action has an open post-hoc review; whoever acted cannot close it; Bob closes it, with a step-up; the review is listed as reviewed. The freeze is removed.

*Evidence:* the refusal, the break-glass command, the control document before and after, the fleet report (`controlApplied`), the review refusal and closure.

*Proves:* a freeze blocks changes and only break-glass overrides it (`GOV-025`); break-glass needs a justification and leaves a review a second person closes (`GOV-024`); the kill switch reaches online devices within the bound (`REL-060`, `NFR-024`, `SYN-060`). The 95% criterion is a floor on the fleet's application count (a device that already had a document applies twice, once at start and once for the kill switch), not a latency percentile; the latency percentile is measured by the control-propagation benchmark (`X3`).

### 5. A mandatory update and a forced rollback (`05-mandatory-update-forced-rollback`)

*Requirements:* `SYN-060`, `REL-006`, `NFR-024`.

With the fleet running, the newest release is made mandatory and the fleet is forced back to the baseline. The fleet applies the control document within the bound. Both are then withdrawn.

*Evidence:* each control command, the control document, the fleet report.

*Proves:* the control document carries a mandatory update and a forced rollback to the devices (`SYN-060`), and a rollback is a release with an earlier release's content (`REL-006`). The synthetic devices do not retain releases, so what is shown is that the document reaches and is applied by the fleet, not that a real app restores a retained bundle; that is shown by the runtime's own tests.

### 6. Audit export and a compliance report (`06-audit-export-compliance-report`)

*Requirements:* `SEC-143`, `GOV-026`.

A syslog destination is created for a TLS sink run for this walkthrough, trusted by a CA made for the run. The sink receives RFC 5424 messages; every one is checked to start with a priority and version 1. A compliance report for the period of the run is produced and verified offline against the installation's audit public key (`plux compliance verify`), and must list approvals.

*Evidence:* the received syslog messages, the report, the verification.

*Proves:* the audit log leaves the installation by syslog over TLS in RFC 5424 form (`SEC-143`); a compliance report lists the period's production changes with authors and approvers and carries a signature that verifies without the server (`GOV-026`). The walkthrough checks that the report names approvals; it does not check every field of every change.

### 7. What did the user see (`07-what-did-the-user-see`, optional `07b-viewer-on-emulator`)

*Requirements:* `SEC-142`, `GOV-024`.

Bob reconstructs what one device of step 4 was shown at the moment of the kill switch, as a signed, read-only reconstruction release, and opens it. The optional step 7b, run only in CI on an emulator, opens the reconstruction in the viewer host and checks that the page is shown.

*Evidence:* the reconstruction, the list of reconstructions, the opened manifest (release, bundle hashes, signatures).

*Proves:* the server can say which release and which bundles a device had at a given time and produce them for review (`SEC-142`); opening it is an audited action. What the viewer renders is the emulator step's claim, and is absent when 7b is not enabled.

### 8. A rolling upgrade (`08-rolling-upgrade`)

*Requirements:* `DEP-004`, `DEP-003`.

The fleet runs with an error budget of zero. In the middle of the run, the chart is upgraded with a changed pod annotation, which rolls every role after the pre-upgrade hook has migrated the database.

*Evidence:* the fleet report of the upgrade window and the output of the upgrade.

*Proves:* a rolling upgrade of every role completes without one failed device request (`DEP-004`, `X5`). The fleet's report counts every request and every failed request, and the step fails if any request failed.

### 9. A data-subject request (`09-data-subject-request`)

*Requirements:* `SEC-162`.

The stores of personal data are listed. Everything held about one device of the fleet is exported, the device is erased, and the export is taken again. Both archives must be real JSON and must differ.

*Evidence:* the map of stores, both archives, the request status. The archives hold what the installation held about a synthetic device; they are personal data in a real installation and should be handled as such.

*Proves:* export and erasure work end to end on a live installation (`SEC-162`). Audit entries remain and name the subject by hash only; the audit chains and their checkpoints are checked by `plux-server audit verify`, which the restore drill also runs.

## What the walkthrough does not show

* It does not show performance: the 60 s bound is a floor on application counts here, and the percentile is a benchmark (`X3`); sizing is `X6`.
* The devices are synthetic. Real attestation, real hardware keys and real retained bundles are the subject of the device-farm suites.
* The second factor and the step-up are performed with a software WebAuthn authenticator (`tools/cmd/webauthn-driver`, the default of `WALKTHROUGH_TOKEN_CMD` and `WALKTHROUGH_STEPUP_CMD`), not a browser. The evidence shows that the server demanded and accepted them; it does not show a person with a physical key.
* The test provider and the syslog sink run on the host that runs the walkthrough, on the container network's gateway address, with a certificate authority made for the run. A real installation reaches its own provider and SIEM.

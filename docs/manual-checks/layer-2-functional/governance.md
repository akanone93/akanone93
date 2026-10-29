<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Enterprise and governance

You decide who may do what, make a production release need a second person, handle an emergency with
break-glass, freeze changes, promote through environments, roll out in stages under a health gate,
switch a plugin off, force an update and read the audit log. Guides: [governance](../../guides/governance.md),
[SSO and SCIM](../../guides/sso-and-scim.md), [rollouts and experiments](../../guides/rollouts-and-experiments.md).

Custom roles, multi-stage approvals, change freezes, SAML, SCIM and compliance reports are enterprise
features and need a licence file (`PLUX_LICENSE_FILE`; `plux admin license` shows the state). Without
one, expect a refusal that names the feature, and note it. Access control itself is never gated
(GOV-032). Approvals need a second person: use R-ADMIN and R-USER2 in two windows.

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-TEAM | Team `qa` with R-USER2 as a member | L2-GOV-003 |
| R-CUSTOMROLE | Custom role `shop-qa` | L2-GOV-005 |
| R-POLICY | Approval policy `shop-prod-publish`: one approver for production publish | L2-GOV-010 |
| R-FREEZE | Change freeze window `check-freeze` | L2-GOV-015 |
| R-ROLLOUT | A staged rollout of R-RELEASE2 | L2-GOV-019 |

### L2-GOV-001 — The ten predefined roles

- **Needs:** R-CLI
- **Do:** `plux roles list`; `plux roles show release-manager`.
- **Expect:** `owner`, `admin`, `release-manager`, `developer`, `designer`, `translator`, `reviewer`, `compliance-officer`, `auditor`, `viewer` with their permissions.
- **Why:** deny by default, roles are explicit (GOV-002, SEC-102).
- **Look at:** `docs/reference/permissions.md` lists risk and step-up per permission.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-002 — Grant a role at app scope

- **Needs:** R-USER2, R-APP
- **Do:** `plux grants create --role designer --user <R-USER2 id> --scope app --app <shop id>`; `plux grants list --user <id>`. In R-USER2's window try to publish.
- **Expect:** R-USER2 edits pages but **Publish** is refused or hidden; the grant is listed.
- **Why:** scoped grants (GOV-003).
- **Look at:** `plux access explain --permission release.publish --user <id> --app <shop id>` names the missing grant.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-003 — Teams: create R-TEAM

- **Needs:** R-CLI, R-USER2
- **Do:** `plux teams create --name QA qa`; `plux teams add --team <id> dev-two@plux.localhost`; `plux teams list`. In Studio open **Administration**, **Teams**.
- **Expect:** the team exists with R-USER2 as a member in both views.
- **Why:** teams carry grants and approvals (GOV-001, GOV-005).
- **Look at:** **Audit log** entries for the team and member.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-004 — Grant to a team, explain access

- **Needs:** R-TEAM
- **Do:** `plux grants create --role reviewer --team <qa id>`; open **Administration**, **Explain access** (`/admin/access`) for R-USER2 and permission `approvals.decide`.
- **Expect:** the answer names the team grant that gives it.
- **Why:** you can always see why someone has a permission (GOV-006, ADR-0086).
- **Look at:** `plux access explain --permission approvals.decide --user <id>` says the same.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-005 — Custom role: create R-CUSTOMROLE

- **Needs:** R-CLI, enterprise licence
- **Do:** `plux roles create --name "Shop QA" --permissions app.read,plugin.edit shop-qa`; `plux roles show shop-qa`; try a permission you do not hold.
- **Expect:** created and listed in **Roles** (`/admin/roles`); a permission you lack is refused. Without a licence the refusal names the feature.
- **Why:** a role never grants more than its creator holds (GOV-002, GOV-032).
- **Look at:** `plux admin license`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-006 — Restrictions beat grants

- **Needs:** R-USER2, step-up (L2-SEC-011)
- **Do:** `plux restrictions create --patterns release.* --subject-kind user --subject <R-USER2 id> --reason "check"`; try to publish as R-USER2 (give them `developer` first).
- **Expect:** refused despite the grant. `plux restrictions list` shows it; `delete <id>` removes it.
- **Why:** a ceiling over grants; owners keep the floor (GOV-006).
- **Look at:** removing the last Owner's abilities is refused (`PLX-8105`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-007 — Invitation and member management

- **Needs:** R-STUDIO
- **Do:** **Administration**, **Members**: open R-USER2, change their role, suspend and reinstate.
- **Expect:** each change is effective at the user's next request.
- **Why:** lifecycle of people (GOV-001).
- **Look at:** the audit trail.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-008 — SSO with a local OpenID Connect provider

- **Needs:** R-STACK
- **Do:**
  1. Start Keycloak: `COMPOSE_PROFILES=keycloak make compose-up` (it listens on <http://localhost:8180>).
  2. Open the Keycloak console at <http://localhost:8180> as `admin` (password
     `KC_BOOTSTRAP_ADMIN_PASSWORD` in `deploy/compose/.secrets/keycloak.env`); create a realm,
     a user, and an OpenID Connect client with the Studio redirect URL.
  3. In Studio **Administration**, **SSO** (`/admin/sso`) add the provider: the realm's issuer
     URL, and the client's id and secret.
  4. Sign out; the sign-in page offers the provider; sign in through it as the Keycloak user.
- **Expect:** you land in Studio as a linked user; groups map to teams if a mapping is set.
- **Why:** sign-in with your identity provider (GOV-004, SEC-100).
- **Look at:** the session shows the authentication context; back-channel logout ends the session.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-009 — SCIM and SAML where possible

- **Needs:** enterprise licence
- **Do:** open **SSO** and look for SCIM tokens and the SAML option; with a licence create a SCIM token and `curl` the SCIM users endpoint shown there.
- **Expect:** with a licence you can provision and deprovision a user by SCIM; without one the page says the feature is enterprise. SAML needs an external provider: note it as not run if you have none.
- **Why:** automatic joiners and leavers (GOV-004).
- **Look at:** a deprovisioned user's sessions end.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-010 — Approval policy: create R-POLICY

- **Needs:** R-CLI, step-up
- **Do:**
  1. `plux policies export > /tmp/policies.json` (see the shape of the defaults for `shop`).
  2. Write `/tmp/shop-prod-publish.json` in that shape: resource release, environment `production`, one approver, author not allowed.
  3. `plux policies put /tmp/shop-prod-publish.json`; open **Approvals**, **Policies**; use the simulator in the policy page with a sample change.
- **Expect:** the policy is versioned and listed; the simulator names who would have to approve.
- **Why:** sensitive changes need a second person (GOV-020, GOV-021).
- **Look at:** `plux policies show <key>`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-011 — Production publish needs approval

- **Needs:** R-POLICY, R-RELEASE2, a `production` environment
- **Do:** as R-ADMIN `plux release promote --env production <sequence>` (or publish to `production` in Studio).
- **Expect:** refused with `PLX-8001` and a request opens in the approvers' **Inbox**.
- **Why:** no approval, no production change (GOV-020).
- **Look at:** `plux approvals list`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-012 — The author cannot approve

- **Needs:** L2-GOV-011
- **Do:** as R-ADMIN `plux approvals approve <id>`.
- **Expect:** refused (`PLX-8003`).
- **Why:** four-eyes (GOV-021).
- **Look at:** step-up is also required to decide.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-013 — Approve as the second person

- **Needs:** L2-GOV-011, R-USER2 as `reviewer` with a passkey
- **Do:** as R-USER2 open **Approvals**, the request: read summary, risk, changes, reports, golden differences; **Approve** with a comment. Then as R-ADMIN repeat the promote.
- **Expect:** the promote goes through; the approval is spent by it.
- **Why:** an approval binds one exact artifact (GOV-022).
- **Look at:** **History** shows the decision; the audit log signs it.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-014 — A changed artifact voids the approval

- **Needs:** L2-GOV-011
- **Do:** request approval for a release, get it approved, then publish a new release and try to promote that one.
- **Expect:** `PLX-8002` or a new request: earlier approvals do not count for new content.
- **Why:** approval is about a content hash (GOV-022).
- **Look at:** the hash shown in the request.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-015 — Change freeze: create R-FREEZE

- **Needs:** R-CLI, enterprise licence
- **Do:** `plux freeze put --name check-freeze --tz Europe/Berlin --start <now, YYYY-MM-DDTHH:MM> --duration 2h --env-key production`; `plux freeze list`; try a production change.
- **Expect:** the change is refused with `PLX-8010`; `plux freeze delete <id>` lifts it.
- **Why:** calendar freezes protect busy periods (GOV-025).
- **Look at:** a break-glass action still goes through (next check).
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-016 — Break-glass

- **Needs:** L2-GOV-015, a role with `breakglass.invoke`
- **Do:** `plux control kill --env production --break-glass --justification "check" plugin catalog`; try again without `--justification`.
- **Expect:** with a justification it proceeds and alerts approvers; without, `PLX-8005`.
- **Why:** emergencies do not wait, but leave evidence (GOV-024).
- **Look at:** **Approvals**, **Break-glass reviews** lists the use with a review due date.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-017 — Post-hoc review of break-glass

- **Needs:** L2-GOV-016, another user with step-up
- **Do:** `plux breakglass list --status open`; as another person `plux breakglass review --comment ok <id>`; try as the person who acted.
- **Expect:** the other person reviews it; the actor cannot.
- **Why:** emergency use is reviewed within 24 hours (GOV-024).
- **Look at:** an overdue review escalates (`PLX-8006`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-018 — Environments and promotion

- **Needs:** R-RELEASE2
- **Do:** in **Releases** switch environments; promote R-RELEASE2 from `development` to `staging` (`plux release promote --env staging --channel staging <sequence>`).
- **Expect:** the same content, same hash, new sequence in the target; each environment has its own channels and keys.
- **Why:** build once, promote (REL-002, ADR-0020).
- **Look at:** Bundle SHA-256 equal in both environments.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-019 — Staged rollout with a health gate: create R-ROLLOUT

- **Needs:** R-RELEASE2, R-DEVICE-SHOP
- **Do:** `plux rollout create --app <shop id> --env staging --release <n> --stages 10%:1h,50%:1d,100% --gate-crash-free 200 --on-regression pause`; `plux rollout show <id>`.
- **Expect:** a rollout in its first stage; devices outside the share keep the channel's release; the gate shows its newest check.
- **Why:** staged delivery limits the damage (REL-050, REL-053).
- **Look at:** `plux_rollout_health_gate_trips_total`; one rollout per channel at a time (`PLX-3153`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-020 — Pause, resume, abort

- **Needs:** R-ROLLOUT
- **Do:** `plux rollout pause --reason test <id>`; `resume <id>`; `abort --reason test <id>`; use the same controls in Studio on the release (**Pause**, **Resume**, **Advance to next stage**, **Abort**).
- **Expect:** each state change is audited; abort returns the channel to the previous release as a new release.
- **Why:** operators control a rollout (REL-051, REL-006).
- **Look at:** [stuck rollout runbook](../../runbooks/stuck-rollout.md).
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-021 — Targeting rules

- **Needs:** R-CLI
- **Do:** create a rollout with `--allow <your device id>` and `--deny <another>`; check which device sees the release.
- **Expect:** allowed devices always get it, denied never.
- **Why:** targeted delivery (REL-052).
- **Look at:** the device's installed release in Studio **Devices**.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-022 — Kill switch for plugin, page, function

- **Needs:** R-DEVICE-SHOP
- **Do:** run `plux control kill -h` for the scope names, then switch off the plugin `catalog` and a page; `plux control get --env development`; `plux control unkill …`.
- **Expect:** the control document lists the switches; devices show fallbacks after their next sync; unkill restores.
- **Why:** stop a faulty part without a release (REL-030).
- **Look at:** control propagation target is 60 s at p95 (`make bench-control`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-023 — Mandatory update and forced rollback

- **Needs:** R-DEVICE-SHOP
- **Do:** `plux control mandatory --env development --release <n> --message update.required`; open the app; then `--clear`. Try `plux control rollback --env development --release <older>`.
- **Expect:** devices activate the release before showing another page; rollback forces devices to an older retained release; clearing ends it.
- **Why:** emergency control of what devices run (REL-031, REL-032).
- **Look at:** the control document version rises.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-024 — Feature flag targeted by segment, experiment promote

- **Needs:** R-APP
- **Do:** `plux flags set --env development --type boolean --default false --segment beta=true new-checkout`; `plux flags get --env development new-checkout`; start the experiment of L2-STU-044 and `plux experiment results --env development <key>`.
- **Expect:** the flag lists per-segment values; results say `PLX-8201` until enough subjects exist.
- **Why:** gradual exposure.
- **Look at:** `plux experiment promote` ships a winner through the normal release path.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-025 — Audit log: search and export

- **Needs:** R-STUDIO
- **Do:** **Administration**, **Audit log** (`/admin/audit`): filter by date, actor, action; **Export CSV**.
- **Expect:** entries for every check of this file; the CSV opens with the same rows.
- **Why:** evidence for auditors (SEC-140, GOV-026).
- **Look at:** the actor, resource and justification fields.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-026 — Compliance report

- **Needs:** enterprise licence, R-CLI
- **Do:** `plux compliance report --from <yesterday> --to <tomorrow> --format json --out /tmp/c.json`; `plux compliance verify --key <id> /tmp/c.json` as the help text says.
- **Expect:** every production change with author, approvers, times and hashes, signed; verification passes offline.
- **Why:** a regulated audit walkthrough (GOV-026).
- **Look at:** `make walkthrough-plan` prints the scripted audit steps; `make walkthrough-kind` runs them.
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-027 — Reconstruct what a device ran

- **Needs:** R-DEVICE-SHOP
- **Do:** `plux audit reconstruct --app <shop id> --device <id> --at <time>`; `plux audit reconstructions --app <shop id>`.
- **Expect:** a signed, read-only reconstruction release of what that device had at that time.
- **Why:** "what did this user see" for disputes.
- **Look at:** it is openable only in the viewer app (`apps/plux_viewer`).
- **Result:** ☐ pass ☐ fail — notes:

### L2-GOV-028 — Licence state

- **Needs:** R-CLI
- **Do:** `plux admin license`; `plux license inspect <file>` for a sample licence; Studio **Administration**, **Licence**.
- **Expect:** state absent, valid, grace, expired or invalid with what it enables; core features work in every state.
- **Why:** licences verify offline and never gate security (GOV-030, GOV-032).
- **Look at:** [licence expiry runbook](../../runbooks/licence-expiry.md).
- **Result:** ☐ pass ☐ fail — notes:

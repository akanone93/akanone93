# 0076. The approval engine: policies as documents, content-hash binding, signed approval records, break-glass, change freezes

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `GOV-020`, `GOV-021`, `GOV-022`, `GOV-023`, `GOV-024`, `GOV-025`, `GOV-026`, `TST-005`, `LIM-006`, `ABT-007`, `SEC-080`, `FN-007`, `FN-055`, `A11Y-008`, `TST-003`

## Context and problem

Earlier phases left stand-ins where a human second opinion is required: `release.promote`,
`function.deploy.production`, `a11y.waive`, `goldens.approve` and the device-function step-up
(plans p7 D6 and p8 D10). A regulated organisation needs one engine that decides who must agree
before a production change, proves afterwards who agreed to exactly what, and still lets an
operator stop an incident without waiting. The question: how are approval rules expressed, what
does an approval bind to, and how are emergencies and freezes handled?

## Decision drivers

- An approval must be about one exact artifact; a later change must not inherit it.
- Rules must be data an administrator edits and audits, not code.
- Evidence must be verifiable offline by an auditor.
- An emergency stop must never be blocked, but must always be reviewed.
- Defaults per security profile (maintainer decision Q16); multi-stage rules, freezes and signed
  reports are enterprise (plan §2.6), the rest is core (`GOV-032`).

## Considered options

1. Policy documents matched on resource, environment, action and risk; approvals bound to a content hash; signed decision records.
2. Hard-coded approval rules per feature (extend the stand-ins).
3. An external workflow engine (BPMN or a ticketing system) called through webhooks.

## Decision

Chosen option: **1**.

- **Policies (`GOV-020`).** `policy` documents, versioned and audited like any document, per
  organisation. `match`: resource type (release, function version, policy, security
  configuration, limit, capability set, experiment, waiver, golden baseline), environment,
  action, risk attributes such as `touchesSecurePage`, `touchesMoneyFlow`,
  `changesFunctionUsedBy`. `require`: approver count, roles, named approvers, distinct from
  author, distinct teams, stages sequential or parallel, expiry. The compiler and the release
  service compute risk attributes from the diff. An invalid document gives `PLX-1700`.
- **Binding (`GOV-022`).** A request names the artifact by content hash (release ID and manifest
  hash, function version hash, policy document hash). Any change makes a new hash and earlier
  approvals stop counting (`PLX-8002`). Other refusals: `8001` approval required, `8003` approver
  not eligible, `8004` expired.
- **Review bundle (`GOV-023`).** JSON diff, validation and accessibility reports, golden-image
  diffs from P8's server-held baselines, the affected install base, the changelog and a computed
  risk summary, through the API and `plux approvals show`.
- **Decisions.** Approve or reject needs WebAuthn step-up (ADR-0075). Each decision is a signed
  record, written with the server's audit signing key (ADR-0019 style) and appended to the audit log.
- **Break-glass (`GOV-024`, core).** Designated roles run kill switch, rollback or function
  disable without approval, with a written justification (`PLX-8005`). Approvers are alerted at
  once (ADR-0081); a post-hoc review is opened, due in a configurable window (default 24 h),
  escalating when overdue (`PLX-8006`).
- **Change freezes (`GOV-025`, enterprise).** Calendar windows, one-off and recurring with time
  zones, per organisation and environment. Production changes are refused with `PLX-8010` except
  through break-glass.
- **Compliance reports (`GOV-026`, enterprise).** For a period: every production change with
  author, approvers, timestamps, content hashes and evidence, as a signed JSON DSSE envelope and
  a PDF from the in-house writer; `plux compliance verify` checks them.
- **Wiring.** Production promotion, function production deploy, device-function enablement,
  security-configuration changes, limit raises (`LIM-006`), capability sets (`SEC-080`),
  accessibility waivers, golden baselines, experiments on secure or money flows (`ABT-007`) and
  required tests (`TST-005`) all call `approval.Require(ctx, artifact)`. Default policies per
  profile are seeded and editable: `maximum` requires four-eyes on production publish, function
  deploy, device-function enablement, security-configuration change, accessibility waiver and
  golden approval; `standard` one approver for production; `development` none.

## Consequences

- **Positive:** one mechanism replaces five stand-ins; approvals cannot be carried over to
  changed content; an auditor can verify evidence without trusting the server's database.
- **Negative:** every guarded path gains a dependency on the engine and its coverage floor (85%);
  policy mistakes can deadlock a release (hence break-glass and a runbook); the matcher is a
  small language to document.
- **Follow-up:** migrations 0028 to 0031; Studio surfaces and "editable in Studio" in P11
  (`GOV-020` stays `WIP` until then); runbooks for deadlock and break-glass review.

## Options in detail

### Option 1 — policy documents and hash-bound approvals

Fits the document model already audited and exportable. Costs a matcher and a record format.

### Option 2 — hard-coded rules

Fast for the first rule, but each new rule is a code change and a release, and the evidence is
inconsistent between features. It is what the stand-ins already are. Rejected.

### Option 3 — an external workflow engine

Moves the authority for production changes outside the audited server, needs network egress and
breaks air-gapped installs. Rejected.

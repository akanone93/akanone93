# Approval Deadlock and Break-Glass Runbook

A guarded change waits for approvals that cannot arrive, or an emergency needs an action that
no approval will allow in time (`GOV-020`–`GOV-024`, [ADR-0076](../adr/0076-approval-engine.md)).
Use it for a release, a function deployment, an experiment start or a money-flow change that
is stuck in `PLX-8001`, and for an emergency kill switch, forced rollback or rollback during a
change freeze.

## Symptom

| What you see | Code | Meaning |
|---|---|---|
| The change is refused and names an open request | `PLX-8001` (`APPROVAL_REQUIRED`) | Nothing approved is on file; the request is open and waits |
| The change is refused after its content changed | `PLX-8002` | The approvals were for another content hash; they no longer count |
| An approver is refused | `PLX-8003` | The requester or the author of the change may not approve it |
| The request is refused as expired | `PLX-8004` | The policy's expiry passed; ask again to open a new request |
| Break-glass is refused | `PLX-8005` | The justification is empty; it is required |
| A break-glass review is overdue | `PLX-8006` (warning) | Nobody completed the post-hoc review in time |

## Impact

A guarded change does not reach its target until the request is approved, rejected or expires.
Break-glass is the one way through a freeze without approval. It is audited, put in the ledger of
production changes, and alerted to every subscriber at once, so it is never silent, but it is
still an exception that must be reviewed.

## Diagnosis

1. Find the request. `ApprovalService.ListApprovals` lists the organisation's requests; `GetApproval`
   returns one with its subject, its content hash and its decisions.
2. Read the policy that matched. `ApprovalService`'s policy RPCs (`ListPolicies`, `GetPolicy`)
   show the number of approvers, the roles and named approvers allowed, distinct-team rules and
   the expiry.
3. Check eligibility. An approval counts only from a person the policy's roles or named approvers
   include, who is not the requester and not the author of the change (`PLX-8003`). A `maximum`
   policy needs approvers from distinct teams. Common deadlock: the only eligible approver is the
   author.
4. Check the hash. A `PLX-8002` means the content changed after approval. Compare the request's
   hash with the content you intend to ship; the approval is for the old one.
5. For break-glass, `BreakGlassService.ListBreakGlass` lists emergency actions newest first. An
   overdue review shows as overdue there. Reviews are due `governance.breakGlassReviewHours` after
   the action (24 hours by default, at most 720).

## Remediation

**Ordinary deadlock.**

- Do not change the content to dodge the hash, and do not split the change to dodge a policy. Both
  are refused by design and both are visible in the audit log.
- Make an eligible approver available: grant a role or add a team member with `plux grants create`
  (needs `members.manage`) or `plux teams add`. Then the approver decides the open request with
  `DecideApproval`; the decision needs a fresh step-up (`SEC-103`).
- If the request expired (`PLX-8004`), the requester asks again, which opens a new request.
- A policy change is itself a guarded change. Do not weaken a policy to unblock one change; the
  change to the policy is reviewed like any other.

**Break-glass (emergency only).**

- Use it only to stop or undo harm: a kill switch (including a function's), a forced rollback or a
  rollback. Set `break_glass` and give a written justification on the request (`ControlService`'s
  `SetKillSwitch`, `ClearKillSwitch`, `SetForcedRollback` and the like); the justification is required
  (`PLX-8005`), and the request is refused without `break_glass`. The caller needs `breakglass.invoke` on
  the environment and a fresh step-up.
- Break-glass skips the freeze check and the approval check, and nothing else. Restrictions still
  apply (`GOV-006`).
- Afterwards, someone who holds `approvals.decide` and did not take the action completes the review
  with `ReviewBreakGlass`. The overdue state is a warning, not a block: an emergency stop is never held
  back by an overdue review.
- If a review is overdue, complete it, or record in the incident why it cannot be completed yet. An
  escalation has already been audited and sent as a critical notification (`PLX-8006`).

## Escalation

- Ordinary deadlock: the organisation's Owner (`PLX-8103` keeps at least one Owner on record).
- Break-glass without a review, or an invocation nobody recognises: the security contact of the
  organisation, and the Owner. Treat an unexplained break-glass as a possible misuse of the role.
- Reference: the [threat model](../security/threat-model.md#p9--enterprise-governance-federation-rollouts-and-analytics)
  for approvals and break-glass abuse.

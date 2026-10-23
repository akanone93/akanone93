# Stuck Rollout Runbook

A staged rollout does not advance, does not end, or leaves a channel in a state nobody
expects (`REL-050`–`REL-053`, [ADR-0078](../adr/0078-rollouts-and-health-gates.md)). Use it when
a rollout has sat in one stage past its schedule, when a rollout is `closing` for longer than a
scheduler pass, or when a channel does not point where its last rollout said it would.

## Symptom

- `RolloutService.ListRollouts` shows a rollout in `scheduled`, `active` or `paused` whose next
  stage is overdue, or in `closing` after several scheduler runs.
- A new rollout for the same channel is refused with `PLX-3153` (`SCHEDULE_OVERLAPS_ROLLOUT`):
  at most one rollout of a channel is in progress at any moment.
- A rollout is `paused` with `PLX-3150` (`ROLLOUT_PAUSED_BY_HEALTH_GATE`), or was rolled back
  with `PLX-3151` (`ROLLOUT_ROLLED_BACK`). Those are decisions, not faults: see the
  [health-gate runbook](health-gate-false-positives.md) before you resume anything.

## Impact

Devices inside a rollout are served the candidate's manifest; devices outside it are served the
channel's. A rollout that stops at 10% keeps 10% on the candidate and 90% on the previous release.
No device receives a release older than its anti-rollback floor (`REL-006`), so a stuck rollout
delays a fix; it does not downgrade devices.

## Diagnosis

1. Read the rollout with `RolloutService.GetRollout`: its state, its stage, the share it has
   reached, its channel, the candidate and the previous release, and its next scheduled step.
2. Check that the scheduler runs. The rollout job runs on the elected leader of the scheduler
   (`internal/scheduler`), once for each organisation. A stopped leader or a queue that does not
   drain (see [alert runbooks](alerts/PluxJobQueueBacklog.md)) makes every rollout of every
   organisation look stuck at once.
3. Read the audit log for the rollout. Every state transition is audited, and a health decision
   names its figures.
4. For `closing`: a two-step ending is finished by the next pass. If the rollout is still
   `closing` after two or three passes with a healthy scheduler, the scheduler is the cause, not the
   rollout.
5. For a channel that points at the wrong release: compare the channel's release with the rollout's
   candidate and previous release. Do not infer the outcome from the Studio view alone.

## Remediation

- **Scheduler stopped or behind.** Restore the scheduler first: the leader must be elected and the
  jobs queue must drain. Nothing else is needed; the next pass does each due step once, because every
  step is a conditional update.
- **Paused by a person.** `RolloutService.ResumeRollout` continues the current stage afresh.
  Resume only when the reason for the pause is gone. A rollout paused by the health gate is resumed
  only after the decision is reviewed.
- **Stop it.** `RolloutService.AbortRollout` ends the rollout and points the channel back at the
  release it had before, as a new release that repeats that one (`REL-006`). Devices inside the
  rollout move back at their next manifest request. Use this when the candidate is the problem.
- **Do not** edit the channel's release directly, delete the rollout row or start a second rollout on
  the same channel to get around `PLX-3153`. Each of those leaves the audit log and the signed
  manifests disagreeing with the channel.
- Record the cause and the action in the change record. A rollout that was stopped by an operator
  is audited as such.

## Escalation

- Scheduler or queue faults: the platform on-call, with the server [runbook](server.md).
- A channel pointing at a release nobody intended, or a candidate that devices cannot install:
  the release owner, and the security contact if a manifest was signed by a key nobody recognises
  (see [key compromise](key-compromise.md)).

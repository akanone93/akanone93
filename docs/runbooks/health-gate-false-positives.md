# Health-Gate False Positives Runbook

A health gate paused a rollout (`PLX-3150`) or rolled it back (`PLX-3151`), and the regression it
names does not come from the release (`REL-053`, [ADR-0078](../adr/0078-rollouts-and-health-gates.md)).
Use it to decide, with evidence, whether the gate was right before anyone resumes a rollout or
loosens a threshold.

## Symptom

- A notification from the rollout says the candidate regressed on a named figure: crash-free
  sessions, Plux errors, action errors, function errors or a custom event per session.
- A rollout was rolled back automatically, and the release you expected to be healthy is the one
  that was reverted.
- Devices on the candidate report no crash or error that matches the figure the gate names.

## Impact

A pause holds the share the rollout had reached and sends no further devices to the candidate. A
rollback moves the channel back to the previous release (as a new release, `REL-006`). A false positive therefore costs
time and attention; it does not put a faulty release on devices, and the gate never relaxes a
device's anti-rollback floor.

## Diagnosis

1. Read the decision in the audit log. Health decisions name the figures the gate compared: the
   candidate's value and the previous release's value for the metric, the window and the sample
   size. Write them down before you act.
2. Check the sample. The gate decides only when both releases have at least the minimum sample of
   sessions in the window, so a decision that exists had enough sessions. Still read the sample sizes
   in the figures: a rate over a few hundred sessions moves far more than one over millions.
3. Check the metric's meaning:
   - crash-free sessions and Plux error rate are about the release itself;
   - action errors and function errors count failed actions and function calls, so an outage of a
     dependency the release calls (the gateway's upstream, a server function) can move them without a
     release fault. Check the [gateway runbook](gateway-upstream-failure.md) and the
     [function outage runbook](function-outage.md) for the same window;
   - a custom event counts what the app sent, so a change in the app's own event naming can move it.
4. Compare the same figures with an earlier window of the previous release. A ratio that was already
   high before the rollout is a baseline, not a regression.
5. Check whether the affected sessions are concentrated in one locale, device class or platform. A
   concentration points at the environment, not at the release as a whole.

## Remediation

- **The gate was right.** Keep the pause or the rollback. Fix the release, publish a new candidate and
  start a new rollout. Do not resume the old rollout.
- **The gate was wrong, and the cause is a dependency.** Fix the dependency first. Then resume the
  paused rollout with `RolloutService.ResumeRollout` (it continues the current stage afresh). A
  rolled-back rollout has ended: start a new rollout for the candidate once the cause is settled.
- **The gate was wrong, and the thresholds are too tight for the app.** Thresholds are set when the
  rollout is created and are part of its audit trail. Changing them is a change of its own, reviewed
  like any other. A threshold of zero switches that check off (`REL-053`); use zero only with the reason
  recorded in the change, never to get a rollout past a check it failed.
- **Never** disable the gate for a production channel, edit the stored figures or lower a sample size
  to make a decision you want.
- Record the figures, the cause and the decision in the change record. A repeated false positive on
  the same metric is a finding to raise with the release owner; it is not fixed by hand each time.

## Escalation

- Disagreement about whether the gate was right: the release owner and the app's product owner, with
  the audit entries attached.
- A dependency outage behind an action or function error: the owner of that dependency (see the
  linked runbooks).
- A rollback that moved devices to a release nobody expected: the [stuck rollout runbook](stuck-rollout.md)
  and the platform on-call.

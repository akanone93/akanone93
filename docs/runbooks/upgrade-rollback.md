# Upgrade Rollback Runbook

Return a Plux installation to the previous release after an upgrade has gone wrong (`DEP-030`,
[ADR-0083](../adr/0083-kubernetes-ha-backups-and-air-gap.md)). The rollback works because the
migrations are expand-only: the previous version keeps running against the schema the new version
has left behind. Use it when a new release fails its health checks, its helm test or its traffic,
and when a migration did not finish.

## Symptom

- The Helm upgrade reports a failed pre-upgrade hook, a failed rollout of the new replicas, or a
  failed `helm test plux --namespace plux`.
- `/readyz` fails on the new replicas, or `plux_rpc_requests_total` shows a rise in server errors
  after the upgrade.
- A migration failed. The server reports it and the replicas do not start.

## Impact

While the new replicas run, device requests and Studio requests go to them. A migration that did not
complete leaves nothing applied (the migration is one unit), and the previous version works against the
unchanged schema. An upgrade that completed and then misbehaved has applied expand-only changes, which
the previous version tolerates for one release by design (`DEP-030`).

## Diagnosis

1. Read the release history: `helm history plux --namespace plux`. Note the revision you want to return
   to: the last one that passed its health checks.
2. Check the new replicas: `kubectl -n plux get pods`, their logs, and `/readyz` on each.
3. Check whether a migration failed: the pre-upgrade hook's log and the server's log at start. A failed
   migration is not a schema change you have to undo by hand.
4. Check the release notes for the new version. They name any step beyond the expand-only rule. A step
   that the release notes call irreversible changes this runbook: stop and escalate before a rollback.

## Remediation

1. **Roll the replicas back.** Helm returns the chart and its values to the chosen revision:
   `helm rollback plux <revision> --namespace plux`. The replicas of the previous version start against
   the schema they already know.
2. **Verify.** `helm test plux --namespace plux` must pass. Check `/readyz` on every replica, and that
   device manifest requests and Studio sign-in succeed.
3. **Keep the new bundle.** Do not delete the newer chart or images until the investigation is done; the
   next attempt uses them.
4. **Do not undo migrations by hand.** The expand-only rule means the previous version does not need them
   undone, and a manual change to the schema is what makes a rollback fail. If the database itself is
   damaged, restore it from backup instead (see the [backup and restore runbook](backup-and-restore.md)),
   and do so only with the platform on-call present.
5. **Check the releases.** Rolling back the server does not change releases or channels. If a release
   itself is the problem, use the [stuck rollout runbook](stuck-rollout.md) or `plux release rollback`.
6. Record the revision, the time, the cause and the verification in the change record.

## Escalation

- A rollback that does not restore `/readyz`, or a helm test that still fails: the platform on-call.
- A migration that the release notes call irreversible, or a database that will not start: the platform
  on-call and the database owner, before any restore.
- A release that must not reach devices at all: the release owner (see the
  [server runbook](server.md#upgrades)).

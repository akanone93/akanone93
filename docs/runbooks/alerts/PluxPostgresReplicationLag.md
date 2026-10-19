# PluxPostgresReplicationLag

**Severity:** ticket. **Expression:**

```
max by (pod)(cnpg_pg_replication_lag) > 30
```

## Symptom

A standby lags the primary.

## Impact

A failover would lose that data unless synchronous replication is on; reads from the replica are stale.

## Diagnosis

- PostgreSQL dashboard: lag, WAL generation, storage latency.

## Remediation

- Relieve the storage or network; a standby that cannot catch up is rebuilt by deleting its pod and PVC.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [backup-and-restore](../backup-and-restore.md).

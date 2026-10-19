# PluxPostgresDown

**Severity:** page. **Expression:**

```
cnpg_collector_up == 0
```

## Symptom

The CloudNativePG exporter cannot reach PostgreSQL.

## Impact

If it is the primary, every role fails readiness until a standby is promoted.

## Diagnosis

- `kubectl get cluster.postgresql.cnpg.io` and `kubectl describe cluster plux-pg` show the phase and the primary.
- Pod logs of the instance.

## Remediation

- CloudNativePG fails over by itself; if it does not, check quorum and storage. If the data is lost, restore from backup.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [server](../server.md), [backup-and-restore](../backup-and-restore.md).

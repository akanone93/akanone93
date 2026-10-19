# PluxBackupTooOld

**Severity:** ticket. **Expression:**

```
time() - max(cnpg_collector_last_available_backup_timestamp) > 86400
```

## Symptom

No base backup completed in a day (the default schedule is every 6 hours).

## Impact

A point-in-time restore needs a longer WAL replay; the retention window may shrink.

## Diagnosis

- `kubectl get backups.postgresql.cnpg.io` and the failed backup's `status.error`.
- Can the instances reach the backup store and does the credential still work?

## Remediation

- Fix the store or credential, then `kubectl create` a Backup to confirm.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [backup-and-restore](../backup-and-restore.md).

# PluxWalArchiveStalled

**Severity:** page. **Expression:**

```
time() - max(cnpg_pg_stat_archiver_last_archived_time) > 300
```

## Symptom

The WAL archive stalled. The marker job writes every minute and `archiveTimeout` forces a segment out, so a quiet database does not explain it.

## Impact

The recovery point objective of 5 minutes (`DEP-010`) is not met: a lost cluster loses the unarchived data. WAL piles up on the volume.

## Diagnosis

- `cnpg_pg_stat_archiver_failed_count` and the instance log (`barman-cloud-wal-archive`).
- The backup store: reachable, credential valid, bucket exists, space left.

## Remediation

- Restore access to the store; the archiver catches up by itself. Watch the volume until WAL drains.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [backup-and-restore](../backup-and-restore.md).

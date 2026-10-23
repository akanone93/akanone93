# PluxVolumeFilling

**Severity:** page. **Expression:**

```
kubelet_volume_stats_available_bytes{persistentvolumeclaim=~".*(plux-pg|seaweedfs|keys).*"} / kubelet_volume_stats_capacity_bytes{persistentvolumeclaim=~".*(plux-pg|seaweedfs|keys).*"} < 0.1
```

## Symptom

A data volume is nearly full.

## Impact

PostgreSQL stops writing when its volume is full; a stalled WAL archive makes this happen. SeaweedFS refuses uploads.

## Diagnosis

- PostgreSQL: WAL directory growth (a stalled archive); SeaweedFS: bucket sizes.

## Remediation

- Expand the PVC (the storage class must allow it); clear the cause of WAL build-up first.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [backup-and-restore](../backup-and-restore.md).

# PluxObjectStorageDown

**Severity:** page. **Expression:**

```
kube_statefulset_status_replicas_ready{statefulset=~".*seaweedfs"} < 1
```

## Symptom

The SeaweedFS pod is not ready. (An external S3 store needs its provider's own alerts; `/readyz` of the api role covers it.)

## Impact

Devices cannot download bundles; publishing fails; backups to this store fail.

## Diagnosis

- `kubectl describe pod` of the SeaweedFS pod; its volume claim.

## Remediation

- Fix scheduling or the volume; for production move to an external S3 store.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [server](../server.md), [backup-and-restore](../backup-and-restore.md).

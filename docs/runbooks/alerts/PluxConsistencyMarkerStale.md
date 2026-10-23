# PluxConsistencyMarkerStale

**Severity:** ticket. **Expression:**

```
time() - max(kube_cronjob_status_last_successful_time{cronjob=~".*backup-marker"}) > 600
```

## Symptom

The marker CronJob does not succeed.

## Impact

There is no exact restore point that matches the object store; the restore drill cannot choose one.

## Diagnosis

- `kubectl get jobs` and the logs of the latest `backup-marker` job (init container `emit`, then `upload`).

## Remediation

- Fix the cause the log shows: database credential, object store credential, a network policy.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [backup-and-restore](../backup-and-restore.md).

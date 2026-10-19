# PluxJobQueueBacklog

**Severity:** ticket. **Expression:**

```
max by (queue)(plux_jobs_queue_depth) > 100
```

## Symptom

Jobs wait in a queue faster than the worker role completes them.

## Impact

Publishes, deltas and sweeps are late; devices keep the previous release meanwhile.

## Diagnosis

- `plux_jobs_queue_depth` by `queue`; worker CPU and memory on the overview; worker logs for failing jobs.

## Remediation

- Scale the worker role (`worker.autoscaling`); fix the job that fails repeatedly.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [server](../server.md).

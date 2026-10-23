# PluxApiLatencyHigh

**Severity:** ticket. **Expression:**

```
histogram_quantile(0.99, sum by (le)(rate(plux_rpc_duration_seconds_bucket[5m]))) > 2.5
```

## Symptom

The 99th percentile call duration exceeds 2.5 s.

## Impact

Slow Studio and CLI; devices may time out and retry, which adds load.

## Diagnosis

- Break down by method: `histogram_quantile(0.99, sum by (le, method)(rate(plux_rpc_duration_seconds_bucket[5m])))`.
- PostgreSQL: connections, locks and replication lag on the PostgreSQL dashboard; CPU saturation on the overview.

## Remediation

- Scale the api role (`api.autoscaling`), or raise `postgresql.maxConnections` with the database resources.
- A slow method after a release: roll back.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [server](../server.md).

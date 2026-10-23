# PluxApiErrorRatioHigh

**Severity:** ticket. **Expression:**

```
sum(rate(plux_rpc_requests_total{code=~"internal|unavailable|unknown|data_loss|deadline_exceeded"}[5m])) / sum(rate(plux_rpc_requests_total[5m])) > 0.05
```

## Symptom

Calls end in `internal`, `unavailable`, `unknown`, `data_loss` or `deadline_exceeded`.

## Impact

Studio, the CLI and devices see errors; the SLO alerts page if devices are affected.

## Diagnosis

- `sum by (service, method, code)(rate(plux_rpc_requests_total{code=~"internal|unavailable|unknown|deadline_exceeded"}[5m]))` names the method.
- Logs carry the request id of a failing call; `/readyz` shows dependencies.

## Remediation

- A dependency is the cause (database, cache, object storage): see the matching alert and restore it.
- One method only: a recent release; roll back.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [server](../server.md).

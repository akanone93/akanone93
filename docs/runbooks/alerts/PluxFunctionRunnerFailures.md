# PluxFunctionRunnerFailures

**Severity:** ticket. **Expression:**

```
1 - sum(rate(plux_function_invocations_total{result=~"ok|business_error"}[5m])) / sum(rate(plux_function_invocations_total[5m])) > 0.05
```

## Symptom

Invocations end in a trap, a timeout, memory exhaustion or a refusal.

## Impact

Functions answer errors; the function SLO burns.

## Diagnosis

- `sum by (result)(rate(plux_function_invocations_total[5m]))`; memory peaks and durations on the overview.

## Remediation

- One function: fix or roll it back. All: scale `fnrunner` or check the module cache.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [function-outage](../function-outage.md).

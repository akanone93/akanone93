# PluxFunctionBurnTicket

**Severity:** ticket. **Expression:**

```
plux:function_error_ratio:rate1d > (3 * 0.001)
and
plux:function_error_ratio:rate2h > (3 * 0.001)
```

## Symptom

Function success (99.9%) is spending its error budget 3 times faster than it accrues, over 1d and 2h. A bad event is a function request that ends in anything but `ok` or `business_error`.

## Impact

At this rate 10% of the 30-day budget in a day is spent. Devices or users see failures now; if it continues, the objective for the 30-day window is missed.

## Diagnosis

- Dashboard "Plux SLO burn": the function panels show the ratio per window.
- `sum by (result)(rate(plux_function_requests_total[5m]))` shows the failing results; compare with `plux_function_invocations_total` at the runner.

## Remediation

- Follow the function outage runbook: `fnrunner`, `egress` and the module cache.

Budget arithmetic: the objective leaves an error ratio of 0.1%. This alert fires when both the 1d and the 2h ratios exceed 3 times that.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [function-outage](../function-outage.md).

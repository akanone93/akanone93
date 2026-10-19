# PluxFunctionBurnSlow

**Severity:** page. **Expression:**

```
plux:function_error_ratio:rate6h > (6 * 0.001)
and
plux:function_error_ratio:rate30m > (6 * 0.001)
```

## Symptom

Function success (99.9%) is spending its error budget 6 times faster than it accrues, over 6h and 30m. A bad event is a function request that ends in anything but `ok` or `business_error`.

## Impact

At this rate 5% of the 30-day budget in six hours is spent. Devices or users see failures now; if it continues, the objective for the 30-day window is missed.

## Diagnosis

- Dashboard "Plux SLO burn": the function panels show the ratio per window.
- `sum by (result)(rate(plux_function_requests_total[5m]))` shows the failing results; compare with `plux_function_invocations_total` at the runner.

## Remediation

- Follow the function outage runbook: `fnrunner`, `egress` and the module cache.

Budget arithmetic: the objective leaves an error ratio of 0.1%. This alert fires when both the 6h and the 30m ratios exceed 6 times that.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [function-outage](../function-outage.md).

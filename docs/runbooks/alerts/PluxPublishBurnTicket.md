# PluxPublishBurnTicket

**Severity:** ticket. **Expression:**

```
plux:publish_error_ratio:rate1d > (3 * 0.005)
and
plux:publish_error_ratio:rate2h > (3 * 0.005)
```

## Symptom

Publish job success (99.5%) is spending its error budget 3 times faster than it accrues, over 1d and 2h. A bad event is a publish job that ends `failed`.

## Impact

At this rate 10% of the 30-day budget in a day is spent. Devices or users see failures now; if it continues, the objective for the 30-day window is missed.

## Diagnosis

- Dashboard "Plux SLO burn": the publish panels show the ratio per window.
- `plux_publish_jobs_total` by `result`; the job rows with `state = failed` carry the diagnostics; worker logs.

## Remediation

- Follow the publish blocked runbook for refusals; for infrastructure failures restore the worker role and its dependencies.

Budget arithmetic: the objective leaves an error ratio of 0.5%. This alert fires when both the 1d and the 2h ratios exceed 3 times that.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [server](../server.md), [publish-blocked-quality](../publish-blocked-quality.md).

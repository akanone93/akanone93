# PluxPublishBurnFast

**Severity:** page. **Expression:**

```
plux:publish_error_ratio:rate1h > (14.4 * 0.005)
and
plux:publish_error_ratio:rate5m > (14.4 * 0.005)
```

## Symptom

Publish job success (99.5%) is spending its error budget 14.4 times faster than it accrues, over 1h and 5m. A bad event is a publish job that ends `failed`.

## Impact

At this rate 2% of the 30-day budget in one hour is spent. Devices or users see failures now; if it continues, the objective for the 30-day window is missed.

## Diagnosis

- Dashboard "Plux SLO burn": the publish panels show the ratio per window.
- `plux_publish_jobs_total` by `result`; the job rows with `state = failed` carry the diagnostics; worker logs.

## Remediation

- Follow the publish blocked runbook for refusals; for infrastructure failures restore the worker role and its dependencies.

Budget arithmetic: the objective leaves an error ratio of 0.5%. This alert fires when both the 1h and the 5m ratios exceed 14.4 times that.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [server](../server.md), [publish-blocked-quality](../publish-blocked-quality.md).

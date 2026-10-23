# PluxManifestBurnSlow

**Severity:** page. **Expression:**

```
plux:manifest_error_ratio:rate6h > (6 * 0.0005)
and
plux:manifest_error_ratio:rate30m > (6 * 0.0005)
```

## Symptom

Manifest availability (99.95%) is spending its error budget 6 times faster than it accrues, over 6h and 30m. A bad event is a manifest request that ends in `error`.

## Impact

At this rate 5% of the 30-day budget in six hours is spent. Devices or users see failures now; if it continues, the objective for the 30-day window is missed.

## Diagnosis

- Dashboard "Plux SLO burn": the manifest panels show the ratio per window.
- `sum by (environment, result)(rate(plux_manifest_requests_total[5m]))` shows which environment fails. Then the PostgreSQL, object storage and signing checks of the server runbook (`/readyz`).

## Remediation

- Restore the failing dependency (database, object storage, signing backend); roll back a bad release of the server.
- A single environment: check its signing key and the metadata freshness.

Budget arithmetic: the objective leaves an error ratio of 0.05%. This alert fires when both the 6h and the 30m ratios exceed 6 times that.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [server](../server.md).

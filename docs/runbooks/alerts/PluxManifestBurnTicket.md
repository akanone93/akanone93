# PluxManifestBurnTicket

**Severity:** ticket. **Expression:**

```
plux:manifest_error_ratio:rate1d > (3 * 0.0005)
and
plux:manifest_error_ratio:rate2h > (3 * 0.0005)
```

## Symptom

Manifest availability (99.95%) is spending its error budget 3 times faster than it accrues, over 1d and 2h. A bad event is a manifest request that ends in `error`.

## Impact

At this rate 10% of the 30-day budget in a day is spent. Devices or users see failures now; if it continues, the objective for the 30-day window is missed.

## Diagnosis

- Dashboard "Plux SLO burn": the manifest panels show the ratio per window.
- `sum by (environment, result)(rate(plux_manifest_requests_total[5m]))` shows which environment fails. Then the PostgreSQL, object storage and signing checks of the server runbook (`/readyz`).

## Remediation

- Restore the failing dependency (database, object storage, signing backend); roll back a bad release of the server.
- A single environment: check its signing key and the metadata freshness.

Budget arithmetic: the objective leaves an error ratio of 0.05%. This alert fires when both the 1d and the 2h ratios exceed 3 times that.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [server](../server.md).

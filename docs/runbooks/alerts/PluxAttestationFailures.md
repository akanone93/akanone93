# PluxAttestationFailures

**Severity:** ticket. **Expression:**

```
sum(rate(plux_attestation_results_total{verdict="failed"}[15m])) / sum(rate(plux_attestation_results_total[15m])) > 0.3
and
sum(rate(plux_attestation_results_total[15m])) > 0.01
```

## Symptom

Evidence fails verification for a large share of devices.

## Impact

Registrations fail with `PLX-6001`. A fraud wave looks the same.

## Diagnosis

- Per platform: `sum by (platform)(rate(plux_attestation_results_total{verdict="failed"}[15m]))`.
- Android only and since a release: a changed signing certificate or Play Integrity key; Google rotated a root.

## Remediation

- Correct `androidCertDigests` or the Play Integrity keys; ship a build with a rotated root.
- If the pattern is not a configuration error, treat it as an attack and see the device revocation runbook.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [attestation-outage](../attestation-outage.md), [device-revocation](../device-revocation.md).

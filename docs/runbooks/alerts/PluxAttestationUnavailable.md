# PluxAttestationUnavailable

**Severity:** page. **Expression:**

```
sum(rate(plux_attestation_results_total{verdict="unavailable"}[10m])) / sum(rate(plux_attestation_results_total[10m])) > 0.2
and
sum(rate(plux_attestation_results_total[10m])) > 0.01
```

## Symptom

Attestation verdicts are `unavailable`: the status list is stale or the app has no attestation configuration.

## Impact

New registrations and re-attestations fail with `PLX-6009`; under `maximum` there is no grace.

## Diagnosis

- `sum by (platform, verdict)(rate(plux_attestation_results_total[10m]))`.
- Can the worker reach Google's status list? Is `attestation.apps.<appID>` present in the configuration?

## Remediation

- Follow the attestation outage runbook: restore the status list fetch or name a file in `attestation.android.statusListFile`.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [attestation-outage](../attestation-outage.md).

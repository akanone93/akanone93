# PluxKeyCompromiseSignals

**Severity:** page. **Expression:**

```
sum(rate(plux_dpop_rejections_total{reason=~"replay|binding_mismatch"}[10m])) > 1
```

## Symptom

Proofs are replayed or presented with a key other than the one the token is bound to.

## Impact

A stolen token or key is being tried, or a client bug repeats requests.

## Diagnosis

- By reason: `sum by (reason)(rate(plux_dpop_rejections_total[10m]))`.
- Audit log: recent token and device events for the affected organisation.

## Remediation

- Revoke the affected devices; if the signals point at a server key, follow the key compromise runbook.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [key-compromise](../key-compromise.md), [device-revocation](../device-revocation.md).

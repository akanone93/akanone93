# PluxCertificateExpiring

**Severity:** ticket. **Expression:**

```
(certmanager_certificate_expiration_timestamp_seconds - time()) < 14 * 24 * 3600
```

## Symptom

A certificate managed by cert-manager is close to expiry. (Certificates the chart generates for internal TLS last ten years and rotate by deleting their Secrets.)

## Impact

After expiry clients refuse the connection.

## Diagnosis

- `kubectl describe certificate` and the issuer's events.

## Remediation

- Fix the issuer so renewal succeeds; for the pinned server key follow the pin rotation runbook.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [pin-rotation](../pin-rotation.md).

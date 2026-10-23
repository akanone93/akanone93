# PluxLicenseExpiringSoon

**Severity:** ticket. **Expression:**

```
plux_license_days_remaining < 30
```

## Symptom

`plux_license_days_remaining` is below 30.

## Impact

After expiry enterprise writes are refused; reads and security controls continue (`GOV-032`).

## Diagnosis

- The remaining days are the metric's value.

## Remediation

- Obtain a new licence file and replace the Secret named in `PLUX_LICENSE_FILE`; restart the roles.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [server](../server.md).

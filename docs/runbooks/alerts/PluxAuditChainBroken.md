# PluxAuditChainBroken

**Severity:** page. **Expression:**

```
plux_audit_chain_verified == 0
```

## Symptom

`plux-server audit verify` or the periodic verification found an entry or checkpoint that does not hold.

## Impact

The audit log may have been altered, or a key changed: evidence integrity is in doubt (`SEC-140`).

## Diagnosis

- Run `plux-server audit verify --json` and note the first broken organisation, sequence and entry.
- Was the audit signing key replaced or restored from another environment?

## Remediation

- Do not repair or delete entries. Preserve the database, then follow the key compromise runbook if tampering is possible.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [key-compromise](../key-compromise.md).

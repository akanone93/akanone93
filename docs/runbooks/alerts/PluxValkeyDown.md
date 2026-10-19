# PluxValkeyDown

**Severity:** ticket. **Expression:**

```
redis_up == 0
```

## Symptom

The Valkey exporter cannot reach an instance.

## Impact

The DPoP replay cache, rate limits and challenges degrade; see the replay-cache alerts.

## Diagnosis

- `kubectl get pods -l app.kubernetes.io/component=valkey`; Sentinel `get-master-addr-by-name`.

## Remediation

- Sentinel promotes a replica; if it does not, restore quorum (two of three).

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [replay-cache-outage](../replay-cache-outage.md).

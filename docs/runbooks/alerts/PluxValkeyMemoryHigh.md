# PluxValkeyMemoryHigh

**Severity:** ticket. **Expression:**

```
redis_memory_max_bytes > 0 and redis_memory_used_bytes / redis_memory_max_bytes > 0.9
```

## Symptom

Memory use approaches `maxmemory`.

## Impact

Keys are evicted or writes fail: replay entries and rate limits are lost.

## Diagnosis

- Key counts by prefix; whether a client leaks keys without a TTL.

## Remediation

- Raise the memory limit; fix the client that writes keys without an expiry.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [replay-cache-outage](../replay-cache-outage.md).

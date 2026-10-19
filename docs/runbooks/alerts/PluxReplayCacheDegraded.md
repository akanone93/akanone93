# PluxReplayCacheDegraded

**Severity:** page. **Expression:**

```
plux_dpop_replay_cache_degraded == 1
```

## Symptom

A replica uses its in-memory replay store.

## Impact

Replay detection weakens to one replica (`SEC-023`); under fail-closed settings requests are refused with `PLX-6015`.

## Diagnosis

- `rate(plux_dpop_replay_cache_fallback_total[5m])` is the size of the exposure.
- Valkey and Sentinel health: the Valkey dashboard, `redis_up`.

## Remediation

- Restore Valkey or the Sentinel quorum; replicas leave the fallback by themselves.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [replay-cache-outage](../replay-cache-outage.md).

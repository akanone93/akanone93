# PluxReplayCacheRefusing

**Severity:** page. **Expression:**

```
sum(rate(plux_dpop_rejections_total{reason="replay_cache_unavailable"}[5m])) > 0
```

## Symptom

DPoP rejections with reason `replay_cache_unavailable`.

## Impact

Devices get `PLX-6015` and cannot sync until the cache is back.

## Diagnosis

- Valkey dashboard; `kubectl get pods -l app.kubernetes.io/component=valkey`.

## Remediation

- Restore Valkey. Do not disable the check: it is a security control.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [replay-cache-outage](../replay-cache-outage.md).

# PluxServerDown

**Severity:** page. **Expression:**

```
up{job=~"plux.*"} == 0
```

## Symptom

Prometheus cannot scrape a Plux role.

## Impact

Fewer replicas serve devices and Studio; with every replica of the api role down no device can sync.

## Diagnosis

- `up{job=~"plux.*"}` by `job` and `instance` shows which role.
- Compose: `docker compose ps` and `docker compose logs plux-server`. Kubernetes: `kubectl get pods`, `kubectl describe pod` and `kubectl logs --previous`.
- `/readyz` names the dependency that fails (PostgreSQL, cache, object storage, KMS).

## Remediation

- A dependency failed: restore it first; the roles recover when `/readyz` passes.
- A crash loop after a deploy: roll back (`helm rollback plux`), then read the logs.
- A network policy or TLS change: check the scrape configuration reaches the metrics port.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [server](../server.md).

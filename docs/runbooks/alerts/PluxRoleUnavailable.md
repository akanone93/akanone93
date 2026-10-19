# PluxRoleUnavailable

**Severity:** page. **Expression:**

```
kube_deployment_status_replicas_available{deployment=~".*plux-(api|worker|fnrunner|egress|builder)"} == 0
```

## Symptom

A role has no ready pod.

## Impact

api: devices cannot sync and Studio is down. worker: publishing, delta generation and sweeps stop. fnrunner or egress: functions fail. builder: function builds fail.

## Diagnosis

- `kubectl get pods -l app.kubernetes.io/component=<role>` and `kubectl describe` the pods (scheduling, image pull, probes).
- Readiness fails when a dependency is down: `kubectl logs` shows the failing check.

## Remediation

- Fix the cause the events show: capacity, image pull secret, a missing Secret.
- After an upgrade: `helm rollback plux <revision>`.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [server](../server.md), [function-outage](../function-outage.md).

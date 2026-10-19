# PluxControlPropagationSlow

**Severity:** page. **Expression:**

```
plux:control_propagation_seconds:p95_30m > 60
and
plux:control_propagation_seconds:p95_5m > 60
```

## Symptom

The 95th percentile of `plux_control_propagation_seconds` (control document published to applied on a device) exceeds the 60 s objective.

## Impact

Kill switches, rollouts and flag changes reach devices late (`NFR-024`). Security-relevant switches are delayed.

## Diagnosis

- Worker queue depth and the control document builder; device sync interval and push availability.
- `bench-control` in CI measures the same path against a known load.

## Remediation

- Scale the worker role; fix the dependency that slows the document build.

The metric is defined in Appendix G.1 and is exported once control events are recorded (ADR-0077); until then this alert cannot fire.

## Escalation

Page: if not mitigated within 30 minutes, escalate to the engineering on-call; for key, audit or replay signals also inform the security contact (SECURITY.md). Related: [server](../server.md), [runtime](../runtime.md).

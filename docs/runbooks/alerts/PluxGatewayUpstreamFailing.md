# PluxGatewayUpstreamFailing

**Severity:** ticket. **Expression:**

```
sum(rate(plux_rpc_requests_total{service="plux.gateway",code=~"5.."}[5m])) / sum(rate(plux_rpc_requests_total{service="plux.gateway"}[5m])) > 0.1
```

## Symptom

Customer upstreams answer with 5xx or not at all.

## Impact

Pages that use the gateway show errors; the fault is usually in the customer API.

## Diagnosis

- By upstream: `sum by (method)(rate(plux_rpc_requests_total{service="plux.gateway",code=~"5.."}[5m]))`.

## Remediation

- Tell the customer; check the allowlist and timeouts as the gateway runbook describes.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [gateway-upstream-failure](../gateway-upstream-failure.md).

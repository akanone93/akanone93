# PluxFunctionEgressRefusals

**Severity:** ticket. **Expression:**

```
sum(rate(plux_egress_tunnels_total{result=~"refused_.*"}[10m])) > 0.1
```

## Symptom

The egress role refuses tunnels: domain not allowed, private address, port, IP literal.

## Impact

A function that needs an external API fails; or a function probes the network.

## Diagnosis

- `sum by (result)(rate(plux_egress_tunnels_total{result=~"refused_.*"}[10m]))`.

## Remediation

- A legitimate domain: add it to the function's declared domains and republish. Otherwise leave the refusals in place and review the function.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [function-egress-refusals](../function-egress-refusals.md).

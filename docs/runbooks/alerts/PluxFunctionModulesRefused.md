# PluxFunctionModulesRefused

**Severity:** ticket. **Expression:**

```
sum(rate(plux_function_module_cache_total{result="refused"}[10m])) > 0
```

## Symptom

Modules fail verification (signature or hash) when loaded.

## Impact

Those functions cannot run. A refusal can mean a tampered module.

## Diagnosis

- Runner logs name the module and the check that failed.

## Remediation

- Rebuild and redeploy the function; if the key set changed, check `functions.artifactKeys`.

## Escalation

Ticket: handle in working hours; escalate to the on-call if it turns into a page alert. Related: [function-outage](../function-outage.md), [function-builder-failure](../function-builder-failure.md).

# Alert runbooks

One page per alert of `deploy/observability/rules/plux.rules.yml` (`OBS-004`, ADR-0085): symptom,
impact, diagnosis, remediation, escalation. An alert's `runbook_url` points at the page named
after it; `make helm-lint` fails when an alert has no page, a page lacks a section, or a page
belongs to no alert.

## Two delivery paths

Infrastructure and service-level rules go to Prometheus and Alertmanager (route `severity: page`
to the pager and `severity: ticket` to the queue). Alerts that users configure on their own app's
analytics (`ANL-030`) are product alerts delivered through the channels of ADR-0081 and never pass
through Alertmanager. The Compose stack evaluates the rules and shows them in Prometheus but ships
no Alertmanager; the chart's `PrometheusRule` is picked up by the Prometheus Operator.

## Service level objectives

| Objective | Target | Bad event | Alerts |
|---|---|---|---|
| Manifest availability | 99.95% | `plux_manifest_requests_total{result="error"}` | `PluxManifestBurnFast`, `PluxManifestBurnSlow`, `PluxManifestBurnTicket` |
| Function success | 99.9% | `plux_function_requests_total` with a result other than `ok` or `business_error` | `PluxFunctionBurnFast`, `PluxFunctionBurnSlow`, `PluxFunctionBurnTicket` |
| Publish job success | 99.5% | `plux_publish_jobs_total{result="failed"}` | `PluxPublishBurnFast`, `PluxPublishBurnSlow`, `PluxPublishBurnTicket` |
| Control propagation | p95 at most 60 s | `plux_control_propagation_seconds` | `PluxControlPropagationSlow`, `PluxControlPropagationDegraded` |

Each ratio objective is a recording rule over seven windows (`plux:<slo>_error_ratio:rate5m` to
`rate3d`). A burn rate is the error ratio divided by the budget (`1 - objective`); each alert pairs a
long and a short window so it fires quickly on a real outage and resets as soon as it ends:
**fast** 14.4 times over 1 h and 5 m (page; 2% of a 30-day budget per hour), **slow** 6 times over
6 h and 30 m (page; 5% per six hours), **ticket** 3 times over 1 d and 2 h (10% per day).

`plux_publish_jobs_total` and `plux_control_propagation_seconds` are specified in Appendix G.1 but
not yet recorded by the server: their alerts cannot fire until it records them.

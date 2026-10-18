# 0085. Service level objectives, dashboards and alert rules

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `OBS-004`, `OBS-005`, `NFR-024`, `ANL-030`, `DEP-004`

## Context and problem

The Compose stack ships Prometheus, Grafana and an OpenTelemetry collector, and P6 runbooks refer
to some alerts. P9 makes operations a commitment: stated objectives, alerts that page for the
right reasons, and a runbook for each alert. Without objectives, alerts either fire on every
blip or miss slow degradation. The questions: which objectives, how they are computed and
alerted, and how dashboards and rules are delivered and verified.

## Decision drivers

- Alerts reflect user-visible impact first, resource usage second.
- Each alert has a runbook, so it is actionable (DoD-9).
- Delivered as files in `deploy/` that work with the chart and with Compose.
- Objectives are testable, not aspirational.

## Considered options

1. Recording rules for the objectives, multi-window burn-rate alerts, dashboards as Grafana JSON in `deploy/`, a runbook per alert.
2. Threshold alerts on resource metrics only (CPU, memory, error counts).
3. A hosted observability product.

## Decision

Chosen option: **1**.

- **Objectives.** Manifest availability 99.95%, function success 99.9%, publish job success
  99.5%, and control-document propagation of at most 60 s at p95 (`NFR-024`, measured by
  `bench-control` and by `control_applied` events, ADR-0077).
- **Rules (`OBS-004`).** Each objective is a Prometheus recording rule with burn-rate alerts over
  paired windows. Infrastructure alerts (database replication lag, backup age, queue depth,
  certificate expiry, licence expiry, disk) are added next to them. The exact windows and
  thresholds are decided in the milestone.
- **Runbooks.** Every alert carries a link to `docs/runbooks/alerts/<name>.md`; the alerts the P6
  runbooks already reference are included.
- **Dashboards (`OBS-005`).** Grafana JSON in `deploy/`: system health per role, SLO status and
  budget, sync health, jobs and queues, database and cache, and the analytics dashboards of
  ADR-0080. Provisioned by the chart and by Compose.
- **Delivery.** Product alerts configured by users (`ANL-030`) use ADR-0081 channels; these
  infrastructure rules go through Alertmanager. The two paths are documented so operators know
  which system fires what.
- **Verification.** How rules and dashboards are tested in CI is decided in the milestone.

## Consequences

- **Positive:** pages track user impact; every alert is actionable; the same rules ship to Compose
  and Kubernetes.
- **Negative:** rules and runbooks are a maintenance burden that must change with metrics;
  objectives we state become promises operators will measure us against.
- **Follow-up:** `docs/runbooks/alerts/`; the `bench-control` job in CI (plan §7).

## Options in detail

### Option 1 — SLOs with burn-rate alerts

Fewer, better alerts, and the budget is visible. Needs careful rule tests.

### Option 2 — resource thresholds only

Easy, but noisy and blind to failures that leave resources normal. Rejected as the main approach;
resource alerts remain as secondary rules.

### Option 3 — hosted product

Egress of operational data and no air-gapped option (`DEP-040`). Rejected.

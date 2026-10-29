<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Operations and deployment

You run the Compose stack as an operator: read logs, health and metrics in Prometheus and Grafana,
look at the installation pages in Studio, take and restore a database backup, load-test, restart
and upgrade, and know where the Kubernetes, air-gapped and infrastructure-as-code paths are.
Guides: [operations](../../guides/operations.md), [on-premises](../../operations/on-premises.md),
[Compose README](../../../deploy/compose/README.md).

The Kubernetes checks need Docker, `kind` and `kubectl`, and take a long time; each is one command.

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-DUMP | `/tmp/plux.dump`, a PostgreSQL dump of the stack | L2-OPS-010 |
| R-LOAD | A JSON load-test report `/tmp/load.json` | L2-OPS-014 |

### L2-OPS-001 — The stack is healthy

- **Needs:** R-STACK
- **Do:** `docker compose -f deploy/compose/compose.yaml ps`
- **Expect:** `plux-server`, `postgres`, `seaweedfs`, `valkey`, `otel-collector`, `prometheus`, `grafana`, `studio`, `plux-fnrunner`, `plux-egress` are up and healthy; the builder and render worker only with their profiles.
- **Why:** one command brings up the single-node installation (DEP-001, DEP-002).
- **Look at:** `docker compose … ps --format json` for health states.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-002 — Server logs are structured and clean

- **Needs:** R-STACK
- **Do:** `docker compose -f deploy/compose/compose.yaml logs --tail 50 plux-server`; perform a Studio edit; read the new lines.
- **Expect:** one JSON object per line with level, time, request id and procedure; no tokens, passwords or document bodies.
- **Why:** logs are useful and never leak secrets (OBS-001, SEC-092).
- **Look at:** `grep -i -E 'plux_pat_|password|Bearer' ` over the logs finds nothing.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-003 — Log level at run time

- **Needs:** R-STACK
- **Do:** set `PLUX_OBSERVABILITY_LOG_LEVEL=debug` for `plux-server` (in `compose.yaml` or an override), `docker compose … up -d plux-server`, repeat an edit.
- **Expect:** more lines appear; setting it back reduces them.
- **Why:** diagnosis on demand (OBS-001).
- **Look at:** the variable is listed in the server reference.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-004 — Prometheus scrapes the server

- **Needs:** R-STACK
- **Do:** open <http://localhost:9090/targets>; query `plux_rpc_requests_total` and `rate(plux_rpc_requests_total[1m])`.
- **Expect:** the `plux-server` target is up; the series grows when you use Studio.
- **Why:** metrics have bounded cardinality and are catalogued (OBS-002).
- **Look at:** the label sets: no user ids or paths.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-005 — Grafana dashboards

- **Needs:** R-STACK
- **Do:** open <http://localhost:3000> (credentials in `deploy/compose/.secrets/grafana.env`); open the dashboards `plux-server-overview`, `plux-slo-burn`, `plux-postgresql`, `plux-valkey`, `plux-object-storage`, and the analytics ones.
- **Expect:** panels show data from your earlier checks (publishes, manifest requests, function calls).
- **Why:** the observability stack is provisioned (OBS-004).
- **Look at:** the JSON in `deploy/observability/dashboards`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-006 — Alert rules load

- **Needs:** R-STACK
- **Do:** open <http://localhost:9090/rules> or `promtool check rules deploy/observability/rules/plux.rules.yml`.
- **Expect:** the SLO burn rules (`PluxManifestBurnFast`, `PluxPublishBurnFast` …) are loaded; each alert links to a runbook in `docs/runbooks/alerts/`.
- **Why:** objectives have burn-rate alerts (OBS-005, ADR-0085).
- **Look at:** the objectives: manifest 99.95%, function 99.9%, publish 99.5%.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-007 — Trace a request

- **Needs:** R-STACK
- **Do:** make a Studio edit; find its `traceparent`/request id in the server log; look for the span in the OpenTelemetry collector output (`docker compose … logs otel-collector`).
- **Expect:** spans for the call with the same trace id as the log line.
- **Why:** logs, metrics and traces correlate (OBS-003).
- **Look at:** the collector config `deploy/compose/otel-collector.yaml`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-008 — Studio installation pages

- **Needs:** R-STUDIO (installation administrator)
- **Do:** open `/installation`: health, **Organisations** (`/installation/organisations`), storage, jobs, backups, trusted CI issuers, audit.
- **Expect:** each page loads: component health, organisations (`acme`, `globex`), storage use, job queues, backup status (empty on Compose), the audit chain state.
- **Why:** operators see the installation without a shell (STU-130).
- **Look at:** `plux_jobs_queue_depth` equals the jobs page.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-009 — Restart without losing anything

- **Needs:** R-STACK, R-DEVICE-SHOP
- **Do:** `make compose-down`; `make compose-up`; open Studio; sync the device.
- **Expect:** all apps, releases and users are still there (volumes are kept); the device syncs.
- **Why:** state lives in volumes, not containers (DEP-001, SRV-007).
- **Look at:** `docker compose … logs plux-server` shows a clean start and `migrateOnStart` finding nothing to apply.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-010 — Back up the database: create R-DUMP

- **Needs:** R-STACK
- **Do:** `docker compose -f deploy/compose/compose.yaml exec -T postgres pg_dump -U postgres -Fc plux > /tmp/plux.dump; ls -l /tmp/plux.dump`
- **Expect:** a non-empty dump file.
- **Why:** a backup you can read is the start of a restore (DEP-010).
- **Look at:** Compose has no WAL archiving; the Kubernetes chart does (L2-OPS-016).
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-011 — Restore into a scratch database

- **Needs:** R-DUMP
- **Do:**
  1. `docker compose … exec -T postgres createdb -U postgres plux_restore`
  2. `docker compose … exec -T postgres pg_restore -U postgres -d plux_restore < /tmp/plux.dump`
  3. `docker compose … exec -T postgres psql -U postgres -d plux_restore -c 'select count(*) from audit_log'` (use any table name `\dt` shows).
  4. `dropdb` when done.
- **Expect:** the restore finishes and counts match the live database.
- **Why:** restores are practised, not assumed (DEP-010).
- **Look at:** [backup and restore runbook](../../runbooks/backup-and-restore.md) for the production procedure.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-012 — Object storage holds bundles

- **Needs:** R-STACK
- **Do:** after a publish, list the bucket through SeaweedFS's S3 port (see `compose.yaml`) or fetch a bundle at `https://localhost:8080/v1/objects/bundles/<hash>`.
- **Expect:** content-addressed objects; the fetched bundle has `Cache-Control: public, max-age=31536000, immutable`.
- **Why:** bundles are immutable and cacheable at a CDN (DEP-041).
- **Look at:** the name is the content hash.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-013 — Dependency outage and recovery

- **Needs:** R-STACK
- **Do:** `docker compose … stop valkey`; call Studio and a device sync; start it again.
- **Expect:** `readyz` reflects the outage; devices keep working from their cache; replay protection follows its fallback setting; after the restart all is normal.
- **Why:** graceful degradation (SRV-007, SEC-024).
- **Look at:** `plux_dpop_replay_cache_degraded` and `plux_dpop_replay_cache_fallback_total`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-014 — Load test: create R-LOAD

- **Needs:** R-STACK
- **Do:** `make load-test > /tmp/load.json`
- **Expect:** a JSON report for a fleet of synthetic devices; no errors beyond expected limits.
- **Why:** capacity numbers are measured on your hardware (NFR-023, NFR-040).
- **Look at:** the Grafana server overview during the run; [sizing guide](../../operations/sizing.md).
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-015 — Configuration validation

- **Needs:** R-STACK
- **Do:** `bin/plux-server config validate -config deploy/compose/plux-server.yaml`; add a typo key to a copy and validate again.
- **Expect:** the good file passes with no connection made; the typo is an error naming the key.
- **Why:** strict configuration (SRV-008).
- **Look at:** a section of a later phase is refused by name.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-016 — Kubernetes: chart, HA and rolling upgrade on kind

- **Needs:** Docker, kind, kubectl
- **Do:** `make helm-lint`, then `make e2e-kind`.
- **Expect:** the chart lints and renders for every values file; on kind it installs with CloudNativePG, `helm test` passes, a rolling upgrade serves requests without failures, and two database failovers recover.
- **Why:** self-hosted high availability (DEP-003, DEP-004).
- **Look at:** `helm history plux -n plux`; roles `api`, `worker`, `fnrunner`, `egress`, `builder`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-017 — Restore drill

- **Needs:** Docker, kind, kubectl
- **Do:** `make restore-drill`
- **Expect:** the installation is restored into a new namespace at a random point in the last hour; manifests, bundle hashes and the audit chain verify; RPO and RTO are reported against 5 minutes and 1 hour.
- **Why:** backups are proven by a drill (DEP-010).
- **Look at:** the `markers/postgresql/` objects and `PluxConsistencyMarkerStale`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-018 — Air-gapped bundle

- **Needs:** Docker, kind, kubectl, enterprise licence
- **Do:** `make airgap-kind`; read [air-gapped guide](../../operations/air-gapped.md).
- **Expect:** a signed bundle is built and verified (a changed file is refused), then installed on kind with outbound networking denied.
- **Why:** installs with no internet (DEP-040).
- **Look at:** `plux-airgap verify … --key bundle.key.pub`; the egress list `docs/operations/egress.md`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-019 — Upgrade and rollback procedure

- **Needs:** L2-OPS-016
- **Do:** during `make e2e-kind` note the upgrade steps; read [upgrade rollback runbook](../../runbooks/upgrade-rollback.md); on the kind cluster run `helm rollback plux <revision> -n plux` and `helm test plux -n plux`.
- **Expect:** the previous version runs against the newer schema (migrations are expand-only) and `helm test` passes.
- **Why:** upgrades can be undone (DEP-030).
- **Look at:** `/readyz` on every replica.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-020 — Infrastructure as code

- **Needs:** OpenTofu (`make iac-check` installs it)
- **Do:** `make iac-check`; read `deploy/iac/README.md`.
- **Expect:** the AWS, GCP and Azure modules format-check, validate and pass their tests; each outputs `helm_values` keys that match the chart.
- **Why:** reproducible cloud installs (DEP-050, DEP-051).
- **Look at:** the secure defaults each module sets.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-021 — Egress list matches the code

- **Needs:** repository checkout
- **Do:** read `docs/operations/egress.md`; run `make go-test` or the egress policy test named in `docs/engineering/ci.md`.
- **Expect:** every outbound destination of the server is listed, and CI fails if one is not.
- **Why:** nothing leaves a self-hosted installation unannounced (SEC-163).
- **Look at:** the generated list.
- **Result:** ☐ pass ☐ fail — notes:

### L2-OPS-022 — Documentation site builds

- **Needs:** Bun
- **Do:** `make docs-site`
- **Expect:** the site builds into `site/dist` and fails on any broken internal link.
- **Why:** docs stay true.
- **Look at:** the Operations guide is in the output.
- **Result:** ☐ pass ☐ fail — notes:

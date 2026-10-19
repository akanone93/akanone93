# deploy

Deployment and operations assets.

| Directory | Contents | Arrives |
|---|---|---|
| [`compose/`](compose/README.md) | Single-node Docker Compose stack (`DEP-002`) | P2 |
| [`helm/`](helm/plux/README.md) | HA Helm chart (`DEP-003`, `DEP-004`): `make helm-lint`, `make e2e-kind` | P9 |
| `terraform/` | Reference infrastructure modules (`DEP-050`) | P9 |
| `observability/` | Grafana dashboards and Prometheus rules (`OBS-004`) | P9 |

# Plux Helm chart

Installs Plux Server in high availability on Kubernetes (`DEP-003`, `DEP-004`; ADR-0083):
the `api` and `worker` roles, and with `functions.enabled` the `fnrunner`, `egress` and
`builder` roles, with PostgreSQL through CloudNativePG, Valkey with Sentinel and SeaweedFS,
or external services for each.

## Install

```sh
helm install plux deploy/helm/plux --namespace plux --create-namespace \
  --set server.publicBaseURL=https://plux.example.com \
  --set signing.vault.address=https://vault.example.com
```

`server.publicBaseURL` and the signing backend's settings are required; the chart refuses
to render without them. `values.schema.json` is strict: a misspelt key or a value of the
wrong kind fails `helm install`.

## Topology

| Value | Modes | Default |
|---|---|---|
| `postgresql.mode` | `cnpg` (a CloudNativePG `Cluster`; install the operator first) or `external` | `cnpg` |
| `valkey.mode` | `sentinel` or `external` | `sentinel` |
| `objectStorage.mode` | `seaweedfs` or `external` (S3; recommended for production) | `seaweedfs` |
| `signing.backend` | `vault` (Transit) or `file` | `vault` |
| `internalTLS.mode` | `generate` or `existingSecrets` | `generate` |

Every role runs at least two replicas with PodDisruptionBudgets, horizontal autoscaling and
topology spread (`scheduling.zoneSpread`, `scheduling.nodeSpread`). Pods run under Pod
Security `restricted`; NetworkPolicies deny by default, and `fnrunner` and `builder` reach
only `egress` and hold no database credential or signing capability (L-4). Credentials come
from existing Secrets, never from inline values.

## Upgrade

`helm upgrade` runs the database migration as a pre-upgrade Job, then rolls each role.
Migrations are expand/contract, so the previous release keeps working during the roll.

## Tests

- `make helm-lint` lints the chart, renders it for every `ci/*-values.yaml` (default,
  external, HA, kind), validates the output against the Kubernetes and CloudNativePG schemas
  and checks Pod Security, L-4 and the values schema. Runs in CI on chart changes.
- `make e2e-kind` installs the chart on a kind cluster with CloudNativePG, runs `helm test`,
  a rolling upgrade and Valkey and PostgreSQL failovers (`deploy/helm/e2e-kind.sh`). Needs
  Docker; runs daily, manually and on pull requests that change the chart or the image.
- Tools: `make install-helm-tools` (pinned Helm, kind, kubeconform).

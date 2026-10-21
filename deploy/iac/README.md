<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Infrastructure-as-code modules (`DEP-050`)

Reference OpenTofu modules that create the infrastructure a Plux installation
runs on, for AWS (`aws/`), Google Cloud (`gcp/`) and Azure (`azure/`). Each
module creates a private Kubernetes cluster, a database (or leaves it to
CloudNativePG on the cluster), a versioned object-storage bucket, a Redis or
Valkey compatible cache, the KMS keys and the networking, and outputs a
`helm_values` object whose keys match `deploy/helm/plux/values.yaml`.

They are references, not a product: copy one into your own root module, pin
its provider versions and back its state up before use.

## Secure defaults

- No public bucket: public access blocks or public access prevention is on.
- Encryption at rest with a customer managed key on every store, and TLS on
  the cache and the database.
- Private endpoints only: the cluster API, the database, the cache and the
  key store have no public address.
- Deletion protection on the database (and on the cluster where the provider
  has the setting). Azure has no such setting for a flexible server, so a
  `CanNotDelete` management lock stands in for it.
- No credential in code or in outputs. The database password is managed by
  the provider (AWS Secrets Manager) or absent (Azure Entra ID only); the
  cache password is a sensitive variable that must come from a secret store.
  `helm_values` carries no secret.

## Layout

| Path | Contents |
|---|---|
| `aws/`, `gcp/`, `azure/` | One module each: `versions.tf`, `variables.tf`, `main.tf`, `outputs.tf` |
| `<cloud>/tests/plan.tftest.hcl` | `tofu test` with `mock_provider`: asserts the planned resources, the secure defaults and the rejected inputs |
| `policy.jq`, `policy.sh` | Secure-default rules over the JSON of a plan (`tofu show -json`) |

The modules configure no provider. The root module that calls them supplies
the credentials, the region and the project, so none of them is written here.

## Checks

`make iac-check` is the intended entry point (`DEP-050`); it is not added yet
(see "Open questions"). It runs, for each module:

1. `tofu init -backend=false` and `tofu validate`;
2. `tofu test`, which plans against a mock provider and needs no account;
3. `policy.sh` over the JSON of a credential-free plan of the module.

Mock-provider tests prove the plan the module would make, not how the cloud
behaves. They catch a removed control or a weakened default; they cannot catch
a setting the provider rejects at apply.

## PostgreSQL: managed or CloudNativePG

`postgresql_mode` chooses where the database runs:

- `managed` creates the cloud's PostgreSQL service and outputs
  `postgresql.mode: external` for the chart.
- `cnpg` creates no database. The chart's CloudNativePG `Cluster` runs on the
  cluster instead (`postgresql.mode: cnpg`); the operator must be installed.

## Running the apply

The apply runs from inside the network the module creates, because the
cluster, the database and the key store are private. Use a runner in that
network, or a bastion, and give it the cloud credentials through the
environment (for example `TF_VAR_valkey_auth_token`), never through a file in
the repository.

## Networking

Subnets are private and have no route to an internet gateway. Nodes pull
images through VPC endpoints (AWS), Private Google Access (Google Cloud) or the
operator's own egress path. The chart's NetworkPolicies are enforced by the
cluster's network plugin: the VPC CNI with network policy (AWS), Dataplane V2
(Google Cloud) and Cilium (Azure).

## Open questions

These are not resolved in this change and need a maintainer decision:

- **OpenTofu is not pinned yet.** `make install-tofu` and the Makefile pin
  (`TOFU_VERSION`, SHA-256 of the linux-amd64 archive) need the release checksum
  from the official OpenTofu release, which was not readable when these modules
  were written. Until then nothing here has been run through `tofu`.
- **Signing key.** The chart's `signing.backend` is `vault` or `file`; it has no
  KMS backend. The AWS, Google Cloud and Azure signing keys are created so that
  a KMS-backed signer can use them, but nothing consumes them yet. The key spec
  (`signing_key_spec`, `signing_algorithm`, `signing_key_curve`) must match the
  Plux server's signing algorithm before the key is used.
- **Azure object storage.** Azure Blob Storage has no S3 API, and the chart's
  `objectStorage` needs one. The Azure module therefore outputs no
  `objectStorage` values: use SeaweedFS in the chart, or a gateway, on Azure.
- **Customer managed keys for the database and the cache.** Cloud SQL and
  Memorystore on Google Cloud need their service agents granted the data key;
  the module grants only the Cloud Storage agent. The cache's password is held
  in the state file.
- **Provider versions.** The provider constraints (`aws` ~> 5.80, `google` ~> 6.0,
  `azurerm` ~> 4.0) and the resource arguments were written without running
  `tofu validate` against them.
- **Sizing.** Node counts and instance sizes are starting points; the sizing
  guide (`DEP-051`) is a separate item of E9.

# ============================================================================
# mk/helm.mk — the Helm chart and its Kubernetes end-to-end test
# ============================================================================
# Included by the Makefile. A change to this file runs the chart's jobs on a
# pull request (ci/affected.json, ADR-0043). The tools are pinned in the
# Makefile header and installed by `make install-helm-tools`.

.PHONY: helm-lint e2e-kind restore-drill

##@ Kubernetes (Helm chart)

HELM_DIR := deploy/helm
# The image the kind test builds from backend/Dockerfile and loads into the
# cluster; E2E_KEEP=1 keeps the cluster afterwards.
E2E_IMAGE ?= plux-server:e2e

# Verifies: DEP-003, DEP-004.
helm-lint: ## Lint the Helm chart, render it for every ci/*-values.yaml and validate the output (Kubernetes and CloudNativePG schemas, Pod Security, L-4)
	HELM="$(HELM)" KUBECONFORM="$(KUBECONFORM)" $(HELM_DIR)/lint.sh

# Verifies: DEP-003, DEP-004.
e2e-kind: ## Install the chart on a kind cluster with CloudNativePG, run helm test, a rolling upgrade and two failovers (needs Docker and kubectl)
	docker build -f backend/Dockerfile --target server -t $(E2E_IMAGE) .
	HELM="$(HELM)" KIND="$(KIND)" E2E_IMAGE="$(E2E_IMAGE)" CNPG_VERSION="$(CNPG_VERSION)" CNPG_SHA256="$(CNPG_SHA256)" $(HELM_DIR)/e2e-kind.sh

# Verifies: DEP-010.
restore-drill: ## Restore the installation into a new namespace on kind from its backups at a random point in the last hour, verify releases, bundles and the audit chain, and measure RPO and RTO (needs Docker and kubectl)
	docker build -f backend/Dockerfile --target server -t $(E2E_IMAGE) .
	GO="$(GO)" HELM="$(HELM)" KIND="$(KIND)" E2E_IMAGE="$(E2E_IMAGE)" CNPG_VERSION="$(CNPG_VERSION)" CNPG_SHA256="$(CNPG_SHA256)" $(HELM_DIR)/restore-drill.sh

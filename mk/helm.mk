# ============================================================================
# mk/helm.mk — the Helm chart and its Kubernetes end-to-end test
# ============================================================================
# Included by the Makefile. A change to this file runs the chart's jobs on a
# pull request (ci/affected.json, ADR-0043). The tools are pinned in the
# Makefile header and installed by `make install-helm-tools`.

.PHONY: helm-lint e2e-kind restore-drill airgap-kind walkthrough-kind walkthrough-plan

##@ Kubernetes (Helm chart)

HELM_DIR := deploy/helm
# The image the kind test builds from backend/Dockerfile and loads into the
# cluster; E2E_KEEP=1 keeps the cluster afterwards.
E2E_IMAGE ?= plux-server:e2e

# Verifies: DEP-003, DEP-004, DEP-010, OBS-004, OBS-005.
helm-lint: ## Lint the Helm chart, render it for every ci/*-values.yaml and validate the output (Kubernetes and CloudNativePG schemas, Pod Security, L-4), and check the alert rules, runbooks and dashboards
	HELM="$(HELM)" KUBECONFORM="$(KUBECONFORM)" $(HELM_DIR)/lint.sh
	cd tools && $(GO) test -count=1 ./internal/observability/

# Verifies: DEP-003, DEP-004.
e2e-kind: ## Install the chart on a kind cluster with CloudNativePG, run helm test, a rolling upgrade and two failovers (needs Docker and kubectl)
	docker build -f backend/Dockerfile --target server -t $(E2E_IMAGE) .
	HELM="$(HELM)" KIND="$(KIND)" E2E_IMAGE="$(E2E_IMAGE)" CNPG_VERSION="$(CNPG_VERSION)" CNPG_SHA256="$(CNPG_SHA256)" $(HELM_DIR)/e2e-kind.sh

# Verifies: DEP-010.
restore-drill: ## Restore the installation into a new namespace on kind from its backups at a random point in the last hour, verify releases, bundles and the audit chain, and measure RPO and RTO (needs Docker and kubectl)
	docker build -f backend/Dockerfile --target server -t $(E2E_IMAGE) .
	GO="$(GO)" HELM="$(HELM)" KIND="$(KIND)" E2E_IMAGE="$(E2E_IMAGE)" CNPG_VERSION="$(CNPG_VERSION)" CNPG_SHA256="$(CNPG_SHA256)" $(HELM_DIR)/restore-drill.sh

# Verifies: DEP-040.
airgap-kind: ## Build and verify an air-gapped bundle, then install it on kind with outbound networking denied, images from a local registry, and run helm test (needs Docker, kubectl, skopeo and the enterprise module)
	docker build -f backend/Dockerfile --target server -t $(E2E_IMAGE) .
	GO="$(GO)" HELM="$(HELM)" KIND="$(KIND)" E2E_IMAGE="$(E2E_IMAGE)" CNPG_VERSION="$(CNPG_VERSION)" CNPG_SHA256="$(CNPG_SHA256)" $(HELM_DIR)/airgap-kind.sh

# Verifies: DEP-004, GOV-006, GOV-007, GOV-020, GOV-021, GOV-022, GOV-024, GOV-025, GOV-026, REL-053, REL-060, SEC-142, SEC-143, SEC-162, NFR-024.
walkthrough-kind: ## Run the scripted regulated-industry audit walkthrough (SSO and SCIM, approvals, staged rollout and health gate, kill switch through break-glass, mandatory update, audit export, reconstruction, rolling upgrade, data-subject request) against the HA chart on kind with the synthetic fleet, and verify the signed evidence bundle (needs Docker, kubectl, the enterprise image and the two browser hooks of test/walkthrough/walkthrough-kind.sh)
	docker build -f backend/Dockerfile --target server -t $(E2E_IMAGE) .
	GO="$(GO)" HELM="$(HELM)" KIND="$(KIND)" E2E_IMAGE="$(E2E_IMAGE)" CNPG_VERSION="$(CNPG_VERSION)" CNPG_SHA256="$(CNPG_SHA256)" test/walkthrough/walkthrough-kind.sh

walkthrough-plan: ## Print the plan of the audit walkthrough: every step, the requirements it exercises and what it does; starts nothing
	cd tools && $(GO) run ./cmd/walkthrough run -dry-run

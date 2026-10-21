# ============================================================================
# mk/iac.mk — the infrastructure-as-code modules (deploy/iac)
# ============================================================================
# Included by the Makefile. The OpenTofu pin (TOFU_VERSION) and its install
# target (install-tofu) are in the Makefile.

.PHONY: iac-check

##@ Infrastructure as code

IAC_CLOUDS := aws gcp azure

# Verifies: DEP-050.
iac-check: install-tofu ## Format-check, initialise, validate and test the OpenTofu modules (DEP-050); init needs the provider registry
	"$(TOFU)" fmt -check -recursive deploy/iac
	@for cloud in $(IAC_CLOUDS); do \
		echo "tofu validate and test: deploy/iac/$$cloud"; \
		(cd "deploy/iac/$$cloud" && "$(TOFU)" init -backend=false -input=false && "$(TOFU)" validate && "$(TOFU)" test) || exit 1; \
	done

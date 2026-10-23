# ============================================================================
# mk/iac.mk — the infrastructure-as-code modules (deploy/iac)
# ============================================================================
# Included by the Makefile. The OpenTofu pin (TOFU_VERSION) and its install
# target (install-tofu) are in the Makefile.

.PHONY: iac-check iac-mirror

##@ Infrastructure as code

IAC_CLOUDS := aws gcp azure

# Verifies: DEP-050.
iac-check: install-tofu ## Format-check, initialise, validate and test the OpenTofu modules (DEP-050); init needs the provider registry
	"$(TOFU)" fmt -check -recursive deploy/iac
	@for cloud in $(IAC_CLOUDS); do \
		echo "tofu validate and test: deploy/iac/$$cloud"; \
		(cd "deploy/iac/$$cloud" && "$(TOFU)" init -backend=false -input=false && "$(TOFU)" validate && "$(TOFU)" test) || exit 1; \
	done

# Where the provider registry is unreachable (agent sessions), the pinned
# providers come from releases.hashicorp.com into a filesystem mirror; then
# TF_CLI_CONFIG_FILE=$(IAC_MIRROR)/tofurc make iac-check runs offline. The
# versions are read from each module's versions.tf, so the mirror always holds
# exactly what CI tests.
IAC_MIRROR ?= $(abspath build/iac-mirror)

iac-mirror: ## Build a local mirror of the pinned OpenTofu providers and its CLI config (for sessions without the registry)
	@set -e; mkdir -p "$(IAC_MIRROR)"; \
	for cloud in $(IAC_CLOUDS); do \
		f="deploy/iac/$$cloud/versions.tf"; \
		name=$$(sed -n 's|^[[:space:]]*source[[:space:]]*=[[:space:]]*"hashicorp/\(.*\)"|\1|p' "$$f" | head -n 1); \
		version=$$(sed -n 's|^[[:space:]]*version[[:space:]]*=[[:space:]]*"\([0-9.]*\)"|\1|p' "$$f" | head -n 1); \
		dir="$(IAC_MIRROR)/registry.opentofu.org/hashicorp/$$name/$$version/linux_amd64"; \
		if [ ! -d "$$dir" ]; then \
			echo "mirror: $$name $$version"; mkdir -p "$$dir"; \
			curl -fsSL -o "$$dir.zip" "https://releases.hashicorp.com/terraform-provider-$$name/$$version/terraform-provider-$${name}_$${version}_linux_amd64.zip"; \
			(cd "$$dir" && unzip -q -o "$$dir.zip"); rm -f "$$dir.zip"; \
		fi; \
	done; \
	printf 'provider_installation {\n  filesystem_mirror {\n    path    = "%s"\n    include = ["registry.opentofu.org/hashicorp/*"]\n  }\n}\n' "$(IAC_MIRROR)" > "$(IAC_MIRROR)/tofurc"; \
	echo "export TF_CLI_CONFIG_FILE=$(IAC_MIRROR)/tofurc"

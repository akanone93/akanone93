# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# ============================================================================
# mk/l10n.mk — localisation data derived from CLDR (ADR-0066)
# ============================================================================
# Included by the Makefile.

.PHONY: cldr-data

# The cldr-json release that schema/l10n/cldr/ is derived from, and the
# SHA-256 of its full archive; move both to update the data.
CLDR_VERSION := 48.2.3
CLDR_SHA256  := d580253a99baea84f7fc361b91a73023f2268331a1671244cf8180b4e008e0e4

cldr-data: ## Re-derive schema/l10n/cldr from the pinned CLDR release (ADR-0066)
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	curl -sSfL -o "$$tmp/cldr.zip" "https://github.com/unicode-org/cldr-json/releases/download/$(CLDR_VERSION)/cldr-$(CLDR_VERSION)-json-full.zip"; \
	echo "$(CLDR_SHA256)  $$tmp/cldr.zip" | sha256sum -c - >/dev/null || { echo "✗ the CLDR archive does not match CLDR_SHA256" >&2; exit 1; }; \
	cd tools && $(GO) run ./cmd/cldrdata -zip "$$tmp/cldr.zip" -version $(CLDR_VERSION) -out ../schema/l10n/cldr

# ============================================================================
# mk/studio.mk — Studio gates (Bun, TypeScript)
# ============================================================================
# Included by the Makefile. A change to this file runs the Studio job on a
# pull request (ci/affected.json, ADR-0043).

.PHONY: studio-check studio-bounds studio-dev studio-build studio-size studio-install studio-fmt studio-lint studio-typecheck studio-test studio-cover studio-gen-theme studio-gen-icons studio-e2e studio-design-gate studio-conformance studio-conformance-update studio-i18n-check

##@ Studio (Bun, TypeScript)

studio-check: studio-install studio-lint studio-bounds studio-i18n-check studio-typecheck studio-cover studio-build studio-size ## All Studio gates

studio-install: ## Install Studio dependencies exactly as locked
	cd studio && bun install --frozen-lockfile

studio-fmt: ## Format and fix Studio code with Biome
	cd studio && bunx biome check --write .

studio-lint: ## Lint and format-check Studio code with Biome
	cd studio && bunx biome ci --colors=off .

studio-bounds: ## Check Studio's import boundaries (studio-development §2.1)
	cd tools && go run ./cmd/studiobounds ../studio

studio-i18n-check: studio-install ## Catalogues are complete and current: strict extract and compile (STU-012)
	cd studio && apps/web/node_modules/.bin/lingui extract --clean && apps/web/node_modules/.bin/lingui compile --strict && rm -f apps/web/src/locales/*/messages.js
	git diff --exit-code -- studio/apps/web/src/locales

studio-typecheck: studio-gen-theme studio-gen-icons ## Type-check Studio code
	cd studio && bun run typecheck

studio-test: studio-gen-theme studio-gen-icons ## Run Studio tests
	cd studio && bun run test

studio-conformance: ## Compare Studio's layout with Flutter's goldens (STU-005)
	cd studio && bun test packages/layout/conformance

studio-conformance-update: ## Regenerate the layout goldens with Flutter
	cd test/layout-conformance && UPDATE_GOLDENS=1 flutter test

studio-cover: studio-test ## Enforce Studio coverage floors (QA-001)
	$(COVGATE) -kind studio studio/coverage/lcov.info

studio-dev: studio-gen-theme ## Run the Studio web app with the Vite dev server
	cd studio && bun run dev

studio-gen-theme: studio-install ## Regenerate Studio's Tailwind theme from the design tokens
	cd studio && bun run gen:theme

studio-gen-icons: studio-install ## Regenerate the Icon set from @phosphor-icons/core (UI language U8)
	cd studio/packages/ui && bun scripts/gen-icons.ts

studio-build: studio-install studio-gen-theme studio-gen-icons ## Build the Studio web app into studio/apps/web/dist
	cd studio && bun run build && bun scripts/precompress.ts apps/web/dist

studio-size: studio-build ## Check the initial JavaScript against the budget (STU-122)
	cd studio && bun run size

# Browsers are preinstalled to match the pinned @playwright/test; never run
# `playwright install` here. Not part of studio-check: it runs at slice gates
# and in CI (O-11).
studio-e2e: studio-build ## Run Studio's end-to-end tests (Playwright, axe)
	cd studio && bunx playwright test --config e2e/playwright.config.ts

# O-4: the detector only reports in S0; the exit status stays 0 until S3.
studio-design-gate: studio-build ## Run the design-gate detector over the built web app (report-only until S3, O-4)
	cd studio && { bunx impeccable detect apps/web/dist --no-config 2>&1 || true; } > design-gate-report.txt
	@tail -n 3 studio/design-gate-report.txt

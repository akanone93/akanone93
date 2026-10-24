# ============================================================================
# mk/studio.mk — Studio gates (Bun, TypeScript)
# ============================================================================
# Included by the Makefile. A change to this file runs the Studio job on a
# pull request (ci/affected.json, ADR-0043).

.PHONY: studio-check studio-bounds studio-dev studio-build studio-size studio-install studio-fmt studio-lint studio-typecheck studio-test studio-cover

##@ Studio (Bun, TypeScript)

studio-check: studio-install studio-lint studio-bounds studio-typecheck studio-cover studio-build studio-size ## All Studio gates

studio-install: ## Install Studio dependencies exactly as locked
	cd studio && bun install --frozen-lockfile

studio-fmt: ## Format and fix Studio code with Biome
	cd studio && bunx biome check --write .

studio-lint: ## Lint and format-check Studio code with Biome
	cd studio && bunx biome ci --colors=off .

studio-bounds: ## Check Studio's import boundaries (studio-development §2.1)
	cd tools && go run ./cmd/studiobounds ../studio

studio-typecheck: ## Type-check Studio code
	cd studio && bun run typecheck

studio-test: ## Run Studio tests
	cd studio && bun test

studio-cover: studio-test ## Enforce Studio coverage floors (QA-001)
	$(COVGATE) -kind studio studio/coverage/lcov.info

studio-dev: ## Run the Studio web app with the Vite dev server
	cd studio && bun run dev

studio-build: studio-install ## Build the Studio web app into studio/apps/web/dist
	cd studio && bun run build

studio-size: studio-build ## Check the initial JavaScript against the budget (STU-122)
	cd studio && bun run size

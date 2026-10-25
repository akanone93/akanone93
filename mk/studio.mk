# ============================================================================
# mk/studio.mk — Studio gates (Bun, TypeScript)
# ============================================================================
# Included by the Makefile. A change to this file runs the Studio job on a
# pull request (ci/affected.json, ADR-0043).

.PHONY: studio-check studio-bounds studio-dev studio-build studio-size studio-install studio-fmt studio-lint studio-typecheck studio-test studio-cover studio-gen-theme studio-gen-icons studio-e2e studio-design-gate studio-conformance studio-conformance-update studio-i18n-check studio-bench-canvas studio-validator zap-studio studio-image-repro

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

studio-typecheck: studio-gen-theme studio-gen-icons studio-validator ## Type-check Studio code
	cd studio && bun run typecheck

studio-test: studio-gen-theme studio-gen-icons ## Run Studio tests
	cd studio && bun run test

studio-conformance: ## Compare Studio's layout with Flutter's goldens (STU-005)
	cd studio && bun test packages/layout/conformance

studio-conformance-update: ## Regenerate the layout goldens with Flutter
	cd test/layout-conformance && UPDATE_GOLDENS=1 flutter test

studio-cover: studio-test ## Enforce Studio coverage floors (QA-001)
	$(COVGATE) -kind studio studio/coverage/lcov.info

# The API the dev BFF proxies to: plux-server from deploy/compose (TLS under its development CA).
STUDIO_DEV_API ?= https://localhost:8080
STUDIO_DEV_CA ?= $(CURDIR)/deploy/compose/.secrets/ca/ca.crt

studio-dev: studio-gen-theme ## Run Studio for development: the BFF and the Vite dev server (http://127.0.0.1:5173)
	cd studio && PLUX_BFF_STATE_SECRET=$$(openssl rand -base64 32) PLUX_BFF_API_UPSTREAM=$(STUDIO_DEV_API) PLUX_BFF_PUBLIC_BASE_URL=http://127.0.0.1:5173 \
		NODE_EXTRA_CA_CERTS=$(STUDIO_DEV_CA) bun run dev

studio-gen-theme: studio-install ## Regenerate Studio's Tailwind theme from the design tokens
	cd studio && bun run gen:theme

studio-gen-icons: studio-install ## Regenerate the Icon set from @phosphor-icons/core (UI language U8)
	cd studio/packages/ui && bun scripts/gen-icons.ts

# The Go validator in WebAssembly (S2.1, F9) and Go's loader, staged beside the validator worker.
# Vite emits both as hashed assets; neither is committed.
STUDIO_VALIDATOR_DIR := studio/packages/validator/build

studio-validator: ## Build the validator WebAssembly module and stage Go's wasm_exec.js (S2.1)
	mkdir -p $(STUDIO_VALIDATOR_DIR)
	cd backend && GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o $(CURDIR)/$(STUDIO_VALIDATOR_DIR)/validator.wasm ./cmd/plux-validator-wasm
	cd backend && install -m 644 "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(CURDIR)/$(STUDIO_VALIDATOR_DIR)/wasm_exec.js

studio-build: studio-install studio-gen-theme studio-gen-icons studio-validator ## Build the Studio web app into studio/apps/web/dist
	cd studio && bun scripts/stage-canvaskit.ts && bun run build && bun scripts/precompress.ts apps/web/dist

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

# Software rendering only (SwiftShader, no GPU): the canvas budgets hold on
# machines without a GPU (ADR-0013 M-12). Not part of studio-check.
studio-bench-canvas: studio-install ## Canvas benchmark in software rendering (STU-045, STU-120, M-12)
	cd studio && bun packages/canvas/bench/run.ts

# Verifies: QA-008. The Compose stack runs Studio on port 8081 (make compose-up).
STUDIO_ZAP_TARGET ?= http://localhost:8081

zap-studio: ## Scan the running Compose stack's Studio with the ZAP baseline, ignoring only test/zap/studio-rules.tsv; report in build/zap (QA-008)
	@for i in $$(seq 60); do \
		curl -fsS $(STUDIO_ZAP_TARGET)/livez >/dev/null 2>&1 && break; \
		[ "$$i" = 60 ] && { echo "✗ no Studio answering at $(STUDIO_ZAP_TARGET): run 'make compose-up' first"; exit 1; }; \
		sleep 2; \
	done
	@mkdir -p $(ZAP_OUT) && chmod a+w $(ZAP_OUT) && cp test/zap/studio-rules.tsv $(ZAP_OUT)/studio-rules.tsv
	docker run --rm --network host -v $(ZAP_OUT):/zap/wrk:rw $(ZAP_IMAGE) \
		zap-baseline.py -t $(STUDIO_ZAP_TARGET) -c studio-rules.tsv -r zap-studio.html -J zap-studio.json -w zap-studio.md

# Verifies: CI-006. Two builds of the same commit must give the same image
# digest: no timestamp, path or ordering may reach the image.
STUDIO_COMMIT_DATE = $(shell git log -1 --format=%ct)

studio-image-repro: ## Build the Studio image twice and fail when the digests differ (CI-006)
	cd studio && bun scripts/stage-canvaskit.ts --to .canvaskit
	@for n in 1 2; do \
		docker buildx build -f studio/Dockerfile --build-arg SOURCE_DATE_EPOCH=$(STUDIO_COMMIT_DATE) \
			--no-cache --provenance=false --sbom=false \
			--output type=oci,dest=build/studio-image-$$n.tar,rewrite-timestamp=true . || exit 1; \
	done
	@a=$$(tar -xOf build/studio-image-1.tar index.json | sha256sum); b=$$(tar -xOf build/studio-image-2.tar index.json | sha256sum); \
	if [ "$$a" = "$$b" ]; then echo "✓ Studio image reproducible"; else echo "✗ Studio image digests differ"; exit 1; fi

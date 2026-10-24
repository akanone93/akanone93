# ============================================================================
# mk/dart.mk — Dart and Flutter gates
# ============================================================================
# Included by the Makefile. A change to this file runs the Dart job on a
# pull request (ci/affected.json, ADR-0043).

.PHONY: dart-check dart-get dart-lock-check dart-fmt dart-fmt-check dart-analyze dart-test dev-app-build dart-cover widgets-api widgets-api-check

DART_PACKAGES := apps/add_to_app/plux_module apps/plux_bank apps/plux_dev apps/plux_express apps/plux_viewer apps/starter packages/plux_auto_route packages/plux_charts packages/plux_components packages/plux_db_drift packages/plux_devtools packages/plux_flutter packages/plux_functions packages/plux_go_router packages/plux_host_kit packages/plux_location packages/plux_lottie packages/plux_maps packages/plux_maps_google packages/plux_markdown packages/plux_media packages/plux_native_scan packages/plux_player packages/plux_rive packages/plux_scanner packages/plux_security packages/plux_svgc packages/plux_widget_api test/bench/runtime
# Generated Dart code is verified by regeneration (CI-003), not by the formatter.
DART_SOURCES  := find packages apps test/bench test/size -name '*.dart' ! -name '*.g.dart' ! -name '*_generated.dart' ! -path '*/build/*' ! -path '*/.dart_tool/*' -print0

##@ Dart and Flutter (packages, apps)

dart-check: dart-lock-check dart-fmt-check dart-analyze widgets-api-check dart-cover ## All Dart gates

# plux_svgc is outside the workspace: the server image builds it with the
# Dart SDK alone, which cannot resolve the workspace's Flutter packages.
dart-get: ## Resolve the pub workspace and plux_svgc (updates the pubspec.lock files)
	flutter pub get
	cd packages/plux_svgc && dart pub get

dart-lock-check: dart-get ## Fail if a pubspec.lock is not in sync with its pubspecs (CI-003)
	@changed=$$(git status --porcelain -- pubspec.lock packages/plux_svgc/pubspec.lock); if [ -n "$$changed" ]; then \
		echo "$$changed"; echo "✗ pubspec.lock is out of date or uncommitted. Run 'make dart-get' and commit."; exit 1; fi

dart-fmt: ## Format hand-written Dart code
	$(DART_SOURCES) | xargs -0 dart format

dart-fmt-check: ## Fail if hand-written Dart code is not formatted
	$(DART_SOURCES) | xargs -0 dart format --output=none --set-exit-if-changed

dart-analyze: ## Analyze Dart code with strict rules; infos are fatal
	flutter analyze --fatal-infos --fatal-warnings

dart-test: ## Run Dart and Flutter tests
	@for p in $(DART_PACKAGES); do (cd $$p && flutter test); done

# The Dev app is the one build with the dev channel in it (DEV-050): without
# plux.devtools=true it cannot pair. PLUX_DEV_ROOT_KEYS is the development
# keys.json of the installation (`plux pull`); without it the app pairs with
# nothing. DEV_APP_TARGET is apk, appbundle or ios.
DEV_APP_TARGET ?= apk
DEV_APP_KEYS ?= /dev/null

dev-app-build: ## Build the Plux Dev app (DEV-001) with the dev channel in it: DEV_APP_TARGET=apk|appbundle|ios, DEV_APP_KEYS=<development keys.json>
	cd apps/plux_dev && flutter build $(DEV_APP_TARGET) --dart-define=plux.devtools=true \
		--dart-define=PLUX_DEV_ROOT_KEYS="$$(cat $(DEV_APP_KEYS))"

# How many packages dart-cover tests at once. Each package's output is kept
# and printed whole, in package order, once all have finished; a line per
# package reports it as it finishes. plux_flutter, the longest, starts first.
DART_JOBS ?= 3

dart-cover: ## Run Dart tests with coverage, DART_JOBS packages at a time; enforce floors (QA-001)
	@logs=$$(mktemp -d); trap 'rm -rf "$$logs"' EXIT; \
	printf '%s\n' $(filter packages/plux_flutter,$(DART_PACKAGES)) $(filter-out packages/plux_flutter,$(DART_PACKAGES)) | \
		xargs -P $(DART_JOBS) -I{} bash -c 'log="$$1/$$(tr / _ <<<"$$2").log"; start=$$SECONDS; \
			if (cd "$$2" && flutter test --coverage) >"$$log" 2>&1; then echo "✓ $$2 ($$((SECONDS - start)) s)"; \
			else echo "✗ $$2 ($$((SECONDS - start)) s)"; touch "$$log.failed"; fi' _ "$$logs" {}; \
	failed=; for p in $(DART_PACKAGES); do log="$$logs/$$(tr / _ <<<"$$p").log"; \
		echo "── $$p"; cat "$$log"; if [ -e "$$log.failed" ]; then failed="$$failed $$p"; fi; done; \
	if [ -n "$$failed" ]; then echo "✗ flutter test failed in:$$failed"; exit 1; fi
	$(COVGATE) -kind dart $(addsuffix /coverage/lcov.info,$(DART_PACKAGES))

WIDGETS_API := cd packages/plux_widget_api && dart run bin/extract.dart --root ../.. --flutter-version $(FLUTTER_VERSION)

widgets-api: ## Snapshot the pinned Flutter SDK's constructors and enums for the coverage table; then run 'make gen' (WGT-003)
	$(WIDGETS_API)

# Verifies: WGT-003.
widgets-api-check: ## Fail if schema/widgets/flutter-api.json does not match the pinned Flutter SDK (WGT-003)
	$(WIDGETS_API) --check

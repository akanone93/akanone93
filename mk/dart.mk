# ============================================================================
# mk/dart.mk — Dart and Flutter gates
# ============================================================================
# Included by the Makefile. A change to this file runs the Dart job on a
# pull request (ci/affected.json, ADR-0043).

.PHONY: dart-check dart-get dart-lock-check dart-fmt dart-fmt-check dart-analyze dart-test dart-cover widgets-api widgets-api-check

DART_PACKAGES := apps/add_to_app/plux_module apps/plux_bank apps/plux_express apps/starter packages/plux_auto_route packages/plux_db_drift packages/plux_devtools packages/plux_flutter packages/plux_functions packages/plux_go_router packages/plux_location packages/plux_lottie packages/plux_media packages/plux_native_scan packages/plux_rive packages/plux_scanner packages/plux_security packages/plux_svgc packages/plux_widget_api test/bench/runtime
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

# How many packages' tests run at once. Most packages spend their time
# starting and compiling rather than testing, so they overlap well; the
# longest start first, so the rest finish in their shadow (ADR-0043,
# Revision 2). Each package's output goes to build/dart-test/ and is
# printed when its tests fail.
DART_TEST_JOBS  ?= 3
DART_TEST_FIRST := packages/plux_flutter packages/plux_native_scan
DART_TEST_ORDER := $(DART_TEST_FIRST) $(filter-out $(DART_TEST_FIRST),$(DART_PACKAGES))

dart-cover: ## Run Dart tests with coverage and enforce floors (QA-001; DART_TEST_JOBS packages at once)
	@mkdir -p build/dart-test
	@printf '%s\n' $(DART_TEST_ORDER) | xargs -P $(DART_TEST_JOBS) -I{} bash -c '\
		log=build/dart-test/$$(printf %s "{}" | tr / _).log; start=$$SECONDS; \
		if (cd "{}" && flutter test --coverage) >"$$log" 2>&1; then echo "✓ {} ($$((SECONDS - start)) s)"; \
		else cat "$$log"; echo "✗ {}: flutter test failed (output in $$log)"; exit 1; fi'
	$(COVGATE) -kind dart $(addsuffix /coverage/lcov.info,$(DART_PACKAGES))

WIDGETS_API := cd packages/plux_widget_api && dart run bin/extract.dart --root ../.. --flutter-version $(FLUTTER_VERSION)

widgets-api: ## Snapshot the pinned Flutter SDK's constructors and enums for the coverage table; then run 'make gen' (WGT-003)
	$(WIDGETS_API)

# Verifies: WGT-003.
widgets-api-check: ## Fail if schema/widgets/flutter-api.json does not match the pinned Flutter SDK (WGT-003)
	$(WIDGETS_API) --check

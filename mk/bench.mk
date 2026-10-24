# ============================================================================
# mk/bench.mk — benchmarks and size (QA-007)
# ============================================================================
# Included by the Makefile. A change to this file runs the benchmark and
# size jobs on a pull request (ci/affected.json, ADR-0043).

.PHONY: load-test bench-control bench-runtime bench-runtime-ab bench-sync bench-fn bench-devloop bench-devapply bench-steps bench-pxl bench-db bench-pull bench-dpop bench-confidential size-android size-ios release-inspect

##@ Benchmarks and size (QA-007)

# Measured runs of the runtime benchmark, and runs per side of the A/B
# comparison.
RUNS ?=
# The commit the A/B comparison measures against.
BASE ?= origin/main
# The parts of each run to measure (startup, open, native, scroll, list;
# comma-separated); all when empty. CI runs them in parallel jobs.
SCENARIOS ?=

bench-runtime: ## Build the runtime benchmark in profile mode and run it under xvfb (Linux; RUNS=5, SCENARIOS=all)
	BENCH_SCENARIOS="$(SCENARIOS)" test/bench/runtime/bench.sh measure $(RUNS)

# Verifies: QA-007.
bench-runtime-ab: ## Compare the runtime benchmark with BASE's runtime; fail on regressions beyond 10% (QA-007; RUNS=10, SCENARIOS=all)
	BENCH_SCENARIOS="$(SCENARIOS)" test/bench/runtime/bench.sh compare $(BASE) $(RUNS)

# Verifies: QA-007, NFR-007.
bench-sync: ## Sync the benchmark app on the simulated slow network against a server built from source (needs PLUX_TEST_DATABASE_URL)
	@test -n "$${PLUX_TEST_DATABASE_URL:-}" || { echo "✗ set PLUX_TEST_DATABASE_URL: run 'make test-db' and export what it prints (docs/engineering/testing.md)"; exit 1; }
	cd backend && PLUX_E2E_FLUTTER="$$(command -v flutter)" $(GO) test -count=1 -timeout 30m -run TestSyncOnSlowNetwork -v ./internal/server

# The scaled run: LOAD_DEVICES synthetic devices at the runtime's pacing
# divided by LOAD_SPEEDUP, for LOAD_DURATION, against the Compose stack with
# the load overlay (docs/operations/sizing.md). The report is JSON.
LOAD_DEVICES  ?= 1000
LOAD_DURATION ?= 5m
LOAD_SPEEDUP  ?= 10
LOAD_OUT      ?= $(abspath $(COMPOSE_DIR))/.secrets/load-report.json

# Verifies: DEP-051, NFR-023.
load-test: ## Run a fleet of synthetic devices against the Compose stack and print the JSON report (LOAD_DEVICES=1000, LOAD_DURATION=5m, LOAD_SPEEDUP=10; needs Docker)
	"$(MAKE)" --no-print-directory compose-seed COMPOSE_OVERLAY=deploy/compose/compose.load.yaml
	. ./$(COMPOSE_DIR)/.secrets/dev.env && cd backend && $(GO) run ./cmd/plux-load \
		-server https://localhost:8080 -app "$$PLUX_DEV_APP" -env staging -forwarded-for \
		-ca-file "$(DEV_CA)" -devices $(LOAD_DEVICES) -duration $(LOAD_DURATION) -speedup $(LOAD_SPEEDUP) | tee "$(LOAD_OUT)"

# Verifies: NFR-024.
bench-control: ## Measure how long a control-document change takes to reach a fleet by polling and with silent push through a fake FCM; prints JSON (PLUX_BENCH_DEVICES=20, PLUX_BENCH_POLL=15s; needs PLUX_TEST_DATABASE_URL)
	@test -n "$${PLUX_TEST_DATABASE_URL:-}" || { echo "✗ set PLUX_TEST_DATABASE_URL: run 'make test-db' and export what it prints (docs/engineering/testing.md)"; exit 1; }
	cd backend && $(GO) test -count=1 -timeout 15m -run TestBenchControlPropagation -v ./internal/server

# Verifies: NFR-022, FN-023.
bench-fn: ## Measure the overhead of a server function invocation, warm and first after deploy (NFR-022; prints JSON; fails over budget)
	cd backend && PLUX_BENCH_FN=1 $(GO) test -count=1 -timeout 10m -run TestFunctionInvocationOverhead -v ./internal/fnrunner

# Verifies: NFR-030, DEV-010.
bench-devloop: ## Measure the edit-to-device latency of the dev loop, server and transport: 200 edits of one page of 50 over good_wifi (NFR-030; prints JSON; fails over 350 ms p95; needs PLUX_TEST_DATABASE_URL)
	cd backend && PLUX_BENCH_DEVLOOP=1 $(GO) test -count=1 -timeout 15m -run TestDevloop -v ./internal/devsession

# Verifies: NFR-030, DEV-010.
bench-devapply: ## Measure the device's share of the dev loop: applying a signed one-page dev bundle (NFR-030; prints JSON; fails over 150 ms p95)
	cd packages/plux_flutter && PLUX_BENCH_DEVAPPLY=1 flutter test --reporter expanded test/devtools_api/dev_apply_bench_test.dart

# The engine's cost per step, an early measurement of NFR-011 (recorded in
# docs/benchmarks/p4-routing.md, not gated: the gate arrives in P5).
bench-steps: ## Measure the action engine's cost per step (NFR-011, early; prints JSON)
	cd packages/plux_flutter && PLUX_BENCH_STEPS=1 flutter test --reporter expanded test/actions/step_bench_test.dart

# Verifies: NFR-010, PXL-004.
bench-pxl: ## Measure the evaluation of typical PXL bindings (NFR-010; prints JSON)
	cd packages/plux_flutter && PLUX_BENCH_PXL=1 flutter test --reporter expanded test/pxl/pxl_bench_test.dart

# Verifies: DB-007.
bench-db: ## Measure the time from a write to a watched query's update (DB-007; prints JSON)
	cd packages/plux_db_drift && PLUX_BENCH_DB=1 flutter test --reporter expanded test/watch_bench_test.dart

# Verifies: SEC-041, SYN-010.
bench-pull: ## Measure the Dart side of the pinned client's pull loop for a 10 MiB body (SEC-041; prints JSON)
	cd packages/plux_flutter && PLUX_BENCH_PULL=1 flutter test --reporter expanded test/platform/pull_bench_test.dart

# Verifies: NFR-012, SEC-021.
bench-dpop: ## Measure DPoP proof creation on the host: Dart side alone, and with pure-Dart P-256 (NFR-012; prints JSON)
	cd packages/plux_flutter && PLUX_BENCH_DPOP=1 flutter test --reporter expanded test/security/dpop_bench_test.dart

# Verifies: NFR-013, SEC-053.
bench-confidential: ## Measure decrypting a confidential bundle on the host with the pure-Dart cipher (NFR-013; prints JSON)
	cd packages/plux_flutter && PLUX_BENCH_CONFIDENTIAL=1 flutter test --reporter expanded test/security/confidential_bench_test.dart

# Verifies: RT-061, NFR-009.
size-android: ## Check what plux_flutter adds to the release APKs and App Bundle downloads (RT-061; needs the Android SDK)
	test/size/size.sh android

# Verifies: RT-061, NFR-009.
size-ios: ## Check what plux_flutter adds to a release iOS app for arm64 (RT-061; needs Xcode)
	test/size/size.sh ios

# The platform whose release builds `make release-inspect` builds: android or ios.
PLATFORM ?= android

# Verifies: SEC-074, DEV-050.
release-inspect: ## Build the starter app and Plux Bank for release and fail on devtools code, dev-only logging, source maps or debug files (SEC-074; PLATFORM=android needs the Android SDK, ios needs Xcode)
	test/release/inspect.sh --build $(PLATFORM)

# ============================================================================
# mk/functions.mk — the Rust SDK of Plux Functions and the vendored WAMR
# ============================================================================
# Included by the Makefile. A change to this file runs the functions job on
# a pull request (ci/affected.json, ADR-0043; maintainer, P7 plan §2.6 M4).

.PHONY: rust-check rust-e2e wamr-check wasm-check

rust-check: ## Format, lint and test the Rust SDK and its examples (FN-001, FN-004)
	cd sdk/fn && cargo fmt --all --check
	cd sdk/fn && cargo clippy --workspace --all-targets --locked -- -D warnings
	cd sdk/fn && cargo test --workspace --locked

# The builder role's own tests with the real toolchain and Binaryen: the
# loan and fee examples go through the sandboxed builder twice and must come
# out byte-identical (FN-016, FN-019); the end-to-end tests also sign and
# deploy them. Needs Rust (rustup reads sdk/fn/rust-toolchain.toml), Bun and,
# for the function package, PLUX_TEST_DATABASE_URL.
rust-e2e: ## Build the example functions with the builder role, twice, and compare (FN-016, FN-019)
	cd backend && PLUX_RUST_E2E=1 $(GO) test -count=1 -run 'E2E|EndToEnd|Rust' ./internal/builder/ ./internal/function/ ./cmd/plux/

# Every committed function module is a build output, rebuilt here with the
# pinned toolchain (Rust 1.97.0) and compared byte for byte with the commit,
# so a module that no longer comes from its source fails: the Dart host's
# fixtures, the ABI probes, the examples' dist/ artefacts and Plux Express's
# copy of the loan module. Scorecard's Binary-Artifacts alerts on them are
# dismissed on this check; the image codecs have wasm-codecs-check.
wasm-check: ## Rebuild the committed function modules (.wasm) and compare them with the commit
	cd sdk/fn && cargo build --release --locked --target wasm32-wasip1 -p loan-module -p fee-module
	cmp sdk/fn/target/wasm32-wasip1/release/loan_module.wasm packages/plux_functions/test/fixtures/loan_module.wasm
	cmp sdk/fn/target/wasm32-wasip1/release/fee_module.wasm packages/plux_functions/test/fixtures/fee_module.wasm
	cd packages/plux_functions/test/fixtures/src/probe && cargo +1.97.0 build --release --locked --target wasm32-wasip1
	cmp packages/plux_functions/test/fixtures/src/probe/target/wasm32-wasip1/release/probe.wasm packages/plux_functions/test/fixtures/probe.wasm
	RUSTUP_TOOLCHAIN=1.97.0 sh schema/testdata/functions/probe/build.sh --check
	cmp sdk/fn/examples/modules/loan-module/dist/module.wasm schema/testdata/documents/plux_express/functions/loan.calculateSchedule/target/plux/module.wasm
	cd backend && PLUX_RUST_E2E=1 $(GO) test -count=1 -run 'TestExampleArtefactsAreWhatTheBuilderMakes' ./internal/function/

# Downloads the upstream WAMR-2.4.3 archive, re-creates the vendored subset
# and fails when it differs from what is committed (P7 plan §2.6 M2, M4).
wamr-check: ## Re-create the vendored WAMR 2.4.3 from upstream and compare it with the commit
	sh packages/plux_functions/native/wamr/import.sh
	git diff --exit-code -- packages/plux_functions/native/wamr

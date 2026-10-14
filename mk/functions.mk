# ============================================================================
# mk/functions.mk — the Rust SDK of Plux Functions and the vendored WAMR
# ============================================================================
# Included by the Makefile. A change to this file runs the functions job on
# a pull request (ci/affected.json, ADR-0043; maintainer, P7 plan §2.6 M4).

.PHONY: rust-check rust-e2e wamr-check

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

# Downloads the upstream WAMR-2.4.3 archive, re-creates the vendored subset
# and fails when it differs from what is committed (P7 plan §2.6 M2, M4).
wamr-check: ## Re-create the vendored WAMR 2.4.3 from upstream and compare it with the commit
	sh packages/plux_functions/native/wamr/import.sh
	git diff --exit-code -- packages/plux_functions/native/wamr

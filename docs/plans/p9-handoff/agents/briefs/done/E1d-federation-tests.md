## Your task: P9 E1 test gap — tests for the core identity federation code (`GOV-004`)

The core federation code was merged without tests of its own (the enterprise SAML/SCIM packages are tested against fakes). Write tests only; change production code only to fix a bug a test finds (then say so in the commit and report).

Code under test (read heads and the functions you test): `backend/internal/auth/{federation.go,provider.go,mapping.go,backchannel.go,directory.go,directory_groups.go,scimtokens.go,saml.go}`, `backend/internal/api/{federation.go,federation_http.go}`, seams `backend/ext/identity/*.go`. Existing patterns to copy: `backend/internal/auth/oidc_integration_test.go` (OIDC test provider, `storagetest.Open`), `backend/internal/auth/governance_integration_test.go` (fixtures for orgs, users, teams, grants). Requirement text: `grep -n "GOV-004" docs/requirements.md`; ADR `docs/adr/0074-*.md` (decision section).

Write `*_integration_test.go` / `*_test.go` in `backend/internal/auth` (names `_GOV_004`) covering:
1. Provider CRUD per installation and per organisation, permission `SSOManage` required, secrets never returned, audit entries written.
2. Group mapping: a sign-in with groups gets exactly the mapped scoped grants and team memberships; a changed mapping changes them on next sign-in; unmapped groups grant nothing.
3. JIT provisioning off (unknown person refused) and on (person created with mapping grants only).
4. `acr`/`amr` and `sid` recorded on a federated sign-in.
5. Back-channel logout: a valid logout token ends the matching sessions; replayed `jti`, a token with `nonce`, a wrong `events` claim, a wrong audience and an expired token are refused.
6. The core `Directory` implementation used by SCIM: create, update, deactivate (revokes the member's tokens and sessions at once — check by authenticating with them afterwards), group ↔ team sync, SCIM token hash-only storage, rotation overlap, revoked token refused.
7. `auth.studio.allowPasswordLogin=false` refused while no provider exists.
Done when: `cd backend && go test -count=1 -race ./internal/auth/... ./internal/api/...` passes.

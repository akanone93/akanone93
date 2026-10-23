<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E1e-b — restriction API, effective permissions, explain, cache (`GOV-006`, `GOV-007`)

E1e-a is merged: restrictions exist in `backend/internal/auth/restrictions.go` and `restrictions_service.go` (owner-only rules in `ownerOnly`), the resolver subtracts them and records which restriction removed what, `organizations.permissions_version` is bumped on every change. Read `docs/adr/0086-restriction-policies-and-effective-permissions.md` (sections Effective permissions, Explain, Caching) and `grep -n "GOV-007" docs/requirements.md`. Implement exactly that.

Deliver:
1. **Proto** (`proto/plux/v1/governance.proto`, additive; `make gen`): `CreateRestriction`, `ListRestrictions`, `DeleteRestriction`, `GetEffectivePermissions` (org, optional app, optional environment → permission names + version), `WatchPermissions` (server stream of `{organization_id, version}`), `ExplainAccess` (person, permission, scope, optional proposed grant or restriction → decision, allowing grant IDs, removing restriction IDs, owner-floor flag). Thin handlers in `backend/internal/api` (copy the pattern of the grant handlers), wiring, matrix entries; self-explain needs no permission, others need `members.manage`.
2. **Explain/simulate** in `backend/internal/auth`: run the same resolver on the stored state, or on it plus the proposal in memory; never write.
3. **Cache:** per replica, key (person, org, app, env, version), lifetime `auth.permissionCacheSeconds`; invalidation by a Valkey channel carrying the org's new version (client in `backend/internal/server/build.go`); subscription down → bypass the cache. Change notices for `WatchPermissions` come from the same channel. Every goroutine owned and stopped on shutdown.
4. **Rate limits** for `ExplainAccess` through the existing limiter (`backend/internal/api/authn.go` `RateLimiter`), key from the registry.
5. **Listings honour app restrictions:** paths that read an organisation-wide `Holds` as "every app" (`AllowedApps` in `backend/internal/auth`, search in `backend/internal/search`; grep `AllowedApps`) must drop an app on which a restriction (`Principal.Restrictions`) removes the permission the listing needs. Test it.
6. **CLI:** `plux restrictions create|list|delete`, `plux access explain`; completion entries.
Tests (`_GOV_007`, `_GOV_006`): effective set per scope, explain names the right grant/restriction, simulate saves nothing, others' explain needs `members.manage`, cache invalidated by a change on another replica (two caches, one Valkey via `redis-server`), bypass when Valkey is down, stream delivers a version after a change, rate limit hits.
Done when: `cd backend && go build ./... && GOWORK=off go build ./... && go test -count=1 ./internal/auth/... ./internal/api/... ./cmd/plux/...` passes.

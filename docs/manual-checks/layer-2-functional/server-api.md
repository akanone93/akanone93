<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 2 — Server and API

You talk to the Plux Server directly: health, TLS, calls with `curl`, sign-in, organisations, apps,
plugins, drafts and locks, the publish pipeline, errors and the OpenAPI description. This shows you
what Studio and the CLI do underneath. Reference: [API](../../reference/api.md), [server](../../reference/server.md).

Every call is a `POST` to `https://localhost:8080/plux.v1.<Service>/<Method>` with a JSON body
(ConnectRPC). Field names are camelCase.

## Resources this file adds

| ID | What | Made by |
|---|---|---|
| R-API | A shell with the variables of L2-SRV-002 set | L2-SRV-002 |
| R-ORG2 | Organisation `globex` | L2-SRV-008 |
| R-PAT | A personal access token `check-token` made in Studio | L2-SRV-006 |

### L2-SRV-001 — Health, readiness, metrics

- **Needs:** R-STACK
- **Do:**
  1. `cd deploy/compose; CA=$PWD/.secrets/ca/ca.crt`
  2. `curl --cacert $CA https://localhost:8080/livez`
  3. `curl --cacert $CA https://localhost:8080/readyz`
  4. `curl --cacert $CA https://localhost:8080/metrics | head -30`
- **Expect:** `livez` and `readyz` answer 200; `metrics` lists Prometheus series such as `plux_rpc_requests_total`.
- **Why:** orchestrators and dashboards need liveness, readiness and metrics (SRV-007, OBS-002).
- **Look at:** `docker compose -f deploy/compose/compose.yaml stop postgres`, then `readyz` answers 503 while `livez` stays 200; start it again.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-002 — First call with a token: create R-API

- **Needs:** R-STACK, R-TOKEN
- **Do:**
  1. `. deploy/compose/.secrets/dev.env; CA=$PWD/deploy/compose/.secrets/ca/ca.crt`
  2. `call(){ curl -sS --cacert $CA -H "Authorization: Bearer $PLUX_DEV_TOKEN" -H "X-Plux-Organization: $PLUX_DEV_ORGANIZATION" -H 'Content-Type: application/json' -H 'Connect-Protocol-Version: 1' -d "$2" https://localhost:8080/plux.v1.$1; }`
  3. `call IdentityService/GetCurrentUser '{}'`
- **Expect:** JSON with the user `dev@plux.localhost` and its memberships. Keep the shell open: it is R-API.
- **Why:** one token works for the CLI and for plain HTTP (SRV-002, CLI-002).
- **Look at:** the server log line for the call (procedure, status, request id); `plux_rpc_requests_total` rises in `/metrics`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-003 — TLS 1.3 and security headers

- **Needs:** R-API
- **Do:**
  1. `curl -sSI --cacert $CA https://localhost:8080/livez`
  2. `curl -sS --cacert $CA --tls-max 1.2 https://localhost:8080/livez`
- **Expect:** the first shows `strict-transport-security`; the second fails the handshake (TLS 1.3 only).
- **Why:** secure defaults, no weak protocol (SEC-040).
- **Look at:** the `server.tls` and `server.hsts` keys in `deploy/compose/plux-server.yaml`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-004 — No credential, a bad credential

- **Needs:** R-API
- **Do:**
  1. `curl -sS --cacert $CA -H 'Content-Type: application/json' -d '{}' https://localhost:8080/plux.v1.IdentityService/GetCurrentUser`
  2. Repeat with `-H 'Authorization: Bearer plux_pat_nonsense'`.
- **Expect:** both fail with an `unauthenticated` Connect error and a `PLX-` code; the message never says which part was wrong.
- **Why:** deny by default, no oracle for attackers (SEC-102, SRV-004).
- **Look at:** the code in [the error reference](../../reference/errors.md).
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-005 — Lists, paging and field masks

- **Needs:** R-API, R-APP
- **Do:**
  1. `call OrgService/ListOrganizations '{}'` and note the organisation id.
  2. `call AppService/ListApps '{"organizationId":"'$PLUX_DEV_ORGANIZATION'","page":{"pageSize":1}}'`
  3. Repeat with the `pageToken` the answer returned.
- **Expect:** one app per page; the token continues after the last item; the apps `demo`, `starter`, `shop` appear across pages.
- **Why:** no list is unbounded; tokens are opaque and integrity-protected (SRV-004).
- **Look at:** change one character of the token: `invalid_argument`, `PLX-1032`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-006 — Create a personal access token in Studio, use it: R-PAT

- **Needs:** R-STUDIO, R-API
- **Do:**
  1. In Studio, **Administration**, **Tokens** (`/admin/tokens`): create `check-token`; copy it once.
  2. `PAT=plux_pat_…; curl -sS --cacert $CA -H "Authorization: Bearer $PAT" -H 'Content-Type: application/json' -d '{}' https://localhost:8080/plux.v1.IdentityService/GetCurrentUser`
  3. Revoke the token in Studio and repeat step 2.
- **Expect:** works until revoked, then `unauthenticated`. The secret is shown only at creation.
- **Why:** CI needs scoped, revocable credentials (SRV-064, SEC-101).
- **Look at:** **Audit log**: token created and revoked.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-007 — Organisation scoping hides what is not yours

- **Needs:** R-API
- **Do:** call `AppService/ListApps` with `organizationId` set to a random UUID, then with `-H 'X-Plux-Organization: <random>'` on a normal call.
- **Expect:** `not_found`, not `permission_denied`.
- **Why:** an organisation you are not in must not be disclosed (GOV-001, SEC-102).
- **Look at:** the same answer for a real foreign id and a made-up one.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-008 — Create an organisation: R-ORG2

- **Needs:** R-API (installation administrator)
- **Do:** `call OrgService/CreateOrganization '{"key":"globex","name":"Globex"}'`; then `call OrgService/ListOrganizations '{}'`.
- **Expect:** `globex` is created and listed; the same key again fails with `already_exists`.
- **Why:** organisations are the top isolation boundary (GOV-001).
- **Look at:** Studio's organisation menu now lists `globex`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-009 — Create an app and a plugin by API

- **Needs:** R-API, R-ORG2
- **Do:**
  1. `call AppService/CreateApp '{"organizationId":"<globex id>","key":"api-app","name":"API app"}'`
  2. `call PluginService/CreatePlugin '{"appId":"<id>","key":"main","name":"Main"}'` (field names as in `proto/plux/v1/plugin.proto`).
  3. `call PluginService/ListPlugins '{"appId":"<id>"}'`
- **Expect:** app and plugin exist; the plugin list holds `main`.
- **Why:** everything Studio does is an API call (SRV-002, SRV-040).
- **Look at:** an `Idempotency-Key` header on step 1, repeated: `Idempotent-Replayed: true`, no second app.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-010 — Idempotency keys

- **Needs:** R-API
- **Do:** run `CreateApp` twice with `-H 'Idempotency-Key: k-001'` and the same body; then once more with another body.
- **Expect:** the second answer equals the first with `Idempotent-Replayed: true`; the third is refused (`already_exists`, `PLX-1031`).
- **Why:** retries must not act twice (SRV-005).
- **Look at:** `ListApps`: one app only.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-011 — Drafts: read a document

- **Needs:** R-API, R-APP, R-PLUGIN
- **Do:** `call DocumentService/ListDocuments '{"appId":"<shop id>","pluginId":"<catalog id>"}'`; then `GetDocument` for a page.
- **Expect:** the documents you made in Studio, as canonical JSON bytes with a revision.
- **Why:** documents are the contract between Studio, CLI and compiler (SRV-030, SCH-003).
- **Look at:** the `revision` field changes after any Studio edit.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-012 — Locks: a second writer is refused

- **Needs:** R-API, R-PLUGIN, R-USER2
- **Do:** with R-ADMIN's token call `PluginService/AcquireLock` for `catalog`; with R-USER2's token (made like L2-SRV-006) call it again; then `PluginService/GetLock`.
- **Expect:** the second call fails and `GetLock` names the holder; `ReleaseLock` frees it.
- **Why:** one writer per plugin (SRV-040, SRV-041).
- **Look at:** the same lock shows as the holder in Studio.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-013 — Writes need the lock and the revision

- **Needs:** L2-SRV-012
- **Do:** try `DocumentService/PatchDocument` without holding the lock; then with the lock but a stale `ifRevision`.
- **Expect:** the first is refused for the lock, the second as a conflict.
- **Why:** no silent overwrite (SRV-030, SRV-040).
- **Look at:** the error code in each answer.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-014 — Validate a draft

- **Needs:** R-API, R-PLUGIN
- **Do:** `call DocumentService/ValidateDraft '{"appId":"<shop id>"}'`.
- **Expect:** diagnostics (possibly none) with codes and JSON paths; the same ones Studio's **Problems** shows.
- **Why:** one validator everywhere (SCH-040, CMP-040).
- **Look at:** a deliberate break in Studio appears here.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-015 — Publish pipeline by API

- **Needs:** R-API, R-RELEASE1
- **Do:** `call PublishService/ListPublishJobs '{"appId":"<shop id>"}'`; `GetPublishJob` for the last job.
- **Expect:** jobs with state `succeeded`, a version number and no diagnostics.
- **Why:** publish is validate, compile, sign, store, observable as a job (SRV-050, SRV-051).
- **Look at:** `plux_publish_jobs_total` and `plux_publish_duration_seconds` in `/metrics`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-016 — Releases and manifest

- **Needs:** R-API, R-RELEASE1
- **Do:** `call ReleaseService/ListReleases '{"appId":"<shop id>","environmentId":"<development id>"}'`.
- **Expect:** releases newest first with sequence numbers; a rollback later adds a higher sequence, never reuses one.
- **Why:** releases are immutable (REL-001, REL-006).
- **Look at:** `plux_manifest_requests_total` after a device syncs.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-017 — Rate limit

- **Needs:** R-API
- **Do:** `for i in $(seq 1 300); do call IdentityService/GetCurrentUser '{}' >/dev/null; done; call IdentityService/GetCurrentUser '{}' -i`
- **Expect:** after the allowance the call fails with `resource_exhausted` and a `Retry-After` header; it works again after the wait.
- **Why:** protection against abuse (SRV-065).
- **Look at:** the limit keys `api.requestsPerMinute*` in `deploy/compose/plux-server.yaml` (if lowered).
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-018 — Oversized request

- **Needs:** R-API
- **Do:** send a body larger than `api.requestSize` (default 4 MiB): `head -c 5000000 /dev/zero | tr '\0' a > /tmp/big.txt`, post `{"name":"…"}` with the file's content.
- **Expect:** refused before a handler runs, with a size error.
- **Why:** bounded input (SEC-104, LIM-001).
- **Look at:** the limit comes from the registry, not the code.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-019 — Error model

- **Needs:** R-API
- **Do:** call `AppService/CreateApp` with an empty `key`.
- **Expect:** `invalid_argument` with a `PLX-` code, a reason, the field and a documentation link.
- **Why:** every error is stable and documented.
- **Look at:** the code exists in `docs/reference/errors.md`.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-020 — OpenAPI description

- **Needs:** repository checkout
- **Do:** open `docs/reference/api/openapi.yaml`; find `/plux.v1.AppService/CreateApp`; run `make proto` only if you changed the contract.
- **Expect:** the operation matches what you called; every service of the table in `docs/reference/api.md` is present.
- **Why:** integrators use plain HTTP and JSON (SRV-002).
- **Look at:** `git status` is clean after `make gen` (generated code is committed).
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-021 — Device endpoints refuse a bearer token

- **Needs:** R-API
- **Do:** call `TokenService/RefreshDeviceToken` with `{}` and the bearer token.
- **Expect:** refused: device calls need `Authorization: DPoP` with a proof.
- **Why:** device and human credentials are separate (SEC-020, SEC-021).
- **Look at:** `plux_dpop_rejections_total` rises.
- **Result:** ☐ pass ☐ fail — notes:

### L2-SRV-022 — Audit trail of this file

- **Needs:** R-STUDIO, the previous checks
- **Do:** in Studio open **Administration**, **Audit log**; filter by action for the app and organisation creations.
- **Expect:** an entry per mutation above with actor, resource and time.
- **Why:** every state change is audited (SEC-140).
- **Look at:** the actor of the token calls is the token's user.
- **Result:** ☐ pass ☐ fail — notes:

# Server API Reference

The Plux Server exposes every capability through one Protocol Buffers
contract in [`proto/plux/v1/`](../../proto), served with ConnectRPC over
gRPC, gRPC-Web and Connect, on HTTP/1.1 and HTTP/2 (`SRV-002`,
[ADR-0005](../adr/0005-connectrpc-and-protobuf.md)). The generated OpenAPI
3.1 description of the same surface is [api/openapi.yaml](api/openapi.yaml),
for integrators who call it as plain HTTP/JSON.

## 1. Services

| Service | Responsibility | Requirements |
|---|---|---|
| `OrgService` | Organisations, teams, memberships, organisation limits | `GOV-001`, `LIM-002` |
| `IdentityService` | Sign-in (built-in and OIDC), second factors (TOTP, WebAuthn), sessions, personal access tokens, CI federation, the device grant, audit | `SEC-100`, `SEC-101`, `SRV-064`, `CLI-002`, `SEC-140` |
| `AppService` | Apps, environments, variables, secrets, channels, trash, app limits | `GOV-010`, `GOV-031`, `SEC-106` |
| `PluginService` | Plugins and editing locks | `SRV-040`–`SRV-042` |
| `DocumentService` | Draft documents, JSON Patch writes, snapshots, validation, export and import | `SRV-030`, `SRV-031`, `SCH-042` |
| `ComponentService` | Components and where each is used | `SCH-041` |
| `TemplateService` | Templates and instantiation with fresh identifiers | `SCH-031` |
| `AssetService` | Uploads, sniffing, variants | `SRV-060`, `AST-003` |
| `PublishService` | Publish jobs with streamed progress | `SRV-050`, `SRV-051` |
| `ReleaseService` | Plugin versions, app releases, promotion, rollback, changelog, compatibility by runtime and host build | `REL-001`–`REL-007`, `REL-080`, `REL-081` |
| `ManifestService` | Signed manifest, sync plan, public keys | `REL-020`–`REL-024`, `REL-030`–`REL-033` |
| `DeviceService` | Registration challenges, attested registration, re-attestation, revocation, SCA key enrolment, installed releases | `SEC-001`–`SEC-006`, `SEC-027`, `REL-080` |
| `TokenService` | DPoP-bound device access tokens, their refresh, and the exchange of an end user's token for an upstream's | `SEC-020`, `SEC-025`, `SEC-026` |
| `GatewayService` | `Call`: a request to a configured upstream made through the server, which holds the upstream's credentials | `SEC-030`, `SEC-031` |
| `ScaService` | `CreateTransactionChallenge`: the single-use challenge a user's confirmation of a transaction is signed over | `SEC-027`, `SEC-028`, `SEC-032` |
| `SecurityAdminService` | An environment's security configuration, upstreams, the user identity provider, and the signed audit checkpoints | `SEC-180`–`SEC-182`, `SEC-026`, `SEC-031`, `SEC-141` |
| `TelemetryService` | Runtime event ingestion and listing | Appendix G.2, `SCH-012` |
| `ControlService` | Kill switches and the mandatory-update flag | `REL-030` |
| `NativeCatalogueService` | Host builds' native catalogues, immutable once uploaded; publishing checks plugins against them, and a build that cannot run a release gets a manifest of the newest release it can | `CLI-006`, `WGT-032`, `REL-080` |

Besides the procedures, the api role serves the HTTP form of the gateway,
`/gateway/v1/{upstream}/{path}` ([Gateway](#gateway)).

Services listed in `SRV-003` for later phases — rollouts, experiments,
functions, localisation, approvals, dev sessions, AI, payments, admin —
arrive with their phases.

## 2. Calling conventions

**Organisation.** A call acts in one organisation: the one its token is
bound to, or, for a session, the one named by the `X-Plux-Organization`
header or the request's `organization_id`, which must agree when both are
given. An organisation the caller has no part in is reported as
`not_found`, so its existence is not disclosed (`GOV-001`, `SEC-102`).

**Paging.** Every list call takes `Page` (`page_size`, `page_token`,
`filter`, `order_by`) and returns `PageResult`. An unset or oversized
`page_size` is the registry limit `api.pageSize`. Page tokens are opaque and
integrity-protected: each is bound to the procedure, the caller's scope, the
filter and the ordering it was issued for, authenticated with an HMAC under
an installation key, valid for a day, and rejected elsewhere
(`invalid_argument`, `PLX-1032`). A filter is a conjunction of equalities
on the fields each list documents, `key = "shop" AND production = "true"`;
each list is served in one ordering, which `order_by` may name. A filtered
page may hold fewer items than asked for; the token continues after the
last item read. No endpoint returns an unbounded list (`SRV-004`).

**Idempotency.** Mutating calls accept an `Idempotency-Key` request header
(1 to 128 printable ASCII characters). A repeat with the same key within 24
hours returns the original result, marked `Idempotent-Replayed: true`,
rather than acting twice; the same key with a different request, or while
the first call is still running, is refused (`already_exists`, `PLX-1031`).
A failed call releases its key. Keys belong to the caller — a user, a
token — and results are stored encrypted, because some carry a secret
returned only once. Calls made without a credential (signing in, polling a
device grant) are never replayed: their results are credentials
(`SRV-005`).

**Field masks.** Every list call's `Page` carries a `read_mask`
(`google.protobuf.FieldMask`) that limits each returned item to the named
fields; an empty mask returns whole items (`SRV-004`).

**Rate limits.** Every call, unary or streaming, counts against its client
address, and an authenticated call also against its principal (a user,
token or device). The allowances are registry limits
(`api.requestsPerMinutePerAddress`, `api.requestsPerMinute`,
`api.requestsPerMinutePerDevice`); a refused call returns
`resource_exhausted` with `Retry-After` (`SRV-065`).

**Documents as bytes.** Document content is carried as canonical JSON bytes
(`SCH-003`), never as protobuf structures, so a document's hash is a property
of what was stored and extension properties survive a round trip (`SCH-004`).

**Concurrency.** Document writes carry `if_revision`; a mismatch is a
conflict the client resolves by re-reading (`SRV-030`). Writing to a draft
also requires holding the editing lock (`SRV-040`).

**Streaming.** `PublishService.WatchPublish` and `DocumentService.ExportDraft`
are server-streaming; `DocumentService.ImportDraft` and
`AssetService.UploadAsset` are client-streaming, with the first message
carrying the target and no payload.

**Publishing and releases** ([ADR-0020](../adr/0020-build-once-promote-releases.md)).
`Publish` freezes a draft at a revision and returns a `queued` job; the
worker compiles, checks, signs and stores it, and `WatchPublish` streams
its stage, percentage and new diagnostics until it ends `succeeded` with a
version number, `failed` with diagnostics, or `cancelled`. An error, or a
warning not acknowledged with `acknowledge_warnings`, fails the job with
nothing recorded (`PLX-8051`). An empty `plugin_id` publishes the app
bundle; `CreateRelease` takes it under the key `""`. `CreateRelease`
answers with diagnostics and no release when a plugin has no version
(`PLX-8052`), a reference does not resolve, or a version was compiled
against other sources (`PLX-8050`). Publishing, creating a release,
promoting and rolling back need a session that presented a second factor.
Limit listings report measured usage in `LimitUsage.value` where P2
measures it: plugins per app, the newest release's size, pages per plugin
and the newest bundle's size (`LIM-005`).

**Assets** ([ADR-0027](../adr/0027-asset-pipeline.md)). `UploadAsset` is
client-streaming: the first message names the app, the file under
`assets/` and the editing session, and every message carries a chunk. The
file is typed from its bytes, packaged (Lottie becomes dotLottie), stripped
of metadata, scanned when a scanner is configured, and listed in the
app-level draft's `assets/index.json` under the app's lock; an upload under
an existing name replaces the file and keeps its asset ID. Assets belong to
the app, so `plugin_id` must be empty. A raster image comes back with
`processing` `pending`; the worker then adds its WebP and AVIF variants and
sets `ready`, or `failed` with diagnostics. Links in `Artifact.urls` are
signed or CDN URLs valid for fifteen minutes. Refusals: `invalid_argument`
for an unknown or malformed file (`PLX-1005`) or one the scanner rejects
(`PLX-6031`), `resource_exhausted` above `asset.fileSize` or
`asset.imagePixels` (`PLX-1320`).

**Drafts** ([ADR-0015](../adr/0015-single-draft-with-snapshots-and-locks.md)).
Every plugin has one draft, and the app has one for its own documents
(`app.json`, theme, translations, native catalogue, components, templates),
each with its own editing lock (`SRV-042`). A write names the editing
`session` holding the lock — 1 to 128 printable ASCII characters chosen by
the client, one per editor tab — and the `if_revision` it read: `0`
creates, and a stale revision is `aborted` with the current one in the
error's `revision` detail (`SRV-030`). A document with a structural error
is refused whole with `invalid_argument` and its file and JSON Pointer as
details; compiler findings about a structurally valid document come back
as diagnostics. A write refused because the lock is held elsewhere
carries `holder` and `expiresAt`; when the writer is the session a
takeover displaced, its write is kept as an unapplied snapshot named in
`preservedSnapshot`, which the new holder can compare and restore
(`SRV-041`). Snapshot pages run newest first. `ImportDraft` for the whole
app creates the plugins its `plugins/<key>/` directories name, takes each
draft's lock for the session, and deletes documents the import does not
contain; assets are not documents and are uploaded with `AssetService`.

## 3. Errors

A failure returns a Connect code, `google.rpc.ErrorInfo` with the reason and
the domain `plux.dev`, and the Plux error code from Appendix F. Domain code
never constructs transport errors: one interceptor translates registered
reasons at the edge (`SRV-006`, [ADR-0018](../adr/0018-unified-error-model.md)).

| Connect code | Used for |
|---|---|
| `invalid_argument` | malformed input, failed structural validation |
| `failed_precondition` | the editing lock is held elsewhere (`PLX-8020`), a publish with errors, a state that forbids the call (`PLX-8033`) |
| `permission_denied` | authorisation refused (`PLX-8030`), a second factor needed (`PLX-8011`) |
| `not_found` | the resource does not exist, or the caller may not see it (`PLX-8031`) |
| `already_exists` | a key already taken (`PLX-8032`), an idempotency key reused (`PLX-1031`) |
| `aborted` | revision conflict on a document write |
| `resource_exhausted` | a rate limit or a limits-registry limit (`SRV-065`, `LIM-003`) |
| `unauthenticated` | no or invalid credential (`PLX-8012`) |
| `internal` | an unexpected failure; the response carries an incident ID only |

Findings about documents are **diagnostics**, not errors: a call that
validates returns `repeated Diagnostic` and succeeds, even when the
diagnostics are errors, so every problem is reported at once.

## 4. Authentication

| Caller | Credential |
|---|---|
| Studio | the `__Host-plux_session` cookie (`HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/`) set by `StartPasswordLogin` or `CompleteMfa`, plus the session's CSRF token in `X-CSRF-Token` on **every** call made with it (`SEC-101`) |
| CLI | `Authorization: Bearer plux_pat_…`: a personal access token, or one obtained by `plux login` through the OAuth 2.0 device authorization grant (`CLI-002`) |
| CI | a bearer token exchanged by `ExchangeWorkloadIdentity` from the provider's OpenID Connect identity token, with no long-lived secret (`SRV-064`) |
| Device | `Authorization: DPoP <access token>` and a `DPoP` proof header: a short-lived RFC 9068 JWT bound to the device's hardware key, obtained by attested registration and `RefreshDeviceToken` ([Device authentication](#device-authentication)). The P2 `plux_dat_…` bearer tokens are gone; `RegisterDevice` and `IssueDeviceToken` answer `PLX-6008` |

A request carrying both a cookie and a bearer token is refused. The procedures that take no credential authenticate by what they carry:
`AcceptInvitation`, `StartPasswordLogin`, `CompleteMfa`,
`StartDeviceAuthorization`, `PollDeviceAuthorization`,
`ExchangeWorkloadIdentity`, `CreateRegistrationChallenge` and
`RegisterAttestedDevice` (the last two carry attestation evidence bound to
a challenge). `RegisterDevice` and `IssueDeviceToken` are declared but
refused with `PLX-6008`, so the contract only grows.

`SecurityAdminService` is for administrators: it takes the people's
credentials above, never a device token.

### Device authentication

A device proves possession of a hardware-bound key on every call (`SEC-001`,
`SEC-020`, `SEC-021`;
[ADR-0012](../adr/0012-dpop-hardware-keys-and-attestation.md)).

1. **Registration challenge.** `CreateRegistrationChallenge(app_id,
   environment)` returns a random, single-use challenge, valid for the
   setting `registrationChallengeTtl` (at most five minutes) and bound to
   the app and environment (`SEC-005`). It needs no credential.
2. **Attested registration.** `RegisterAttestedDevice` carries the
   challenge, the device's DPoP public key as a JWK, where it claims the
   key lives (`key_storage`), and `evidence`: an Android Key Attestation
   chain with a Play Integrity token, an iOS App Attest attestation, or,
   in non-production environments that allow it, development evidence. The
   server verifies the evidence against the challenge and the thumbprints
   of every key registered with it (`SEC-002`–`SEC-004`), computes the
   assurance level `AL0` to `AL3` (`SEC-007`) and stores the key's
   thumbprint. It returns the `Device`; the level is the server's, the
   device never claims one. A software key is refused (`PLX-6003`) unless
   the environment allows it, and a verification that cannot be made
   because the attestation provider is unavailable follows
   `attestationOutageGrace` (`PLX-6009`, `SEC-009`).
3. **Access token.** `RefreshDeviceToken` issues the token and is also how
   the first one is obtained. The DPoP proof alone authenticates it, and the
   proof's key must be the named device's (`PLX-6013`). An iOS device adds
   an App Attest assertion over the proof, an Android device a Play
   Integrity token where `androidRefreshRequiresIntegrity` is set
   (`SEC-025`). The response carries `access_token`, `expires_at`,
   `token_type` (`DPoP`) and `assurance_level`. There is no refresh token. A
   device whose attestation is older than `reattestationInterval` gets
   `PLX-6007` and must re-attest first.
4. **Re-attestation and revocation.** `ReattestDevice` takes a fresh
   challenge and new evidence for the same DPoP key and recomputes the
   level (`SEC-006`); like the refresh it is authenticated by the proof
   alone. `EnrolScaKey` stores the key behind strong customer
   authentication once its attestation is verified (`SEC-027`).
   `RevokeDevice`, for administrators, withdraws a device: its tokens stop
   working on every replica and it cannot refresh (`PLX-6006`).

The **access token** is an RFC 9068 JWT (`typ: at+jwt`, ES256, signed by a
key of the environment's class) with the claims `iss`, `aud` (both the
server's public base URL), `sub` (the device), `client_id` (the app), `env`,
`org`, `hb` (the host build), `al` (the assurance level), `cnf.jkt` (the
key's thumbprint, RFC 7800), `iat`, `exp` and `jti`. Its lifetime is the
setting `accessTokenLifetime`, never more than 15 minutes (`SEC-020`).

A **DPoP proof** (RFC 9449) is an ES256 JWT with `typ: dpop+jwt` and the
public key in its header. Its claims are `htm` (the HTTP method; `POST` for
every Connect call), `htu` (the public base URL and the procedure's path),
`iat`, a unique `jti`, the server's current `nonce`, and, on a call that
presents an access token, `ath`, the token's hash (`SEC-021`, `SEC-022`). A
proof is accepted once (`PLX-6011`) and only within the window
`dpopIatWindow`, narrowed to `dpopIatWindowFallback` while the replay cache
is degraded; an environment whose `replayCacheFallback` is `failClosed`
refuses instead (`PLX-6015`, `SEC-023`).

**Nonces.** Every response to a device call, successful or not, carries the
server's current `DPoP-Nonce`. A proof without it, or with a stale one, is
refused with `PLX-6012`, HTTP 401 and `WWW-Authenticate: DPoP
error="use_dpop_nonce"`; the device retries once with the nonce of the
response (`SEC-024`). Other refusals name their cause in the same header:
`invalid_dpop_proof` (`PLX-6010`, `PLX-6011`, `PLX-6013`) and
`invalid_token` (`PLX-6014`, `PLX-6006`).

A device token opens these procedures and no others:

| Procedure | Needs |
|---|---|
| `ManifestService.GetManifest`, `ManifestService.GetRootKeys`, `DeviceService.ReportInstalled`, `TelemetryService.IngestEvents` | token and proof |
| `GatewayService.Call`, `ScaService.CreateTransactionChallenge`, `TokenService.ExchangeUserToken`, `DeviceService.EnrolScaKey` | token and proof |
| `TokenService.RefreshDeviceToken`, `DeviceService.ReattestDevice` | proof alone |

`GetRootKeys` is also open to people and CI for an environment of an app
they can read (`plux pull`). Every other procedure refuses a device token. A
device's calls count against its own allowance,
`api.requestsPerMinutePerDevice` (`SRV-065`).

### Gateway

`GatewayService.Call` forwards one request to an upstream registered with
`SecurityAdminService.CreateUpstream`: it names the upstream, an optional
operation, the method, the path, the headers the upstream may receive and
the body. The server applies the environment's security configuration and
adds the upstream's credentials, which the device never sees (`SEC-030`,
`SEC-031`). An operation that needs a confirmation takes
`transaction_challenge_id` and `transaction_signature` (ES256, 64 bytes) from
`ScaService.CreateTransactionChallenge` and the device's SCA key
(`SEC-032`). An end user's token travels in the `Plux-User-Token` header;
`TokenService.ExchangeUserToken` trades it, following RFC 8693, for a token
scoped to one audience (`SEC-026`): an upstream registered for the environment,
or `function:<name>` for a server function that has a version in it, which
receives the verified claims (`FN-002`). A token for one audience is refused at
every other.

A body that must stream, a WebSocket or server-sent events use the HTTP
route `/gateway/v1/{upstream}/{path}` instead: the method the device wants,
the same `Authorization: DPoP` token and `DPoP` proof (with the request's own
method and path as `htm` and `htu`), and the other typed fields as the
headers `Plux-Operation`, `Plux-Plugin`, `Plux-User-Token`,
`Plux-Challenge-Id` and `Plux-Transaction-Signature`. Both bodies are
streamed; only a transaction's is read, up to `gateway.requestBytes`.

### Security administration

`SecurityAdminService` sets and reads an environment's security
configuration (`SetSecurityConfig`, which takes `expected_version`, and
`GetEffectiveSecurityConfig`), registers the upstreams of its gateway
(`CreateUpstream`, `GetUpstream`, `ListUpstreams`, `UpdateUpstream`,
`DeleteUpstream`), sets the identity provider whose user tokens may be
exchanged (`SetUserIdentityProvider`, `GetUserIdentityProvider`,
`DeleteUserIdentityProvider`) and lists the signed checkpoints over the
audit log (`ListAuditCheckpoints`, `SEC-141`). A setting may only tighten
the profile's preset (`PLX-6041`, `PLX-6042`).

### Manifest and sync plan

`GetManifest` answers with the newest manifest the worker signed for the
device's own app, environment and the requested channel (default
`production`). `signed` holds the RFC 8785 canonical JSON the signatures
cover: type, spec version, role (`targets`), app, environment, channel,
release sequence, issue and expiry times, the app bundle and every
plugin's key, version, bundle hash, size, required features and minimum
runtime, the control switches, and experiment assignments (empty until
P9) (`REL-030`). The typed fields repeat it for convenience; a device
verifies `signed`, never a re-encoding.

The request lists the bundles the device holds; each bundle's `sync` says
`keep`, `delta` (with the hash it applies to, its URL and size) or `full`
(`REL-032`). A delta is offered only when it is at most 60 % of the
compressed bundle (`REL-023`). The plan and the download URLs are **not**
part of `signed`: they depend on the device and on where objects are
hosted, and they need no signature, because the device checks what it
rebuilds or downloads against the signed hashes. The response carries an
`ETag` over the manifest and the installed bundles; sending it back as
`if_none_match` returns `not_modified` and nothing else (`REL-031`).

Authorisation is deny-by-default and evaluated for every call against RBAC
permissions and resource scope, with PostgreSQL row-level security as a
second barrier (`SEC-102`). The roles of P2 are owner, admin, developer and
viewer; a team's members hold the roles the team is granted on single apps.
Publishing, promoting, managing keys, members, secrets and tokens need a
session in which a second factor was presented (`SEC-100`); tokens carry
scopes fixed when they were minted, never more than their creator held
([ADR-0026](../adr/0026-identity-tenancy-and-access.md)).

## 5. Artifacts

Bundles, deltas and assets are **not** returned through this API. They are
content-addressed objects served over plain HTTP from object storage, a CDN
or the server itself, with `Cache-Control: public, max-age=31536000,
immutable` and range support (`SRV-023`, `REL-024`, `DEP-041`). The API
returns their hashes, sizes and URLs; the manifest signs the hashes.
Without a CDN the api role serves bundles and deltas itself at
`GET /v1/objects/<kind>/<xx>/<sha256>`.

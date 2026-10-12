# Key Compromise Runbook

A key may be exposed: a leaked token key, a stolen online metadata key, a suspected root
key, a read of the secrets store. This runbook says which key is which, what an attacker
gains, and how to rotate. The offline root has its own procedure in the
[key ceremony runbook](key-ceremony.md); follow it for root keys and for any new `root`.

Treat every suspected compromise as an incident: record the time you learned of it, keep the
logs, and rotate first. Proving exposure comes second.

## Inventory

| Key | Held in | Signs or protects | If an attacker has it | Rotate with |
|---|---|---|---|---|
| **Token key** (ES256, one per class: `production`, `development`) | Signing backend (Vault Transit `plux-tokens-<class>`, or PKCS#11 labels); the `api` and `gateway` roles hold a sign-only capability | Access tokens (`at+jwt`), user tokens (`plux-user+jwt`), `Plux-Assertion` | Mint tokens for any device at any assurance level, bound to a key the attacker owns, and forge assertions that upstreams trust. Severe | §1 |
| **Online metadata keys** (Ed25519: `targets`, `snapshot`, `timestamp`, per environment) | Signing backend; the `worker` signs | Manifests, snapshot, timestamp | Sign a manifest and bundles hash list. Devices also check version monotonicity and anti-rollback, but a valid newer manifest would install | §2 |
| **Root keys** (2 of 3, offline) | Three holders' HSM tokens | The `root` metadata | One: nothing alone. Two: sign a root naming their own keys | [Key ceremony](key-ceremony.md) |
| **Audit key** | Signing backend, `worker` | Audit checkpoints | Forge a checkpoint over a rewritten chain | §3 |
| **Wrap key** (`signing.vault.wrapKey` / `signing.pkcs11.wrapKey`) | Signing backend | Data keys of every stored secret: upstream credentials, Play Integrity keys, confidential content keys | With database access, read all environment secrets | §4 |
| **Installation keys** (`dpop-nonce`, `page-token`) | PostgreSQL, sealed by the wrap key | DPoP nonce derivation, page tokens | Predict nonces (freshness only), forge page tokens (bound to scope) | §5 |
| **Confidential content key** | PostgreSQL, sealed; wrapped to devices | One release's sections | Read that release's bundles | §6 |
| **Device keys** | Device hardware | One device's proofs | Impersonate that device (needs the hardware) | [Device revocation](device-revocation.md) |
| **Upstream credentials, Play Integrity keys** | Environment secrets | Gateway to upstream; attestation | Call the upstream as the gateway; read Play Integrity verdicts | Rotate at the source, then `SetSecret` |
| **TLS keys** | Edge | Transport | Intercept traffic if pinning is absent; devices pin | [Pin rotation](pin-rotation.md) |

## First hour

1. Name the key and the window of exposure.
2. Stop the leak: rotate credentials the key could reach, remove the access path.
3. Decide containment by the table above; start with §1 for any token-key suspicion.
4. Run `plux-server audit verify -org <id>` to prove the audit trail is intact, and read the
   audit log for administrative changes in the window (`ListAuditEntries`).
5. Tell the maintainer and affected customers. A token-key or online-key compromise is
   reportable under your own incident policy.

## 1. Token key

Impact lasts as long as old verification keys are trusted. Access tokens live at most 15
minutes, so the cut-off is fast once the old key is gone.

1. **Replace the key.** With Vault Transit: `vault write -f transit/keys/plux-tokens-production/rotate`
   adds a version; new tokens use it at once and `TokenKeys` lists every version, oldest
   first. With PKCS#11 the backend holds one key per class under one label
   (`<prefix>-production`): replace the key object on the token with a new one. There is no
   overlap, so all tokens signed by the old key stop verifying, and devices simply refresh.
2. **Remove the compromised versions** so they stop verifying. In Vault, trim the old
   versions (`transit/keys/<name>/trim`; see Vault's documentation for its version
   constraints). Until you do, forged tokens still verify. PKCS#11 needs no extra step.
3. **Wait for replicas.** Each replica caches the key list for 30 seconds
   (`tokenKeysTTL`). After that, tokens under the removed key are refused and devices
   refresh with a new one. Expect a short rise in `RefreshDeviceToken` traffic. Devices keep their
   registrations; no re-registration is needed.
4. **Tell upstreams.** The key set at
   `/.well-known/plux/gateway-jwks/<class>` changes. Upstreams that cache it must drop the
   old key; ask customers to flush their JWKS cache. Any assertion minted with the old key
   is valid up to 60 seconds after it was issued.
5. **Hunt.** In the exposure window, look for devices acting at an assurance level that
   does not match their record, requests from unknown `jti`s, and gateway calls
   with `sca` claims you did not issue. `GetDevice` shows the registered level. Revoke any
   device that does not match ([device revocation](device-revocation.md)); the thumbprint
   flag is checked on every request, so even a forged token for it fails.
6. A *development*-class compromise affects non-production environments only: production
   refuses development-class tokens and the reverse (`PLX-6021`).

## 2. Online metadata keys

A new `root` names the replacement key; devices follow the chain of roots at sync.

1. Create a replacement key (the signing backend) and export the environment's public online
   keys: `plux-server metadata keys -config plux-server.yaml -organization <org> -environment <env>`.
2. Run the **rotation** of the [key ceremony](key-ceremony.md): a new root version with the
   new online key in place of the old, signed by two holders of the new keys and two of the
   previous keys, then `upload-root`.
3. Publish a fresh release (or promote the current one) so a manifest and a timestamp are
   signed with the new keys; the `worker` re-signs `snapshot` and `timestamp` before they
   expire.
4. Remove the old key from the backend after devices have had time to fetch the new root
   (a day is enough for devices that sync daily; devices offline keep their last good
   release and fetch the chain when they next sync).
5. Until step 4, an attacker who can also reach devices' network path could serve a manifest
   the old key signed. Pinned TLS and the anti-rollback rules (`SEC-055`) limit this; the
   quicker the new root reaches devices, the smaller the window.

## 3. Audit key

Rotate the audit signing key in the backend and keep the old public key available so older
checkpoints still verify (`audit.PublicKeys`). Run `plux-server audit verify` before and
after. Checkpoints signed by a compromised key during the window are not evidence; re-anchor
with a fresh checkpoint after the rotation and compare the chain with backups.

## 4. Wrap key

1. Rotate the key in the backend (Vault Transit keeps older versions so older secrets still
   unwrap).
2. **Re-seal and rotate what it protected**, because the old version can still read old data:
   set every upstream credential and every Play Integrity key again (`SetSecret`, new values
   obtained at the source), and treat confidential content keys as exposed (§6).
3. After re-sealing, remove the compromised key version.

## 5. Installation keys

`dpop-nonce` and `page-token` are random 32-byte secrets in the `installation_secrets` table,
sealed by the wrap key. No command rotates them. After rotating the wrap key, an operator
may delete the two rows in a maintenance window and restart the replicas; the first replica
to start creates new ones. In flight, nonces and page tokens become invalid: clients retry
with `use_dpop_nonce` and restart paging.

## 6. Confidential content keys

A content key belongs to one release. If it may be exposed, treat that release's content as
disclosed to whoever held the ciphertext. Publish a new release: it gets a new key, and devices
download the new sealed bundles in full (no deltas). Remove the old release from channels.

## Recovery and prevention

- Confirm: fresh token key version in use, old version gone, `TokenKeys` lists only intended
  versions, upstreams verify with the new set, `audit verify` passes.
- Keep root keys offline; use an HSM or PKCS#11 token for every online key in production
  (`signing.backend: pkcs11` or `vault`, never `file`; the file backend refuses production).
- Grant the signing capability to the `worker` and a sign-only token capability to
  `api`/`gateway`. A handler bug then cannot sign a release.
- Rehearse: `TestRootCeremonyOnSoftHSM_SEC_121` runs the root procedure against SoftHSM2; do
  the same with your HSM in staging.
- Rotate the token key on a schedule (yearly), so the procedure is routine.

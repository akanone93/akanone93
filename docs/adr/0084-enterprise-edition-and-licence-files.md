# 0084. Enterprise edition boundary and licence files

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `GOV-030`, `GOV-032`, `GOV-033`, `GOV-034`

## Context and problem

ADR-0022 fixed the licensing model: Apache-2.0 client side, AGPL-3.0 server and Studio, and a
commercial enterprise edition under `ee/`. P9 is the first phase that puts code there. Two things
are open: where the boundary runs in code and in the feature list, and how a licence is issued
and checked without weakening security or availability. A licence check that locks operators out,
phones home, or can be bypassed by editing a config file would each be wrong in a different way.

## Decision drivers

- Security capabilities are never gated by edition (`GOV-032`).
- The core builds, tests and runs without `ee/` (`GOV-033`).
- Offline verification only; air-gapped installs must work.
- The signing key never touches a server, CI or the repository.
- An expired licence never deletes data or blocks device traffic.

## Considered options

1. A separate Go module `ee/backend` plugging into public extension points of the core, with Ed25519-signed offline licence files.
2. Build tags in one module, with enterprise code beside core code.
3. Online licence validation against a Plux service.

## Decision

Chosen option: **1** (maintainer decisions Q6, Q7, Q8; plan decision D-L).

- **Boundary.** `ee/backend` is its own module importing the core's extension points:
  `identity.Provider` (SAML), `scim.Server`, `governance.Stages`, `analytics.Store`
  (ClickHouse), `export.Sink` (Parquet, CEF, OTLP logs, webhooks), `freeze.Calendar`,
  `compliance.Reporter`, `airgap.Bundler`. The core registers no-op or community implementations;
  `cmd/plux-server-ee` links both. CI builds and tests the core without `ee/` in a dedicated job,
  and the `ee` image separately. The edition split is the table in the ADR-0022 Revision of this
  date.
- **Format.** A JSON payload in the signed envelope used by update metadata (ADR-0004):
  `licenseId`, `organization`, `edition` (`enterprise`), `issuedAt`, `notBefore`, `expiresAt`,
  `graceDays` (30), `features`, `limits`, optional `installationId`. Features are named from the
  edition split; limits from the limit registry (`LIM-001`), so a licence can only cap what the
  registry knows.
- **Signature.** Ed25519. Public keys, with key IDs, are compiled into `ee/license`, so rotation
  ships a new public key beside the old one. The private key is held offline by the maintainer
  (hardware token or an encrypted file on an offline machine) and used by `plux-license`
  (`ee/cmd/plux-license`), which is part of no image.
- **Verification.** At start-up and every hour, offline, for the file named by
  `PLUX_LICENSE_FILE` or the `license` Helm secret. Invalid or absent: the core runs fully and
  enterprise features are off (`PLX-8601`, `PLX-8600` on use). Expired: `graceDays` of warnings
  (log, metric `plux_license_days_remaining`, an admin notice; `PLX-8602`), then enterprise
  features turn read-only; data stays readable and exportable and nothing is deleted. Over a
  licensed limit: warnings and an audit entry (`PLX-8603`), never refused device traffic.
- **Never gated.** Security capabilities, the audit log, the kill switch, rollbacks and
  break-glass work identically with or without a licence.
- **CLI.** `plux license inspect <file>` and `plux admin license`. A licence can also be uploaded
  through the API (Revision 2026-10-27).
- **Licence text.** `ee/LICENSE`, drafted by the lead and marked for legal review before the
  first public release (it stays on the open-decisions list). `REUSE.toml` covers `ee/` with
  `LicenseRef-Plux-Enterprise`.

## Consequences

- **Positive:** an auditable boundary; the core is complete for security; licences work air-gapped;
  failure modes are gentle.
- **Negative:** extension points are an API to keep stable; a licence cannot be revoked remotely
  before it expires (short terms mitigate); the commercial text is unreviewed until legal sign-off.
- **Follow-up:** migration 0045; licence key rotation runbook; legal review.

## Revision 2026-10-27

The maintainer decided that a licence may also be uploaded through the API, so an installation
administrator can renew it from Studio without access to the host. `LicenseService.InstallLicense`
stays:

- **Who.** An installation administrator only (`GOV-030`); an organisation role never suffices.
- **Size.** At most 64 KiB, a limit well above any licence file and below anything useful for abuse.
- **Verification.** The edition verifies the signature offline, exactly as for a file on disk; a file
  that does not verify is refused (`PLX-8601`) and changes nothing. The core without the enterprise
  edition cannot hold a licence and refuses the call.
- **Audit.** Every installation is recorded in the installation's own chain as `license.state_changed`.
- **File and upload together.** The file named by `PLUX_LICENSE_FILE` (or the `license` Helm secret)
  remains the way to deliver a licence at start-up and for air-gapped installs; an uploaded licence
  replaces the installation's current one, and the hourly verification applies to both.

Nothing else in this decision changes: the licence is still verified offline, never calls home, and
never gates a security capability (`GOV-032`).

## Options in detail

### Option 1 — separate module and offline licences

Clear dependency direction and a core buildable alone. Costs interfaces and a signing tool.

### Option 2 — build tags

Easier to write, but hides the boundary and makes core-only builds easy to break. Rejected.

### Option 3 — online validation

Fails air-gapped installs and adds an availability dependency on a vendor. Rejected.

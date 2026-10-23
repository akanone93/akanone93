# 0082. Audit export, reconstruction releases, data-subject requests and the egress list

- **Status:** Accepted
- **Date:** 2026-10-18
- **Requirements:** `SEC-142`, `SEC-143`, `SEC-162`, `SEC-163`, `SRV-061`, `GOV-011`, `GOV-032`, `SEC-054`

## Context and problem

An auditor must be able to answer "what did this user see at that moment", an operator must feed
the audit log to a SIEM, a data subject can ask for their data to be exported or erased, and the
operator must know and control every outbound connection the server makes. The server also gains
search and Git export. Reconstructing a screen tempts one to build a server-side renderer, which
would add a second, different implementation of the rendering model. The questions: how is a
past moment reproduced faithfully, how does data leave for audit, and how are destinations
controlled?

## Decision drivers

- Faithfulness: what the auditor sees must be what the device rendered, from the same code.
- Verify before load applies to reconstruction too (`SEC-052`).
- No personal data in reconstruction unless retained under consent.
- Erasure is complete and auditable; leaves tombstones only.
- Egress is deny by default (`SEC-080`).
- The audit log, reconstruction, DSR and egress control are never edition-gated (`GOV-032`).

## Considered options

1. A **reconstruction release**: a signed, read-only release of the retained bundles, opened on a device or emulator through a viewer host.
2. A server-side renderer that draws a screenshot or HTML from the document.
3. Store screenshots or session replays on the device and upload them.

## Decision

Chosen option: **1** (maintainer decision Q14: no server-side renderer).

- **Reconstruction (`SEC-142`).** Given an app, device or user ID and a timestamp, the server
  resolves the release active then (from device sync reports already recorded), the bundle
  hashes loaded, the control document, flags and experiment assignments in force, locale, theme,
  text scale and device class. It packages them as a signed, read-only release on a dedicated
  `audit` channel containing the retained bundles exactly as shipped plus a reconstruction
  manifest (route, parameters, recorded state the runtime may keep; no personal data unless
  retained under consent). The `auditor` role (permission `reconstruction.view`) opens it
  through the viewer host (`apps/plux_viewer`, a minimal host that P10 grows into the Dev app).
  The runtime runs in reconstruction mode: no network beyond the server, no actions with side
  effects, a banner naming the moment. Every view is audited. `PLX-8502` when the bundles are no
  longer retained.
- **SIEM export (`SEC-143`).** Syslog RFC 5424 over TLS (core); CEF, OTLP logs and signed
  webhooks (enterprise). At-least-once with a cursor (`PLX-8500` unreachable). Retention follows
  ADR-0080.
- **Data subjects (`SEC-162`).** Export (JSON archive) and erase for a device or pseudonymous
  user ID across events, rollup contributions, device records, function logs and outbox
  metadata. A generated personal-data map states where each field is stored and for how long.
  Erasure is audited and leaves tombstones only (`PLX-8501` subject not found).
- **Egress (`SEC-163`).** One egress client refuses unconfigured destinations: attestation, AI
  provider, push, webhooks, SIEM, SMTP, OTLP, Git remotes. The generated list
  `docs/operations/egress.md` is checked in CI against the code's registry of destinations.
- **Search (`SRV-061`).** PostgreSQL full-text indexes over apps, plugins, pages, components,
  templates, translation keys and functions; results are filtered by the caller's permissions in
  SQL (row-level security), never after fetching.
- **Git export and import (`GOV-011`).** On every publish (a per-app setting) the worker writes
  the `SCH-006` layout to a configured remote with server-held credentials through
  `go-git/go-git/v5` (approved A1; pure Go, since the images have no `git` binary). `plux git
  import` brings a repository back through normal validation. A refusal gives `PLX-8503`.

## Consequences

- **Positive:** the auditor sees the real renderer on real bundles, with no second rendering
  implementation to keep in sync; the release is signed and its use is audited; egress is
  inspectable by anyone reading one file.
- **Negative:** reconstruction needs retained bundles and recorded sync state, so its horizon is
  bounded by retention (`reconstruction.retentionDays`); an auditor needs a device or emulator
  and the viewer host; data that was never recorded cannot be shown.
- **Follow-up:** migrations 0043 to 0044; the viewer host; `dependencies.md` row for `go-git`;
  runbooks for SIEM backlog and DSR.

## Options in detail

### Option 1 — reconstruction release

Reuses the shipped bundles and the production renderer. Costs packaging logic and a viewer host.

### Option 2 — server-side renderer

Would need a Flutter rendering path on the server, with fonts and platform differences that make
its output approximate evidence. Rejected.

### Option 3 — device screenshots

Captures personal and secure content by design, contradicts secure UI (ADR-0059) by design, and only works for devices that opted in. Rejected.

## Revision (2026-10-22, P10: the viewer stays a separate app)

The Decision above says the viewer host is "a minimal host that P10 grows into the Dev app".
Decided by the maintainer (P10 plan D-6; ADR-0093): `apps/plux_viewer` does **not** become the Dev
app. The Dev app is a second app, `apps/plux_dev`, and the two share the new unpublished package
`packages/plux_host_kit` (sign-in, link handling, root-key pinning, release opener). The viewer
keeps pinning production root keys and opening reconstruction releases; the Dev app pins
development root keys. One binary never trusts both sets (`SEC-056`). The viewer's behaviour does
not change.

# Runbooks

One runbook per alert and per failure mode introduced in a phase (DoD-9, `OBS-004`). The first runbooks are written in **P2**.

| Runbook | Covers | Phase |
|---|---|---|
| [Server](server.md) | `plux-server`: dependencies, publishing and signing, manifests and devices, data, upgrades | P2 |
| [Runtime](runtime.md) | `plux_flutter` on devices: sync, releases, rendering, telemetry, host builds | P3 |
| [Key ceremony](key-ceremony.md) | The offline root of the update metadata: first root (2 of 3), rotation, lost or compromised keys (`SEC-121`) | P6 |
| [Attestation outage](attestation-outage.md) | Devices cannot register or re-attest: Play Integrity, Key Attestation or App Attest failing, misconfigured trust (`SEC-002`–`SEC-006`, `SEC-009`) | P6 |
| [Replay-cache outage](replay-cache-outage.md) | Valkey or Sentinel unavailable; per-replica fallback and fail-closed behaviour (`SEC-023`) | P6 |
| [Device revocation](device-revocation.md) | Withdrawing trust from one device or many (`SEC-006`) | P6 |
| [Key compromise](key-compromise.md) | Token key, online metadata keys, audit and wrap keys, content keys; root keys through the ceremony (`SEC-120`, `SEC-121`) | P6 |
| [Pin rotation](pin-rotation.md) | Rotating the server's TLS key and the SPKI pins through signed root metadata (`SEC-041`) | P6 |
| [Gateway upstream failure](gateway-upstream-failure.md) | A customer API behind the gateway is down, slow or rejecting it (`SEC-030`, `SEC-031`) | P6 |
| [Function outage](function-outage.md) | `fnrunner` down, unreachable, overloaded or refusing modules (`FN-009`–`FN-013`) | P7 |
| [Function egress refusals](function-egress-refusals.md) | A function's outbound HTTP is refused by the allowlist or the private-address rules (`FN-012`, `FN-017`) | P7 |
| [Function builder failure](function-builder-failure.md) | Function builds fail, hang or are refused by the `builder` role (`FN-016`, `FN-019`) | P7 |
| [Locale fallbacks](locale-fallbacks.md) | A locale shows keys or the default language: fallback chain, `l10n_missing`, `PLX-4400`/`4401`, the localisation report (`I18N-005`, `I18N-006`) | P8 |
| [Translation import](translation-import.md) | An import is refused or downgraded: size, DTD and entities, `PLX-1620`–`1623` (`I18N-008`, `I18N-009`) | P8 |
| [Publish blocked](publish-blocked-quality.md) | Accessibility, goldens, profile floor or pseudo-locales refuse a publish: `PLX-3060`–`3064` (`A11Y-002`, `TST-003`) | P8 |
| [Golden runs](golden-runs.md) | Golden runs are non-canonical or differ without a change: fonts, frames, Linux renderer (`TST-003`) | P8 |
| [Wrong calendar day](calendar-wrong-day.md) | A date shows the wrong day: Umm al-Qura table range, `PLX-1615` (`I18N-003`) | P8 |
| [Backup and restore](backup-and-restore.md) | PostgreSQL WAL archive and base backups, object-store versioning, the consistency marker, restoring to a point in time, the restore drill (`DEP-010`) | P9 |
| [Studio BFF](../operations/studio-bff.md) | Studio's backend for frontend: configuration, health, sessions, logs and common failures (`STU-002`) | P11 |
| [Alert runbooks](alerts/README.md) | One page per alert of the Prometheus rules: service level objectives with burn-rate alerts, and the infrastructure and security alerts (`OBS-004`) | P9 |
| [Approval deadlock and break-glass](approval-deadlock-and-break-glass.md) | A guarded change waits for approvals that cannot arrive, or an emergency action needs break-glass and its review: `PLX-8001`–`PLX-8006` (`GOV-020`–`GOV-024`) | P9 |
| [Stuck rollout](stuck-rollout.md) | A staged rollout does not advance, does not end or leaves a channel on an unexpected release: `PLX-3153`, closing rollouts, the scheduler (`REL-050`–`REL-053`) | P9 |
| [Health-gate false positives](health-gate-false-positives.md) | A health gate paused or rolled back a rollout on a figure that does not come from the release: `PLX-3150`, `PLX-3151` (`REL-053`) | P9 |
| [Control-document key rotation](control-key-rotation.md) | Rotating the key that signs the control document through the root, and the exposure case (`SYN-060`, `REL-070`) | P9 |
| [Telemetry backlog](telemetry-backlog.md) | Accepted telemetry batches wait in the Valkey stream for the writers; trimming is the last resort (`ANL-010`, `ANL-011`) | P9 |
| [ClickHouse switch-over](clickhouse-switch-over.md) | Moving analytics storage from PostgreSQL to ClickHouse at the sizing threshold; the migration of existing events is an open decision (`ANL-010`) | P9 |
| [SIEM backlog](siem-backlog.md) | An audit export destination has not acknowledged the newest entries: `PLX-8500`, `PLX-8600` (`SEC-143`) | P9 |
| [Data subject request](data-subject-request.md) | Exporting or erasing what an organisation holds about a device or a user: `PLX-1320`, `PLX-8501` (`SEC-162`) | P9 |
| [Licence expiry](licence-expiry.md) | The enterprise licence is in grace, expired or invalid; enterprise writes refused with `PLX-8600` (`GOV-030`, `GOV-032`) | P9 |
| [Upgrade rollback](upgrade-rollback.md) | Returning to the previous release after a failed upgrade or migration, with expand-only migrations (`DEP-030`) | P9 |
| [Air-gapped upgrade](air-gapped-upgrade.md) | Verifying a signed installation bundle and upgrading an installation with no route to the internet (`DEP-040`) | P9 |

Restoring the installation from backups is the [backup and restore runbook](backup-and-restore.md), listed above.

## Signals

Alert rules and dashboards ship in `deploy/observability` since P9 (`OBS-004`, [alert runbooks](alerts/README.md)). The signals they and the runbooks read are `/readyz`, the
metrics of [Appendix G.1](../requirements.md#appendix-g--metrics-and-telemetry-events), the JSON
logs, the audit log and the runtime's telemetry events. The server exports today:
`plux_rpc_requests_total`, `plux_rpc_duration_seconds`, `plux_manifest_requests_total`,
`plux_delta_bytes`, `plux_delta_generation_seconds`, `plux_publish_jobs_total`,
`plux_publish_duration_seconds`, `plux_jobs_queue_depth`, `plux_dpop_replay_cache_degraded` and
`plux_dpop_replay_cache_fallback_total`. Appendix G.1 also catalogues
`plux_dpop_rejections_total`, `plux_attestation_results_total`, `plux_devices_by_assurance` and
`plux_audit_chain_verified`, which the server does not export yet; the P6 runbooks use the
RPC counters in their place and name the catalogued metric where it will go. Gateway calls
are recorded as RPCs of service `plux.gateway` (upstream as `method`, HTTP status as `code`).

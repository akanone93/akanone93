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

## Signals

Alert rules and dashboards arrive in P9 (`OBS-004`). Until then the signals are `/readyz`, the
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

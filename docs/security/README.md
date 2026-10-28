# Security documentation

| Document | Requirement | Arrives |
|---|---|---|
| [threat-model.md](threat-model.md) — STRIDE threat model, reviewed every phase; per-phase deltas until P6, the full model from P6 (DoD-6) | `SEC-000` | P1 (deltas), P6 (full model) |
| [whitepaper.md](whitepaper.md) — the security whitepaper, first edition: architecture, trust chain, data protection, gateway and SCA, RASP limits, verification and honest limits | `DX-002` | P6 (first edition), P9 (compliance pack) |
| [device-trust.md](device-trust.md) — assurance levels per platform, the software-key rule, and how attestation is tested (CI vectors, the development provider, the real-device check) | `SEC-001`–`SEC-008`, `SEC-190` | P6 (decided 2026-10-07) |
| [masvs.md](masvs.md) — OWASP MASVS v2 L2 + RESILIENCE checklist: each control mapped to requirements, automated tests, emulator evidence and the maintainer's real-device checks; MASTG results per release | `SEC-190` | P6 |
| [real-device-checklist.md](real-device-checklist.md) — what only real devices prove, per phase: the steps, the expected results and what stays `WIP` until they pass (P7: the Bank fee success path, the Express device timings) | `FN-020`, `FN-056`, `SEC-002`–`SEC-004`, `SEC-190` | P7 |
| [privacy.md](privacy.md) — processing activities, legal bases, retention and data-subject rights | `SEC-160`, `SEC-161` | P6 |
| [asvs-server.md](asvs-server.md) — ASVS 5.0 L2 and API Top 10 checklist for the server; Studio's half in P11 | `SEC-109` | P2 (server), P11 (Studio) |
| [asvs-studio.md](asvs-studio.md) — ASVS 5.0 L2 checklist for Studio (browser and BFF controls), the ZAP baseline of Studio and the open gaps | `SEC-109`, `QA-008` | P11 |
| [Key ceremony](../runbooks/key-ceremony.md) and the security [runbooks](../runbooks/README.md) | `SEC-121` | P6 |

Report vulnerabilities as described in [SECURITY.md](../../SECURITY.md).

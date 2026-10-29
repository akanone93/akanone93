<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Layer 1 — WIP requirements

One check per requirement still `WIP` in the [specification](../../requirements.md): 83 rows in
80 checks (three pairs share one). Each check ends with **Moves to DONE when:** the evidence
that closes its row. Run [setup](../00-setup.md) first.

## Order

| File | Phases | Checks |
|---|---|---|
| [p00-p01.md](p00-p01.md) | P0 foundations, P1 schema and compiler | 13 |
| [p02.md](p02.md) | P2 backend core | 9 |
| [p03.md](p03.md) | P3 runtime rendering | 15 |
| [p05.md](p05.md) | P5 actions, state, data, local database | 7 |
| [p06-security.md](p06-security.md) | P6 security hardening | 23 |
| [p07-p10.md](p07-p10.md) | P7 functions, P9 operations, P10 dev app | 9 |
| [p11.md](p11.md) | P11 Studio | 4 |

## How each row closes

- **By you, by hand:** most checks; record the result in place.
- **By CI once it runs:** the release tag jobs (CI-008, DEP-001, RT-001), `release-inspect`
  (SEC-074, DEV-050), the kind and restore jobs (DEP-004, DEP-010), the ZAP scan (QA-008),
  `studio-image-repro` (with a hand run of the render worker, STU-100). The check names the job
  and what to look at.
- **By other people:** a screen-reader pass (STU-011), native German and Arabic speakers
  (STU-012), recorded sessions with new developers (HST-034, DX-001), a legal review (GOV-034),
  the security review half of SEC-000.
- **On reference hardware:** the mid-range and low-end phones and the reference laptop
  (NFR-001, NFR-002, NFR-003, NFR-008, NFR-041, RT-004, ANL-002, STU-120, NFR-031).
- **Only after code exists:** the checks say so; the gaps are listed below.

## Gaps the checks found

| Row | Missing |
|---|---|
| CI-001 | No workflow runs OSV-Scanner |
| QA-003 | No DPoP conformance vectors |
| QA-004 | No nightly Dart fuzzing |
| QA-006 | No real-device farm job with a low-end Android |
| QA-007 | The nightly load run has no regression gate |
| QA-010 | Needs three released versions of the runtime and the server |
| DEP-020 | `make dev` does not start Studio's hot reload |
| DX-002 | The use-case guides do not exist |
| REL-003 | No test for native route references |
| SRV-024 | No Studio test names it |
| SYN-010 | Range resume and parallel downloads untested |
| WGT-005, GOV-032 | No test names them |
| RT-060 | Waits for `plux_payments` (P13) |
| DAT-001 | The runtime has no function or static source kind |
| DB-003 | Adapters deferred, or a specification change |
| SYN-014 | No host app turns background sync on |
| ACT-008, PXL-004, DB-007, NFR-013 | No on-phone benchmark harness |
| NFR-025 | No load harness through the gateway |
| SEC-000 | `threat-model.md` has no P10 section |
| — | `device-trust.md` §4 and the attestation-outage runbook say Apple's fraud receipt is not checked; `backend/internal/server/fraudreceipt.go` builds its client |

## Questions for the maintainer

- **SRV-001** asks for every role in one process; the isolation of the function roles (L-4,
  FN-013) may rule that out.
- **SEC-054** may stay open until the web runtime's review (P16).

## Requirement → check

| Requirement | Check |
|---|---|
| `ACT-008` | [L1-RUN-502](p05.md#l1-run-502--action-interpreter-overhead-on-the-reference-phone-act-008-nfr-011) |
| `ANL-002` | [L1-RUN-011](p03.md#l1-run-011--telemetry-costs--1-battery-and--100-kib-a-day-anl-002) |
| `BND-008` | [L1-RUN-001](p00-p01.md#l1-run-001--unknown-required-features-are-refused-bnd-008) |
| `BND-018` | [L1-RUN-002](p00-p01.md#l1-run-002--unknown-optional-content-is-ignored-bnd-018) |
| `CI-001` | [L1-OPS-002](p00-p01.md#l1-ops-002--ci-ok-covers-every-gate-ci-001-lists-ci-001) |
| `CI-001` | [L1-OPS-002](p00-p01.md#l1-ops-002--ci-ok-covers-every-gate-ci-001-lists-ci-001) |
| `CI-008` | [L1-OPS-004](p00-p01.md#l1-ops-004--signed-versioned-component-release-ci-008) |
| `CI-009` | [L1-OPS-001](p00-p01.md#l1-ops-001--main-is-protected-by-a-ruleset-ci-009) |
| `DAT-001` | [L1-RUN-506](p05.md#l1-run-506--which-data-source-kinds-run-dat-001) |
| `DB-002` | [L1-RUN-505](p05.md#l1-run-505--the-local-database-is-encrypted-on-a-phone-under-strict-db-002) |
| `DB-003` | [L1-RUN-507](p05.md#l1-run-507--a-host-app-supplies-its-own-database-adapter-db-003) |
| `DB-007` | [L1-RUN-504](p05.md#l1-run-504--a-watched-1000-row-query-updates-within-a-frame-db-007) |
| `DEP-001` | [L1-OPS-008](p02.md#l1-ops-008--signed-multi-arch-images-with-sbom-and-provenance-dep-001) |
| `DEP-002` | [L1-OPS-007](p02.md#l1-ops-007--one-command-starts-the-whole-stack-dep-002) |
| `DEP-004` | [L1-OPS-702](p07-p10.md#l1-ops-702--ha-install-rolling-upgrade-and-failover-dep-004) |
| `DEP-010` | [L1-OPS-703](p07-p10.md#l1-ops-703--restore-from-backup-within-rpo-and-rto-dep-010) |
| `DEP-020` | [L1-DEV-001](p02.md#l1-dev-001--make-dev-hot-reloads-go-bun-and-flutter-dep-020) |
| `DEV-001` | [L1-DEV-705](p07-p10.md#l1-dev-705--the-dev-app-installs-and-previews-on-both-platforms-dev-001) |
| `DEV-050` | [L1-DEV-706](p07-p10.md#l1-dev-706--no-devtools-in-a-release-build-dev-050) |
| `DX-001` | [L1-DOC-708](p07-p10.md#l1-doc-708--the-scripted-part-of-the-quick-start-is-fast-dx-001) |
| `DX-002` | [L1-DOC-001](p03.md#l1-doc-001--the-documentation-site-covers-what-the-row-lists-dx-002) |
| `FN-056` | [L1-FN-701](p07-p10.md#l1-fn-701--a-device-function-computes-within-20-ms-on-the-reference-phone-fn-056) |
| `GOV-032` | [L1-GOV-001](p00-p01.md#l1-gov-001--security-is-in-the-open-core-gov-032) |
| `GOV-034` | [L1-GOV-002](p00-p01.md#l1-gov-002--contributions-under-a-cla-gov-034) |
| `HST-034` | [L1-DOC-709](p07-p10.md#l1-doc-709--new-developers-reach-a-published-page-in-10-minutes-hst-034) |
| `LIM-005` | [L1-SRV-002](p02.md#l1-srv-002--usage-against-every-limit-api-and-studio-lim-005) |
| `NFR-001` | [L1-RUN-006](p03.md#l1-run-006--pluxinitialize-with-a-cached-release-nfr-001) |
| `NFR-002` | [L1-RUN-008](p03.md#l1-run-008--tap-to-first-frame-of-a-cached-page-nfr-002) |
| `NFR-003` | [L1-RUN-009](p03.md#l1-run-009--page-build-of-300-nodes-nfr-003) |
| `NFR-004` | [L1-RUN-501](p05.md#l1-run-501--1000-item-list-scrolls-without-jank-on-the-reference-phone-nfr-004) |
| `NFR-008` | [L1-RUN-010](p03.md#l1-run-010--memory-with-50-plugins-nfr-008) |
| `NFR-010` | [L1-RUN-503](p05.md#l1-run-503--pxl-binding-evaluation-on-the-reference-phone-pxl-004-nfr-010) |
| `NFR-011` | [L1-RUN-502](p05.md#l1-run-502--action-interpreter-overhead-on-the-reference-phone-act-008-nfr-011) |
| `NFR-012` | [L1-SEC-604](p06-security.md#l1-sec-604--dpop-proof-creation-within-15-ms-on-the-phones-nfr-012) |
| `NFR-013` | [L1-SEC-612](p06-security.md#l1-sec-612--decrypting-a-confidential-bundle-within-10-ms-per-mib-nfr-013) |
| `NFR-025` | [L1-SEC-620](p06-security.md#l1-sec-620--gateway-overhead-and-request-verification-nfr-025) |
| `NFR-031` | [L1-STU-804](p11.md#l1-stu-804--canvases-pan-and-zoom-at-60-fps-on-the-reference-laptop-stu-120-nfr-031) |
| `NFR-040` | [L1-OPS-704](p07-p10.md#l1-ops-704--9995-monthly-availability-of-manifest-and-control-nfr-040) |
| `NFR-041` | [L1-RUN-012](p03.md#l1-run-012--telemetry-cost-on-device-nfr-041) |
| `PXL-004` | [L1-RUN-503](p05.md#l1-run-503--pxl-binding-evaluation-on-the-reference-phone-pxl-004-nfr-010) |
| `PXL-006` | [L1-CMP-001](p00-p01.md#l1-cmp-001--pxl-operators-and-the-standard-library-pxl-006) |
| `QA-003` | [L1-CMP-002](p00-p01.md#l1-cmp-002--one-conformance-suite-three-languages-qa-003) |
| `QA-004` | [L1-OPS-006](p00-p01.md#l1-ops-006--continuous-fuzzing-qa-004) |
| `QA-006` | [L1-OPS-009](p03.md#l1-ops-009--device-end-to-end-in-ci-and-on-a-device-farm-qa-006) |
| `QA-007` | [L1-OPS-010](p03.md#l1-ops-010--benchmarks-fail-on-regressions-qa-007) |
| `QA-008` | [L1-SEC-623](p06-security.md#l1-sec-623--the-security-test-suite-is-complete-qa-008) |
| `QA-010` | [L1-OPS-011](p03.md#l1-ops-011--compatibility-across-the-last-three-releases-qa-010) |
| `QA-072` | [L1-OPS-003](p00-p01.md#l1-ops-003--a-requirement-pr-updates-its-status-qa-072) |
| `REL-003` | [L1-SRV-004](p02.md#l1-srv-004--a-release-with-an-unresolved-reference-is-refused-rel-003) |
| `REL-080` | [L1-SRV-005](p02.md#l1-srv-005--the-publisher-sees-which-devices-can-use-a-release-rel-080) |
| `RT-001` | [L1-RUN-005](p03.md#l1-run-005--pluxflutter-on-pubdev-with-maximum-pub-points-rt-001) |
| `RT-002` | [L1-RUN-003](p03.md#l1-run-003--android-70-and-ios-15-on-the-supported-flutter-rt-002) |
| `RT-004` | [L1-RUN-007](p03.md#l1-run-007--initialize-within-budget-off-the-ui-isolate-rt-004) |
| `RT-060` | [L1-RUN-004](p03.md#l1-run-004--optional-capabilities-in-separate-packages-rt-060) |
| `SEC-000` | [L1-SEC-601](p06-security.md#l1-sec-601--the-threat-model-covers-every-phase-and-is-reviewed-sec-000) |
| `SEC-001` | [L1-SEC-603](p06-security.md#l1-sec-603--the-dpop-key-lives-in-secure-hardware-sec-001) |
| `SEC-002` | [L1-SEC-605](p06-security.md#l1-sec-605--a-real-key-attestation-chain-verifies-sec-002) |
| `SEC-003` | [L1-SEC-606](p06-security.md#l1-sec-606--a-real-play-integrity-verdict-verifies-sec-003) |
| `SEC-004` | [L1-SEC-607](p06-security.md#l1-sec-607--app-attest-registration-and-assertions-on-the-iphone-sec-004) |
| `SEC-006` | [L1-SEC-609](p06-security.md#l1-sec-609--re-attestation-and-revocation-sec-006) |
| `SEC-007` | [L1-SEC-608](p06-security.md#l1-sec-608--evidence-maps-to-the-right-assurance-level-sec-007) |
| `SEC-027` | [L1-SEC-610](p06-security.md#l1-sec-610--strong-customer-authentication-signs-a-transfer-sec-027) |
| `SEC-041` | [L1-SEC-613](p06-security.md#l1-sec-613--pinning-refuses-an-intercepting-proxy-sec-041) |
| `SEC-042` | [L1-SEC-614](p06-security.md#l1-sec-614--optional-pins-for-a-customer-api-domain-sec-042) |
| `SEC-053` | [L1-SEC-611](p06-security.md#l1-sec-611--confidential-bundles-open-only-on-the-registered-phone-sec-053) |
| `SEC-054` | [L1-SEC-001](p00-p01.md#l1-sec-001--no-downloaded-native-code-sec-054) |
| `SEC-070` | [L1-SEC-615](p06-security.md#l1-sec-615--rasp-detects-a-hostile-phone-sec-070) |
| `SEC-071` | [L1-SEC-616](p06-security.md#l1-sec-616--a-detection-gets-its-response-and-lowers-the-level-sec-071) |
| `SEC-072` | [L1-SEC-617](p06-security.md#l1-sec-617--nothing-sensitive-in-plain-storage-sec-072) |
| `SEC-074` | [L1-SEC-602](p06-security.md#l1-sec-602--release-builds-carry-no-dev-tooling-sec-074) |
| `SEC-091` | [L1-SEC-618](p06-security.md#l1-sec-618--secure-pages-block-capture-and-blur-in-the-app-switcher-sec-091) |
| `SEC-093` | [L1-SEC-619](p06-security.md#l1-sec-619--touches-under-an-overlay-are-filtered-sec-093) |
| `SEC-109` | [L1-SEC-002](p02.md#l1-sec-002--asvs-50-l2-and-api-top-10-evidence-sec-109) |
| `SEC-160` | [L1-SEC-621](p06-security.md#l1-sec-621--no-personal-data-leaves-the-phone-without-consent-sec-160) |
| `SEC-190` | [L1-SEC-622](p06-security.md#l1-sec-622--mastg-results-recorded-for-a-release-sec-190) |
| `SRV-001` | [L1-SRV-001](p02.md#l1-srv-001--one-binary-roles-by-configuration-srv-001) |
| `SRV-024` | [L1-SRV-003](p02.md#l1-srv-003--durable-jobs-visible-to-the-administrator-srv-024) |
| `STU-011` | [L1-STU-801](p11.md#l1-stu-801--studio-works-with-a-screen-reader-stu-011) |
| `STU-012` | [L1-L10N-802](p11.md#l1-l10n-802--german-and-arabic-read-naturally-arabic-mirrors-stu-012) |
| `STU-100` | [L1-STU-803](p11.md#l1-stu-803--the-publish-flow-shows-a-server-rendered-visual-diff-stu-100) |
| `STU-120` | [L1-STU-804](p11.md#l1-stu-804--canvases-pan-and-zoom-at-60-fps-on-the-reference-laptop-stu-120-nfr-031) |
| `SYN-010` | [L1-SYN-001](p03.md#l1-syn-001--background-download-resume-and-backoff-syn-010) |
| `SYN-014` | [L1-SYN-707](p07-p10.md#l1-syn-707--background-sync-stages-a-release-never-activates-it-syn-014) |
| `WGT-005` | [L1-OPS-005](p00-p01.md#l1-ops-005--the-flutter-pin-is-the-latest-stable-wgt-005) |

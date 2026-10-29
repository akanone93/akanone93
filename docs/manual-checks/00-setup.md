<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Setup and shared resources

Everything the checks run against. Do this once; later checks reuse what it leaves behind.
The commands come from [Run it locally](../guides/run-locally.md), which has the details.

## S-1 — Tools

- **Do:** install Go (the `toolchain` of `backend/go.mod`), Flutter 3.47, Bun 1.3, Python 3,
  GNU Make 4 and Docker; then `make setup` and `make studio-install` from the repository root.
- **Expect:** `make help` lists the targets; `make setup` ends without an error.

## S-2 — The stack with Studio, seeded

- **Do:** `make compose-seed`.
- **Expect:** `docker compose -f deploy/compose/compose.yaml ps` shows every service healthy;
  `deploy/compose/.secrets/dev.env` exists.
- **Look at:** `curl -k https://localhost:8080/livez`, `curl http://localhost:8081/readyz`.

| Address | What |
|---|---|
| <https://localhost:8080> | Plux Server API (TLS under the development CA in `deploy/compose/.secrets/`) |
| <http://localhost:8081> | Studio (BFF and web app) |
| <http://localhost:3000> | Grafana |
| <http://localhost:9090> | Prometheus |

Object storage (SeaweedFS) is reachable only inside the Compose network; port 8180 is
Keycloak's, under `COMPOSE_PROFILES=keycloak`.

## S-3 — A device with the starter app

- **Do:** start an Android emulator (or an iOS simulator or a phone), then `make dev`.
- **Expect:** the starter app opens on the device: a native screen with a Plux page, a sync
  tile, **Open welcome**, consent and theme switches, and the devtools overlay.

## Shared resources

A check names the resources it needs. Setup makes the first ones; the check in the last column
makes the rest, and later checks reuse them.

| ID | What | Made by |
|---|---|---|
| R-STACK | The seeded stack of S-2 | S-2 |
| R-ADMIN | Administrator `dev@plux.localhost`, password `PLUX_DEV_PASSWORD` in `dev.env` | S-2 |
| R-TOKEN | Command-line token `PLUX_DEV_TOKEN` in `dev.env` | S-2 |
| R-ORG | Organisation `acme` (`PLUX_DEV_ORGANIZATION`) | S-2 |
| R-APP-DEMO | App `demo`, the loan calculator, promoted to `staging` (`PLUX_DEV_APP`) | S-2 |
| R-APP-STARTER | App `starter` (`PLUX_DEV_STARTER`) | S-2 |
| R-DEVICE | The emulator, simulator or phone running the starter app | S-3 |
| R-STUDIO | A browser signed in to Studio as R-ADMIN | [L2-STU-001](layer-2-functional/studio.md) |

### Layer 1 resources

Hardware, people and tools the [layer 1](layer-1-wip/README.md) checks need; each file's
"Resources this file adds" table has the details.

| ID | What | Defined in |
|---|---|---|
| R-L1-GH | `gh` signed in with admin rights on the repository | [p00-p01.md](layer-1-wip/p00-p01.md) |
| R-L1-TAG | The first signed release tags (0.0.1) | [p00-p01.md](layer-1-wip/p00-p01.md) |
| R-L1-API | Shell function `pluxapi` calling the API as R-TOKEN | [p02.md](layer-1-wip/p02.md) |
| R-L1-MID | Mid-tier reference Android phone (spec §30.1) on `adb` | [p03.md](layer-1-wip/p03.md) |
| R-L1-LOW | Low-end reference Android phone (spec §30.1) on `adb` | [p03.md](layer-1-wip/p03.md) |
| R-L1-BENCH | Runtime benchmark runs per phone and their report | [p03.md](layer-1-wip/p03.md) |
| R-L1-IPHONE | iPhone with App Attest in an Apple Developer team, and a Mac with Xcode | [p06-security.md](layer-1-wip/p06-security.md) |
| R-L1-ATTEST | The stack reachable from the phones with real attestation | [p06-security.md](layer-1-wip/p06-security.md) |
| R-L1-BANK | Release builds of Plux Bank and Plux Express on R-L1-ATTEST | [p06-security.md](layer-1-wip/p06-security.md) |
| R-L1-DEVLIST | The device-list API call | [p06-security.md](layer-1-wip/p06-security.md) |
| R-L1-ROOTED | A rooted Android phone with Frida and LSPosed | [p06-security.md](layer-1-wip/p06-security.md) |
| R-L1-PROXY | `mitmproxy` with its CA on both phones | [p06-security.md](layer-1-wip/p06-security.md) |
| R-L1-KIND | A machine for the kind and Helm checks | [p07-p10.md](layer-1-wip/p07-p10.md) |
| R-L1-HA | A production-like HA installation from the Helm chart | [p07-p10.md](layer-1-wip/p07-p10.md) |
| R-L1-DEVAPP | The Plux Dev app on R-L1-MID and R-L1-IPHONE | [p07-p10.md](layer-1-wip/p07-p10.md) |
| R-L1-NEWDEVS | Developers new to Plux, with recording | [p07-p10.md](layer-1-wip/p07-p10.md) |
| R-L1-SR | Screen readers: NVDA on Windows, VoiceOver on macOS | [p11.md](layer-1-wip/p11.md) |
| R-L1-NATIVE | Native German and Arabic speakers with Studio accounts | [p11.md](layer-1-wip/p11.md) |
| R-L1-LAPTOP | The reference laptop (spec §30) | [p11.md](layer-1-wip/p11.md) |

Layer files add rows here when a check creates a resource others reuse (a second user, a
plugin, fonts, a function, a paired Dev app device…).

<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Open items and known gaps

## For the maintainer

- None open: every decision is recorded in decisions.md (2026-10-28 batch).

## Known gaps (fix before the phase exit)

- Nothing that needs Docker has run here (no daemon): the Studio image build (`studio/Dockerfile`, now with the validator WebAssembly stage, `studio/Dockerfile.dockerignore` and the notices), `make studio-image-repro`, `make compose-up` with Studio, and the render worker image (`deploy/render-worker/Dockerfile`, whose pub cache and SDK ownership were fixed without a build) are for a machine with Docker or the phase-exit CI run. The render worker's deployment is written but unrun: the Compose service `plux-render-worker` (profile `visual-diff`, not `read_only` because the Flutter tool writes below its SDK; try a read-only root with a writable cache once the image runs) and the Helm role (`renderWorker.enabled`; an init container copies the SDK into an `emptyDir` so the root stays read-only, with `--flutter` and `PUB_CACHE` pointing at the copy) need `COMPOSE_PROFILES=visual-diff make compose-up` and a kind install to confirm the container starts, answers mutual TLS and renders. `make helm-lint` and `init-secrets.sh` (the internal CA now names `plux-render-worker`) are checked here. The CI jobs that build the Studio image now install Bun and Flutter and stage the CanvasKit (`make studio-image-inputs`); their first run is the check.
- The release workflow's Studio and render worker image jobs (`release.yml`, `image.yml`: SBOM attestation, cosign, SLSA provenance, reproducible timestamps) are checked by `actionlint` only; they run on the first `studio/v*` and `backend/v*` tags.
- A request larger than the default `archive.expandedBytes` (twice, both projects) plus the function catalogue allowance (`renderworker.CatalogueAllowance`, 8 MiB: at most `app.functions` names and their schemas) is refused by the worker (`renderworker.MaxRequestBytes`), and `golden.casesPerRun` / `golden.imageBytes` above their defaults are not honoured by the worker (it only tightens); a visual diff renders one locale, the light theme and text scale 1 on the standard devices.

## Closed in S9.5

- Reproducible Studio build: `make studio-build-repro` (two `vite build` runs plus the pre-compression, byte-identical `dist/`, run here: reproducible, about 8 minutes because of Brotli) and `studio-image-repro` in CI (Docker).
- Render worker catalogue (this slice): the render request carries the environment's function declarations and the `deviceFunctions` setting, the same catalogue a release build compiles against (`function.Service.Catalogue` through `releasereq.Functions`); the worker compiles with it and with stub device modules, so apps that call server or device functions render.
- Visual-diff renderer client (S7.1/O-9): `renderclient` (worker role) calls `plux render-worker serve` (render worker image) over mutual TLS; the worker role claims, limits and completes the jobs as before (`releasereq.RunRender`), the worker holds no credential (L-4). Studio fetches the stored images by hash with `ReleaseRequestService.GetVisualDiffImage` (`plugin.read`, the hash must be in the request's current diff, other organisations get not found) and shows thumbnails in the diffs step.

## Legal reviews still open for the maintainer (O-7, conditions of release; not closed here)

1. The ported Flutter layout code (BSD-3-Clause inside the AGPL `studio/`, `REUSE.toml`).
2. SF Pro through Local Font Access (not shipped; Studio reads the installed copy).
3. The OFL fonts (Roboto, Noto Sans in the canvas; the `@fontsource` fonts; texts in the image under `fonts/` and the package folders).
4. Phosphor paths (MIT, `@phosphor-icons/core`).
5. `axe-core` (MPL-2.0, development only; confirm it is in no shipped file).
6. The notices of CanvasKit/Skia, Monaco, CodeMirror and every shipped package: `studio/notices/THIRD_PARTY_NOTICES.txt` and the per-package files (`scripts/release/js-notices.sh`) are in the image; the enumeration of the components inside the CanvasKit binary is not complete.

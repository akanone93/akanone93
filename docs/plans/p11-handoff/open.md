<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Open items and known gaps

## For the maintainer

- None open: every decision is recorded in decisions.md (2026-10-28 batch).

## Known gaps (fix before the phase exit)

- Nothing that needs Docker has run here (no daemon): the Studio image build (`studio/Dockerfile`, now with the validator WebAssembly stage, `studio/Dockerfile.dockerignore` and the notices), `make studio-image-repro`, `make compose-up` with Studio, and the render worker image (`deploy/render-worker/Dockerfile`, whose pub cache and SDK ownership were fixed without a build) are for a machine with Docker or the phase-exit CI run. The CI jobs that build the Studio image now install Bun and Flutter and stage the CanvasKit (`make studio-image-inputs`); their first run is the check.
- The release workflow's Studio and render worker image jobs (`release.yml`, `image.yml`: SBOM attestation, cosign, SLSA provenance, reproducible timestamps) are checked by `actionlint` only; they run on the first `studio/v*` and `backend/v*` tags.
- Visual diffs of apps that call server or device functions: the render worker compiles with no function catalogue, so such a project fails with the compiler's diagnostics (the job fails after its attempts). Handing the catalogue to the worker is follow-up work in S9.
- The render worker has no Compose service or Helm template yet: an installation sets `renderWorker.address` and runs `plux render-worker serve` from the image with an internal certificate. Compose and Helm wiring (and the certificate in `init-secrets.sh`) is the remaining part of the render worker's deployment.
- A request larger than the default `archive.expandedBytes` (twice, both projects) is refused by the worker (`renderworker.MaxRequestBytes`), and `golden.casesPerRun` / `golden.imageBytes` above their defaults are not honoured by the worker (it only tightens); a visual diff renders one locale, the light theme and text scale 1 on the standard devices.

## Closed in S9.5

- Reproducible Studio build: `make studio-build-repro` (two `vite build` runs plus the pre-compression, byte-identical `dist/`, run here: reproducible, about 8 minutes because of Brotli) and `studio-image-repro` in CI (Docker).
- Visual-diff renderer client (S7.1/O-9): `renderclient` (worker role) calls `plux render-worker serve` (render worker image) over mutual TLS; the worker role claims, limits and completes the jobs as before (`releasereq.RunRender`), the worker holds no credential (L-4). Studio fetches the stored images by hash with `ReleaseRequestService.GetVisualDiffImage` (`plugin.read`, the hash must be in the request's current diff, other organisations get not found) and shows thumbnails in the diffs step.

## Legal reviews still open for the maintainer (O-7, conditions of release; not closed here)

1. The ported Flutter layout code (BSD-3-Clause inside the AGPL `studio/`, `REUSE.toml`).
2. SF Pro through Local Font Access (not shipped; Studio reads the installed copy).
3. The OFL fonts (Roboto, Noto Sans in the canvas; the `@fontsource` fonts; texts in the image under `fonts/` and the package folders).
4. Phosphor paths (MIT, `@phosphor-icons/core`).
5. `axe-core` (MPL-2.0, development only; confirm it is in no shipped file).
6. The notices of CanvasKit/Skia, Monaco, CodeMirror and every shipped package: `studio/notices/THIRD_PARTY_NOTICES.txt` and the per-package files (`scripts/release/js-notices.sh`) are in the image; the enumeration of the components inside the CanvasKit binary is not complete.

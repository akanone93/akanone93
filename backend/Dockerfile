# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# Images of plux-server, the plux CLI and the function builder (DEP-001,
# SEC-108). plux-server and plux are static, cgo-free binaries running as the
# nonroot user on distroless images; the build uses the same flags as
# `make go-build`, so an image holds the same bytes as the released binary
# (CI-006). The server image also carries plux-svgc and its library, so it is
# based on distroless "base-nossl" (glibc) rather than "static". It runs the
# api and worker roles and, as separate containers, the fnrunner and egress
# roles, which hold no credential and need nothing more. The builder image
# runs the builder role (FN-016, ADR-0063): the same binary on Debian, with
# the pinned Rust toolchain, Bun and Binaryen that compile uploaded function
# sources. Each image carries the third-party notices of what it ships under
# /usr/share/doc.
#
#   docker buildx build -f backend/Dockerfile --target server .
#   docker buildx build -f backend/Dockerfile --target server-ee .
#   docker buildx build -f backend/Dockerfile --target cli .
#   docker buildx build -f backend/Dockerfile --target builder .
#
# Base images are pinned by digest and updated by Dependabot.

FROM --platform=$BUILDPLATFORM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG COMMIT_DATE=unknown
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download
COPY backend/ ./backend/
RUN cd backend && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
      -ldflags "-s -w -buildid= \
        -X github.com/nightCode42/plux3/backend/internal/buildinfo.version=$VERSION \
        -X github.com/nightCode42/plux3/backend/internal/buildinfo.commit=$COMMIT \
        -X github.com/nightCode42/plux3/backend/internal/buildinfo.commitDate=$COMMIT_DATE" \
      -o /out/ ./cmd/plux-server ./cmd/plux && mkdir -p /out/keys
# The third-party notices of each binary, for the target platform.
COPY scripts/release/go-notices.sh /src/scripts/release/
RUN cd backend && export CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH && \
    sh ../scripts/release/go-notices.sh /out/notices/plux-server ./cmd/plux-server && \
    sh ../scripts/release/go-notices.sh /out/notices/plux ./cmd/plux

# plux-svgc, the worker's SVG compiler (CMP-031, ADR-0027 Revision), and
# the Skia path operations it loads. Both are built for the target
# platform on Debian 12, the base of the server image, so they need no
# newer glibc than it has.
FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS pathops
COPY packages/plux_svgc/native/build.sh /src/build.sh
RUN sh /src/build.sh /out

FROM dart:3.13.4@sha256:33faf91bc941466a767ce845b4bbb5d578ecd180abe5ee243e9c8c039109d215 AS svgc
WORKDIR /src/packages/plux_svgc
COPY packages/plux_svgc/pubspec.yaml packages/plux_svgc/pubspec.lock ./
RUN dart pub get --enforce-lockfile
COPY packages/plux_svgc/ ./
# The golden SVG and its encoding, which the tests compare.
COPY schema/testdata/documents/widgets/assets/icons/check.svg /src/schema/testdata/documents/widgets/assets/icons/
COPY schema/testdata/bundles/widgets/variants/check.vec /src/schema/testdata/bundles/widgets/variants/
COPY --from=pathops /out/libpath_ops.so /out/libpath_ops.so
COPY --from=pathops /out/*.LICENSE /out/notices/
# The tests run the compiled library before the image ships it, and
# compare its output with the golden every architecture must reproduce.
RUN PLUX_PATH_OPS=/out/libpath_ops.so dart test && \
    dart compile exe bin/plux_svgc.dart -o /out/plux-svgc && \
    sh native/notices.sh /out/notices

# Distroless "base-nossl": glibc for the Dart runtime and the library;
# still no shell and no package manager (SEC-108).
FROM gcr.io/distroless/base-nossl-debian12:nonroot@sha256:be40c00dfabd86576d92666e87e406714d5618342de1a0c213ad232de255172e AS server
COPY --from=build /out/plux-server /usr/local/bin/plux-server
COPY --from=svgc /out/plux-svgc /usr/local/bin/plux-svgc
COPY --from=svgc /out/libpath_ops.so /usr/local/lib/plux/libpath_ops.so
COPY --from=build /out/notices/plux-server /usr/share/doc/plux-server/
COPY --from=svgc /out/notices /usr/share/doc/plux-svgc/
# The file signing backend's directory, owned by nonroot so a volume
# mounted there is writable with a read-only root filesystem.
COPY --from=build --chown=65532:65532 /out/keys /var/lib/plux/keys
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/plux-server"]
CMD ["serve", "--config", "/etc/plux/plux.yaml"]

# The enterprise server (GOV-030, ADR-0084): the server image with
# plux-server-ee in place of plux-server. The ee/ module resolves the core
# through its replace directive, so the build copies both trees. The flags
# are those of the core build, without its version stamp, which the ee/
# module cannot import; the image keeps the notices of the core server.
FROM build AS build-ee
ARG TARGETOS
ARG TARGETARCH
COPY ee/backend/ ./ee/backend/
RUN cd ee/backend && go mod download && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
      -ldflags "-s -w -buildid=" -o /out-ee/plux-server ./cmd/plux-server-ee

FROM server AS server-ee
COPY --from=build-ee /out-ee/plux-server /usr/local/bin/plux-server

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS cli
COPY --from=build /out/plux /usr/local/bin/plux
COPY --from=build /out/notices/plux /usr/share/doc/plux/
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/plux"]

# The pinned Bun (the Makefile's BUN_VERSION), which runs the pinned Binaryen
# release below; the same pair `plux fn build` installs on a developer's
# machine (E10 of docs/plans/p7.md).
FROM oven/bun:1.3.11-slim@sha256:478281fdd196871c7e51ba6a820b7803a8ae97042ec86cdbc2e1c6b6626442d9 AS bun

# Binaryen 132 from npm, run by Bun: wasm-opt for every function module
# (FN-001). Installed exactly, so a different release is never picked up.
FROM bun AS binaryen
WORKDIR /opt/binaryen
RUN bun add --exact binaryen@132.0.0

# The builder role: the pinned Rust toolchain (sdk/fn/rust-toolchain.toml)
# with the wasm32-wasip1 target, a C linker for procedural macros and build
# scripts (both run on the build host, inside the sandbox), Bun, Binaryen and
# the SDK crate the builder pins. Debian rather than distroless, because
# cargo and rustc need a complete userland. It runs as an unprivileged user
# and holds no credential for the database, the object store or the signing
# keys (L-4).
FROM rust:1.97.0-slim-bookworm@sha256:6d220bf85c74e842a79da63997af8d2e74455c0b8847d8bb3a5888572334991d AS builder
RUN rustup target add wasm32-wasip1 \
 && useradd --uid 65532 --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin nonroot \
 && install -d -o 65532 -g 65532 -m 0700 /var/lib/plux/build
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
COPY --from=binaryen /opt/binaryen /opt/binaryen
COPY sdk/fn/plux-fn /opt/plux/sdk/plux-fn
COPY --from=build /out/plux-server /usr/local/bin/plux-server
COPY --from=build /out/notices/plux-server /usr/share/doc/plux-server/
# The builder checks `rustc --version` against the pin (FN-001); this keeps
# rustup from choosing any other toolchain.
ENV RUSTUP_TOOLCHAIN=1.97.0
USER nonroot:nonroot
EXPOSE 8446
ENTRYPOINT ["/usr/local/bin/plux-server"]
CMD ["serve", "--config", "/etc/plux/plux.yaml"]

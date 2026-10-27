# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0

# The render worker image (O-9, ADR-0097): the headless Flutter toolchain that
# renders the visual diffs of release requests. It runs the P8 golden harness
# (`plux golden run`, ADR-0070) with flutter_tester on the standard devices.
#
# It is its own image, like the builder (backend/Dockerfile, target builder),
# so that a large toolchain never sits beside credentials: the image holds no
# database credential, no object-store credential and no signing key, and the
# process is given its input and returns images (L-4, SRV-052). It runs as an
# unprivileged user, and the Flutter SDK and every package are fetched at
# build time, so a render needs no network.
#
# Build from the repository root, with the SDK archive's digest from the
# release pipeline (the Makefile's FLUTTER_VERSION is the pin):
#
#   docker buildx build -f deploy/render-worker/Dockerfile \
#     --build-arg FLUTTER_VERSION=3.47.5 --build-arg FLUTTER_SHA256=<sha256 of the archive> .

# The `plux` CLI, which holds the golden harness.
FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS plux
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download
COPY backend/ ./backend/
RUN cd backend && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-s -w -buildid=" -o /out/plux ./cmd/plux

# The Flutter SDK at the pinned version, checked against its digest before it
# is unpacked, and warmed up for the Linux test runner only.
FROM dart:3.13.4@sha256:33faf91bc941466a767ce845b4bbb5d578ecd180abe5ee243e9c8c039109d215 AS flutter
ARG FLUTTER_VERSION
ARG FLUTTER_SHA256
RUN test -n "$FLUTTER_VERSION" && test -n "$FLUTTER_SHA256"
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl xz-utils git unzip \
 && rm -rf /var/lib/apt/lists/*
RUN curl --fail --silent --show-error --location --output /tmp/flutter.tar.xz \
      "https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_${FLUTTER_VERSION}-stable.tar.xz" \
 && echo "${FLUTTER_SHA256}  /tmp/flutter.tar.xz" | sha256sum --check --strict - \
 && mkdir -p /opt && tar -xJf /tmp/flutter.tar.xz -C /opt && rm /tmp/flutter.tar.xz
ENV PATH=/opt/flutter/bin:$PATH
RUN git config --global --add safe.directory /opt/flutter \
 && flutter config --no-analytics --no-cli-animations \
 && flutter precache --no-android --no-ios --no-web --no-macos --no-windows --no-fuchsia --linux
# The runtime and the packages its widgets use, resolved against their
# lockfiles, so that no render fetches anything.
WORKDIR /opt/plux/packages
COPY packages/ ./
RUN cd plux_flutter && flutter pub get --enforce-lockfile

# Debian, because flutter_tester needs a complete userland. The golden fonts
# are the ones the harness pins (schema/goldens/fonts/VERSION), so that a
# diff changes only when the content does.
FROM dart:3.13.4@sha256:33faf91bc941466a767ce845b4bbb5d578ecd180abe5ee243e9c8c039109d215 AS render-worker
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git libglu1-mesa libstdc++6 \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 65532 --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin nonroot \
 && install -d -o 65532 -g 65532 -m 0700 /var/lib/plux/render
COPY --from=flutter /opt/flutter /opt/flutter
COPY --from=flutter /opt/plux/packages /opt/plux/packages
COPY --from=plux /out/plux /usr/local/bin/plux
COPY schema/goldens/ /opt/plux/schema/goldens/
ENV PATH=/opt/flutter/bin:$PATH \
    FLUTTER_SUPPRESS_ANALYTICS=true \
    PUB_OFFLINE=true \
    HOME=/var/lib/plux/render
# The image holds no credential for the database, the object store or the
# signing keys. The work directory is the only place the process writes.
USER nonroot:nonroot
WORKDIR /var/lib/plux/render
ENTRYPOINT ["/usr/local/bin/plux"]
CMD ["golden", "run", "--help"]

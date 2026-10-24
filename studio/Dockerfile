# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: AGPL-3.0-only

# The Studio image (DEP-001): the BFF (apps/bff/src/main.ts) run by Bun,
# serving the built web app with its pre-compressed .br files. The build uses
# the same steps as `make studio-build` with a frozen lockfile, so the image
# holds the bytes CI measured (CI-006). The runtime image runs as the numeric
# user 65532 and needs no writable path but /tmp, so a read-only root
# filesystem works. It carries the licence files of the production
# dependencies under /usr/share/doc/plux-studio (the legal review stays open,
# O-7).
#
#   docker buildx build -f studio/Dockerfile --build-arg VERSION=0.0.1 .
#
# Reproducible: no timestamps enter the image; SOURCE_DATE_EPOCH is the commit
# date (build with --output type=image,rewrite-timestamp=true). The base image
# is pinned by digest and updated by Dependabot; its Bun version equals
# BUN_VERSION in the Makefile.

FROM oven/bun:1.3.11-slim@sha256:478281fdd196871c7e51ba6a820b7803a8ae97042ec86cdbc2e1c6b6626442d9 AS build
WORKDIR /src/studio
COPY studio/ ./
RUN bun install --frozen-lockfile \
 && bun run gen:theme \
 && (cd packages/ui && bun scripts/gen-icons.ts) \
 && bun run build \
 && bun scripts/precompress.ts apps/web/dist

FROM oven/bun:1.3.11-slim@sha256:478281fdd196871c7e51ba6a820b7803a8ae97042ec86cdbc2e1c6b6626442d9 AS prod
WORKDIR /src/studio
COPY studio/ ./
RUN bun install --frozen-lockfile --production
COPY scripts/release/js-notices.sh /src/scripts/release/
RUN sh /src/scripts/release/js-notices.sh node_modules /out/notices

FROM oven/bun:1.3.11-slim@sha256:478281fdd196871c7e51ba6a820b7803a8ae97042ec86cdbc2e1c6b6626442d9 AS studio
ARG VERSION=dev
ARG COMMIT=unknown
ARG SOURCE_DATE_EPOCH=0
LABEL org.opencontainers.image.title="plux-studio" \
      org.opencontainers.image.description="Plux Studio: the web app and its backend for frontend" \
      org.opencontainers.image.source="https://github.com/nightCode42/plux3" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT"
ENV NODE_ENV=production \
    PLUX_BFF_HOST=0.0.0.0 \
    PLUX_BFF_PORT=8081
WORKDIR /app
COPY --from=prod /src/studio/node_modules ./node_modules
COPY --from=prod /src/studio/package.json /src/studio/bunfig.toml ./
COPY --from=prod /src/studio/packages ./packages
COPY --from=prod /src/studio/apps/bff ./apps/bff
COPY --from=build /src/studio/apps/web/package.json ./apps/web/package.json
COPY --from=build /src/studio/apps/web/dist ./apps/web/dist
COPY --from=prod /out/notices /usr/share/doc/plux-studio/
USER 65532:65532
EXPOSE 8081
# /livez checks the process only; /readyz also asks the API upstream.
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD ["bun", "-e", "const r = await fetch('http://127.0.0.1:8081/livez'); process.exit(r.ok ? 0 : 1)"]
ENTRYPOINT ["bun", "run", "apps/bff/src/main.ts"]

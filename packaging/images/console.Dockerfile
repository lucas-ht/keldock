FROM node:24.22.1-bookworm-slim@sha256:be8204d4bd327363357f532187b752ff62759c31bedf98e9e18fc08c9cbd605c AS build
ENV NEXT_TELEMETRY_DISABLED=1
WORKDIR /src
RUN npm install --global pnpm@11.1.2
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/console/package.json ./apps/console/package.json
RUN pnpm install --frozen-lockfile --ignore-scripts
COPY apps/console/ ./apps/console/
RUN pnpm --filter @keldock/console build

FROM node:24.22.1-bookworm-slim@sha256:be8204d4bd327363357f532187b752ff62759c31bedf98e9e18fc08c9cbd605c
ARG VERSION=0.6.0-alpha.1
ARG REVISION=development
LABEL org.opencontainers.image.source="https://github.com/lucas-ht/keldock" org.opencontainers.image.version="${VERSION}" org.opencontainers.image.revision="${REVISION}" org.opencontainers.image.licenses="AGPL-3.0-only"
ENV NEXT_TELEMETRY_DISABLED=1 NODE_ENV=production PORT=3800 HOSTNAME=0.0.0.0 KELDOCK_API_ORIGIN=http://127.0.0.1:3801
WORKDIR /app
COPY --from=build --chown=65532:65532 /src/apps/console/.next/standalone/ ./
COPY --from=build --chown=65532:65532 /src/apps/console/.next/static/ ./apps/console/.next/static/
COPY LICENSE /usr/share/licenses/keldock/LICENSE
USER 65532:65532
EXPOSE 3800
CMD ["node", "apps/console/server.js"]

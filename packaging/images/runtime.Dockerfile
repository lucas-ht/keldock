FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS build
ARG VERSION=0.6.0-alpha.1
ARG REVISION=development
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN --mount=type=cache,id=keldock-go-build,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -p 2 -buildvcs=false -trimpath -ldflags="-s -w -X github.com/lucas-ht/keldock/internal/buildinfo.Version=${VERSION} -X github.com/lucas-ht/keldock/internal/buildinfo.Revision=${REVISION}" -o /out/keldock ./cmd/keldock

FROM postgres:18.4-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15
ARG VERSION=0.6.0-alpha.1
ARG REVISION=development
LABEL org.opencontainers.image.source="https://github.com/lucas-ht/keldock" org.opencontainers.image.version="${VERSION}" org.opencontainers.image.revision="${REVISION}" org.opencontainers.image.licenses="AGPL-3.0-only"
COPY --from=build /out/keldock /usr/local/bin/keldock
COPY LICENSE /usr/share/licenses/keldock/LICENSE
USER 65532:65532
WORKDIR /tmp
ENTRYPOINT ["/usr/local/bin/keldock"]

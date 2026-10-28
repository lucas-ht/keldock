# A disposable S3 test dependency built from pinned AGPLv3 MinIO source.
# This image is not installed or recommended as a production storage service.
FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS build
ADD --checksum=sha256:8b11129f6a6830768bcdebab569290ad956618f5ff6ef4857b633acb64f95126 \
    https://codeload.github.com/minio/minio/tar.gz/01ce918d8279a20e4706b96a64396146894adee4 /tmp/minio.tar.gz
WORKDIR /src
RUN tar -xzf /tmp/minio.tar.gz --strip-components=1
RUN CGO_ENABLED=0 go build -p 2 -trimpath -ldflags="-s -w -X github.com/minio/minio/cmd.Version=2025-09-07T16:13:09Z -X github.com/minio/minio/cmd.ReleaseTag=RELEASE.2025-09-07T16-13-09Z -X github.com/minio/minio/cmd.CommitID=01ce918d8279a20e4706b96a64396146894adee4" -o /out/minio .
FROM postgres:18.4-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15
COPY --from=build /out/minio /usr/local/bin/minio
COPY --from=build /src/LICENSE /usr/share/licenses/minio/LICENSE
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/minio"]
CMD ["server", "/data", "--console-address", ":9001"]

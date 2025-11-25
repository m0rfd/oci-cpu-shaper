# syntax=docker/dockerfile:1.6

ARG GO_VERSION=1.25.4
ARG VERSION="dev"
ARG GIT_COMMIT="unknown"
ARG BUILD_DATE="unknown"

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION}-bookworm AS builder

WORKDIR /src

RUN --mount=type=bind,source=go.mod,target=/src/go.mod,readonly \
    --mount=type=bind,source=go.sum,target=/src/go.sum,readonly \
    --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

ARG TARGETOS=linux
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION
ARG GIT_COMMIT
ARG BUILD_DATE

ENV CGO_ENABLED=0

RUN --mount=type=bind,source=.,target=/src,readonly \
    --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH:-amd64} GOARM=${TARGETVARIANT#v} \
    go build \
      -trimpath \
      -ldflags="-s -w -X oci-cpu-shaper/internal/buildinfo.Version=${VERSION} -X oci-cpu-shaper/internal/buildinfo.GitCommit=${GIT_COMMIT} -X oci-cpu-shaper/internal/buildinfo.BuildDate=${BUILD_DATE}" \
      -o /out/oci-cpu-shaper \
      ./cmd/shaper && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH:-amd64} GOARM=${TARGETVARIANT#v} \
    go build \
      -trimpath \
      -o /out/oci-cpu-shaper-healthcheck \
      ./cmd/healthcheck

FROM gcr.io/distroless/static:nonroot AS rootless

ARG VERSION
ARG GIT_COMMIT
ARG BUILD_DATE
ARG IMAGE_SOURCE="https://github.com/oci-cpu-shaper/oci-cpu-shaper"

LABEL org.opencontainers.image.title="oci-cpu-shaper" \
      org.opencontainers.image.description="OCI CPU shaper (distroless rootless)" \
      org.opencontainers.image.source="${IMAGE_SOURCE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=builder /out/oci-cpu-shaper /usr/local/bin/oci-cpu-shaper
COPY --from=builder /out/oci-cpu-shaper-healthcheck /usr/local/bin/oci-cpu-shaper-healthcheck
COPY configs/offline-smoke.yaml /etc/oci-cpu-shaper/config.yaml
COPY configs /etc/oci-cpu-shaper/configs

USER nonroot:nonroot

EXPOSE 9108

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/oci-cpu-shaper-healthcheck"]

ENTRYPOINT ["/usr/local/bin/oci-cpu-shaper"]

FROM gcr.io/distroless/static:latest AS rootful

ARG VERSION
ARG GIT_COMMIT
ARG BUILD_DATE
ARG IMAGE_SOURCE="https://github.com/oci-cpu-shaper/oci-cpu-shaper"

LABEL org.opencontainers.image.title="oci-cpu-shaper" \
      org.opencontainers.image.description="OCI CPU shaper (distroless rootful)" \
      org.opencontainers.image.source="${IMAGE_SOURCE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=builder /out/oci-cpu-shaper /usr/local/bin/oci-cpu-shaper
COPY --from=builder /out/oci-cpu-shaper-healthcheck /usr/local/bin/oci-cpu-shaper-healthcheck
COPY configs/offline-smoke.yaml /etc/oci-cpu-shaper/config.yaml
COPY configs /etc/oci-cpu-shaper/configs

USER 0:0

EXPOSE 9108

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/oci-cpu-shaper-healthcheck"]

ENTRYPOINT ["/usr/local/bin/oci-cpu-shaper"]

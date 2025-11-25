# syntax=docker/dockerfile:1.6

ARG GO_VERSION=1.25.4

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION}-bookworm AS builder

WORKDIR /src

RUN --mount=type=bind,source=go.mod,target=/src/go.mod,readonly \
    --mount=type=bind,source=go.sum,target=/src/go.sum,readonly \
    --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

ARG TARGETOS=linux
ARG TARGETARCH
ARG TARGETVARIANT

ENV CGO_ENABLED=0

RUN --mount=type=bind,source=.,target=/src,readonly \
    --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH:-amd64} GOARM=${TARGETVARIANT#v} \
    go build \
      -trimpath \
      -o /out/oci-cpu-shaper-healthcheck \
      ./cmd/healthcheck

FROM gcr.io/distroless/static:nonroot AS runtime

COPY --from=builder /out/oci-cpu-shaper-healthcheck /usr/local/bin/oci-cpu-shaper-healthcheck

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/oci-cpu-shaper-healthcheck"]

# syntax=docker/dockerfile:1.7
# Development build for the fork: build the Go server from this repository.
# The upstream release uses the root Dockerfile and prebuilt ghcr.io images.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG A9CAM_REPO=https://github.com/R0b1ns/a9-camera-ha-add-on.git
ARG A9CAM_REF=main
# BUILD_VERSION changes with every release. Using it in this RUN invalidates
# Docker's layer cache, otherwise an old clone of main would be rebuilt forever.
ARG BUILD_VERSION=dev
RUN apk add --no-cache git
WORKDIR /src
RUN git clone --depth 1 --branch ${A9CAM_REF} ${A9CAM_REPO} repo \
    && printf '%s\n' "${BUILD_VERSION}" > /src/.a9cam-build-version
WORKDIR /src/repo/server
# BuildKit cache mounts survive a version bump: only changed Go packages are
# recompiled, not the whole dependency tree (~60 s -> a few seconds).
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOARM=${TARGETVARIANT#v} CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w" -o /a9cam .

FROM alpine:3.22
ARG BUILD_ARCH
ARG BUILD_VERSION
LABEL io.hass.type="addon" io.hass.arch="${BUILD_ARCH}" io.hass.version="${BUILD_VERSION}" \
      org.opencontainers.image.source="https://github.com/R0b1ns/a9-camera-ha-add-on"
COPY --from=build /a9cam /usr/bin/a9cam
ENTRYPOINT ["/usr/bin/a9cam"]

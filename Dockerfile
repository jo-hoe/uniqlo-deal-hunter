# syntax=docker/dockerfile:1.7
# Multi-stage build for uniqlo-deal-hunter.
# Final image is distroless static (~15 MB) running as nonroot.
# CGO is disabled because modernc.org/sqlite is a pure-Go driver.

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS build
WORKDIR /src

# Cached module download.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Compile.
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOFLAGS="-trimpath" \
    go build -ldflags="-s -w" -o /out/uniqlo-deal-hunter ./cmd/uniqlo-deal-hunter

# Distroless static: no shell, no package manager, no /tmp, nonroot user.
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/jo-hoe/uniqlo-deal-hunter" \
      org.opencontainers.image.description="Kubernetes-native deal hunter for uniqlo.com/de/en" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/uniqlo-deal-hunter /uniqlo-deal-hunter
USER nonroot:nonroot
ENTRYPOINT ["/uniqlo-deal-hunter"]

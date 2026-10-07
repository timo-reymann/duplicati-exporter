# syntax=docker/dockerfile:1

##
## Stage 1: build the static binary
##
FROM golang:1.26-bookworm AS builder

WORKDIR /src

# Cache module downloads separately from the source tree.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG BUILD_TIME=unknown
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT_REF=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w \
        -X github.com/timo-reymann/duplicati-exporter/internal/buildinfo.Version=${BUILD_VERSION} \
        -X github.com/timo-reymann/duplicati-exporter/internal/buildinfo.GitSha=${BUILD_COMMIT_REF} \
        -X github.com/timo-reymann/duplicati-exporter/internal/buildinfo.BuildTime=${BUILD_TIME}" \
      -o /out/duplicati-exporter \
      ./cmd/duplicati-exporter

##
## Stage 2: licenses only
##
FROM scratch AS license
COPY LICENSE /LICENSE
COPY NOTICE /NOTICE

##
## Stage 3: runtime
##
FROM gcr.io/distroless/static-debian12:nonroot

ARG BUILD_TIME=unknown
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT_REF=unknown

LABEL org.opencontainers.image.title="duplicati-exporter" \
      org.opencontainers.image.description="Prometheus exporter for Duplicati backups" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.vendor="Timo Reymann <mail@timo-reymann.de>" \
      org.opencontainers.image.authors="Timo Reymann <mail@timo-reymann.de>" \
      org.opencontainers.image.url="https://github.com/timo-reymann/duplicati-exporter" \
      org.opencontainers.image.documentation="https://github.com/timo-reymann/duplicati-exporter" \
      org.opencontainers.image.source="https://github.com/timo-reymann/duplicati-exporter.git" \
      org.opencontainers.image.created=${BUILD_TIME} \
      org.opencontainers.image.version=${BUILD_VERSION} \
      org.opencontainers.image.revision=${BUILD_COMMIT_REF}

COPY --from=license / /
COPY --from=builder /out/duplicati-exporter /bin/duplicati-exporter

EXPOSE 9685
USER nonroot:nonroot
ENTRYPOINT ["/bin/duplicati-exporter"]

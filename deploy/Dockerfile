# syntax=docker/dockerfile:1.7
#
# Luna peer image. Single multi-stage build that produces the lunapeer,
# txgen, and secret binaries on a slim Alpine runtime.
#
# The smart-contract VM is intentionally NOT bundled here; it runs as a
# sibling container from ghcr.io/vg-grape/luna-smc and the peer talks to
# it over gRPC. See deploy/docker-compose.yml.

ARG GO_VERSION=1.24.6
ARG ALPINE_VERSION=3.20

# -----------------------------------------------------------------------------
# Builder
# -----------------------------------------------------------------------------
FROM golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache git ca-certificates build-base

WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Bring in the rest of the source.
COPY . .

# Build all entry points. CGO is off so the binaries are static and the
# runtime image stays minimal.
ENV CGO_ENABLED=0 GOOS=linux GOFLAGS="-trimpath"
ARG VERSION=dev
ARG LDFLAGS="-s -w -X github.com/VG-Grape/luna/version.Version=${VERSION}"
RUN go build -ldflags="${LDFLAGS}" -o /out/lunapeer ./cmd/lunapeer && \
    go build -ldflags="${LDFLAGS}" -o /out/txgen    ./cmd/txgen    && \
    go build -ldflags="${LDFLAGS}" -o /out/secret   ./cmd/secret

# -----------------------------------------------------------------------------
# Runtime
# -----------------------------------------------------------------------------
FROM alpine:${ALPINE_VERSION}

RUN apk add --no-cache bash ca-certificates curl openssl tini && \
    addgroup -S luna && adduser -S -G luna -u 1000 luna && \
    mkdir -p /home/luna/.grap3 && \
    chown -R luna:luna /home/luna

# Default config templates. Operators are expected to mount their own
# overrides at /home/luna/.grap3 in production.
COPY --chown=luna:luna config/lunapeer-smc.yml      /home/luna/.grap3/grapepeer.yml
COPY --chown=luna:luna config/txgenerator.yml       /home/luna/.grap3/txgenerator.yml
COPY --chown=luna:luna config/bootstrap.json        /home/luna/.grap3/bootstrap.json
COPY --chown=luna:luna config/grapepk.json          /home/luna/.grap3/grapepk.json

# Binaries.
COPY --from=builder /out/lunapeer /usr/local/bin/lunapeer
COPY --from=builder /out/txgen    /usr/local/bin/txgen
COPY --from=builder /out/secret   /usr/local/bin/secret

COPY --chown=luna:luna deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN sed -i 's/\r$//' /usr/local/bin/entrypoint.sh && chmod +x /usr/local/bin/entrypoint.sh

USER luna
WORKDIR /home/luna

# REST API, gRPC, and libp2p ports. Adjust to your config.
EXPOSE 8010 33331 39399

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]
CMD ["lunapeer"]

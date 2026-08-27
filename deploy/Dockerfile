# syntax=docker/dockerfile:1.7
#
# Grape peer image. Single multi-stage build that produces the grapepeer,
# txgen, and secret binaries on a slim Alpine runtime.
#
# The smart-contract VM is intentionally NOT bundled here; it runs as a
# sibling container from ghcr.io/vg-grape/grape-smc and the peer talks to
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
ARG LDFLAGS="-s -w -X github.com/Grape-Chain/Grape-Dag/version.Version=${VERSION}"
RUN go build -ldflags="${LDFLAGS}" -o /out/grapepeer ./cmd/grapepeer && \
    go build -ldflags="${LDFLAGS}" -o /out/txgen    ./cmd/txgen    && \
    go build -ldflags="${LDFLAGS}" -o /out/secret   ./cmd/secret

# Web wallet: the signer is the same crypto/tx code compiled to WebAssembly, so
# the browser produces exactly the bytes the node validates. Served pre-gzipped
# (~5 MB instead of ~25 MB).
RUN mkdir -p /out/wallet && \
    cp web/wallet/index.html web/wallet/app.js web/wallet/units.js /out/wallet/ && \
    cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" /out/wallet/wasm_exec.js && \
    GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o /out/wallet/wallet.wasm ./cmd/walletwasm && \
    gzip -9 -k /out/wallet/wallet.wasm

# -----------------------------------------------------------------------------
# Runtime
# -----------------------------------------------------------------------------
FROM alpine:${ALPINE_VERSION}

RUN apk add --no-cache bash ca-certificates curl openssl tini && \
    addgroup -S grape && adduser -S -G grape -u 1000 grape && \
    mkdir -p /home/grape/.grap3 && \
    chown -R grape:grape /home/grape

# Default config templates. Operators are expected to mount their own
# overrides at /home/grape/.grap3 in production.
COPY --chown=grape:grape config/grapepeer-smc.yml      /home/grape/.grap3/grapepeer.yml
COPY --chown=grape:grape config/txgenerator.yml       /home/grape/.grap3/txgenerator.yml
COPY --chown=grape:grape config/bootstrap.json        /home/grape/.grap3/bootstrap.json
COPY --chown=grape:grape config/grapepk.json          /home/grape/.grap3/grapepk.json

# Binaries.
# Web wallet assets, served by the peer at /wallet/ (peer.walletdir).
COPY --from=builder --chown=grape:grape /out/wallet /home/grape/wallet

COPY --from=builder /out/grapepeer /usr/local/bin/grapepeer
COPY --from=builder /out/txgen    /usr/local/bin/txgen
COPY --from=builder /out/secret   /usr/local/bin/secret

COPY --chown=grape:grape deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN sed -i 's/\r$//' /usr/local/bin/entrypoint.sh && chmod +x /usr/local/bin/entrypoint.sh

USER grape
WORKDIR /home/grape

# REST API, gRPC, and libp2p ports. Adjust to your config.
EXPOSE 8010 33331 39399

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]
CMD ["grapepeer"]

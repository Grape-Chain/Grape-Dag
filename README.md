# Luna

A DAG-based distributed ledger with EVM-compatible smart contracts.

> **Project status:** early public release. APIs and on-disk formats may
> change before `v1.0.0`. See [CHANGELOG.md](CHANGELOG.md).

## Overview

Luna is an open-source peer-to-peer network that combines:

- **DAG consensus** with MCMC+ weighted random-walk tip selection
- **Deterministic finality** via leader-issued checkpoint ("pin") transactions
- **EVM-compatible smart contracts** executed by a JVM-based VM over gRPC
- **libp2p networking** with GossipSub propagation and DHT/mDNS peer discovery

The peer node, the JVM-based smart-contract VM, a transaction-generation
CLI, and a small key/secret tool are all built from this single repository.
The peer exposes its REST and WebSocket API directly under `services/rest/`
— there is no separate API gateway in this release.

## Quick Start (Docker)

```sh
cp deploy/.env.example deploy/.env   # then edit the values
make compose-up
```

This brings up the full local development stack:

| Service | Count | Description |
|---------|-------|-------------|
| `bootstrap1`, `bootstrap2` | 2 | Bootstrap peers — seed DHT, handle mDNS discovery |
| `leader` | 1 | Genesis node with gRPC (`:50333`) and REST API (`:8010`) |
| `peer1`–`peer5` | 5 | Regular peers; start after genesis seeding completes |
| `smc` | 1 | JVM-based smart-contract VM (gRPC `:29299`) |
| `mongo` | 1 | MongoDB for node stats (`:47329` on host) |
| `txspammer-l` | 1 | One-shot genesis transaction seeder; exits when done |
| `init-wallets` | 1 | One-shot wallet funder; seeds 5 preset addresses |

The leader's REST endpoint listens on `https://localhost:8010`.

A few example calls against the running stack:

```sh
examples/payment.sh        # submit a signed payment
examples/publish_nft.sh    # deploy a sample ERC721-style contract
examples/spammer.sh        # generate sustained transaction load
```

## Build From Source

Requirements: Go 1.24+, JDK 17 + Maven 3.9+ (for the SMC VM), Docker (for
image builds), `make`.

```sh
make build      # Go binaries in ./bin/
make test       # Go unit tests with -race
make lint       # gofmt + go vet
make docker     # build the peer image (deploy/Dockerfile)

# build the SMC VM
cd smc && mvn -T 1C clean install -DskipTests
```

## Repository Layout

```
luna/
├── cmd/
│   ├── lunapeer/        peer node binary
│   ├── txgen/           transaction generator
│   └── secret/          wallet key / secret utility
├── crypto/              cryptographic primitives (Ed25519, hashing, wallet)
├── dag/                 DAG state, tip selection, pin transactions
├── network/             libp2p host, peer registry
├── diffusion/           gossip publish / subscribe
├── peer/                peer lifecycle and identity
├── statemachine/        node-lifecycle FSM (bootstrap → syncing → ready)
├── services/            REST and gRPC services exposed by the peer
├── tx/                  transaction types and encoding
├── vm/                  smart-contract VM client (gRPC to luna-smc)
├── smc/                 JVM-based smart-contract VM (Maven multi-module)
├── examples/            ready-to-run demo scripts
├── deploy/              Dockerfile, docker-compose, sample env
└── config/              default YAML configuration
```

## Configuration

Default configuration lives under `config/`. Operators are expected to
override values via environment variables or a local override file
(`config.local.yml`, gitignored). Sensitive values — database passwords,
API auth tokens, TLS keys — must come from environment variables or a
secret manager. The repository ships **no production credentials**.

See `deploy/.env.example` for the required environment variables.

## Documentation

- REST and WebSocket API reference: [`services/rest/api/openapi.yml`](services/rest/api/openapi.yml)
- Network setup notes: [`network/readme.md`](network/readme.md)
- Database setup: [`db/postgres/README.md`](db/postgres/README.md)
- Smart-contract VM: [`smc/README.md`](smc/README.md)

A consolidated `docs/` tree (architecture, operations guide,
contributor design notes) lands in a future release.

## Contributing

Pull requests are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md)
for the workflow, coding standards, and Developer Certificate of Origin
sign-off requirement.

## Security

To report a vulnerability, see [SECURITY.md](SECURITY.md). Do not open a
public issue.

## License

Apache License 2.0. See [LICENSE](LICENSE).

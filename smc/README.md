# Grape SMC

Smart Contract Virtual Machine for the Grape DAG blockchain. Implements an
EVM-compatible interpreter and exposes it as a gRPC service that the peer
(`cmd/grapepeer`) calls into to execute transactions.

## Modules

| Module         | Purpose                                                          |
|----------------|------------------------------------------------------------------|
| `grape1utils`   | Shared low-level helpers (Bytes, HexUtils, JsonUtils, FileUtils) |
| `grap3-crypto` | Cryptographic primitives (DSA, hashing, key derivation)          |
| `grap3-ether`  | Ethereum-compatible crypto (secp256k1, BLS12-381, RLP)           |
| `commons`      | Shared VM model: math (UInt256/Word256), config, BCEI interface  |
| `l1vm`         | The interpreter — opcode tables, gas, stack machine, precompiles |
| `vm`           | High-level VM façade consumed by `server`                        |
| `server`       | gRPC entry point. Listens on `:29299` (calls), `:39399` (state)  |
| `grapech`       | Java client SDK plus shared `vm.proto` / `txvX.proto`            |

## Build

Requires JDK 17 and Maven 3.9+.

```bash
# from this directory
mvn -T 1C clean install -DskipTests
java -jar server/target/server-*.jar
```

The server listens on port `29299` for incoming smart-contract calls (gRPC)
and connects out to a peer-side state server on port `39399`.

Note: there is also a Go package at `../smc/` (`pool.go`, `store.go`) — that's
the Go-side queue/cache that talks to *this* VM over gRPC. They share a
directory but build independently.

## Container

```bash
docker build -t grape-smc:dev -f Dockerfile .
docker run --rm -p 29299:29299 -p 39399:39399 grape-smc:dev
```

The full peer + VM + Mongo stack is wired up in `../deploy/docker-compose.yml`.

## License

Apache-2.0.

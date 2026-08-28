# Setting up a development machine

Covers macOS (Apple Silicon or Intel) and Linux. Two commands do most of it:

```bash
make setup                         # what is missing, and the command that fixes it
SETUP_ARGS=--verify make setup     # then build everything and run the tests
make localnet-measure              # a two-node network, loaded, measured
```

The rest of this page is what those commands check, and why each check exists.

## Why the machine's size matters more than it looks

Agent fan-out is capped at `min(16, cores - 2)`. On a four-core box that is **two**
concurrent agents, whatever number is requested — a thirty-agent survey runs as
fifteen sequential rounds and takes over an hour. On a twenty-core machine the cap
is the full sixteen and the same survey finishes in ten to fifteen minutes.

`make setup` prints the number for the machine it runs on, because it is the
figure that decides whether a large fan-out is worth starting.

Memory matters for a different reason: a saturating node holds a bounded working
set but the retain window and the archive are sized in transactions, so a long run
at a few thousand a second is measured in gigabytes. 16 GiB is enough for a
five-minute run; a soak wants more.

## What has to be installed

| | Why |
|---|---|
| **Go 1.24.6+** | The node. There is no cgo anywhere in the tree and no architecture-specific files, so it builds natively on Apple Silicon with nothing extra. |
| **JDK 17** | The contract VM under `smc/`. Seventeen specifically: `smc/pom.xml` pins source and target to 17, and Lombok 1.18.26 does not run on 21 or later. |
| **Maven** | Builds `smc/`. |

```bash
brew install go maven
brew install --cask temurin@17
export JAVA_HOME=$(/usr/libexec/java_home -v 17)
```

## The one real Apple Silicon risk

`smc/grap3-ether` depends on Besu's `bls12-381` through JNA. That is a wrapper
around a **compiled native library**, and native code is per-architecture. If the
jar carries no `darwin-aarch64` build, the JVM fails at class load on an M-series
Mac.

`make setup` checks the jar once `smc/` has been built, and names the platforms it
actually bundles. It matters less than it sounds — BLS12-381 precompiles are not on
Ethereum mainnet, so a node that never executes one is unaffected — but it fails at
load rather than at first use, so it is worth knowing before it surprises anyone. If
the arm64 build is absent, run the JVM under Docker (`linux/amd64`) and keep the Go
node native.

## Raise the open-file limit

A node opens a libp2p connection per peer, and the saturation benchmark opens 48
gRPC connections of its own. macOS ships a low soft limit and the failure looks
like random dial errors rather than like a limit:

```bash
ulimit -n 65536      # add it to ~/.zshrc to make it stick
```

## Running a local network

`scripts/localnet.sh` provisions and runs a leader plus one peer. It works the same
on macOS and Linux — an earlier version of this harness used `setsid`,
`/proc/loadavg` and `free(1)`, none of which exist on macOS.

```bash
./scripts/localnet.sh up              # provision if needed, start both nodes
./scripts/localnet.sh load 300s 48    # offer load as fast as it is taken
./scripts/localnet.sh rate 55 5       # five 55-second windows
./scripts/localnet.sh prof 45         # profiles, taken UNDER load
./scripts/localnet.sh down
./scripts/localnet.sh measure         # all of the above, in the right order
```

Everything lands in `.localnet/`, which is gitignored — it holds generated node
private keys.

### Three things the script does that are not obvious

**It provisions through `grapepeer join`.** Not by hand-writing yaml, so the real
onboarding path gets exercised on every run instead of rotting untested. The wallet
is never overwritten, so restarting keeps the same identities and balances.

**Node 0 runs as the genesis wallet.** Genesis funding flows from the
chain-creating node's own dag wallet out to the exodus wallets, so a node started
with `-genesis` while holding a freshly generated identity funds nobody: every
exodus payment is drawn on an empty account and the benchmark fails at setup with
`Balance for wallet ... not found`. `join` is right to generate a fresh identity —
it provisions a node to *join* a chain. Creating one is a different job. The script
substitutes the genesis keys, taken from `config/txgenerator-t2.yml` so the leader
and the generator cannot disagree about which account is the faucet.

**It enables gRPC by flag, not by config.** `join` writes `grpc: false`, which is
the correct default: that port publishes transactions with nothing in front of it.
The benchmark needs it, so the script passes `-grpc` rather than weakening the
template every node is provisioned from.

### A leader with no peers will not finish starting

`up solo` exists to reproduce this, not to measure with. `RunSynchronization` does
not return when there are no peers, and `RunRoboTraderService` is called after it —
so a lone leader looks alive and serves nothing at all. Measure with a peer.

## Reading the measurements

```
t=  40s ingress= 2801/s inserted= 2133/s confirmed= 2118/s live=  412 pubq=32800 rss= 921MB load=5.88
```

- **ingress / inserted / confirmed** — the three agreeing is what distinguishes
  throughput from buffering. Ingress alone can run ahead while the publish queue
  fills; once that queue reaches its ceiling the enqueue blocks and the three
  converge. That convergence is the reading to trust.
- **pubq** — publish-queue depth. Pinned at its ceiling means the single publisher
  goroutine is the limit, which is the current bottleneck. See
  `docs/benchmarking.md`.
- **load** — recorded beside every sample on purpose. A rate taken while the
  machine is three times oversubscribed is a rate for a machine that is three times
  oversubscribed, and without the number written down next to it there is no
  telling that reading from a real regression later. Two of this project's runs had
  to be thrown away for exactly that reason.

**Do not publish throughput figures measured on a Mac.** Apple Silicon performance
cores flatter the number considerably against the x86 reference box the target is
written against. Use the Mac for development and agents; take the number for the
record on reference hardware.

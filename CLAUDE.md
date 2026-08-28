# Grape-Dag

A DAG distributed ledger implementing the VINE design (see `Grape__Techpaper_V4.pdf`).
Go node, plus a JVM smart-contract VM under `smc/`.

## The shape of it

**Sites.** Every payment is wrapped in a *site* (`dag.Node`, wire type `pb.Node`) that
approves two earlier sites. That graph is the DAG.

**Commit transactions**, called *pins* in the code (`pb.TxPin`, `dag/pin.go`). A linear,
hash-linked chain, one roughly every 5 seconds. Each carries the sites it settled, the
balances as of that point, and executed contract results. **This chain is the ledger.**
The DAG in front of it is provisional.

**Confirmation** (`dag/confirmtracker.go`). A site is confirmed when the share of current
tips that reach it, directly or indirectly, crosses `dag.confirmshare` (permille, default
667). A *tip* is a site nothing approves. This is a documented departure from the paper's
literal 100% — see `docs/confirmation.md`.

**Slicing.** Settled sites leave the live graph and go to the archive, which is what keeps
memory bounded. `grape_live_sites` staying in the hundreds under load is slicing working.

**Consensus** (`dag/consensus.go`). `dag.consensus` is `"leader"` (default) or `"quorum"`.
Quorum is t0–t4 with `q = ⌊2N/3⌋+1` and deterministic proposer rotation.

## Invariants — breaking one of these is a consensus bug, not a test failure

1. **Anything hashed or signed must be byte-reproducible on every node.** Protobuf map
   ordering is nondeterministic in Go: use `proto.MarshalOptions{Deterministic: true}`
   for any message containing a map. `Deterministic` does **not** sort repeated fields —
   sort those yourself.
2. **No floating point in a consensus value.** `float64` results depend on operation order.
   Reward and fee arithmetic is integer-only (`dag/rewards.go`) on purpose. *Known
   violation:* `pb.Node.cumWeight`/`txWeight` are floats inside `PrototypeHash`, and
   `txWeight` is a fresh random draw per site — so no validator can currently rebuild a
   pin and byte-compare it. `dag/attribution.go` already excludes both from the processor
   signature for this reason.
3. **Lock order is `dag.mux` → everything else.** Never take `dag.mux` while holding the
   pin lock. `dag.mux` is a `sync.RWMutex`; `rlock()`/`runlock()` are the shared half.
   **A shared section must not write anything `dag.mux` protects, and must not take
   `dag.mux` again** — `RWMutex` deadlocks on a recursive `RLock` once a writer queues.
   The six existing read sections are audited and listed above `rlock` in `dag/dag.go`.
4. **A settled site must never be settled twice**, and a released body must never make a
   settled site unknown. The archive's `Has`/`PinOf` are unbounded; only bodies are
   evicted.

## Testing standard

`go test -race ./...` must be green. Beyond that: **for anything on the consensus, money
or confirmation path, mutation-test the change.** Break the code deliberately and confirm
a test fails. Several tests in this repo passed for the wrong reason until that was done —
one asserted a metric existed rather than that it increased; one produced an error from a
signature mismatch while claiming to cover a hash failure.

A benchmark fixture that builds a chain instead of a frontier reports a flat, fast,
meaningless number. `dag/confirmtracker_test.go`'s `reportFrontier` fails the benchmark if
the frontier collapses — keep that habit.

## Measuring

```bash
make setup                  # what this machine is missing
make localnet-measure       # provision, load, measure
```

Read `docs/benchmarking.md` before quoting a number. In short: ingress, insertion and
confirmation agreeing is what distinguishes throughput from buffering; the generator's
own "stalls" line above 1% means you measured the generator; and the load average is
recorded beside every sample because two runs had to be discarded for want of it.

Current ceiling ≈ **2,700 payments/second** on 4 vCPU. The limit is the **single publisher
goroutine** — `grape_queue_depth{queue="publish"}` sits pinned at its 32,768 ceiling — not
CPU (1.26 of 4 cores used) and not lock contention (`AddTxDag` is 1.97% of mutex delay,
down from 63.54%).

## Current work: replacing the contract VM with go-ethereum's EVM

Decided. See `docs/evm-migration.md` for the plan and the evidence behind it.

The one-paragraph version: `smc/` is a **hand-written** Java EVM (not Besu's, not
Burrow's — Besu appears only as crypto primitives) with no conformance suite, a gas
schedule matching no Ethereum fork, `CREATE2` priced at 2 gas, a `SELFDESTRUCT` that does
not delete, and Cancun bytecode silently mis-decoded as withdrawn `JUMPSUB`. It is being
replaced by geth's `core/vm`, which is already a dependency.

**Do not import `core/state` or `trie` from geth v1.12.0** — they pull `ethdb/pebble`,
which does not compile against this project's Pebble v1.1.5. `vm.StateDB` is an interface;
implement it over the existing store in `vm/server.go`, which already has the right shape.

## Gotchas that have cost real time

- **`grapepeer` needs a config file.** `grapepeer join` writes one. A node with none used
  to segfault; it now says so.
- **A leader with no peers never finishes starting.** `RunSynchronization` does not return
  without peers, and the gRPC service starts after it — so a lone node looks alive and
  serves nothing.
- **A node started with `-genesis` must *be* the genesis wallet.** Genesis funding flows
  from the creating node's own dag wallet, so a freshly generated identity funds nobody.
- **The REST API's refusal to start without credentials is fatal.** Load the node's
  `api-credentials.env` or it opens its transaction port and then exits.
- **`peer.visualize` off by default is deliberate.** With it on, every commit writes a new
  `./dag.graph.N.gv` and renders the whole graph.
- **Contract calls never enter the DAG.** They go to a local side pool that is not
  gossiped, so a call submitted to a non-leader never executes — while the client already
  got `Successful: true`.

## Conventions

- Comments explain **why**, especially why an alternative was rejected. Match the
  surrounding density; this codebase comments heavily and deliberately.
- Commit messages state what was wrong, how it was found, and what the fix costs.
- No secrets in the repo. `config/txgenerator-t2.yml` and the genesis wallets in
  `vm/genesis.go` contain committed testnet keys — pre-existing, local-only, and the
  faucet they unlock holds the opening supply of whatever chain it is pointed at.

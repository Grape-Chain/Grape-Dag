# Work breakdown

The durable version of the task list. Session task lists do not survive a session, and
this work spans several; this file is the handoff.

Read `CLAUDE.md` first for the invariants, then `docs/evm-migration.md` for the decision
behind Phase 0 and 1.

## What the geth decision cancels

Worth stating before the list, because roughly half of the previously-identified defect
inventory is now **wasted work**. The Java EVM under `smc/` is being replaced, so anything
that only makes *it* correct is effort spent on code that is going to be deleted.

**Do not fix** — these die with the swap:

- The gas schedule (`CREATE2` at 2 gas, EIP-2929 surcharges stacked on Frontier bases,
  `maxCodeSize` 49152, flat code-deposit charge)
- The opcode table (`BEGINSUB`/`RETURNSUB`/`JUMPSUB` at `0x5c`–`0x5e`, `TLOAD`/`TSTORE` at
  the obsolete `0xb3`/`0xb4`, missing `MCOPY`)
- The six `StateServerDei` stubs (`suicide`, `clearContractStorage`, `hasSuicided`, …)
- `getCommittedContractStorage` returning live storage, breaking EIP-2200 pricing
- The JVM's orphaned checkpoints (`begin`/`endStateTransaction` with no `try/finally`)
- The JVM's per-slot gRPC hot loop and `BigInteger` allocation per opcode
- Re-enabling `smc/`'s excluded integration tests

**Do fix** — these are Go-side or outlive the engine entirely. That is the list below.

## Phase 0 — before the swap

Small, and their absence makes everything after them unverifiable.

**0.1 Contract-state recovery from stored pins.** *(highest value per effort in the whole
plan)*
Today every deployed contract reports empty code and zero storage after any node restart,
while its balance still looks correct — silent loss. `dag/recovery.go` does not import
`vm` at all, `NewStorage()` is always fresh, and recovery runs *before* the state server
exists. The data needed to rebuild is already on disk inside `pin.Diffs` and `pin.SmcTxs`;
nothing replays it.
Files: `dag/recovery.go`, `store/`, `run/run.go` (start ordering).

**0.2 An `ethereum/tests` conformance harness.**
Build it *before* the swap, so the new engine's correctness is measured rather than
assumed and the old one's divergence is quantified. This is the single item that turns "a
hand-written EVM with no conformance suite" from a permanent liability into a number.
Files: new package, e.g. `evmconf/`.

**0.3 Stop discarding the re-execution verdict.**
`runSmartContractStageFullNode` re-executes every contract tx in a received pin and
compares fuel, status, output and logs — on the **default** node type — and its error
return is discarded at the call site. A node that proves the proposer lied still appends
the pin. Same for `checkBalances`.
Files: `dag/syncpin.go`, `dag/pin.go` call sites.

**0.4 Guard the proposer's double execution.**
In quorum mode the winning proposer runs `keepCandidate()` and then `applyPin()` on its
own pin, executing the same transactions a second time against already-advanced state. A
`counter++` becomes `counter += 2` on the proposer, and the receipt comparison cannot
catch it — identical gas, status, output and logs.
Files: `dag/consensusnet.go`.

## Phase 1 — the swap

**1.1 Implement `vm.StateDB` over the existing store.** The seam. `vm/server.go` already
has `putAccount`/`putValue`/`checkpoint`/`revert`, which is close to the right shape.
**Do not import geth's `core/state` or `trie`** — see `docs/evm-migration.md`.

**1.2 Route `RunCall`/`RunCode`/`EstimateGas` through geth's `core/vm`** at Shanghai
rules, behind a config flag, with the JVM path still selectable until 0.2 passes on the
new engine.

**1.3 Add `accounts/abi`.** Already a dependency, never imported. This is what lets
anything read an ERC-20's balance or decode its events — the actual prerequisite for
tokens.

**1.4 Deploy a stock, unmodified ERC-20 and drive it from `cast`/Hardhat.** The proof the
rest landed.

## Phase 2 — wallets and tooling

**2.1 A real chain ID, decoupled from `ChainType`.** Currently only 1, 2 and 3 are
accepted and `GetChainType()` **panics** on anything else — reachable from the ingest path
with one crafted transaction. So the chain must impersonate mainnet or Ropsten, which
inverts EIP-155 replay protection and means MetaMask refuses to add it. Touches the tx
schema and every network-equality check.

**2.2 EIP-2718 typed envelopes** (1559/2930). `rlp.DecodeBytes` cannot consume
`0x02‖rlp(...)`. Survives today only because no `baseFeePerGas` is advertised, so clients
fall back to legacy.

**2.3 A durable tx/receipt/log index.** Pin bodies are released after the retain window,
so older blocks silently return empty transaction arrays and unfindable receipts, and
lookup by hash is a linear rescan. This is the gate for `eth_getLogs`, which is the gate
for every wallet, indexer and explorer.

**2.4 The remaining RPC surface**: `eth_getLogs`, filters, `eth_subscribe`,
`eth_getBlockByHash`, `eth_feeHistory`, batching, correct JSON-RPC error codes, a 256-byte
`logsBloom` (currently 32, so strict formatters in ethers.js and web3.py throw), a real
`receiptsRoot`, and `gasUsed` that is not always equal to `gasLimit`.

## Phase 3 — determinism and verification

In this order; nothing later is expressible before the first item.

**3.1 Floats out of the hashed payload.** `pb.Node.cumWeight`/`txWeight` are floats inside
`PrototypeHash`, and `txWeight` is a fresh random draw per site. No validator can rebuild
a pin and byte-compare while that is true. `dag/attribution.go` already excludes both from
the processor signature for exactly this reason.

**3.2 Canonical ordering.** Sort `pin.Diffs` and receipt logs (`Deterministic: true` does
not sort repeated fields). Sort contract-tx candidates by (sender, nonce, tx hash) *before*
the greedy filter — `smc/pool.go` currently ranges a Go map, so nodes select different
*sets*, not merely different orders. Replace the `Node.time` sort in `dag/pin.go` with the
site-uuid sort the consensus layer already uses.

**3.3 Gossip the contract mempool.** Contract calls never enter the DAG — they go to a
local side pool that is not diffused, so a call submitted to a non-proposer never executes
while the client already received `Successful: true`.

**3.4 Rebuild-and-compare voting.** `onProposal` checks proposer identity, epoch, round,
pin number and per-site justification — and never reads `SmcTxs`, `Diffs`, `Balance` or
`Nodes`, then signs a hash covering all of them.

## Phase 4 — state root

A `StateDB` over a real trie, root in the pin, validators checking it. Precondition for
light clients, proofs and any trust-minimised bridge. Do not start before Phase 3.

## Carried over, not EVM work

- **Fee activation.** `docs/economics.md` documents five blockers to clear before setting
  `tx.feestartpin`, and awaits two decisions: whether 5× is the right stake tilt, and
  whether rounding remainders are banked or burned.
- **A leader with no peers never finishes starting.** `RunSynchronization` does not return
  without peers and the gRPC service starts after it, so a lone node looks alive and
  serves nothing. Matters for one-click onboarding when a wallet user's bootstrap peers
  are all unreachable.
- **The publisher is single-threaded.** The measured ~2,700 payments/second ceiling is one
  goroutine doing sign, marshal and publish per transaction. Fanning it out needs
  `AddTxDag` safe for concurrent callers first. See `docs/benchmarking.md`.
- **Licensing.** `smc/` carries geth's verbatim comments and test-vector format with no
  attribution while declaring Apache-2.0 in its pom and Proprietary at the repo root.
  Needs an IP opinion, not an engineering fix — and it resolves either way once `smc/` is
  deleted.

## Running this with many agents

The hard-won lesson from doing this with four: **give every agent strictly disjoint file
ownership, in writing, in its prompt.** Sixteen agents in `dag/consensus.go` is a merge
conflict, not parallelism.

What parallelises well here: conformance triage (thousands of independent fixtures — one
agent per opcode family), the RPC methods (each independent), the Phase 2 index work,
writing tests, mutation-testing each change.

What does not: anything touching consensus ordering (Phase 3 is shared mutable core — do
it with one or two agents and review carefully), and the state-model decision, which is a
design call that has to be made once before 1.1 begins.

Two further cautions learned the hard way:

- **Run mutation harnesses in a git worktree.** An agent that edits source in place for
  forty seconds per mutation will break every other agent's test run, and the failures
  look like real regressions in unrelated packages.
- **Watch the load average.** Four agents took a four-core box to loadavg 27, which
  corrupted two measurement runs and made a test hang for ten minutes. Measurement and
  heavy fan-out cannot share a machine.

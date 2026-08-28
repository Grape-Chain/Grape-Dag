# Replacing the contract VM with go-ethereum's EVM

Decided. This page is the plan, the evidence behind it, and the one design question
that has to be answered before anyone writes code.

## Why

`smc/` is a **hand-written Java EVM interpreter**, closely modelled on go-ethereum's
`core/vm` — geth's function names, geth's idioms, geth's verbatim source comments,
geth's test-vector format. It is not Besu's EVM and not Burrow's: Besu appears only as
crypto primitives inside precompiles (altbn128, blake2bf, BLS12-381 via JNA), and web3j
is exactly one call, contract-address derivation.

It works. 213 interpreter tests pass, 22 more drive real compiled Solidity through the
full path, deployment is wired and default-on, and contract addresses match what Ethereum
tooling predicts — CREATE and CREATE2 derivation were independently reproduced against
go-ethereum, including an official EIP-1014 vector.

It is also carrying defects that a hand-written EVM with no conformance suite will keep
generating:

| | |
|---|---|
| No `ethereum/tests` harness anywhere | Correctness is owned, not inherited |
| Gas schedule matches no Ethereum fork | Frontier-era base costs with EIP-2929 stacked additively |
| `CREATE2` costs **2 gas**, not 32,000 | Opcodes with no `chain.json` entry silently fall back to base = 2 |
| `SELFDESTRUCT` drains the balance and does not delete | And `hasSuicided` always returns false, so it can be repeated |
| `0x5c`/`0x5d`/`0x5e` are `BEGINSUB`/`RETURNSUB`/`JUMPSUB` | The **withdrawn** EIP-2315. Cancun assigns those bytes to `TLOAD`/`TSTORE`/`MCOPY`, and solc ≥ 0.8.24 targets Cancun by default — so `MCOPY` dispatches into `opJumpSub`, which pops a stack word and **jumps** |
| Six production state operations are stubs | `putAccount`, `deleteAccount`, `clearContractStorage`, `hasSuicided`, `dumpState`, `suicide` — while the in-memory test double implements them correctly |
| Integration tests excluded from the default build | `**/*IT*` is filtered out of surefire with no failsafe plugin |
| No Merkle-Patricia trie, no RLP, no state root | `storageRoot` is a hardcoded empty array; the RPC returns a constant `stateRoot` |

Separately, and needing a legal opinion rather than an engineering one: `smc/` contains
geth's verbatim comments and test-vector format with **no attribution**, while geth is
LGPL-3.0/GPL-3.0. `smc/pom.xml` declares Apache-2.0 and the root `LICENSE` declares
Proprietary. Git history for the directory is one squashed commit whose author names do
not match the committer.

## Does the DAG still process the tokens? Yes

go-ethereum is a monolith of separable parts. This migration takes the **execution
engine** and none of Ethereum's consensus or networking:

**Taken:** `core/vm` (the interpreter), `params` (fork rules), `accounts/abi` (the ABI
codec this project currently lacks entirely).

**Not taken:** `consensus/`, `eth/`, block production, the mempool, syncing.

The EVM is a pure function: state plus transaction in, new state plus receipt and logs
out. It has no opinion about how the transaction came to be ordered. Ethereum feeds it
from blocks produced by proof-of-stake; here it is fed from commit transactions produced
by the DAG.

So the DAG keeps every job it has: tip selection, the confirmation-share rule, slicing,
quorum consensus, the pin chain, processor attribution and fees, and ~2,700 payments a
second. What it gains is a job it is already half doing — **being the sequencer.** A pin
already carries `pin.Nodes` (payment sites) and `pin.SmcTxs` (contract calls), and the
eth-RPC layer already presents the pair as one block with
`transactionIndex = len(pin.Nodes) + smcTxIdx`. The total order already exists in the data
structure; it is produced nondeterministically and never verified, which is the work.

**Expectation to set:** contract execution serialises by nature — Ethereum executes a
block's transactions one after another too. Token throughput will be an order of magnitude
below payment throughput whichever EVM is used. The DAG's parallelism is what makes
payments fast.

## What was verified before committing to this

Run against this repository, not assumed:

- **geth v1.12.0 is already a direct dependency**, already importing `crypto`, `common`,
  `rlp` and `core/types`. Adding `core/vm` is using more of a library that is already
  linked — so the LGPL exposure exists today and does not increase.
- **`core/vm` + `params` + `accounts/abi` compile cleanly** against this project's current
  dependency set.
- **`core/state` and `trie` do NOT.** They pull `ethdb/pebble`, which does not compile
  against this project's Pebble v1.1.5 — three API mismatches. **Do not import them.**
- **`vm.StateDB` is an interface**, so the existing store implements it and geth's
  state/trie/ethdb are never needed. The conflict is avoided rather than fought.
- **The build is pure Go** (`CGO_ENABLED=0` succeeds), so no C toolchain and a clean
  native build on Apple Silicon.
- **geth v1.12.0 is Shanghai**: `PUSH0` present, Cancun absent, `0x5e` **undefined**. So
  Cancun bytecode fails cleanly as an invalid opcode rather than jumping to
  attacker-influenced data — strictly safer than the current behaviour. Shanghai is also
  the fork target the assessment recommended independently.

## The design question to answer first: one state model or two?

Payments live in a flat `address → big.Int` balance map, mirrored in four places and
carried in `pin.Balance`/`pin.Diffs`. geth's EVM wants a `vm.StateDB` with accounts,
nonces, code and storage slots.

There is already one path where the two touch: the VM debits gas, writes it through the Go
state store, and it is folded into `pin.Balance`/`pin.Diffs` and applied by
`settled.applyPin`.

**Option A — one state.** Payments also go through the `StateDB` implementation. Cleaner,
one source of truth, and the only version that can ever produce a single state root over
the whole ledger. Bigger change: it touches the four mirrored balance maps and the fee and
reward code.

**Option B — two states, kept consistent.** Faster to reach, and a permanent source of
divergence bugs — exactly the class of bug this chain currently cannot detect.

**Recommendation: A**, and scope it before writing code. B's speed is borrowed against a
category of defect that is undetectable without a state root, which is the thing A is
required for anyway.

## Sequence

**Phase 0 — before any EVM work.** These are small and their absence makes everything
after them untestable.
1. Contract-state recovery from stored pins. Today every deployed contract reports empty
   code and zero storage after any restart, while its balance still looks correct. The
   data to rebuild is already on disk in `pin.Diffs`/`pin.SmcTxs`; nothing replays it.
2. Stand up an `ethereum/tests` conformance harness. Do this **before** the swap so the
   new engine's correctness is measured rather than assumed, and so the old one's
   divergence is quantified.
3. Turn the excluded integration tests back on.

**Phase 1 — the swap.** Implement `vm.StateDB` over the existing store; route
`RunCall`/`RunCode`/`EstimateGas` through geth's `core/vm` at Shanghai rules; keep the
JVM path behind a config flag until the conformance suite passes on the new one. Add
`accounts/abi`.

**Phase 2 — wallets and tooling.** Real chain ID decoupled from the `ChainType` enum
(currently only 1, 2 and 3 are accepted and `GetChainType()` **panics** otherwise, so the
chain has to impersonate mainnet); EIP-2718 typed envelopes; `baseFeePerGas`; a durable
tx/receipt/log index, which is the gate for `eth_getLogs`, which is the gate for every
wallet, indexer and explorer.

**Phase 3 — determinism and verification.** Floats out of the hashed payload; canonical
ordering of diffs and of contract selection; gossip the contract mempool; then
rebuild-and-compare voting. Nothing here is expressible while a per-site random float sits
inside the signed bytes.

**Phase 4 — state root.** Now affordable: a `StateDB` over a real trie, with the root in
the pin and validators checking it. This is what light clients, proofs and any
trust-minimised bridge need.

## The biggest risk, stated plainly

Not the swap. It is that **no node's execution result is currently checked against
another's**: the re-execution comparison exists and its error return is discarded, peer
nodes copy the proposer's diffs verbatim, the diff list is nondeterministically ordered,
there is no state root, and the signed payload contains a random float. Divergence is not
merely possible — it is undetectable. Replacing the engine removes the largest *source* of
divergence; Phase 3 is what makes any remaining divergence visible.

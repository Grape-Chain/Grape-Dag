# Live network validation

Notes from running the fixes and the web wallet against a real two-node network,
rather than only against unit tests. Everything below was observed on a running
chain; the commands are here so it can be repeated.

## Why not docker compose

`make compose-up` is the documented way to get a local stack. It needs to pull
base images, so it only works where the environment allows reaching a container
registry. Where it does not, two peers run fine as plain processes — that is
what this document describes. The one thing a native run cannot cover is the
JVM smart-contract VM, so payments are exercised and contract execution is not.

## Two peers by hand

A single peer blocks at startup waiting to discover another one (`-single` is
parsed but unused), so a live check needs at least two.

```sh
make build
TN=/tmp/grapetn; mkdir -p $TN/nodeA/.grap3 $TN/nodeB/.grap3 $TN/logs
```

Give each node its own `HOME` (config lives in `~/.grap3`), its own libp2p port,
its own API port, and — for A, which is the leader — a fixed port so B can dial
it. Both configs are `config/grap3peer-local.yml` with:

```yaml
peer:
  port: 43431          # 43432 for node B
  apiport: 8010        # 8011 for node B
  grpcport: 50333      # 50334 for node B
  apitlsenabled: false
  apiauthdisabled: true   # local development only
  walletdir: /path/to/repo/web/wallet
dag:
  faucetwallet: 0xd09ec4a81cde61b57de012d3fe80beae3f28fb68
  faucetpublickey: <genesis public key>
  faucetprivatekey: <genesis private key>
```

Start the leader, then point B at it:

```sh
HOME=$TN/nodeA ./bin/grapepeer -id bspeer1 -bootstrap -leader -genesis -node_type 0 -purge &
# take A's peer id from its log, then:
cat > $TN/nodeB/.grap3/bootstrap.json <<EOF
{ "nodeA": "/ip4/127.0.0.1/tcp/43431/p2p/<A peer id>" }
EOF
HOME=$TN/nodeB ./bin/grapepeer -id peer1 -node_type 0 -purge &
```

Pins only form while transactions are arriving, so drive load with
`HOME=$TN/nodeA ./bin/txgen -mode trader -grpc_port 50333`.

## What the live run showed

**Pin numbering.** Both nodes report pin 0 (genesis) at rest, then 1, 2, 3 …
monotonically as load arrives. Before the fix the genesis pin and the first
formed pin were both numbered 0, and the peer logged a gap on the leader's very
first announce. There are now zero "Gap detected" lines on a peer that has been
connected from the start.

**Gap recovery.** Freezing a peer (`kill -STOP`), advancing the leader, then
resuming it and letting a new pin be announced exercises the catch-up path:

```
[Gap detected] Our current latest pin=3, but got pin=13 from leader
Received pin=5 … Received pin=13        # nine distinct pins in one batch
[Gap detected] Processed downloaded pin at height=5 … height=13
[Gap detected] Processed pins up to 13 height
```

Both nodes then converge on the same height. This path had three separate
defects, all of which had to be fixed before any of it worked:

1. the receive loop treated the channel's `ok` value as "closed" and stopped on
   the first pin;
2. the loop ran on the sync subscriber goroutine, which is also the only
   goroutine that dispatches the leader's response into the channel it was
   waiting on — so the batch could never arrive and the node stalled for the
   full 120s timeout;
3. the batch reader never advanced its offset, so a batch of N pins was read as
   N copies of the first pin. The node advanced one pin per round trip and
   logged the rest as out of order.

Only the first two were found by review; the third was only visible on a real
network, which is the point of this exercise.

**Supply conservation.** `txgen -mode balance` sums every known wallet against
the initial offering. After ~15 pins, several hundred payments, a stall and
catch-up cycle, and the wallet's own payment:

```
Successfully received 603 wallets
Initial coin offering: $10000000000000000000000000000000
total balance for all wallets: $10000000000000000000000000000000
<<< SUCCESS >>>
```

No value created or destroyed. No duplicate transactions were reported by the
query layer, which is where a site pinned twice would surface.

**No crashes.** Zero panics, zero fatal errors and zero race reports across
both nodes for the whole run.

## The wallet against a live node

```sh
make wallet
node scripts/wallet_live.mjs http://localhost:8010
```

This runs the real `wallet.wasm` (the node's own signing code) against the
running peer: fetches the wallet's assets over HTTP, creates an account, pulls
from the faucet, waits for the funds to be confirmed in a pin, signs a payment,
submits it, and waits for both sides of the transfer to settle.

Observed: the faucet credited 1,000 GRAPE; a 7.25 GRAPE payment was accepted
(`executionStatus: SUCCESSFUL`); the recipient was credited exactly 7.25 and the
sender debited exactly 7.25; the payment appeared in history with the right
amount; and `GET /transactions` and `GET /accounts` with no query parameters
returned 200 rather than dereferencing a nil pointer.

## Known rough edges seen while running

Not regressions — pre-existing, and worth knowing:

- The peer logs a few messages with unexpanded format strings
  (`id=%s<uuid>`, `as %!d(string=…)`) on the balance-snapshot path.
- `Cannot change state SYNC_DISPATCH_END -> SYNC_HANDLE_END` appears once per
  snapshot sync: the subscriber drives a transition the table does not allow.
  Harmless today because the handler runs regardless.
- A peer that is behind re-requests missing sites once a second with no backoff,
  and those responses are dropped when the requester has already deleted its
  tracking state, so the request repeats.
- `/accounts` sums to slightly less than the pin-backed total: it reads the VM
  state mirror, while `/accounts/{id}` prefers the pin balance. The two balance
  sources do not always agree.
- The faucet response reports `chainId: TESTNET0` where `/network-info` reports
  `TESTNET2`, and `TESTNET0` is not in the generated enum.

## Confirmation by tip share (technical paper section 5.1)

The two-approver rule was replaced by the paper's actual definition: a site is
confirmed when every current tip confirms it, directly or indirectly. Selected
with `dag.confirmation` (`share100`, the default, or `legacy` for the old rule).

Checked the same way — against the definition, then against a live network.

`dag/confirmtracker_test.go` computes the answer the slow, obvious way (walk
backwards from every tip, mark what it reaches, confirm what all of them reached)
and compares that to the incremental tracker after **every** insert across
several seeded random DAGs, in both directions: the tracker may never confirm a
site the definition does not, and may never miss one it does. It also asserts the
invariant the implementation leans on — no tip ever reaches the denominator,
because a tip does not confirm itself.

Deliberately breaking each mechanism shows the tests are load-bearing: not
unmarking a retired tip's bit, confirming at 99% instead of 100%, counting
detached sites in the denominator, and not following the backward closure were
all caught. One survived — allowing tips to be confirmed — because it is provably
unreachable, which is why the invariant above is asserted directly instead.

On the live two-node network, with three waves of load:

```
pins: A=9 B=9
dag sites: 301
sites pinned: 286        -> 95% confirmed, the remaining 15 being the frontier
```

Supply still conserved exactly, no panics, no races, and no site reported as
stranded. Memory behaves as designed: after 400 inserts in the unit test the
active region holds **2 sites**, because confirmation is closed downwards and
confirmed sites leave the tracker — what stays resident is the frontier, not the
ledger.

One departure from the paper, for liveness: a tip that goes unapproved for
`dag.tiptimeout` (30s by default) stops counting towards the denominator, since
one abandoned tip would otherwise stall confirmation for everything newer. It
keeps being offered for approval — an earlier version dropped it from selection
too, which would have stranded its transaction outside every commit transaction.
That path is now covered by a test.

## Slicing (technical paper section 6)

Confirmation bounds its own working set, but the DAG itself did not: the node
slice, the edge list and both lookup maps grew for the life of the process, and
every site kept its neighbours alive through its edge pointers, so nothing could
be collected. Tip selection, weight updates and every fallback lookup walked that
ever-growing structure.

Sites settled by a commit transaction now leave the live graph and go to a slice
archive, which keeps the protobuf form the commit transaction already holds plus
an index. Controlled by `dag.slicing` (on by default).

The subtlety is incoming edges. A site still in the graph may approve one that
has just been settled; leaving that pointer in place would pull the settled site
— and transitively its neighbours — back into memory. The pointer is dropped and
the id recorded on the approving site, which still reports it on the wire, so a
peer rebuilding those edges sees every approval. Confirmation is unaffected: it
is closed downwards, so a walk that stops at a settled site has stopped somewhere
everything below is already settled.

Slicing also invalidated three assumptions elsewhere, all now fixed: the live
slice's first element was taken to be the genesis site (it is held explicitly
now), and both the tip-selection warm-up and the leader-ready check measured
`len(live graph)` against the configured width — which, once slicing shrinks the
graph, would have dropped the node back into the genesis-fanout phase and linked
new sites straight to genesis. Both now measure how many sites have ever been
added.

Same load as the run above, with slicing on:

```
pins: A=9 B=9
live graph: 14 sites          (301 before slicing, same traffic and same pins)
supply: conserved exactly
```

The unit tests cover it from the other side: 300 inserts across 30 commit
transactions leave 31 sites and 0 edges resident with 270 archived, settled sites
stay findable through the archive with their commit-transaction number, and
settled sites cannot be confirmed a second time. Removing the node filter, the
pointer breaking, the id recording, or the archiving is caught by those tests.

The wallet flow was re-run against the sliced chain to confirm settled
transactions are still queryable: faucet, balance, signed payment, settlement to
the unit on both sides, and history all still pass.

## Persistence (restart without resyncing)

`dag/dag.go` used to log "Persisting the latest DAG state" next to a `@TODO:
Implement DAG serialization`, and persist nothing: every restart rebuilt the
ledger from the network. The commit-transaction chain is now written to an
embedded Pebble store as it is formed, and a restarting node rebuilds from it.

What is persisted is the chain, and only the chain. A commit transaction already
carries the sites it settled, the balances, the executed contract transactions
and the state diffs, so balances, the slice archive and the confirmation record
are derived on start-up rather than stored separately — nothing can disagree
with the chain because nothing else is stored. Unconfirmed sites are
deliberately not persisted: no commit transaction has settled them, so they come
back from the network, and anything the chain is behind on arrives through the
gap-download path on the next announcement.

Controlled by `store.enabled` (on) and `store.path` (`data/ledger`, relative to
`~/.grap3`). A store belonging to another network is refused rather than
misread.

### The balance maps are not a faithful snapshot

The first attempt rebuilt balances from the balance map each commit transaction
carries. On a live restart, conservation then failed by 11 units — small, and
stable, which is what made it worth chasing rather than dismissing.

`cmd/ledgercheck` reads a stored chain without needing the node and computes the
balances two ways: as the chain states them, and as its settled payments imply.
On that chain:

```
opening total:  10000000000000000000000000000000
replayed total: 10000000000000000000000000000000
stated total:    9999999999999999999999999999984
value is conserved: replaying every settled payment returns the opening total
1 wallet(s) where the chain states a balance the transactions do not support
```

The map is written from the live cache at the moment the commit transaction is
formed, so it also reflects sites that were still unconfirmed — it is not a
statement of the ledger at that point. Receiving nodes never trusted it; their
confirmed cache is built by replaying payments, and recovery now does the same.
The maps are left as they are: they are consensus-visible, and correcting them
belongs with the validator work, not here.

Two further problems surfaced while getting this right:

- The leader never maintained a confirmed balance cache at all — only the
  receive path filled it — so there was no settled state to persist on the node
  that forms the commit transactions. There is now one settled ledger,
  maintained identically whichever side of the chain a node is on.
- The pin that *opens* a node's chain arrives outside the ordinary commit path:
  genesis on a node starting a ledger, the leader's snapshot on a node joining
  one. Both state balances outright rather than as transactions, and neither was
  being folded in, so the settled ledger seeded from a later commit transaction
  instead. That is the bug conservation was reporting.

### What the live restart showed

Two peers, three waves of load, then both stopped and restarted on their
existing data directories:

```
before restart   pins A=9 B=9, conservation exact
after restart    [store] Seeded balances from the snapshot taken at pin 9 (286 wallets)
                 [store] Recovered 10 commit transaction(s) from disk, chain head is pin 9
                 [store] Continuing an existing ledger; skipping the balance snapshot handshake
                 pins A=9 B=9, conservation exact
then             pins advance to 14, conservation still exact
```

Neither node requested a balance snapshot from the leader: both recovered from
disk. The chain continued from where it left off rather than renumbering, and
the wallet flow — faucet, balance, signed payment, settlement, history — passes
against the recovered chain. The store held 412K for 18 commit transactions and
571 settled sites.

Balances after a restart are legitimately *higher* than before it for accounts
that had payments in flight: those sites were never settled, so their debits are
not part of the ledger. They arrive again from the network.

### On the tests

Ten deliberate breakages are caught: not persisting, not folding the opening pin
in, not updating the settled balances on commit, never snapshotting, not
snapshotting on shutdown, replaying pins a snapshot already covers, crediting a
recipient without debiting the sender, ignoring the opening statement, skipping
the network check, and leaving the working cache empty.

Three of those initially survived, which was worth more than the ones that did
not: recovery falls back to replaying the whole chain when there is no snapshot,
and that fallback was quietly covering for the live bug. The tests now shut down
the way the node does, so the snapshot recovery trusts is the one the run
produced.

## Authorised commit transactions, on two peers

Run after the pin-authority and deterministic-hash work, because both change
whether a commit transaction is applied at all — a mistake there stops the chain
rather than degrading it, and no unit test can prove the wiring.

Leader A and joining peer B, as above. What the logs showed:

```
A  [pin auth] Adopting 940d33cc..ff2057e3 as the only signer whose commit
   transactions this node will apply, learned from the chain-opening statement
B  [pin auth] Adopting 940d33cc..ff2057e3 ...          (from A's snapshot)
B  [No gaps detected] Process latest pin from leader as our latest pin=8
B  [slice] Settled 9 site(s) into pin=8; live graph now holds 2 site(s),
   0 edge(s), archive 54
```

- B adopted A's signing key from the snapshot it was given on joining, and then
  applied every commit transaction A produced: **eight commit transactions, zero
  refusals**.
- Confirmation, tip selection and slicing all ran on the new rules: the live
  graph stayed at 2 sites while the archive grew to 54.
- `ledgercheck` on both stores reported identical chains and conserved value.

The one discrepancy is the pre-existing one recorded above: the faucet wallet's
*stated* balance is 5 below what its transactions imply, because a commit
transaction's balance map is written from the live cache and can include
transactions that were still unconfirmed. Nodes rebuild balances by replaying
settled payments, so it does not affect them.

### Driving load: three things that waste an afternoon

`txgen` needs its own `txgenerator.yml` in the node's `~/.grap3`, and three
settings in it are not optional even though nothing complains when they are
missing:

```yaml
generator:
  trader: true       # without this, genesis and trader modes both fall through
                     # to a random-wallet generator whose payments are all
                     # correctly rejected for insufficient funds
  network: 2         # must match peer.network, or every tx is refused as
                     # coming from a different network
  wallet: 0x...      # the faucet wallet from the node's dag config, with its
  publickey: ...     # keys - otherwise the sender has no balance
  privatekey: ...
```

With `trader` unset the node logs a steady stream of `Invalid balance ... Ignore`
and nothing ever enters the graph, which looks exactly like a broken node.

Confirmation also needs more load than it first appears: at around 11 sites
nothing had confirmed yet and `* [PIN] sites: 0` repeated every five seconds. By
about 60 sites commit transactions were forming steadily with 5, 6 and 9 sites
each. Enable debug logging (`-d`) to see those lines at all — the pin builder and
the slicer both log at debug level.

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

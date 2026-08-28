# Fee and reward economics

**Status: a proposal for review.** The mechanism below is implemented behind a
switch that is off by default (`tx.feestartpin: -1`). Nothing charges a fee and
nothing pays a reward until somebody sets that. Every number in the table at the
end is a parameter, not a constant, and they are the numbers I am asking you to
agree or change.

## What has to be true

1. A payment must cost something, or the cheapest attack on the network is to
   flood it with payments.
2. The node that did the work must be the node that gets paid, and every other
   node must be able to check that independently. A reward is a ledger entry; if
   two nodes compute it differently the chain forks.
3. Holding Grape should be worth something to a node operator, or there is no
   reason to hold it.
4. **A user who has just installed the wallet must be able to earn.** This is
   the one that constrains the rest, and it is where the first draft of this
   design was wrong — see "The stake gate I removed" below.

## The fee

**No wire change.** A payment already carries `fuel_limit` and `fuel_price`, and
the funds check already includes them, so the fee rides on fields that exist:

```
fee = fuel_limit x fuel_price
```

A payment is valid once fees are active only if `fuel_limit == 1` and
`fuel_price >= tx.minpaymentfee`. Fixing the limit at 1 makes the fee exactly
the price, so there is one number to reason about and no room to express a fee
in two different ways.

Escrow is the obvious part and the part worth stating: the sender is debited
`amount + fee`, the recipient is credited `amount`, and the fee goes to the
pool. The sender pays the fee; the recipient does not.

Enforced in three places, because one is not enough:

- `SendRawTransaction` / `executePaymentTx`, so a client is told immediately.
- `NewDagNode` / `UpdateBalanceIfValid`, so a peer cannot bypass the API and
  diffuse an underpaying payment directly.
- The commit-transaction build, so the fee that enters the pool is recomputed
  from the transaction rather than taken on trust.

Before `tx.feestartpin` the fee is zero and every path behaves exactly as it
does today. That is what makes this shippable ahead of the decision to switch
it on.

## The reward

Every commit transaction carries its own reward split, and every node recomputes
it from the commit transaction's own contents. Nothing is taken on trust and
nothing depends on when a node happened to see something.

```
feePool  = sum of payment fees in this commit transaction
         + sum of (usedFuel x fuelPrice) for its smart-contract transactions

work_i   = sites in this commit transaction attributed to processor i
weight_i = the stake weight of processor i, in permille (below)

reward_i = floor( feePool x work_i x weight_i / sum_j( work_j x weight_j ) )
remainder -> tx.coinbaseaccount
```

**Integer arithmetic throughout, deliberately.** A reward is a consensus value:
every node must reach the same number to the last neutrino. Floating point does
not guarantee that across compilers and architectures, so there is no float
anywhere in this calculation. The weight is in permille for the same reason — it
is a fixed-point number pretending to be nothing else.

The floor means the split almost never divides exactly. The remainder is a few
neutrinos and it goes to the coinbase account, which makes the sum exact by
construction: `feePool == sum(reward_i) + remainder`, checkable on every node.

### Stake weight

```
weight_i = 1000                                     if balance < tx.minstake
         = min(1000 + 301 x floor(log2(balance / tx.minstake)),
               tx.stakecapmilli)                    otherwise
```

- **1000 permille is 1.0x** — the base every processor gets for doing the work.
- **301 per doubling** is log10(2) x 1000, so each *ten-fold* increase in stake
  adds 1.0x. A holder with 10x the minimum earns 2.0x, 100x earns 3.0x, and so
  on until the cap.
- **`floor(log2(...))` is a bit-length**, so the whole thing is integer shifts
  and adds. No `math.Log`, nothing to disagree about.
- **The cap** (proposed 5000 = 5.0x) is what stops the largest holder on the
  network from collecting nearly everything.

Splitting a balance across many wallets to game this does not work, and does not
need preventing separately — but the reason is about `work x weight`, not about
the weight on its own. Two wallets each collect the base weight, and each also
does half the work, so the base cancels out; what remains is the bonus per unit
of work, and the concave curve makes that fall:

| Stake held as | Weight each | Total `work x weight` |
| ------------- | ----------- | --------------------- |
| one wallet, 1024x minimum, 1024 units of work | 4010 | 4,106,240 |
| two wallets, 512x each, 512 units each | 3709 | 3,798,016 |
| four wallets, 256x each, 256 units each | 3408 | 3,489,792 |
| eight wallets, 128x each, 128 units each | 3107 | 3,181,568 |

A whale's best move is one wallet, which is also what we want.

### The stake gate I removed

The first draft of this design gave `weight = 0` below `tx.minstake`, as a sybil
gate. I have taken that out, because it contradicts the product: the one-click
processing node exists so that somebody who has just installed a wallet can
contribute and earn. A brand-new user has no Grape. Under the gate they would
run a node, do real work, and be paid nothing — and would have no way to find
out why except by reading this document.

Sybil resistance does not need the gate. Rewards are proportional to *work*, and
work is bounded by what a machine can actually process; creating a hundred
wallets does not create a hundred machines. The gate was defending against an
attack the work term already handles, at the cost of the users we most want.

What `tx.minstake` now means is only "where the stake bonus starts".

## Worked example

One commit transaction settling 5000 sites, each paying the minimum fee of 1000
neutrinos, so `feePool = 5,000,000` neutrinos (0.05 Grape). `minstake` is 100
Grape, the cap is 5.0x.

| Processor | Sites | Balance | Weight | Reward (neutrinos) | Share |
| --------- | ----- | ------- | ------ | ------------------ | ----- |
| A | 2500 | 100 Grape (= minstake) | 1000 (1.0x) | 1,451,463 | 29.0% |
| B | 2000 | 10,000 Grape (100x) | 2806 (2.8x) | 3,258,244 | 65.2% |
| C | 500 | 50 Grape (below minstake) | 1000 (1.0x) | 290,292 | 5.8% |

`sum(work x weight) = 8,612,000`. Distributed 4,999,999; **remainder 1 neutrino
to the coinbase.**

Read the second row before agreeing to this: **B did 40% of the work and takes
65% of the pool**, because it holds a hundred times the minimum stake. That is
what a 2.8x weight does. If that tilt is wrong, `tx.stakecapmilli` is the dial —
set it to 1000 and rewards become purely work-based, with stake worth nothing.
I have proposed 5000 rather than something larger for exactly this reason, and I
would not argue hard against 2000.

Row C is the point of the previous section: a user below the minimum stake still
earns, in proportion to the work they did.

## Parameters for sign-off

| Setting | Proposed | What it does | If you change it |
| ------- | -------- | ------------ | ---------------- |
| `tx.feemode` | `fixed` | How the fee is set. `fixed` is the only implemented mode. | A market/priority mode is a later change; the field exists so it need not be a migration. |
| `tx.minpaymentfee` | 1000 neutrinos (1e-5 Grape) | The minimum a payment must pay. | Higher prices out spam sooner and makes small payments dearer. At 1000 TPS this is 0.05 Grape/second flowing to processors. |
| `tx.feestartpin` | `-1` (off) | The commit-transaction number fees begin at. | Must be agreed network-wide before it is set. A node that switches alone rejects payments everyone else accepts. |
| `tx.minstake` | 100 Grape | Where the stake bonus starts. | Higher makes the bonus rarer, not the reward smaller — nobody is excluded either way. |
| `tx.stakecapmilli` | 5000 (5.0x) | Ceiling on the stake bonus. | 1000 = ignore stake entirely. Larger = more tilt toward big holders. **The main economic dial.** |
| `tx.coinbaseaccount` | existing setting | Where rounding remainders go. | A treasury address, or a burn address if the remainder should leave circulation. |

Two questions I cannot answer for you:

1. **Is 5.0x the right tilt toward stake?** The worked example shows what it
   does. This is a policy decision about who the network is for.
2. **Should the remainder be burned rather than banked?** It is a few neutrinos
   per commit transaction — about 17 Grape a year at 1000 TPS — so it matters
   for supply policy, not for anyone's balance.

## What this does not do

- **No fee market.** The fee is a fixed minimum, not a bid. Under congestion
  transactions queue rather than outbid each other. `feemode` is the seam for
  changing that later.
- **No block reward or inflation.** Processors are paid out of fees only. If fee
  volume is too low to pay for the hardware, the answer is a parameter change or
  a subsidy decision, and neither is implemented.
- **No slashing.** A processor that misbehaves is not paid for the work it
  fails to get confirmed, which is the only penalty.
- **Validators are not paid separately** for running the commit-transaction
  quorum. They earn as processors, like everyone else. Worth revisiting if
  validating turns out to cost materially more than processing.

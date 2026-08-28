# Validator consensus on commit transactions

A commit transaction is the ledger's only irrevocable statement. It names the
sites that are settled, states the balances that follow, and every node applies
it and discards the settled sites from its graph. In leader mode one node
decides all of that and everyone else takes its word for it.

Quorum mode replaces that with agreement. This document is what the protocol
does, what it deliberately does not do, and how to turn it on.

## Turning it on

```yaml
dag:
  consensus: quorum
  validators: "04ab...ef,04cd...12,04ef...34,0412...ab"
```

Every node on the network needs the same `validators` list, including nodes that
are not in it: that list is how a node decides whether a commit transaction it
receives carries enough agreement to apply. A node whose own public key is in
the list runs the protocol; every other node applies what the set agrees.

The default is still `leader`. A node that switches alone stops applying
anything, because nothing else on its network is producing certificates.

## The exchange

An epoch is one commit transaction, numbered by the pin it will become, and
lasts one commit interval (five seconds by default).

| | |
| --- | --- |
| **t0** | Clear the previous epoch. |
| **t1** | Every validator broadcasts the sites it holds confirmed, signed. |
| **t2** | The settleable set is the sites at least a quorum of validators reported. |
| **t3** | The epoch's proposer builds a commit transaction over that set and broadcasts it. Every validator checks it against its own reports before signing. |
| **t4** | The proposer collects a quorum of signatures into the certificate and publishes. |

The quorum is `⌊2n/3⌋+1`, derived from the set size rather than configured, so
it cannot drift out of step with the membership.

**The proposer for `(epoch, round)` is `sortedValidators[(epoch+round)%N]`.** A
proposer that says nothing costs one round rather than the epoch, and replacing
it needs no election — only a quorum agreeing the round is over.

### Why a quorum of reports rather than all of them

Validators do not see the graph at the same instant. A site published by one
node reaches the others milliseconds later, so at any moment each validator
holds a slightly different confirmed set. Requiring every validator to have
reported a site would mean the slowest validator decided what settles;
requiring one would mean any single validator could. A quorum tolerates the
skew without letting any one of them decide alone.

It is also the same fraction as `dag.confirmshare`, so a site is settled under
the same share of the validator set that confirmed it within the graph. See
[confirmation.md](confirmation.md).

## What a validator checks before it signs

The check that makes the quorum worth having is that **every site a proposal
settles has to be one a quorum of validators reported, on the receiving
validator's own evidence.** A proposal that settles anything else is refused, so
a dishonest proposer cannot settle a site the network has not confirmed — it can
only fail to get its round agreed.

Alongside that: the proposal has to come from the validator whose turn it is,
for the epoch and round the receiver is in; votes count only for the proposal in
hand, only from validators, only with a signature that verifies, and only in the
round they were cast in.

## Building a commit transaction before anyone has agreed to it

The proposer has to build the whole commit transaction to propose it, and
building one is not free of consequence. The smart-contract stage executes the
transactions it includes, moves them out of the unconfirmed pool, and
invalidates the wallet cache for every account it touches. A proposer that built
one and then lost its round would be left holding state no other node has:
contracts executed against a commit transaction that does not exist.

So a build is speculative and reversible. `pinCandidate` records the three
things a build changes — the VM state store (behind a checkpoint), the
smart-contract pool, and the wallet cache — and the driver undoes all three
whenever a round ends without publishing. Winning the round drops the checkpoint
instead, and the work stands.

The confirmed sites themselves are never at risk, because reporting them does
not consume them: `peek` reports, `take` consumes, and only a published commit
transaction calls `take`. A round that fails leaves every site exactly where it
was, to be reported again in the next epoch.

## Bounds

Three things the protocol has to bound, each of which stopped a live network
before it did.

| Bound | Value | Why |
| ----- | ----- | --- |
| Sites one commit transaction settles | 5,000 | A proposal travels as one gossip message carrying the whole candidate, and every validator rebuilds and checks it inside the voting window. This is the design target said as a bound: a thousand transactions a second at the five-second cadence. |
| Sites one report names | 10,000 | A report is a message, and it is counted on every round. |
| Commit transactions one catch-up response carries | half of `peer.msize` | The response is one message too. Over the limit it is dropped while the receiving side reads it, and the node that asked simply never hears back. |

The first two are prefixes of a set ordered by site id, so every validator takes
the same one without exchanging anything, and sites past the cut are not
dropped — they are the next commit transaction's. The report cap is twice the
settlement bound so that validators holding slightly different sets still
overlap by more than the bound.

## What this does not do

- **The validator set is a configuration list.** There is no stake-based
  election and no on-chain membership change. Changing the set means changing
  every node's configuration.
- **An epoch that agrees nothing produces nothing.** That is not a failure and
  deliberately does not trigger a view change: an idle chain would otherwise
  churn its proposer every voting window for ever, reporting each time that
  nothing had happened.
- **Two simultaneous failures in a set of four are beyond it.** A quorum of two
  thirds plus one tolerates `⌊(n-1)/3⌋` faults; asking four validators to route
  around two is asking for the impossible.

## Tests

The whole validator set runs in one process on a clock the tests move by hand.
A view change takes seconds of wall-clock time and needs a node to actually die,
which no real network can exercise at any useful rate; here it is a function
call.

Fourteen mutations of the safety checks — the justification rule, the proposer
rotation, the vote hash and signature, the round scoping, the quorum thresholds
and the sender identity — are each caught by a test. Getting there rewrote two
tests that had been passing vacuously, which is recorded here because the shape
of the mistake recurs:

- One silenced every validator but the proposer, so no proposal ever formed and
  the votes it injected were dropped before any check ran. It asserted nothing
  was published, and nothing was — for the wrong reason. `TestInjectedVotesCompleteTheQuorum`
  is now the control that shows injected votes *can* complete a quorum, so the
  negative results are about the checks and not the harness.
- The other was refused by the justification rule before the rotation check it
  meant to exercise could matter. Two checks that both refuse the same message
  look like one working check until each is removed on its own.

Eleven further mutations cover the wiring: reporting that consumes what it
reports, settling sites the set did not agree on, settling one twice, building
that appends, and each of the three halves of the rollback.

## What running it showed

Four validators as separate processes on one four-core machine, driven by
`txgen -mode trader`. Every defect below was found this way and by nothing else;
the unit tests passed throughout.

| What happened | Why |
| ------------- | --- |
| Nothing was ever settled | A validator reported what it held confirmed once, when it opened an epoch — and validators open epochs at different moments, so the first report each sent was the only one the others held. |
| The genesis node stopped at its own first commit transaction while the others reached eleven | A node only became ready to apply a commit transaction by syncing one from a peer, because in leader mode the node that starts a chain is the only one that settles it. |
| Killing one of four livelocked the chain through 116 view changes | A validator counted only the reports it happened to have received, so a report still in flight made it refuse a proposal that was justified. With three live validators and a quorum of three there is no slack for that. |
| A restarted validator never rejoined | Its catch-up response was one message of megabytes, dropped while being read; nothing reports that. |
| Catching up crashed the node | A placeholder site — an approval target seen before its transaction — was asked what kind of transaction it carried. |
| The chain stopped settling with 87,000 sites waiting | Unbounded proposals and reports made every round cost tens of seconds. |

After the fixes, at a rate the machine sustains: all four settle together to
pin 38 with no view changes and no queue pressure; killing one leaves the other
three settling through single-round view changes; restarting it sixteen commit
transactions behind recovers from disk, catches up in batches, and rejoins as a
proposer. Zero refusals, zero crashes.

At a rate the machine does not sustain — four validators and the generator on
four cores, insert queue at its ceiling, transactions arriving six minutes
late — the backlog outgrows what a node can build per epoch and the chain falls
behind. The bounds keep that a slowdown rather than a livelock. Closing it is
throughput work, not consensus work.

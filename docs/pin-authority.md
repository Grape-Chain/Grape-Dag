# Who may settle the ledger

A commit transaction is the ledger's only irrevocable statement. It names the
sites that are settled and the balances that follow, and every node that applies
one rewrites its balances and slices the settled sites out of its graph.

Until this was added, nothing checked where one came from. `TxPin.VerifyTx`
existed and was called from nowhere, so any peer on the gossip topic could
publish a commit transaction of its own devising and every node would apply it.

## Why a signature check alone is not enough

`VerifyTx` verifies the signature against `TxPin.Pk` — the public key carried
*inside the commit transaction being verified*. A forgery signed with the
forger's own key carries the matching public key, so it is perfectly
self-consistent and passes. `TestAForgeryWithItsOwnValidSignatureIsRefused` is
that case written down.

A signature is only evidence once it is checked against a key authorised
beforehand. So the node keeps a set of authorised signers, and a commit
transaction has to satisfy both checks: correctly signed, **and** signed by a key
in the set.

## Where the set comes from

### `dag.pinsigners` — the strong form

```yaml
dag:
  pinsigners: "04ab...ef,04cd...12"
```

Public keys in hex, comma separated, `0x` prefix optional. The node applies
commit transactions from these keys and no others, whatever it is told —
including the snapshot it is given when it joins. A peer cannot hand a joining
node a chain of its own construction.

A value that is not hex stops the node at start-up rather than being skipped: a
typo that quietly left the node trusting nobody, or falling back to trusting the
first peer to talk to it, is the failure this exists to prevent.

### Adoption — the weak form, used when nothing is configured

With `dag.pinsigners` empty, the node adopts the signer of its chain-opening
statement: the genesis commit transaction on a node that starts a ledger, or the
snapshot on a node that joins one. Every later commit transaction must then come
from that same key.

Adoption happens **once**. A second opening statement from a different signer is
refused, not added alongside the first — otherwise a peer answering a snapshot
request after the chain is already open could add its own key to the set.

This is weaker, and the node says so at start-up: a peer that lies about the
*whole* chain still succeeds, because there is nothing to check it against. It is
trust on first use, and it is the right default only for a testnet.

Recovery re-establishes the set the same way, from the stored chain's opening
statement. Without that a restarted node would come back with no authorised
signer and refuse every live commit transaction.

## Operational consequences

- **Rotating the signing key breaks existing peers.** Peers hold the old key,
  either from configuration or from adoption, and will refuse commit
  transactions signed by the new one — correctly, since from their point of view
  the identity settling the ledger has changed. Rotation means updating
  `dag.pinsigners` everywhere, or having the peers resynchronise from scratch.
- **A refusal is logged at ERROR** with the offending key, abbreviated. A node
  that has stopped following the chain will say why on every announcement.
- **The leader does not authorise its own commit transactions.** It signs them
  and applies them directly; the check is for what arrives over the network.

## What this is not, yet

This is a set of authorised signers with an implicit quorum of one. The validator
work replaces that with a set and a quorum count — "at least ⅔ of these keys
signed" rather than "one of these keys signed" — and the seam is deliberate:
nothing outside `dag/pinauth.go` cares how many keys there are or how many
signatures are needed.

Until then, a single compromised signing key is a compromised ledger. That is the
main reason the validator quorum is the next piece of work rather than a later
one.

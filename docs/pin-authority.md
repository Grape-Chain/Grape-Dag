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

## Quorum mode — the verification half

`dag.consensus: "quorum"` replaces "one authorised key asserted it" with "at
least two thirds of the validator set agreed to it".

```yaml
dag:
  consensus: "quorum"
  validators: "04ab...ef,04cd...12,04ef...34,0412...ab"
```

The quorum is derived from the size of the set — `⌊2n/3⌋+1`, the most faults a
set of that size can tolerate at all — rather than configured separately, so it
cannot drift out of step with the membership.

A commit transaction then carries a `QuorumCert`: the pin number, the prototype
hash the validators signed, the view-change round, and their signatures. The
prototype hash excludes the signature, the public key **and** the certificate
itself, which is what lets every validator sign the same bytes.

Verification requires all of:

- the certificate is for this pin number;
- it names the hash of the commit transaction actually in hand;
- each signature verifies against that hash, under a key **in the set**;
- signatures from one validator count once however many times they appear;
- at least the quorum many distinct validators remain.

In quorum mode the proposer's own `SignTx` signature is not consulted at all. If
it counted, the quorum would be decoration.

The protocol that produces those certificates is the t0–t4 exchange in
`dag/consensus.go`, described in [consensus.md](consensus.md).

## What this is not, yet

In leader mode this is a set of authorised signers with an implicit quorum of
one, so a single compromised signing key is a compromised ledger. Quorum mode is
what removes that, and it is implemented — but `dag.consensus` still defaults to
`leader`, because the validator set has to be agreed and distributed before a
network can switch, and a node that switches alone stops applying anything.

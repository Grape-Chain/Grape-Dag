package dag

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/Grape-Chain/Grape-Dag/store"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/google/uuid"
)

/*
Recovery from the stored commit-transaction chain.

A commit transaction states the balances as of that point, lists the sites it
settled and carries those sites whole, so the chain is enough to rebuild
everything a restarting node needs:

	the chain itself        replayed into the pin store
	account balances        taken from each pin's balance map, last write wins
	the slice archive       refilled from the sites each pin settled
	confirmation state      those sites recorded as already pinned

Nothing else is read from disk, and nothing else needs to be: unconfirmed sites
were never settled by a commit transaction, so they come back from the network
the same way they would after any brief absence. Anything the chain is behind on
arrives through the ordinary gap-download path on the next announcement.
*/

// recoverFromStore - rebuild state from the stored chain. Reports whether a
// chain was found, so the caller knows to skip the from-scratch genesis path
// and the balance snapshot handshake.
func recoverFromStore() (bool, error) {
	head, err := ledgerStore.Head()
	if err == store.ErrEmpty {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if head.SchemaVersion > store.SchemaVersion {
		return false, fmt.Errorf(
			"the ledger store was written by a newer version (schema %d, this build understands %d)",
			head.SchemaVersion, store.SchemaVersion)
	}
	if head.Network != peerConfig.Network {
		return false, fmt.Errorf(
			"the ledger store belongs to network %d but this node runs on network %d; point it at a different data directory",
			head.Network, peerConfig.Network)
	}

	// Start from an empty chain: a leader has already minted a genesis pin of
	// its own by this point, and the stored chain is the authority.
	_pins_ = newNodeTxPin()
	settled = newSettledLedger()

	// A stored snapshot is a starting point for the balances; the commit
	// transactions after it are replayed onto it.
	snapshotAt, snapshotBalances, err := ledgerStore.Balances()
	if err != nil {
		return false, fmt.Errorf("reading the balance snapshot: %w", err)
	}
	if snapshotAt >= 0 && len(snapshotBalances) > 0 {
		settled.seed(snapshotAt, snapshotBalances)
		logger.Infof("[store] Seeded balances from the snapshot taken at pin %d (%d wallets)",
			snapshotAt, len(snapshotBalances))
	}

	loaded := 0
	var lastSign []byte
	err = ledgerStore.LoadPins(func(pin *pb.TxPin) error {
		if loaded > 0 && len(pin.Prev) > 0 && !bytes.Equal(pin.Prev, lastSign) {
			// Worth reporting but not worth refusing to start: the chain is
			// still usable and the node will reconcile with the network.
			logger.Warnf("[store] Pin %d does not follow pin %d by signature", pin.PinNumber, loaded-1)
		}
		lastSign = pin.Sign

		// Re-establish who may settle this chain. Recovery bypasses applyPin, so
		// without this a restarted node would come back with no authorised
		// signer and refuse every live commit transaction that followed. The
		// stored chain is this node's own history, so the opening statement is
		// the right thing to take it from - and when dag.pinsigners names the
		// signers, a stored chain that does not match them is refused here
		// rather than quietly continued.
		if loaded == 0 {
			if err := authoriseChainStart(pin); err != nil {
				return fmt.Errorf("the stored chain's opening commit transaction is not one this node may apply: %w", err)
			}
		}

		_pins_.lock("recover")
		_pins_.unsafe_appendPin(pin)
		_pins_.unlock()

		applyRecoveredPin(pin)
		loaded++
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("replaying the stored chain: %w", err)
	}
	if loaded == 0 {
		return false, nil
	}

	// Publish the replayed balances as this node's confirmed state, and start
	// the working cache from them.
	settled.installAsConfirmed()

	_pins_.lock("recover-ready")
	_pins_.ready = true
	_pins_.unlock()

	if len(head.GenesisSign) > 0 {
		if first := _pins_.GetPin(0); first != nil && !bytes.Equal(first.Sign, head.GenesisSign) {
			logger.Warnf("[store] The stored genesis pin does not match the signature recorded in the head")
		}
	}

	utils.ColorizeInfo(logger, "[store] Recovered %d commit transaction(s) from disk, chain head is pin %d",
		loaded, _pins_.CurrentHeight())
	return true, nil
}

// applyRecoveredPin - fold one stored commit transaction back into memory.
func applyRecoveredPin(pin *pb.TxPin) {
	// Balances come from replaying what the chain settled, not from the balance
	// map the commit transaction carries: that map is written from the live
	// cache and so includes transactions that were still unconfirmed at the
	// time. applyPin ignores commit transactions already covered by the
	// snapshot we seeded from.
	settled.applyPin(pin)

	// The sites this pin settled are archived rather than returned to the live
	// graph: they are settled, and the live graph holds the frontier.
	if sliceArchive != nil && len(pin.Nodes) > 0 {
		sliceArchive.Archive(pin.PinNumber, pin.Nodes)
	}

	// Record them as pinned, so a late arrival naming one as an approval target
	// cannot get it confirmed - and, once fees land, paid - a second time.
	if confirmationCounter != nil {
		for _, site := range pin.Sites {
			if site == nil {
				continue
			}
			if id, err := uuid.FromBytes(site.Id); err == nil {
				confirmationCounter.markHarvested(id)
			}
		}
	}
}

// recoveredGenesisSite - the genesis site as the stored chain has it, so a
// recovered node keeps the graph root it started with rather than minting a new
// one. Returns nil when the chain does not carry it.
func recoveredGenesisSite() *Node {
	first := _pins_.GetPin(0)
	if first == nil {
		return nil
	}
	for _, pbn := range first.Nodes {
		if pbn == nil || pbn.Id == nil {
			continue
		}
		if id, err := uuid.FromBytes(pbn.Id.Id); err == nil && id == uuid.Nil {
			node := &Node{}
			node.FromPbNode(pbn)
			node.missingTargets = nil
			node.valid = true
			return node
		}
	}
	return nil
}

// storeBalanceOf - the recovered balance for a wallet, for the leader's
// start-up check.
func storeBalanceOf(wallet string) (*big.Int, bool) {
	b, err := walletCacheConfirmed.get(wallet)
	if err != nil || b == nil {
		return nil, false
	}
	return b, true
}

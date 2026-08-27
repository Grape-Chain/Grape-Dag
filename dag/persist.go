package dag

import (
	"github.com/Grape-Chain/Grape-Dag/store"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
)

// ledgerStore - where the commit-transaction chain is kept. Never nil after
// Init: persistence turned off is a store that reports itself empty.
var ledgerStore store.Store = store.NoopStore{}

// settled - the balances the commit-transaction chain supports, maintained on
// every node regardless of role and persisted so a restart resumes from it.
var settled *settledLedger = newSettledLedger()

// snapshotEveryPins - how often the settled balances are written out. The chain
// after the snapshot is replayed on recovery, so this only trades a little disk
// traffic against a slightly longer start-up.
const snapshotEveryPins = 32

// recoveringChain - set during Init when the store already holds a chain, so
// that the from-scratch genesis path stands down.
var recoveringChain bool

// storedChainExists - whether the store already holds a commit-transaction
// chain. Asked before anything is written, so a first run is not mistaken for a
// recovery.
func storedChainExists() (bool, error) {
	head, err := ledgerStore.Head()
	if err == store.ErrEmpty {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return head != nil && head.PinCount > 0, nil
}

// SetStore - hand the dag its durable store. Called before Init, so that
// recovery can run as part of it.
func SetStore(s store.Store) {
	if s == nil {
		ledgerStore = store.NoopStore{}
		return
	}
	ledgerStore = s
}

// GetStore - the store in use.
func GetStore() store.Store { return ledgerStore }

// pinCommitted - everything that has to happen once a commit transaction is
// part of the chain, on whichever side of it we are: the leader that formed it
// and every node that applied it run exactly this.
//
// Order matters. The pin is written to disk before its sites leave the live
// graph, so a crash in between costs nothing: the sites are still in the graph
// and the pin that settles them is already durable.
func pinCommitted(pin *pb.TxPin) {
	if pin == nil {
		return
	}
	if err := ledgerStore.AppendPin(pin, peerConfig.Network); err != nil {
		// Worth being loud about: from here on the node's disk state is behind
		// its memory, so a restart will resync more than it should have to.
		logger.Errorf("[store] Failed to persist pin=%d: %s", pin.PinNumber, err.Error())
	}
	// Fold it into the settled balances before the sites leave the graph: this
	// is the arithmetic a restarting node reproduces.
	settled.applyPin(pin)
	if pin.PinNumber == 0 || pin.PinNumber%snapshotEveryPins == 0 {
		snapshotSettledBalances()
	}
	sliceAppliedPin(pin)
}

// chainStartCommitted - a commit transaction that opens this node's chain: the
// genesis pin on a node that starts a ledger, or the leader's snapshot pin on a
// node that joins one. Both carry balances outright rather than transactions to
// derive them from, and both arrive outside the ordinary commit path.
//
// Persist and fold in, but do not slice: the sites named here are history this
// node never held in its graph, and on a genesis chain the genesis site is the
// graph's root.
func chainStartCommitted(pin *pb.TxPin) {
	if pin == nil {
		return
	}
	if err := ledgerStore.AppendPin(pin, peerConfig.Network); err != nil {
		logger.Errorf("[store] Failed to persist the opening pin=%d: %s", pin.PinNumber, err.Error())
	}
	settled.applyPin(pin)
	snapshotSettledBalances()
}

// snapshotSettledBalances - write the settled balances out, so recovery has a
// starting point and does not have to replay the whole chain.
func snapshotSettledBalances() {
	upTo, balances := settled.snapshot()
	if upTo < 0 || len(balances) == 0 {
		return
	}
	if err := ledgerStore.PutBalances(upTo, balances); err != nil {
		logger.Errorf("[store] Failed to write the balance snapshot at pin=%d: %s", upTo, err.Error())
	}
}

// closeStore - flush and release the store on shutdown.
func closeStore() {
	// A snapshot on the way out keeps the next start-up short.
	snapshotSettledBalances()
	if err := ledgerStore.Close(); err != nil {
		logger.Errorf("[store] Failed to close the ledger store: %s", err.Error())
	}
	ledgerStore = store.NoopStore{}
}

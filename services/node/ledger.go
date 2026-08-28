// Package node exposes what a wallet application needs in order to run the
// machine it is installed on as a processing node: the node's state, what that
// node has earned, and a switch that starts and stops the encapsulation of
// transactions into sites.
//
// The package depends on nothing inside the ledger. Everything it needs arrives
// through the Ledger interface, which is what keeps the endpoints, the state
// machine and the processing gate testable on their own, and what lets the real
// implementation be wired in later without this package changing. See
// docs/processing-node.md for where that wiring goes.
package node

import (
	"math/big"
	"time"
)

// Credit - one fee payment made to a node's wallet for encapsulating
// transactions into a site.
type Credit struct {
	// Pin - the commit transaction that settled the credit. Until one has, the
	// credit is pending rather than earned.
	Pin int64
	// Site - the site whose fees these are. Empty when the credit is an
	// aggregate over a whole commit transaction rather than a single site.
	Site string
	// Amount - in the ledger's smallest unit. A big.Int because a fee total does
	// not fit in a float64 and must not be rounded on the way to a wallet.
	Amount *big.Int
	// At - when the node recorded the credit.
	At time.Time
}

// Ledger - the view of a running node that this package needs. Kept to the five
// questions the endpoints actually ask, so that the real implementation has a
// small surface to satisfy and a fake in a test has a small one to fill in.
type Ledger interface {
	// PinHeight - the number of the newest commit transaction this node has
	// applied. Rises whether or not the node is processing, which is what makes
	// it the honest evidence that a stopped node is still syncing.
	PinHeight() int64
	// PeerCount - how many peers this node is connected to. A node with none can
	// build a site but has nobody to diffuse it to.
	PeerCount() int
	// Syncing - true while the node is still catching up with the network.
	Syncing() bool
	// WalletAddress - the account this node's fees are paid to.
	WalletAddress() string
	// EarningsFor - fees credited to wallet. lifetime is everything a commit
	// transaction has settled, pending is earned but not yet settled, and recent
	// is the most recent credits, newest first.
	EarningsFor(wallet string) (lifetime, pending *big.Int, recent []Credit, err error)
}

// StubLedger - the default Ledger: a node that knows nothing.
//
// It exists so that this package builds and is testable without the dag package,
// and so that mounting the endpoints before the real implementation is wired
// answers with honest empty values instead of a nil-interface panic. Syncing
// reports true because a node that cannot say whether it has caught up has not
// earned the right to claim it has.
type StubLedger struct{}

func (StubLedger) PinHeight() int64      { return 0 }
func (StubLedger) PeerCount() int        { return 0 }
func (StubLedger) Syncing() bool         { return true }
func (StubLedger) WalletAddress() string { return "" }

func (StubLedger) EarningsFor(string) (lifetime, pending *big.Int, recent []Credit, err error) {
	// A non-nil empty slice: the endpoint renders it as [] rather than null,
	// which saves every caller a null check.
	return big.NewInt(0), big.NewInt(0), []Credit{}, nil
}

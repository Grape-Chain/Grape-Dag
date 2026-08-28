package services

import (
	"testing"

	"github.com/Grape-Chain/Grape-Dag/services/node"
)

// A wallet application polls the status endpoint from the moment it launches the
// node, so every accessor is called before the node has finished starting: no
// libp2p host, no sync manager, and on a bare process no configuration either.
// Each one has to answer rather than panic, because the first thing a new user
// would see otherwise is the node dying when its own control page loaded.
func TestTheLiveLedgerAnswersBeforeTheNodeHasStarted(t *testing.T) {
	l := NewNodeLedger()

	if got := l.PinHeight(); got < 0 {
		t.Fatalf("pin height before start-up is %d", got)
	}
	if got := l.PeerCount(); got != 0 {
		t.Fatalf("peer count with no host is %d, want 0", got)
	}
	// Not "ready": a node that has not started trying to catch up has not
	// caught up.
	if !l.Syncing() {
		t.Fatal("a node with no sync manager reports itself in sync")
	}
	// An address may or may not be configured in a test binary; what matters is
	// that asking does not panic and does not invent one.
	_ = l.WalletAddress()
}

// Earnings are read from the commit-transaction chain. With no chain - which is
// the state of a node that has not started - the answer is nought earned and no
// credits, and it must not be nil: the JSON encoder renders a nil slice as null,
// and a wallet application reading null where it expects a list is a crash on
// somebody else's machine.
func TestEarningsWithNoChainReportNothingRatherThanNil(t *testing.T) {
	lifetime, pending, recent, err := NewNodeLedger().EarningsFor("0xabc")
	if err != nil {
		t.Fatalf("earnings returned err=%v, want nil", err)
	}
	if lifetime == nil || pending == nil {
		t.Fatal("earnings returned nil totals, which the JSON encoder cannot render")
	}
	if lifetime.Sign() != 0 || pending.Sign() != 0 {
		t.Fatalf("a node with no chain reports lifetime %s pending %s", lifetime, pending)
	}
	if recent == nil {
		t.Fatal("recent credits are nil; the endpoint must render [] rather than null")
	}
	if len(recent) != 0 {
		t.Fatalf("a node with no chain reports %d credits", len(recent))
	}
}

// The live implementation has to satisfy the interface the endpoints are built
// against. Compile-time, but stated as a test so that changing either side
// fails here with a clear reason rather than somewhere in the REST wiring.
func TestTheLiveLedgerSatisfiesTheEndpointsInterface(t *testing.T) {
	var _ node.Ledger = NewNodeLedger()
	if NewNodeLedger() == nil {
		t.Fatal("NewNodeLedger returned nil")
	}
}

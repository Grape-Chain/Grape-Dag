package services

import (
	"errors"
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

// Fees are not credited to anybody yet. The endpoint must say so rather than
// report zero, which a wallet application cannot tell from "you have earned
// nothing" - and which would show a user a confident 0.00 for a figure the node
// is not computing at all.
func TestEarningsReportNotWiredRatherThanZero(t *testing.T) {
	lifetime, pending, recent, err := NewNodeLedger().EarningsFor("0xabc")
	if !errors.Is(err, node.ErrNotWired) {
		t.Fatalf("earnings returned err=%v, want node.ErrNotWired", err)
	}
	if lifetime == nil || pending == nil {
		t.Fatal("earnings returned nil totals, which the JSON encoder cannot render")
	}
	if recent == nil {
		t.Fatal("recent credits are nil; the endpoint must render [] rather than null")
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

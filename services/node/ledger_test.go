package node

import "testing"

func TestTheStubLedgerReportsANodeThatKnowsNothingRatherThanPanicking(t *testing.T) {
	var l Ledger = StubLedger{}

	if got := l.PinHeight(); got != 0 {
		t.Errorf("PinHeight() = %d, want 0", got)
	}
	if got := l.PeerCount(); got != 0 {
		t.Errorf("PeerCount() = %d, want 0", got)
	}
	if !l.Syncing() {
		t.Error("Syncing() = false; an implementation that cannot tell must not claim to have caught up")
	}
	if got := l.WalletAddress(); got != "" {
		t.Errorf("WalletAddress() = %q, want the empty string", got)
	}

	lifetime, pending, recent, err := l.EarningsFor("0xabc")
	if err != nil {
		t.Fatalf("EarningsFor returned %s, want no error", err)
	}
	if lifetime == nil || lifetime.Sign() != 0 {
		t.Errorf("lifetime = %v, want a zero big.Int rather than nil", lifetime)
	}
	if pending == nil || pending.Sign() != 0 {
		t.Errorf("pending = %v, want a zero big.Int rather than nil", pending)
	}
	if recent == nil {
		t.Error("recent is nil; the endpoint has to be able to render it as []")
	}
	if len(recent) != 0 {
		t.Errorf("recent has %d entries, want none", len(recent))
	}
}

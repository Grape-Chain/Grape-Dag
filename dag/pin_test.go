package dag

import (
	"testing"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
)

func TestNextPinNumberStartsAtZeroAndIncrements(t *testing.T) {
	p := &NodeTxPin{pins: []*pb.TxPin{}}

	// the genesis pin is number 0
	if got := p.unsafe_nextPinNumber(); got != 0 {
		t.Fatalf("first pin number = %d, want 0", got)
	}

	// every subsequent pin continues the chain rather than restarting from a
	// process-local counter (which is what produced two pins numbered 0)
	for want := int64(0); want < 5; want++ {
		got := p.unsafe_nextPinNumber()
		if got != want {
			t.Fatalf("pin number = %d, want %d", got, want)
		}
		p.pins = append(p.pins, &pb.TxPin{PinNumber: got})
	}
}

func TestNextPinNumberFollowsChainHeadAfterSync(t *testing.T) {
	// A node that snapshot-synced holds pins that start above zero; the next pin
	// must follow the head, not the number of pins we happen to hold.
	p := &NodeTxPin{pins: []*pb.TxPin{
		{PinNumber: 41},
		{PinNumber: 42},
	}}
	if got := p.unsafe_nextPinNumber(); got != 43 {
		t.Fatalf("pin number after sync = %d, want 43", got)
	}
}

func TestCurrentHeightTracksChainHead(t *testing.T) {
	p := &NodeTxPin{pins: []*pb.TxPin{}}
	if got := p.CurrentHeight(); got != 0 {
		t.Fatalf("empty chain height = %d, want 0", got)
	}
	p.pins = append(p.pins, &pb.TxPin{PinNumber: 7})
	if got := p.CurrentHeight(); got != 7 {
		t.Fatalf("height = %d, want 7", got)
	}
	// CurrentHeight takes the pin lock; the unsafe_ variant must agree
	p.lock("test")
	unsafeGot := p.unsafe_currentHeight()
	p.unlock()
	if unsafeGot != 7 {
		t.Fatalf("unsafe_currentHeight = %d, want 7", unsafeGot)
	}
}

func TestSnapshotPinsIsolatesCallerFromAppends(t *testing.T) {
	p := &NodeTxPin{pins: []*pb.TxPin{{PinNumber: 0}}}
	snap := p.snapshotPins()
	p.pins = append(p.pins, &pb.TxPin{PinNumber: 1})
	if len(snap) != 1 {
		t.Fatalf("snapshot changed under the caller: len = %d, want 1", len(snap))
	}
}

// Locked accessors must not deadlock when called in sequence (they take and
// release the same non-reentrant mutex).
func TestLockedPinAccessorsDoNotDeadlock(t *testing.T) {
	p := &NodeTxPin{pins: []*pb.TxPin{{PinNumber: 3}}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.CurrentHeight()
		_ = p.CurrentTS()
		_ = p.GetLastPin()
		_ = p.GetPin(3)
		_ = p.snapshotPins()
	}()
	<-done
}

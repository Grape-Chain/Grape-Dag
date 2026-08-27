package dag

import (
	"testing"
	"time"

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
		p.unsafe_appendPin(&pb.TxPin{PinNumber: got})
	}
}

func TestNextPinNumberFollowsChainHeadAfterSync(t *testing.T) {
	// A node that snapshot-synced holds pins that start above zero; the next pin
	// must follow the head, not the number of pins we happen to hold.
	p := &NodeTxPin{}
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 41})
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 42})
	if got := p.unsafe_nextPinNumber(); got != 43 {
		t.Fatalf("pin number after sync = %d, want 43", got)
	}
}

func TestCurrentHeightTracksChainHead(t *testing.T) {
	p := &NodeTxPin{}
	if got := p.CurrentHeight(); got != 0 {
		t.Fatalf("empty chain height = %d, want 0", got)
	}
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 7})
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
	p := &NodeTxPin{}
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 0})
	snap := p.snapshotPins()
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 1})
	if len(snap) != 1 {
		t.Fatalf("snapshot changed under the caller: len = %d, want 1", len(snap))
	}
}

// The reader view is what every lock-free accessor returns, so a mutation that
// forgets to republish is invisible to readers. This pins that invariant down.
func TestAppendRepublishesTheReaderView(t *testing.T) {
	p := &NodeTxPin{}
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 0})
	if got := len(p.chain()); got != 1 {
		t.Fatalf("view length = %d, want 1 after append", got)
	}

	// a direct mutation is only visible once republished
	p.pins = append(p.pins, &pb.TxPin{PinNumber: 1})
	if got := len(p.chain()); got != 1 {
		t.Fatalf("view length = %d, want 1 before republish", got)
	}
	p.unsafe_publish()
	if got := len(p.chain()); got != 2 {
		t.Fatalf("view length = %d, want 2 after republish", got)
	}
}

// Readers must not block while a pin is being applied: applying re-executes
// smart contracts against the VM, which can take a long time.
func TestReadersDoNotBlockWhileTheChainIsLocked(t *testing.T) {
	p := &NodeTxPin{}
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 3})

	p.LockPin() // simulate a pin being applied
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.CurrentHeight()
		_ = p.CurrentTS()
		_ = p.GetLastPin()
		_ = p.GetPin(3)
		_ = p.snapshotPins()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		p.UnlockPin()
		t.Fatalf("readers blocked while the pin lock was held")
	}
	p.UnlockPin()
}

// A node that joined via a balance snapshot holds pins numbered from wherever
// the leader was, so lookups cannot assume position == number.
func TestGetPinFindsPinsOnASnapshotSyncedChain(t *testing.T) {
	p := &NodeTxPin{}
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 41})
	p.unsafe_appendPin(&pb.TxPin{PinNumber: 42})

	for _, n := range []int{41, 42} {
		got := p.GetPin(n)
		if got == nil {
			t.Fatalf("GetPin(%d) = nil, want the pin", n)
		}
		if got.PinNumber != int64(n) {
			t.Fatalf("GetPin(%d) returned pin %d", n, got.PinNumber)
		}
	}
	if p.GetPin(0) != nil {
		t.Fatalf("GetPin(0) returned a pin we do not have")
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

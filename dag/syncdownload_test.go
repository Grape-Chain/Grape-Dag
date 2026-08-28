package dag

import (
	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/google/uuid"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// buildPinDownloadResponse - the wire shape respondAllPinsFrom produces:
// Details[0] is the count, Details[1:] are the marshalled pins.
func buildPinDownloadResponse(t *testing.T, numbers ...int64) *tx.Syncv1 {
	t.Helper()
	rec := tx.NewSyncv1()
	count, err := anypb.New(wrapperspb.Int32(int32(len(numbers))))
	if err != nil {
		t.Fatal(err)
	}
	rec.Details = append(rec.Details, count)
	for _, n := range numbers {
		pin := &pb.TxPin{PinNumber: n}
		raw, err := pin.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		wrapped, err := anypb.New(wrapperspb.Bytes(raw))
		if err != nil {
			t.Fatal(err)
		}
		rec.Details = append(rec.Details, wrapped)
	}
	return rec
}

func drainPins(ch chan *pb.TxPin) []int64 {
	got := []int64{}
	for pin := range ch {
		got = append(got, pin.PinNumber)
	}
	return got
}

// The receiver used to read Details[1] on every iteration instead of walking the
// list, so a batch of N pins arrived as N copies of the first one. A node that
// had fallen behind then advanced by at most one pin per round trip and logged
// the rest as out of order - in practice it never caught up.
func TestDownloadedPinsAreReadInOrder(t *testing.T) {
	prev := _pins_
	_pins_ = newNodeTxPin()
	defer func() { _pins_ = prev }()

	_pins_.openPinDownloading()
	ch := _pins_.downloadedPins

	want := []int64{5, 6, 7, 8, 9}
	if err := handleDownloadedPinsFromLeader(buildPinDownloadResponse(t, want...)); err != nil {
		t.Fatalf("handling the batch: %s", err.Error())
	}

	got := drainPins(ch)
	if len(got) != len(want) {
		t.Fatalf("received %d pins %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pin at position %d = %d, want %d (full batch %v)", i, got[i], want[i], got)
		}
	}
}

func TestDownloadedPinsRejectsAShortBatch(t *testing.T) {
	prev := _pins_
	_pins_ = newNodeTxPin()
	defer func() { _pins_ = prev }()
	_pins_.openPinDownloading()

	// claims three pins but carries one
	rec := buildPinDownloadResponse(t, 5)
	count, _ := anypb.New(wrapperspb.Int32(3))
	rec.Details[0] = count

	if err := handleDownloadedPinsFromLeader(rec); err == nil {
		t.Fatalf("expected an error when the batch is shorter than announced")
	}
}

// A late pin must not panic the node: the producer closes the channel to mark
// the end of a batch, and a send on a closed channel is fatal in Go.
func TestAddDownloadedPinAfterCloseIsSafe(t *testing.T) {
	p := newNodeTxPin()
	p.openPinDownloading()
	p.ClosePinDownloading()

	p.AddDownloadedPin(&pb.TxPin{PinNumber: 1}) // must not panic
	p.ClosePinDownloading()                     // double close must not panic
}

func TestAddDownloadedPinDropsWhenBufferIsFull(t *testing.T) {
	p := &NodeTxPin{}
	p.dlMu.Lock()
	p.downloadedPins = make(chan *pb.TxPin, 1)
	p.dlClosed = false
	p.dlMu.Unlock()

	p.AddDownloadedPin(&pb.TxPin{PinNumber: 1})
	p.AddDownloadedPin(&pb.TxPin{PinNumber: 2}) // dropped, not blocked

	if got := len(p.downloadedPins); got != 1 {
		t.Fatalf("buffered %d pins, want 1", got)
	}
}

// A catch-up response is one gossip message, so it has to fit in one. Under a
// validator quorum a commit transaction carries every transaction it settles,
// which at load is megabytes each; nine of them went over the pubsub limit and
// were dropped while the receiving side read them. Nothing reports that: the
// sender logs a successful send, and the node that asked for the range simply
// never hears back. Observed on a four-validator network - a restarted
// validator asked for pins 11 to 19 and timed out two minutes later, still at
// pin 10.
func TestACatchUpBatchFitsInOneMessage(t *testing.T) {
	const budget = config.MB / 2
	// Each of these is comfortably over a tenth of the budget, so only a few
	// can travel together.
	pins := make([]*pb.TxPin, 0, 20)
	for i := 0; i < 20; i++ {
		pin := pb.NewTxPin(make([]byte, 100*1024))
		pin.PinNumber = int64(i)
		pins = append(pins, pin)
	}

	batch, size := pinsThatFit(pins, budget)
	if size > budget {
		t.Fatalf("a batch of %d pin(s) came to %d bytes, over the %d-byte budget", len(batch), size, budget)
	}
	if len(batch) == 0 {
		t.Fatal("a batch of nothing can never make progress")
	}
	if len(batch) == len(pins) {
		t.Fatal("every pin fitted, so the budget was not exercised")
	}
	// And the batch is the front of the range, so the requester can ask again
	// from where it left off.
	for i, pin := range batch {
		if pin.PinNumber != int64(i) {
			t.Fatalf("batch position %d holds pin %d; the batch is not the front of the range", i, pin.PinNumber)
		}
	}
}

// One commit transaction larger than the whole budget still travels. A batch of
// none can never make progress, and an oversized commit transaction is a
// problem to see rather than to stall on quietly.
func TestACatchUpBatchAlwaysSendsAtLeastOnePin(t *testing.T) {
	huge := pb.NewTxPin(make([]byte, 4*config.MB))
	huge.PinNumber = 5

	batch, _ := pinsThatFit([]*pb.TxPin{huge}, config.MB/2)
	if len(batch) != 1 {
		t.Fatalf("a single oversized commit transaction produced a batch of %d", len(batch))
	}
}

// The budget follows peer.msize, so an operator who raises the pubsub limit
// raises this with it, and a node with no setting still gets a sane one rather
// than a budget of zero - which would send one pin per message for ever.
func TestTheCatchUpBudgetIsBounded(t *testing.T) {
	got := catchUpBudget()
	if got <= 0 {
		t.Fatalf("the catch-up budget is %d bytes", got)
	}
	if cfg := config.GetConfig(); cfg != nil && cfg.Peer.Msize > 0 {
		if want := cfg.Peer.Msize * config.MB / 2; got != want {
			t.Fatalf("budget is %d, want half of peer.msize (%d)", got, want)
		}
		return
	}
	if got != 8*config.MB {
		t.Fatalf("with no peer.msize the budget is %d, want the 8 MB fallback", got)
	}
}

// A site can be in the graph before its transaction is: an approval target
// named by a site that arrived first is inserted as a placeholder and filled in
// when the transaction turns up. The depth notifier asked every site what kind
// of transaction it carried, which on a placeholder is a nil dereference and
// takes the whole node down. Seen on a validator catching up eleven commit
// transactions at once.
func TestTheDepthNotifierSurvivesASiteWithNoTransactionYet(t *testing.T) {
	placeholder := &Node{id: NodeID{id: uuid.New(), idMajor: 1}}
	if placeholder.tx != nil {
		t.Fatal("the placeholder has a transaction, so the test proves nothing")
	}
	// Panics before the guard; returns quietly after it.
	DepthHandler{}.Notify(TxVL{vertex: *placeholder})
}

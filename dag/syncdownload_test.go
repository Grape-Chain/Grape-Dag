package dag

import (
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

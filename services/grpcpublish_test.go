package services

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A publish request with no transaction in it must not take the node down.
//
// PublishTx is reachable by anyone who can open a gRPC connection, and the
// server is created with no recovery interceptor, so grpc-go lets a panic in a
// handler unwind into the process. Txv1.UnmarshalBinary reads its argument's
// fields directly rather than through the generated getters, and direct field
// access on a nil message is a nil dereference - so an empty request was one
// packet that stopped a node.
func TestPublishingNothingDoesNotCrashTheNode(t *testing.T) {
	srv := &RoboTraderServer{}
	cases := []struct {
		name string
		req  *pb.TxPublishRequest
	}{
		{"no transaction", &pb.TxPublishRequest{}},
		{"an explicitly nil transaction", &pb.TxPublishRequest{Tx: nil}},
		{"no request at all", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := srv.PublishTx(context.Background(), c.req)
			if err != nil {
				t.Fatalf("PublishTx returned a transport error rather than a refusal: %s", err)
			}
			if res == nil {
				t.Fatal("PublishTx returned no response and no error")
			}
			// The exact status, not just a non-zero one. The recover inside
			// readPublishedTx would also turn this into a refusal, so a test that
			// only checked for "refused" would pass with the empty-request case
			// deleted, and a client would then be told its transaction was
			// unreadable when in fact it sent none.
			if res.GetStatus() != publishNoTx {
				t.Errorf("PublishTx answered status %d (%q) for %s, want %d - a missing transaction and an unreadable one are different problems",
					res.GetStatus(), res.GetMsg(), c.name, publishNoTx)
			}
		})
	}
}

// The counters behind accepted-versus-offered have to move on the path the
// benchmark actually uses. They were wired only into SendRawTransaction, the
// REST entry point, so a five-minute gRPC saturation run that offered 739,785
// transactions left grape_tx_accepted_total reading 0 and the whole ingress
// histogram empty. A metric that stays at zero under load is worse than a
// missing one: it reads as an answer.
func TestPublishingOverGrpcIsCounted(t *testing.T) {
	srv := &RoboTraderServer{}
	acceptedBefore := counterValue(t, "grape_tx_accepted_total")
	ingressBefore := histogramCount(t, "grape_tx_ingress_seconds")

	res, err := srv.PublishTx(context.Background(), &pb.TxPublishRequest{Tx: publishablePb()})
	if err != nil {
		t.Fatalf("publishing a well-formed transaction: %s", err)
	}
	if res.GetStatus() != 0 {
		t.Fatalf("a well-formed transaction was refused with status %d: %s", res.GetStatus(), res.GetMsg())
	}

	if after := counterValue(t, "grape_tx_accepted_total"); after <= acceptedBefore {
		t.Errorf("grape_tx_accepted_total went %v -> %v across a gRPC publish; an ingress rate read from /metrics would be silently zero", acceptedBefore, after)
	}
	if after := histogramCount(t, "grape_tx_ingress_seconds"); after <= ingressBefore {
		t.Errorf("grape_tx_ingress_seconds recorded %v observations before and %v after a gRPC publish", ingressBefore, after)
	}
}

// A refusal has to be counted as a refusal, or accepted-versus-offered reads 1.0
// however much the node is throwing away.
func TestARefusedPublishIsCountedAsRejected(t *testing.T) {
	srv := &RoboTraderServer{}
	before := counterValue(t, "grape_tx_rejected_total")
	if _, err := srv.PublishTx(context.Background(), &pb.TxPublishRequest{}); err != nil {
		t.Fatalf("refusing an empty request: %s", err)
	}
	if after := counterValue(t, "grape_tx_rejected_total"); after <= before {
		t.Errorf("grape_tx_rejected_total went %v -> %v across a refused publish", before, after)
	}
}

func publishablePb() *pb.Txv1 {
	return &pb.Txv1{
		TxType:    0,
		ChainType: pb.ChainType(chaintype),
		Sender:    make([]byte, addressBytes),
		Recepient: make([]byte, addressBytes),
		Amount:    big.NewInt(500000).Bytes(),
		FuelLimit: big.NewInt(0).Bytes(),
		FuelPrice: big.NewInt(0).Bytes(),
		Nonce:     7,
	}
}

// A transaction whose sender is not an address is refused rather than queued,
// and refusing it must not panic. It reached a panic three ways before this: the
// nil dereference in the unmarshaller, the log line that renders the transaction
// as JSON, and BytesToAddress inside that rendering.
func TestAPublishedTransactionWithoutASenderIsRefused(t *testing.T) {
	srv := &RoboTraderServer{}
	for _, sender := range [][]byte{nil, {}, make([]byte, 19), make([]byte, 21)} {
		bad := publishablePb()
		bad.Sender = sender
		res, err := srv.PublishTx(context.Background(), &pb.TxPublishRequest{Tx: bad})
		if err != nil {
			t.Fatalf("a %d-byte sender produced a transport error rather than a refusal: %s", len(sender), err)
		}
		if res.GetStatus() == 0 {
			t.Errorf("a %d-byte sender address was accepted", len(sender))
		}
	}
}

// counterValue sums every series in a counter family, so a labelled counter is
// read as its total. Returns 0 when the family has not been touched at all,
// which is the state this test file exists to notice.
func counterValue(t *testing.T, name string) float64 {
	t.Helper()
	families, err := stats.Registry().Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %s", err)
	}
	total := 0.0
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			total += m.GetCounter().GetValue()
		}
	}
	return total
}

func histogramCount(t *testing.T, name string) uint64 {
	t.Helper()
	families, err := stats.Registry().Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %s", err)
	}
	var total uint64
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			total += m.GetHistogram().GetSampleCount()
		}
	}
	return total
}

// The interceptor is the backstop for the handlers nobody has audited yet, so it
// has to actually catch. Tested directly rather than through a server, because
// what matters is that it converts the panic into an error for that one call and
// does not let it reach the process.
func TestTheInterceptorTurnsAPanicIntoAnError(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/pb.RoboTrader/Exploding"}
	panicking := func(context.Context, any) (any, error) { panic("a handler dereferenced something") }

	resp, err := recoverUnary(context.Background(), nil, info, panicking)
	if err == nil {
		t.Fatal("a panicking handler produced no error - the panic reached the process")
	}
	if resp != nil {
		t.Errorf("a panicking handler produced a response: %v", resp)
	}
	if got := status.Code(err); got != codes.Internal {
		t.Errorf("a recovered panic reported code %s, want %s", got, codes.Internal)
	}
}

// WatchDog divides its timeout by its retry count, and an integer division by
// zero panics, so a request with no retries was the second one-packet way to
// stop a node.
func TestWatchDogRefusesZeroRetries(t *testing.T) {
	srv := &RoboTraderServer{}
	for _, retries := range []int32{0, -1} {
		_, err := srv.WatchDog(context.Background(), &pb.WatchDogRequest{Timeout: int64(time.Second), Retries: retries})
		if err == nil {
			t.Errorf("WatchDog accepted %d retries", retries)
			continue
		}
		if got := status.Code(err); got != codes.InvalidArgument {
			t.Errorf("WatchDog with %d retries reported %s, want %s", retries, got, codes.InvalidArgument)
		}
	}
}

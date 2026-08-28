package diffusion

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/peer"
)

/*
The property every test here is circling is the same one: an unverifiable
transaction must never reach the insert queue, whatever the concurrency does.
Everything else - order, bounds, throughput - is secondary to that, so the
tests are written so that breaking the gate breaks them.
*/

// signedTx - a payment transaction with a signature that checks out.
//
// nonce is carried through so a test can tell one message from another in a
// stream, which is what makes the ordering test possible.
func signedTx(t testing.TB, nonce uint64) *tx.Txv1 {
	t.Helper()
	w := grape_wallet.NewWallet()
	x := tx.NewTxv1(tx.PRIVATE_TESTNET)
	x.Tx_Type = tx.PAYMENT
	x.Sender_Pubk = *w.PublicKey()
	x.Sender = grape_wallet.AddressToBytes(w.WalletAddress())
	x.Recepient = grape_wallet.AddressToBytes(grape_wallet.NewWallet().WalletAddress())
	x.Amount = big.NewInt(1000).Bytes()
	x.Nonce = nonce
	x.Timestamp = time.Now()
	x.Fuel_Limit = big.NewInt(21000).Bytes()
	x.Fuel_Price = big.NewInt(1).Bytes()
	x.Sign(w.PrivateKey())
	if err := x.VerifySignature(); err != nil {
		t.Fatalf("a freshly signed transaction should verify, got: %s", err.Error())
	}
	return x
}

// tamperedTx - a transaction whose signature does not belong to it.
//
// The amount is changed after signing rather than the signature being
// scribbled on, because that is the interesting forgery: the signature is a
// real signature, of a different transaction.
func tamperedTx(t testing.TB, nonce uint64) *tx.Txv1 {
	t.Helper()
	x := signedTx(t, nonce)
	x.Amount = big.NewInt(999999).Bytes()
	if err := x.VerifySignature(); err == nil {
		t.Fatal("a transaction altered after signing must not verify")
	}
	return x
}

// record - a gossip payload carrying one transaction, as it arrives on the wire.
func record(t testing.TB, transaction tx.Transaction) []byte {
	t.Helper()
	rec := &tx.GrapeTx{
		Tx:          uuid.New().String(),
		Version:     tx.VersionType(tx.TVX1),
		Ids:         tx.Ids{},
		Transaction: transaction,
	}
	raw, err := rec.MarshalRecord()
	if err != nil {
		t.Fatalf("marshalling the record: %s", err.Error())
	}
	return raw
}

// sink - collects what the pipeline delivers, and refuses anything that did not
// come through the gate.
//
// The refusal is the point. A test that only counted deliveries would pass with
// verification removed; this one records a failure, so removing the gate is
// caught wherever the sink is used.
type sink struct {
	mu       sync.Mutex
	nonces   []uint64
	unverifi int
}

func (s *sink) deliver(v verifiedRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !v.verifiedFor() {
		s.unverifi++
		return
	}
	s.nonces = append(s.nonces, v.rec.Transaction.GetNonce())
}

func (s *sink) result() ([]uint64, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.nonces...), s.unverifi
}

func TestAnUnverifiableTransactionNeverReachesTheInsertQueue(t *testing.T) {
	const valid, invalid = 200, 200
	raws := make([][]byte, 0, valid+invalid)
	wantNonces := map[uint64]bool{}
	for i := 0; i < valid+invalid; i++ {
		// Interleaved so that valid and invalid messages are in flight in the
		// same batch on every worker, which is the case a serial verifier
		// cannot get wrong and a parallel one can.
		if i%2 == 0 {
			nonce := uint64(1000 + i)
			raws = append(raws, record(t, signedTx(t, nonce)))
			wantNonces[nonce] = true
		} else {
			raws = append(raws, record(t, tamperedTx(t, uint64(i))))
		}
	}

	s := &sink{}
	p := startVerifyPipeline(8, s.deliver)
	for _, raw := range raws {
		p.submit(raw, peer.ID("test-peer"))
	}
	p.stop()

	got, unverified := s.result()
	if unverified != 0 {
		t.Fatalf("%d records reached the sink without having been verified", unverified)
	}
	if len(got) != valid {
		t.Fatalf("expected %d verified records to be delivered, got %d", valid, len(got))
	}
	for _, nonce := range got {
		if !wantNonces[nonce] {
			t.Fatalf("a record that should have been refused was delivered (nonce %d)", nonce)
		}
	}
}

// TestAnUnverifiableTransactionIsRefusedHoweverManyAreInFlight - the same
// property under submission from several goroutines at once, which is how the
// race detector gets a chance to see the pipeline's internals.
func TestAnUnverifiableTransactionIsRefusedHoweverManyAreInFlight(t *testing.T) {
	const submitters, each = 8, 40
	s := &sink{}
	p := startVerifyPipeline(8, s.deliver)

	var delivered atomic.Int64
	wg := sync.WaitGroup{}
	for i := 0; i < submitters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < each; j++ {
				if (id+j)%3 == 0 {
					p.submit(record(t, signedTx(t, uint64(id*1000+j))), peer.ID("test-peer"))
					delivered.Add(1)
					continue
				}
				p.submit(record(t, tamperedTx(t, uint64(j))), peer.ID("test-peer"))
			}
		}(i)
	}
	wg.Wait()
	p.stop()

	got, unverified := s.result()
	if unverified != 0 {
		t.Fatalf("%d records reached the sink without having been verified", unverified)
	}
	if int64(len(got)) != delivered.Load() {
		t.Fatalf("expected %d verified records to be delivered, got %d", delivered.Load(), len(got))
	}
}

// TestTheVerifierConcurrencyIsBounded - an unbounded goroutine or channel slot
// per message would let whoever is sending decide how much memory the node
// uses, so the bound is a security property rather than a tidiness one.
func TestTheVerifierConcurrencyIsBounded(t *testing.T) {
	const workers = 4
	var inFlight, peak atomic.Int64

	// A sink that is slower than the workers, so the pipeline is under pressure
	// for the whole test rather than draining as fast as it fills.
	slow := func(verifiedRecord) {
		time.Sleep(200 * time.Microsecond)
		inFlight.Add(-1)
	}
	p := startVerifyPipeline(workers, slow)
	limit := int64(p.maxInFlight())

	for i := 0; i < 400; i++ {
		raw := record(t, signedTx(t, uint64(i)))
		n := inFlight.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		p.submit(raw, peer.ID("test-peer"))
	}
	p.stop()

	if peak.Load() > limit {
		t.Fatalf("in-flight records reached %d, above the bound of %d", peak.Load(), limit)
	}
	if peak.Load() < 2 {
		t.Fatalf("in-flight records peaked at %d: the pipeline was never concurrent, so this test proves nothing", peak.Load())
	}
	if got := p.maxInFlight(); got != workers*inFlightPerWorker+2 {
		t.Fatalf("the bound should be workers*inFlightPerWorker+2 = %d, got %d", workers*inFlightPerWorker+2, got)
	}
}

// TestAValidStreamIsDeliveredInArrivalOrder - the pipeline verifies out of
// order and delivers in order.
//
// Nothing downstream needs this: InsertTxDag records approvals it cannot
// resolve in missingTargets and a ticker reconciles them, and gossip offers no
// ordering across peers in the first place. It is preserved because it costs
// one bounded channel, and because the alternative is arguing about it.
func TestAValidStreamIsDeliveredInArrivalOrder(t *testing.T) {
	const count = 300
	s := &sink{}
	p := startVerifyPipeline(8, s.deliver)
	for i := 0; i < count; i++ {
		p.submit(record(t, signedTx(t, uint64(i))), peer.ID("test-peer"))
	}
	p.stop()

	got, unverified := s.result()
	if unverified != 0 {
		t.Fatalf("%d records reached the sink without having been verified", unverified)
	}
	if len(got) != count {
		t.Fatalf("expected %d records, got %d", count, len(got))
	}
	for i, nonce := range got {
		if nonce != uint64(i) {
			t.Fatalf("record %d arrived as nonce %d: delivery is not in arrival order", i, nonce)
		}
	}
}

func TestAMalformedRecordIsDiscardedRatherThanStoppingTheSubscriber(t *testing.T) {
	s := &sink{}
	p := startVerifyPipeline(4, s.deliver)
	// Rubbish that is not a protobuf at all, an empty payload, and a valid one
	// after them: the point is that the third still arrives.
	p.submit([]byte{0xff, 0xfe, 0xfd, 0x01, 0x02}, peer.ID("test-peer"))
	p.submit(nil, peer.ID("test-peer"))
	p.submit(record(t, signedTx(t, 7)), peer.ID("test-peer"))
	p.stop()

	got, unverified := s.result()
	if unverified != 0 {
		t.Fatalf("%d records reached the sink without having been verified", unverified)
	}
	if len(got) != 1 || got[0] != 7 {
		t.Fatalf("expected only the one good record to be delivered, got %v", got)
	}
}

// TestARecordSwappedAfterVerificationIsRefused - the gate says "this
// transaction was verified", not "something was verified".
func TestARecordSwappedAfterVerificationIsRefused(t *testing.T) {
	verified := verifyRecord(record(t, signedTx(t, 1)), peer.ID("test-peer"))
	if !verified.verifiedFor() {
		t.Fatal("a valid record should pass the gate")
	}

	swapped := verified
	swapped.rec = &tx.GrapeTx{
		Tx:          uuid.New().String(),
		Version:     tx.VersionType(tx.TVX1),
		Transaction: tamperedTx(t, 2),
	}
	if swapped.verifiedFor() {
		t.Fatal("a record swapped after verification must not pass the gate")
	}
}

func TestARecordThatWasNeverVerifiedIsRefused(t *testing.T) {
	never := verifiedRecord{rec: &tx.GrapeTx{Transaction: signedTx(t, 1)}}
	if never.verifiedFor() {
		t.Fatal("a record with no recorded verification must not pass the gate")
	}
	if (verifiedRecord{}).verifiedFor() {
		t.Fatal("the zero record must not pass the gate")
	}
}

func TestVerifyRecordRefusesAnAlteredTransaction(t *testing.T) {
	if v := verifyRecord(record(t, tamperedTx(t, 1)), peer.ID("test-peer")); v.rec != nil {
		t.Fatal("a record whose transaction was altered after signing must be refused")
	}
	if v := verifyRecord(record(t, signedTx(t, 1)), peer.ID("test-peer")); v.rec == nil {
		t.Fatal("a record with a good signature must be accepted")
	}
}

// TestARecordWithAnUnusableTxIdIsRefused - rec.Tx used to be handed to
// uuid.MustParse on the subscriber goroutine, so one message with a malformed
// id stopped the node.
func TestARecordWithAnUnusableTxIdIsRefused(t *testing.T) {
	rec := &tx.GrapeTx{
		Tx:          "not-a-uuid",
		Version:     tx.VersionType(tx.TVX1),
		Ids:         tx.Ids{},
		Transaction: signedTx(t, 1),
	}
	raw, err := rec.MarshalRecord()
	if err != nil {
		t.Fatalf("marshalling the record: %s", err.Error())
	}
	if v := verifyRecord(raw, peer.ID("test-peer")); v.rec != nil {
		t.Fatal("a record whose tx id is not a uuid must be refused, not panic the subscriber")
	}
}

func TestTheWorkerCountIsClampedToSomethingSensible(t *testing.T) {
	for _, env := range []string{"", "0", "-4", "not-a-number", "1", "9999"} {
		t.Setenv("GRAPE_VERIFY_WORKERS", env)
		got := verifyWorkers()
		if got < verifyWorkersMin || got > verifyWorkersMax {
			t.Fatalf("GRAPE_VERIFY_WORKERS=%q gave %d workers, outside [%d, %d]",
				env, got, verifyWorkersMin, verifyWorkersMax)
		}
	}
	t.Setenv("GRAPE_VERIFY_WORKERS", "6")
	if got := verifyWorkers(); got != 6 {
		t.Fatalf("an explicit worker count of 6 should be honoured, got %d", got)
	}
}

/*
Benchmarks.

BenchmarkVerifyRecord is the serial cost of one message: unmarshal plus the
ed25519 verify. BenchmarkVerifyRecordParallel is the same work under
b.RunParallel, which is the measurement that matters, because the question the
pipeline answers is whether this work scales with cores. If the parallel figure
per operation does not fall as GOMAXPROCS rises, fanning it out was pointless.
*/

func BenchmarkVerifyRecord(b *testing.B) {
	raw := record(b, signedTx(b, 1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if v := verifyRecord(raw, peer.ID("bench")); v.rec == nil {
			b.Fatal("the record should verify")
		}
	}
}

func BenchmarkVerifyRecordParallel(b *testing.B) {
	raw := record(b, signedTx(b, 1))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if v := verifyRecord(raw, peer.ID("bench")); v.rec == nil {
				b.Fatal("the record should verify")
			}
		}
	})
}

// BenchmarkVerifyPipeline - end to end through the pipeline, so the fan-out,
// the ordered delivery and the job pooling are all in the measurement. Reported
// per message submitted.
func BenchmarkVerifyPipeline(b *testing.B) {
	for _, workers := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			raw := record(b, signedTx(b, 1))
			var seen atomic.Int64
			p := startVerifyPipeline(workers, func(v verifiedRecord) {
				if !v.verifiedFor() {
					panic("an unverified record reached the sink")
				}
				seen.Add(1)
			})
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p.submit(raw, peer.ID("bench"))
			}
			p.stop()
			b.StopTimer()
			if seen.Load() != int64(b.N) {
				b.Fatalf("submitted %d records, delivered %d", b.N, seen.Load())
			}
		})
	}
}

// TestATransactionWithAMalformedPublicKeyIsRefusedRatherThanCrashingTheNode -
// crypto/ed25519.Verify panics on a public key that is not 32 bytes, and the
// key arrives from the network. One such message stopped the whole process.
func TestATransactionWithAMalformedPublicKeyIsRefusedRatherThanCrashingTheNode(t *testing.T) {
	for _, size := range []int{1, 5, 31, 33, 64} {
		x := signedTx(t, 1)
		if size < len(x.Sender_Pubk) {
			x.Sender_Pubk = x.Sender_Pubk[:size]
		} else {
			x.Sender_Pubk = append(x.Sender_Pubk, make([]byte, size-len(x.Sender_Pubk))...)
		}
		raw := record(t, x)
		if v := verifyRecord(raw, peer.ID("test-peer")); v.rec != nil {
			t.Fatalf("a transaction with a %d-byte public key must be refused", size)
		}
	}
}

// TestAMalformedPublicKeyDoesNotStopTheStream - the same message, in a stream,
// through the real pipeline: the bad one is dropped and the good ones after it
// still arrive.
func TestAMalformedPublicKeyDoesNotStopTheStream(t *testing.T) {
	bad := signedTx(t, 1)
	bad.Sender_Pubk = bad.Sender_Pubk[:5]

	s := &sink{}
	p := startVerifyPipeline(4, s.deliver)
	p.submit(record(t, signedTx(t, 10)), peer.ID("test-peer"))
	p.submit(record(t, bad), peer.ID("test-peer"))
	p.submit(record(t, signedTx(t, 11)), peer.ID("test-peer"))
	p.stop()

	got, unverified := s.result()
	if unverified != 0 {
		t.Fatalf("%d records reached the sink without having been verified", unverified)
	}
	if len(got) != 2 || got[0] != 10 || got[1] != 11 {
		t.Fatalf("expected the two good records either side of the bad one, got %v", got)
	}
}

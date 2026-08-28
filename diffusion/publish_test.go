package diffusion

import (
	"testing"

	txqueue "github.com/Grape-Chain/Grape-Dag/queues"
	"github.com/Grape-Chain/Grape-Dag/tx"
)

/*
A transaction on the publish queue is one a client has already been told was
accepted. Losing it is not a dropped packet, it is a payment that was
acknowledged and never happened - so the property under test is that a refused
insert does not lose the transaction, and that when it is eventually given up on
it is given up on visibly.
*/

func publishSourceFor(t testing.TB, txs ...tx.Transaction) *publishSource {
	t.Helper()
	// A queue of its own, not the package singleton, so this test neither sees
	// nor leaves anything for the rest of the package.
	q := txqueue.NewLockFreeQueueCapOf[any](true, 64)
	for _, x := range txs {
		q.Enqueue(x)
	}
	return &publishSource{queue: q}
}

// TestARefusedTransactionIsOfferedAgainRatherThanLost - the bug this replaces
// was a bare continue after the dequeue.
func TestARefusedTransactionIsOfferedAgainRatherThanLost(t *testing.T) {
	first, second := signedTx(t, 1), signedTx(t, 2)
	s := publishSourceFor(t, first, second)

	got, _ := s.next()
	if got != tx.Transaction(first) {
		t.Fatal("expected the first transaction off the queue")
	}
	if !s.refused(got) {
		t.Fatal("one refusal should not be enough to give up on a transaction")
	}

	// The same transaction, and the queue untouched behind it.
	again, depth := s.next()
	if again != tx.Transaction(first) {
		t.Fatal("a refused transaction must be offered again, not replaced by the next one")
	}
	if depth != 1 {
		t.Fatalf("expected the second transaction still to be queued, depth %d", depth)
	}

	s.accepted()
	next, _ := s.next()
	if next != tx.Transaction(second) {
		t.Fatal("once accepted, the queue should move on to the next transaction")
	}
}

// TestARepeatedlyRefusedTransactionIsDroppedRatherThanRetriedForever - the
// publisher is the queue's only consumer, so a transaction that can never be
// inserted would otherwise stall every payment behind it indefinitely.
func TestARepeatedlyRefusedTransactionIsDroppedRatherThanRetriedForever(t *testing.T) {
	doomed, following := signedTx(t, 1), signedTx(t, 2)
	s := publishSourceFor(t, doomed, following)

	got, _ := s.next()
	attempts := 0
	for {
		attempts++
		if !s.refused(got) {
			break
		}
		if attempts > maxPublishAttempts*10 {
			t.Fatal("a refused transaction is being retried without limit")
		}
		if again, _ := s.next(); again != got {
			t.Fatal("expected the same transaction to be retried")
		}
	}
	if attempts != maxPublishAttempts {
		t.Fatalf("expected the transaction to be dropped after %d attempts, took %d", maxPublishAttempts, attempts)
	}

	// Dropped, and the queue moves on. A stalled publisher would be worse than
	// the drop.
	next, _ := s.next()
	if next != tx.Transaction(following) {
		t.Fatal("after a drop the publisher must move on to the next transaction")
	}
}

// TestADroppedPublishIsCounted - the drop is acceptable; a silent drop is not.
// This is what tells an operator that accepted-versus-inserted does not
// reconcile, and why.
func TestADroppedPublishIsCounted(t *testing.T) {
	for _, reason := range []string{dropUnusable, dropInsertRefused, dropMarshal, dropNotATransaction} {
		before, _ := labelledValue(t, scrape(t), "grape_tx_publish_dropped_total", "reason", reason)

		s := publishSourceFor(t, signedTx(t, 1))
		got, _ := s.next()
		if got == nil {
			t.Fatal("expected a transaction")
		}
		s.drop(reason)

		after, ok := labelledValue(t, scrape(t), "grape_tx_publish_dropped_total", "reason", reason)
		if !ok || after != before+1 {
			t.Fatalf("a drop for reason %q was not counted: %v -> %v (present=%t)", reason, before, after, ok)
		}
		if s.pending != nil || s.attempts != 0 {
			t.Fatalf("a dropped transaction must not stay held (pending=%v attempts=%d)", s.pending != nil, s.attempts)
		}
	}
}

// TestSomethingThatIsNotATransactionOnThePublishQueueIsCountedNotPanicked - the
// dequeue used to be an unchecked type assertion on the publisher goroutine, and
// an unrecovered panic there stops the node.
func TestSomethingThatIsNotATransactionOnThePublishQueueIsCountedNotPanicked(t *testing.T) {
	q := txqueue.NewLockFreeQueueCapOf[any](true, 8)
	q.Enqueue("not a transaction at all")
	s := &publishSource{queue: q}

	before, _ := labelledValue(t, scrape(t), "grape_tx_publish_dropped_total", "reason", dropNotATransaction)
	got, _ := s.next()
	if got != nil {
		t.Fatal("a value that is not a transaction must not be returned as one")
	}
	after, ok := labelledValue(t, scrape(t), "grape_tx_publish_dropped_total", "reason", dropNotATransaction)
	if !ok || after != before+1 {
		t.Fatalf("the discarded value was not counted: %v -> %v (present=%t)", before, after, ok)
	}
}

func TestAnEmptyPublishQueueYieldsNothingRatherThanBlockingForever(t *testing.T) {
	// An unsynchronised queue is the one that answers "nothing there" rather
	// than waiting, which is what makes this checkable without a timeout.
	s := &publishSource{queue: txqueue.NewLockFreeQueueCapOf[any](false, 0)}
	if got, _ := s.next(); got != nil {
		t.Fatalf("expected nothing from an empty queue, got %v", got)
	}
}

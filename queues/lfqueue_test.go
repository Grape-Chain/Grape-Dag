package txqueue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAQueuePreservesFifoOrder(t *testing.T) {
	q := NewLockFreeQueueCapOf[int](true, 1024)
	const count = 500
	for i := 0; i < count; i++ {
		q.Enqueue(i)
	}
	for i := 0; i < count; i++ {
		got, sz, ok := q.TryDequeue()
		if !ok {
			t.Fatalf("item %d: the queue should have had something to return", i)
		}
		if got != i {
			t.Fatalf("expected %d, got %d", i, got)
		}
		if want := int64(count - i - 1); sz != want {
			t.Fatalf("expected a remaining size of %d, got %d", want, sz)
		}
	}
}

// TestEveryItemComesOutOfTheQueueExactlyOnceUnderContention - the retry path in
// TryDequeue used to take a wake token on every attempt, so a consumer that lost
// a compare-and-swap consumed a second token. The token count then drifted below
// the item count and consumers eventually waited on items that were already in
// the queue: a queue that stalled with work in it.
//
// The failure needs several consumers racing to show up at all, which is why
// this is a contention test rather than a unit one.
func TestEveryItemComesOutOfTheQueueExactlyOnceUnderContention(t *testing.T) {
	const producers, perProducer, consumers = 8, 500, 8
	const total = producers * perProducer

	// Room for every item plus one stop sentinel per consumer, so nothing in
	// this test blocks on the ceiling and the only thing under test is the
	// token accounting.
	q := NewLockFreeQueueCapOf[int](true, total+consumers)
	seen := make([]atomic.Int32, total)
	var taken atomic.Int64

	consumerWg := sync.WaitGroup{}
	for i := 0; i < consumers; i++ {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			for {
				v, _, ok := q.TryDequeue()
				if !ok {
					continue
				}
				if v < 0 {
					// The stop sentinel, one per consumer.
					return
				}
				seen[v].Add(1)
				taken.Add(1)
			}
		}()
	}

	producerWg := sync.WaitGroup{}
	for p := 0; p < producers; p++ {
		producerWg.Add(1)
		go func(base int) {
			defer producerWg.Done()
			for j := 0; j < perProducer; j++ {
				q.Enqueue(base*perProducer + j)
			}
		}(p)
	}
	producerWg.Wait()

	// The failure this test exists for looks exactly like this: items in the
	// queue and consumers not being woken for them.
	deadline := time.Now().Add(10 * time.Second)
	for taken.Load() < total {
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d items came out of the queue; %d are still in it with consumers waiting",
				taken.Load(), total, q.Len())
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < consumers; i++ {
		q.Enqueue(-1)
	}
	consumerWg.Wait()

	for i := range seen {
		if got := seen[i].Load(); got != 1 {
			t.Fatalf("item %d came out of the queue %d times, expected exactly once", i, got)
		}
	}
	if q.Len() != 0 {
		t.Fatalf("the queue should be empty, %d items left", q.Len())
	}
}

// TestAFullQueueHoldsTheProducerRatherThanLosingTheItem - the ceiling is
// backpressure, not a drop. Which of the two it is decides whether an
// accepted-transaction rate measured at ingress describes the node or the queue.
func TestAFullQueueHoldsTheProducerRatherThanLosingTheItem(t *testing.T) {
	const capacity = 4
	q := NewLockFreeQueueCapOf[int](true, capacity)
	for i := 0; i < capacity; i++ {
		q.Enqueue(i)
	}
	if q.EnqueueBlocked() != 0 {
		t.Fatalf("filling the queue to capacity should not have blocked, got %d", q.EnqueueBlocked())
	}

	blocked := make(chan struct{})
	go func() {
		q.Enqueue(capacity)
		close(blocked)
	}()

	select {
	case <-blocked:
		t.Fatal("an enqueue past the ceiling returned without waiting; the ceiling is not backpressure")
	case <-time.After(50 * time.Millisecond):
	}

	// One dequeue makes room, and the held producer completes.
	if _, _, ok := q.TryDequeue(); !ok {
		t.Fatal("the queue should have had something to return")
	}
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("the held producer should have completed once there was room")
	}

	if q.EnqueueBlocked() != 1 {
		t.Fatalf("expected one blocked enqueue to be recorded, got %d", q.EnqueueBlocked())
	}
	if q.EnqueueWaitSeconds() <= 0 {
		t.Fatal("expected the time spent waiting to be recorded")
	}
	// Nothing was lost: capacity items plus the one that waited.
	count := 0
	for {
		_, _, ok := q.TryDequeue()
		if !ok {
			break
		}
		count++
		if count == capacity {
			break
		}
	}
	if count != capacity {
		t.Fatalf("expected %d items still in the queue, got %d", capacity, count)
	}
}

func TestTheQueueCeilingIsExplicitAndConfigurable(t *testing.T) {
	if DefaultSyncCapacity != 1<<15 {
		t.Fatalf("the documented default ceiling is 1<<15, got %d", DefaultSyncCapacity)
	}
	if got := NewLockFreeQueue(true).Capacity(); got != int64(syncCapacity) {
		t.Fatalf("a synchronised queue should report the resolved ceiling %d, got %d", syncCapacity, got)
	}
	// An unsynchronised queue never waits, so it has no ceiling to report.
	if got := NewLockFreeQueue(false).Capacity(); got != 0 {
		t.Fatalf("an unsynchronised queue has no ceiling, got %d", got)
	}
	if got := NewLockFreeQueueCapOf[int](true, 7).Capacity(); got != 7 {
		t.Fatalf("expected an explicit ceiling of 7, got %d", got)
	}
}

func TestTheQueueCeilingEnvironmentOverrideIsValidated(t *testing.T) {
	if got := resolveSyncCapacity("4096"); got != 4096 {
		t.Fatalf("expected the override to be honoured, got %d", got)
	}
	for _, bad := range []string{"", "0", "-1", "lots", "32k"} {
		if got := resolveSyncCapacity(bad); got != DefaultSyncCapacity {
			t.Fatalf("GRAPE_QUEUE_CAPACITY=%q should have fallen back to the default, got %d", bad, got)
		}
	}
	// Zero would deadlock the node on its first transaction, which is why an
	// unparseable value falls back rather than being taken literally.
	if resolveSyncCapacity("0") <= 0 {
		t.Fatal("the resolved ceiling must always be positive")
	}
}

// TestTheInterfaceQueueStillBehavesAsItDid - the interface{} queue is what the
// publish path and the stats queue use, and their producers are outside this
// package. The alias must keep the nil-on-empty contract those callers test for.
func TestTheInterfaceQueueStillBehavesAsItDid(t *testing.T) {
	var q *LockFreeQueue = NewLockFreeQueue(false)
	if v, sz := q.Dequeue(); v != nil || sz != 0 {
		t.Fatalf("an empty interface queue should dequeue as (nil, 0), got (%v, %d)", v, sz)
	}
	q.Enqueue("a value")
	v, sz := q.Dequeue()
	if s, ok := v.(string); !ok || s != "a value" {
		t.Fatalf("expected the value back unchanged, got %v", v)
	}
	if sz != 0 {
		t.Fatalf("expected a remaining size of 0, got %d", sz)
	}
}

// BenchmarkEnqueueDequeue - the allocation count is the point. A queue of
// interface{} allocates twice per struct item, once to box it and once for the
// holder; a queue of the concrete type allocates once.
func BenchmarkEnqueueDequeue(b *testing.B) {
	type item struct {
		a, c uint64
		b    [16]byte
	}
	b.Run("typed", func(b *testing.B) {
		q := NewLockFreeQueueCapOf[item](true, b.N+1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			q.Enqueue(item{a: uint64(i)})
		}
		for i := 0; i < b.N; i++ {
			q.TryDequeue()
		}
	})
	b.Run("interface", func(b *testing.B) {
		q := NewLockFreeQueueCapOf[any](true, b.N+1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			q.Enqueue(item{a: uint64(i)})
		}
		for i := 0; i < b.N; i++ {
			q.TryDequeue()
		}
	})
}

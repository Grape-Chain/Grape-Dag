package txgen

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// tokenBucket paces the offered rate of all bench workers together.
//
// A sleep between transactions cannot pace much above a thousand per second. At
// that rate the interval is one millisecond, which is at or below the resolution
// of a Go timer, so a sleep-per-transaction loop settles at whatever the runtime
// happens to deliver rather than at the rate that was asked for. That is the
// ceiling the old ticker-per-transaction loop ran into, and no arithmetic fix to
// the interval can lift it.
//
// The bucket sidesteps timer resolution by handing out permits in batches. It
// refills continuously from the wall clock at rate permits per second, and a
// worker that draws k permits sends k transactions back to back with no sleep at
// all. Workers sleep only when the bucket is empty, and then for the whole time
// it takes to earn one permit, so the number of sleeps per second is bounded by
// the number of workers rather than by the offered rate. capacity bounds how much
// of an idle stretch may be spent as a burst.
//
// A rate of zero means unpaced: take grants immediately and never touches the
// clock or the lock, which is what makes -bench_max find the node's ceiling
// rather than the generator's.
type tokenBucket struct {
	mu       sync.Mutex
	rate     float64 // permits per second, 0 for unpaced
	capacity float64
	tokens   float64
	last     time.Time

	// now is the clock. A field rather than a direct call to time.Now so the
	// tests can step time deterministically instead of sleeping.
	now func() time.Time

	waits atomic.Uint64 // how often a worker had to block, for the tests
}

func newTokenBucket(rate float64, capacity float64) *tokenBucket {
	if capacity < 1 {
		capacity = 1
	}
	b := &tokenBucket{
		rate:     rate,
		capacity: capacity,
		now:      time.Now,
	}
	// Starts empty rather than full. A full bucket would let the first instant of
	// the run offer capacity transactions on top of the configured rate, and a
	// short run reports that overshoot as its offered rate.
	b.last = b.now()
	return b
}

func (b *tokenBucket) unpaced() bool {
	return b.rate <= 0
}

// take grants up to max permits. When it can grant none it grants zero and
// reports how long the caller should wait before asking again.
func (b *tokenBucket) take(max int) (granted int, wait time.Duration) {
	if max < 1 {
		max = 1
	}
	if b.unpaced() {
		return max, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked()
	if b.tokens < 1 {
		// Time to earn the one permit that is missing, rounded up: waking a hair
		// short of it means finding the bucket empty again and sleeping twice for
		// one permit.
		deficit := 1 - b.tokens
		return 0, time.Duration(math.Ceil(deficit / b.rate * float64(time.Second)))
	}
	granted = int(b.tokens)
	if granted > max {
		granted = max
	}
	b.tokens -= float64(granted)
	return granted, 0
}

// refillLocked credits the permits earned since the last call. Permits earned
// beyond capacity are dropped on purpose - that is what stops an idle generator
// from firing a stored-up burst at the node.
func (b *tokenBucket) refillLocked() {
	now := b.now()
	elapsed := now.Sub(b.last)
	if elapsed <= 0 {
		return
	}
	b.last = now
	b.tokens += elapsed.Seconds() * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
}

// acquire blocks until at least one permit is available and returns how many the
// caller may spend. It returns 0 only when ctx is done.
func (b *tokenBucket) acquire(ctx context.Context, max int) int {
	for {
		granted, wait := b.take(max)
		if granted > 0 {
			return granted
		}
		b.waits.Add(1)
		if !sleepCtx(ctx, wait) {
			return 0
		}
	}
}

// sleepCtx waits for d and reports false when ctx finished first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// benchBurst sizes the bucket and the batch a single worker may draw.
//
// Capacity is a tenth of a second's worth of permits: enough that a worker
// blocked on one slow round trip does not throw away the permits it earned while
// waiting, small enough that the catch-up afterwards is not a spike the node
// sees as a different workload. The batch is that capacity split between the
// workers, so no single worker can drain the bucket and starve the others.
func benchBurst(rate float64, workers int) (capacity float64, batch int) {
	if workers < 1 {
		workers = 1
	}
	capacity = rate / 10
	if capacity < float64(workers) {
		capacity = float64(workers)
	}
	batch = int(capacity) / workers
	if batch < 1 {
		batch = 1
	}
	return capacity, batch
}

// txBudget hands out the -bench_txmax allowance so that all workers together
// offer exactly that many transactions and none of them needs to know the total.
type txBudget struct {
	remaining atomic.Int64
	unbounded bool
}

func newTxBudget(max uint64) *txBudget {
	b := &txBudget{unbounded: max == 0}
	b.remaining.Store(int64(max))
	return b
}

// claim reserves up to n transactions and returns how many were actually
// reserved. Zero means the budget is spent.
func (b *txBudget) claim(n int) int {
	if b.unbounded {
		return n
	}
	if n < 1 {
		return 0
	}
	for {
		left := b.remaining.Load()
		if left <= 0 {
			return 0
		}
		take := int64(n)
		if take > left {
			take = left
		}
		if b.remaining.CompareAndSwap(left, left-take) {
			return int(take)
		}
	}
}

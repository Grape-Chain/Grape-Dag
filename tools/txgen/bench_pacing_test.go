package txgen

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock steps time on demand, so the pacing tests assert on the bucket's
// arithmetic rather than on how well the machine's timers behave under load.
type fakeClock struct {
	at time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time          { return c.at }
func (c *fakeClock) advance(d time.Duration) { c.at = c.at.Add(d) }

// bucketOnFakeClock wires a bucket to a stepped clock.
func bucketOnFakeClock(rate, capacity float64) (*tokenBucket, *fakeClock) {
	clock := newFakeClock()
	b := newTokenBucket(rate, capacity)
	b.now = clock.now
	b.last = clock.now()
	return b, clock
}

// drain takes permits until the bucket is empty and reports how many it granted,
// how many draws came back empty, and the largest single grant.
func drain(b *tokenBucket, batch int) (granted, empties, largest int) {
	for {
		n, _ := b.take(batch)
		if n == 0 {
			empties++
			return granted, empties, largest
		}
		granted += n
		if n > largest {
			largest = n
		}
	}
}

func TestTokenBucketOffersTheConfiguredRateOverASecond(t *testing.T) {
	const workers = 32
	for _, rate := range []float64{1, 100, 1000, 10000, 100000} {
		capacity, batch := benchBurst(rate, workers)
		b, clock := bucketOnFakeClock(rate, capacity)

		granted := 0
		for step := 0; step < 10; step++ {
			clock.advance(100 * time.Millisecond)
			n, _, _ := drain(b, batch)
			granted += n
		}

		want := int(rate)
		// A permit is indivisible, so the bucket can be holding a fraction of one
		// at the end of the second. It must never be ahead of the rate.
		if granted > want || granted < want-1 {
			t.Errorf("rate %.0f/s granted %d permits in a second, want %d", rate, granted, want)
		}
	}
}

// TestPacingAtAThousandPerSecondDoesNotSleepPerTransaction is the property that
// the old ticker-per-transaction loop could not have. A sleep per transaction at
// 1000/s needs a thousand one-millisecond sleeps a second, which the Go timer
// cannot deliver; the bucket grants permits in batches instead, so the number of
// blocking draws stays small however high the rate goes.
func TestPacingAtAThousandPerSecondDoesNotSleepPerTransaction(t *testing.T) {
	const workers = 32
	const rate = 1000.0
	capacity, batch := benchBurst(rate, workers)
	b, clock := bucketOnFakeClock(rate, capacity)

	granted, empties, largest := 0, 0, 0
	for step := 0; step < 10; step++ {
		clock.advance(100 * time.Millisecond)
		n, e, l := drain(b, batch)
		granted += n
		empties += e
		if l > largest {
			largest = l
		}
	}

	if granted < 999 {
		t.Fatalf("granted %d permits in a second at %0.f/s, want about %.0f", granted, rate, rate)
	}
	if empties > workers {
		t.Errorf("%d blocking draws for %d permits, want no more than one per worker (%d)", empties, granted, workers)
	}
	if largest < 2 {
		t.Errorf("largest single grant was %d permit, so pacing is still one transaction at a time", largest)
	}
}

func TestTokenBucketNeverExceedsItsCapacityInABurst(t *testing.T) {
	b, clock := bucketOnFakeClock(10000, 1000)
	// Idle for a minute. The permits earned while idle are dropped past capacity,
	// so the generator does not open with a burst the node never asked for.
	clock.advance(time.Minute)
	granted, _, _ := drain(b, 1<<20)
	if granted != 1000 {
		t.Errorf("granted %d permits after a minute idle, want the capacity of 1000", granted)
	}
}

func TestTokenBucketWithZeroRateGrantsImmediatelyAndWithoutWaiting(t *testing.T) {
	b, _ := bucketOnFakeClock(0, 0)
	if !b.unpaced() {
		t.Fatal("a bucket with rate 0 should be unpaced")
	}
	for i := 0; i < 1000; i++ {
		granted, wait := b.take(8)
		if granted != 8 || wait != 0 {
			t.Fatalf("take(8) on an unpaced bucket = (%d, %s), want (8, 0s)", granted, wait)
		}
	}
	if got := b.waits.Load(); got != 0 {
		t.Errorf("unpaced bucket recorded %d waits, want 0", got)
	}
}

func TestTokenBucketReportsTheWaitUntilTheNextPermit(t *testing.T) {
	cases := []struct {
		rate float64
		want time.Duration
	}{
		{1, time.Second},
		{100, 10 * time.Millisecond},
		{1000, time.Millisecond},
		{10000, 100 * time.Microsecond},
	}
	for _, c := range cases {
		b, _ := bucketOnFakeClock(c.rate, 100)
		granted, wait := b.take(1)
		if granted != 0 {
			t.Fatalf("rate %.0f/s: a bucket starts empty, but take granted %d", c.rate, granted)
		}
		if wait != c.want {
			t.Errorf("rate %.0f/s: wait until the next permit = %s, want %s", c.rate, wait, c.want)
		}
	}
}

func TestTokenBucketAcquireReturnsZeroOnceTheContextIsDone(t *testing.T) {
	b, _ := bucketOnFakeClock(1, 1) // one permit a second, and the clock never moves
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := b.acquire(ctx, 4); got != 0 {
		t.Errorf("acquire on a cancelled context = %d, want 0", got)
	}
}

func TestBenchBurstSplitsTheBucketEvenlyBetweenWorkers(t *testing.T) {
	cases := []struct {
		rate         float64
		workers      int
		wantCapacity float64
		wantBatch    int
	}{
		{0, 32, 32, 1},     // unpaced: capacity only has to cover the workers
		{100, 32, 32, 1},   // a tenth of a second is less than one permit each
		{1000, 32, 100, 3}, // 100ms of permits, three per worker draw
		{10000, 32, 1000, 31},
		{10000, 1, 1000, 1000},
		{100, 0, 10, 10}, // a nonsense worker count is treated as one, not divided by
	}
	for _, c := range cases {
		capacity, batch := benchBurst(c.rate, c.workers)
		if capacity != c.wantCapacity || batch != c.wantBatch {
			t.Errorf("benchBurst(%.0f, %d) = (%.0f, %d), want (%.0f, %d)",
				c.rate, c.workers, capacity, batch, c.wantCapacity, c.wantBatch)
		}
		if batch < 1 {
			t.Errorf("benchBurst(%.0f, %d) gave a batch of %d, which would stall every worker", c.rate, c.workers, batch)
		}
	}
}

func TestTxBudgetHandsOutExactlyTheConfiguredMaximum(t *testing.T) {
	const max = 10000
	const claimers = 16
	b := newTxBudget(max)

	var wg sync.WaitGroup
	claimed := make([]int, claimers)
	for i := 0; i < claimers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				n := b.claim(7)
				if n == 0 {
					return
				}
				claimed[i] += n
			}
		}(i)
	}
	wg.Wait()

	total := 0
	for _, n := range claimed {
		total += n
	}
	if total != max {
		t.Errorf("%d claimers together took %d transactions, want exactly %d", claimers, total, max)
	}
}

func TestTxBudgetOfZeroIsUnbounded(t *testing.T) {
	b := newTxBudget(0)
	for i := 0; i < 100; i++ {
		if got := b.claim(64); got != 64 {
			t.Fatalf("claim(64) on an unbounded budget = %d, want 64", got)
		}
	}
}

func TestSleepCtxReportsWhetherItSleptOrWasCancelled(t *testing.T) {
	if !sleepCtx(context.Background(), time.Millisecond) {
		t.Error("sleepCtx should report true when it slept the whole duration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepCtx(ctx, time.Hour) {
		t.Error("sleepCtx should report false when the context finished first")
	}
	if !sleepCtx(context.Background(), 0) {
		t.Error("sleepCtx with no duration should report true on a live context")
	}
}

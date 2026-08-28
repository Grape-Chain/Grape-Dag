package dag

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

// newTestSite - a bare site usable by the confirmation counter: it only reads
// the id and the target links.
func newTestSite(targets ...*Node) *Node {
	return &Node{
		id:      NodeID{id: uuid.New()},
		targets: targets,
	}
}

// confirm site s by adding `n` approvers to the counter
func approve(c *ConfirmationCounter, s *Node, n int) {
	for i := 0; i < n; i++ {
		c.add(newTestSite(s))
	}
}

func TestConfirmedSiteIsHarvestedOnlyOnce(t *testing.T) {
	c := newConfirmationCounter()
	target := newTestSite()
	c.add(target)

	// two approvers promote the target to confirmed
	approve(c, target, 2)

	first := c.pop()
	if len(first) != 1 {
		t.Fatalf("expected exactly 1 confirmed site, got %d", len(first))
	}
	if first[0].id.id != target.id.id {
		t.Fatalf("expected the confirmed site to be the target")
	}

	// A site can be referenced again as an approval target long after it was
	// pinned: InsertTxDag resolves targets out of past pin transactions. It must
	// not be confirmed - and paid - a second time.
	approve(c, target, 3)
	second := c.pop()
	if len(second) != 0 {
		t.Fatalf("already-pinned site was confirmed again: got %d sites", len(second))
	}
}

func TestHarvestedSiteDoesNotReturnAsTip(t *testing.T) {
	c := newConfirmationCounter()
	target := newTestSite()
	c.add(target)
	approve(c, target, 2)
	c.pop()

	// A late site approves the already-pinned target - the normal case, since
	// InsertTxDag resolves approval targets out of past pin transactions. The
	// pinned site must not re-enter the tip set, or tip selection would hand it
	// out as an approval candidate again.
	approve(c, target, 1)

	if c.isTip(target.id.id) {
		t.Fatalf("harvested site is reported as a tip after a late approval")
	}
	for _, tip := range c.getTips() {
		if tip.id.id == target.id.id {
			t.Fatalf("harvested site came back in the tip set")
		}
	}
}

// The harvest bookkeeping in pop() is the second line of defence, independent of
// the tip-side guard in add(). Drive pop() directly so this test fails if only
// the pop-side guard regresses.
func TestPopNeverHarvestsTheSameSiteTwice(t *testing.T) {
	c := newConfirmationCounter()
	s := newTestSite()

	c.cache[s.id.id] = s
	c.confirmed.Insert(s.id.id)
	if got := len(c.pop()); got != 1 {
		t.Fatalf("first pop returned %d sites, want 1", got)
	}

	// the site is promoted again, as a resurrected tip would be
	c.cache[s.id.id] = s
	c.confirmed.Insert(s.id.id)
	if got := len(c.pop()); got != 0 {
		t.Fatalf("second pop harvested an already-pinned site: got %d sites, want 0", got)
	}
}

func TestPopSkipsSitesMissingFromCache(t *testing.T) {
	c := newConfirmationCounter()
	// A confirmed id with nothing in the cache must not yield a nil *Node: the
	// pin builder sorts sites by timestamp and would dereference it.
	c.confirmed.Insert(uuid.New())
	got := c.pop()
	for i, n := range got {
		if n == nil {
			t.Fatalf("pop returned a nil site at index %d", i)
		}
	}
}

func TestGetTipsAndTipNeverReturnNil(t *testing.T) {
	c := newConfirmationCounter()
	s := newTestSite()
	c.add(s)
	// simulate a cache/tips inconsistency
	c.tips[uuid.New()] = []uuid.UUID{uuid.New()}

	for i, n := range c.getTips() {
		if n == nil {
			t.Fatalf("getTips returned nil at index %d", i)
		}
	}
	for i, n := range c.tip() {
		if n == nil {
			t.Fatalf("tip returned nil at index %d", i)
		}
	}
}

// getTips used to read the maps without holding the mutex while add() mutated
// them from another goroutine, which is a fatal concurrent map access in Go.
// Run with -race.
func TestConfirmationCounterConcurrentAccess(t *testing.T) {
	c := newConfirmationCounter()
	root := newTestSite()
	c.add(root)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// The writer is bounded as well as stoppable, and the bound is the point.
	//
	// It used to add sites until the readers finished, and getTips is linear in
	// the number of tips the writer is creating - so reader throughput fell as
	// the writer ran, which let the writer run longer, which lowered reader
	// throughput again. Normally that resolved in 0.18s; under -race it took
	// about five seconds; on a loaded machine running the whole suite it did not
	// resolve at all and took Go's ten-minute timeout with it. A test whose
	// runtime depends on the ratio between two goroutines' speeds is a test that
	// will eventually hang in CI, and it hung here.
	//
	// Ten thousand sites is far more than the readers can exhaust and keeps every
	// interleaving the test was written for, while making the work finite.
	const writerSites = 10000
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < writerSites; i++ {
			select {
			case <-stop:
				return
			default:
				c.add(newTestSite(root))
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			c.getTips()
			c.tip()
			c.isTip(root.id.id)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c.pop()
		}
	}()

	// Read from this goroutine too, then stop the writer and collect everyone.
	// There used to be a detached `go wg.Wait()` here whose result nothing read;
	// it did nothing except look like synchronisation.
	for i := 0; i < 2000; i++ {
		c.getTips()
	}
	close(stop)
	wg.Wait()
}

// Genesis reaches the first pin without going through the confirmed pool, so it
// has to be recorded as harvested explicitly - otherwise the first two sites to
// approve it promote it and it lands in a second pin.
func TestMarkHarvestedKeepsASiteOutOfLaterPins(t *testing.T) {
	c := newConfirmationCounter()
	genesis := newTestSite()
	c.add(genesis)

	c.markHarvested(genesis.id.id)

	if c.isTip(genesis.id.id) {
		t.Fatalf("harvested genesis is still a tip")
	}
	approve(c, genesis, 3)
	if got := len(c.pop()); got != 0 {
		t.Fatalf("harvested genesis was confirmed again: got %d sites", got)
	}
}

func TestMarkHarvestedIsIdempotent(t *testing.T) {
	c := newConfirmationCounter()
	s := newTestSite()
	c.add(s)
	c.markHarvested(s.id.id)
	c.markHarvested(s.id.id)
	if c.isTip(s.id.id) {
		t.Fatalf("site returned to the tip set")
	}
}

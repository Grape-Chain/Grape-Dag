package dag

import (
	"testing"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
)

// archivedSites - the protobuf form of n settled sites, as a commit transaction
// carries them.
func archivedSites(t testing.TB, from, n int) ([]*pb.Node, []uuid.UUID) {
	t.Helper()
	nodes := make([]*pb.Node, 0, n)
	ids := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		site := siteWithTx(from + i)
		nodes = append(nodes, site.ToPbNode())
		ids = append(ids, site.id.id)
	}
	return nodes, ids
}

// The archive's job is to answer whether a site has been settled, for any site
// it has ever been given. The bodies it holds to answer with are a cache and are
// bounded; the answer itself is not, because a late arrival naming a settled
// site as an approval target must not be told the site is unknown.
func TestASettledSiteStaysKnownAfterItsBodyIsReleased(t *testing.T) {
	prevRetain := pinBodyRetainPins
	pinBodyRetainPins = 2
	defer func() { pinBodyRetainPins = prevRetain }()

	a := newRamArchive()
	nodes, ids := archivedSites(t, 0, 3)
	for i, node := range nodes {
		a.Archive(int64(i), []*pb.Node{node})
	}

	// Pin 0 is outside a two-pin window; its body has gone, but the site is
	// still settled and the archive still knows which pin settled it.
	if _, ok := a.Lookup(ids[0]); ok {
		t.Fatal("the archive is still holding the body of a site settled outside the retain window")
	}
	if !a.Has(ids[0]) {
		t.Fatal("a settled site became unknown when its body was released")
	}
	if pin, ok := a.PinOf(ids[0]); !ok || pin != 0 {
		t.Fatalf("PinOf for a released site = %d, %t; want 0, true", pin, ok)
	}
	// And inside the window the body is there, with its transaction.
	node, ok := a.Lookup(ids[2])
	if !ok {
		t.Fatal("a site settled inside the retain window has no body")
	}
	if node.tx == nil {
		t.Fatal("the archived site came back without its transaction")
	}
	// Len reports what the archive knows, not what it is holding: the ledger
	// has not forgotten these sites.
	if a.Len() != 3 {
		t.Fatalf("archive reports %d site(s), want 3", a.Len())
	}
}

// settledSite is what the insert path asks, and it must keep answering for a
// site whose body has been released - otherwise an arriving transaction that
// approves a settled site is treated as approving something missing, and the
// node goes looking on the network for a site it settled itself.
func TestASettledSiteIsStillRecognisedOnTheInsertPath(t *testing.T) {
	prevArchive := sliceArchive
	prevRetain := pinBodyRetainPins
	sliceArchive = newRamArchive()
	pinBodyRetainPins = 1
	defer func() { sliceArchive, pinBodyRetainPins = prevArchive, prevRetain }()

	nodes, ids := archivedSites(t, 0, 2)
	sliceArchive.Archive(0, nodes[:1])
	sliceArchive.Archive(1, nodes[1:])

	node, settled := settledSite(ids[0])
	if !settled {
		t.Fatal("a settled site whose body was released is no longer recognised as settled")
	}
	if node != nil {
		t.Fatal("a released site came back with a node; the caller must be given nil rather than a guess")
	}
	if node, settled := settledSite(ids[1]); !settled || node == nil {
		t.Fatal("a site settled inside the retain window is not being returned")
	}
	if _, settled := settledSite(uuid.New()); settled {
		t.Fatal("a site that was never settled is reported as settled")
	}
}

// Releasing is driven by the newest commit transaction seen, not by a count of
// sites, so a window of n pins means the same thing however many sites each pin
// settles.
func TestTheArchiveWindowIsMeasuredInCommitTransactions(t *testing.T) {
	prevRetain := pinBodyRetainPins
	pinBodyRetainPins = 3
	defer func() { pinBodyRetainPins = prevRetain }()

	a := newRamArchive()
	held := map[int64][]uuid.UUID{}
	for pin := int64(0); pin < 10; pin++ {
		nodes, ids := archivedSites(t, int(pin)*100, 5)
		a.Archive(pin, nodes)
		held[pin] = ids
	}

	for pin, ids := range held {
		inWindow := pin > 9-int64(pinBodyRetainPins)
		for _, id := range ids {
			_, ok := a.Lookup(id)
			if ok != inWindow {
				t.Fatalf("site from pin %d: body held = %t, want %t", pin, ok, inWindow)
			}
			if !a.Has(id) {
				t.Fatalf("site from pin %d is no longer known to be settled", pin)
			}
		}
	}
	if a.Len() != 50 {
		t.Fatalf("archive reports %d site(s), want 50", a.Len())
	}
}

// The archive holds pointers to the commit-transaction chain's own pb.Node
// values, so its window and the chain's window have to be the same window. If
// the archive keeps a body the chain has released, the chain's release frees
// nothing at all and its bound becomes decorative.
//
// This is not a hypothetical. The chain's bound was changed from a pin count to
// a site count and this one was left on the pin count; a heap profile of a node
// five minutes into a saturation run still put 42% of the live heap in the site
// bodies the chain had dutifully let go of.
func TestTheArchiveHonoursTheSiteBoundAsWellAsThePinBound(t *testing.T) {
	restoreSites, restorePins := pinBodyRetainSites, pinBodyRetainPins
	pinBodyRetainSites, pinBodyRetainPins = 50, 1000 // sites must bite first
	t.Cleanup(func() { pinBodyRetainSites, pinBodyRetainPins = restoreSites, restorePins })

	a := newRamArchive()
	var everyID []uuid.UUID
	for pin := int64(0); pin < 20; pin++ {
		nodes, ids := archivedSites(t, int(pin)*100, 10)
		everyID = append(everyID, ids...)
		a.Archive(pin, nodes)
	}

	a.mu.RLock()
	held := len(a.bodies)
	a.mu.RUnlock()
	// Bounded above, because that is the bug this is about; and bounded below,
	// because over-releasing is its own failure. If the held counter ever drifts
	// upward - say it is incremented on archive but not decremented on release -
	// the site bound reads as permanently exceeded and the archive throws away
	// everything except the newest commit on every call. Nothing breaks: Has and
	// PinOf still answer, so no settled site becomes unknown. It just quietly
	// stops being a cache, which is exactly the kind of regression that survives
	// a test asserting only "not too much".
	if held > pinBodyRetainSites+10 {
		t.Errorf("the archive holds %d bodies against a %d-site bound and a %d-pin bound - the pin bound is governing when the site bound should",
			held, pinBodyRetainSites, pinBodyRetainPins)
	}
	if floor := pinBodyRetainSites / 2; held < floor {
		t.Errorf("the archive holds only %d bodies against a %d-site budget - it is releasing far more than the bound asks for", held, pinBodyRetainSites)
	}

	// Releasing a body must never make a settled site unknown. That is the
	// property the whole archive exists for, and the one a size bound is most
	// likely to break.
	for _, id := range everyID {
		if !a.Has(id) {
			t.Fatalf("site %s stopped being known once its body was released", id)
		}
		if _, ok := a.PinOf(id); !ok {
			t.Fatalf("site %s lost the commit transaction that settled it", id)
		}
	}
}

// A commit larger than the whole body budget must not empty the archive: the
// newest commit is the one every lookup is about.
func TestACommitLargerThanTheBudgetStillKeepsItsBodies(t *testing.T) {
	restoreSites, restorePins := pinBodyRetainSites, pinBodyRetainPins
	pinBodyRetainSites, pinBodyRetainPins = 10, 1000
	t.Cleanup(func() { pinBodyRetainSites, pinBodyRetainPins = restoreSites, restorePins })

	a := newRamArchive()
	nodes, ids := archivedSites(t, 9000, 100)
	a.Archive(0, nodes)

	found := 0
	for _, id := range ids {
		if _, ok := a.Lookup(id); ok {
			found++
		}
	}
	if found == 0 {
		t.Errorf("a single commit of %d sites against a %d-site budget left no bodies at all", len(nodes), pinBodyRetainSites)
	}
}

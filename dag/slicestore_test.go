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

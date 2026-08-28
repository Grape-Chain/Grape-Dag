package dag

import (
	"testing"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
)

// sliceFixture - a small live graph plus the globals sliceSites reads.
func sliceFixture(t *testing.T) (*Dag, func()) {
	t.Helper()
	prevCfg := dagConfig
	prevArchive := sliceArchive
	prevCounter := confirmationCounter
	prevDag := _dag_

	dagConfig = config.DagConfiguration{Slicing: true, Approvetx: 2}
	sliceArchive = newRamArchive()
	confirmationCounter = newConfirmTracker(0, 1000)

	d := &Dag{
		mapped_vertices: make(map[uuid.UUID]*Node),
		mapped_edges:    make(map[uuid.UUID][]uuid.UUID),
	}
	_dag_ = d
	return d, func() {
		dagConfig = prevCfg
		sliceArchive = prevArchive
		confirmationCounter = prevCounter
		_dag_ = prevDag
	}
}

func addSite(d *Dag, n *Node, parents ...*Node) {
	for _, p := range parents {
		tlink(n, p)
		d._links_ = append(d._links_, Link{source: n, target: p})
		d.mapped_edges[n.id.id] = append(d.mapped_edges[n.id.id], p.id.id)
	}
	d._dag_ = append(d._dag_, n)
	d.mapped_vertices[n.id.id] = n
}

func pinOver(number int64, sites ...*Node) *pb.TxPin {
	pin := &pb.TxPin{PinNumber: number}
	for _, s := range sites {
		pin.Sites = append(pin.Sites, &pb.SiteID{Id: s.id.id[:]})
		pin.Nodes = append(pin.Nodes, s.ToPbNode())
	}
	return pin
}

// A site with a transaction, so ToPbNode has something to marshal.
func siteWithTx(n int) *Node {
	node := tnode(n)
	node.tx = tx.NewTxv1(tx.PRIVATE_TESTNET)
	return node
}

func TestSlicingTakesSettledSitesOutOfTheLiveGraph(t *testing.T) {
	d, restore := sliceFixture(t)
	defer restore()

	genesis := siteWithTx(0)
	a := siteWithTx(1)
	b := siteWithTx(2)
	frontier := siteWithTx(3)
	addSite(d, genesis)
	addSite(d, a, genesis)
	addSite(d, b, genesis)
	addSite(d, frontier, a, b)

	if len(d._dag_) != 4 {
		t.Fatalf("fixture holds %d sites, want 4", len(d._dag_))
	}
	linksBefore := len(d._links_)

	removed := d.sliceSites(pinOver(1, genesis, a, b))
	if removed != 3 {
		t.Fatalf("sliced %d sites, want 3", removed)
	}
	if len(d._dag_) != 1 || d._dag_[0].id.id != frontier.id.id {
		t.Fatalf("live graph holds %d sites, want just the frontier", len(d._dag_))
	}
	if len(d._links_) >= linksBefore {
		t.Fatalf("edges were not dropped: %d before, %d after", linksBefore, len(d._links_))
	}
	for _, id := range []uuid.UUID{genesis.id.id, a.id.id, b.id.id} {
		if _, ok := d.mapped_vertices[id]; ok {
			t.Fatalf("settled site %s is still in the lookup map", id.String())
		}
	}

	// the frontier's pointers to settled sites are gone, but the approvals are
	// still recorded
	if len(frontier.targets) != 0 {
		t.Fatalf("frontier still points at %d settled sites", len(frontier.targets))
	}
	if len(frontier.slicedTargets) != 2 {
		t.Fatalf("frontier recorded %d settled approvals, want 2", len(frontier.slicedTargets))
	}
	// and they are still reported on the wire
	advertised := frontier.ToPbNode().MissingTargets
	for _, id := range []uuid.UUID{a.id.id, b.id.id} {
		if !advertised[id.String()] {
			t.Fatalf("settled approval %s is not advertised to peers", id.String())
		}
	}
}

func TestSettledSitesStayFindable(t *testing.T) {
	d, restore := sliceFixture(t)
	defer restore()

	genesis := siteWithTx(0)
	a := siteWithTx(1)
	addSite(d, genesis)
	addSite(d, a, genesis)

	d.sliceSites(pinOver(1, genesis, a))

	for _, want := range []*Node{genesis, a} {
		got, ok := settledSite(want.id.id)
		if !ok {
			t.Fatalf("settled site %s cannot be found in the archive", want.id.id.String())
		}
		if got.id.id != want.id.id {
			t.Fatalf("archive returned %s for %s", got.id.id.String(), want.id.id.String())
		}
		if len(got.targets) != 0 || len(got.sources) != 0 {
			t.Fatalf("archived site came back with live edges")
		}
		if pin, ok := sliceArchive.PinOf(want.id.id); !ok || pin != 1 {
			t.Fatalf("archive reports pin %d for %s, want 1", pin, want.id.id.String())
		}
	}
	if sliceArchive.Len() != 2 {
		t.Fatalf("archive holds %d sites, want 2", sliceArchive.Len())
	}
}

func TestSlicingKeepsTheLiveGraphBounded(t *testing.T) {
	d, restore := sliceFixture(t)
	defer restore()

	genesis := siteWithTx(0)
	addSite(d, genesis)
	prev := genesis
	maxSeen := 0

	// 30 rounds of "add a few sites, settle all but the last"
	for round := 1; round <= 30; round++ {
		batch := []*Node{}
		for i := 0; i < 10; i++ {
			n := siteWithTx(round*100 + i)
			addSite(d, n, prev)
			batch = append(batch, n)
			prev = n
		}
		if len(d._dag_) > maxSeen {
			maxSeen = len(d._dag_)
		}
		// settle everything except the newest, which stays the frontier
		d.sliceSites(pinOver(int64(round), batch[:len(batch)-1]...))
	}

	if len(d._dag_) > 40 {
		t.Fatalf("live graph holds %d sites after 300 inserts; slicing is not bounding it", len(d._dag_))
	}
	if len(d._links_) > 40 {
		t.Fatalf("live graph holds %d edges after 300 inserts", len(d._links_))
	}
	if sliceArchive.Len() < 250 {
		t.Fatalf("archive holds only %d sites; settled sites are being lost", sliceArchive.Len())
	}
	t.Logf("after 300 inserts: live=%d sites / %d edges (peak %d), archived=%d",
		len(d._dag_), len(d._links_), maxSeen, sliceArchive.Len())
}

func TestSlicingCanBeTurnedOff(t *testing.T) {
	d, restore := sliceFixture(t)
	defer restore()
	dagConfig.Slicing = false

	genesis := siteWithTx(0)
	a := siteWithTx(1)
	addSite(d, genesis)
	addSite(d, a, genesis)

	if removed := d.sliceSites(pinOver(1, genesis, a)); removed != 0 {
		t.Fatalf("slicing is disabled but %d sites were removed", removed)
	}
	if len(d._dag_) != 2 {
		t.Fatalf("live graph holds %d sites, want both", len(d._dag_))
	}
}

// A settled site must not be confirmed again: it is already in a commit tx.
func TestSettledSitesCannotBeConfirmedAgain(t *testing.T) {
	d, restore := sliceFixture(t)
	defer restore()

	genesis := siteWithTx(0)
	a := siteWithTx(1)
	addSite(d, genesis)
	addSite(d, a, genesis)
	confirmationCounter.add(genesis)
	confirmationCounter.add(a)

	d.sliceSites(pinOver(1, genesis, a))

	if confirmationCounter.isTip(a.id.id) {
		t.Fatalf("a settled site is still offered as an approval target")
	}
	confirmationCounter.add(a)
	for _, got := range confirmationCounter.pop() {
		if got.id.id == a.id.id {
			t.Fatalf("a settled site was confirmed a second time")
		}
	}
}

package dag

import (
	"sync"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
)

/*
SliceArchive is where sites go once a commit transaction has made them
irrevocable - the "slice" of the technical paper's section 6.

Sites in a slice are settled: they are not re-verified, they are not candidates
for approval, and they take no further part in confirmation. They still have to
be *findable*, because a site arriving late can name one of them as an approval
target, and the query API has to be able to return their transactions. So they
leave the live graph and stay in the archive.

The interface is deliberately narrow, and deliberately not backed by the live
DAG: the persistence work replaces the in-memory implementation with one that
spills to disk, and nothing else has to change.
*/
type SliceArchive interface {
	// Archive - record the sites a commit transaction has settled.
	Archive(pinNumber int64, sites []*pb.Node)
	// Lookup - find a settled site. The returned node carries the site and its
	// transaction but no graph links: it is settled, so its edges are history.
	//
	// Reports false for a site whose body has been released past the retain
	// window even though the site is settled and known. Ask Has for the
	// membership question; see settledSite in dag/slicer.go, which is the
	// production caller and asks both.
	Lookup(id uuid.UUID) (*Node, bool)
	// Has - whether this site has been settled, for every site ever archived.
	Has(id uuid.UUID) bool
	// PinOf - which commit transaction settled a site.
	PinOf(id uuid.UUID) (int64, bool)
	Len() int
}

// ramArchive - an index of every settled site, plus the protobuf bodies of the
// recently settled ones.
//
// It used to keep the body of every settled site for the life of the process:
// 196,000 of them in one profiled run, and the largest single retainer of live
// bytes on the node. Those bodies are the pin's own pin.Nodes - the archive is
// handed that slice and stores the pointers - so they are released here in step
// with pinBodyRetainPins, which releases the pin's copy. Releasing one without
// the other frees nothing at all.
//
// What is kept for the whole history is the id-to-pin index, which is what makes
// "has this site been settled?" an O(1) question. That is the only question any
// production caller asks, and one index entry per settled site is smaller than
// the pb.SiteID the commit-transaction chain retains for the same site anyway.
// It is still linear in the ledger's history, and bounding it needs somewhere
// else to ask: a point read of a stored pin, which store.Store cannot answer
// today.
type ramArchive struct {
	mu     sync.RWMutex
	pinOf  map[uuid.UUID]int64
	bodies map[uuid.UUID]*pb.Node
	// window - the ids whose bodies are still held, grouped by the commit
	// transaction that settled them and ordered oldest first. Grouped so that a
	// whole pin's worth can be released without scanning the body map, which is
	// the difference between constant and linear work per commit.
	window []archivedPin
	// held - bodies currently in the window. Counted rather than taken from
	// len(bodies), so that the volume bound costs nothing to evaluate.
	held   int
	newest int64
	seen   bool
}

type archivedPin struct {
	pin int64
	ids []uuid.UUID
}

func newRamArchive() *ramArchive {
	return &ramArchive{
		pinOf:  make(map[uuid.UUID]int64),
		bodies: make(map[uuid.UUID]*pb.Node),
	}
}

func (a *ramArchive) Archive(pinNumber int64, sites []*pb.Node) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry := archivedPin{pin: pinNumber, ids: make([]uuid.UUID, 0, len(sites))}
	for _, s := range sites {
		if s == nil || s.Id == nil {
			continue
		}
		id, err := uuid.FromBytes(s.Id.Id)
		if err != nil {
			continue
		}
		a.pinOf[id] = pinNumber
		a.bodies[id] = s
		entry.ids = append(entry.ids, id)
	}
	if len(entry.ids) > 0 {
		a.window = append(a.window, entry)
		a.held += len(entry.ids)
	}
	if !a.seen || pinNumber > a.newest {
		a.newest, a.seen = pinNumber, true
	}
	a.releaseOldBodies()
}

// releaseOldBodies - drop the bodies of sites settled outside the retain
// window. Caller holds a.mu.
//
// Both bounds, and they have to be the same two the commit-transaction chain
// uses, because this map holds pointers to that chain's own pb.Node values. The
// chain replacing a pin with a body-less copy frees nothing at all while these
// pointers are still here, so a window here that is wider than the chain's makes
// the chain's bound decorative.
//
// That is not hypothetical - it is what happened. The chain's bound was changed
// from pins to sites and this one was left on pins, and a heap profile of a node
// five minutes into a saturation run still put 727 MB, 42% of the live heap, in
// Node.ToPbNode reached through serialiseSites, with the chain dutifully
// releasing bodies that this map went on holding.
func (a *ramArchive) releaseOldBodies() {
	oldest := int64(-1)
	if pinBodyRetainPins > 0 {
		oldest = a.newest - int64(pinBodyRetainPins)
	}
	for len(a.window) > 0 {
		overPins := pinBodyRetainPins > 0 && a.window[0].pin <= oldest
		overSites := pinBodyRetainSites > 0 && a.held > pinBodyRetainSites
		if !overPins && !overSites {
			return
		}
		// Never release the only entry on the site bound alone: a single commit
		// larger than the whole budget would otherwise leave the archive holding
		// no bodies at all, and the newest commit is the one every lookup is
		// about.
		if !overPins && len(a.window) == 1 {
			return
		}
		for _, id := range a.window[0].ids {
			delete(a.bodies, id)
		}
		a.held -= len(a.window[0].ids)
		a.window[0] = archivedPin{}
		a.window = a.window[1:]
	}
}

func (a *ramArchive) Lookup(id uuid.UUID) (*Node, bool) {
	a.mu.RLock()
	pbn, ok := a.bodies[id]
	a.mu.RUnlock()
	if !ok {
		return nil, false
	}
	node := &Node{}
	node.FromPbNode(pbn)
	// A settled site has no live edges. Clearing this stops the caller from
	// mistaking the wire field (which carries the site's own approvals) for a
	// list of things this node is waiting on.
	node.missingTargets = nil
	return node, true
}

func (a *ramArchive) Has(id uuid.UUID) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.pinOf[id]
	return ok
}

func (a *ramArchive) PinOf(id uuid.UUID) (int64, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	p, ok := a.pinOf[id]
	return p, ok
}

// Len - how many settled sites the archive knows of, which is what the size
// gauge and the slice log report. Not the number of bodies it is holding: that
// rises and falls with the retain window and would make the ledger look as
// though it were forgetting its history.
func (a *ramArchive) Len() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.pinOf)
}

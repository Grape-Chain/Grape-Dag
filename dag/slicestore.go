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
	Lookup(id uuid.UUID) (*Node, bool)
	Has(id uuid.UUID) bool
	// PinOf - which commit transaction settled a site.
	PinOf(id uuid.UUID) (int64, bool)
	Len() int
}

// ramArchive - keeps the protobuf form of every settled site, which the commit
// transaction already holds in memory, plus an index into it. What it does not
// keep is the live *Node with its edges, and that is the point: the edge set is
// what grows with the ledger.
type ramArchive struct {
	mu    sync.RWMutex
	sites map[uuid.UUID]*pb.Node
	pinOf map[uuid.UUID]int64
}

func newRamArchive() *ramArchive {
	return &ramArchive{
		sites: make(map[uuid.UUID]*pb.Node),
		pinOf: make(map[uuid.UUID]int64),
	}
}

func (a *ramArchive) Archive(pinNumber int64, sites []*pb.Node) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range sites {
		if s == nil || s.Id == nil {
			continue
		}
		id, err := uuid.FromBytes(s.Id.Id)
		if err != nil {
			continue
		}
		a.sites[id] = s
		a.pinOf[id] = pinNumber
	}
}

func (a *ramArchive) Lookup(id uuid.UUID) (*Node, bool) {
	a.mu.RLock()
	pbn, ok := a.sites[id]
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
	_, ok := a.sites[id]
	return ok
}

func (a *ramArchive) PinOf(id uuid.UUID) (int64, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	p, ok := a.pinOf[id]
	return p, ok
}

func (a *ramArchive) Len() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.sites)
}

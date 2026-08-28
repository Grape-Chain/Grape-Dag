package dag

import (
	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
)

/*
Slicing: once a commit transaction has settled a set of sites, take them out of
the live graph.

Confirmation already bounds its own working set, but the DAG itself did not: the
node slice, the edge list and both lookup maps grew for the lifetime of the
process, and every site kept its neighbours alive through its edge pointers, so
nothing could be collected. Tip selection, weight updates and every fallback
lookup walked that ever-growing structure.

After slicing, the live graph holds the unsettled frontier. Settled sites live in
the archive, which keeps the protobuf form the commit transaction already holds
plus an index, and can hand back a linkless node when a late arrival names one as
an approval target.

The one subtlety is incoming edges. A site still in the graph may approve a site
that has just been settled; if that pointer stayed, the settled site (and its own
neighbours, transitively) could never be collected. The pointer is dropped and
the id recorded on the approving site instead, so it can still be reported on the
wire. Confirmation is unaffected: it is closed downwards, so a walk that stops at
a settled site has stopped somewhere everything below is already settled.
*/

// sliceSites - remove the sites a commit transaction settled from the live graph
// and hand them to the archive. Caller must hold dag.mux.
func (d *Dag) sliceSites(pin *pb.TxPin) int {
	if pin == nil || !dagConfig.Slicing {
		return 0
	}
	settled := settledIds(pin)
	if len(settled) == 0 {
		return 0
	}
	archiveSettled(pin)
	removed := d.spliceSettled(pin, settled)
	harvestSettled(settled)
	return removed
}

// settledIds - the ids of the sites a commit transaction settled.
//
// Split out because it is the one part of slicing that is pure: decoding a uuid
// per site and building a map of them is work proportional to the pin, and it
// used to be done while holding dag.mux, where every insert and every tip
// selection waited on it.
func settledIds(pin *pb.TxPin) map[uuid.UUID]struct{} {
	settled := make(map[uuid.UUID]struct{}, len(pin.Sites))
	for _, s := range pin.Sites {
		if s == nil {
			continue
		}
		if id, err := uuid.FromBytes(s.Id); err == nil {
			settled[id] = struct{}{}
		}
	}
	return settled
}

// archiveSettled - hand the settled sites to the archive, which has a lock of
// its own and needs nothing from the graph.
func archiveSettled(pin *pb.TxPin) {
	if sliceArchive != nil {
		sliceArchive.Archive(pin.PinNumber, pin.Nodes)
	}
}

// harvestSettled - tell the confirmation tracker these sites are settled, so
// none of them can be confirmed a second time. Its own lock, and nothing from
// the graph.
func harvestSettled(settled map[uuid.UUID]struct{}) {
	if confirmationCounter == nil {
		return
	}
	for id := range settled {
		confirmationCounter.markHarvested(id)
	}
}

// spliceSettled - take the settled sites out of the live graph. This is the part
// that needs dag.mux, and now the only part of slicing that holds it.
// Caller must hold dag.mux.
func (d *Dag) spliceSettled(pin *pb.TxPin, settled map[uuid.UUID]struct{}) int {
	// Drop the settled sites from the node slice.
	kept := d._dag_[:0]
	removed := 0
	for _, n := range d._dag_ {
		if n == nil {
			continue
		}
		if _, gone := settled[n.id.id]; gone {
			removed++
			continue
		}
		kept = append(kept, n)
	}
	for i := len(kept); i < len(d._dag_); i++ {
		d._dag_[i] = nil // let the nodes go
	}
	d._dag_ = kept

	// Drop edges that touch them.
	keptLinks := d._links_[:0]
	for _, l := range d._links_ {
		if l.source == nil || l.target == nil {
			continue
		}
		_, srcGone := settled[l.source.id.id]
		_, dstGone := settled[l.target.id.id]
		if srcGone || dstGone {
			continue
		}
		keptLinks = append(keptLinks, l)
	}
	for i := len(keptLinks); i < len(d._links_); i++ {
		d._links_[i] = Link{}
	}
	d._links_ = keptLinks

	// Break the pointers the surviving sites still hold, recording the ids so a
	// site can still report every approval it made.
	for _, n := range d._dag_ {
		n.targets = dropSettled(n.targets, settled, &n.slicedTargets)
		n.sources = dropSettled(n.sources, settled, nil)
	}

	d.mu_map.Lock()
	for id := range settled {
		delete(d.mapped_vertices, id)
		delete(d.mapped_edges, id)
	}
	for id, targets := range d.mapped_edges {
		out := targets[:0]
		for _, t := range targets {
			if _, gone := settled[t]; gone {
				continue
			}
			out = append(out, t)
		}
		d.mapped_edges[id] = out
	}
	d.mu_map.Unlock()

	if removed > 0 {
		logger.Debugf("[slice] Settled %d site(s) into pin=%d; live graph now holds %d site(s), %d edge(s), archive %d",
			removed, pin.PinNumber, len(d._dag_), len(d._links_), archiveLen())
	}
	return removed
}

// dropSettled - remove settled neighbours from an edge list, optionally
// recording their ids.
func dropSettled(nodes []*Node, settled map[uuid.UUID]struct{}, record *[]uuid.UUID) []*Node {
	if len(nodes) == 0 {
		return nodes
	}
	out := nodes[:0]
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if _, gone := settled[n.id.id]; gone {
			if record != nil {
				*record = append(*record, n.id.id)
			}
			continue
		}
		out = append(out, n)
	}
	for i := len(out); i < len(nodes); i++ {
		nodes[i] = nil
	}
	return out
}

func archiveLen() int {
	if sliceArchive == nil {
		return 0
	}
	return sliceArchive.Len()
}

// settledSite - look a settled site up in the archive.
//
// The boolean is the authoritative answer to "has this site been settled?" and
// is true for every site the archive has ever been given. The node is nil when
// the site is settled but its body has been released past the retain window: the
// caller on the insert path only asks whether the approval target is settled -
// it deliberately does not link to it - so a nil node there costs nothing. Any
// future caller that wants the site itself has to handle nil, and should be
// reading it from the store rather than from a cache.
func settledSite(id uuid.UUID) (*Node, bool) {
	if sliceArchive == nil {
		return nil, false
	}
	if node, ok := sliceArchive.Lookup(id); ok {
		return node, true
	}
	return nil, sliceArchive.Has(id)
}

// sliceAppliedPin - take the dag lock and settle the sites a commit transaction
// just made irrevocable. Called from both sides of the chain: the leader after
// it forms a pin, and every other node after it applies one, so the live graph
// is bounded the same way everywhere.
// Only the graph surgery runs under dag.mux. Decoding the settled ids, handing
// them to the archive and recording them as harvested all need locks of their
// own but nothing from the graph, and they are proportional to the size of the
// commit transaction - thousands of sites at load - so doing them inside the
// graph lock made every insert wait on work that had no reason to be there.
//
// Harvesting now happens before the splice rather than after it. Both orders are
// correct: the sites are irrevocable by the time this runs, so a confirmation
// that arrives in between must not promote them either way, and marking them
// first is the side that refuses.
func sliceAppliedPin(pin *pb.TxPin) {
	if pin == nil || _dag_ == nil || !dagConfig.Slicing {
		return
	}
	defer stats.Time(stats.PinSlice)()
	settled := settledIds(pin)
	if len(settled) == 0 {
		return
	}
	archiveSettled(pin)
	harvestSettled(settled)

	_dag_.mux.Lock()
	defer _dag_.mux.Unlock()
	_dag_.spliceSettled(pin, settled)
}

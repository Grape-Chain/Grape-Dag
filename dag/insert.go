package dag

import (
	"time"

	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// InsertIfNotExist - add the sites in this batch that the live graph does not
// already hold.
//
// Caller holds dag.mux: both callers - handleSyncSiteResponse in dag/sync.go and
// the site-request handler in dag/synchandle.go - take it before calling, and
// insertMissing appends to dag._dag_.
func (d *Dag) InsertIfNotExist(vertices []*Node) error {
	for _, v := range vertices {
		if v == nil {
			continue
		}
		// Asks the lookup map only. The full search behind getById(_, false) was
		// a linear scan of the entire live graph per site in the batch, and it
		// cannot answer differently: every path that appends to dag._dag_
		// registers the site in the same map inside the same locked section
		// (addToDag, insertMissing), and slicing deletes from both. It was also
		// unsound - _getById_ asks for dag.mux with TryLock and carries on
		// without it when the attempt fails, which here it always does, because
		// the caller is already holding it - so the scan read a slice that
		// another goroutine may have been appending to.
		if d.getById(v.id.id, true) != nil {
			continue
		}
		// A site that has already been settled must not come back into the live
		// graph. Nothing would ever settle it again - the confirmation tracker
		// records it as harvested - so it would sit in the live graph for the
		// life of the process, and every pass over the graph would carry it.
		if _, settled := settledSite(v.id.id); settled {
			continue
		}
		if err := d.insertMissing(v); err != nil {
			logger.Errorf("Failed to insert a missing site: %s", err.Error())
		}
	}
	return nil
}

func (d *Dag) unsafe_bypassInsert(vertices []*Node) (*Dag, error) {
	var err error
	for _, n := range vertices {
		if d, err = d.addToDag(n, nil); err != nil {
			return d, errors.Errorf("Failed to bypass insert Tx %s into dag. %s", n.id.id.String(), err.Error())
		}
	}
	return d, nil
}

// InsertTxDag - insert a new site(vertex[aka node]) into dag, given its relative location
// based on the ids of the sites this site approves in other nodes; Hence, this site
// should locate the approvees (this site in the original node) that were approved by this site
// in the original node
//
// The subscriber's half of the insert path, and the busier of the two: this node
// receives every transaction the network publishes, not only its own. It is split
// the same way AddTxDag is, and for the same reason. Resolving the approvals and
// linking the site needs the exclusive lock; checking the claim the site arrived
// with does not, and that check is an ed25519 verification. See
// checkProcessorClaim for the invariant that makes moving it out safe rather than
// merely faster.
func (dag *Dag) InsertTxDag(
	inVertex *Node,
	txId uuid.UUID,
	idMajor uint64,
	idMinor uint32,
	ids ...tx.UuidSlice, // this is a slice of ids for the sites that this site(vertex) has approved elsewhere
) error {
	// When creating a site/vertex from a transaction, the versioning information does not get preserved
	// restore it here.
	//
	// Before the lock: inVertex was built by the subscriber and no other
	// goroutine can reach it yet, so these four writes are private.
	inVertex.id.id = txId
	inVertex.id.idMajor = idMajor
	inVertex.id.idMinor = idMinor
	inVertex.time = time.Now()

	// A site this node already holds is the ordinary case under diffusion - each
	// transaction arrives from every peer that relays it - and asking the lookup
	// map costs one read of an RWMutex that dag.mux does not gate. Doing it here
	// keeps a duplicate off the dag.mux queue entirely. The answer can go stale
	// between here and the lock, which is why the same check is repeated inside;
	// this one can only ever skip work, never authorise it.
	if dag.getById(inVertex.id.id, true) != nil {
		return nil
	}

	inGraph, err := dag.linkReceivedSite(inVertex, ids...)

	// Both of the next two are outside the exclusive section, and both are
	// conditional on the site having actually been added - which is not the same
	// question as whether an error came back. The missing-target path adds the
	// site and then reports the gap, and a site with unresolved approvals still
	// carries a claim worth checking.
	if inGraph {
		dag.checkProcessorClaim(inVertex)
		if traceSites {
			// logLast walks the site's edges to format itself, which is a read of
			// dag._links_ and nothing more, so the shared lock is enough.
			dag.rlock()
			dag.logLast("INSERT:", inVertex, 1)
			dag.runlock()
		}
	}
	return err
}

// linkReceivedSite - the exclusive half of a received insert: resolve what the
// site approves, link it, and put it in the graph.
//
// Returns whether the site is now in the live graph, which the caller needs in
// order to decide whether there is anything left to do, and separately the error
// the caller reports. The two differ on the missing-target path, which adds the
// site so that it can be relinked later and still reports that this node is out
// of sync.
//
// Everything here needs the exclusive lock: the resolution loop's answers have to
// stay true until the edges are appended, dag._links_ is a shared slice whose
// append can reallocate, and addToDag applies the payment to the wallet cache and
// writes the node slice, the lookup maps and the confirmation tracker.
func (dag *Dag) linkReceivedSite(inVertex *Node, ids ...tx.UuidSlice) (bool, error) {
	dag.mux.Lock()
	defer dag.mux.Unlock()

	if v := dag.getById(inVertex.id.id, true); v != nil {
		return false, nil
	}

	// Vertex(node) to insert may be missing sites in the current version of the dag
	// this is the result of this node missing previous tx (out of sync)
	// wait till the next pin tx, and check if the tx after it can be
	// linked to either dag or pin tx
	candidates := []*Node{}
	settledIds := []uuid.UUID{}
	idsNotFound := []uuid.UUID{}
	for _, id := range ids {
		// first, check dag cache (and more thorough search if required)
		// we are looking for the sites/vertices that this site/vertex has
		// already approved elsewhere
		if n := dag.getById(id.Id, true); n != nil {
			candidates = append(candidates, n)
		} else if _, ok := settledSite(id.Id); ok {
			// The approved site has been settled by a commit transaction. It is
			// resolved - not missing - but it gets no edge: keeping one would
			// pull a settled site back into the live graph and stop it, and its
			// neighbours, from ever being collected.
			settledIds = append(settledIds, id.Id)
		} else if n = _pins_.getById(id.Id); n != nil {
			candidates = append(candidates, n)
		} else {
			idsNotFound = append(idsNotFound, id.Id)
		}
	}
	if len(settledIds) > 0 {
		inVertex.slicedTargets = append(inVertex.slicedTargets, settledIds...)
	}
	// let's see if we have the approvees [aka candidates for local approval]
	if len(candidates)+len(settledIds) != len(ids) {
		// In order to relink later, need to preserve target Ids
		// store in dag without trying to approve target sites it references
		// this node
		// so, the current version of dag does not have the sites referenced by inVertex site
		for _, id := range idsNotFound {
			if inVertex.missingTargets == nil {
				inVertex.missingTargets = make(map[string]bool, len(idsNotFound))
			}
			inVertex.missingTargets[id.String()] = true
			logger.Warnf("Tx %s is missing from this DAG. Requesting...", id.String())
		}
		if _, err := dag.addToDag(inVertex, candidates); err != nil {
			return false, errors.Errorf("Failed to insert Tx %s into dag. %s", inVertex.id.id.String(), err.Error())
		}
		logger.Warn("Local DAG is out of sync. Synchronization is underway...")

		// @TODO: add to queue until a chain can be built and inserted
		return true, errors.Errorf("Tx %s is missing source Txs: %q", inVertex.id.id.String(), idsNotFound)
	}

	// Version-collision handling used to sit here, behind dag.versioncollision,
	// and it has been removed rather than completed. What it did, when the
	// setting was on: it looked through the approvers of this site's approval
	// targets for one carrying the same idMajor and an idMinor at or above this
	// site's, and on finding one it set nextToNode and bumped this site's
	// version. It then tested nextToNode.id.id != inVertex.id.id and returned an
	// error if so.
	//
	// That test could not be false. nextToNode was only ever assigned inside a
	// branch guarded by candidateNode.id.id != inVertex.id.id, so every value it
	// could hold failed the test by construction. So the whole feature reduced
	// to: with dag.versioncollision on, a received site that collides on version
	// is refused outright and never enters the graph, while every other peer
	// holds it and this node's balances never move for it. The splice into
	// dag._dag_ below that test, and the addToDag calls beside it, were
	// unreachable - which is also why they had drifted into skipping the balance
	// update, the site counter, the confirmation tracker and the site's own
	// target list.
	//
	// Completing it would have meant inventing the semantics: there is no
	// specification for what a version collision should do, no test, and no
	// caller that turns the setting on. Leaving it was the worst option, because
	// the flag reads as a supported feature. Removed, so that the only behaviour
	// is the one that is actually exercised. dag.versioncollision is now inert
	// and Init says so if it is set; the config field and its viper default
	// should go too - see the report.
	if _, err := dag.addToDag(inVertex, candidates); err != nil {
		return false, err
	}

	// after the new node has been added to dag, update links
	for _, targetNode := range candidates {
		// link to these candidates - there should be at least one or ideally, two candidates
		dag._links_ = append(dag._links_, Link{source: inVertex, target: targetNode})
		// Guarded at the call site rather than left to the log level: the
		// arguments are evaluated either way, and each id.String() allocates a
		// 36-byte string. Two of them per approval per received site, inside
		// dag.mux, for a line nobody reads unless -verbose is on.
		if traceSites {
			logger.Debugf("[SUB LINK] %s|%d|%d ==> %s|%d|%d",
				inVertex.id.id.String(), inVertex.id.idMajor, inVertex.id.idMinor,
				targetNode.id.id.String(), targetNode.id.idMajor, targetNode.id.idMinor,
			)
		}
	}

	// Cumulative weights are no longer recomputed here either; see dag/weights.go.
	// dag.dag = updateFwdWeights(dag.dag, dag.links)
	return true, nil
}

// checkProcessorClaim - verify the claim a received site arrived with, and strip
// it if it does not hold up.
//
// Whoever built this site claimed it, and the claim is checkable: the signature
// covers the site's id, the hash of its transaction, and the ids of the sites it
// approves.
//
// A site with no attribution at all is accepted as it is - it came from a node
// predating attribution, and it simply earns nobody a fee. A site whose
// attribution does not verify is kept as well, but stripped: the site is still a
// valid part of the graph and refusing it would let a bad claim deny the network
// a transaction, whereas keeping the claim would let the liar be paid.
//
// # Why this is outside the exclusive section
//
// It is an ed25519 verification plus an address derivation, and it used to run
// inside the section that every insert, the slicer, the commit builder and the
// size gauges all queue behind. See BenchmarkSiteAttributionVerification for what
// that cost the rest of the node per received site.
//
// What it reads, and why the shared lock is enough for the reading half:
//
//   - the site's id and its transaction. Both are written before the site is
//     reachable by anything, and never again.
//
//   - the three claim fields. Nothing else writes them for a received site:
//     signProcessor only ever writes them for a site this node built itself, and
//     only before that site is published, and clearProcessor is reached from here
//     alone. One goroutine inserts a given site, because the duplicate checks in
//     InsertTxDag mean the second arrival returns before reaching this.
//
//   - the site's approval-target ids, through approvalTargetIDs.
//
// # The invariant the last one rests on
//
// The exclusive section has been released by the time this runs, so a commit
// transaction or the reconciler can run between it and the shared lock below.
// approvalTargetIDs takes the union of targets, slicedTargets and missingTargets,
// discards nils and duplicates and sorts what is left. Every concurrent writer of
// those three fields MOVES an id between them; none adds one and none drops one:
//
//   - slicing moves an id out of targets and into slicedTargets. sliceSites does
//     it through dropSettled, and dropSettledTargets does it for a site caught in
//     AddTxDag's signing window. Both record the id as they drop the pointer.
//
//   - relinking moves an id out of missingTargets and into targets.
//     ReconcileMissingTargets appends the target and deletes the key in the same
//     step.
//
//   - the only writers that add to the union - the resolution loop in
//     linkReceivedSite, and linkApprovals - run once per site, inside the
//     exclusive section, before this is called.
//
// Both movers also drop nil pointers out of targets without recording anything,
// which loses no id because approvalTargetIDs skips nils.
//
// So the union, and therefore the payload, is the same whichever side of a slice
// or a relink this happens to observe. That is what makes the check correct
// outside the exclusive section and not just cheaper. If any path could add an id
// to the union or drop one from all three, the payload would differ from the one
// the builder signed: an honest claim would fail to verify and this would strip a
// processor of the fee for work it did, or a tampered claim would pass and pay
// somebody who did nothing. TestTheApprovalIdSetIsInvariantUnderSlicingAndRelinking
// is that invariant, checked.
func (dag *Dag) checkProcessorClaim(site *Node) {
	dag.rlock()
	err := verifyProcessor(site)
	dag.runlock()
	if err == nil || errors.Is(err, ErrNoProcessorAttribution) {
		return
	}

	// Stripping is a write, so it needs the exclusive lock, and Go has no lock
	// upgrade: the shared lock has to be released first, which reopens the
	// window. Hence the re-check on the other side. A site that has left the live
	// graph in the meantime must not be written to - it is in the archive by
	// then, and the commit transaction that settled it has already recorded which
	// processor it credited, so clearing the field afterwards would change
	// nothing except the object a late arrival is resolved from.
	dag.mux.Lock()
	stillLive := dag.getById(site.id.id, true) != nil
	if stillLive {
		clearProcessor(site)
	}
	dag.mux.Unlock()

	if stillLive {
		logger.Warnf("[attribution] Site %s carries an unusable claim, dropping it: %s",
			site.id.id.String(), err.Error())
		return
	}
	logger.Warnf("[attribution] Site %s carries an unusable claim, but it was settled before the claim could be dropped: %s",
		site.id.id.String(), err.Error())
}

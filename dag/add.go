package dag

import (
	"time"

	"github.com/pkg/errors"

	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/google/uuid"
)

// Note:
// 	other vertices (nodes) this vertex references in DAG are targets for this node
//	and all other vertices (nodes) that reference this vertex (node) in DAG are sources
//  in regard to this node which is their target
//  vertex (node) <<--[source]-- vertex (current node) <<-- [target] -- vertex (after)

/*
AddTxDag - build a site around one of this node's own transactions and put it
into the graph.

The lock discipline is the point of this file. dag.mux serialises the whole node:
every insert, the slicer, the commit builder, the size gauges and the reconciler
take it, so whatever is done while holding it is done one transaction at a time
for the entire node. A mutex profile of a saturating node attributed 63.5% of all
contention - 3.93s of 9.12s - to the acquisition in this function, at 2,600
inserts a second and 35% CPU on four cores. Not enough work to saturate the
machine, so the ceiling was this lock and not the hardware.

Three things came out from under it, in descending order of what they cost:

  - the ed25519 signature over the site's processor claim. Measured at 54-73us
    per site (BenchmarkSiteAttributionSignature, the spread being how loaded the
    machine is) against 3-8us for tip selection and confirmation together
    (BenchmarkGraphGrowth), out of an uncontended insert of about 64us. Around
    nine tenths of the section, spent on a signature that concerns nobody but
    this node's own fee.

  - approveTx, which takes the pin lock once per approval target. That put a
    cross-lock dependency on the hot path - dag.mux held while waiting for a lock
    the commit builder also takes - and the pin lock is the second largest source
    of contention in the same profile, at 29.67%. approveTx reads the tips' tx
    and valid fields, both fixed when a site is built, and the pin store's
    balances behind the pin store's own lock. It touches no graph state at all.

  - the notification to the dag watcher, which was a blocking send into a channel
    of capacity one. See notifyDagModified.

What stays under the lock is in linkNewSite and publishNewSite, item by item.

The cost is three acquisitions where there was one, so an insert is no longer a
single atomic step, and two windows open in it:

  - between selection and linking, a chosen tip can be settled by a commit
    transaction. linkNewSite re-checks the tips against the live index, and if
    that leaves nothing it selects again under its own lock rather than refusing
    the transaction - a refusal here is a lost payment, because the publisher has
    already dequeued it.

  - between linking and publishing - the window the signature is made in - an
    approval target can be settled while the site is not yet in dag._dag_, where
    sliceSites cannot reach it. publishNewSite does to the site what sliceSites
    would have done: drops the pointer, keeps the id.

Neither window is free, but both are closed rather than tolerated, and the
alternative is holding the node's one global lock across an ed25519 signature.
*/
func (dag *Dag) AddTxDag(node *Node) ([]uuid.UUID, map[string][]byte, error) {
	defer stats.Time(stats.SiteInsert)()

	tips, viaGenesis, err := dag.approvalTargets(node)
	if err != nil {
		return nil, nil, err
	}

	// No dag lock held. approveTx reads node.valid and node.tx on the chosen
	// tips - both written when a site is built and never again - and the pin
	// store's balances, which the pin store guards itself. It touches no graph
	// state: not dag._dag_, not dag._links_, not the lookup maps, no Node field
	// that any other goroutine writes.
	//
	// The balances it sees may be marginally staler than they were when this ran
	// under the lock. That is not the check that decides whether the payment is
	// valid - UpdateBalanceIfValid inside linkApprovals is, and it still runs
	// under the lock - and approveNode only logs what it finds.
	if !dag.approveTx(tips) {
		// Unchanged behaviour: a transaction with nothing to approve, which is
		// anything that is not a payment, does not enter the graph, and that is
		// not an error. Empty rather than nil results, as before, because the
		// publisher iterates them.
		return []uuid.UUID{}, map[string][]byte{}, nil
	}

	linked, err := dag.linkNewSite(node, tips, viaGenesis)
	if err != nil {
		return nil, nil, err
	}

	// This node encapsulated the transaction, so it is the site's processor and
	// the party the site's fee is owed to. Signed after the approvals are linked
	// because they are part of what is signed - signing first would sign an
	// empty approval set - and before the site is published because that is what
	// makes writing to it safe.
	//
	// No dag lock held. signProcessor reads this site's id, its transaction and
	// its approval-target ids, and writes processorAddress, processorPk and
	// processorSig - three fields nothing else in the process ever writes. At
	// this moment the site is reachable only from the source lists of the tips
	// it approves: it is in neither dag._dag_, the lookup maps, nor the
	// confirmation tracker, and the tracker filters those source lists by its
	// own membership, so no walk, no commit build and no size gauge can arrive
	// at this site yet.
	//
	// Signing after publication would have been one lock acquisition cheaper and
	// wrong: a site is confirmable as soon as something approves it, the commit
	// builder reads confirmed sites through ToPbNode with no dag lock held, and
	// ToPbNode reads exactly these fields.
	//
	// A failure is logged and not returned: an unattributed site is a valid site
	// that earns nobody a fee, and refusing to publish a transaction because
	// this node cannot claim payment for it would be putting the fee ahead of
	// the ledger.
	if err := signProcessor(node, dag.Wallet()); err != nil {
		logger.Warnf("[attribution] Cannot claim site %s as ours: %s", node.id.id.String(), err.Error())
	}

	dag.publishNewSite(node, linked)

	// Cumulative weights are no longer recomputed here. See dag/weights.go: the
	// pass walked every live site on every insert and was the largest single
	// consumer of CPU in the node, at 15.95% of a profile under load, and
	// nothing reads what it produced.

	//dag.dag = updateFwdWeights(dag.dag, dag.links)
	if traceSites {
		// logLast walks the site's edges to format itself, so this is real graph
		// work done only to produce a log line. It reads dag._links_ and writes
		// nothing, so the shared lock is enough - and it gets its own
		// acquisition, so that the section every insert pays for does not carry a
		// debugging aid.
		dag.rlock()
		dag.logLast("ADD:", node, 1)
		dag.runlock()
	}
	return linked.uuIDs, linked.signatures, nil
}

// linkedSite - what the linking half of an insert hands to the publishing half,
// and what AddTxDag hands back to the publisher.
//
// Carried in one value rather than as three returns because the two halves have
// to agree on exactly this, across a window in which the lock is not held.
type linkedSite struct {
	// uuIDs, signatures - the approvals, in the form the publisher puts on the
	// wire so that a peer can rebuild the same site.
	uuIDs      []uuid.UUID
	signatures map[string][]byte
	// edges - the approved sites by value, for the dag watcher. Copied under the
	// lock because it copies out of slices other goroutines rewrite.
	edges []Node
}

// approvalTargets - the sites this new site will approve, and whether they came
// from the genesis fan-out rather than from selection.
//
// The read half of an insert, and the only part of AddTxDag that reads shared
// graph state without writing any. Selection walks Node.sources and Node.targets
// through the confirmation tracker, and linkApprovals appends to both while
// sliceSites rewrites both, under this same lock - so the walk has to hold it.
// Taking a read out from under a write lock is only safe when it moves under a
// read lock; moving it out altogether would be a plain data race dressed up as
// an optimisation.
//
// Tracker state does change during a walk - getTips runs expireStaleTips, which
// retires tips that have gone unapproved - but that state lives behind the
// tracker's own mutex, not behind dag.mux, and the tracker writes no Node field.
// So this section writes nothing dag.mux protects: not dag._dag_, not
// dag._links_, not the lookup maps, not the wallet cache, no Node field. That is
// what makes it the section a sync.RWMutex would let overlap with the other
// readers. See Dag.rlock.
func (dag *Dag) approvalTargets(node *Node) ([]*Node, bool, error) {
	if node.tx.GetTransactionType() != tx.PAYMENT {
		// Only a payment approves anything, and approveTx refuses an empty set,
		// so the site does not enter the graph. Long-standing behaviour, kept.
		return nil, false, nil
	}

	var (
		tips       []*Node
		viaGenesis bool
	)
	selectionStart := time.Now()
	dag.rlock()
	// How far the ledger has come, not how much of it is resident: slicing
	// shrinks the live graph, and measuring that would drop the node back
	// into the genesis-fanout phase and link new sites to genesis again.
	//
	// The counter is now incremented in publishSite, two acquisitions later, so
	// several concurrent inserts during the opening phase can all read a count
	// below the width and all fan out to genesis - a few more exodus sites than
	// dag.initialwidth asked for. That costs nothing: genesis is recorded as
	// harvested when its own commit transaction is formed, so extra approvers
	// cannot get it settled twice, and SyncUp treats "past the width" and
	// "exactly at the width" alike.
	if dag.sitesAdded.Load() < uint64(dag.width) {
		tips, viaGenesis = []*Node{dag.getGenesis()}, true
	} else {
		tips = dag.selectApprovalTargets()
	}
	dag.runlock()
	stats.Since(stats.TipSelection, selectionStart)

	if len(tips) == 0 {
		// Refusing beats accepting and discarding. The approval gate in AddTxDag
		// returns false for an empty set and there is no else branch, so this
		// used to return success with the site never added: the caller went on
		// to broadcast it, and the site then existed on every peer except its
		// author - whose balance was never updated for it either, since that
		// happens inside addToDag.
		//
		// This is still the least bad outcome and not a good one: the publisher
		// logs the error and moves on, and it has already taken the transaction
		// off the queue, so the transaction is dropped rather than retried. The
		// comment that used to be here said it was retried; it is not. Making it
		// a retry is a change in diffusion/publish.go, and it is in the report -
		// which is also why linkNewSite re-selects instead of failing.
		return nil, false, errors.Errorf("no site is available to approve, so payment tx %s cannot enter the dag", node.id.id.String())
	}
	return tips, viaGenesis, nil
}

// selectApprovalTargets - run the configured selection. Caller holds dag.mux,
// in either half of AddTxDag: this is a read of the graph, so the write lock
// satisfies it too.
func (dag *Dag) selectApprovalTargets() []*Node {
	if dagAlgorithm() == DAG_ALGO_RANDOM.Type() {
		return dag.uniformTips()
	}
	return dag.selectTips(dagConfig.Alpha)
}

// linkNewSite - the first write half of an insert: everything that decides what
// the site approves, and nothing that does not.
//
// Every item here is here because it has to be:
//
//   - the tip re-check. Selection ran under its own acquisition, so a commit
//     transaction may have settled a chosen tip since, and sliceSites may have
//     taken it out of the graph. Linking to it anyway would put a pointer to an
//     archived site into the live edge list, where no later commit will ever
//     prune it, the site being settled already so that nothing names it again.
//
//   - dag._links_ is a shared slice, and appending to it can reallocate the
//     backing array, so it cannot happen while anything is reading it.
//
//   - node.height is derived from the tips' heights, which only this lock keeps
//     from being rewritten under us, and is set before the site is published so
//     that no reader can find the site with a height of zero and watch it
//     change.
//
//   - linkApprovals applies the payment to the wallet cache and links the site
//     to the tips in both directions. The balance change is a read-modify-write
//     across two accounts and the wallet cache's own mutex only makes each half
//     of it atomic.
//
// The uuid and signature lists are built in the same loop as the links. They are
// derived from fields that are fixed once a site is built and could be built
// outside the lock, but that would mean a second pass over the tips to save two
// field reads.
func (dag *Dag) linkNewSite(node *Node, tips []*Node, viaGenesis bool) (*linkedSite, error) {
	dag.mux.Lock()
	defer dag.mux.Unlock()

	if !viaGenesis {
		tips = dag.liveOnly(tips)
		if len(tips) == 0 {
			// Every site chosen to approve was settled in the window between
			// selection and here. Select again rather than refusing the
			// transaction: the publisher has already dequeued it and does
			// nothing but log when an insert fails (diffusion/publish.go), so a
			// refusal here is a lost payment and not a retry.
			//
			// Safe under this lock, and only under a lock as exclusive as this
			// one: no site can be settled while it is held - sliceSites takes
			// dag.mux and takes settled sites out of the confirmation tracker
			// inside it - so what selection offers here is live, and stays live
			// until the edges below are appended. That is why the result is not
			// passed through liveOnly a second time, and it is deliberate that
			// it is not: liveOnly answers from the lookup map, and a graph built
			// by GenerateWideDag registers its sites with the tracker without
			// ever filling that map in.
			//
			// Not put through approveTx again either. That gate is "the set is
			// non-empty and holds no nil", which selection satisfies by
			// construction, and its balance lookups only log; running it here
			// would take the pin lock while holding dag.mux to decide nothing.
			tips = dag.selectApprovalTargets()
			// The counter selection already uses for "the first attempt did not
			// produce a usable set".
			stats.SelectionFallbacks.Inc()
			if len(tips) == 0 {
				// The graph has nothing approvable at all, which is the same
				// condition an empty first selection reports.
				return nil, errors.Errorf(
					"every site chosen to approve payment tx %s was settled, and the dag offered no replacement",
					node.id.id.String())
			}
		}
	}
	// The genesis fan-out is exempt from the re-check on purpose: genesis is
	// held as a pointer but is in no local index on a follower or a recovered
	// node - adoptGenesis keeps the pointer and empties the maps - so checking
	// it would refuse the first dag.initialwidth transactions on such a node.
	// That is a change of behaviour, not a fix; see uniformTips for what
	// approving genesis there actually costs.

	linked := &linkedSite{
		uuIDs:      make([]uuid.UUID, 0, len(tips)),
		signatures: make(map[string][]byte, len(tips)),
	}
	height := Height{0, 0}
	// add links from the current (new) node to the one/two tips in the dag
	for _, tip := range tips {
		linked.signatures[tip.id.id.String()] = tip.tx.GetSignature()
		dag._links_ = append(dag._links_, Link{source: node, target: tip})
		linked.uuIDs = append(linked.uuIDs, tip.id.id)
		if tip.height.maxheight > height.maxheight {
			height.maxheight = tip.height.maxheight + 1
		}
		if tip.height.minheight > height.minheight {
			height.minheight = tip.height.minheight + 1
		}
	}
	node.height = height

	edges, _, err := dag.linkApprovals(node, tips)
	if err != nil {
		return nil, err
	}
	linked.edges = edges
	return linked, nil
}

// publishNewSite - the second write half of an insert: make the site reachable.
//
// Separated from linkNewSite only so that the signature between them is made
// with the lock released; see AddTxDag. The target ids are re-derived here rather
// than carried across the window, because dropSettledTargets may have changed
// them - and an id in mapped_edges that Vertex() cannot resolve is a nil
// dereference waiting in dag/traverse.go.
func (dag *Dag) publishNewSite(node *Node, linked *linkedSite) {
	dag.mux.Lock()
	defer dag.mux.Unlock()
	if dag.dropSettledTargets(node) {
		logger.Debugf("[slice] Site %s had an approval target settled while it was being signed; the approval is reported by id",
			node.id.id.String())
	}
	dag.publishSite(node, linked.edges, targetIDs(node.targets))
}

func vertexToNode(vertex *tx.Vertex) *Node {
	return &Node{
		id: NodeID{
			id:      vertex.Id.Id,
			idMajor: vertex.Id.IdMajor,
			idMinor: vertex.Id.IdMinor,
		},
		cumWeight: vertex.CumWeight,
		txWeight:  vertex.TxWeight,
		time:      vertex.Timestamp,
		tx:        vertex.Tx,
	}
}

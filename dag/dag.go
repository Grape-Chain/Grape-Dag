package dag

import (
	"math/big"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/enescakir/emoji"
	"github.com/google/uuid"
	golog "github.com/ipfs/go-log/v2"
	"github.com/ledongthuc/goterators"
	"github.com/pkg/errors"
)

type Link struct {
	source *Node // a source vertex is a vertex that a target vertex references and approves
	target *Node // a target vertex is a vertex that approves the source vertex
}

type Dag struct {
	// optimize lookups by replacing the slice with a map keyed on UUID
	_dag_           []*Node
	mapped_vertices map[uuid.UUID]*Node
	_links_         []Link
	mapped_edges    map[uuid.UUID][]uuid.UUID
	mu_map          sync.RWMutex
	// prevMajor - the major half of the next site id, handed out by NewDagNode.
	//
	// Atomic because two goroutines hand out site ids: the publisher inserting a
	// transaction this node accepted, and the subscriber inserting one a peer
	// announced. It was a plain uint64 incremented and then read back as two
	// separate statements, which is both a data race and a correctness bug - two
	// overlapping inserts could read the same value back and give two different
	// sites the same major id.
	prevMajor     atomic.Uint64
	prevMinor     uint32
	mux           sync.RWMutex
	txCh          chan TxVL
	depthCh       chan Node
	pinCh         chan bool
	stopCh        chan bool
	pins          []*Node
	width         uint8
	exodusWallets []*grape1crypto.Wallet
	// genesis - held explicitly. The live node slice no longer holds the whole
	// ledger once settled sites are sliced out of it, so its first element is
	// not necessarily the genesis site.
	genesis *Node
	// sitesAdded - how many sites have ever been added, as opposed to how many
	// are still resident. The width checks below are about how far the ledger
	// has come, not about how much of it is currently in memory.
	sitesAdded atomic.Uint64
}

var (
	_dag_                *Dag                 = nil // global dag
	_pins_               *NodeTxPin           = nil
	__leaderReady__      atomic.Bool          = atomic.Bool{}
	chaintype            tx.ChainType         = tx.PRIVATE_TESTNET
	dagWallet            *grape_wallet.Wallet = nil // node's wallet with its own set of encr keys
	confirmationCounter  confirmations        = nil
	walletCache          *WalletCache         = nil // keep track of the current balances
	walletCacheConfirmed *WalletCache         = nil // keep track of the current balances without unconfirmed payment tx
	// sliceArchive - settled sites, out of the live graph but still findable.
	sliceArchive SliceArchive = nil
)

// traceSites - log a line for every site inserted and every approval made.
//
// Off unless -verbose is given. These lines were tied to peer.console, which is
// also what decides whether the node logs at all: an operator who wanted any
// log at all got a line per site and per approval with it, and one of them
// walked the site's edges to format itself. A node under load wrote two hundred
// megabytes of log in seven minutes, and writing it was the single largest
// consumer of CPU in a profile of that node - ahead of signature verification.
var traceSites bool

// traceBalances - log every wallet-cache credit and debit.
//
// Same switch as traceSites, kept as its own flag because these lines are about
// money rather than about graph shape and are wanted separately often enough to
// be worth the extra name. Read from a package variable rather than by calling
// config.GetConfig() at each credit, for two reasons: the balance path runs twice
// per insert, and GetConfig returns a file-backed global that is nil in a test
// binary - so asking it on the hot path meant the balance code could not be
// tested without standing up a configuration file.
var traceBalances bool

// confirmations - how the DAG decides a site is confirmed and ready for a
// commit transaction. Two implementations exist: ConfirmTracker, which measures
// the share of current tips that confirm each site (the technical paper's
// section 5.1), and ConfirmationCounter, the original fixed two-approver rule,
// kept selectable while the new one earns trust.
type confirmations interface {
	add(vertex *Node)
	relink(vertex *Node)
	pop() []*Node
	// peek - the confirmed sites without consuming them, and take - consume
	// exactly the named ones. Splitting pop() in two is what lets a validator
	// report what it holds confirmed without settling it: the report happens
	// before agreement, and possibly several times if a round has to be
	// repeated.
	peek() []*Node
	take(ids []uuid.UUID) []*Node
	tip() []*Node
	getTips() []*Node
	isTip(id uuid.UUID) bool
	markHarvested(id uuid.UUID)
	// walkFrom - one step's worth of the region a tip-selection walk runs over:
	// whether the site may be approved, its confirmation count, and the sites
	// that approve it with theirs.
	walkFrom(from *Node) (selectable bool, potential int, next []*Node, nextPotential []int)
	// walkBack - the sites this one approves that are still in the region, for
	// throwing a walk particle a bounded depth below the tips.
	walkBack(from *Node) []*Node
}

type DagAlgo uint8

const (
	DAG_ALGO_MCMCP DagAlgo = iota
	DAG_ALGO_MCMCPP
	DAG_ALGO_RANDOM
)

func (a DagAlgo) Type() string {
	return []string{"mcmc+", "mcmc++", "random"}[a]
}

const (
	// CONSENSUS_LEADER - a commit transaction is applied because a single
	// authorised signer asserted it. What the chain has always done.
	CONSENSUS_LEADER = "leader"
	// CONSENSUS_QUORUM - a commit transaction is applied because at least two
	// thirds of the validator set agreed to it.
	CONSENSUS_QUORUM = "quorum"
)

// consensusMode - the configured mode, normalised. An unrecognised value falls
// back to leader rather than to quorum: falling the other way would mean a node
// that cannot apply anything, which looks like a network outage rather than a
// configuration mistake.
func consensusMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case CONSENSUS_QUORUM:
		return CONSENSUS_QUORUM
	case CONSENSUS_LEADER, "":
		return CONSENSUS_LEADER
	default:
		logger.Warnf("[consensus] dag.consensus=%q is not %q or %q; using %q",
			raw, CONSENSUS_LEADER, CONSENSUS_QUORUM, CONSENSUS_LEADER)
		return CONSENSUS_LEADER
	}
}

const (
	DAG_ALPHA = 0.5
	// DAG_WALK_DEPTH - default for the paper's W. Deep enough that the forward
	// walk passes through several sites and the bias has somewhere to act,
	// shallow enough that selection stays cheap at a thousand transactions a
	// second.
	DAG_WALK_DEPTH = 10
	// DAG_CONFIRM_SHARE - default share of live tips that must confirm a site,
	// in permille. Two thirds, not the technical paper's literal 1000, and the
	// reasoning is in docs/confirmation.md: the literal rule stops confirming
	// entirely once more than a handful of nodes publish concurrently, because
	// it waits for every last tip while new tips keep joining the denominator.
	// Two thirds is the same fraction as the validator quorum, so a site is
	// confirmed under the share of the graph that a commit transaction needs of
	// the validator set. Set dag.confirmshare to 1000 for the literal rule.
	DAG_CONFIRM_SHARE = 667
	DAG_APPROVE_TX    = 2
	DAG_WIDTH         = 5
	DAG_LAMBDA        = 1
	DAG_TOTAL_TX      = 75
	UUID_TEMPLATE     = "6d89b2ad-9e07-46c4-8484-33c10b2dd6f%x"
	TX_MAXFUEL        = 999
	TX_MAXPRICE       = 10000000
	TX_NEUTRINO       = 0.00000001
)

var logger golog.EventLogger = golog.Logger("dag")
var peerConfig config.PeerConfiguration
var dagConfig config.DagConfiguration
var txConfig config.TxConfiguration

func (dag *Dag) Genesis() *Node {
	return dag.getGenesis()
}

func tipCache() confirmations {
	return confirmationCounter
}

func (d *Dag) GetWalletCache() *WalletCache {
	return walletCache
}

func (dag *Dag) Wallet() *grape_wallet.Wallet {
	if dagWallet == nil {
		dagWallet = grape1crypto.NewWallet()
	}
	return dagWallet
}

func (dag *Dag) Size() uint64 {
	dag.mux.Lock()
	defer dag.mux.Unlock()
	return uint64(len(dag._dag_))
}

func (d *Dag) Tps() float64 {
	d.mux.Lock()
	defer d.mux.Unlock()
	l := len(d._dag_)
	if l > 1 {
		n1 := d._dag_[1]
		nX := d._dag_[l-1]
		dur := nX.time.Sub(n1.time)
		return float64(l) / dur.Seconds()
	}
	return 0.
}

func (d *Dag) AvgDelay() int64 {
	d.mux.Lock()
	defer d.mux.Unlock()
	l := len(d._dag_)
	if l > 1 {
		n1 := d._dag_[1]
		n1dur := n1.time.Sub(time.UnixMilli(int64(n1.tx.GetTimestamp())))
		nM := d._dag_[l/2-1]
		nMdur := nM.time.Sub(time.UnixMilli(int64(nM.tx.GetTimestamp())))
		nX := d._dag_[l-1]
		nXdur := nX.time.Sub(time.UnixMilli(int64(nX.tx.GetTimestamp())))
		return (n1dur.Microseconds() + nXdur.Microseconds() + nMdur.Microseconds()) / 3
	}
	return 0.
}

// SnapshotNodes - a shallow copy of the current site slice, taken under the dag
// lock. Readers iterate the copy so that an append (which may reallocate the
// backing array) cannot race them.
func (d *Dag) SnapshotNodes() []*Node {
	d.mux.Lock()
	defer d.mux.Unlock()
	out := make([]*Node, len(d._dag_))
	copy(out, d._dag_)
	return out
}

func (d *Dag) lookupCache(id uuid.UUID) bool {
	_, ok := d.mapped_vertices[id]
	return ok
}

// mapped Vertices - is cache optimized lookup optimization
func (dag *Dag) lookupCacheUpdate(vertex *Node, targetIds []uuid.UUID) {
	// this, most likely, is a re-entry into dag from the primary thread of execution
	// hence use trylock
	dag.mu_map.Lock()
	defer dag.mu_map.Unlock()
	dag.mapped_vertices[vertex.id.id] = vertex
	if len(targetIds) == 0 {
		// Nothing to record. The append below would otherwise store a key with a
		// nil value for every site that approves nothing - which is every site
		// inserted by insertMissing or by the bypass path, i.e. every site on a
		// node that is catching up - and mapped_edges is walked whole by
		// updateMappedEdges and by the slice.
		return
	}
	dag.mapped_edges[vertex.id.id] = append(dag.mapped_edges[vertex.id.id], targetIds...)
}

// rlock, runlock - the read half of dag.mux, which is now a real shared lock.
//
// These began as names for the sections a shared lock would be able to overlap,
// while the lock was still exclusive. They are the shared acquisition now, so
// what was a naming convention has become a correctness claim: a section that
// quietly mutates anything dag.mux protects is corruption from here on, not just
// a missed opportunity. Every caller of these two therefore says what it touches
// and what it does not, and the concurrency tests in dag/lock_test.go run the
// read sections against inserts, slicing and commit-transaction harvesting under
// -race, which is what turns the claim into something checked.
//
// The read sections: Dag.approvalTargets (tip selection), which reads
// Node.sources and Node.targets through the confirmation tracker - the tracker
// mutation inside expireStaleTips is behind the tracker's own mutex and writes no
// Node field. Outside this file: Size, Tps, AvgDelay, SnapshotNodes and
// getFromLastPinTx here, GetConfirmedSites and its siblings in dag/site.go
// (pop/take mutate the tracker, not the graph), and refreshSizeGauges in
// dag/metrics.go.
//
// Also read sections, both added since: Dag.checkProcessorClaim in dag/insert.go,
// which verifies the claim a received site arrived with and takes the exclusive
// lock only for the rare failing claim it has to strip, and the two logLast calls
// on the insert paths, which walk dag._links_ to format a trace line.
//
// Not a read section, despite looking like one: anything that reaches
// clearProcessor or signProcessor. Those write Node fields, and clearProcessor is
// why checkProcessorClaim releases the shared lock and re-checks rather than
// pretending Go has a lock upgrade.
//
// One hazard the conversion to a shared lock created that did not exist before
// it: sync.RWMutex deadlocks on a recursive RLock as soon as a writer is queued.
// The writer blocks new readers, and a second RLock taken by a goroutine that
// already holds one then waits behind that writer, which is waiting for the
// first RLock to be released. A plain Mutex has no such shape, so a nested
// acquisition inside a read section is new surface and has to be checked for
// rather than assumed away.
//
// All six read sections were audited and none reaches dag.mux again:
// approvalTargets calls getGenesis and selectApprovalTargets, checkProcessorClaim
// calls verifyClaim, logLast walks dag._links_, prepareSites calls
// serialiseSites, and handleSiteRequest in dag/sync.go calls Node.ToPbNode. The
// confirmation tracker is taken beneath dag.mux by readers and writers alike and
// never reaches back - its one outward call, resolveSite, takes the lookup map's
// mutex - so that order is uniform in one direction too. A new read section
// needs the same check.
func (d *Dag) rlock()   { d.mux.RLock() }
func (d *Dag) runlock() { d.mux.RUnlock() }

// liveOnly - the subset of these sites that the live graph still holds.
//
// Membership of the lookup map is the same question: every path that appends to
// dag._dag_ registers the site in the map inside the same locked section
// (addToDag, insertMissing), and sliceSites deletes from both. Caller holds
// dag.mux, which is what keeps the answer true for long enough to act on it.
func (d *Dag) liveOnly(nodes []*Node) []*Node {
	for i, n := range nodes {
		if n != nil && d.getById(n.id.id, true) != nil {
			continue
		}
		// A site went between selection and here. Rare, so it is the only
		// branch that allocates.
		kept := make([]*Node, 0, len(nodes)-1)
		kept = append(kept, nodes[:i]...)
		for _, rest := range nodes[i+1:] {
			if rest != nil && d.getById(rest.id.id, true) != nil {
				kept = append(kept, rest)
			}
		}
		return kept
	}
	return nodes
}

// getById - get a node by its UUID
// Arguments:
//
//	id - site unique id
//	cacheOnly - check inly in cache if true, full search otherwise
//
// Returns:
//
//	*Node - if found, nil otherwise
func (d *Dag) getById(id uuid.UUID, cacheOnly bool) *Node {
	d.mu_map.RLock()
	defer d.mu_map.RUnlock()
	v, ok := d.mapped_vertices[id]
	if !ok && !cacheOnly {
		v = _dag_._getById_(id)
	}
	return v
}

// getById - get a *Node by its id (uuid)
// returns *Node if found, nil otherwise
//
// No callers left: InsertIfNotExist was the only one and now asks the lookup map
// alone, which cannot answer differently - see the note there. Kept rather than
// deleted because getById's cacheOnly parameter is called from files this change
// does not own, but it should go with them. It cannot be made safe as written:
// TryLock carries on unlocked when the lock is held, which is precisely when the
// scan below is racing an append to _dag_, and taking the lock unconditionally
// instead would deadlock the callers that already hold it.
func (d *Dag) _getById_(id uuid.UUID) *Node {
	if d.mux.TryLock() {
		defer d.mux.Unlock()
	}
	// the lookup can be optimized
	node, _, err := goterators.Find(d._dag_, func(n *Node) bool {
		return n.id.id == id
	})
	if err != nil {
		logger.Warnf("Site with id=%s not found in dag", id.String())
		return nil
	}
	return node
}

func (dag *Dag) updateMappedVertices() {
	dag.mu_map.Lock()
	defer dag.mu_map.Unlock()
	goterators.ForEach(dag._dag_, func(vertex *Node) {
		dag.mapped_vertices[vertex.id.id] = vertex
	})
}

func (dag *Dag) updateMappedEdges() {
	dag.mu_map.Lock()
	defer dag.mu_map.Unlock()
	goterators.ForEach(dag._links_, func(edge Link) {
		if _, _, err := goterators.Find(dag.mapped_edges[edge.source.id.id], func(id uuid.UUID) bool {
			return id == edge.target.id.id
		}); err != nil {
			dag.mapped_edges[edge.source.id.id] = append(dag.mapped_edges[edge.source.id.id], edge.target.id.id)
		}
	})
}

func (dag *Dag) Vertex(id uuid.UUID) *Node {
	dag.mu_map.RLock()
	defer dag.mu_map.RUnlock()
	v, ok := dag.mapped_vertices[id]
	if ok {
		return v
	}
	return nil
}

func (dag *Dag) Terminate() {
	utils.ColorizeInfo(logger, "[dag] Persisting the latest DAG state")
	// The commit-transaction chain is written as it is formed, so there is
	// nothing to flush here beyond closing the store. Unconfirmed sites are
	// deliberately not persisted: no commit transaction has settled them, so
	// they come back from the network.
	closeStore()
	logger.Info("[dag] Stopping the DAG watcher")
	dag.stopCh <- true
	// Detached under the lock, then closed outside it.
	//
	// A send on a closed channel panics, and notifyDagModified sends into this
	// one from insertMissing and publishSite - both of which hold dag.mux, which
	// is why taking it here is enough to know no send is in flight. Closing it
	// while an insert was mid-send took the node down on shutdown, which is a
	// crash on the way out rather than a crash in service, but it is also the
	// last thing an operator sees and it looks exactly like data loss.
	//
	// The field is cleared rather than only closed so that an insert arriving
	// after this point finds nil and returns, instead of finding a closed channel
	// and panicking.
	dag.detachNotify()
	if dag.depthCh != nil {
		logger.Info("[dag] Closing the tx depth channel")
		close(dag.depthCh)
	}
	if dag.pinCh != nil {
		logger.Info("[dag] Closing the pin tx channel")
		close(dag.pinCh)
	}
	t := time.NewTimer(time.Duration(time.Millisecond * 300))
	<-t.C
	t.Stop()
	logger.Info("[dag] Closing the Stop channel")
	close(dag.stopCh)
	utils.ColorizeInfo(logger, "[dag] DAG successfully persisted and terminated")
}

// detachNotify - take the watcher channel away from the insert path, then close
// it. Split out of Terminate so the ordering can be tested without standing up a
// store and a host.
func (d *Dag) detachNotify() {
	d.mux.Lock()
	notify := d.txCh
	d.txCh = nil
	d.mux.Unlock()
	if notify != nil {
		logger.Info("[dag] Closing the TX channel")
		close(notify)
	}
}

func (d *Dag) countConfirmed(vertex *Node) {
	confirmationCounter.add(vertex)
}

func (d *Dag) insertMissing(vertex *Node) error {
	d._dag_ = append(d._dag_, vertex)
	d.sitesAdded.Add(1)
	// update lookup cache
	// Node: this step is important for efficient lookups
	d.lookupCacheUpdate(vertex, targetIDs(vertex.targets))
	d.notifyDagModified(vertex, nodeValues(vertex.targets))

	return nil
}

// targetIDs, nodeValues - the two shapes the insert path needs its neighbour
// list in. Plain loops rather than goterators.Map: both run inside the dag lock
// on every site, and a closure per site per shape is a closure per site the node
// holds the whole ledger still for.
func targetIDs(nodes []*Node) []uuid.UUID {
	if len(nodes) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		ids = append(ids, n.id.id)
	}
	return ids
}

func nodeValues(nodes []*Node) []Node {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		out = append(out, *n)
	}
	return out
}

// addToDag - put a site into the graph: apply its payment, link it to the sites
// it approves, and publish it. Caller holds dag.mux.
//
// The two halves are separate functions because AddTxDag has to let go of
// dag.mux between them: the site's processor signature is an ed25519 signature,
// measured at 72.7us against 4-10us for tip selection and confirmation together,
// and it has to be made after the approvals are linked and before the site is
// reachable. Everything else goes through here and gets both halves under one
// acquisition, exactly as before.
func (dag *Dag) addToDag(node *Node, linksTo []*Node) (*Dag, error) {
	edges, ids, err := dag.linkApprovals(node, linksTo)
	if err != nil {
		return dag, err
	}
	dag.publishSite(node, edges, ids)
	return dag, nil
}

// linkApprovals - the half of an insert that decides what the site approves: the
// payment, and the edges in both directions. Caller holds dag.mux.
//
// Returns the neighbours by value and their ids because the publish half needs
// both, and both have to be taken while the lock is held: they copy out of the
// same edge slices that this function and sliceSites rewrite.
func (dag *Dag) linkApprovals(node *Node, linksTo []*Node) ([]Node, []uuid.UUID, error) {
	// at this point we either accept the transaction, update its balance or bail out
	// unless dealing with synchronization
	// in case of synchronization: we still need to reconcile the balances,
	// but updating balances when there are missing transactions makes it difficult
	// @TODO: find a solution to balance update during synchronization; for now skip
	// balance update when we are in sync mode: aka linksTo is nil
	//
	// Under dag.mux, and it has to be: the payment is a read-modify-write across
	// two accounts, and the wallet cache's own mutex only makes each half of it
	// atomic. Two inserts overlapping here would interleave a debit and a credit
	// and leave a balance neither of them computed.
	if linksTo != nil && !node.UpdateBalanceIfValid() {
		logger.Errorf("[!] Invalid balance for %s. Ignore", node.tx.String())
		return nil, nil, errors.Errorf("[!] Invalid balance for %s. Ignore", node.tx.String())
	}

	e := make([]Node, 0, len(linksTo))
	ids := make([]uuid.UUID, 0, len(linksTo))
	for _, v := range linksTo {
		if v == nil {
			continue
		}
		e = append(e, *v)
		ids = append(ids, v.id.id)
		// add this node to the tips as
		if v.sources == nil {
			v.sources = []*Node{}
		}
		v.sources = append(v.sources, node)
		if node.targets == nil {
			node.targets = []*Node{}
		}
		node.targets = append(node.targets, v)
	}
	return e, ids, nil
}

// publishSite - the half of an insert that makes the site reachable: the node
// slice, the lookup maps, the confirmation tracker and the dag watcher. Caller
// holds dag.mux.
//
// Nothing here can fail, which is what makes it safe for AddTxDag to run it in a
// second acquisition: there is no state to unwind if the site turns out not to
// be publishable, because by this point it always is.
func (dag *Dag) publishSite(node *Node, edges []Node, ids []uuid.UUID) {
	node.time = time.Now()
	dag._dag_ = append(dag._dag_, node)
	dag.sitesAdded.Add(1)
	stats.SitesAdded.Inc()
	// update lookup cache
	// Node: this step is important for efficient lookups
	dag.lookupCacheUpdate(node, ids)
	// after adding a new vertex to DAG, update the tip counter
	// as we need them when finding confirmed vertices
	dag.countConfirmed(node)

	// notify the dag watcher for changes - dag watcher keeps track of all the changes
	// and builds a reverse graph representation of our dag
	// dag_watcher(sync.go) reads from the channel and calls Notify
	// which keeps the graph up to date
	dag.notifyDagModified(node, edges)
}

// dropSettledTargets - drop approval targets that have been settled since they
// were linked, keeping their ids.
//
// Exactly what sliceSites does to a site that is in the graph when a commit
// transaction lands. AddTxDag needs it because it links a site's approvals in
// one locked section and publishes the site in another, so a commit can settle
// one of its targets in between - at which point the site is not yet in
// dag._dag_ and sliceSites does not reach it. Without this the site would hold a
// pointer that nothing will ever prune, keeping an archived site alive, and its
// entry in mapped_edges would name a site that Vertex() cannot resolve.
//
// The approval itself is not lost: ToPbNode reports both the pointers and the
// recorded ids, so a peer rebuilds the same approval set either way.
//
// Caller holds dag.mux. Returns true when something was dropped.
func (d *Dag) dropSettledTargets(node *Node) bool {
	if !dagConfig.Slicing || len(node.targets) == 0 {
		return false
	}
	dropped := false
	kept := node.targets[:0]
	for _, t := range node.targets {
		if t == nil {
			continue
		}
		if d.getById(t.id.id, true) == nil {
			node.slicedTargets = append(node.slicedTargets, t.id.id)
			dropped = true
			continue
		}
		kept = append(kept, t)
	}
	for i := len(kept); i < len(node.targets); i++ {
		node.targets[i] = nil
	}
	node.targets = kept
	return dropped
}

// txNotifyBuffer - how many site notifications may be in flight to the dag
// watcher.
//
// Was one. A capacity of one makes every send a rendezvous with the watcher, and
// the send happens with dag.mux held, so the whole node ran at the pace of the
// watcher's slowest Notify: chansend was 25% of a node's blocking profile over
// 2.6 hours of accumulated blocking. Four thousand is a few hundred kilobytes of
// TxVL and absorbs any burst the watcher can catch up on within a commit
// interval.
const txNotifyBuffer = 4096

// txNotifyDrops - notifications the watcher was too far behind to take.
var txNotifyDrops atomic.Uint64

// notifyDagModified - hand a new site to the dag watcher without ever waiting
// for it.
//
// The watcher feeds DepthHandler.Notify (dag/sync.go), which builds a second,
// reverse copy of the whole graph in package graph/ and renders it to a new
// ./dag.graph.N.gv file on every commit transaction. That copy is read by
// exactly one function, getLatestDagSlice in dag/traverse.go, which has no
// caller anywhere in the repository - so the only thing the feed produces is the
// visualisation, and the visualisation was never gated on the setting that
// exists to ask for it. A node left running for a day wrote seventeen thousand
// files and did seventeen thousand full-graph DOT renders that nobody asked for.
//
// So the channel is now only created when peer.visualize is set (see
// RunSynchronization), and this returns immediately when it is not - which is
// the default. Gated rather than deleted: the visualisation is a real feature
// for whoever wants to look at the graph, and it is the mirror's only purpose,
// so switching it off is the fix and removing it is a decision for its users.
//
// When it is on, it must still not be able to stop the ledger. The send is
// non-blocking: a dropped notification costs a vertex in a structure only the
// picture reads, whereas a blocking send costs every insert, every slice, every
// gauge sample and every commit behind dag.mux - chansend was 25% of a node's
// blocking profile when this was a rendezvous. Drops are counted rather than
// left silent, because a drop makes Notify log a missing-source-site error for
// the sites that follow it.
//
// The TxVL is built by the caller under the lock, because it copies the site and
// its neighbours by value out of slices that other goroutines mutate.
func (dag *Dag) notifyDagModified(vertex *Node, edges []Node) {
	if dag.txCh == nil {
		return
	}
	select {
	case dag.txCh <- TxVL{vertex: *vertex, edges: edges}:
	default:
		// Logged on the first drop and then sparsely: the condition is a
		// watcher that has fallen behind, and it either resolves or it does not.
		if n := txNotifyDrops.Add(1); n == 1 || n%100000 == 0 {
			logger.Warnf("[dag] The dag watcher is behind; dropped %d site notification(s). Nothing reads what it builds - see notifyDagModified", n)
		}
	}
}

func (d *Dag) getGenesis() *Node {
	if d == nil {
		return nil
	}
	if d.genesis != nil {
		return d.genesis
	}
	if len(d._dag_) > 0 {
		return d._dag_[0]
	}
	return nil
}

func initDagWallet(dagConfig config.DagConfiguration) *grape_wallet.Wallet {
	return grape_wallet.LoadWallet(dagConfig.Publickey, dagConfig.Privatekey)
}

func InitDagToStats(statsId uuid.UUID) {
	goterators.ForEach(_dag_._dag_, func(n *Node) {
		stats.Enqueue(statsId, tx.ConvertToGrapeTx(grapepeer.GetHost(), n.tx), stats.TX_TYPE_PUB, 0, time.Duration(0))
	})
}

func Init() {
	logger = golog.Logger("dag")
	x := config.GetConfig()
	if x != nil {
		peerConfig = x.Peer
		dagConfig = x.Dag
		txConfig = x.Tx
		chaintype = tx.ChainType(x.Peer.Network)
	} else {
		chaintype = tx.PRIVATE_TESTNET
		peerConfig = config.PeerConfiguration{
			Network: 2, // Default value is set to PRIVATE_TESTNET
		}
		dagConfig = config.DagConfiguration{
			Algorithm:    DAG_ALGO_MCMCP.Type(),
			Alpha:        DAG_ALPHA,
			Approvetx:    DAG_APPROVE_TX,
			Initialwidth: DAG_WIDTH,
			Lambda:       DAG_LAMBDA,
			Totaltx:      DAG_TOTAL_TX,
		}
		logger.Warnf("Using default DAG config\n%s", &dagConfig)
		txConfig = configTxFallback()
	}
	walletCache = newWalletCache()
	walletCacheConfirmed = newWalletCache()
	if x != nil {
		traceSites = x.Host.Verbose > 0
		traceBalances = x.Host.Verbose > 0
	}
	dagWallet = initDagWallet(dagConfig)
	confirmationCounter = newConfirmations(dagConfig)
	if err := configurePinSigners(dagConfig.Pinsigners); err != nil {
		// Starting with a misread signer list would mean starting with no
		// authorised signer, which is the state this check exists to prevent.
		logger.Fatalf("[pin auth] %s", err.Error())
	}
	if consensusMode(dagConfig.Consensus) == CONSENSUS_QUORUM {
		if err := configureValidators(dagConfig.Validators); err != nil {
			// A misread validator list would shrink the set and lower the quorum
			// with it, which is worse than not starting.
			logger.Fatalf("[pin auth] %s", err.Error())
		}
	}
	logPinAuthority()
	if consensusMode(dagConfig.Consensus) == CONSENSUS_QUORUM {
		runner, err := newConsensusDriver(config.PIN_TX_TIMER_DEF)
		if err != nil {
			// A validator that cannot run the protocol is a validator missing
			// from every quorum, which is worse than not starting.
			logger.Fatalf("[consensus] %s", err.Error())
		}
		consensusRunner = runner
		if runner != nil {
			utils.ColorizeInfo(logger, "[consensus] This node is a validator and will take part in agreeing commit transactions")
		} else {
			logger.Infof("[consensus] This node is not in the validator set; it applies what the set agrees")
		}
	}
	logTipSelection()
	logVersionCollision()
	sliceArchive = newRamArchive()
	_pins_ = newNodeTxPin()
	// Note: genesis node is the only node that is authorized to create the genesis tx as the starting
	// point in dag. This is not currently implemented.
	// The genesis node creates a genesis tx, all other nodes, upon start, must synchronize with the
	// genesis node and obtain, among other tx, the genesis tx.
	// Currently, every node that starts up, creates its own genesis tx. This must be implemented asap.
	// Decide before any genesis is minted whether this node already has a
	// ledger. Checking afterwards would find the genesis pin newDag just wrote
	// and mistake a first run for a recovery.
	hasStoredChain, err := storedChainExists()
	if err != nil {
		// Refusing to start beats silently forking away from our own history.
		logger.Fatalf("[store] Cannot read the ledger store: %s", err.Error())
	}
	recoveringChain = hasStoredChain

	_dag_ = newDag(dagConfig)

	if hasStoredChain {
		recovered, err := recoverFromStore()
		if err != nil {
			logger.Fatalf("[store] Cannot recover the ledger: %s", err.Error())
		}
		if recovered {
			if genesis := recoveredGenesisSite(); genesis != nil {
				_dag_.adoptGenesis(genesis)
			}
			utils.ColorizeInfo(logger, "[store] Continuing an existing ledger; skipping the balance snapshot handshake")
			return
		}
	}

	if config.GetConfig().Host.Leader {
		// Only meaningful on a fresh chain: once the genesis wallet has spent
		// anything, its balance is no longer the initial offering.
		ico, err := _pins_.unsafe_getBalanceForWallet(dagWallet.WalletAddress())
		if err != nil {
			panic(err.Error())
		}
		if ico == nil || ico.Cmp(_dag_.getGenesis().tx.GetAmount()) != 0 {
			panic("Genesis amount in cache is incorrect. Cannot contine initialization")
		}
	}
}

// logVersionCollision - say at startup that dag.versioncollision does nothing.
//
// The setting used to select a branch in InsertTxDag that refused any received
// site colliding on version and could never do anything else; see the note where
// it was removed. An operator who has it set in a config file is expecting
// behaviour that is not there, and silence would let them keep expecting it.
func logVersionCollision() {
	if dagConfig.Versioncollision {
		logger.Warnf("[dag] dag.versioncollision is set but has no effect: the collision branch it selected refused every colliding site and never entered one into the graph, and it has been removed. Drop the setting.")
	}
}

// adoptGenesis - take the genesis site from the recovered chain as the graph
// root, in place of the one minted at start-up.
func (d *Dag) adoptGenesis(genesis *Node) {
	if d == nil || genesis == nil {
		return
	}
	d.mux.Lock()
	defer d.mux.Unlock()
	d.genesis = genesis
	// The recovered chain already settled it, so it belongs in the archive
	// rather than the live graph. Keep the live graph empty of it and let
	// sitesAdded reflect a ledger that is past its opening phase.
	d._dag_ = []*Node{}
	d.mapped_vertices = make(map[uuid.UUID]*Node)
	d.mapped_edges = make(map[uuid.UUID][]uuid.UUID)
	d._links_ = nil
	if added := uint64(d.width) + 1; d.sitesAdded.Load() < added {
		d.sitesAdded.Store(added)
	}
}

func (sm *DagSyncMngr) RunSynchronization(leader bool, wait_connect bool) {
	if _dag_ != nil {
		if host := grapepeer.GetHost(); host == nil {
			logger.Fatal("This peer's host has not been initialized")
		}
		// we set the sync channel for transactions here so that we can run
		// it only when synchronization is enabled
		//
		// Only when peer.visualize asks for it. The channel feeds the graph
		// mirror, whose only reader is the graphviz render, so with the setting
		// off there is nothing downstream to receive a notification - and a nil
		// channel is what makes notifyDagModified cost nothing at all rather than
		// cost a select on every insert. See txNotifyBuffer and
		// notifyDagModified: this used to be a capacity of one, sent into with
		// dag.mux held.
		if peerConfig.Visualize > 0 {
			_dag_.txCh = make(chan TxVL, txNotifyBuffer)
		} else {
			logger.Info("[dag] Graph mirror and graphviz output are off (peer.visualize=0)")
		}
		_dag_.depthCh = make(chan Node, 1)
		_dag_.pinCh = make(chan bool, 1)
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go sm.dag_watcher(leader, wait_connect, wg)
		wg.Wait()
		logger.Info("[dag sync] DAG synchronization is running...")
	} else {
		logger.Fatal("DAG has not been initialized")
	}
}

func GetDag() *Dag {
	return _dag_
}

func GetPin() *NodeTxPin {
	return _pins_
}

func genRandomTxWeight() float64 {
	txWeight := rand.NormFloat64() + config.TX_WEIGHT_MEAN
	if txWeight < config.TX_WEIGHT_LOWER_LIMIT {
		txWeight = config.TX_WEIGHT_LOWER_LIMIT
	} else if txWeight > config.TX_WEIGHT_UPPER_LIMIT {
		txWeight = config.TX_WEIGHT_UPPER_LIMIT
	}
	return txWeight
}

func (d *Dag) approveNode(node *Node) error {
	if node == nil {
		return errors.New("[approveNode] node is nil")
	}
	//
	if !node.valid {
		return errors.New("[approveNode] node is invalid")
	}
	// check balance
	balances, err := GetPin().GetBalances([][]byte{node.tx.GetSender()})
	if err != nil {
		return err
	}
	sndBal := big.NewInt(0).SetBytes(balances[0])
	if sndBal.Sign() < 0 {
		return errors.New("Sender balance is negative")
	}
	// if necessary, check the receiver's balance as well
	return nil
}

// approveTx - process tx approval logic
// requires additional implementation
func (dag *Dag) approveTx(nodes []*Node) bool {
	if len(nodes) == 0 {
		return false
	}
	count := dagConfig.Approvetx
	goterators.ForEach(nodes, func(node *Node) {
		if node != nil {
			if err := dag.approveNode(node); err == nil {
				// Guarded at the call site, not inside the log call: the
				// arguments are evaluated either way, and id.String() allocates.
				if traceSites {
					utils.ColorizeInfo(logger, "Approved TX:[%s|%d.%d]", node.id.id.String(), node.id.idMajor, node.id.idMinor)
				}
			} else {
				utils.ColorizeError(logger, "Failed TX:[%s|%d.%d] err:", node.id.id.String(), node.id.idMajor, node.id.idMinor, err.Error())
			}
		} else {
			count--
		}
	})
	return count == dagConfig.Approvetx
}

func (dag *Dag) getAdjEdges(vertex *Node) []*Node {
	// see where the current node is the source
	edges := goterators.Filter(dag._links_, func(link Link) bool {
		return link.source.Equal(vertex)
	})
	targetVertices := []*Node{}
	goterators.ForEach(edges, func(edge Link) {
		targetVertices = append(targetVertices, edge.target)
	})
	return targetVertices
}

func (dag *Dag) logLast(pref string, node *Node, depth int) {
	//nodes, _ := getDescendants(dag._dag_, dag._links_, node)
	nodes := dag.getAdjEdges(node)
	if len(nodes) == 2 {
		// left_node := fmt.Sprintf("%03d.%d|cW:%04f,tW:%04f", nodes[0].id.idMajor, nodes[0].id.idMinor, nodes[0].cumWeight, nodes[0].txWeight)
		// right_node := fmt.Sprintf("%03d.%d|cW:%04f,tW:%04f", nodes[1].id.idMajor, nodes[1].id.idMinor, nodes[1].cumWeight, nodes[1].txWeight)
		// this_node := fmt.Sprintf("%03d.%d|cW:%04f,tW:%04f", node.id.idMajor, node.id.idMinor, node.cumWeight, node.txWeight)
		utils.ColorizeInfo(logger,
			"\n\n%s [%03d.%d] <<-- [%03d.%d] -->> [%03d.%d]\n",
			pref,
			nodes[0].id.idMajor, nodes[0].id.idMinor,
			node.id.idMajor, node.id.idMinor,
			nodes[1].id.idMajor, nodes[1].id.idMinor,
		)
	} else {
		goterators.ForEach(nodes, func(v *Node) {
			utils.ColorizeInfo(logger,
				//"\n\n%s[%03d.%d|cW:%04f,tW:%04f] <<-- [%03d.%d|cW:%04f,tW:%04f]\n",
				"\n\n%s[%03d.%d] <<-- [%03d.%d]\n",
				pref,
				v.id.idMajor, v.id.idMinor, node.id.idMajor, node.id.idMinor,
			)
		})
	}
}

// getApprovers is gone with the version-collision branch in dag/insert.go, which
// was its only caller. It answered "who approves this site" by scanning the whole
// edge list whenever the site's own source list happened to be empty, which is a
// pass over the live graph to recover something the site already records.

func getTargetLists(nodes []*Node, links []Link) map[uint64][]*Node {
	childrenLists := map[uint64][]*Node{}
	// go over all links and for each source tx node add a target tx node
	for _, link := range links {
		childrenLists[link.source.id.idMajor] = append(childrenLists[link.source.id.idMajor], link.target)
	}
	return childrenLists
}

func (d *Dag) SyncUp() {
	added := int(d.sitesAdded.Load())
	if added == int(d.width) || (added > int(d.width) && !__leaderReady__.Load()) {
		// when we reach this condition we form yet another pin tx with exodus sites
		// exodus sites are sites that directly link to genesis
		genPinTx()
		logger.Infof(" %s LEADER %s READY", emoji.Pushpin, emoji.CheckMarkButton)
		// allow sync to run on the leader peer
		__leaderReady__.Store(true)
	}
}

// reconcileMissingTargets - attemp to re-link targets for sites that were added
// without target links. This happens when peer is out of sync with the network
// and gradually attempts to re-build its own copy of dag
func (d *Dag) ReconcileMissingTargets() {
	d.mux.Lock()
	defer d.mux.Unlock()

	// find all Vertices(nodes) that have missingTargets as not nil
	// and see if we can link them to the sites in our copy of dag
	incompleteVertices := goterators.Filter(d._dag_, func(v *Node) bool {
		return len(v.missingTargets) > 0
	})

	requestVertices := []string{}
	// now, that we have the incomplete vertices, find target vertices
	// to link to
	goterators.ForEach(incompleteVertices, func(v1 *Node) {
		// go over each incomplete vertex and find vertices we can link to
		possibleTargets := goterators.Filter(d._dag_, func(v2 *Node) bool {
			// skip our copy
			if v1.id.id == v2.id.id {
				return false
			}
			if _, ok := v1.missingTargets[v2.id.id.String()]; ok {
				// this is a vertex we can link
				return true
			}
			return false
		})
		goterators.ForEach(possibleTargets, func(v3 *Node) {
			if _, ok := v1.missingTargets[v3.id.id.String()]; ok {
				v1.targets = append(v1.targets, v3)
				d._links_ = append(d._links_, Link{
					source: v1,
					target: v3,
				})
				delete(v1.missingTargets, v3.id.id.String())
			}
		})
		if len(v1.missingTargets) == 0 {
			v1.missingTargets = nil
			// Now that its approval targets are linked, this site can take part
			// in confirmation. Without this it stayed invisible to the tracker
			// and could never be confirmed.
			if confirmationCounter != nil {
				confirmationCounter.relink(v1)
			}
		} else {
			for k := range v1.missingTargets {
				requestVertices = append(requestVertices, k)
			}
		}
	})
	if len(requestVertices) > 0 {
		// request missing sites from other nodes
		requestMissingVertices(requestVertices)
	}
}

func requestMissingVertices(vertexIds []string) {
	logger.Infof("Request missing sites with IDs: %q", vertexIds)
	err := transactSiteRequest(vertexIds)
	if err != nil {
		logger.Errorf("Request missing sites - err:%s", err.Error())
	}
}

func (d *Dag) getVertices(ids []string) ([]*Node, error) {
	vIds := goterators.Map(ids, func(id string) uuid.UUID {
		mapId, _ := uuid.Parse(id)
		return mapId
	})
	var err error = nil
	vertices := []*Node{}
	goterators.ForEach(vIds, func(id uuid.UUID) {
		n := d.Vertex(id)
		if n == nil {
			logger.Warnf("Site with id: %s is missing", id.String())
		} else {
			vertices = append(vertices, n)
		}
	})
	if len(ids) > len(vertices) {
		err = errors.Errorf("Not all sites have been found")
	}
	return vertices, err
}

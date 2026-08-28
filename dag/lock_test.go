package dag

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/config"
	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/google/uuid"
)

/*
What dag.mux protects, and what happens when several goroutines want it at once.

A running node has at least two goroutines inserting - the publisher, which
builds sites out of this node's own transactions, and the diffusion subscriber,
which builds them out of everybody else's - plus the commit builder draining the
confirmed set, the slicer taking settled sites out of the graph, the size gauge
sampler, the missing-target reconciler, and whatever a REST or eth-RPC caller
asks for. All of them take dag.mux, and AddTxDag no longer holds it across the
whole of an insert: it selects under one acquisition, links under a second and
publishes under a third, signing the site in between with the lock released.

That is three windows where there was none, so these tests assert the properties
the single acquisition used to give for free:

  - no data race, which is what -race is for and why every reader here goes
    through the same entry points the node's own goroutines use;

  - the parts of the graph agree with each other afterwards - the node slice, the
    edge list, both lookup maps, the site counter, the confirmation tracker and
    each site's own edge lists;

  - every site's processor claim verifies, which is the property that pins the
    signature to the right place in the sequence. The claim covers the site's
    approval-target ids, so a claim that verifies is proof the signature was made
    after the approvals were linked and that nothing changed them afterwards.

Run them the way they are meant to be run: go test -race ./dag/ -run Concurrent
*/

// lockFixture - the process globals an insert reaches for, restored on cleanup.
//
// Deliberately opens neither a store nor a network: nothing on the insert path
// needs either, and the point of these tests is to run many goroutines against
// one graph rather than to exercise durability.
func lockFixture(t testing.TB, width uint16) *Dag {
	t.Helper()
	prevDag, prevCfg, prevTx, prevPeer := _dag_, dagConfig, txConfig, peerConfig
	prevCounter, prevPins, prevArchive := confirmationCounter, _pins_, sliceArchive
	prevCache, prevConfirmed, prevWallet := walletCache, walletCacheConfirmed, dagWallet
	prevTrace := traceSites

	dagConfig = config.DagConfiguration{
		Algorithm:    DAG_ALGO_MCMCP.Type(),
		Alpha:        DAG_ALPHA,
		Approvetx:    DAG_APPROVE_TX,
		Initialwidth: width,
		Walkdepth:    DAG_WALK_DEPTH,
		Confirmshare: DAG_CONFIRM_SHARE,
		Slicing:      true,
	}
	// Fees off, stated rather than left to the zero value - see feesOff.
	txConfig = config.TxConfiguration{Feestartpin: feesOff}
	peerConfig = config.PeerConfiguration{Network: 2}
	confirmationCounter = newConfirmTracker(0, DAG_CONFIRM_SHARE)
	sliceArchive = newRamArchive()
	_pins_ = newNodeTxPin()
	walletCache, walletCacheConfirmed = newWalletCache(), newWalletCache()
	dagWallet = grape_wallet.NewWallet()
	traceSites = false

	// One commit transaction stating a balance for the account these sites pay
	// from, so that approveTx's balance lookup finds something. Without it every
	// insert logs a not-found error per approval target, which is noise in a
	// test and distortion in a benchmark.
	opening := storedPin(0, nil, map[string]string{addrStr(0xaa): "1000000000000000"})
	_pins_.LockPin()
	_pins_.unsafe_appendPin(opening)
	_pins_.UnlockPin()

	genesis := lockSite(0)
	d := &Dag{
		_dag_:           []*Node{genesis},
		mapped_vertices: map[uuid.UUID]*Node{genesis.id.id: genesis},
		mapped_edges:    map[uuid.UUID][]uuid.UUID{},
		genesis:         genesis,
		width:           uint8(width),
	}
	d.sitesAdded.Store(1)
	confirmationCounter.add(genesis)
	// resolveSite, refreshSizeGauges and sliceAppliedPin all reach the graph
	// through the package global rather than through a parameter.
	_dag_ = d

	t.Cleanup(func() {
		_dag_, dagConfig, txConfig, peerConfig = prevDag, prevCfg, prevTx, prevPeer
		confirmationCounter, _pins_, sliceArchive = prevCounter, prevPins, prevArchive
		walletCache, walletCacheConfirmed, dagWallet = prevCache, prevConfirmed, prevWallet
		traceSites = prevTrace
	})
	return d
}

// lockSite - a site carrying a self-payment.
//
// Sender and recipient are the same account so that UpdateBalanceIfValid takes
// its early return. The branch that credits a recipient reads
// config.GetConfig().Host.Verbose, and the configuration is a file-backed
// process global with no test seam, so it is nil in a test binary and that
// branch panics on it. The balance arithmetic is covered elsewhere; what these
// tests need is the lock discipline of the insert path.
func lockSite(n int) *Node {
	s := paymentSite(n, 0xaa, 0xaa, 1)
	s.valid = true
	return s
}

// buildSites - the sites a test will insert, built before any goroutine starts.
//
// Built up front because NewDagNode increments Dag.prevMajor with no lock at
// all, so building sites concurrently the way the publisher does would report a
// race in a file this change does not own. That race is real and is in the
// report; it is not what these tests are measuring.
func buildSites(writers, each int) [][]*Node {
	out := make([][]*Node, writers)
	for w := range out {
		out[w] = make([]*Node, each)
		for i := range out[w] {
			out[w][i] = lockSite(w*each + i + 1)
		}
	}
	return out
}

// insertConcurrently - run AddTxDag from writers goroutines at once. Returns the
// sites that went in and the errors that came back.
func insertConcurrently(t *testing.T, d *Dag, sites [][]*Node) (added []*Node, failed []error) {
	t.Helper()
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for w := range sites {
		wg.Add(1)
		go func(batch []*Node) {
			defer wg.Done()
			for _, s := range batch {
				_, _, err := d.AddTxDag(s)
				mu.Lock()
				if err != nil {
					failed = append(failed, err)
				} else {
					added = append(added, s)
				}
				mu.Unlock()
			}
		}(sites[w])
	}
	wg.Wait()
	return added, failed
}

// readUntilStopped - a goroutine doing what the node's other goroutines do to
// the graph while it is being written. Returns a function that stops it and
// waits for it.
func readUntilStopped(d *Dag, read func()) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			read()
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

// assertGraphIsConsistent - every part of the graph agrees with every other.
//
// Written out in full rather than checking a count, because a lock-scope mistake
// does not usually lose a site: it leaves a site in one structure and not
// another, or an edge recorded in one direction only.
func assertGraphIsConsistent(t *testing.T, d *Dag) {
	t.Helper()
	d.mux.Lock()
	defer d.mux.Unlock()

	inSlice := make(map[uuid.UUID]*Node, len(d._dag_))
	for _, n := range d._dag_ {
		if n == nil {
			t.Fatal("the live graph holds a nil site")
		}
		if _, dup := inSlice[n.id.id]; dup {
			t.Fatalf("site %s is in the live graph twice", n.id.id.String())
		}
		inSlice[n.id.id] = n
	}

	d.mu_map.RLock()
	mapped := make(map[uuid.UUID]*Node, len(d.mapped_vertices))
	for id, n := range d.mapped_vertices {
		mapped[id] = n
	}
	edges := make(map[uuid.UUID][]uuid.UUID, len(d.mapped_edges))
	for id, ids := range d.mapped_edges {
		edges[id] = append([]uuid.UUID(nil), ids...)
	}
	d.mu_map.RUnlock()

	for id, n := range inSlice {
		m, ok := mapped[id]
		if !ok {
			t.Fatalf("site %s is in the live graph but not in the lookup map, so nothing can find it", id.String())
		}
		if m != n {
			t.Fatalf("site %s is a different object in the lookup map than in the live graph", id.String())
		}
	}
	for id := range mapped {
		if _, ok := inSlice[id]; !ok {
			t.Fatalf("site %s is in the lookup map but not in the live graph", id.String())
		}
	}
	// An id in mapped_edges that the lookup map cannot resolve is a nil
	// dereference waiting in Dag.visit.
	for source, targets := range edges {
		if _, ok := mapped[source]; !ok {
			t.Fatalf("mapped_edges names source site %s, which is not in the graph", source.String())
		}
		for _, target := range targets {
			if _, ok := mapped[target]; !ok {
				t.Fatalf("site %s records an approval of %s, which is not in the graph", source.String(), target.String())
			}
		}
	}

	if added := d.sitesAdded.Load(); added != uint64(len(d._dag_)) {
		t.Fatalf("the site counter says %d sites have been added but the live graph holds %d, and nothing was sliced in this test",
			added, len(d._dag_))
	}

	// Edges: recorded in the link list, on the approving site and on the
	// approved site, and all three have to say the same thing.
	wantEdges := 0
	for _, n := range inSlice {
		if len(n.targets) > int(dagConfig.Approvetx) {
			t.Fatalf("site %s approves %d sites, more than dag.approvetx=%d",
				n.id.id.String(), len(n.targets), dagConfig.Approvetx)
		}
		wantEdges += len(n.targets)
		for _, target := range n.targets {
			if target == nil {
				t.Fatalf("site %s holds a nil approval target", n.id.id.String())
			}
			found := false
			for _, source := range target.sources {
				if source == n {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("site %s approves %s, but %s does not record it as an approver, so a walk cannot get back up",
					n.id.id.String(), target.id.id.String(), target.id.id.String())
			}
		}
	}
	if len(d._links_) != wantEdges {
		t.Fatalf("the link list holds %d edges but the sites hold %d between them", len(d._links_), wantEdges)
	}
	for _, l := range d._links_ {
		if l.source == nil || l.target == nil {
			t.Fatal("the link list holds a half-formed edge")
		}
		if _, ok := inSlice[l.source.id.id]; !ok {
			t.Fatalf("the link list holds an edge from %s, which is not in the graph", l.source.id.id.String())
		}
		if _, ok := inSlice[l.target.id.id]; !ok {
			t.Fatalf("the link list holds an edge to %s, which is not in the graph", l.target.id.id.String())
		}
	}
}

// assertEveryClaimVerifies - each of these sites carries a processor claim that
// checks out.
//
// The claim covers the site's approval-target ids, so this is what says the
// signature was made in the right place in the sequence: after the approvals
// were linked, and before anything could change them.
func assertEveryClaimVerifies(t *testing.T, sites []*Node) {
	t.Helper()
	for _, s := range sites {
		if err := verifyProcessor(s); err != nil {
			t.Fatalf("site %s carries a claim that does not verify, so it was signed over a different approval set: %s",
				s.id.id.String(), err.Error())
		}
	}
}

// Tip selection reads the same site edge lists that linking writes, so an insert
// and a selection running at once is the pairing dag.mux exists for. Selection
// now runs under its own acquisition, so this is the test that says moving it
// there did not move it out from under the lock.
func TestConcurrentInsertsDoNotRaceWithTipSelection(t *testing.T) {
	d := lockFixture(t, 1)
	sites := buildSites(8, 30)

	// A payment to select against, reused: approvalTargets reads only its
	// transaction type and never puts it in the graph.
	probe := lockSite(-1)
	stop := readUntilStopped(d, func() {
		if _, _, err := d.approvalTargets(probe); err != nil {
			// Nothing settles anything in this test, so the graph always has a
			// tip to offer.
			t.Errorf("selection found nothing to approve while the graph was growing: %s", err.Error())
		}
	})

	added, failed := insertConcurrently(t, d, sites)
	stop()

	for _, err := range failed {
		t.Errorf("an insert failed with no commit transaction in sight: %s", err.Error())
	}
	if len(added) != 8*30 {
		t.Fatalf("%d of %d sites went into the graph", len(added), 8*30)
	}
	assertGraphIsConsistent(t, d)
	assertEveryClaimVerifies(t, added)
}

// The size gauges, the throughput readings and the tip and confirmed-site
// queries are what an operator and the commit builder ask the graph while it is
// being written. Each takes dag.mux; refreshSizeGauges deliberately does not
// wait for it, which is a case of its own.
func TestConcurrentInsertsDoNotRaceWithGaugesAndQueries(t *testing.T) {
	d := lockFixture(t, 1)
	sites := buildSites(6, 30)

	stop := readUntilStopped(d, func() {
		_ = d.Size()
		_ = d.Tps()
		_ = d.AvgDelay()
		_ = d.GetTips()
		_ = d.SnapshotNodes()
		_ = d.PeekConfirmedSites()
		refreshSizeGauges()
	})

	added, failed := insertConcurrently(t, d, sites)
	stop()

	for _, err := range failed {
		t.Errorf("an insert failed with no commit transaction in sight: %s", err.Error())
	}
	assertGraphIsConsistent(t, d)
	assertEveryClaimVerifies(t, added)
}

// The commit builder drains the confirmed set and then serialises each site with
// ToPbNode, holding no dag lock while it does. ToPbNode reads the three
// processor-claim fields, which AddTxDag writes with the lock released - so this
// is the pairing that decides whether the signature may happen before or after
// the site is published. It has to be before.
func TestConcurrentInsertsDoNotRaceWithConfirmedSiteHarvesting(t *testing.T) {
	d := lockFixture(t, 1)
	sites := buildSites(6, 40)

	harvested := int64(0)
	stop := readUntilStopped(d, func() {
		for _, s := range d.GetConfirmedSites() {
			// Exactly what unsafe_buildPin does with a confirmed site.
			_ = s.ToPbNode()
			atomic.AddInt64(&harvested, 1)
		}
	})

	added, failed := insertConcurrently(t, d, sites)
	stop()

	for _, err := range failed {
		t.Errorf("an insert failed with no commit transaction in sight: %s", err.Error())
	}
	assertEveryClaimVerifies(t, added)
	if atomic.LoadInt64(&harvested) == 0 {
		t.Fatal("nothing was confirmed while 240 sites were inserted, so the harvesting path was never exercised")
	}
}

// Slicing is the other writer: it rewrites every surviving site's edge lists and
// deletes from both lookup maps. An insert that chose a tip which is settled
// before the approval is made must not link to it, and one whose approval target
// is settled while the site is being signed must record the approval by id.
func TestConcurrentInsertsDoNotRaceWithSlicing(t *testing.T) {
	d := lockFixture(t, 1)
	sites := buildSites(6, 40)

	pinNumber := int64(0)
	sliced := int64(0)
	stop := readUntilStopped(d, func() {
		confirmed := d.GetConfirmedSites()
		if len(confirmed) == 0 {
			return
		}
		pinNumber++
		sliceAppliedPin(storedPin(pinNumber, nil, nil, confirmed...))
		atomic.AddInt64(&sliced, int64(len(confirmed)))
	})

	added, failed := insertConcurrently(t, d, sites)
	stop()

	// An insert may legitimately fail here: every site it chose to approve can
	// be settled in the window between choosing and linking. That is the case
	// linkNewSite exists to catch, and refusing the transaction so the publisher
	// can retry it is the correct outcome - what must not happen is a site
	// linked to something the graph has stopped holding.
	for _, err := range failed {
		t.Logf("insert refused, which is allowed while sites are being settled: %s", err.Error())
	}
	if atomic.LoadInt64(&sliced) == 0 {
		t.Fatal("nothing was settled while 240 sites were inserted, so the slicing path was never exercised")
	}
	assertEveryClaimVerifies(t, added)

	// Whatever survived slicing has to be coherent: no edge may point at a site
	// the slice took out, in either direction, and no lookup entry may name one.
	d.mux.Lock()
	defer d.mux.Unlock()
	live := make(map[uuid.UUID]struct{}, len(d._dag_))
	for _, n := range d._dag_ {
		live[n.id.id] = struct{}{}
	}
	for _, n := range d._dag_ {
		for _, target := range n.targets {
			if _, ok := live[target.id.id]; !ok {
				t.Fatalf("site %s still points at %s, which was settled and taken out of the graph",
					n.id.id.String(), target.id.id.String())
			}
		}
		for _, source := range n.sources {
			if _, ok := live[source.id.id]; !ok {
				t.Fatalf("site %s is still recorded as approved by %s, which was settled and taken out of the graph",
					n.id.id.String(), source.id.id.String())
			}
		}
	}
	for _, l := range d._links_ {
		if _, ok := live[l.source.id.id]; !ok {
			t.Fatalf("the link list holds an edge from settled site %s", l.source.id.id.String())
		}
		if _, ok := live[l.target.id.id]; !ok {
			t.Fatalf("the link list holds an edge to settled site %s", l.target.id.id.String())
		}
	}
}

// The window AddTxDag opens between linking a site's approvals and publishing
// it, driven directly rather than waited for: a commit transaction settles the
// approval target while the site is out of the graph being signed.
//
// sliceSites cannot reach the site - it is not in dag._dag_ yet - so
// publishNewSite has to do what sliceSites would have done, or the site holds an
// archived site alive through a pointer nothing will ever prune and its entry in
// mapped_edges names a site that Vertex() cannot resolve.
func TestAnApprovalTargetSettledWhileSigningIsRecordedById(t *testing.T) {
	d := lockFixture(t, 1)
	target := d.getGenesis()
	site := lockSite(1)

	// linkNewSite, as far as the approvals.
	d.mux.Lock()
	edges, _, err := d.linkApprovals(site, []*Node{target})
	d.mux.Unlock()
	if err != nil {
		t.Fatalf("linking the approval: %s", err.Error())
	}
	if len(site.targets) != 1 {
		t.Fatalf("the site approves %d sites, expected 1", len(site.targets))
	}

	// A commit transaction lands while the signature is being made.
	sliceAppliedPin(storedPin(1, nil, nil, target))
	if _, settled := settledSite(target.id.id); !settled {
		t.Fatal("the target was not settled, so the test proves nothing")
	}

	d.publishNewSite(site, &linkedSite{edges: edges})

	if len(site.targets) != 0 {
		t.Fatalf("the site still holds %d pointer(s) to a settled approval target, which nothing will ever prune",
			len(site.targets))
	}
	if len(site.slicedTargets) != 1 || site.slicedTargets[0] != target.id.id {
		t.Fatalf("the settled approval was not recorded by id: slicedTargets=%v", site.slicedTargets)
	}
	// The approval still has to reach a peer, or the peer rebuilds a different
	// site and the processor claim over it stops verifying.
	if pbn := site.ToPbNode(); !pbn.MissingTargets[target.id.id.String()] {
		t.Fatal("the settled approval is not reported on the wire, so a peer cannot rebuild this site's approval set")
	}
	d.mu_map.RLock()
	recorded := d.mapped_edges[site.id.id]
	d.mu_map.RUnlock()
	for _, id := range recorded {
		if d.getById(id, true) == nil {
			t.Fatalf("mapped_edges records an approval of %s, which is not in the graph", id.String())
		}
	}
}

// BenchmarkConcurrentInserts - the measurement the lock work is for: how much
// insert throughput the node gets out of more than one goroutine.
//
// The interesting number is not ns/op on its own but how it moves with -cpu:
// with the whole of an insert under one acquisition it barely moves at all,
// because the section is serialised and the ed25519 signature is nine tenths of
// it.
//
// Building the site is inside the timed loop, because b.N is not known up front
// and holding b.N sites live distorts a benchmark of this length. It costs a
// uuid and a transaction, which is small against the insert and identical either
// side of this change.
func BenchmarkConcurrentInserts(b *testing.B) {
	d := lockFixture(b, 1)
	// Grow a frontier first, so selection is walking a region rather than
	// fanning out from genesis.
	for i := 1; i <= 200; i++ {
		if _, _, err := d.AddTxDag(lockSite(i)); err != nil {
			b.Fatalf("growing the frontier: %s", err.Error())
		}
	}

	var next atomic.Int64
	next.Store(1 << 20)
	var failures atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			// b.Fatal is not usable from a RunParallel body, so failures are
			// counted and reported once the parallel section is over.
			if _, _, err := d.AddTxDag(lockSite(int(next.Add(1)))); err != nil {
				failures.Add(1)
			}
		}
	})
	b.StopTimer()
	if n := failures.Load(); n > 0 {
		b.Fatalf("%d insert(s) failed", n)
	}
}

// BenchmarkSiteAttributionSignature - the cost that decides the shape of
// AddTxDag. It is why the site is signed with dag.mux released, at the price of
// a third lock acquisition and the window publishNewSite has to close.
func BenchmarkSiteAttributionSignature(b *testing.B) {
	w := grape_wallet.NewWallet()
	site := lockSite(1)
	tlink(site, lockSite(2))
	tlink(site, lockSite(3))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := signProcessor(site, w); err != nil {
			b.Fatalf("signing: %s", err.Error())
		}
	}
}

/*
The attribution invariant, and the two paths that could break it.

Dag.checkProcessorClaim verifies a received site's processor claim after the
exclusive section has been released, which means a commit transaction or the
missing-target reconciler can run between the site being linked and the claim
being checked. What makes that safe is that neither of them changes the set of
approval ids the claim is signed over: slicing moves an id from targets to
slicedTargets, relinking moves one from missingTargets to targets, and
approvalTargetIDs unions all three.

If that were wrong the failure would be silent and monetary - an honest processor
stripped of a fee it earned, or a forged claim left standing - so it is checked
three ways: on the id set directly, and on a real signature either side of each
of the two movers.
*/

// remoteSigner - the wallet a peer signs its own sites with. Any wallet will do:
// verifyProcessor requires the claimed address to be the one the presented key
// produces, and nothing more, because a processor is entitled to claim its own
// work and no authority list is involved.
var remoteSigner = grape_wallet.NewWallet()

// receivedSite - a site as it arrives from a peer: built and signed elsewhere
// over the approvals it made there, then handed over with its edges stripped, so
// that this node resolves and links them itself.
//
// That is the shape the invariant has to survive. The builder signed over the ids
// of the sites it approved; the receiver reaches the same id set by a different
// route, and must keep reaching it after a slice or a relink.
func receivedSite(t testing.TB, n int, targets []*Node) (*Node, []tx.UuidSlice) {
	t.Helper()
	site := lockSite(n)
	// Sign as the builder did, over its own approval set...
	site.targets = append([]*Node(nil), targets...)
	if err := signProcessor(site, remoteSigner); err != nil {
		t.Fatalf("signing the site as its builder would: %s", err.Error())
	}
	// ...then hand it over the way the wire does, with no edges.
	site.targets = nil
	ids := make([]tx.UuidSlice, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, tx.UuidSlice{Id: target.id.id})
	}
	return site, ids
}

func approvalIdSet(n *Node) map[uuid.UUID]struct{} {
	out := map[uuid.UUID]struct{}{}
	for _, id := range approvalTargetIDs(n) {
		out[id] = struct{}{}
	}
	return out
}

func sameIdSet(a, b map[uuid.UUID]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if _, ok := b[id]; !ok {
			return false
		}
	}
	return true
}

// The set of ids a processor claim is signed over does not change when a site's
// approval target is settled, or when a target it was missing turns up. Both
// move an id between the three fields approvalTargetIDs unions; neither adds one
// and neither drops one.
func TestTheApprovalIdSetIsInvariantUnderSlicingAndRelinking(t *testing.T) {
	d := lockFixture(t, 1)

	settledTarget := lockSite(1)
	liveTarget := lockSite(2)
	relinkedTarget := lockSite(3)
	for _, target := range []*Node{settledTarget, liveTarget, relinkedTarget} {
		d.mux.Lock()
		if _, _, err := d.linkApprovals(target, nil); err != nil {
			t.Fatalf("seeding a target: %s", err.Error())
		}
		d.publishSite(target, nil, nil)
		d.mux.Unlock()
	}

	// A site with an id in each of the three fields at once, which is the state
	// the union has to cope with.
	site := lockSite(4)
	d.mux.Lock()
	if _, _, err := d.linkApprovals(site, []*Node{settledTarget, liveTarget}); err != nil {
		t.Fatalf("linking: %s", err.Error())
	}
	site.missingTargets = map[string]bool{relinkedTarget.id.id.String(): true}
	d.publishSite(site, nil, nil)
	d.mux.Unlock()

	want := approvalIdSet(site)
	if len(want) != 3 {
		t.Fatalf("the site should approve 3 sites, approvalTargetIDs says %d", len(want))
	}

	// Mover one: a commit transaction settles one of the linked targets.
	sliceAppliedPin(storedPin(1, nil, nil, settledTarget))
	if got := approvalIdSet(site); !sameIdSet(want, got) {
		t.Fatalf("settling an approval target changed the signed id set: %d ids before, %d after", len(want), len(got))
	}
	if len(site.slicedTargets) == 0 {
		t.Fatal("the settled target's id was not recorded, so it left the union rather than moving within it")
	}

	// Mover two: the missing target turns up and is relinked.
	d.ReconcileMissingTargets()
	if len(site.missingTargets) != 0 {
		t.Fatalf("the target was not relinked, so this half of the test proves nothing: %v", site.missingTargets)
	}
	if got := approvalIdSet(site); !sameIdSet(want, got) {
		t.Fatalf("relinking a missing approval target changed the signed id set: %d ids before, %d after", len(want), len(got))
	}
}

// The same invariant, through a real signature: a claim made over a site's
// approvals still verifies after one of those approvals has been settled. This is
// the case that decides whether checkProcessorClaim may run outside the exclusive
// section, because a slice can land in exactly that window.
func TestAClaimStillVerifiesAfterItsApprovalTargetIsSettled(t *testing.T) {
	d := lockFixture(t, 1)

	target := lockSite(1)
	d.mux.Lock()
	if _, _, err := d.linkApprovals(target, nil); err != nil {
		t.Fatalf("seeding the target: %s", err.Error())
	}
	d.publishSite(target, nil, nil)
	d.mux.Unlock()

	site, ids := receivedSite(t, 2, []*Node{target})
	if err := d.InsertTxDag(site, site.id.id, site.id.idMajor, site.id.idMinor, ids...); err != nil {
		t.Fatalf("inserting the received site: %s", err.Error())
	}
	if err := verifyProcessor(site); err != nil {
		t.Fatalf("the claim did not verify even before anything moved: %s", err.Error())
	}

	sliceAppliedPin(storedPin(1, nil, nil, target))
	if _, settled := settledSite(target.id.id); !settled {
		t.Fatal("the target was not settled, so the test proves nothing")
	}

	if err := verifyProcessor(site); err != nil {
		t.Fatalf("settling an approval target broke an honest claim, so checkProcessorClaim would strip a processor of a fee it earned: %s", err.Error())
	}
	if len(site.processorSig) == 0 {
		t.Fatal("the claim was stripped")
	}
}

// The other mover: a claim over approvals this node had not yet seen still
// verifies once they turn up and are relinked. Without this, every site inserted
// while the node was catching up would lose its processor's fee the moment the
// reconciler caught up.
func TestAClaimStillVerifiesAfterAMissingApprovalTargetIsRelinked(t *testing.T) {
	d := lockFixture(t, 1)

	// Signed over a target this node does not hold yet, so the id lands in
	// missingTargets rather than in targets.
	absent := lockSite(1)
	site, ids := receivedSite(t, 2, []*Node{absent})
	err := d.InsertTxDag(site, site.id.id, site.id.idMajor, site.id.idMinor, ids...)
	if err == nil {
		t.Fatal("expected the insert to report the missing approval target")
	}
	if d.getById(site.id.id, true) == nil {
		t.Fatal("the site was not added, so there is nothing to relink")
	}
	if len(site.missingTargets) != 1 {
		t.Fatalf("expected one missing approval target, got %v", site.missingTargets)
	}
	if err := verifyProcessor(site); err != nil {
		t.Fatalf("a claim over an approval this node has not resolved yet must still verify: %s", err.Error())
	}

	// The absent target turns up.
	d.mux.Lock()
	if _, _, err := d.linkApprovals(absent, nil); err != nil {
		t.Fatalf("seeding the late target: %s", err.Error())
	}
	d.publishSite(absent, nil, nil)
	d.mux.Unlock()
	d.ReconcileMissingTargets()
	if len(site.missingTargets) != 0 {
		t.Fatalf("the target was not relinked, so the test proves nothing: %v", site.missingTargets)
	}

	if err := verifyProcessor(site); err != nil {
		t.Fatalf("relinking broke an honest claim: %s", err.Error())
	}
}

// checkProcessorClaim keeps a claim that holds up, strips one that does not, and
// leaves a site with no claim at all alone. Driven directly, because the three
// outcomes are what the whole exercise is protecting.
func TestAnUnusableClaimIsStrippedAndAnHonestOneIsKept(t *testing.T) {
	d := lockFixture(t, 1)

	target := lockSite(1)
	d.mux.Lock()
	if _, _, err := d.linkApprovals(target, nil); err != nil {
		t.Fatalf("seeding the target: %s", err.Error())
	}
	d.publishSite(target, nil, nil)
	d.mux.Unlock()

	insert := func(n int, tamper func(*Node)) *Node {
		site, ids := receivedSite(t, n, []*Node{target})
		tamper(site)
		if err := d.InsertTxDag(site, site.id.id, site.id.idMajor, site.id.idMinor, ids...); err != nil {
			t.Fatalf("inserting: %s", err.Error())
		}
		return site
	}

	honest := insert(2, func(*Node) {})
	if len(honest.processorSig) == 0 {
		t.Fatal("an honest claim was stripped, so this node will not pay a processor for work it did")
	}

	forged := insert(3, func(s *Node) { s.processorSig[0] ^= 0xff })
	if len(forged.processorSig) != 0 {
		t.Fatal("a claim whose signature does not check out was kept, so a liar gets paid")
	}
	if d.getById(forged.id.id, true) == nil {
		t.Fatal("the site itself was refused; a bad claim must not deny the network a transaction")
	}

	// A claim naming somebody else's address, self-consistently signed. The
	// signature verifies against the presented key, so only the address check
	// catches it - and it is the check that stops a node pinning its work on an
	// innocent third party.
	swapped := insert(4, func(s *Node) {
		s.processorAddress = grape_wallet.AddressToBytes(grape_wallet.NewWallet().WalletAddress())
	})
	if len(swapped.processorSig) != 0 {
		t.Fatal("a claim whose address is not the one its key produces was kept")
	}

	unattributed := insert(5, clearProcessor)
	if len(unattributed.processorSig) != 0 || len(unattributed.processorAddress) != 0 {
		t.Fatal("a site that claims nothing came back with a claim")
	}
	if d.getById(unattributed.id.id, true) == nil {
		t.Fatal("a site from a peer predating attribution was refused")
	}
}

// The subscriber's path under concurrency: received sites arriving while the
// graph is read, sliced and harvested. Claim checking now runs under the shared
// lock, so this is what says the shared acquisition is genuinely a read.
func TestConcurrentReceivedInsertsDoNotRaceWithClaimChecking(t *testing.T) {
	d := lockFixture(t, 1)

	target := lockSite(1)
	d.mux.Lock()
	if _, _, err := d.linkApprovals(target, nil); err != nil {
		t.Fatalf("seeding the target: %s", err.Error())
	}
	d.publishSite(target, nil, nil)
	d.mux.Unlock()

	const writers, perWriter = 6, 25
	batches := make([][]*Node, writers)
	batchIds := make([][][]tx.UuidSlice, writers)
	for w := 0; w < writers; w++ {
		for i := 0; i < perWriter; i++ {
			site, ids := receivedSite(t, w*perWriter+i+10, []*Node{target})
			// Every third claim is forged, so the write-locked stripping branch
			// runs alongside everything else rather than only the read path.
			if i%3 == 0 {
				site.processorSig[0] ^= 0xff
			}
			batches[w] = append(batches[w], site)
			batchIds[w] = append(batchIds[w], ids)
		}
	}

	probe := lockSite(-1)
	stop := readUntilStopped(d, func() {
		_ = d.Size()
		_ = d.GetTips()
		_ = d.SnapshotNodes()
		for _, s := range d.GetConfirmedSites() {
			_ = s.ToPbNode()
		}
		if _, _, err := d.approvalTargets(probe); err != nil {
			t.Errorf("selection found nothing to approve: %s", err.Error())
		}
		refreshSizeGauges()
	})

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i, site := range batches[w] {
				if err := d.InsertTxDag(site, site.id.id, site.id.idMajor, site.id.idMinor, batchIds[w][i]...); err != nil {
					t.Errorf("inserting a received site: %s", err.Error())
				}
			}
		}(w)
	}
	wg.Wait()
	stop()

	// Every claim reached the outcome it deserved, and no site was refused.
	for w := 0; w < writers; w++ {
		for i, site := range batches[w] {
			if d.getById(site.id.id, true) == nil {
				t.Fatalf("received site %s never entered the graph", site.id.id.String())
			}
			forged := i%3 == 0
			stripped := len(site.processorSig) == 0
			if forged && !stripped {
				t.Fatalf("forged claim on site %s survived", site.id.id.String())
			}
			if !forged && stripped {
				t.Fatalf("honest claim on site %s was stripped", site.id.id.String())
			}
		}
	}
	assertGraphIsConsistent(t, d)
}

// A site that collides on version enters the graph, pays, counts and confirms.
//
// With dag.versioncollision on, the branch that used to handle this refused the
// site outright: nextToNode was only ever assigned inside a guard that its id
// differed from the incoming site's, and the branch then returned an error
// whenever those ids differed, so it always did. The site never entered the
// graph, its payment never moved this node's balances, and every peer kept it.
func TestAVersionCollidingReceivedSiteStillEntersTheGraph(t *testing.T) {
	d := lockFixture(t, 1)
	prev := dagConfig.Versioncollision
	dagConfig.Versioncollision = true
	t.Cleanup(func() { dagConfig.Versioncollision = prev })

	genesis := d.getGenesis()

	// An existing approver of genesis, at major version 7.
	approver, approverIds := receivedSite(t, 7, []*Node{genesis})
	if err := d.InsertTxDag(approver, approver.id.id, 7, 0, approverIds...); err != nil {
		t.Fatalf("inserting the first approver: %s", err.Error())
	}

	// A second site at the same major version and no higher minor, approving the
	// same target: exactly the collision the removed branch triggered on.
	colliding, collidingIds := receivedSite(t, 7, []*Node{genesis})
	before := d.sitesAdded.Load()
	if err := d.InsertTxDag(colliding, colliding.id.id, 7, 0, collidingIds...); err != nil {
		t.Fatalf("a version-colliding site was refused: %s", err.Error())
	}

	if d.getById(colliding.id.id, true) == nil {
		t.Fatal("the colliding site is not in the lookup map, so nothing can find it")
	}
	if got := d.sitesAdded.Load(); got != before+1 {
		t.Fatalf("the site counter went from %d to %d; a site that is in the graph but uncounted moves the width gate", before, got)
	}
	if len(colliding.targets) != 1 || colliding.targets[0] != genesis {
		t.Fatalf("the colliding site approves %d sites; it must approve the one it named", len(colliding.targets))
	}
	if !confirmationCounter.isTip(colliding.id.id) {
		t.Fatal("the colliding site is not known to the confirmation tracker, so it can never be confirmed or settled")
	}
	if len(colliding.processorSig) == 0 {
		t.Fatal("the colliding site's honest claim was stripped")
	}
	assertGraphIsConsistent(t, d)
}

// BenchmarkConcurrentReceivedInserts - the subscriber path, which is the busier
// of the two: this node receives every transaction the network publishes.
//
// The sites are built and signed before the timer starts, because building one
// costs a signature of its own and that is the cost being moved off the lock, not
// the cost being measured. They all approve genesis, so the shape of the graph
// stays fixed and what varies is the locking.
func BenchmarkConcurrentReceivedInserts(b *testing.B) {
	d := lockFixture(b, 1)
	genesis := d.getGenesis()

	sites := make([]*Node, b.N)
	ids := make([][]tx.UuidSlice, b.N)
	for i := 0; i < b.N; i++ {
		sites[i], ids[i] = receivedSite(b, i+10, []*Node{genesis})
	}

	var next atomic.Int64
	next.Store(-1)
	var failures atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			i := int(next.Add(1))
			if i >= len(sites) {
				return
			}
			s := sites[i]
			if err := d.InsertTxDag(s, s.id.id, s.id.idMajor, s.id.idMinor, ids[i]...); err != nil {
				failures.Add(1)
			}
		}
	})
	b.StopTimer()
	if n := failures.Load(); n > 0 {
		b.Fatalf("%d received insert(s) failed", n)
	}
}

// BenchmarkSiteAttributionVerification - the cost checkProcessorClaim takes off
// the exclusive lock, once per received site. Its twin,
// BenchmarkSiteAttributionSignature, is the cost AddTxDag takes off it once per
// site this node builds.
func BenchmarkSiteAttributionVerification(b *testing.B) {
	site := lockSite(1)
	tlink(site, lockSite(2))
	tlink(site, lockSite(3))
	if err := signProcessor(site, remoteSigner); err != nil {
		b.Fatalf("signing: %s", err.Error())
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := verifyProcessor(site); err != nil {
			b.Fatalf("verifying: %s", err.Error())
		}
	}
}

// BenchmarkReceivedInsertsWithAConcurrentPublisher - the subscriber's path with
// somebody else using the graph at the same time, which is the only arrangement
// where moving the claim check to the shared lock can pay.
//
// BenchmarkConcurrentReceivedInserts, above, deliberately does not show a gain:
// with nothing but subscribers running, a shared section that lasts as long as
// the exclusive one it replaced blocks a waiting writer for exactly as long. What
// changes is that a verification no longer excludes the publisher's tip
// selection, the size gauges or the commit builder's harvest.
//
// So the timed loop is the subscriber and the reported metric is the other
// participant: how much the publisher got done per received site. Both sides
// scale with whatever CPU the machine has spare, so the ratio survives a loaded
// box in a way that ns/op does not.
//
// The received sites form a chain - each approves the one before it - so that the
// tip set stays small and what is being measured is the locking rather than the
// growth of the confirmation frontier.
func BenchmarkReceivedInsertsWithAConcurrentPublisher(b *testing.B) {
	d := lockFixture(b, 1)

	sites := make([]*Node, b.N)
	ids := make([][]tx.UuidSlice, b.N)
	prev := d.getGenesis()
	for i := 0; i < b.N; i++ {
		sites[i], ids[i] = receivedSite(b, i+10, []*Node{prev})
		prev = sites[i]
	}

	var published, refused atomic.Int64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		n := 1 << 20
		for {
			select {
			case <-stop:
				return
			default:
			}
			n++
			if _, _, err := d.AddTxDag(lockSite(n)); err != nil {
				refused.Add(1)
				continue
			}
			published.Add(1)
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := sites[i]
		if err := d.InsertTxDag(s, s.id.id, s.id.idMajor, s.id.idMinor, ids[i]...); err != nil {
			b.StopTimer()
			close(stop)
			<-done
			b.Fatalf("received insert %d failed: %s", i, err.Error())
		}
	}
	b.StopTimer()
	close(stop)
	<-done

	b.ReportMetric(float64(published.Load())/float64(b.N), "published/received")
	if refused.Load() > 0 {
		b.Fatalf("%d publish(es) were refused", refused.Load())
	}
}

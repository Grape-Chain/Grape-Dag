package dag

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/config"
	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
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

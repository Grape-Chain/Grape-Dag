package dag

import (
	"testing"

	"math/big"

	"github.com/Grape-Chain/Grape-Dag/smc"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/vm"
	"time"

	"github.com/google/uuid"
)

// The report a validator broadcasts must not consume what it reports. A round
// can fail, and can be repeated; a report that harvested its own sites would
// leave the node with nothing to settle and strand the sites it had already
// taken.

// chainOf - a straight chain of n sites, each approving the one before, wired
// into a tracker. Returns the tracker and the sites in order.
func chainOf(t *testing.T, n int, share uint16) (*ConfirmTracker, []*Node) {
	t.Helper()
	tr := newConfirmTracker(0, share)
	nodes := make([]*Node, 0, n)
	for i := 0; i < n; i++ {
		v := tnode(i)
		if i > 0 {
			tlink(v, nodes[i-1])
		}
		tr.add(v)
		nodes = append(nodes, v)
	}
	return tr, nodes
}

func idsOf(nodes []*Node) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(nodes))
	for _, n := range nodes {
		if n != nil {
			out = append(out, n.id.id)
		}
	}
	return out
}

func TestPeekDoesNotConsumeWhatItReports(t *testing.T) {
	tr, _ := chainOf(t, 8, 667)

	first := tr.peek()
	if len(first) == 0 {
		t.Fatal("nothing confirmed to report, so the test proves nothing")
	}
	second := tr.peek()
	if len(second) != len(first) {
		t.Fatalf("a second report saw %d sites, the first saw %d: reporting consumed the set",
			len(second), len(first))
	}
	// And what was reported is still there to be settled.
	took := tr.take(idsOf(first))
	if len(took) != len(first) {
		t.Fatalf("reported %d sites but could only settle %d", len(first), len(took))
	}
}

func TestTakeConsumesOnlyTheAgreedSites(t *testing.T) {
	tr, _ := chainOf(t, 10, 667)

	confirmed := tr.peek()
	if len(confirmed) < 3 {
		t.Fatalf("need at least three confirmed sites to split the set, got %d", len(confirmed))
	}
	agreed := confirmed[:2]
	rest := confirmed[2:]

	took := tr.take(idsOf(agreed))
	if len(took) != len(agreed) {
		t.Fatalf("settled %d of the %d agreed sites", len(took), len(agreed))
	}

	// The sites the round did not agree on are still confirmed, waiting for the
	// next commit transaction rather than harvested by one that does not name
	// them.
	left := tr.peek()
	if len(left) != len(rest) {
		t.Fatalf("expected %d sites left for the next commit transaction, got %d", len(rest), len(left))
	}
	settled := map[uuid.UUID]bool{}
	for _, n := range took {
		settled[n.id.id] = true
	}
	for _, n := range left {
		if settled[n.id.id] {
			t.Fatalf("site %s was both settled and left behind", n.id.id.String())
		}
	}
}

func TestTakeIsIdempotent(t *testing.T) {
	tr, _ := chainOf(t, 8, 667)
	confirmed := idsOf(tr.peek())
	if len(confirmed) == 0 {
		t.Fatal("nothing confirmed, so the test proves nothing")
	}

	if got := len(tr.take(confirmed)); got != len(confirmed) {
		t.Fatalf("settled %d of %d sites", got, len(confirmed))
	}
	// Slicing harvests the same sites on the way through pinCommitted, so this
	// runs twice on every commit. The second time has to be a no-op, not a
	// second settlement of the same sites.
	if got := len(tr.take(confirmed)); got != 0 {
		t.Fatalf("settling the same sites twice returned %d site(s) the second time", got)
	}
}

// Settling is what pays a site's processor and discards it from the graph, so
// settling one twice is the failure the guard exists for. Slicing harvests the
// same sites on its way through pinCommitted, so this guard is reached with a
// site that is harvested and still queued - however it came to be both.
func TestTakeRefusesASiteAlreadyHarvested(t *testing.T) {
	tr, _ := chainOf(t, 8, 667)
	confirmed := tr.peek()
	if len(confirmed) == 0 {
		t.Fatal("nothing confirmed, so the test proves nothing")
	}
	victim := confirmed[0]

	// Harvested by another path - slicing - while still queued here.
	tr.mu.Lock()
	tr.harvested[victim.id.id] = struct{}{}
	tr.mu.Unlock()

	if got := tr.take([]uuid.UUID{victim.id.id}); len(got) != 0 {
		t.Fatalf("settled %d site(s) that had already been harvested", len(got))
	}
}

// The legacy counter implements the same split, because dag.confirmations
// chooses between them at run time and the protocol must not depend on which
// one is in use.
func TestLegacyCounterPeekDoesNotConsume(t *testing.T) {
	c := newConfirmationCounter()
	// Three approvers, because that is what this counter calls confirmed.
	parent := tnode(0)
	c.add(parent)
	for i := 1; i <= 3; i++ {
		child := tnode(i)
		tlink(child, parent)
		c.add(child)
	}
	first := c.peek()
	if len(first) == 0 {
		t.Fatal("nothing confirmed to report, so the test proves nothing")
	}
	if len(c.peek()) != len(first) {
		t.Fatal("reporting consumed the set")
	}
	if got := len(c.take(idsOf(first))); got != len(first) {
		t.Fatalf("settled %d of the %d reported sites", got, len(first))
	}
	if got := len(c.peek()); got != 0 {
		t.Fatalf("%d site(s) still reported after being settled", got)
	}
}

func TestLegacyCounterTakeConsumesOnlyTheAgreedSites(t *testing.T) {
	c := newConfirmationCounter()
	// Two sites that this counter calls confirmed: three approvers each.
	parents := []*Node{tnode(0), tnode(100)}
	for _, parent := range parents {
		c.add(parent)
	}
	next := 1
	for _, parent := range parents {
		for i := 0; i < 3; i++ {
			child := tnode(next)
			next++
			tlink(child, parent)
			c.add(child)
		}
	}

	confirmed := c.peek()
	if len(confirmed) != 2 {
		t.Fatalf("expected both parents confirmed, got %d", len(confirmed))
	}
	agreed := confirmed[:1]

	took := c.take(idsOf(agreed))
	if len(took) != 1 {
		t.Fatalf("settled %d sites for an agreed set of 1", len(took))
	}
	if took[0].id.id != agreed[0].id.id {
		t.Fatal("settled a site the set did not agree on")
	}
	left := c.peek()
	if len(left) != 1 {
		t.Fatalf("expected 1 site left for the next commit transaction, got %d", len(left))
	}
	if left[0].id.id == agreed[0].id.id {
		t.Fatal("the settled site is still reported as confirmed")
	}

	// And a site that is known but not confirmed is not settleable at all. A
	// proposal naming one is refused by the protocol, but the ledger must not
	// depend on that: nothing unconfirmed leaves the graph.
	unconfirmed := tnode(999)
	c.add(unconfirmed)
	if got := c.take([]uuid.UUID{unconfirmed.id.id}); len(got) != 0 {
		t.Fatalf("settled %d site(s) that were never confirmed", len(got))
	}
}

// A commit transaction has to be buildable without being committed, or a
// validator could not propose one before the set has agreed to it.
func TestBuildingACommitTransactionDoesNotAppendIt(t *testing.T) {
	recoveryFixture(t, t.TempDir())
	vm.AttachInMemoryStateStore()
	prevWallet := dagWallet
	dagWallet = testPinWallet
	t.Cleanup(func() { dagWallet = prevWallet })
	p := _pins_

	first, err := p.unsafe_buildPin(nil, nil)
	if err != nil {
		t.Fatalf("building: %s", err.Error())
	}
	if first == nil {
		t.Fatal("built nothing")
	}
	if head := p.unsafe_getLastPin(); head != nil {
		t.Fatalf("building put commit transaction %d on the chain", head.PinNumber)
	}

	// Appending is what puts it on the chain, and only then does the next one
	// take the next number.
	p.unsafe_appendPin(first)
	if head := p.unsafe_getLastPin(); head != first {
		t.Fatal("appending did not put the built commit transaction on the chain")
	}
	second, err := p.unsafe_buildPin(nil, nil)
	if err != nil {
		t.Fatalf("building the second: %s", err.Error())
	}
	if second.PinNumber != first.PinNumber+1 {
		t.Fatalf("built commit transaction %d after %d", second.PinNumber, first.PinNumber)
	}
	if head := p.unsafe_getLastPin(); head != first {
		t.Fatalf("building the second moved the head to %d", head.PinNumber)
	}
}

// The tick has to be a fraction of the commit interval. A tick as long as the
// interval would notice every phase deadline a whole interval late, which is
// exactly one lost round per failure.
func TestConsensusTicksFasterThanTheCommitInterval(t *testing.T) {
	got := consensusTickInterval()
	if got <= 0 {
		t.Fatalf("tick interval is %s", got)
	}
	if got > time.Second {
		t.Fatalf("tick interval %s is too coarse for phases inside a commit interval", got)
	}
}

// Building a commit transaction has consequences before anyone has agreed to
// it: the smart-contract stage executes the transactions it includes and moves
// them out of the unconfirmed pool, and the wallet cache is invalidated for
// every account it touches. A proposer that loses its round has to be able to
// put all of that back, or it carries state no other node has.

func TestRollingBackALostRoundReturnsTheContractTransactions(t *testing.T) {
	vm.AttachInMemoryStateStore()
	prevCache := walletCache
	walletCache = newWalletCache()
	t.Cleanup(func() { walletCache = prevCache })

	pending := smcTxsInPool(t, 2)
	cand := &pinCandidate{epoch: 1, cache: newWalletCache(), smcTxs: pending}

	// The smart-contract stage would have moved them out of the pool; do
	// exactly that, then undo it.
	for _, tr := range pending {
		smc.AddConfirmed(tx.ConfirmedTx{IdentifiableTx: tx.IdentifiableTx{Transaction: tr}})
	}
	for _, tr := range pending {
		if _, ok := smc.FindConfirmed(txId(tr)); !ok {
			t.Fatal("the transactions were not settled, so the test proves nothing")
		}
	}

	cand.undo()

	for _, tr := range pending {
		id := txId(tr)
		if _, stillConfirmed := smc.FindConfirmed(id); stillConfirmed {
			t.Fatalf("transaction %s is still settled against a commit transaction that was never published", id)
		}
		if !inUnconfirmedPool(id) {
			t.Fatalf("transaction %s was not returned to the unconfirmed pool, so it can never reach a commit transaction", id)
		}
	}
}

func TestRollingBackALostRoundRestoresTheWalletCache(t *testing.T) {
	vm.AttachInMemoryStateStore()
	prevCache := walletCache
	walletCache = newWalletCache()
	t.Cleanup(func() { walletCache = prevCache })

	const wallet = "0x00000000000000000000000000000000000000aa"
	walletCache.setBalance(wallet, big.NewInt(500))

	snapshot := newWalletCache()
	if err := snapshot.copyFrom(walletCache); err != nil {
		t.Fatalf("snapshotting the cache: %s", err.Error())
	}
	cand := &pinCandidate{epoch: 1, cache: snapshot}

	// What the build does to the cache for every account it touches.
	if err := walletCache.remove(wallet, []string{"tx-not-yet-settled"}); err != nil {
		t.Fatalf("removing: %s", err.Error())
	}
	if _, err := walletCache.get(wallet); err == nil {
		t.Fatal("the build did not invalidate the cache, so the test proves nothing")
	}

	cand.undo()

	restored, err := walletCache.get(wallet)
	if err != nil {
		t.Fatalf("the pending balance was not restored after the rollback: %s", err.Error())
	}
	if restored.Cmp(big.NewInt(500)) != 0 {
		t.Fatalf("restored balance is %s, want 500", restored.String())
	}
}

// The state store is the third thing a build changes, and the one that would
// silently fork this node from the rest of the network.
func TestRollingBackALostRoundRevertsTheStateStore(t *testing.T) {
	vm.AttachInMemoryStateStore()
	prevCache := walletCache
	walletCache = newWalletCache()
	t.Cleanup(func() { walletCache = prevCache })

	const account = "0x00000000000000000000000000000000000000bb"
	vm.SyncBalances(map[string][]byte{account: big.NewInt(11).Bytes()})
	before, ok := vm.AccountBalance(account)
	if !ok {
		t.Fatal("the state store has no balance to revert to")
	}

	cand := &pinCandidate{epoch: 1, cache: newWalletCache()}
	vm.Checkpoint()
	cand.vmMarked = true
	vm.SyncBalances(map[string][]byte{account: big.NewInt(9999).Bytes()})

	cand.undo()

	after, ok := vm.AccountBalance(account)
	if !ok {
		t.Fatal("the rollback removed the account")
	}
	if after.Cmp(before) != 0 {
		t.Fatalf("balance after the rollback is %s, want the pre-build %s", after.String(), before.String())
	}
}

// Winning the round is the other half: the work has to stand, and the
// checkpoint has to be released so the next epoch starts from a clean one.
func TestKeepingAWonRoundLeavesTheWorkInPlace(t *testing.T) {
	vm.AttachInMemoryStateStore()

	const account = "0x00000000000000000000000000000000000000cc"
	cand := &pinCandidate{epoch: 1, cache: newWalletCache()}
	vm.Checkpoint()
	cand.vmMarked = true
	vm.SyncBalances(map[string][]byte{account: big.NewInt(77).Bytes()})

	cand.keep()

	got, ok := vm.AccountBalance(account)
	if !ok {
		t.Fatal("keeping the round lost the account")
	}
	if got.Cmp(big.NewInt(77)) != 0 {
		t.Fatalf("balance after keeping the round is %s, want 77", got.String())
	}
	// A second keep must not drop a checkpoint that is not ours: the store
	// panics on a drop with nothing outstanding, which would take the node down
	// on the next epoch rather than here.
	cand.keep()
}

// smcTxsInPool - n distinct transactions sitting in the unconfirmed pool, as
// they would be when a proposer picks them up.
func smcTxsInPool(t *testing.T, n int) []tx.Transaction {
	t.Helper()
	out := make([]tx.Transaction, 0, n)
	for i := 0; i < n; i++ {
		payment := tx.NewTxv1(tx.PRIVATE_TESTNET)
		payment.Tx_Type = tx.PAYMENT
		payment.Sender = addr(byte(0x40 + i))
		payment.Recepient = addr(byte(0x80 + i))
		payment.Amount = big.NewInt(int64(i + 1)).Bytes()
		smc.AddUnconfirmed(payment)
		out = append(out, payment)
	}
	return out
}

func txId(t tx.Transaction) string {
	idt := tx.IdentifiableTx{Transaction: t}
	return idt.Id()
}

func inUnconfirmedPool(id string) bool {
	found := smc.FilterUnconfirmed(func(candidate *tx.IdentifiableTx) bool {
		return candidate.Id() == id
	})
	return len(found) > 0
}

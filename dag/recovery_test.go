package dag

import (
	"encoding/hex"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/store"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// recoveryFixture - the globals recovery touches, restored on cleanup.
func recoveryFixture(t testing.TB, dir string) store.Store {
	t.Helper()
	prevCfg, prevPeer := dagConfig, peerConfig
	prevArchive, prevCounter, prevPins := sliceArchive, confirmationCounter, _pins_
	prevCache, prevConfirmed, prevStore := walletCache, walletCacheConfirmed, ledgerStore
	prevSettled := settled

	dagConfig = config.DagConfiguration{Slicing: true, Approvetx: 2}
	peerConfig = config.PeerConfiguration{Network: 2}
	sliceArchive = newRamArchive()
	confirmationCounter = newConfirmTracker(2, 0)
	_pins_ = newNodeTxPin()
	walletCache = newWalletCache()
	walletCacheConfirmed = newWalletCache()
	// Init builds this per process; a test that inherited another test's ledger
	// would skip its own opening statement as already applied.
	settled = newSettledLedger()

	s, err := store.Open(dir)
	if err != nil {
		t.Fatalf("opening the store: %s", err.Error())
	}
	ledgerStore = s

	t.Cleanup(func() {
		s.Close()
		dagConfig, peerConfig = prevCfg, prevPeer
		sliceArchive, confirmationCounter, _pins_ = prevArchive, prevCounter, prevPins
		walletCache, walletCacheConfirmed, ledgerStore = prevCache, prevConfirmed, prevStore
		settled = prevSettled
	})
	return s
}

// addr - a usable 20-byte account address.
func addr(b byte) []byte {
	a := make([]byte, 20)
	a[19] = b
	return a
}

func addrStr(b byte) string { return "0x" + hex.EncodeToString(addr(b)) }

// paymentSite - a settled site carrying a real payment, which is what the
// balances are rebuilt from.
func paymentSite(n int, from, to byte, amount int64) *Node {
	node := tnode(n)
	t := tx.NewTxv1(tx.PRIVATE_TESTNET)
	t.Tx_Type = tx.PAYMENT
	t.Sender = addr(from)
	t.Recepient = addr(to)
	t.Amount = big.NewInt(amount).Bytes()
	node.tx = t
	return node
}

// storedPin - a commit transaction as it would be formed: settled sites, the
// balances as of that point, and a link to its predecessor.
func storedPin(number int64, prev []byte, balances map[string]string, sites ...*Node) *pb.TxPin {
	pin := pb.NewTxPin(prev)
	pin.PinNumber = number
	pin.Sign = []byte{byte(number + 1)}
	pin.Ts = timestamppb.Now()
	for w, b := range balances {
		v, _ := new(big.Int).SetString(b, 10)
		pin.Balance.Balance[w] = v.Bytes()
	}
	for _, s := range sites {
		pin.Sites = append(pin.Sites, &pb.SiteID{Id: s.id.id[:]})
		pin.Nodes = append(pin.Nodes, s.ToPbNode())
	}
	return pin
}

// A node that restarts must come back on the chain it was already part of,
// rather than starting a new one and resyncing from scratch.
func TestRecoveryRebuildsTheChainAndBalances(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")

	// --- first run: an opening balance, then two settled payments ---
	// The balance maps deliberately disagree with the payments: they are
	// written from the live cache when a commit transaction is formed, so they
	// pick up transactions that are still unconfirmed. Recovery must follow the
	// payments, not the maps.
	func() {
		recoveryFixture(t, dir)
		pinCommitted(storedPin(0, nil, map[string]string{addrStr(0xaa): "1000"}))
		pinCommitted(storedPin(1, []byte{1}, map[string]string{addrStr(0xaa): "1", addrStr(0xbb): "999"},
			paymentSite(1, 0xaa, 0xbb, 100)))
		pinCommitted(storedPin(2, []byte{2}, map[string]string{addrStr(0xaa): "7"},
			paymentSite(2, 0xaa, 0xbb, 100)))
		// Shut down the way the node does, so the balance snapshot recovery
		// trusts is written from the live settled state.
		closeStore()
	}()

	// --- second run: recover ---
	recoveryFixture(t, dir)
	recovered, err := recoverFromStore()
	if err != nil {
		t.Fatalf("recovering: %s", err.Error())
	}
	if !recovered {
		t.Fatalf("recovery reported nothing to recover")
	}

	if got := _pins_.CurrentHeight(); got != 2 {
		t.Errorf("chain head is pin %d, want 2", got)
	}
	if !_pins_.IsReady() {
		t.Errorf("a recovered node should not wait for a balance snapshot")
	}
	for n := 0; n <= 2; n++ {
		if _pins_.GetPin(n) == nil {
			t.Errorf("pin %d is missing from the recovered chain", n)
		}
	}

	// 1000 opening, two payments of 100 out: the payments decide, not the maps
	for wallet, want := range map[string]string{addrStr(0xaa): "800", addrStr(0xbb): "200"} {
		got, err := walletCacheConfirmed.get(wallet)
		if err != nil {
			t.Errorf("recovered balance for %s: %s", wallet, err.Error())
			continue
		}
		if got.String() != want {
			t.Errorf("recovered balance for %s is %s, want %s", wallet, got.String(), want)
		}
	}
	// and the working cache starts from the settled state
	if got, err := walletCache.get(addrStr(0xaa)); err != nil || got.String() != "800" {
		t.Errorf("working balance is %v (err %v), want 800", got, err)
	}
	// nothing created or destroyed by the rebuild
	if got := settled.total().String(); got != "1000" {
		t.Errorf("settled total after recovery is %s, want the 1000 the chain opened with", got)
	}
	// a snapshot must have been written on the way out, or every restart
	// replays the whole chain
	at, balances, err := ledgerStore.Balances()
	if err != nil {
		t.Fatalf("reading the balance snapshot: %s", err.Error())
	}
	if at != 2 {
		t.Errorf("the stored snapshot names pin %d, want 2 (the chain head at shutdown)", at)
	}
	if len(balances) == 0 {
		t.Errorf("the stored snapshot holds no balances")
	}
}

// The sites a recovered chain settled must still be findable, and must not be
// eligible for confirmation a second time.
func TestRecoveryRefillsTheArchive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	var settledNode *Node

	func() {
		recoveryFixture(t, dir)
		settledNode = siteWithTx(7)
		pinCommitted(storedPin(0, nil, map[string]string{addrStr(0xaa): "5"}, settledNode))
		closeStore()
	}()

	recoveryFixture(t, dir)
	if _, err := recoverFromStore(); err != nil {
		t.Fatalf("recovering: %s", err.Error())
	}

	if _, ok := settledSite(settledNode.id.id); !ok {
		t.Fatalf("a settled site is not in the archive after recovery")
	}
	confirmationCounter.add(settledNode)
	if confirmationCounter.isTip(settledNode.id.id) {
		t.Fatalf("a settled site came back as an approval candidate after recovery")
	}
	for _, got := range confirmationCounter.pop() {
		if got.id.id == settledNode.id.id {
			t.Fatalf("a settled site was confirmed again after recovery")
		}
	}
}

func TestRecoveryOnAnEmptyStoreDoesNothing(t *testing.T) {
	recoveryFixture(t, filepath.Join(t.TempDir(), "ledger"))
	recovered, err := recoverFromStore()
	if err != nil {
		t.Fatalf("recovering from an empty store: %s", err.Error())
	}
	if recovered {
		t.Fatalf("an empty store reported a recovery")
	}
}

// A store from another network must not be mistaken for this node's history.
func TestRecoveryRefusesAnotherNetworksStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	func() {
		recoveryFixture(t, dir)
		peerConfig.Network = 0 // written as mainnet
		pinCommitted(storedPin(0, nil, map[string]string{addrStr(0xaa): "1"}))
		closeStore()
	}()

	recoveryFixture(t, dir)
	peerConfig.Network = 2 // started as a private testnet
	if _, err := recoverFromStore(); err == nil {
		t.Fatalf("recovery accepted a store belonging to another network")
	}
}

// Persisting must not depend on slicing, or a node with slicing off would
// restart onto an empty chain.
func TestPinsArePersistedWithSlicingOff(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	func() {
		recoveryFixture(t, dir)
		dagConfig.Slicing = false
		pinCommitted(storedPin(0, nil, map[string]string{addrStr(0xaa): "1"}))
		pinCommitted(storedPin(1, []byte{1}, map[string]string{addrStr(0xaa): "2"}))
		closeStore()
	}()

	recoveryFixture(t, dir)
	recovered, err := recoverFromStore()
	if err != nil || !recovered {
		t.Fatalf("recovery with slicing off: recovered=%v err=%v", recovered, err)
	}
	if got := _pins_.CurrentHeight(); got != 1 {
		t.Fatalf("chain head is pin %d, want 1", got)
	}
}

func TestRecoveredGenesisSiteIsAdopted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	func() {
		recoveryFixture(t, dir)
		genesis := siteWithTx(0)
		genesis.id.id = uuid.Nil // the genesis site's fixed identity
		pinCommitted(storedPin(0, nil, map[string]string{addrStr(0xaa): "1"}, genesis))
		closeStore()
	}()

	recoveryFixture(t, dir)
	if _, err := recoverFromStore(); err != nil {
		t.Fatalf("recovering: %s", err.Error())
	}
	got := recoveredGenesisSite()
	if got == nil {
		t.Fatalf("the genesis site was not recovered from the chain")
	}
	if got.id.id != uuid.Nil {
		t.Fatalf("recovered genesis site has id %s, want the nil uuid", got.id.id)
	}
}

// The pin that opens a node's chain - genesis on a node that starts a ledger,
// the leader's snapshot on a node that joins one - arrives outside the ordinary
// commit path. If it is not folded in, the settled ledger seeds from a later
// commit transaction instead, which only states the balances it happened to
// touch, and every balance after that is wrong. That is exactly what happened
// on a live network: conservation failed by a handful of units.
func TestOpeningPinSeedsTheSettledLedger(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")

	func() {
		recoveryFixture(t, dir)
		// The opening statement: all the money, before any transaction.
		chainStartCommitted(storedPin(0, nil, map[string]string{addrStr(0xaa): "1000"}))
		// Then ordinary commit transactions, whose balance maps are wrong on
		// purpose - a live cache would have folded in unconfirmed activity.
		pinCommitted(storedPin(1, []byte{1}, map[string]string{addrStr(0xaa): "3"},
			paymentSite(1, 0xaa, 0xbb, 100)))
		pinCommitted(storedPin(2, []byte{2}, map[string]string{addrStr(0xaa): "9"},
			paymentSite(2, 0xaa, 0xcc, 250)))
		closeStore()
	}()

	recoveryFixture(t, dir)
	if _, err := recoverFromStore(); err != nil {
		t.Fatalf("recovering: %s", err.Error())
	}

	// Nothing created or destroyed: the opening total survives the restart.
	if got := settled.total().String(); got != "1000" {
		t.Fatalf("settled total after recovery is %s, want the 1000 the chain opened with", got)
	}
	for wallet, want := range map[string]string{
		addrStr(0xaa): "650",
		addrStr(0xbb): "100",
		addrStr(0xcc): "250",
	} {
		got, ok := settled.get(wallet)
		if !ok {
			t.Errorf("no recovered balance for %s", wallet)
			continue
		}
		if got.String() != want {
			t.Errorf("recovered balance for %s is %s, want %s", wallet, got.String(), want)
		}
	}
}

// A snapshot must not be double-counted: the commit transactions it already
// covers are skipped, not replayed onto it.
func TestRecoveryDoesNotReplayWhatTheSnapshotCovers(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")

	func() {
		recoveryFixture(t, dir)
		chainStartCommitted(storedPin(0, nil, map[string]string{addrStr(0xaa): "500"}))
		// pin 0 is a snapshot point, so this one is covered by a later snapshot
		pinCommitted(storedPin(1, []byte{1}, nil, paymentSite(1, 0xaa, 0xbb, 200)))
		closeStore() // writes the snapshot at the current height
	}()

	recoveryFixture(t, dir)
	if _, err := recoverFromStore(); err != nil {
		t.Fatalf("recovering: %s", err.Error())
	}
	if got, _ := settled.get(addrStr(0xbb)); got == nil || got.String() != "200" {
		t.Fatalf("recipient holds %v after recovery, want 200 - the snapshot's pins were replayed onto it", got)
	}
	if got := settled.total().String(); got != "500" {
		t.Fatalf("settled total is %s after recovery, want 500", got)
	}
}

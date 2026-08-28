package dag

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/crypto/eth"
	"github.com/Grape-Chain/Grape-Dag/smc"
	"github.com/Grape-Chain/Grape-Dag/stats"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/Grape-Chain/Grape-Dag/vm"
	"github.com/enescakir/emoji"
	"github.com/golang-collections/collections/set"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
)

var storageNode NodeTxPin

type NodeTxPin struct {
	NodeType int
	pins     []*pb.TxPin
	mu       sync.Mutex
	last     string
	ready    bool
	// view - an immutable copy of the chain published on every mutation, so
	// readers (REST, eth-RPC, balance lookups) never take p.mu. Taking the
	// mutex on the read path would serialise every query behind pin
	// application, which re-executes smart contracts against the VM over gRPC
	// and can therefore block for a long time - or indefinitely, if the VM is
	// unreachable.
	view atomic.Pointer[[]*pb.TxPin]
	// buildMu - only one commit transaction may be under construction at a
	// time.
	//
	// This used to be a side effect of add() holding p.mu across the whole
	// build. It no longer does, and building is not re-entrant: the
	// smart-contract stage enables diff capture on the VM state store, and
	// vm.CaptureStateStoreDiffs panics outright if capture is already on. Two
	// builds at once is a reachable state - genPinTx runs on the dag watcher
	// and again from Dag.SyncUp, which the diffusion goroutines call - so the
	// serialisation that was accidental is now stated.
	//
	// Always taken before p.mu, never while p.mu is held: add() holds it across
	// the whole attempt, including the two short windows in which it holds the
	// pin lock. The quorum path has to take it through LockBuild before it takes
	// the pin lock, for the same reason.
	buildMu sync.Mutex
	// downloadedPins carries a batch of pins fetched to close a gap. dlMu and
	// dlClosed exist because the producer closes the channel to signal the end
	// of a batch, and a late send on a closed channel would panic the node.
	downloadedPins chan *pb.TxPin
	dlMu           sync.Mutex
	dlClosed       bool
}

// pinBodyRetainPins - how many commit transactions at the head of the chain
// keep the transactions they settled.
//
// The chain itself is retained in full: every pin keeps its signature, its
// predecessor, its number, the site ids it settled and the balances it stated,
// because those are what the readers of the chain ask for and several of them
// (gap detection, the balance lookups, the wallet enumeration) walk all the way
// back. What is released past the window is pin.Nodes - a protobuf copy of
// every transaction the pin settled, which is the bulk of the bytes and which
// only the query paths read - together with the contract transactions and state
// diffs.
//
// A var rather than a const so that a test can shrink the window; nothing else
// writes it.
//
// Sixty-four commit transactions is about five minutes at the default commit
// interval, which covers the query paths' working set and any gap a peer closes
// by asking for pins. It is deliberately not "enough for any peer, ever": the
// store holds the whole chain, and serving a deep catch-up from disk is the
// right fix, not an unbounded cache. See getAllFrom, which refuses to hand out
// a pin whose transactions have been released rather than serve one that the
// receiver would reject as unsigned.
var pinBodyRetainPins = 64

// pinHead - the chain head a commit transaction is built against.
//
// A build reads two things from the chain: the head's signature, which becomes
// the new pin's Prev, and its number, which decides the new pin's number. Both
// are taken once, under a short lock, so that the build itself - which reads
// balances, executes contracts and signs - can run with the lock released. The
// pair is then checked again before the append: see appendIfHeadUnmoved.
type pinHead struct {
	prev   []byte
	number int64 // the number the next pin must carry
	height int64 // the head's own number, -1 when the chain is empty
}

// unsafe_head - the head as a build anchor. Caller must hold p.mu.
func (p *NodeTxPin) unsafe_head() pinHead {
	last := p.unsafe_getLastPin()
	if last == nil {
		return pinHead{prev: []byte{}, number: 0, height: -1}
	}
	return pinHead{prev: last.Sign, number: last.PinNumber + 1, height: last.PinNumber}
}

// headSnapshot - the head as a build anchor, taken under a short lock.
func (p *NodeTxPin) headSnapshot() pinHead {
	p.lock("headSnapshot")
	defer p.unlock()
	return p.unsafe_head()
}

// appendIfHeadUnmoved - append a commit transaction built against head, and
// report whether it went on the chain.
//
// The number and the signature are both checked. The number alone would miss a
// head replaced at the same number, which insertIfNotFound can do to the
// opening pin; the signature alone would miss nothing, but the pair is what the
// new pin actually names.
func (p *NodeTxPin) appendIfHeadUnmoved(head pinHead, pin *pb.TxPin) bool {
	p.lock("appendIfHeadUnmoved")
	defer p.unlock()
	now := p.unsafe_head()
	if now.height != head.height || !bytes.Equal(now.prev, head.prev) {
		return false
	}
	p.unsafe_appendPin(pin)
	return true
}

// chain - the current published view of the pin chain. Lock-free; the returned
// slice is never mutated in place, only replaced.
func (p *NodeTxPin) chain() []*pb.TxPin {
	if v := p.view.Load(); v != nil {
		return *v
	}
	return nil
}

// unsafe_publish - republish the reader view after mutating p.pins.
// Caller must hold p.mu.
func (p *NodeTxPin) unsafe_publish() {
	cp := make([]*pb.TxPin, len(p.pins))
	copy(cp, p.pins)
	p.view.Store(&cp)
}

// unsafe_appendPin - add a pin to the chain and republish the reader view.
// Every mutation of p.pins must go through here (or call unsafe_publish
// itself), otherwise readers keep seeing a stale chain. Caller must hold p.mu.
func (p *NodeTxPin) unsafe_appendPin(pin *pb.TxPin) {
	p.pins = append(p.pins, pin)
	p.unsafe_releaseOldBodies()
	p.unsafe_publish()
}

// unsafe_releaseOldBodies - drop the settled transactions from pins that have
// fallen out of the retain window. Caller must hold p.mu.
//
// The pin is replaced rather than emptied in place. A reader may be holding the
// pointer - the published view hands them out, and eth-RPC keeps one while it
// marshals a block - so mutating it would be a data race and would change a pin
// under a reader's feet. Replacing costs one small allocation per pin and lets
// whoever still holds the old pointer keep the whole thing until they are done
// with it.
//
// Index 0 is never released: on a recovered node it is the chain's opening
// statement, and dag.Init reads the genesis site back out of its Nodes to
// restore the graph root. Releasing it would make a restarted node mint a new
// root instead of keeping the one its chain was built on.
func (p *NodeTxPin) unsafe_releaseOldBodies() {
	if pinBodyRetainPins <= 0 {
		return
	}
	for i := len(p.pins) - 1 - pinBodyRetainPins; i > 0; i-- {
		if p.pins[i] == nil || !pinCarriesBody(p.pins[i]) {
			// Everything below has already been released; releasing walks
			// backwards from the window edge on every append, so it only ever
			// has one pin to do.
			return
		}
		p.pins[i] = pinWithoutBody(p.pins[i])
	}
}

// pinCarriesBody - whether a pin still holds the transactions it settled.
//
// A pin that settled nothing - which is what a chain-start pin carrying only
// balances is - counts as carrying its body: there is nothing to release, and
// reporting it as released would make getAllFrom refuse to serve it.
func pinCarriesBody(pin *pb.TxPin) bool {
	return pin != nil && (len(pin.Nodes) > 0 || len(pin.Sites) == 0)
}

// pinWithoutBody - the same commit transaction with the settled transactions,
// the contract transactions and the state diffs left out.
//
// Everything else is carried over, including the signature. The signature no
// longer verifies against what is left, which is deliberate: a pin in this form
// is a local record, and anything that would put it back on the wire has to
// refuse it rather than send something a peer will reject as unauthorised.
func pinWithoutBody(pin *pb.TxPin) *pb.TxPin {
	return &pb.TxPin{
		Prev:         pin.Prev,
		Ts:           pin.Ts,
		Sites:        pin.Sites,
		Balance:      pin.Balance,
		Pk:           pin.Pk,
		Sign:         pin.Sign,
		PinNumber:    pin.PinNumber,
		Quorum:       pin.Quorum,
		Proposer:     pin.Proposer,
		FeePool:      pin.FeePool,
		Rewards:      pin.Rewards,
		FeeRemainder: pin.FeeRemainder,
		Coinbase:     pin.Coinbase,
	}
}

func (p *NodeTxPin) GetPin(number int) *pb.TxPin {
	pins := p.chain()
	// Prefer the pin whose number matches. Indexing by position only works
	// while the chain starts at zero: a node that joined via a balance snapshot
	// holds pins numbered from wherever the leader was.
	if number >= 0 && number < len(pins) && pins[number].PinNumber == int64(number) {
		return pins[number]
	}
	for _, pin := range pins {
		if pin.PinNumber == int64(number) {
			return pin
		}
	}
	return nil
}

func (p *NodeTxPin) GetLastPin() *pb.TxPin {
	pins := p.chain()
	if len(pins) > 0 {
		return pins[len(pins)-1]
	}
	return nil
}

// unsafe_getLastPin - caller must hold p.mu
func (p *NodeTxPin) unsafe_getLastPin() *pb.TxPin {
	if len(p.pins) > 0 {
		return p.pins[len(p.pins)-1]
	}
	return nil
}

// unsafe_nextPinNumber - the number the next pin appended to the chain must
// carry: one past the current head, or 0 for an empty chain (the genesis pin).
// Caller must hold p.mu.
func (p *NodeTxPin) unsafe_nextPinNumber() int64 {
	if last := p.unsafe_getLastPin(); last != nil {
		return last.PinNumber + 1
	}
	return 0
}

func (p *NodeTxPin) unlock() {
	// clear the diagnostic before releasing, or concurrent lock/unlock pairs
	// race on the field itself
	p.last = ""
	p.mu.Unlock()
}

func (p *NodeTxPin) lock(caller string) {
	p.mu.Lock()
	p.last = caller
}

func (p *NodeTxPin) LockPin() {
	p.mu.Lock()
}

func (p *NodeTxPin) UnlockPin() {
	p.mu.Unlock()
}

func (p *NodeTxPin) IsReady() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ready
}

func (p *NodeTxPin) InPin(vertex *Node) bool {
	p.lock("InPin")
	defer p.unlock()
	for l := len(p.pins) - 1; l >= 0; l-- {
		_, _, err := goterators.Find(p.pins[l].Sites, func(site *pb.SiteID) bool {
			return bytes.Equal(site.Id, vertex.id.id[:])
		})
		if err == nil {
			return true
		}
	}
	return false
}

func newNodeTxPin() *NodeTxPin {
	return &NodeTxPin{
		pins:           []*pb.TxPin{},
		mu:             sync.Mutex{},
		last:           string(""),
		ready:          false,
		downloadedPins: make(chan *pb.TxPin, 1000),
	}
}

func (p *NodeTxPin) set(genesis *Node, wallet string) {
	p.lock("set")
	defer p.unlock()
	pin := pb.NewTxPin([]byte{})
	s := &pb.SiteID{
		Id:      append([]byte(nil), genesis.id.id[:]...),
		Address: genesis.id.address,
		IdMajor: genesis.id.idMajor,
		IdMinor: genesis.id.idMinor,
	}
	pin.Balance.Balance[wallet] = genesis.tx.GetAmount().Bytes()
	pin.Sites = append(pin.Sites, s)
	// The genesis pin is number 0. This must be set before signing: the
	// signature covers the marshalled pin, PinNumber included.
	pin.PinNumber = p.unsafe_nextPinNumber()
	pin.SignTx(_dag_.Wallet())

	p.unsafe_appendPin(pin)
	// This node has a chain of its own now, so it is ready to apply commit
	// transactions from others. Only a node that synced one from a peer used to
	// set this, because in leader mode the node that starts a chain is also the
	// only node that ever settles it. Under a validator quorum every validator
	// publishes when it is the proposer, so a chain-starting node that never
	// became ready would refuse every commit transaction it did not build - it
	// would watch the rest of the network settle the ledger and stay at its own
	// genesis.
	p.ready = true
	// The genesis pin is where the initial offering enters the ledger, so it is
	// both the store's record of the chain's identity and the only statement of
	// where the money came from.
	chainStartCommitted(pin)
	// The genesis site reaches a pin without going through the confirmed pool,
	// so record it as harvested here: otherwise the first two sites to approve
	// genesis promote it and it lands in a second pin.
	if confirmationCounter != nil {
		confirmationCounter.markHarvested(genesis.id.id)
	}
}

// snapshotPins - a shallow copy of the pin chain, taken under the pin lock.
// Readers iterate the copy so that an append (which may reallocate the backing
// array) cannot race them.
func (p *NodeTxPin) snapshotPins() []*pb.TxPin {
	return p.chain()
}

func (p *NodeTxPin) getLast() *pb.TxPin {
	p.lock("getLast")
	defer p.unlock()
	if len(p.pins) > 0 {
		return p.pins[len(p.pins)-1]
	}
	return nil
}

// getAllFrom - the chain from this pin number onwards, for a peer closing a gap.
//
// Selected by pin number rather than by position. The two agree only on a chain
// that starts at zero; a node that joined from a balance snapshot holds pins
// numbered from wherever the leader was, and indexing by position there either
// returned the wrong pins or, more often, nothing at all.
//
// A pin whose transactions have been released past the retain window is not
// served. The receiver derives balances from those transactions and verifies the
// signature over the whole pin, so sending one would either be refused as
// unauthorised or - worse, if authorisation were ever relaxed - applied as a pin
// that settled nothing. The requester is told nothing rather than told a lie,
// and the gap is closed by serving the chain from the store; see
// pinBodyRetainPins.
func (p *NodeTxPin) getAllFrom(fromPinNumber int) []*pb.TxPin {
	p.lock("getAllFrom")
	defer p.unlock()
	if fromPinNumber < 0 {
		return nil
	}
	from := -1
	for i, pin := range p.pins {
		if pin != nil && pin.PinNumber >= int64(fromPinNumber) {
			from = i
			break
		}
	}
	if from < 0 {
		return nil
	}
	// The longest run from there that can honestly be sent. A copy of the
	// pointers, not a window onto p.pins: the caller marshals them with the
	// lock released, and the slice it was handed used to be the live one, so an
	// append that reallocated - or a release that replaced an element - changed
	// what was being sent while it was being sent.
	out := []*pb.TxPin{}
	for _, pin := range p.pins[from:] {
		if !pinCarriesBody(pin) {
			break
		}
		out = append(out, pin)
	}
	if len(out) == 0 {
		logger.Errorf("[pin] Cannot serve pin=%d from memory: its transactions were released past the %d-pin retain window. The peer asking from %d has to be served from the store.",
			p.pins[from].PinNumber, pinBodyRetainPins, fromPinNumber)
		return nil
	}
	return out
}

func (p *NodeTxPin) openPinDownloading() {
	p.dlMu.Lock()
	defer p.dlMu.Unlock()
	p.downloadedPins = make(chan *pb.TxPin, 1000)
	p.dlClosed = false
}

func (p *NodeTxPin) GetBalance(wallet []byte) (*big.Int, error) {
	balances, err := p.GetBalances([][]byte{wallet})
	if err != nil {
		return big.NewInt(0), err
	}
	return big.NewInt(0).SetBytes(balances[0]), nil
}

func (p *NodeTxPin) AddDownloadedPin(pin *pb.TxPin) {
	p.dlMu.Lock()
	defer p.dlMu.Unlock()
	if p.dlClosed || p.downloadedPins == nil {
		logger.Warnf("[Gap detected] Discarding pin=%d: no download in progress", pin.PinNumber)
		return
	}
	select {
	case p.downloadedPins <- pin:
	default:
		logger.Warnf("[Gap detected] Download buffer full, discarding pin=%d", pin.PinNumber)
	}
}

func (p *NodeTxPin) ClosePinDownloading() {
	p.dlMu.Lock()
	defer p.dlMu.Unlock()
	if p.dlClosed || p.downloadedPins == nil {
		return
	}
	p.dlClosed = true
	close(p.downloadedPins)
}

// Method <GetLatestBalance> - for the given wallet address return the latest known balance
// args:
//
//	wallet_address - wallet address for each to retrieve the latest balance
//
// returns:
//
//	*big.Int - if wallet address is found, return is last known balance
//	error - if an error occurs, such as there is no information on the wallet
func (p *NodeTxPin) unsafe_getLatestBalance(wallet_address string) (*big.Int, error) {
	// Reads the published view rather than p.pins, so callers that hold only
	// their own lock (the wallet cache, on the payment hot path) do not race
	// the appends made under p.mu.
	pins := p.chain()
	// lookups take place in reverce order to that we always find the latest first
	for ri := len(pins) - 1; ri >= 0; ri-- {
		if v, ok := pins[ri].Balance.Balance[wallet_address]; ok {
			return big.NewInt(0).SetBytes(v), nil
		}
	}
	return nil, fmt.Errorf("unknown wallet %s, cannot obtain balance", wallet_address)
}

func (p *NodeTxPin) insertIfNotFound(ptx *pb.TxPin) {
	p.lock("inserIfNotFound")
	defer p.unlock()
	// this function splices p.pins in several branches; republish the reader
	// view once, whichever branch ran
	defer p.unsafe_publish()
	logger.Infof("%s  ~ %s Inserting pin tx \nS:%s P:%s",
		emoji.RoundPushpin,
		emoji.ClockwiseVerticalArrows,
		hex.EncodeToString(ptx.Sign)[:10],
		func() string {
			if len(ptx.Prev) > 0 {
				return hex.EncodeToString(ptx.Prev)[:10]
			} else {
				return "NIL"
			}
		}(),
	)
	if _, _, err := goterators.Find(p.pins, func(p *pb.TxPin) bool {
		return bytes.Equal(p.Sign, ptx.Sign)
	}); err != nil {
		if len(ptx.Prev) == 0 {
			// the new transaction is a genesis tx, insert it if it does not exist
			if len(p.pins) > 0 {
				if len(p.pins[0].Prev) == 0 {
					// is this a duplicate of the genesis tx?
					if bytes.Equal(p.pins[0].Sign, ptx.Sign) {
						// this is a duplicate, can ignore
						logger.Infof("%s  ~ genesis pin tx already exists", emoji.RoundPushpin)
					} else {
						logger.Warnf("%s  ~ %s different versions of genesis tx", emoji.RoundPushpin, emoji.Warning)
						// replace with the new genesis
						logger.Infof("%s  ~ insert pin tx as genesis S:%s", emoji.RoundPushpin, hex.EncodeToString(ptx.Sign)[:10])
						p.pins[0] = ptx
					}
				} else {
					// let's see where we need to insert this pin tx
					for i, x := range p.pins {
						if bytes.Equal(x.Prev, ptx.Sign) {
							// we found the place
							p.pins = append(p.pins[:i+1], p.pins[i:]...)
							logger.Infof("%s  ~ insert pin tx at %d S:%s", emoji.RoundPushpin, i, hex.EncodeToString(ptx.Sign)[:10])
							p.pins[i] = ptx
							break
						}
					}
				}
			} else {
				// it's empty - add as first
				logger.Infof("%s  ~ insert pin tx as last S:%s", emoji.RoundPushpin, hex.EncodeToString(ptx.Sign)[:10])
				p.pins = append(p.pins, ptx)
			}
		} else {
			// new pin tx needs to be inserted in the right place
			// let's find a pin tx that references this tx
			inserted := false
			for i, x := range p.pins {
				if bytes.Equal(x.Prev, ptx.Sign) {
					// we found the place
					p.pins = append(p.pins[:i+1], p.pins[i:]...)
					logger.Infof("%s  ~ insert pin tx at %d S:%s", emoji.RoundPushpin, i, hex.EncodeToString(ptx.Sign)[:10])
					p.pins[i] = ptx
					inserted = true
					break
				}
			}
			if !inserted {
				//add at the end
				logger.Infof("%s  ~ insert pin tx as last S:%s", emoji.RoundPushpin, hex.EncodeToString(ptx.Sign)[:10])
				p.pins = append(p.pins, ptx)
			}
		}

		// // insert only when we can
		// if ptx.Prev == nil {
		// 	// this is the genesis pin
		// 	logger.Infof("%s  ~ insert genesis pin %s", emoji.RoundPushpin, emoji.MoneyBag)
		// 	l := len(p.pins)
		// 	if l > 1 {
		// 		p.pins = append(p.pins[:1], p.pins[:]...)
		// 		p.pins[0] = ptx
		// 	} else if l == 0 {
		// 		p.pins = append(p.pins, ptx)
		// 	}
		// } else {
		// 	found := false
		// 	for idx, v := range p.pins {
		// 		if bytes.Equal(v.Sign, ptx.Prev) {
		// 			logger.Infof("%s  ~ insert in order %d pin %s", emoji.RoundPushpin, idx, emoji.Coin)
		// 			p.pins = append(p.pins[:idx+1], p.pins[idx:]...)
		// 			p.pins[idx] = ptx
		// 			found = true
		// 			break
		// 		}
		// 	}
		// 	if !found {
		// 		// append as last - the pin sync will request missing...
		// 		logger.Infof("%s  ~ insert as last pin %s", emoji.RoundPushpin, emoji.Coin)
		// 		p.pins = append(p.pins, ptx)
		// 	}
		// }
	} else {
		logger.Infof("%s  ~  pin tx S:%s already exits. Ignore", emoji.RoundPushpin, hex.EncodeToString(ptx.Sign)[:10])
	}
}

func (p *NodeTxPin) unsafe_getBalanceForWallet(wallet string) (*big.Int, error) {
	sb, err := walletCache.get(wallet)
	if err != nil && sb == nil {
		// p, not the package global: the two are the same object on a running
		// node, but reaching for the global from a method that has a receiver
		// made this unusable from a test and would have followed a nil global on
		// any path that ran before Init.
		sb, err = p.unsafe_getLatestBalance(wallet)
		if err != nil {
			// we did not have luck finding this wallet in pin txs
			err = fmt.Errorf("[pin tx] Balance for wallet %s cannot be found. Out of sync or DS", wallet)
			sb = big.NewInt(0)
		}
	}
	return sb, err
}

func (p *NodeTxPin) getById(id uuid.UUID) *Node {
	p.lock("findById")
	defer p.unlock()
	idb, _ := id.MarshalBinary()

	for l := len(p.pins) - 1; l >= 0; l-- {
		n, _, err := goterators.Find(p.pins[l].Nodes, func(node *pb.Node) bool {
			return bytes.Equal(node.Id.Id, idb)
		})
		if err == nil {
			node := &Node{}
			node.FromPbNode(n)
			return node
		}
	}
	return nil
}

// Method <add>  -  adds a new pinning tx with a list of sites that are confirmed and limited list of executed smart contracts
// args:
//
//		a slice of confirmed sites (Nodes) in dag
//
//	    a list of smart contract transactions to execute and add to pin tx
//
// returns:
//
//	error if a balancing error occurs

// add - build a commit transaction over these sites and append it to the chain.
// The single-signer path: what a leader does, where building it and committing
// it are the same act because nobody else has to agree first.
//
// The pin lock is taken twice and briefly: once to read the head the pin is
// built against, once to append it. The build in between runs with the lock
// released, which is the whole point - it reads balances, executes the
// smart-contract transactions against the VM, and signs a payload that is
// megabytes at load, and every reader of the chain (REST balances, eth-RPC,
// CurrentHeight, the consensus driver, the insert path's balance lookups) used
// to wait behind all of it. dag/syncmngr.go appendPinUnderLock splits an
// incoming pin the same way and for the same reason.
//
// If the head moves while the build is unlocked the built commit transaction is
// stale - it names a predecessor that is no longer the head and a number that is
// taken - and it is discarded, never appended. A commit transaction is the
// ledger's only irrevocable statement, so the build is thrown away and repeated
// against the new head; the sites are still in hand, so nothing is lost by
// repeating it. If the head moves again the attempt is abandoned and reported:
// the caller consumed these sites from the confirmed pool, so losing them is
// worth being loud about, but appending a stale pin would be worse.
func (p *NodeTxPin) add(sites []*Node, smcTxs []tx.Transaction) error {
	// One build at a time, and for the whole attempt rather than only around the
	// build: what makes a discarded build undoable is a VM checkpoint, and
	// checkpoints nest, so a second build opening its own between this one's
	// checkpoint and the decision to keep or revert would have the two unwound
	// in the wrong order - reverting a commit transaction that is already on the
	// chain. See buildMu.
	p.LockBuild()
	defer p.UnlockBuild()

	const attempts = 2
	var lastHead pinHead
	for attempt := 1; attempt <= attempts; attempt++ {
		head := p.headSnapshot()
		built, err := p.buildCheckpointed(head, sites, smcTxs)
		if err != nil {
			return err
		}
		if p.appendIfHeadUnmoved(head, built.pin) {
			built.keep()
			return nil
		}
		built.discard()
		lastHead = head
		logger.Warnf("[pin] Discarding the commit transaction built for pin=%d: the chain head moved from %d while it was being built (attempt %d of %d)",
			head.number, head.height, attempt, attempts)
	}
	return fmt.Errorf("the chain head moved away from pin %d while a commit transaction was being built for it; %d site(s) were not settled",
		lastHead.height, len(sites))
}

// LockBuild, UnlockBuild - claim the right to build a commit transaction.
//
// Exported for the quorum path, which builds with the pin lock held and so
// cannot take buildMu from inside the builder without inverting the order. It
// has to take it around its own LockPin/UnlockPin pair instead. Not strictly
// required today - a node is either in leader mode or in quorum mode, and the
// two builders never run together - but the panic if they ever did is in the VM,
// several frames away from anything that would explain it.
func (p *NodeTxPin) LockBuild()   { p.buildMu.Lock() }
func (p *NodeTxPin) UnlockBuild() { p.buildMu.Unlock() }

// buildCheckpointed - form a commit transaction against head, with the state
// store marked so that the build can be undone.
//
// Runs with the pin lock released and buildMu held. Everything it reads is
// either taken from head, published lock-free (unsafe_getLatestBalance reads the
// atomic view), or behind a lock of its own: the wallet cache, the VM, the
// contract pool.
func (p *NodeTxPin) buildCheckpointed(head pinHead, sites []*Node, smcTxs []tx.Transaction) (*builtPin, error) {
	// Marked before the smart-contract stage runs, so that a build the head
	// invalidates can put the state store back. The quorum path takes its own
	// checkpoint around unsafe_buildPin; checkpoints nest, so the two do not
	// interfere.
	vm.Checkpoint()
	built, err := p.buildPin(head, sites, smcTxs)
	if err != nil {
		vm.RevertCheckpoint()
		return nil, err
	}
	built.vmMarked = true
	return built, nil
}

// builtPin - a commit transaction that has been formed but not yet appended,
// with the consequences of forming it and how to settle them.
//
// Forming one is not free of consequence: the smart-contract stage executes its
// transactions against the state store and moves them out of the unconfirmed
// pool, and the balances the pin states supersede what the wallet cache holds
// for those accounts. Whether that stands depends on whether the pin goes on the
// chain, which is not known until the head has been checked again.
type builtPin struct {
	pin *pb.TxPin
	// invalidate - accounts whose cached balance this pin supersedes. Applied
	// only once the pin is on the chain: removing them earlier - which is what
	// the builder used to do, inline - leaves a window in which a balance
	// lookup falls through to the previous pin and so forgets the very
	// transactions this pin settles, which is a window in which they could be
	// spent again.
	invalidate []string
	smcTxs     []tx.Transaction
	vmMarked   bool
}

// keep - the pin is on the chain, so the build stands.
func (b *builtPin) keep() {
	if b.vmMarked {
		vm.DropCheckpoint()
		b.vmMarked = false
	}
	b.invalidateCache()
}

// invalidateCache - drop the cached balances this pin has superseded, so the
// next lookup reads them from the chain.
func (b *builtPin) invalidateCache() {
	for _, wallet := range b.invalidate {
		walletCache.remove(wallet, nil)
	}
}

// discard - the pin is not going on the chain, so the build must not stand.
//
// The same three undos as pinCandidate in dag/consensusnet.go, minus the wallet
// cache, which this path has not touched yet. Not shared with it because that
// type also carries the epoch and candidate bookkeeping the quorum protocol
// needs, and because the two paths differ in exactly this: the quorum path has
// to snapshot and restore the cache, this one only has to not invalidate it.
func (b *builtPin) discard() {
	if b.vmMarked {
		vm.RevertCheckpoint()
		b.vmMarked = false
	}
	// Back to the unconfirmed pool: from the network's point of view the
	// execution never happened. AddUnconfirmed also takes them out of the
	// confirmed pool, which is where the stage put them.
	for _, t := range b.smcTxs {
		if t != nil {
			smc.AddUnconfirmed(t)
		}
	}
}

// unsafe_buildPin - form a commit transaction over these sites, signed and
// numbered, but not appended to the chain.
//
// Split out of add() for the quorum path, where a validator proposes a commit
// transaction and only appends it once the rest of the set has agreed. Building
// it is not free of consequence - the smart-contract stage executes, and the
// wallet cache is invalidated for every account it touches - so a proposer that
// loses its round has to undo that; see pinCandidate in dag/consensusnet.go.
// Caller holds the pin lock.
func (p *NodeTxPin) unsafe_buildPin(sites []*Node, smcTxs []tx.Transaction) (*pb.TxPin, error) {
	built, err := p.buildPin(p.unsafe_head(), sites, smcTxs)
	if err != nil {
		return nil, err
	}
	// The quorum path expects the cache to have been invalidated by the time
	// this returns: pinCandidate restores the cache from a snapshot when it
	// rolls a lost round back, and would have nothing to restore if the entries
	// had never been removed. add() defers it instead, to the point where the
	// pin is on the chain - see builtPin.invalidate.
	built.invalidateCache()
	return built.pin, nil
}

// buildPin - the body of a build, anchored to the head it was given rather than
// to whatever the chain head happens to be while it runs.
//
// Reads no part of p.pins, so it does not need the pin lock: balances come from
// the wallet cache and, behind it, the published chain view, and the head's
// signature and number come from the anchor. That is what lets add() run it
// unlocked; unsafe_buildPin calls it with the lock held because its caller is
// already holding it for other reasons.
func (p *NodeTxPin) buildPin(head pinHead, sites []*Node, smcTxs []tx.Transaction) (*builtPin, error) {
	defer stats.Time(stats.PinBuild)()

	// migrate the wallet cache balance to pin tx - reconcile balances,
	// detect conflicts, update balances for confirmed tx
	// Note: pin tx do not reflect the latest balances,
	// only the balances for the confirmed txs.
	// Wallet cache may still have new balance information for the unconfirmed tx
	// Keep that in mind when updating balances here
	// The most important aspect: detect balance conflicts

	// sort sites based on time in an ascending order - from past to future
	sort.SliceStable(sites, func(i, j int) bool {
		return sites[i].time.Before(sites[j].time)
	})

	// The two accounts each site moves value between, converted once.
	// BytesToAddress hex-encodes and allocates, and this was called eight times
	// per site: twice to look the balances up, and six more times to index the
	// same two maps and the cache again while writing the pin out.
	type siteWallets struct{ sender, recipient string }
	wallets := make([]siteWallets, len(sites))

	// What each account holds as this build sees it: the wallet cache first,
	// and the chain behind it. One value per account, where there used to be a
	// slice per account of which only the last entry was ever read - and every
	// entry in it was the same lookup repeated, because nothing between them
	// changed the cache.
	balances := make(map[string]*big.Int, 2*len(sites))
	// settlingBalance - the balance to state for an account this pin settles
	// for. Loud about a miss, because a payment whose sender has no balance is
	// the shape a double spend takes.
	settlingBalance := func(wallet, role string) *big.Int {
		if b, ok := balances[wallet]; ok {
			return b
		}
		b, err := p.unsafe_getBalanceForWallet(wallet)
		if err != nil {
			logger.Error(err.Error())
			// this is a fatal error - perhaps it's related to double spending, fraud, etc.
			logger.Warnf("[@DEVNOTE] %s balance not found. This is not normal. Decide what to do with this.", role)
			b = big.NewInt(0)
		}
		balances[wallet] = b
		return b
	}
	for i, vertex := range sites {
		w := siteWallets{
			sender:    grape1crypto.BytesToAddress(vertex.tx.GetSender()),
			recipient: grape1crypto.BytesToAddress(vertex.tx.GetRecipient()),
		}
		wallets[i] = w
		settlingBalance(w.sender, "Sender")
		// the receiver may not be known to us at all at this point
		settlingBalance(w.recipient, "Receiver")
	}

	pin := pb.NewTxPin(head.prev)
	// Sized up front: one entry per site each, and this is the allocation the
	// heap profile finds the node holding most of its live bytes in.
	pin.Nodes = make([]*pb.Node, 0, len(sites))
	pin.Sites = make([]*pb.SiteID, 0, len(sites))
	built := &builtPin{
		pin:        pin,
		invalidate: make([]string, 0, 2*len(sites)),
		smcTxs:     smcTxs,
	}
	// each sites indicates a new transaction - process all transactions
	for i, val := range sites {
		w := wallets[i]
		// store sites in pin [for now] - need for synch
		pin.Nodes = append(pin.Nodes, val.ToPbNode())
		// Copied, not sliced. Slicing takes a reference into the live Node's
		// uuid array, so every commit transaction the node retains keeps every
		// site it settled reachable - with its edge slices - and slicing them
		// out of the graph frees nothing. Sixteen bytes per site against
		// holding the site itself.
		s := &pb.SiteID{
			Id:      append([]byte(nil), val.id.id[:]...),
			Address: val.id.address,
			IdMajor: val.id.idMajor,
			IdMinor: val.id.idMinor,
		}
		// for each site update the balance, when valid
		senderBalance := balances[w.sender]
		if senderBalance.Sign() < 0 {
			// this transaction cannot be processed
			val.valid = false
			logger.Warnf("[@DEVNOTE] Need to revert balances in cache")
			logger.Errorf("Sender wallet %s balance is %s", w.sender, senderBalance.String())
		}
		receiverBalance := balances[w.recipient]
		pin.Sites = append(pin.Sites, s)
		// allow tx balance update
		pin.Balance.Balance[w.sender] = senderBalance.Bytes()
		pin.Balance.Balance[w.recipient] = receiverBalance.Bytes()
		// The cached balances this pin supersedes. Removed once it is on the
		// chain, not here: see builtPin.invalidate.
		built.invalidate = append(built.invalidate, w.sender, w.recipient)
	}
	// The number the anchor says this pin must carry. Derived from the chain
	// head rather than a process-local counter, so it stays correct across
	// restarts and after syncing pins from a peer. Set before signing and
	// before the smart-contract stage, which reports it to the VM as the block
	// number.
	pin.PinNumber = head.number
	p.runSmartContractStage(pin, smcTxs, &built.invalidate)

	// The fee split, once the smart-contract stage has said what its
	// transactions actually burned. Before signing, because the split is part of
	// what the signature and the validator quorum cover: a commit transaction
	// that could be re-split after being agreed would let the proposer pay
	// itself. A no-op while fees are off, which is the default.
	recordRewards(pin, sites, func(account string) *big.Int {
		// Shares the lookups above rather than repeating them, and stays quiet
		// about an account it cannot find: a processor that has never held a
		// balance is ordinary, and is worth nothing rather than worth a log
		// line on every commit.
		if b, ok := balances[account]; ok {
			return b
		}
		balance, err := p.unsafe_getBalanceForWallet(account)
		if err != nil || balance == nil {
			balance = big.NewInt(0)
		}
		balances[account] = balance
		return balance
	}, rewardSettingsFrom(txSettings{
		Minstake:      txConfig.Minstake,
		Stakecapmilli: txConfig.Stakecapmilli,
	}, dagConfig.Coinbaseaccount))

	// now that all the information has been collected, sign it
	pin.SignTx(dagWallet)
	return built, nil
}

// runSmartContractStage - execute the contract transactions this pin carries and
// fold what they changed into it.
//
// invalidate collects the accounts whose cached balance the stage has
// superseded, rather than removing them from the cache here: the caller decides
// when that becomes true, which is when the pin reaches the chain.
func (p *NodeTxPin) runSmartContractStage(pin *pb.TxPin, smcTxs []tx.Transaction, invalidate *[]string) {
	// Smart contracts execution stage
	// First step is balances synchronization - put updated balances from this pin tx into VM state store
	// Second step - execute smart contract transactions and get receipts (execution results) with logs produced during execution
	// Third step - collect changes applied to the state
	// Fourth step - synchronize balances affected during sc execution back to pin.Balance.Balace
	// Fifth step - remove temporary cached balances within wallet_cache (affected during sc execution)
	// to force it to update balance on next lookup
	logger.Infof("Smart contract stage started, pin=%d, txs=%d", pin.PinNumber, len(smcTxs))
	startTime := time.Now()
	vm.SyncBalances(pin.Balance.Balance)   // sync balances to storage
	vm.CaptureStateStoreDiffs()            // enable state store changes capture
	defer vm.ResetCaptureStateStoreDiffs() // disable state store changes capture even when this method fails for some reason
	executedTxs := []*pb.ExecutedSmcTx{}
	pinHash, _ := pin.Hash(crypto.SHA256)
	pinHashStr := "0x" + hex.EncodeToString(pinHash)
	timestamp := pin.Ts.AsTime().Unix()
	gasUsed := 0
	for _, node := range pin.GetNodes() {
		if node.Tx.GetRlpEthTx() != nil { // increase nonce for non-payment eth txs
			vm.NonceIncr(node.Tx.Sender)
		}
	}
	for smcTxIdx, smcTx := range smcTxs {

		execResult, err := p.executeSMCTx(smcTx, int64(timestamp), pin.PinNumber)
		if err != nil {
			hash := smcTx.GetHash()
			logger.Warnf("Transaction %s is invalid (during execution), removing from smc pool", "0x"+hex.EncodeToString(hash))
			smc.RemoveUnconfirmed(smcTx)
			continue
		}
		status := tx.Successful
		if !execResult.Successful {
			status = tx.Failed
		}
		gasUsed += int(execResult.GasUsed)
		smc.AddConfirmed(tx.ConfirmedTx{IdentifiableTx: tx.IdentifiableTx{Transaction: smcTx},
			Status: status, StatusMessage: execResult.Output, PinTxNumber: int(pin.PinNumber), UsedFuel: int(execResult.GasUsed),
			PinTxHash: pinHashStr, TxIndex: len(pin.Nodes) + smcTxIdx, CumulativeGasUsed: gasUsed})
		receipt := execResultToReceipt(execResult)
		execSmc := pb.ExecutedSmcTx{Tx: smcTx.MarshalBinary(), Receipt: receipt}
		executedTxs = append(executedTxs, &execSmc)
	}
	pin.SmcTxs = executedTxs
	diffSlice := vm.GetStateStoreDiffs()
	pin.Diffs = diffToPb(diffSlice)

	for _, diff := range diffSlice {
		if diff.HasAccount() {
			address := "0x" + diff.Account.Address
			balance := new(big.Int)
			balance, _ = balance.SetString(diff.Account.Balance, 10)
			pin.Balance.Balance[address] = balance.Bytes()
			// Queued rather than removed here, to force a balance update on the
			// next lookup once this pin is on the chain.
			if invalidate != nil {
				*invalidate = append(*invalidate, address)
			}
			logger.Infof("Account %s balance was synchronized with dag, now balance -  %v", address, balance)
		}
	}
	logger.Infof("Smart contract stage finished in %d mcs", time.Since(startTime).Microseconds())
}

func (pin *NodeTxPin) runSmartContractStageFullNode(balances map[string][]byte, recentPin *pb.TxPin) error {
	logger.Infof("Smart contract stage started at full node, pin=%d, txs=%d", recentPin.PinNumber, len(recentPin.SmcTxs))
	vm.SyncBalances(balances)
	vm.CaptureStateStoreDiffs()
	defer vm.ResetCaptureStateStoreDiffs()
	executedTxs := []*pb.ExecutedSmcTx{}
	timestamp := recentPin.Ts.AsTime().Unix()
	execTx := recentPin.SmcTxs
	for _, smcTx := range execTx {
		realTx := tx.UnmarshalBinary(smcTx.Tx)
		realJson := realTx.String()
		logger.Infof("Transaction info:  %s", realJson)
		execResult, err := pin.executeSMCTx(realTx, int64(timestamp), recentPin.PinNumber)
		if err != nil {
			hash := realTx.GetHash()
			logger.Warnf("Synchronization failed! Transaction %s is invalid (during execution)", "0x"+hex.EncodeToString(hash))
			return errors.New("Fullnode sync failed")
		}
		status := tx.Successful
		statusMsg := "SUCCESSFUL"
		if !execResult.Successful {
			status = tx.Failed
			statusMsg = "FAILED"
		}

		receipt := execResultToReceipt(execResult)
		execSmc := pb.ExecutedSmcTx{Tx: realTx.MarshalBinary(), Receipt: receipt}
		executedTxs = append(executedTxs, &execSmc)

		logger.Debugf("Number of pin tx - %d, Smc execution result is %s, %s, fuel used - %d ", len(pin.pins), status, receipt, int(execResult.GasUsed))

		if int32(execResult.GasUsed) != smcTx.Receipt.FuelUsed {
			logger.Warnf("Fullnode is out of sync! Fuel used expected %d, used - %d", int32(execResult.GasUsed), smcTx.Receipt.FuelUsed)
			return errors.New("Fullnode sync failed")

		}
		if tx.Status(statusMsg) != tx.Status(smcTx.Receipt.Status.String()) {
			logger.Warnf("Fullnode is out of sync! Expected status - %s, real status - %s ", tx.Status(statusMsg), tx.Status(smcTx.Receipt.Status.String()))
			return errors.New("Fullnode sync failed")
		}
		if execResult.Output != smcTx.Receipt.StatusMessage {
			logger.Warnf("Fullnode is out of sync! Execution results are not the same! Expected - %s, real - %s", execResult.Output, smcTx.Receipt.StatusMessage)
			return errors.New("Fullnode sync failed")
		}
		logs := execResultToReceipt(execResult).Logs
		for i, log := range logs {
			if !bytes.Equal(log.GetData(), smcTx.Receipt.Logs[i].GetData()) {
				return errors.New("fullnode sync failed! Sync failed by logs")
			}
		}
	}
	diffs := vm.GetStateStoreDiffs()
	for _, diff := range diffs {
		if diff.HasAccount() {
			address := "0x" + diff.Account.Address
			balance := new(big.Int)
			balance, _ = balance.SetString(diff.Account.Balance, 10)
			walletCacheConfirmed.cache[address] = append(walletCacheConfirmed.cache[address], newPair[string, *big.Int]("sc", balance))
			cacheBalance, _ := walletCacheConfirmed.get(address)
			pinBalance := big.NewInt(0).SetBytes(recentPin.Balance.Balance[address])
			logger.Infof("Account %s balance updated according to VM execution results, now balance -  %s, in confirmed cache - %s, in received pin %s", address, balance.String(), cacheBalance.String(), pinBalance)

		}

	}

	logger.Info("Sync by diffs is successfull")
	return nil
}

func (p *NodeTxPin) CurrentHeight() int {
	pins := p.chain()
	if len(pins) == 0 {
		return 0
	}
	return int(pins[len(pins)-1].PinNumber)
}

// unsafe_currentHeight - the pin number at the head of the chain.
// Caller must hold p.mu.
func (p *NodeTxPin) unsafe_currentHeight() int {
	if len(p.pins) == 0 {
		return 0
	}
	return int(p.pins[len(p.pins)-1].PinNumber)
}

func (p *NodeTxPin) CurrentTS() int64 {
	pins := p.chain()
	if len(pins) == 0 {
		return 0
	}
	return pins[len(pins)-1].Ts.AsTime().Unix()
}

func (p *NodeTxPin) UpdateIfValid(wallet string, amount *big.Int) bool {
	p.lock("UpdateIfValid")
	defer p.unlock()
	valid := false
	// start looking for wallets in reverse
	known_balance := big.NewInt(0)
	for l := len(p.pins) - 1; l >= 0; l-- {
		txmap := p.pins[l].Balance.Balance
		if b, ok := txmap[wallet]; ok {
			balance := big.NewInt(0).SetBytes(b)
			balance = balance.Sub(balance, amount)
			if balance.Cmp(big.NewInt(0)) >= 0 {
				p.pins[l].Balance.Balance[wallet] = balance.Bytes()
				known_balance = balance
				valid = true
				break
			}
		}
	}
	logger.Infof("[$] Update balance for %s. valid %t, amount %d", wallet, valid, known_balance.Int64())
	return valid
}

// GetWallets - return all known wallets
func (p *NodeTxPin) GetWallets() ([][]byte, error) {
	p.lock("GetWallets")
	defer p.unlock()
	resp := [][]byte{}
	wallet_set := set.New()
	// genesis wallet should be the first, if not only wallet known at start up time
	for _, pin := range p.pins {
		for wallet, _ := range pin.Balance.Balance {
			wallet_set.Insert(wallet)
		}
	}
	// add what we have in cache
	walletCache.lock()
	for wallet, _ := range walletCache.cache {
		wallet_set.Insert(wallet)
	}
	walletCache.unlock()

	wallet_set.Do(func(i interface{}) {
		wallet_address := i.(string)
		resp = append(resp, grape1crypto.AddressToBytes(wallet_address))
	})

	return resp, nil
}

func (p *NodeTxPin) GetPinnedBalances(pinNumber int64) (map[string][]byte, error) {
	p.lock("GetPinnedBalances")
	defer p.unlock()
	resp := map[string][]byte{}

	// genesis wallet should be the first, if not only wallet known at start up time
	for _, pin := range p.pins {
		if pin.PinNumber > pinNumber {
			break
		}
		for wallet, balance := range pin.Balance.Balance {
			resp[wallet] = balance
		}
	}
	return resp, nil
}

func (p *NodeTxPin) GetBalances(wallets [][]byte) ([][]byte, error) {
	p.lock("GetBalances")
	defer p.unlock()
	resp := [][]byte{}
	// go through all the wallets we are interested in
	for _, wallet := range wallets {
		// go through the pins in a descending order until we either find
		// or not the desired balance
		success := false
		// var bs []*Pair[string, *big.Int]
		// var ok bool
		walletCache.lock()
		bs, ok := walletCache.cache[grape1crypto.BytesToAddress(wallet)]
		walletCache.unlock()
		if !ok || len(bs) == 0 {
			for l := len(p.pins) - 1; l >= 0; l-- {
				wallet_str := grape1crypto.BytesToAddress(wallet)
				if balance, ok := p.pins[l].Balance.Balance[wallet_str]; ok {
					logger.Debugf("[GetBalance] *** Wallet %s found in pin %d", wallet_str, l)
					resp = append(resp, balance)
					success = true
					break
				}
			}
		} else {
			if len(bs) == 0 {
				// wallet is present, but the balance is unknown.
				logger.Warnf("[@DEVNOTE] Wallet %s is in cache but balance unknown", wallet)
				resp = append(resp, big.NewInt(0).Bytes())
			} else {
				resp = append(resp, bs[len(bs)-1].second.Bytes())
			}
			success = true
		}
		if !success {
			return nil, fmt.Errorf("Balance for wallet %s not found", grape1crypto.BytesToAddress(wallet))
		}
	}
	return resp, nil
}

func diffToPb(diffs []vm.Diff) []*pb.Diff {
	result := []*pb.Diff{}
	for _, diff := range diffs {
		if diff.HasAccount() {
			accountData := pb.AccountData{}
			accountData.Address = diff.Account.AddressBytes()
			balance, _ := big.NewInt(0).SetString(diff.Account.Balance, 10)
			nonce, _ := big.NewInt(0).SetString(diff.Account.Nonce, 10)
			accountData.Balance = balance.Bytes()
			accountData.Nonce = nonce.Int64()
			accountData.Codehash, _ = hex.DecodeString(diff.Account.CodeHash)
			accountData.Code, _ = hex.DecodeString(diff.Account.Code)
			result = append(result, &pb.Diff{MappingOrAccount: &pb.Diff_AccountDiff{AccountDiff: &pb.AccountDiff{NewValue: &accountData}}})
		} else {
			mappingDiff := pb.Mapping{}
			mappingDiff.Address = diff.MappingDiffValue.Address
			mappingDiff.Key = diff.MappingDiffValue.Key
			mappingDiff.Value = diff.MappingDiffValue.Value
			result = append(result, &pb.Diff{MappingOrAccount: &pb.Diff_MappingDiff{MappingDiff: &mappingDiff}})
		}
	}
	return result
}

func execResultToReceipt(r ExecutionResult) *pb.TxReceipt {
	hash, err := hex.DecodeString(r.Hash)
	if err != nil {
		panic(err)
	}
	logs := vm.GetLogsForTx(hash)
	execLogs := []*pb.TxExecLog{}
	for _, v := range logs {
		topics := []*pb.LogTopic{}
		for _, t := range v.Topics {
			topics = append(topics, &pb.LogTopic{Hash: t})
		}
		execLogs = append(execLogs, &pb.TxExecLog{ContractAddress: v.ContractAddress, Topics: topics})
	}
	status := pb.TxReceipt_SUCCESSFUL
	if !r.Successful {
		status = pb.TxReceipt_FAILED
	}
	receipt := pb.TxReceipt{FuelUsed: int32(r.GasUsed), Status: status, StatusMessage: r.Output, Logs: execLogs}
	return &receipt
}

// parseVmError - a revert payload from the VM carries an ABI-encoded reason;
// anything else is a message from the VM itself.
//
// This used to be parsed inline here, with a panic on every unexpected shape -
// a short payload, unparseable hex, an offset that did not fit. That turned
// malformed contract output into a dead node, on the pin path of all places.
// The decoding now lives in crypto/eth, is shared with the REST layer, and
// returns a value for input it cannot read.
func parseVmError(err string) error {
	if eth.IsRevertPayload(err) {
		return eth.ParseRevert(err)
	}
	return fmt.Errorf("system VM error during tx execution: %s", err)
}

func emptyHex(hex string) bool {
	return hex == "" || hex == "0x" || len(hex) <= 2
}

type ExecutionResult struct {
	Hash       string
	Successful bool
	Output     string
	GasUsed    int64
}

// executeSMCTx - run one smart-contract tx on the VM in the context of the pin
// identified by pinNumber (reported to the VM as the block number).
func (p *NodeTxPin) executeSMCTx(transaction tx.Transaction, timestamp int64, pinNumber int64) (ExecutionResult, error) {
	hash := transaction.GetHash()
	execResult := ExecutionResult{}
	client, err := vm.ConnectToVm()
	if err != nil {
		logger.Errorf("Unable to connect to VM: %s", err.Error())
		return execResult, err
	}
	coinbaseAddrBytes, err := eth.ParseEthAddress(config.GetConfig().Dag.Coinbaseaccount)
	if err != nil {
		return execResult, fmt.Errorf("invalid coinbase account %q in configuration: %w",
			config.GetConfig().Dag.Coinbaseaccount, err)
	}
	txPin := &pb.PinTxHeader{CoinbaseAccountAddress: &pb.Address{AddBytes: coinbaseAddrBytes},
		Timestamp: timestamp, TxNumber: int32(pinNumber)}
	response, vmErr := client.RunCall(context.TODO(), &pb.WriteContractRequest{Tx: transaction.MarshalBinary(), Header: txPin})
	if vmErr != nil {
		logger.Errorf("VM error occurred during tx=%v execution: %s", transaction, vmErr.Error())
		return execResult, vmErr
	}
	if response.Status == -2 { // system error occurred
		logger.Debug("Smc execution has gone wrong, vm response status is -2")
		return execResult, parseVmError(response.Error) // bad tx, abandon
	} else {
		gasUsed, err := strconv.ParseInt(response.GasUsed, 10, 64)
		if err != nil {
			return execResult, err
		}
		if response.Status > 0 || response.Status == -1 { // solidity error or general VM error occurred (out of gas, bad instruction, etc), keep tx
			parsedErr := ""
			if emptyHex(response.Msg) {
				parsedErr = parseVmError(response.Error).Error()
			} else {
				parsedErr = parseVmError(response.Msg).Error()
			}
			execResult.Successful = false
			execResult.Output = parsedErr
			execResult.GasUsed = gasUsed
		} else {
			execResult.Output = response.Msg
			execResult.Successful = true
			execResult.GasUsed = gasUsed
		}
		status := tx.Successful
		if !execResult.Successful {
			status = tx.Failed
		}

		format := "Transaction %s executed, status = %s, output=%s, gas=%d"
		var colorizedFormat string
		if execResult.Successful {
			colorizedFormat = string(utils.Green) + format + string(utils.Reset)
		} else {
			colorizedFormat = string(utils.Red) + format + string(utils.Reset)
		}
		logger.Infof(colorizedFormat, hex.EncodeToString(hash), status, execResult.Output, execResult.GasUsed)
		execResult.Hash = hex.EncodeToString(hash)
		return execResult, nil
	}
}

func (pin *NodeTxPin) SyncPins(recentPin *pb.TxPin) {
	logger.Debug("Executing slices at fullnode started")
	balances := make(map[string][]byte)
	sites := recentPin.Nodes
	//here we adding balances to fill walletcacheConfirmed

	for _, payment := range sites {
		senderBalance, err := walletCacheConfirmed.get(grape1crypto.BytesToAddress(payment.Tx.Sender))
		if err != nil {
			senderBalance, err = pin.unsafe_getLatestBalance(grape1crypto.BytesToAddress(payment.Tx.Sender))
			if err != nil {
				logger.Errorf("Sender's wallet %s does not exist", grape1crypto.BytesToAddress(payment.Tx.Sender))

			}
		}
		transferAmount := new(big.Int)
		transferAmount.SetBytes(payment.Tx.Amount)
		walletCacheConfirmed.sub(grape1crypto.BytesToAddress(payment.Tx.Sender), payment.Tx.String(), transferAmount)

		walletCacheConfirmed.add(grape1crypto.BytesToAddress(payment.Tx.Recepient), payment.Tx.String(), transferAmount)
		receiverBalance, _ := walletCacheConfirmed.get(grape1crypto.BytesToAddress(payment.Tx.Recepient))
		// update sender's balance after sub operation
		senderBalance, _ = walletCacheConfirmed.get(grape1crypto.BytesToAddress(payment.Tx.Sender))
		balances[grape1crypto.BytesToAddress(payment.Tx.Sender)] = senderBalance.Bytes()
		balances[grape1crypto.BytesToAddress(payment.Tx.Recepient)] = receiverBalance.Bytes()
	}
	// peerConfig, not config.GetConfig(): the package caches the peer
	// configuration at Init and everything else here reads that copy. Reaching
	// for the global instead is a nil dereference on any path that runs before
	// or without a loaded configuration - which is how this function panicked
	// the first time a test drove a commit transaction through it.
	if peerConfig.NodeType == 0 {
		logger.Info("Dump new balances to vm state store")
		pin.runSmartContractStageFullNode(balances, recentPin)
	} else if peerConfig.NodeType == 1 {
		//here balances from diffs are dumping to vm.state store
		//We take balances from latest pin because peernode doesn't proceed smc
		vm.SyncBalances(balances)
		diffsPinBalances := make(map[string][]byte)
		diffsFromPin := recentPin.Diffs

		for _, diff := range diffsFromPin {
			if diff.GetAccountDiff() != nil {
				address := "0x" + hex.EncodeToString(diff.GetAccountDiff().NewValue.Address)
				acDiffBalance := new(big.Int).SetBytes(diff.GetAccountDiff().NewValue.Balance)
				walletCacheConfirmed.cache[address] = append(walletCacheConfirmed.cache[address], newPair[string, *big.Int]("sc", acDiffBalance))
				diffsPinBalances[address] = acDiffBalance.Bytes()
				cacheBalance, _ := walletCacheConfirmed.get(address)
				pinBalance := big.NewInt(0).SetBytes(recentPin.Balance.Balance[address])
				if acDiffBalance.Cmp(cacheBalance) != 0 || acDiffBalance.Cmp(pinBalance) != 0 {
					logger.Warnf("Sync at peernode is failed by address - %s,  now balance -  %s, in confirmed cache - %s, in received pin %s", address, acDiffBalance.String(), cacheBalance.String(), pinBalance)
				}
			}
			if diff.GetMappingDiff() != nil {
				vm.DumpMappingDiffs(diff)

			}
		}
		if len(diffsPinBalances) > 0 {
			vm.SyncBalances(diffsPinBalances)
		}

		pin.checkBalances(recentPin)

	}
}

func (pin *NodeTxPin) checkBalances(recentPin *pb.TxPin) error {
	for address, balanceBytes := range recentPin.Balance.Balance {
		cachedBalance, _ := walletCacheConfirmed.get(address)
		pinBalance := big.NewInt(0).SetBytes(balanceBytes)
		vmAcc := vm.SearchAccount(address)
		if vmAcc != nil {
			vmBalance := vmAcc.Balance
			if cachedBalance.Cmp(pinBalance) != 0 || pinBalance.Cmp(&vmBalance) != 0 {
				logger.Warnf("[Applied Pin Post-Check] Balances are different for address %s! In received pin - %d, in comfirmed cache - %d, stored in vm - %s", address, pinBalance.String(), cachedBalance.String(), &vmBalance)
				return errors.New("node sync failed")
			}
		} else {
			if cachedBalance.Cmp(pinBalance) != 0 {
				logger.Warnf("[Applied Pin Post-Check] Balances are different for address %s! In received pin - %d, in comfirmed cache - %d", address, pinBalance.String(), cachedBalance.String())
				return errors.New("node sync failed")

			}
		}
	}
	return nil
}

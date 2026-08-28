package dag

import (
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	txqueue "github.com/Grape-Chain/Grape-Dag/queues"
	"github.com/Grape-Chain/Grape-Dag/smc"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/vm"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/anypb"
)

/*
The live wiring for the validator protocol in dag/consensus.go.

The engine is written against an interface so that a whole validator set can be
run in one process on a clock the tests move by hand. This file is the other
implementation: the one where confirmedSites() reads the real tracker, buildPin
forms a real commit transaction, publish applies and announces it, and broadcast
puts a message on the sync topic every node is already subscribed to.

Two things here are not just plumbing.

Building a commit transaction has consequences before anyone has agreed to it.
The smart-contract stage executes the transactions it includes, moves them from
the unconfirmed pool to the confirmed one, and invalidates the wallet cache for
every account it touches. A proposer that builds a commit transaction and then
loses its round - because too few validators voted before the deadline - would
otherwise be left holding state no other node has: contracts executed against a
pin that does not exist. pinCandidate records what a build changed so it can be
undone, and the driver undoes it whenever a round ends without publishing.

Nothing is settled until the certificate is complete. The confirmed sites are
reported, not consumed, at the start of an epoch, so a round that fails leaves
them exactly where they were, to be reported again in the next one.
*/

// consensusRunner - the driver, when this node is a validator running the
// quorum protocol. nil on every other node, which is what turns the whole
// mechanism off.
var consensusRunner *consensusDriver

type consensusDriver struct {
	engine   *consensusEngine
	interval time.Duration

	mu sync.Mutex
	// candidate - the speculative work behind the commit transaction this node
	// proposed for the current round, held until the round is won or lost.
	candidate *pinCandidate
	// epochOpened - when the current epoch was opened, so a chain with nothing
	// to settle re-opens once per interval rather than once per tick.
	epochOpened time.Time
	// reported - when we last said what we hold confirmed, so the repeat is once
	// per commit interval rather than once per tick.
	reported time.Time
}

// pinCandidate - what building a commit transaction changed, and how to put it
// back.
type pinCandidate struct {
	epoch int64
	// smcTxs - the smart-contract transactions the build consumed. Undoing the
	// build means returning every one of them to the unconfirmed pool, whether
	// the stage executed it or discarded it as invalid: neither happened, as far
	// as the rest of the network is concerned.
	smcTxs []tx.Transaction
	// cache - the wallet cache as it was before the build invalidated entries.
	cache *WalletCache
	// vmMarked - whether a state-store checkpoint is outstanding.
	vmMarked bool
}

// newConsensusDriver - build the driver, or return nil if this node is not a
// validator running the quorum protocol.
func newConsensusDriver(interval time.Duration) (*consensusDriver, error) {
	if !pinAuth.quorumMode() {
		return nil, nil
	}
	set, quorum := pinAuth.validatorSet()
	if len(set) == 0 {
		return nil, errors.New("quorum mode with an empty validator set")
	}
	if dagWallet == nil {
		return nil, errors.New("a validator needs a wallet to sign with")
	}
	validators := make([]string, 0, len(set))
	for key := range set {
		validators = append(validators, key)
	}
	self := strings.ToLower(hex.EncodeToString(*dagWallet.PublicKey()))
	if _, ours := set[self]; !ours {
		// Not a validator: this node applies what the set agrees and takes no
		// part in agreeing it. That is the ordinary case for a processing node.
		return nil, nil
	}
	d := &consensusDriver{interval: interval}
	engine, err := newConsensusEngine(dagWallet, validators, quorum, d, interval)
	if err != nil {
		return nil, err
	}
	d.engine = engine
	return d, nil
}

// consensusActive - is this node running the validator protocol?
func consensusActive() bool { return consensusRunner != nil && consensusRunner.engine != nil }

// ------------------------------------------------------------------ the loop

// drive - one turn of the protocol. Called from the dag watcher's ticker, which
// is the same goroutine that would otherwise be forming commit transactions as
// a leader, so applying an agreed one here blocks nothing that was not already
// blocked in leader mode.
func (d *consensusDriver) drive() {
	if d == nil || d.engine == nil {
		return
	}
	next := int64(_pins_.CurrentHeight()) + 1
	epoch, _, phase, _, _ := d.engine.state()

	if epoch != next {
		// The chain moved on: either our commit transaction was applied or a
		// peer's was. Whatever we were building is for a number that is taken.
		d.rollbackCandidate("the chain moved on")
		d.openEpoch(next)
		return
	}
	if phase == phaseIdle || phase == phaseDone {
		// The epoch finished without moving the chain, so it settled nothing.
		// Re-open it, but no more often than the commit interval: an idle chain
		// should tick over at the commit cadence, not at the tick cadence.
		if time.Since(d.epochStarted()) < d.interval {
			return
		}
		d.rollbackCandidate("the round ended without a commit transaction")
		d.openEpoch(next)
		return
	}
	d.engine.tick()

	// Say again what we hold confirmed. Validators open an epoch when they see
	// the chain move, which they do at different moments, so a report sent once
	// at the open reaches only the validators that were already in the epoch.
	// See consensusEngine.rebroadcastReport.
	d.mu.Lock()
	due := time.Since(d.reported) >= d.interval
	if due {
		d.reported = time.Now()
	}
	d.mu.Unlock()
	if due {
		d.engine.rebroadcastReport()
	}
}

func (d *consensusDriver) openEpoch(epoch int64) {
	d.mu.Lock()
	d.epochOpened = time.Now()
	d.reported = d.epochOpened
	d.mu.Unlock()
	d.engine.startEpoch(epoch)
	// One line per commit interval, and the only place the protocol's state is
	// visible from outside. A quorum that never forms looks exactly like an idle
	// chain from the ledger alone; this says which it is.
	_, round, phase, reports, _ := d.engine.state()
	logger.Infof("[consensus] Epoch %d round %d open: %d site(s) held confirmed, %d report(s), proposer %s, phase %s",
		epoch, round, len(d.confirmedSites()), reports,
		shortKey(d.engine.proposerFor(epoch, round)), phase)
}

func (d *consensusDriver) epochStarted() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.epochOpened
}

// ------------------------------------------------------------- consensusNet

func (d *consensusDriver) now() time.Time { return time.Now() }

// confirmedSites - what this node holds confirmed, without consuming it.
func (d *consensusDriver) confirmedSites() []uuid.UUID {
	if _dag_ == nil {
		return nil
	}
	sites := _dag_.PeekConfirmedSites()
	out := make([]uuid.UUID, 0, len(sites))
	for _, s := range sites {
		if s != nil {
			out = append(out, s.id.id)
		}
	}
	return out
}

// buildPin - form the commit transaction this node is proposing, over exactly
// the sites the set agreed on. Speculative: see pinCandidate.
func (d *consensusDriver) buildPin(epoch int64, ids []uuid.UUID) (*pb.TxPin, error) {
	// Anything left from an earlier round of this epoch is dead now.
	d.rollbackCandidate("superseded by a new proposal")

	nodes := confirmedNodesFor(ids)
	if len(nodes) == 0 {
		return nil, errors.Errorf("none of the %d agreed site(s) are in this node's graph", len(ids))
	}
	if len(nodes) < len(ids) {
		// Settle what we hold rather than nothing. The agreed set is what a
		// quorum reported, which can name a site this node has not seen; every
		// site we do settle is still quorum-reported, so a smaller commit
		// transaction is safe and the rest wait for the next one. Refusing to
		// build unless we held every agreed site stopped the chain outright on a
		// four-validator network the moment one validator's view lagged.
		logger.Infof("[consensus] Epoch %d: settling %d of the %d agreed site(s); the rest are not in this node's graph yet",
			epoch, len(nodes), len(ids))
	}

	cand := &pinCandidate{epoch: epoch, cache: newWalletCache()}
	if walletCache != nil {
		_ = cand.cache.copyFrom(walletCache)
	}
	cand.smcTxs = smc.GetAllUncofirmed(int(config.GetConfig().Tx.Maxfuellimit))
	vm.Checkpoint()
	cand.vmMarked = true

	// The sites are serialised before any pin lock is taken, under the graph's
	// own read lock. ToPbNode reads each site's approval targets, its
	// settled-target ids, its height and its processor claim, and inserts append
	// to those while slicing rewrites them - so doing it inside the pin lock, as
	// this did, raced every insert on the node and could put a torn view of the
	// graph into a commit transaction that then gets signed and shipped.
	//
	// It cannot simply take the graph lock inside the pin lock instead: the
	// documented order is dag.mux first, and reversing it here is the deadlock
	// that was already fixed once. So the graph read happens first and finishes,
	// and the builder is handed finished messages.
	//
	// LockBuild is taken around the whole thing rather than just the pin lock,
	// because the smart-contract stage calls vm.CaptureStateStoreDiffs, which
	// panics if capture is already on - and two builders is reachable, since
	// commit transactions are formed both here and from genPinTx.
	_pins_.LockBuild()
	prepared := prepareSites(nodes)
	_pins_.LockPin()
	pin, err := _pins_.unsafe_buildPinPrepared(prepared, cand.smcTxs)
	_pins_.UnlockPin()
	_pins_.UnlockBuild()
	if err != nil {
		cand.undo()
		return nil, err
	}
	if pin.PinNumber != epoch {
		// The chain head moved while we were building. Proposing this would ask
		// the set to sign a commit transaction for a number that is taken.
		cand.undo()
		return nil, errors.Errorf("built commit transaction %d for epoch %d", pin.PinNumber, epoch)
	}

	d.mu.Lock()
	d.candidate = cand
	d.mu.Unlock()
	return pin, nil
}

// publish - the certificate is complete. Apply it here and announce it; every
// other node applies it through the ordinary announce path, which verifies the
// certificate before touching the ledger.
func (d *consensusDriver) publish(pin *pb.TxPin) error {
	if pin == nil {
		return errors.New("nothing to publish")
	}
	// The speculative work stands: keep it, and stop tracking it.
	d.keepCandidate()

	if !applyPin(pin) {
		return errors.Errorf("the agreed commit transaction %d was not applied", pin.PinNumber)
	}
	// Consume exactly what was settled. Slicing marks these harvested too, so
	// this is belt and braces on a node with dag.slicing off - and harmless
	// either way, because harvesting is idempotent.
	if _dag_ != nil {
		_dag_.TakeConfirmedSites(sitesOf(pin))
	}
	logger.Infof("[consensus] Published commit transaction %d agreed by %d validator(s)",
		pin.PinNumber, len(pin.GetQuorum().GetSignatures()))
	return announceNewPin()
}

// broadcast - put a consensus message on the sync topic.
//
// Called with the engine's lock held, so it enqueues and returns; the publisher
// goroutine takes it from there. Delivering it synchronously would re-enter the
// lock on the next hop.
func (d *consensusDriver) broadcast(env *pb.ConsensusEnvelope) error {
	wrapped, err := anypb.New(env)
	if err != nil {
		return errors.Wrap(err, "wrapping a consensus message")
	}
	host := grapepeer.GetHost()
	if host == nil {
		return errors.New("no host to send a consensus message from")
	}
	hostpk := host.Peerstore().PrivKey(host.ID())
	stdkey, err := crypto.PrivKeyToStdKey(hostpk)
	if err != nil {
		return errors.Wrap(err, "reading the host key")
	}
	pk, pubkey, err := crypto.KeyPairFromStdKey(stdkey)
	if err != nil {
		return errors.Wrap(err, "deriving the host key pair")
	}

	stx := tx.NewSyncv1()
	stx.Ver_type = tx.STVX1
	stx.Sync_Type = tx.LATEST
	stx.Msg_Type = tx.STX_CONSENSUS
	stx.Details = append(stx.Details, wrapped)
	stx.Sender_Pubk, err = crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return errors.Wrap(err, "marshaling the host public key")
	}
	stx.SyncHash = []byte{}
	stx.Signature = stx.GenerateSignature(pk)

	txqueue.GetSyncQueue().Enqueue(stx)
	return nil
}

// ------------------------------------------------------------------ receiving

// isConsensusMessage - a validator protocol message on the sync topic.
func isConsensusMessage(stx *tx.Syncv1) bool {
	return stx.Ver_type == tx.STVX1 && stx.Msg_Type == tx.STX_CONSENSUS
}

// handleConsensusMessage - hand a received message to the engine. A node that is
// not a validator ignores these: it learns what was agreed from the commit
// transaction itself, whose certificate it verifies.
func handleConsensusMessage(stx *tx.Syncv1) {
	if !consensusActive() {
		return
	}
	for _, detail := range stx.Details {
		if detail == nil {
			continue
		}
		env := &pb.ConsensusEnvelope{}
		if err := detail.UnmarshalTo(env); err != nil {
			logger.Warnf("[consensus] Unreadable consensus message: %s", err.Error())
			continue
		}
		if err := consensusRunner.engine.deliver(env); err != nil {
			// Refusals are the protocol working, not a fault: a validator
			// declining a proposal it cannot justify says so this way.
			logger.Debugf("[consensus] Refused a consensus message: %s", err.Error())
		}
	}
}

// --------------------------------------------------------------- speculation

// rollbackCandidate - undo a build whose round ended without publishing.
func (d *consensusDriver) rollbackCandidate(why string) {
	d.mu.Lock()
	cand := d.candidate
	d.candidate = nil
	d.mu.Unlock()
	if cand == nil {
		return
	}
	logger.Warnf("[consensus] Discarding the commit transaction built for epoch %d: %s", cand.epoch, why)
	cand.undo()
}

// keepCandidate - the round was won, so the build stands.
func (d *consensusDriver) keepCandidate() {
	d.mu.Lock()
	cand := d.candidate
	d.candidate = nil
	d.mu.Unlock()
	if cand == nil {
		return
	}
	cand.keep()
}

func (c *pinCandidate) keep() {
	if c.vmMarked {
		vm.DropCheckpoint()
		c.vmMarked = false
	}
}

func (c *pinCandidate) undo() {
	if c.vmMarked {
		vm.RevertCheckpoint()
		c.vmMarked = false
	}
	// Back to the unconfirmed pool, for whichever of these the stage executed
	// and whichever it discarded: from the network's point of view neither
	// happened.
	for _, t := range c.smcTxs {
		if t != nil {
			smc.AddUnconfirmed(t)
		}
	}
	if c.cache != nil && walletCache != nil {
		_ = walletCache.copyFrom(c.cache)
	}
}

// ------------------------------------------------------------------- helpers

// confirmedNodesFor - the sites behind these ids, without consuming them.
func confirmedNodesFor(ids []uuid.UUID) []*Node {
	if _dag_ == nil {
		return nil
	}
	want := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]*Node, 0, len(ids))
	for _, n := range _dag_.PeekConfirmedSites() {
		if n == nil {
			continue
		}
		if _, ok := want[n.id.id]; ok {
			out = append(out, n)
		}
	}
	return out
}

// sitesOf - the site ids a commit transaction settles.
func sitesOf(pin *pb.TxPin) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(pin.GetSites()))
	for _, s := range pin.GetSites() {
		if s == nil {
			continue
		}
		if id, err := uuid.FromBytes(s.Id); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// consensusTickInterval - how often to drive the protocol. The phases divide
// the commit interval, so the tick has to be a fraction of it or a phase
// deadline is noticed a whole interval late.
func consensusTickInterval() time.Duration {
	d := config.PIN_TX_TIMER_DEF / 10
	if d < 50*time.Millisecond {
		d = 50 * time.Millisecond
	}
	return d
}

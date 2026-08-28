package dag

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

/*
Agreeing a commit transaction, rather than being told one.

Until now a single node decided what the ledger had settled: it built a commit
transaction, signed it, and every other node applied it. The signer check added
alongside this makes that a named privilege rather than an open door, but it is
still one key deciding. This is the protocol that replaces the assertion with an
agreement.

The shape, per commit-transaction number ("epoch"), matching the technical
paper's t0-t4:

  t0  Everyone clears the previous epoch's state.

  t1  Every validator broadcasts the set of sites it believes is confirmed.

  t2  The settleable set is the sites at least a quorum of validators reported.
      That is the load-bearing rule: a proposer cannot settle a site the rest of
      the network has not confirmed, because the others will not vote for a
      proposal containing one they cannot justify from the reports they hold.

  t3  The round's proposer builds a commit transaction over that set and
      broadcasts it. Every validator checks it against its own reports and, if
      it agrees, signs the prototype hash and broadcasts the signature.

  t4  The proposer collects a quorum of signatures, attaches them as a
      QuorumCert, and publishes the commit transaction the ordinary way. Every
      node - validator or not - applies it because the certificate verifies,
      which is the check that already exists in dag/pinauth.go.

Two design points worth stating, because both are choices rather than
consequences:

  - Only the proposer assembles the certificate and publishes. Every validator
    could collect the votes and commit locally, which would be more robust, but
    validators would then hold commit transactions differing in which
    signatures they happened to collect - and the chain links each commit
    transaction to the previous one *by its signature*, so those nodes would
    diverge immediately. One publisher keeps the chain byte-identical
    everywhere. If the proposer dies mid-round, the view change below covers it.

  - Validators accept a proposal containing fewer sites than they would have
    included, but never one containing a site they cannot justify. Requiring the
    exact set would deadlock the moment two validators saw different reports,
    which is most of the time; the sites left out are simply settled by the next
    commit transaction.

The proposer for a round is sortedValidators[(epoch+round) % N], so a silent
proposer costs one round rather than a stall, and no election is needed to
recover from one.
*/

// consensusPhase - where an epoch has got to.
type consensusPhase uint8

const (
	// phaseIdle - between epochs; nothing to do until the next one opens.
	phaseIdle consensusPhase = iota
	// phaseCollecting - t1/t2: gathering what each validator says is confirmed.
	phaseCollecting
	// phaseVoting - t3/t4: a proposal is on the table and votes are arriving.
	phaseVoting
	// phaseDone - this epoch produced a commit transaction; wait for the next.
	phaseDone
)

func (p consensusPhase) String() string {
	return []string{"idle", "collecting", "voting", "done"}[p]
}

// consensusNet - everything the state machine needs from the world around it,
// injected so that a test can run a whole validator set in one process against
// a clock it controls. The protocol is the part worth testing exhaustively, and
// it cannot be tested through a real network at any useful rate.
type consensusNet interface {
	// broadcast - send to the other validators.
	//
	// Called with the engine's lock held, so it must not deliver synchronously
	// back into deliver() - on this engine or on any other that might answer.
	// A gossip topic publishes asynchronously and satisfies this naturally; the
	// test harness queues and drains for the same reason.
	broadcast(env *pb.ConsensusEnvelope) error
	// confirmedSites - what this node currently believes is confirmed and ready
	// to settle.
	confirmedSites() []uuid.UUID
	// buildPin - form a candidate commit transaction settling exactly these
	// sites, for this epoch.
	buildPin(epoch int64, sites []uuid.UUID) (*pb.TxPin, error)
	// publish - a commit transaction has been agreed: apply and announce it.
	publish(pin *pb.TxPin) error
	now() time.Time
}

// consensusTiming - how long each phase waits. Derived from the commit interval
// rather than configured separately, so the phases cannot add up to more than
// the interval they have to fit inside.
type consensusTiming struct {
	collect time.Duration // t1 -> t2
	vote    time.Duration // t3 -> t4
}

func timingFor(interval time.Duration) consensusTiming {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return consensusTiming{
		collect: interval / 5,
		vote:    2 * interval / 5,
	}
}

type consensusEngine struct {
	mu sync.Mutex

	self       *grape_wallet.Wallet
	selfKey    string   // hex of self's public key
	validators []string // hex public keys, sorted, so rotation is deterministic
	quorum     int
	net        consensusNet
	timing     consensusTiming

	epoch int64
	round uint32
	phase consensusPhase
	// deadline - when the current phase gives up.
	deadline time.Time

	// reports - epoch t1: which sites each validator says are confirmed.
	reports map[string]map[uuid.UUID]struct{}
	// proposal - the commit transaction under consideration, and its hash.
	proposal     *pb.TxPin
	proposalHash []byte
	// votes - signatures over proposalHash, by validator. Only the proposer
	// acts on these, but every validator collects them so that a validator
	// promoted by a view change is not starting from nothing.
	votes map[string]*pb.ValidatorSignature
	// viewChanges - validators that have given up on the current round.
	viewChanges map[string]struct{}

	// pending - messages for an epoch this node has not opened yet.
	//
	// Validators do not open an epoch at the same instant: whoever is first
	// broadcasts its confirmed set while the others are still finishing the
	// previous one, and a report dropped for being early is a validator missing
	// from the count that decides what may be settled. Holding them briefly
	// costs nothing and removes a whole class of wasted round.
	pending map[int64][]*pb.ConsensusEnvelope
}

// pendingEpochs - how many distinct future epochs to hold messages for.
//
// Bounded by count rather than by distance from the current epoch, because at
// start-up the current epoch is zero while the chain may be thousands in: a
// window of "within two of where I think I am" holds nothing at all exactly
// when it is needed most. The oldest held epoch is dropped when the count is
// exceeded, so a peer announcing ever-higher epochs evicts its own earlier
// noise rather than this node's memory.

const pendingEpochs = 4

// pendingPerEpoch - a cap, so that a peer cannot grow this node's memory by
// announcing a future epoch repeatedly.
const pendingPerEpoch = 64

func newConsensusEngine(self *grape_wallet.Wallet, validators []string, quorum int, net consensusNet, interval time.Duration) (*consensusEngine, error) {
	if self == nil {
		return nil, errors.New("a validator needs a wallet to sign with")
	}
	if len(validators) == 0 {
		return nil, errors.New("a validator set cannot be empty")
	}
	if quorum < 1 || quorum > len(validators) {
		return nil, errors.Errorf("a quorum of %d is not reachable in a set of %d", quorum, len(validators))
	}
	sorted := append([]string(nil), validators...)
	for i := range sorted {
		sorted[i] = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(sorted[i]), "0x"))
	}
	sort.Strings(sorted)
	return &consensusEngine{
		self:        self,
		selfKey:     strings.ToLower(hex.EncodeToString(*self.PublicKey())),
		validators:  sorted,
		quorum:      quorum,
		net:         net,
		timing:      timingFor(interval),
		phase:       phaseIdle,
		reports:     map[string]map[uuid.UUID]struct{}{},
		votes:       map[string]*pb.ValidatorSignature{},
		viewChanges: map[string]struct{}{},
		pending:     map[int64][]*pb.ConsensusEnvelope{},
	}, nil
}

// isValidator - is this key one of ours to listen to?
func (e *consensusEngine) isValidator(key string) bool {
	for _, v := range e.validators {
		if v == key {
			return true
		}
	}
	return false
}

// proposerFor - whose turn it is. Rotating on epoch+round means a silent
// proposer costs one round, and needs no election to replace.
func (e *consensusEngine) proposerFor(epoch int64, round uint32) string {
	n := int64(len(e.validators))
	idx := (epoch + int64(round)) % n
	if idx < 0 {
		idx += n
	}
	return e.validators[idx]
}

// startEpoch - t0 and t1: clear the previous epoch and say what we have.
func (e *consensusEngine) startEpoch(epoch int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.unsafeStartEpoch(epoch)
}

func (e *consensusEngine) unsafeStartEpoch(epoch int64) {
	e.epoch = epoch
	e.round = 0
	e.phase = phaseCollecting
	e.reports = map[string]map[uuid.UUID]struct{}{}
	e.votes = map[string]*pb.ValidatorSignature{}
	e.viewChanges = map[string]struct{}{}
	e.proposal, e.proposalHash = nil, nil
	e.deadline = e.net.now().Add(e.timing.collect)

	sites := e.net.confirmedSites()
	msg := &pb.ConfirmedSet{Epoch: epoch}
	for _, id := range sites {
		msg.SiteIds = append(msg.SiteIds, append([]byte(nil), id[:]...))
	}
	// Our own report counts, and is recorded here rather than waiting for our
	// own broadcast to come back to us.
	e.recordReport(e.selfKey, sites)
	if err := e.sign(&pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Confirmed{Confirmed: msg},
	}); err != nil {
		logger.Errorf("[consensus] Cannot sign our confirmed set for epoch %d: %s", epoch, err.Error())
	}

	// Anything that arrived for this epoch before we opened it counts now.
	held := e.pending[epoch]
	for older := range e.pending {
		if older <= epoch {
			delete(e.pending, older)
		}
	}
	for _, env := range held {
		key := strings.ToLower(hex.EncodeToString(env.Pk))
		if err := e.unsafeRoute(key, env); err != nil {
			logger.Debugf("[consensus] Held message for epoch %d from %s: %s", epoch, shortKey(key), err.Error())
		}
	}
}

// epochOf - which epoch a message concerns, and whether it names one at all.
func epochOf(env *pb.ConsensusEnvelope) (int64, bool) {
	switch p := env.Payload.(type) {
	case *pb.ConsensusEnvelope_Confirmed:
		return p.Confirmed.GetEpoch(), true
	case *pb.ConsensusEnvelope_Proposal:
		return p.Proposal.GetEpoch(), true
	case *pb.ConsensusEnvelope_Vote:
		return p.Vote.GetEpoch(), true
	case *pb.ConsensusEnvelope_ViewChange:
		return p.ViewChange.GetEpoch(), true
	}
	return 0, false
}

func (e *consensusEngine) recordReport(key string, sites []uuid.UUID) {
	set := make(map[uuid.UUID]struct{}, len(sites))
	for _, id := range sites {
		set[id] = struct{}{}
	}
	e.reports[key] = set
}

// agreedSites - the sites at least a quorum of validators reported, in a stable
// order so that every validator builds the same set from the same reports.
func (e *consensusEngine) agreedSites() []uuid.UUID {
	counts := map[uuid.UUID]int{}
	for _, set := range e.reports {
		for id := range set {
			counts[id]++
		}
	}
	out := make([]uuid.UUID, 0, len(counts))
	for id, n := range counts {
		if n >= e.quorum {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i][:], out[j][:]) < 0 })
	return out
}

// justified - can this site be settled on the evidence we hold? Used to check a
// proposal rather than to build one, so it asks only whether a quorum reported
// the site, not whether we would have included it.
func (e *consensusEngine) justified(id uuid.UUID) bool {
	n := 0
	for _, set := range e.reports {
		if _, ok := set[id]; ok {
			n++
		}
	}
	return n >= e.quorum
}

// sign - sign an envelope as this validator and broadcast it. Caller holds the
// lock.
func (e *consensusEngine) sign(env *pb.ConsensusEnvelope) error {
	env.Pk = *e.self.PublicKey()
	env.Sign = nil
	payload, err := consensusPayloadHash(env)
	if err != nil {
		return err
	}
	env.Sign = grape_wallet.NewDSA().Sign(*e.self.PrivateKey(), payload)
	return e.net.broadcast(env)
}

// consensusPayloadHash - the bytes a consensus message is signed over: the
// envelope with the signature cleared, deterministically encoded.
func consensusPayloadHash(env *pb.ConsensusEnvelope) ([]byte, error) {
	c, ok := proto.Clone(env).(*pb.ConsensusEnvelope)
	if !ok {
		return nil, errors.New("cannot copy the consensus envelope to hash it")
	}
	c.Sign = nil
	buf, err := proto.MarshalOptions{Deterministic: true}.Marshal(c)
	if err != nil {
		return nil, err
	}
	return utils.GetBuilder().Build(crypto.SHA256).Hash(buf), nil
}

// deliver - a consensus message arrived. Everything is checked before it is
// acted on: the sender has to be a validator and the signature has to verify,
// or the message is one an attacker wrote.
func (e *consensusEngine) deliver(env *pb.ConsensusEnvelope) error {
	if env == nil {
		return errors.New("empty consensus envelope")
	}
	key := strings.ToLower(hex.EncodeToString(env.Pk))
	if !e.isValidator(key) {
		return errors.Errorf("consensus message from %s, which is not a validator", shortKey(key))
	}
	if key == e.selfKey {
		// Our own message, arriving back through the topic. Everything we send is
		// accounted for locally at the point we send it - a topic that echoes to
		// the sender would otherwise re-enter the lock we are already holding,
		// and a topic that does not would leave us out of our own count.
		return nil
	}
	payload, err := consensusPayloadHash(env)
	if err != nil {
		return err
	}
	if !grape_wallet.NewDSA().Verify(grape_wallet.PublicKey(env.Pk), env.Sign, payload) {
		return errors.Errorf("consensus message from %s is not correctly signed", shortKey(key))
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Ahead of us: hold it until we open that epoch, rather than drop a
	// validator's contribution for arriving first.
	if epoch, named := epochOf(env); named && epoch > e.epoch {
		if len(e.pending[epoch]) < pendingPerEpoch {
			e.pending[epoch] = append(e.pending[epoch], env)
		}
		for len(e.pending) > pendingEpochs {
			oldest := int64(0)
			first := true
			for held := range e.pending {
				if first || held < oldest {
					oldest, first = held, false
				}
			}
			delete(e.pending, oldest)
		}
		return nil
	}
	return e.unsafeRoute(key, env)
}

// unsafeRoute - dispatch a message that has already been authenticated. Caller
// holds the lock.
func (e *consensusEngine) unsafeRoute(key string, env *pb.ConsensusEnvelope) error {
	switch p := env.Payload.(type) {
	case *pb.ConsensusEnvelope_Confirmed:
		return e.onConfirmedSet(key, p.Confirmed)
	case *pb.ConsensusEnvelope_Proposal:
		return e.onProposal(key, p.Proposal)
	case *pb.ConsensusEnvelope_Vote:
		return e.onVote(key, p.Vote)
	case *pb.ConsensusEnvelope_ViewChange:
		return e.onViewChange(key, p.ViewChange)
	}
	return errors.New("consensus envelope carries no payload")
}

func (e *consensusEngine) onConfirmedSet(key string, msg *pb.ConfirmedSet) error {
	if msg.Epoch != e.epoch || e.phase != phaseCollecting {
		// Not this epoch, or we have moved past collecting. Reports for an epoch
		// we have finished are not evidence for the next one.
		return nil
	}
	sites := make([]uuid.UUID, 0, len(msg.SiteIds))
	for _, raw := range msg.SiteIds {
		if id, err := uuid.FromBytes(raw); err == nil {
			sites = append(sites, id)
		}
	}
	e.recordReport(key, sites)
	// Everyone has spoken: no reason to wait out the rest of the window.
	if len(e.reports) >= len(e.validators) {
		e.unsafeOpenVoting()
	}
	return nil
}

// unsafeOpenVoting - t2 into t3. Caller holds the lock.
func (e *consensusEngine) unsafeOpenVoting() {
	if e.phase != phaseCollecting {
		return
	}
	e.phase = phaseVoting
	e.deadline = e.net.now().Add(e.timing.vote)
	if e.proposerFor(e.epoch, e.round) != e.selfKey {
		return
	}
	sites := e.agreedSites()
	if len(sites) == 0 {
		// Nothing a quorum agrees is settled. That is not a failure - it is an
		// epoch with nothing to commit - so there is no proposal and no view
		// change to make.
		e.phase = phaseDone
		return
	}
	pin, err := e.net.buildPin(e.epoch, sites)
	if err != nil || pin == nil {
		logger.Errorf("[consensus] Cannot build a commit transaction for epoch %d: %v", e.epoch, err)
		return
	}
	if err := e.sign(&pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Proposal{Proposal: &pb.PinProposal{
			Epoch: e.epoch, Round: e.round, Pin: pin,
		}},
	}); err != nil {
		logger.Errorf("[consensus] Cannot broadcast our proposal for epoch %d: %s", e.epoch, err.Error())
		return
	}
	if err := e.unsafeAcceptProposal(pin); err != nil {
		logger.Errorf("[consensus] Cannot vote for our own proposal for epoch %d: %s", e.epoch, err.Error())
	}
}

func (e *consensusEngine) onProposal(key string, msg *pb.PinProposal) error {
	if msg.Epoch != e.epoch || msg.Round != e.round {
		return nil
	}
	if want := e.proposerFor(e.epoch, e.round); key != want {
		return errors.Errorf("epoch %d round %d proposal came from %s, but %s is the proposer",
			e.epoch, e.round, shortKey(key), shortKey(want))
	}
	if e.phase == phaseCollecting {
		// The proposer got there before our own collection window closed. Its
		// proposal is the signal that t2 has passed.
		e.phase = phaseVoting
		e.deadline = e.net.now().Add(e.timing.vote)
	}
	if e.phase != phaseVoting || msg.Pin == nil {
		return nil
	}
	if msg.Pin.PinNumber != e.epoch {
		return errors.Errorf("proposal for epoch %d carries a commit transaction numbered %d", e.epoch, msg.Pin.PinNumber)
	}
	// The check that makes the quorum mean something: every site settled here has
	// to be one a quorum of validators reported, on our own evidence.
	for _, site := range msg.Pin.Sites {
		if site == nil {
			continue
		}
		id, err := uuid.FromBytes(site.Id)
		if err != nil {
			return errors.Errorf("proposal for epoch %d names a site with an unreadable id", e.epoch)
		}
		if !e.justified(id) {
			return errors.Errorf("proposal for epoch %d settles site %s, which fewer than %d validators reported",
				e.epoch, id.String(), e.quorum)
		}
	}

	return e.unsafeAcceptProposal(msg.Pin)
}

// unsafeAcceptProposal - take a proposal as the one under consideration, vote
// for it, and see whether that completes a quorum. The proposer runs this on its
// own proposal too, rather than waiting for it to come back through the topic.
// Caller holds the lock.
func (e *consensusEngine) unsafeAcceptProposal(pin *pb.TxPin) error {
	hash, err := pin.PrototypeHash()
	if err != nil {
		return err
	}
	e.proposal, e.proposalHash = pin, hash

	sig := grape_wallet.NewDSA().Sign(*e.self.PrivateKey(), hash)
	e.votes[e.selfKey] = &pb.ValidatorSignature{Pk: *e.self.PublicKey(), Sign: sig}
	if err := e.sign(&pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Vote{Vote: &pb.PinVote{
			Epoch: e.epoch, Round: e.round, PinHash: hash, Sign: sig,
		}},
	}); err != nil {
		return err
	}
	e.unsafeTryCommit()
	return nil
}

func (e *consensusEngine) onVote(key string, msg *pb.PinVote) error {
	if msg.Epoch != e.epoch || msg.Round != e.round || e.phase != phaseVoting {
		return nil
	}
	if e.proposalHash == nil || !bytes.Equal(msg.PinHash, e.proposalHash) {
		// A vote for something other than what we are looking at. Either we have
		// not seen the proposal yet, or this validator saw a different one -
		// neither is evidence for the commit transaction in hand.
		return nil
	}
	pk, err := hex.DecodeString(key)
	if err != nil {
		return err
	}
	if !grape_wallet.NewDSA().Verify(grape_wallet.PublicKey(pk), msg.Sign, e.proposalHash) {
		return errors.Errorf("vote from %s does not verify against the proposal", shortKey(key))
	}
	e.votes[key] = &pb.ValidatorSignature{Pk: pk, Sign: msg.Sign}
	e.unsafeTryCommit()
	return nil
}

// unsafeTryCommit - t4. Only the proposer publishes; see the file comment for
// why every validator does not. Caller holds the lock.
func (e *consensusEngine) unsafeTryCommit() {
	if e.phase != phaseVoting || e.proposal == nil || len(e.votes) < e.quorum {
		return
	}
	if e.proposerFor(e.epoch, e.round) != e.selfKey {
		// Not ours to publish. We keep the votes: a view change could make us
		// the proposer, and then we are not starting from nothing.
		return
	}
	cert := &pb.QuorumCert{
		PinNumber: e.proposal.PinNumber,
		PinHash:   append([]byte(nil), e.proposalHash...),
		Round:     e.round,
	}
	for _, key := range e.validators {
		if sig, ok := e.votes[key]; ok {
			cert.Signatures = append(cert.Signatures, sig)
		}
	}
	e.proposal.Quorum = cert
	e.phase = phaseDone
	if err := e.net.publish(e.proposal); err != nil {
		logger.Errorf("[consensus] Cannot publish the agreed commit transaction for epoch %d: %s", e.epoch, err.Error())
	}
}

func (e *consensusEngine) onViewChange(key string, msg *pb.ViewChange) error {
	if msg.Epoch != e.epoch || msg.Round != e.round {
		return nil
	}
	e.viewChanges[key] = struct{}{}
	if len(e.viewChanges) >= e.quorum {
		e.unsafeAdvanceRound()
	}
	return nil
}

// unsafeAdvanceRound - give up on this round's proposer and move to the next.
// Caller holds the lock.
func (e *consensusEngine) unsafeAdvanceRound() {
	if e.phase == phaseDone || e.phase == phaseIdle {
		return
	}
	e.round++
	e.phase = phaseVoting
	e.deadline = e.net.now().Add(e.timing.vote)
	e.votes = map[string]*pb.ValidatorSignature{}
	e.viewChanges = map[string]struct{}{}
	e.proposal, e.proposalHash = nil, nil
	logger.Warnf("[consensus] Epoch %d moving to round %d; the proposer is now %s",
		e.epoch, e.round, shortKey(e.proposerFor(e.epoch, e.round)))

	// The reports are still good - they are about what is confirmed, not about
	// who proposes - so the new proposer can build immediately.
	if e.proposerFor(e.epoch, e.round) != e.selfKey {
		return
	}
	sites := e.agreedSites()
	if len(sites) == 0 {
		e.phase = phaseDone
		return
	}
	pin, err := e.net.buildPin(e.epoch, sites)
	if err != nil || pin == nil {
		logger.Errorf("[consensus] Cannot build a commit transaction for epoch %d round %d: %v", e.epoch, e.round, err)
		return
	}
	if err := e.sign(&pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Proposal{Proposal: &pb.PinProposal{
			Epoch: e.epoch, Round: e.round, Pin: pin,
		}},
	}); err != nil {
		logger.Errorf("[consensus] Cannot broadcast our proposal for epoch %d round %d: %s", e.epoch, e.round, err.Error())
		return
	}
	if err := e.unsafeAcceptProposal(pin); err != nil {
		logger.Errorf("[consensus] Cannot vote for our own proposal for epoch %d round %d: %s", e.epoch, e.round, err.Error())
	}
}

// tick - drive the phase deadlines. Called from a ticker in production and
// directly from tests, which is why the clock is injected rather than read.
func (e *consensusEngine) tick() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.phase == phaseIdle || e.phase == phaseDone {
		return
	}
	if e.net.now().Before(e.deadline) {
		return
	}
	switch e.phase {
	case phaseCollecting:
		e.unsafeOpenVoting()
	case phaseVoting:
		if len(e.agreedSites()) == 0 {
			// Nothing a quorum agrees is settled, so there is nothing the
			// proposer can have failed to propose and no reason to replace it.
			// Without this an idle chain calls a view change every voting window
			// for ever, churning the proposer and logging a warning each time to
			// report that nothing happened. The phase is left alone so that a
			// proposal still arriving is still voted on.
			e.deadline = e.net.now().Add(e.timing.vote)
			return
		}
		// The round produced nothing in time. Say so; a quorum saying the same
		// moves everyone to the next proposer together.
		if _, said := e.viewChanges[e.selfKey]; !said {
			e.viewChanges[e.selfKey] = struct{}{}
			if err := e.sign(&pb.ConsensusEnvelope{
				Payload: &pb.ConsensusEnvelope_ViewChange{ViewChange: &pb.ViewChange{
					Epoch: e.epoch, Round: e.round,
				}},
			}); err != nil {
				logger.Errorf("[consensus] Cannot broadcast a view change for epoch %d: %s", e.epoch, err.Error())
			}
			if len(e.viewChanges) >= e.quorum {
				e.unsafeAdvanceRound()
			}
			return
		}
		// Already said it and still nothing: extend rather than spin, so a slow
		// network is not mistaken for a dead proposer over and over.
		e.deadline = e.net.now().Add(e.timing.vote)
	}
}

// state - for tests and diagnostics.
func (e *consensusEngine) state() (epoch int64, round uint32, phase consensusPhase, reports, votes int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.epoch, e.round, e.phase, len(e.reports), len(e.votes)
}

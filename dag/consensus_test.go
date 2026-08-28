package dag

import (
	"encoding/hex"
	"testing"
	"time"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// A whole validator set in one process, sharing a clock the test moves by hand.
//
// The protocol is the part worth testing exhaustively and the part a real
// network cannot test at any useful rate: a view change takes seconds of
// wall-clock time and depends on a node actually dying. Here it takes a
// function call.

// testCluster - N validators wired to each other, with delivery under the
// test's control so that partitions and silent nodes are just bookkeeping.
type testCluster struct {
	t        *testing.T
	wallets  []*grape_wallet.Wallet
	keys     []string
	engines  []*consensusEngine
	nets     []*testNet
	clock    time.Time
	interval time.Duration

	// queue - messages in flight. The engine calls broadcast with its lock
	// held, exactly as it does in production, so delivery has to happen after
	// that lock is released - a gossip topic publishes asynchronously and this
	// models that. Delivering synchronously deadlocks, which is worth knowing
	// rather than working around.
	queue []*pb.ConsensusEnvelope

	// published - agreed commit transactions, by whoever published them.
	published []*pb.TxPin
	// proposals - every proposal that went out, so a test can hand one to a
	// validator that did not hear it.
	proposals []*pb.ConsensusEnvelope
	// silent - validators that send nothing (a crashed proposer).
	silent map[int]bool
	// deaf - validators that receive nothing (a partitioned node).
	deaf map[int]bool
}

type testNet struct {
	cluster *testCluster
	index   int
	// confirmed - what this validator says is settled, so the test can give
	// different validators different views.
	confirmed []uuid.UUID
}

func (n *testNet) now() time.Time { return n.cluster.clock }

func (n *testNet) confirmedSites() []uuid.UUID { return n.confirmed }

func (n *testNet) broadcast(env *pb.ConsensusEnvelope) error {
	if n.cluster.silent[n.index] {
		return nil
	}
	if _, isProposal := env.Payload.(*pb.ConsensusEnvelope_Proposal); isProposal {
		// A copy, taken as it goes out. The engine keeps working on the same pin
		// afterwards - unsafeTryCommit attaches the certificate to it - and a
		// recorded pointer would show a message nobody sent, with a signature
		// that no longer matches. Production marshals the envelope inside
		// broadcast, so it takes its copy at the same moment.
		if sent, ok := proto.Clone(env).(*pb.ConsensusEnvelope); ok {
			n.cluster.proposals = append(n.cluster.proposals, sent)
		}
	}
	n.cluster.queue = append(n.cluster.queue, env)
	return nil
}

// lastProposal - the most recent proposal any validator sent.
func (c *testCluster) lastProposal() *pb.ConsensusEnvelope {
	c.t.Helper()
	if len(c.proposals) == 0 {
		c.t.Fatal("no proposal was made, so there is nothing to hand anyone")
	}
	return c.proposals[len(c.proposals)-1]
}

// drain - deliver everything in flight, and everything it causes, until the
// cluster goes quiet.
func (c *testCluster) drain() {
	for step := 0; len(c.queue) > 0; step++ {
		if step > 10000 {
			c.t.Fatal("the cluster never went quiet: messages are looping")
		}
		env := c.queue[0]
		c.queue = c.queue[1:]
		for i, e := range c.engines {
			if c.deaf[i] {
				continue
			}
			if err := e.deliver(env); err != nil {
				// Rejections are the protocol working - a validator refusing a
				// proposal it cannot justify - so they are logged, not fatal.
				c.t.Logf("validator %d rejected a message: %s", i, err.Error())
			}
		}
	}
}

func (n *testNet) buildPin(epoch int64, sites []uuid.UUID) (*pb.TxPin, error) {
	if n.cluster.silent[n.index] {
		return nil, errors.New("this validator is not answering")
	}
	return buildTestPin(epoch, sites), nil
}

// buildTestPin - the commit transaction a validator would build over this set.
// The injection tests write their own proposals with it, so what separates a
// forged proposal from a real one is only what the test changed, never an
// incidental difference in how the bytes were made.
func buildTestPin(epoch int64, sites []uuid.UUID) *pb.TxPin {
	pin := pb.NewTxPin([]byte{byte(epoch)})
	pin.PinNumber = epoch
	// A fixed timestamp: every validator has to be able to reproduce the exact
	// bytes, and a wall-clock reading would differ per node.
	pin.Ts = timestamppb.New(time.Unix(1700000000, 0))
	for _, id := range sites {
		pin.Sites = append(pin.Sites, &pb.SiteID{Id: append([]byte(nil), id[:]...)})
	}
	return pin
}

func (n *testNet) publish(pin *pb.TxPin) error {
	n.cluster.published = append(n.cluster.published, pin)
	return nil
}

func newTestCluster(t *testing.T, n int, sites []uuid.UUID) *testCluster {
	t.Helper()
	c := &testCluster{
		t:        t,
		clock:    time.Unix(1700000000, 0),
		interval: 5 * time.Second,
		silent:   map[int]bool{},
		deaf:     map[int]bool{},
	}
	for i := 0; i < n; i++ {
		w := grape_wallet.NewWallet()
		c.wallets = append(c.wallets, w)
		c.keys = append(c.keys, hexKey(w))
	}
	for i := 0; i < n; i++ {
		net := &testNet{cluster: c, index: i, confirmed: sites}
		c.nets = append(c.nets, net)
		e, err := newConsensusEngine(c.wallets[i], c.keys, quorumFor(n), net, c.interval)
		if err != nil {
			t.Fatalf("building validator %d: %s", i, err.Error())
		}
		c.engines = append(c.engines, e)
	}
	return c
}

func hexKey(w *grape_wallet.Wallet) string {
	return hex.EncodeToString(*w.PublicKey())
}

// startEpoch - open an epoch on every validator, in index order, then let the
// resulting messages flow. Validators opening an epoch at different moments is
// the normal case, not a contrivance.
func (c *testCluster) startEpoch(epoch int64) {
	for _, e := range c.engines {
		e.startEpoch(epoch)
		c.drain()
	}
	c.drain()
}

// advance - move the shared clock, let every validator notice, and deliver
// whatever that produces.
func (c *testCluster) advance(d time.Duration) {
	c.clock = c.clock.Add(d)
	for _, e := range c.engines {
		e.tick()
		c.drain()
	}
	c.drain()
}

// indexOfProposer - which validator proposes for this epoch and round.
func (c *testCluster) indexOfProposer(epoch int64, round uint32) int {
	want := c.engines[0].proposerFor(epoch, round)
	for i, k := range c.keys {
		if k == want {
			return i
		}
	}
	c.t.Fatalf("the proposer for epoch %d round %d is not in the set", epoch, round)
	return -1
}

func testSites(n int) []uuid.UUID {
	out := make([]uuid.UUID, n)
	for i := range out {
		out[i] = uuid.New()
	}
	return out
}

// sign - an envelope as validator i would have sent it. The tests that inject
// messages have to write ones the engine cannot tell from real, or they prove
// nothing about the checks that run after the signature.
func (c *testCluster) sign(i int, env *pb.ConsensusEnvelope) *pb.ConsensusEnvelope {
	c.t.Helper()
	env.Pk = *c.wallets[i].PublicKey()
	env.Sign = nil
	payload, err := consensusPayloadHash(env)
	if err != nil {
		c.t.Fatalf("hashing an envelope for validator %d: %s", i, err.Error())
	}
	env.Sign = grape_wallet.NewDSA().Sign(*c.wallets[i].PrivateKey(), payload)
	return env
}

// deliverVote - hand one validator a vote from another, with every field the
// test's to set.
func (c *testCluster) deliverVote(to, from int, epoch int64, round uint32, pinHash, sign []byte) error {
	c.t.Helper()
	return c.engines[to].deliver(c.sign(from, &pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Vote{Vote: &pb.PinVote{
			Epoch: epoch, Round: round, PinHash: pinHash, Sign: sign,
		}},
	}))
}

// deafenAllBut - every validator but one receives nothing. The one left hearing
// makes its proposal and draws no votes for it, so the vote count is entirely
// the test's to set.
func (c *testCluster) deafenAllBut(i int) {
	for j := range c.engines {
		if j != i {
			c.deaf[j] = true
		}
	}
}

// proposalHash - what this validator is currently voting on.
func (c *testCluster) proposalHash(i int) []byte {
	c.t.Helper()
	c.engines[i].mu.Lock()
	defer c.engines[i].mu.Unlock()
	if c.engines[i].proposalHash == nil {
		c.t.Fatalf("validator %d has no proposal in hand to vote on", i)
	}
	return append([]byte(nil), c.engines[i].proposalHash...)
}

// votesHeld - how many votes a validator has collected. A validator that
// accepted a proposal has at least its own.
func (c *testCluster) votesHeld(i int) int {
	_, _, _, _, votes := c.engines[i].state()
	return votes
}

// ---------------------------------------------------------------- happy path

func TestConsensusAgreesACommitTransaction(t *testing.T) {
	sites := testSites(4)
	c := newTestCluster(t, 4, sites)

	c.startEpoch(7)
	// Everyone reported, so collection closes without waiting for the deadline
	// and the proposer proposes; votes follow immediately.
	c.advance(c.interval / 4)

	if len(c.published) != 1 {
		t.Fatalf("expected exactly one agreed commit transaction, got %d", len(c.published))
	}
	pin := c.published[0]
	if pin.PinNumber != 7 {
		t.Fatalf("agreed commit transaction is numbered %d, want 7", pin.PinNumber)
	}
	if len(pin.Sites) != len(sites) {
		t.Fatalf("agreed commit transaction settles %d sites, want %d", len(pin.Sites), len(sites))
	}

	// And it carries a certificate the ordinary verification accepts.
	validators := map[string]struct{}{}
	for _, k := range c.keys {
		validators[k] = struct{}{}
	}
	if err := pin.VerifyQuorum(validators, quorumFor(4)); err != nil {
		t.Fatalf("the agreed commit transaction does not carry a valid quorum: %s", err.Error())
	}
}

// Only the proposer publishes. If every validator published its own, they would
// hold commit transactions differing in which signatures they collected - and
// the chain links each one to the previous by its signature, so the nodes would
// diverge on the next commit.
func TestOnlyTheProposerPublishes(t *testing.T) {
	c := newTestCluster(t, 4, testSites(3))
	c.startEpoch(1)
	c.advance(c.interval / 4)

	if len(c.published) != 1 {
		t.Fatalf("expected one publication, got %d", len(c.published))
	}
}

// The rotation has to be a function of epoch and round only, so that every
// validator agrees on whose turn it is without exchanging anything.
func TestProposerRotatesDeterministically(t *testing.T) {
	c := newTestCluster(t, 4, testSites(1))
	seen := map[string]bool{}
	for epoch := int64(0); epoch < 4; epoch++ {
		want := c.engines[0].proposerFor(epoch, 0)
		for i, e := range c.engines {
			if got := e.proposerFor(epoch, 0); got != want {
				t.Fatalf("validator %d thinks epoch %d's proposer is %s, validator 0 thinks %s",
					i, epoch, shortKey(got), shortKey(want))
			}
		}
		seen[want] = true
	}
	if len(seen) != 4 {
		t.Fatalf("four consecutive epochs should rotate through all four validators, got %d distinct", len(seen))
	}
	// And a round bump moves it on, which is what makes a view change useful.
	if c.engines[0].proposerFor(5, 0) == c.engines[0].proposerFor(5, 1) {
		t.Fatal("the next round has the same proposer, so a view change would change nothing")
	}
}

// ---------------------------------------------------------------- view change

// A proposer that says nothing must cost one round, not the epoch.
func TestASilentProposerIsReplaced(t *testing.T) {
	sites := testSites(3)
	c := newTestCluster(t, 4, sites)

	const epoch = 2
	c.silent[c.indexOfProposer(epoch, 0)] = true

	c.startEpoch(epoch)
	// Collection closes; the proposer for round 0 says nothing.
	c.advance(c.interval / 4)
	if len(c.published) != 0 {
		t.Fatal("a silent proposer still produced a commit transaction")
	}

	// The voting window expires, a quorum calls a view change, and the next
	// proposer takes over.
	c.advance(c.interval)
	if len(c.published) != 1 {
		t.Fatalf("expected the next proposer to publish, got %d publication(s)", len(c.published))
	}
	if got := c.published[0].Quorum.Round; got != 1 {
		t.Fatalf("the agreed commit transaction was reached in round %d, want 1", got)
	}
	if len(c.published[0].Sites) != len(sites) {
		t.Fatal("the replacement proposer settled a different set of sites")
	}
}

// Two silent proposers in a row cost two rounds, not the epoch.
//
// Seven validators, not four: the quorum is two thirds plus one, so a set of
// four tolerates a single failure and two silent members is beyond what any
// protocol of this shape can survive. Asking four validators to route around two
// failures is asking for the impossible, and a test that demanded it would be
// demanding a broken quorum.
func TestConsecutiveSilentProposersAreBothReplaced(t *testing.T) {
	c := newTestCluster(t, 7, testSites(2))
	const epoch = 3
	c.silent[c.indexOfProposer(epoch, 0)] = true
	c.silent[c.indexOfProposer(epoch, 1)] = true

	c.startEpoch(epoch)
	c.advance(c.interval / 4)
	c.advance(c.interval) // round 0 -> 1
	c.advance(c.interval) // round 1 -> 2

	if len(c.published) != 1 {
		t.Fatalf("expected one publication after two view changes, got %d", len(c.published))
	}
	if got := c.published[0].Quorum.Round; got != 2 {
		t.Fatalf("agreement was reached in round %d, want 2", got)
	}
}

// ---------------------------------------------------------------- safety

// The rule that makes the quorum worth having: a proposer cannot settle a site
// the rest of the network has not confirmed. Here the proposer alone claims an
// extra site, and the others refuse to vote for it.
func TestAProposerCannotSettleASiteNobodyElseConfirmed(t *testing.T) {
	shared := testSites(2)
	c := newTestCluster(t, 4, shared)

	const epoch = 1
	proposer := c.indexOfProposer(epoch, 0)
	// Only the proposer claims this one.
	invented := uuid.New()
	c.nets[proposer].confirmed = append(append([]uuid.UUID{}, shared...), invented)

	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	if len(c.published) != 1 {
		t.Fatalf("expected the honest sites to still be agreed, got %d publication(s)", len(c.published))
	}
	for _, s := range c.published[0].Sites {
		if id, err := uuid.FromBytes(s.Id); err == nil && id == invented {
			t.Fatal("a site only the proposer confirmed was settled")
		}
	}
	if len(c.published[0].Sites) != len(shared) {
		t.Fatalf("expected the %d shared sites, got %d", len(shared), len(c.published[0].Sites))
	}
}

// A validator's view being behind must not stop agreement: the sites it has not
// seen are simply left for the next commit transaction.
func TestAValidatorBehindTheOthersDoesNotBlockAgreement(t *testing.T) {
	shared := testSites(3)
	c := newTestCluster(t, 4, shared)
	// One validator has seen only the first site.
	c.nets[3].confirmed = shared[:1]

	c.startEpoch(1)
	c.advance(c.interval / 4)

	if len(c.published) != 1 {
		t.Fatalf("expected agreement despite one validator being behind, got %d", len(c.published))
	}
	// A quorum of 3 of 4 saw all three sites, so all three are settleable.
	if len(c.published[0].Sites) != 3 {
		t.Fatalf("expected all three sites to be agreed by the other three validators, got %d",
			len(c.published[0].Sites))
	}
}

// Below a quorum of participants, nothing is agreed. Producing a commit
// transaction with fewer would be worse than producing none.
func TestBelowAQuorumNothingIsAgreed(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	// Two of four unreachable leaves two, and the quorum is three.
	c.deaf[2], c.deaf[3] = true, true
	c.silent[2], c.silent[3] = true, true

	c.startEpoch(1)
	c.advance(c.interval / 4)
	c.advance(c.interval)
	c.advance(c.interval)

	if len(c.published) != 0 {
		t.Fatalf("a commit transaction was agreed by fewer than a quorum: %d published", len(c.published))
	}
}

// A message from outside the validator set is not evidence, however well formed.
func TestConsensusIgnoresNonValidators(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	c.startEpoch(1)

	outsider := grape_wallet.NewWallet()
	env := &pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Confirmed{Confirmed: &pb.ConfirmedSet{
			Epoch:   1,
			SiteIds: [][]byte{uuid.New().NodeID()},
		}},
	}
	env.Pk = *outsider.PublicKey()
	payload, err := consensusPayloadHash(env)
	if err != nil {
		t.Fatalf("hashing: %s", err.Error())
	}
	env.Sign = grape_wallet.NewDSA().Sign(*outsider.PrivateKey(), payload)

	if err := c.engines[0].deliver(env); err == nil {
		t.Fatal("a consensus message from outside the validator set was accepted")
	}
}

// A message claiming to be from a validator, but not signed by it, is a forgery.
func TestConsensusRejectsAForgedSenderIdentity(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	c.startEpoch(1)

	forger := grape_wallet.NewWallet()
	env := &pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_ViewChange{ViewChange: &pb.ViewChange{Epoch: 1, Round: 0}},
	}
	// Claims a real validator's key, signed with someone else's.
	env.Pk = *c.wallets[1].PublicKey()
	payload, err := consensusPayloadHash(env)
	if err != nil {
		t.Fatalf("hashing: %s", err.Error())
	}
	env.Sign = grape_wallet.NewDSA().Sign(*forger.PrivateKey(), payload)

	if err := c.engines[0].deliver(env); err == nil {
		t.Fatal("a consensus message signed by someone other than the key it claims was accepted")
	}
}

// A proposal is legitimate only from the validator whose turn it is. The set
// here is the one every validator reported, so nothing but whose turn it is
// separates this proposal from one that should be voted for - which is the only
// way to know the rotation check is what refuses it.
func TestAProposalFromTheWrongValidatorIsRefused(t *testing.T) {
	shared := testSites(2)
	c := newTestCluster(t, 4, shared)
	const epoch = 1

	proposer := c.indexOfProposer(epoch, 0)
	// The validator whose turn it is says nothing, so the only proposal in the
	// cluster is the impostor's. With the real one also circulating, a validator
	// that wrongly accepted this one would look just like one that refused it.
	c.silent[proposer] = true
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	impostor := (proposer + 1) % 4
	target := (proposer + 2) % 4

	env := c.sign(impostor, &pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Proposal{Proposal: &pb.PinProposal{
			Epoch: epoch, Round: 0, Pin: buildTestPin(epoch, shared),
		}},
	})
	if err := c.engines[target].deliver(env); err == nil {
		t.Fatal("a proposal from a validator whose turn it was not was accepted")
	}
	if votes := c.votesHeld(target); votes != 0 {
		t.Fatalf("the receiving validator voted for it anyway (%d vote(s) held)", votes)
	}
}

// The rule that makes the quorum worth having, tested against a proposer that
// is dishonest rather than merely mistaken: it hand-builds a commit transaction
// over a site short of a quorum of reports, instead of the one the protocol
// would have built for it. Two of the four validators reported the site, so a
// check that merely asks whether anyone reported it would let this through.
func TestAProposalSettlingAnUnderreportedSiteIsRefused(t *testing.T) {
	shared := testSites(2)
	c := newTestCluster(t, 4, shared)
	const epoch = 1

	proposer := c.indexOfProposer(epoch, 0)
	underreported := uuid.New()
	extra := append(append([]uuid.UUID{}, shared...), underreported)
	c.nets[(proposer+1)%4].confirmed = extra
	c.nets[(proposer+3)%4].confirmed = extra

	// Silent, so the honest proposal it would otherwise make is not there to be
	// voted for and confuse the count.
	c.silent[proposer] = true
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	target := (proposer + 2) % 4
	env := c.sign(proposer, &pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Proposal{Proposal: &pb.PinProposal{
			Epoch: epoch, Round: 0, Pin: buildTestPin(epoch, extra),
		}},
	})
	if err := c.engines[target].deliver(env); err == nil {
		t.Fatalf("a proposal settling a site only %d of %d validators reported was accepted",
			2, len(c.engines))
	}
	if votes := c.votesHeld(target); votes != 0 {
		t.Fatalf("the receiving validator voted for it anyway (%d vote(s) held)", votes)
	}
}

// A message naming a round the receiver is not in is ignored rather than
// refused - it is a message out of step, not misbehaviour - but it must not be
// acted on. Here the round-0 proposer claims round 1, which belongs to someone
// else, so only the round field stands between this and a proposal that would
// be voted for.
func TestAProposalForAnotherRoundIsIgnored(t *testing.T) {
	shared := testSites(2)
	c := newTestCluster(t, 4, shared)
	const epoch = 1

	proposer := c.indexOfProposer(epoch, 0)
	c.silent[proposer] = true
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	target := (proposer + 1) % 4
	env := c.sign(proposer, &pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Proposal{Proposal: &pb.PinProposal{
			Epoch: epoch, Round: 1, Pin: buildTestPin(epoch, shared),
		}},
	})
	if err := c.engines[target].deliver(env); err != nil {
		t.Fatalf("a proposal for another round should be ignored, not refused: %s", err.Error())
	}
	if votes := c.votesHeld(target); votes != 0 {
		t.Fatalf("voted for a proposal naming a round the validator is not in (%d vote(s) held)", votes)
	}
}

// Injecting votes has to be able to complete a quorum, or the three tests below
// would pass simply because nothing the test writes ever counts. This is the
// control that shows they are testing the checks and not the harness.
func TestInjectedVotesCompleteTheQuorum(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	const epoch = 1
	proposer := c.indexOfProposer(epoch, 0)

	// Everyone still reports, so a proposal forms; nobody but the proposer hears
	// it, so the only vote it holds is its own.
	c.deafenAllBut(proposer)
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	hash := c.proposalHash(proposer)
	if len(c.published) != 0 {
		t.Fatalf("published on the proposer's own vote alone, got %d publication(s)", len(c.published))
	}

	for _, i := range []int{(proposer + 1) % 4, (proposer + 2) % 4} {
		sign := grape_wallet.NewDSA().Sign(*c.wallets[i].PrivateKey(), hash)
		if err := c.deliverVote(proposer, i, epoch, 0, hash, sign); err != nil {
			t.Fatalf("a genuine vote from validator %d was refused: %s", i, err.Error())
		}
	}
	if len(c.published) != 1 {
		t.Fatalf("two genuine votes did not complete the quorum, got %d publication(s)", len(c.published))
	}

	validators := map[string]struct{}{}
	for _, k := range c.keys {
		validators[k] = struct{}{}
	}
	if err := c.published[0].VerifyQuorum(validators, quorumFor(4)); err != nil {
		t.Fatalf("the certificate built from those votes does not verify: %s", err.Error())
	}
}

// Votes are counted against the proposal in hand. The signature here is over
// the real proposal and verifies, so only the hash the vote names is wrong: a
// validator's signature must not be counted towards a commit transaction its
// message did not name.
func TestAVoteForADifferentProposalDoesNotCount(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	const epoch = 1
	proposer := c.indexOfProposer(epoch, 0)

	c.deafenAllBut(proposer)
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	hash := c.proposalHash(proposer)
	other := append([]byte(nil), hash...)
	other[0] ^= 0xff

	for _, i := range []int{(proposer + 1) % 4, (proposer + 2) % 4} {
		sign := grape_wallet.NewDSA().Sign(*c.wallets[i].PrivateKey(), hash)
		_ = c.deliverVote(proposer, i, epoch, 0, other, sign)
	}
	if len(c.published) != 0 {
		t.Fatalf("votes naming a different commit transaction completed the quorum (%d publication(s))",
			len(c.published))
	}
}

// The vote's own signature is what a certificate is made of. One that does not
// verify is not an endorsement of anything, and counting it would put a
// signature into the certificate that no later verifier will accept.
func TestAVoteWhoseSignatureDoesNotVerifyIsRefused(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	const epoch = 1
	proposer := c.indexOfProposer(epoch, 0)

	c.deafenAllBut(proposer)
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	hash := c.proposalHash(proposer)
	other := append([]byte(nil), hash...)
	other[0] ^= 0xff

	// The right hash is named, but signed over something else.
	for _, i := range []int{(proposer + 1) % 4, (proposer + 2) % 4} {
		sign := grape_wallet.NewDSA().Sign(*c.wallets[i].PrivateKey(), other)
		if err := c.deliverVote(proposer, i, epoch, 0, hash, sign); err == nil {
			t.Fatalf("a vote from validator %d whose signature does not verify was accepted", i)
		}
	}
	if len(c.published) != 0 {
		t.Fatalf("unverifiable votes completed the quorum (%d publication(s))", len(c.published))
	}
}

// Votes are scoped to a round. A round that produced nothing must not have its
// signatures harvested into the next round's certificate, so a vote naming a
// round the validator is not in is ignored however well formed it is.
func TestAVoteFromAnotherRoundDoesNotCount(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	const epoch = 1
	proposer := c.indexOfProposer(epoch, 0)

	c.deafenAllBut(proposer)
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	hash := c.proposalHash(proposer)
	for _, i := range []int{(proposer + 1) % 4, (proposer + 2) % 4} {
		sign := grape_wallet.NewDSA().Sign(*c.wallets[i].PrivateKey(), hash)
		if err := c.deliverVote(proposer, i, epoch, 1, hash, sign); err != nil {
			t.Fatalf("a vote for another round should be ignored, not refused: %s", err.Error())
		}
	}
	if len(c.published) != 0 {
		t.Fatalf("votes cast in another round completed this one's quorum (%d publication(s))",
			len(c.published))
	}
}

// An epoch in which a quorum agrees nothing is settled produces no commit
// transaction, and no view change either - there is nothing wrong.
func TestAnEmptyEpochProducesNothing(t *testing.T) {
	c := newTestCluster(t, 4, nil)
	c.startEpoch(1)
	c.advance(c.interval / 4)
	c.advance(c.interval)

	if len(c.published) != 0 {
		t.Fatalf("an epoch with nothing confirmed produced %d commit transaction(s)", len(c.published))
	}
	if _, round, _, _, _ := c.engines[0].state(); round != 0 {
		t.Fatalf("an epoch with nothing to settle triggered a view change: round %d", round)
	}
}

// Consecutive epochs, to check nothing carries over: the reports, votes and
// view changes of one epoch must not count towards the next.
func TestStateDoesNotCarryBetweenEpochs(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	c.startEpoch(1)
	c.advance(c.interval / 4)
	if len(c.published) != 1 {
		t.Fatalf("epoch 1 produced %d commit transactions", len(c.published))
	}

	// A fresh set of sites for the next epoch.
	next := testSites(3)
	for _, n := range c.nets {
		n.confirmed = next
	}
	c.startEpoch(2)
	c.advance(c.interval / 4)

	if len(c.published) != 2 {
		t.Fatalf("epoch 2 did not produce a commit transaction: %d total", len(c.published))
	}
	second := c.published[1]
	if second.PinNumber != 2 {
		t.Fatalf("the second commit transaction is numbered %d", second.PinNumber)
	}
	if len(second.Sites) != 3 {
		t.Fatalf("the second commit transaction settles %d sites, want 3", len(second.Sites))
	}
	if got := second.Quorum.Round; got != 0 {
		t.Fatalf("the second epoch started at round %d, so state carried over", got)
	}
}

// ------------------------------------------------------- evidence

// A validator judges a proposal by counting, in the reports it holds, how many
// validators reported each site. Counting only the reports that happened to
// arrive makes the verdict depend on delivery order, and with no slack in the
// quorum - three live validators and a quorum of three - a single report still
// in flight makes a receiver refuse a perfectly justified proposal. Observed on
// a four-validator network: killing one validator livelocked the chain through
// a hundred view changes without a single commit transaction.
//
// So a proposal carries the reports it was built from.
func TestAProposalCarriesTheEvidenceThatJustifiesIt(t *testing.T) {
	shared := testSites(3)
	c := newTestCluster(t, 4, shared)
	const epoch = 1

	proposer := c.indexOfProposer(epoch, 0)
	// This one hears nothing at all, so the only evidence it can possibly have
	// is what the proposal brings with it.
	isolated := (proposer + 1) % 4
	c.deaf[isolated] = true

	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	env := c.lastProposal()
	carried := env.GetProposal().GetReports()
	if len(carried) < quorumFor(4) {
		t.Fatalf("the proposal carries %d report(s), fewer than the quorum of %d it needs to justify itself",
			len(carried), quorumFor(4))
	}
	if _, _, _, reports, _ := c.engines[isolated].state(); reports > 1 {
		t.Fatalf("the isolated validator heard %d reports, so it is not isolated", reports)
	}

	if err := c.engines[isolated].deliver(env); err != nil {
		t.Fatalf("a validator that heard no reports refused a proposal that carried them: %s", err.Error())
	}
	if votes := c.votesHeld(isolated); votes == 0 {
		t.Fatal("the isolated validator did not vote for a proposal whose evidence it could check")
	}
}

// Carrying the evidence adds no trust: every report is checked against the
// validator set and its own signature before it is counted. A proposer cannot
// manufacture agreement by attaching reports its peers never wrote.
func TestAProposalCarryingForgedEvidenceIsStillRefused(t *testing.T) {
	shared := testSites(3)
	c := newTestCluster(t, 4, shared)
	const epoch = 1

	proposer := c.indexOfProposer(epoch, 0)
	isolated := (proposer + 1) % 4
	c.deaf[isolated] = true

	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	// The same proposal, with every carried report's signature broken.
	env, ok := proto.Clone(c.lastProposal()).(*pb.ConsensusEnvelope)
	if !ok {
		t.Fatal("cannot copy the proposal")
	}
	for _, report := range env.GetProposal().GetReports() {
		report.Sign[0] ^= 0xff
	}
	// Re-sign the envelope itself, so what fails is the evidence and not the
	// message carrying it.
	c.sign(proposer, env)

	if err := c.engines[isolated].deliver(env); err == nil {
		t.Fatal("a proposal justified only by forged reports was accepted")
	}
	if votes := c.votesHeld(isolated); votes != 0 {
		t.Fatalf("the validator voted on forged evidence (%d vote(s) held)", votes)
	}
}

// A report says "these sites are confirmed", and within an epoch that stays
// true. Replacing a validator's report with its next one would make its
// contribution jump around with the timing of its repeats, so the same site
// could be counted at the proposer and not at a receiver purely because their
// copies were taken at different moments.
func TestReportsAccumulateRatherThanReplace(t *testing.T) {
	c := newTestCluster(t, 4, nil)
	const epoch = 1
	c.startEpoch(epoch)

	first := testSites(2)
	second := testSites(2)
	sender := 1
	target := 0

	for _, batch := range [][]uuid.UUID{first, second} {
		msg := &pb.ConfirmedSet{Epoch: epoch}
		for _, id := range batch {
			msg.SiteIds = append(msg.SiteIds, append([]byte(nil), id[:]...))
		}
		env := c.sign(sender, &pb.ConsensusEnvelope{
			Payload: &pb.ConsensusEnvelope_Confirmed{Confirmed: msg},
		})
		if err := c.engines[target].deliver(env); err != nil {
			t.Fatalf("delivering a report: %s", err.Error())
		}
	}

	c.engines[target].mu.Lock()
	held := len(c.engines[target].reports[c.keys[sender]])
	c.engines[target].mu.Unlock()
	if held != len(first)+len(second) {
		t.Fatalf("the validator holds %d site(s) for a peer that reported %d then %d: the second report replaced the first",
			held, len(first), len(second))
	}
}

// A report that arrives after voting opened is still a true statement about
// what that validator holds confirmed. Refusing it only makes this node's
// evidence differ from everyone else's, which is the thing that stops the chain.
func TestAReportIsStillCountedAfterVotingOpens(t *testing.T) {
	c := newTestCluster(t, 4, testSites(2))
	const epoch = 1
	proposer := c.indexOfProposer(epoch, 0)
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	if _, _, phase, _, _ := c.engines[proposer].state(); phase == phaseCollecting {
		t.Fatal("voting never opened, so the test proves nothing")
	}

	late := testSites(1)
	msg := &pb.ConfirmedSet{Epoch: epoch}
	msg.SiteIds = append(msg.SiteIds, append([]byte(nil), late[0][:]...))
	sender := (proposer + 1) % 4
	env := c.sign(sender, &pb.ConsensusEnvelope{
		Payload: &pb.ConsensusEnvelope_Confirmed{Confirmed: msg},
	})
	if err := c.engines[proposer].deliver(env); err != nil {
		t.Fatalf("delivering a late report: %s", err.Error())
	}

	c.engines[proposer].mu.Lock()
	_, counted := c.engines[proposer].reports[c.keys[sender]][late[0]]
	c.engines[proposer].mu.Unlock()
	if !counted {
		t.Fatal("a report that arrived after voting opened was thrown away")
	}
}

// indexOfValidator - which validator a public key belongs to, or -1.
func (c *testCluster) indexOfValidator(pk []byte) int {
	want := hex.EncodeToString(pk)
	for i, k := range c.keys {
		if k == want {
			return i
		}
	}
	return -1
}

// isolatedProposal - a proposal, and a validator that heard nothing else, so
// what the proposal carries is the only evidence in play. The tests below
// damage the carried reports in one way each and check that the damage is
// noticed.
func isolatedProposal(t *testing.T) (*testCluster, int, *pb.ConsensusEnvelope) {
	t.Helper()
	c := newTestCluster(t, 4, testSites(3))
	const epoch = 1
	proposer := c.indexOfProposer(epoch, 0)
	isolated := (proposer + 1) % 4
	c.deaf[isolated] = true
	c.startEpoch(epoch)
	c.advance(c.interval / 4)

	env, ok := proto.Clone(c.lastProposal()).(*pb.ConsensusEnvelope)
	if !ok {
		t.Fatal("cannot copy the proposal")
	}
	return c, isolated, env
}

// A report is evidence because a validator wrote it. One signed by someone
// outside the set is not evidence at all, however well formed, or a proposer
// could manufacture a quorum out of keys it made up.
func TestAProposalCarryingReportsFromOutsidersIsRefused(t *testing.T) {
	c, isolated, env := isolatedProposal(t)

	for _, report := range env.GetProposal().GetReports() {
		outsider := grape_wallet.NewWallet()
		report.Pk = *outsider.PublicKey()
		report.Sign = nil
		payload, err := consensusPayloadHash(report)
		if err != nil {
			t.Fatalf("hashing a report: %s", err.Error())
		}
		report.Sign = grape_wallet.NewDSA().Sign(*outsider.PrivateKey(), payload)
	}
	c.sign(c.indexOfProposer(1, 0), env)

	if err := c.engines[isolated].deliver(env); err == nil {
		t.Fatal("a proposal justified only by reports from outside the validator set was accepted")
	}
	if votes := c.votesHeld(isolated); votes != 0 {
		t.Fatalf("the validator voted on evidence from outsiders (%d vote(s) held)", votes)
	}
}

// A report names the epoch it is about. One about a different epoch says
// nothing about this one - the sites it lists may already have been settled -
// so replaying old reports must not justify a new commit transaction.
func TestAProposalCarryingReportsFromAnotherEpochIsRefused(t *testing.T) {
	c, isolated, env := isolatedProposal(t)

	for _, report := range env.GetProposal().GetReports() {
		confirmed, ok := report.Payload.(*pb.ConsensusEnvelope_Confirmed)
		if !ok {
			t.Fatal("a carried report is not a confirmed set")
		}
		author := c.indexOfValidator(report.Pk)
		if author < 0 {
			t.Fatal("a carried report is not from a validator")
		}
		// Genuinely signed by its author, but about a different epoch.
		confirmed.Confirmed.Epoch = 99
		c.sign(author, report)
	}
	c.sign(c.indexOfProposer(1, 0), env)

	if err := c.engines[isolated].deliver(env); err == nil {
		t.Fatal("a proposal justified only by reports about another epoch was accepted")
	}
	if votes := c.votesHeld(isolated); votes != 0 {
		t.Fatalf("the validator voted on evidence from another epoch (%d vote(s) held)", votes)
	}
}

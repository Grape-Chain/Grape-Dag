package dag

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// attributionTx - a signed payment transaction, built without touching the
// package globals so these tests do not depend on a configured dag.
func attributionTx(t *testing.T, nonce uint64) *tx.Txv1 {
	t.Helper()
	w := grape_wallet.NewWallet()
	x := tx.NewTxv1(tx.PRIVATE_TESTNET)
	x.Tx_Type = tx.PAYMENT
	x.Sender_Pubk = *w.PublicKey()
	x.Sender = grape_wallet.AddressToBytes(w.WalletAddress())
	x.Recepient = grape_wallet.AddressToBytes(grape_wallet.NewWallet().WalletAddress())
	x.Amount = big.NewInt(int64(1000 + nonce)).Bytes()
	x.Nonce = nonce
	x.Timestamp = time.Now()
	x.Fuel_Limit = big.NewInt(21000).Bytes()
	x.Fuel_Price = big.NewInt(1).Bytes()
	x.Sign(w.PrivateKey())
	return x
}

// attributionTarget - a bare site standing in for an approved one. Only its id
// is ever read.
func attributionTarget() *Node {
	return &Node{id: NodeID{id: uuid.New()}}
}

// attributionSite - a site as it looks on the node that built it: an id, a
// transaction, and live edges to the sites it approves.
func attributionSite(t *testing.T, targets ...*Node) *Node {
	t.Helper()
	return &Node{
		id:      NodeID{id: uuid.New()},
		tx:      attributionTx(t, 1),
		targets: targets,
	}
}

func TestASignedSiteVerifies(t *testing.T) {
	w := grape_wallet.NewWallet()
	site := attributionSite(t, attributionTarget(), attributionTarget())

	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}
	if err := verifyProcessor(site); err != nil {
		t.Fatalf("a freshly signed site should verify, got: %s", err.Error())
	}
}

func TestSigningStampsTheProcessorsOwnAddressAndKey(t *testing.T) {
	w := grape_wallet.NewWallet()
	site := attributionSite(t, attributionTarget())

	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}
	if want := grape_wallet.AddressToBytes(w.WalletAddress()); !bytes.Equal(site.processorAddress, want) {
		t.Fatalf("expected the signer's address to be stamped on the site")
	}
	if !bytes.Equal(site.processorPk, *w.PublicKey()) {
		t.Fatalf("expected the signer's public key to be stamped on the site")
	}
	if len(site.processorSig) == 0 {
		t.Fatal("expected a signature to be stamped on the site")
	}
}

// The case that decides whether attribution can be rolled out at all. A site
// from a peer built before attribution existed carries none of the three fields,
// and it has to stay valid.
func TestASiteWithNoAttributionIsReportedAsUnattributedRatherThanInvalid(t *testing.T) {
	site := attributionSite(t, attributionTarget())

	err := verifyProcessor(site)
	if err == nil {
		t.Fatal("an unattributed site should be reported as unattributed, not as verified")
	}
	if !errors.Is(err, ErrNoProcessorAttribution) {
		t.Fatalf("expected ErrNoProcessorAttribution, got: %s", err.Error())
	}
	// The distinction is the point: an unattributed site must never be mistaken
	// for a site that is lying about who built it.
	if errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatal("an unattributed site must not be reported as a bad attribution")
	}
}

// Half an attribution is not an old peer - an old peer sends none of it.
func TestAnIncompleteAttributionIsRejectedRatherThanTreatedAsAbsent(t *testing.T) {
	w := grape_wallet.NewWallet()

	for _, drop := range []struct {
		name  string
		blank func(n *Node)
	}{
		{"address", func(n *Node) { n.processorAddress = nil }},
		{"public key", func(n *Node) { n.processorPk = nil }},
		{"signature", func(n *Node) { n.processorSig = nil }},
	} {
		site := attributionSite(t, attributionTarget())
		if err := signProcessor(site, w); err != nil {
			t.Fatalf("signing the site: %s", err.Error())
		}
		drop.blank(site)

		err := verifyProcessor(site)
		if !errors.Is(err, ErrBadProcessorAttribution) {
			t.Fatalf("a site missing only its processor %s should be a bad attribution, got: %v", drop.name, err)
		}
		if errors.Is(err, ErrNoProcessorAttribution) {
			t.Fatalf("a site missing only its processor %s must not read as unattributed", drop.name)
		}
	}
}

func TestAlteringTheProcessorAddressAfterSigningFailsVerification(t *testing.T) {
	w := grape_wallet.NewWallet()
	site := attributionSite(t, attributionTarget())
	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	// Redirecting the fee to another wallet is the obvious attack on a signed
	// site, so it is the one that has to fail loudest.
	site.processorAddress = grape_wallet.AddressToBytes(grape_wallet.NewWallet().WalletAddress())

	if err := verifyProcessor(site); !errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatalf("a site whose processor address was swapped should fail, got: %v", err)
	}
}

// A processor is free to sign a payload naming a third party's address with its
// own key: the signature is then perfectly self-consistent. Only the check that
// the address is the one the key produces catches it.
func TestAnAddressThatIsNotDerivedFromTheProcessorKeyFailsVerification(t *testing.T) {
	liar := grape_wallet.NewWallet()
	innocent := grape_wallet.NewWallet()
	site := attributionSite(t, attributionTarget())

	victimAddress := grape_wallet.AddressToBytes(innocent.WalletAddress())
	payload, err := processorPayload(site, victimAddress)
	if err != nil {
		t.Fatalf("building the payload: %s", err.Error())
	}
	site.processorAddress = victimAddress
	site.processorPk = *liar.PublicKey()
	site.processorSig = grape_wallet.NewDSA().Sign(*liar.PrivateKey(), payload)

	// Self-consistent: the signature really is over these bytes by this key.
	if !grape_wallet.NewDSA().Verify(grape_wallet.PublicKey(site.processorPk), site.processorSig, payload) {
		t.Fatal("the forged attribution should be self-consistent, or this test proves nothing")
	}
	if err := verifyProcessor(site); !errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatalf("attributing a site to an address the signer does not control should fail, got: %v", err)
	}
}

func TestAlteringTheSiteTransactionAfterSigningFailsVerification(t *testing.T) {
	w := grape_wallet.NewWallet()
	site := attributionSite(t, attributionTarget())
	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	// Editing the amount in place, which is the change that would matter.
	site.tx.(*tx.Txv1).Amount = big.NewInt(999999).Bytes()

	if err := verifyProcessor(site); !errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatalf("a site whose transaction was edited should fail, got: %v", err)
	}
}

func TestSubstitutingTheSiteTransactionAfterSigningFailsVerification(t *testing.T) {
	w := grape_wallet.NewWallet()
	site := attributionSite(t, attributionTarget())
	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	site.tx = attributionTx(t, 2)

	if err := verifyProcessor(site); !errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatalf("a site whose transaction was swapped wholesale should fail, got: %v", err)
	}
}

func TestAlteringTheApprovalTargetsAfterSigningFailsVerification(t *testing.T) {
	w := grape_wallet.NewWallet()
	first, second := attributionTarget(), attributionTarget()
	site := attributionSite(t, first, second)
	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	// Rewriting history: claiming this site approved something else.
	site.targets = []*Node{first, attributionTarget()}

	if err := verifyProcessor(site); !errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatalf("a site whose approvals were rewritten should fail, got: %v", err)
	}
}

func TestDroppingAnApprovalTargetAfterSigningFailsVerification(t *testing.T) {
	w := grape_wallet.NewWallet()
	first, second := attributionTarget(), attributionTarget()
	site := attributionSite(t, first, second)
	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	site.targets = []*Node{first}

	if err := verifyProcessor(site); !errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatalf("a site that quietly dropped an approval should fail, got: %v", err)
	}
}

// The other half of the contract, and the one that makes attribution usable at
// all: every field a peer recomputes for itself has to stay outside the
// signature, or a correctly signed site would fail everywhere except on the node
// that built it.
func TestAlteringTheLocallyRecomputedFieldsAfterSigningStillVerifies(t *testing.T) {
	w := grape_wallet.NewWallet()

	for _, field := range []struct {
		name   string
		mutate func(n *Node)
	}{
		// Recomputed by updateCumWeights as approvers arrive.
		{"cumWeight", func(n *Node) { n.cumWeight.Store(1234.5) }},
		// A fresh random normal draw per node; see genRandomTxWeight.
		{"txWeight", func(n *Node) { n.txWeight = 9.75 }},
		// Overwritten with time.Now() by addToDag, and again by InsertTxDag on
		// every receiving peer.
		{"time", func(n *Node) { n.time = time.Now().Add(72 * time.Hour) }},
		// The result of this peer's own signature check.
		{"valid", func(n *Node) { n.valid = !n.valid }},
		// Derived from whichever tips this peer happened to hold.
		{"height", func(n *Node) { n.height = Height{minheight: 41, maxheight: 43} }},
	} {
		site := attributionSite(t, attributionTarget(), attributionTarget())
		if err := signProcessor(site, w); err != nil {
			t.Fatalf("signing the site: %s", err.Error())
		}
		field.mutate(site)

		if err := verifyProcessor(site); err != nil {
			t.Fatalf("%s is local and must not be covered by the signature, but changing it broke verification: %s",
				field.name, err.Error())
		}
	}
}

// The approval set is the union of the three places an approval can be recorded,
// so a signature taken when the site was inserted still verifies after the
// ledger has moved on. Without the union this is the test that fails: the
// signature would have a shelf life ending at the next reconcile.
func TestTheSignatureSurvivesApprovalsMovingBetweenTheTargetSets(t *testing.T) {
	w := grape_wallet.NewWallet()
	first, second := attributionTarget(), attributionTarget()
	site := attributionSite(t, first, second)
	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	// As a peer holds it before it has fetched either approved site.
	site.targets = nil
	site.missingTargets = map[string]bool{
		first.id.id.String():  true,
		second.id.id.String(): true,
	}
	if err := verifyProcessor(site); err != nil {
		t.Fatalf("a site whose approvals are all still missing should verify: %s", err.Error())
	}

	// Part way through, as ReconcileMissingTargets links one of them.
	site.targets = []*Node{first}
	site.missingTargets = map[string]bool{second.id.id.String(): true}
	if err := verifyProcessor(site); err != nil {
		t.Fatalf("a site part way through relinking should verify: %s", err.Error())
	}

	// Fully linked.
	site.targets = []*Node{first, second}
	site.missingTargets = nil
	if err := verifyProcessor(site); err != nil {
		t.Fatalf("a fully relinked site should verify: %s", err.Error())
	}

	// And after the slicer settles both approved sites and drops the edges.
	site.targets = nil
	site.slicedTargets = []uuid.UUID{first.id.id, second.id.id}
	if err := verifyProcessor(site); err != nil {
		t.Fatalf("a site whose approvals have been settled into a slice should verify: %s", err.Error())
	}
}

// Approving A then B is the same site as approving B then A, so the order the
// edges happen to be held in must not matter. missingTargets is a Go map, whose
// iteration order is randomised, so without an explicit sort this fails
// intermittently rather than cleanly.
func TestTheOrderTheApprovalsAreHeldInDoesNotChangeTheSignedBytes(t *testing.T) {
	first, second, third := attributionTarget(), attributionTarget(), attributionTarget()
	site := attributionSite(t, first, second, third)
	address := grape_wallet.AddressToBytes(grape_wallet.NewWallet().WalletAddress())

	forward, err := processorPayload(site, address)
	if err != nil {
		t.Fatalf("building the payload: %s", err.Error())
	}
	site.targets = []*Node{third, first, second}
	reordered, err := processorPayload(site, address)
	if err != nil {
		t.Fatalf("building the payload: %s", err.Error())
	}
	if !bytes.Equal(forward, reordered) {
		t.Fatal("reordering the approval edges changed the signed bytes")
	}

	// And the same set arriving as missingTargets rather than as edges.
	site.targets = nil
	site.missingTargets = map[string]bool{
		first.id.id.String():  true,
		second.id.id.String(): true,
		third.id.id.String():  true,
	}
	asMissing, err := processorPayload(site, address)
	if err != nil {
		t.Fatalf("building the payload: %s", err.Error())
	}
	if !bytes.Equal(forward, asMissing) {
		t.Fatal("the same approvals held as missing targets produced different signed bytes")
	}
}

func TestASignatureMadeByADifferentWalletFailsVerification(t *testing.T) {
	honest := grape_wallet.NewWallet()
	other := grape_wallet.NewWallet()
	site := attributionSite(t, attributionTarget())
	if err := signProcessor(site, honest); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	// The honest processor's address and key, but a signature from somebody else.
	payload, err := processorPayload(site, site.processorAddress)
	if err != nil {
		t.Fatalf("building the payload: %s", err.Error())
	}
	site.processorSig = grape_wallet.NewDSA().Sign(*other.PrivateKey(), payload)

	if err := verifyProcessor(site); !errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatalf("a signature from another wallet should fail, got: %v", err)
	}
}

func TestAnAttributionLiftedFromAnotherSiteFailsVerification(t *testing.T) {
	w := grape_wallet.NewWallet()
	signed := attributionSite(t, attributionTarget())
	if err := signProcessor(signed, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	// Copying a valid attribution onto a different site earns the processor a fee
	// for work it did not do on that site.
	other := attributionSite(t, attributionTarget())
	other.processorAddress = signed.processorAddress
	other.processorPk = signed.processorPk
	other.processorSig = signed.processorSig

	if err := verifyProcessor(other); !errors.Is(err, ErrBadProcessorAttribution) {
		t.Fatalf("an attribution copied from another site should fail, got: %v", err)
	}
}

func TestTwoDifferentSitesDoNotProduceTheSameSignedBytes(t *testing.T) {
	w := grape_wallet.NewWallet()
	address := grape_wallet.AddressToBytes(w.WalletAddress())
	shared := attributionTarget()

	// Differing in one component at a time, so a collision cannot be blamed on
	// two components cancelling out.
	base := attributionSite(t, shared)

	differentID := &Node{id: NodeID{id: uuid.New()}, tx: base.tx, targets: base.targets}
	differentTx := &Node{id: base.id, tx: attributionTx(t, 7), targets: base.targets}
	differentTargets := &Node{id: base.id, tx: base.tx, targets: []*Node{attributionTarget()}}
	extraTarget := &Node{id: base.id, tx: base.tx, targets: []*Node{shared, attributionTarget()}}
	noTargets := &Node{id: base.id, tx: base.tx}

	payloads := map[string][]byte{}
	for name, site := range map[string]*Node{
		"base":              base,
		"different id":      differentID,
		"different tx":      differentTx,
		"different targets": differentTargets,
		"extra target":      extraTarget,
		"no targets":        noTargets,
	} {
		payload, err := processorPayload(site, address)
		if err != nil {
			t.Fatalf("building the payload for the %s site: %s", name, err.Error())
		}
		for other, seen := range payloads {
			if bytes.Equal(payload, seen) {
				t.Fatalf("the %s site and the %s site produced the same signed bytes", name, other)
			}
		}
		payloads[name] = payload
	}

	// And the signatures themselves differ, not merely the payloads.
	left := attributionSite(t, attributionTarget())
	right := attributionSite(t, attributionTarget())
	if err := signProcessor(left, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}
	if err := signProcessor(right, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}
	if bytes.Equal(left.processorSig, right.processorSig) {
		t.Fatal("two different sites signed by the same wallet produced the same signature")
	}
}

// The processor's address is bound into the payload, so the same site attributed
// to two different processors is two different sets of signed bytes. Without
// this, an attribution could be moved between processors.
func TestTheProcessorAddressChangesTheSignedBytes(t *testing.T) {
	site := attributionSite(t, attributionTarget())

	first, err := processorPayload(site, grape_wallet.AddressToBytes(grape_wallet.NewWallet().WalletAddress()))
	if err != nil {
		t.Fatalf("building the payload: %s", err.Error())
	}
	second, err := processorPayload(site, grape_wallet.AddressToBytes(grape_wallet.NewWallet().WalletAddress()))
	if err != nil {
		t.Fatalf("building the payload: %s", err.Error())
	}
	if bytes.Equal(first, second) {
		t.Fatal("the processor address is not covered by the signed bytes")
	}
}

// ed25519.Verify panics on a public key of the wrong length, and this key
// arrives from the network, so a malformed site must be rejected before it
// reaches the verifier. Otherwise one peer could stop a node with one site.
func TestAMalformedProcessorKeyOrSignatureIsRejectedWithoutPanicking(t *testing.T) {
	w := grape_wallet.NewWallet()

	for _, broken := range []struct {
		name    string
		corrupt func(n *Node)
	}{
		// The address is rewritten to the one these malformed keys produce, so
		// the attribution stays self-consistent and the address check cannot be
		// what rejects it. Without that the site is turned away before it ever
		// reaches ed25519.Verify, and the test would pass whether or not the
		// length guard exists. An attacker picks both fields, so this is the
		// shape a real malformed site would take.
		{"short public key", func(n *Node) {
			n.processorPk = []byte{1, 2, 3}
			n.processorAddress = grape_wallet.AddressToBytes(
				grape_wallet.AddressFromPulicKey(grape_wallet.PublicKey(n.processorPk)))
		}},
		{"long public key", func(n *Node) {
			n.processorPk = bytes.Repeat([]byte{7}, 64)
			n.processorAddress = grape_wallet.AddressToBytes(
				grape_wallet.AddressFromPulicKey(grape_wallet.PublicKey(n.processorPk)))
		}},
		{"short signature", func(n *Node) { n.processorSig = []byte{1, 2, 3} }},
		{"long signature", func(n *Node) { n.processorSig = bytes.Repeat([]byte{7}, 128) }},
		{"short address", func(n *Node) { n.processorAddress = []byte{1} }},
	} {
		site := attributionSite(t, attributionTarget())
		if err := signProcessor(site, w); err != nil {
			t.Fatalf("signing the site: %s", err.Error())
		}
		broken.corrupt(site)

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("verifying a site with a %s panicked: %v", broken.name, r)
				}
			}()
			if err := verifyProcessor(site); !errors.Is(err, ErrBadProcessorAttribution) {
				t.Fatalf("a site with a %s should be a bad attribution, got: %v", broken.name, err)
			}
		}()
	}
}

// Neither function may touch anything but the three attribution fields.
// Attribution is stamped on a site that is already in the graph, so a stray
// write here would corrupt live ledger state.
func TestSigningAndVerifyingTouchNothingButTheAttributionFields(t *testing.T) {
	w := grape_wallet.NewWallet()
	first, second := attributionTarget(), attributionTarget()
	site := attributionSite(t, first, second)
	site.cumWeight.Store(3.5)
	site.txWeight = 1.5
	site.time = time.Now()
	site.valid = true
	site.height = Height{minheight: 2, maxheight: 4}
	site.missingTargets = map[string]bool{uuid.New().String(): true}
	site.slicedTargets = []uuid.UUID{uuid.New()}
	site.sources = []*Node{attributionTarget()}

	type snapshot struct {
		id                  NodeID
		cumWeight, txWeight float64
		time                time.Time
		valid               bool
		height              Height
		transaction         tx.Transaction
		targets, sources    []*Node
		missingTargets      map[string]bool
		slicedTargets       []uuid.UUID
	}
	before := snapshot{
		id: site.id, cumWeight: site.cumWeight.Load(), txWeight: site.txWeight,
		time: site.time, valid: site.valid, height: site.height, transaction: site.tx,
		targets: site.targets, sources: site.sources,
		missingTargets: site.missingTargets, slicedTargets: site.slicedTargets,
	}
	txBytes := site.tx.GetHash()

	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}
	if err := verifyProcessor(site); err != nil {
		t.Fatalf("verifying the site: %s", err.Error())
	}

	after := snapshot{
		id: site.id, cumWeight: site.cumWeight.Load(), txWeight: site.txWeight,
		time: site.time, valid: site.valid, height: site.height, transaction: site.tx,
		targets: site.targets, sources: site.sources,
		missingTargets: site.missingTargets, slicedTargets: site.slicedTargets,
	}
	if before.id != after.id {
		t.Error("the site's id changed")
	}
	if before.cumWeight != after.cumWeight || before.txWeight != after.txWeight {
		t.Error("the site's weights changed")
	}
	if !before.time.Equal(after.time) {
		t.Error("the site's time changed")
	}
	if before.valid != after.valid {
		t.Error("the site's valid flag changed")
	}
	if before.height != after.height {
		t.Error("the site's height changed")
	}
	if before.transaction != after.transaction {
		t.Error("the site's transaction was replaced")
	}
	if !bytes.Equal(txBytes, site.tx.GetHash()) {
		t.Error("the site's transaction was mutated")
	}
	if len(before.targets) != len(after.targets) || len(before.sources) != len(after.sources) {
		t.Error("the site's edges changed")
	}
	if len(before.missingTargets) != len(after.missingTargets) {
		t.Error("the site's missing targets changed")
	}
	if len(before.slicedTargets) != len(after.slicedTargets) {
		t.Error("the site's sliced targets changed")
	}
}

// Attribution is only worth anything if it survives the wire, and this is the
// path a receiving peer actually takes.
func TestAttributionSurvivesARoundTripThroughTheWireForm(t *testing.T) {
	w := grape_wallet.NewWallet()
	first, second := attributionTarget(), attributionTarget()
	site := attributionSite(t, first, second)
	if err := signProcessor(site, w); err != nil {
		t.Fatalf("signing the site: %s", err.Error())
	}

	received := &Node{}
	received.FromPbNode(site.ToPbNode())

	if err := verifyProcessor(received); err != nil {
		t.Fatalf("a signed site should still verify after a round trip through pb.Node: %s", err.Error())
	}
	if !bytes.Equal(received.processorAddress, site.processorAddress) {
		t.Error("the processor address did not survive the round trip")
	}
}

// The compatibility direction that matters on an upgrade: a site serialised
// without attribution still reads back as a valid, merely unattributed site.
func TestAnUnattributedSiteSurvivesTheWireFormAsUnattributed(t *testing.T) {
	site := attributionSite(t, attributionTarget())

	received := &Node{}
	received.FromPbNode(site.ToPbNode())

	if err := verifyProcessor(received); !errors.Is(err, ErrNoProcessorAttribution) {
		t.Fatalf("an unattributed site should round trip as unattributed, got: %v", err)
	}
}

func TestSigningASiteWithNoTransactionIsRefused(t *testing.T) {
	w := grape_wallet.NewWallet()
	site := &Node{id: NodeID{id: uuid.New()}}

	if err := signProcessor(site, w); err == nil {
		t.Fatal("a site with no transaction has nothing to attribute and should be refused")
	}
	// And nothing was stamped on the way out.
	if len(site.processorAddress) != 0 || len(site.processorPk) != 0 || len(site.processorSig) != 0 {
		t.Fatal("a failed signing must leave the site unattributed rather than half attributed")
	}
}

func TestSigningWithoutAWalletOrSiteIsRefused(t *testing.T) {
	if err := signProcessor(nil, grape_wallet.NewWallet()); err == nil {
		t.Error("signing a nil site should be refused")
	}
	if err := signProcessor(attributionSite(t), nil); err == nil {
		t.Error("signing without a wallet should be refused")
	}
	if err := verifyProcessor(nil); err == nil {
		t.Error("verifying a nil site should be refused")
	}
}

// A known-answer test pinning the exact bytes a processor signature is taken
// over.
//
// The other tests all check relative properties - change this, verification
// breaks - and every one of them would still pass if the payload layout were
// rewritten wholesale, because they sign and verify with the same code. That is
// exactly the change that must not happen quietly: the signed bytes are a
// consensus rule, and altering the domain tag, the field order, the framing or
// the hash would invalidate every attribution already on the ledger while
// looking perfectly correct in isolation.
//
// If this test fails, the wire format has changed. That is not necessarily
// wrong, but it needs a new attributionDomain version and a migration plan, not
// a new golden value.
func TestTheSignedBytesMatchThePinnedFormat(t *testing.T) {
	const goldenPayload = "c04092790520fe535a610cedbf429ae536a7d62fed8155f3e2d108107239aea2"

	// Everything fixed: no wallets, no clocks, no fresh uuids.
	transaction := tx.NewTxv1(tx.PRIVATE_TESTNET)
	transaction.Tx_Type = tx.PAYMENT
	transaction.Sender_Pubk = bytes.Repeat([]byte{0x11}, 32)
	transaction.Sender = bytes.Repeat([]byte{0x22}, 20)
	transaction.Recepient = bytes.Repeat([]byte{0x33}, 20)
	transaction.Amount = big.NewInt(4242).Bytes()
	transaction.Nonce = 17
	transaction.Timestamp = time.Unix(1700000000, 0).UTC()
	transaction.Fuel_Limit = big.NewInt(21000).Bytes()
	transaction.Fuel_Price = big.NewInt(3).Bytes()
	transaction.Signature = bytes.Repeat([]byte{0x44}, 64)

	site := &Node{
		id: NodeID{id: uuid.MustParse("11111111-2222-3333-4444-555555555555")},
		tx: transaction,
		targets: []*Node{
			{id: NodeID{id: uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")}},
			{id: NodeID{id: uuid.MustParse("99999999-8888-7777-6666-555555555555")}},
		},
	}
	address := bytes.Repeat([]byte{0x55}, 20)

	payload, err := processorPayload(site, address)
	if err != nil {
		t.Fatalf("building the payload: %s", err.Error())
	}
	if got := hex.EncodeToString(payload); got != goldenPayload {
		t.Fatalf("the signed bytes have changed.\n  pinned: %s\n  got:    %s\n"+
			"If this is intended, bump attributionDomain and plan the migration.",
			goldenPayload, got)
	}
}

// A site's claim has to survive diffusion. The subscribing peer builds its own
// site from the transaction rather than taking the sender's, so if the claim
// does not travel on the gossip record it is lost at the first hop and only the
// node that built a site ever knows it built it. These two functions are that
// route, so they are tested as a pair.
func TestAClaimSurvivesBeingCarriedOntoAnotherPeersSite(t *testing.T) {
	w := grape_wallet.NewWallet()
	targets := []*Node{attributionTarget(), attributionTarget()}
	built := attributionSite(t, targets...)
	if err := signProcessor(built, w); err != nil {
		t.Fatalf("signing: %s", err.Error())
	}

	address, pk, sig := ProcessorAttribution(built)
	if len(address) == 0 || len(pk) == 0 || len(sig) == 0 {
		t.Fatal("a signed site reported an empty claim")
	}

	// What the subscribing peer does: its own site from the same transaction
	// and the same approvals, with the claim copied across. Same id, because
	// the site's id is derived from the transaction and both peers derive it.
	received := &Node{id: built.id, tx: built.tx, targets: targets}
	SetProcessorAttribution(received, address, pk, sig)

	if err := verifyProcessor(received); err != nil {
		t.Fatalf("a claim carried onto an equivalent site does not verify: %s", err.Error())
	}
}

// Reading a claim off a site that has none must not invent one, or every
// unattributed site would arrive looking like a malformed claim.
func TestReadingAClaimFromAnUnattributedSiteReportsNothing(t *testing.T) {
	address, pk, sig := ProcessorAttribution(attributionSite(t, attributionTarget()))
	if address != nil || pk != nil || sig != nil {
		t.Fatalf("an unattributed site reported a claim: %v %v %v", address, pk, sig)
	}
	if a, p, s := ProcessorAttribution(nil); a != nil || p != nil || s != nil {
		t.Fatal("a nil site reported a claim")
	}
}

// A claim that does not verify is stripped rather than refused: the site is a
// valid part of the graph, and refusing it would let anyone deny the network a
// transaction by attaching a bad claim. What must not survive is the claim.
func TestStrippingAClaimLeavesTheSiteUnattributedRatherThanInvalid(t *testing.T) {
	w := grape_wallet.NewWallet()
	n := attributionSite(t, attributionTarget(), attributionTarget())
	if err := signProcessor(n, w); err != nil {
		t.Fatalf("signing: %s", err.Error())
	}
	clearProcessor(n)

	if err := verifyProcessor(n); !errors.Is(err, ErrNoProcessorAttribution) {
		t.Fatalf("a stripped site verifies as %v, want ErrNoProcessorAttribution", err)
	}
	clearProcessor(nil) // must not panic
}

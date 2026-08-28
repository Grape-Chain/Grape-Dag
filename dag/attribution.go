package dag

import (
	"bytes"
	"crypto"
	"encoding/binary"
	"sort"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
	"google.golang.org/protobuf/proto"
)

// Processor attribution - which node encapsulated a transaction into a site.
//
// A site says who built it so that the fee for building it can be paid to
// somebody, and so that the claim is checkable rather than merely asserted.
// Section 3.1 of the technical paper asks for the same thing as a site-level
// signature.

var (
	// ErrNoProcessorAttribution - the site names no processor at all.
	//
	// Distinct from ErrBadProcessorAttribution on purpose, and the distinction is
	// the whole reason both exist. Attribution is a strictly additive wire
	// change, so a site built by a node that predates it arrives with all three
	// fields empty. That site is honest and must stay valid - it simply earns
	// nobody a fee. Collapsing this into the failure case would make every older
	// peer's traffic look like forgery and partition the network on an upgrade.
	ErrNoProcessorAttribution = errors.New("site carries no processor attribution")

	// ErrBadProcessorAttribution - the site names a processor, but the claim does
	// not hold up.
	//
	// This is not a tolerable state. Attribution is only present because somebody
	// put it there, so a signature that does not check out is evidence of a lie:
	// either a site edited in flight, or a node claiming work it did not do.
	// Callers should treat it as grounds for rejecting the site, and should use
	// errors.Is rather than comparing directly, since the returned errors are
	// wrapped with the reason they failed.
	ErrBadProcessorAttribution = errors.New("site processor attribution does not verify")
)

// attributionDomain - domain separation for the signed payload.
//
// Keeps a processor signature from ever being replayed as some other signature
// this wallet produces over a bare hash, and carries a version so the covered
// field set can change later without a new signature silently verifying against
// the old rules.
const attributionDomain = "grape-site-processor-v1"

// approvalTargetIDs - the ids of the sites this site approves, deduplicated and
// in a fixed order.
//
// Taken as the union of three places an approval can be recorded, because no
// single one of them is complete at all times and on all peers:
//
//   - targets - live edges. All of them on the builder once addToDag has run,
//     none of them on a peer that has not yet found the approved sites.
//   - slicedTargets - approvals whose sites have since been settled into a
//     slice. The edge pointer is dropped so the settled site can be collected,
//     but the approval still happened.
//   - missingTargets - approvals whose sites this peer has not got yet.
//
// The union is what makes the signature stable, and that is the point. Sites
// migrate between the three sets as the ledger progresses:
// ReconcileMissingTargets moves an id out of missingTargets and into targets,
// and the slicer moves one out of targets and into slicedTargets. Every one of
// those moves leaves the union untouched, so a signature taken at insertion time
// still verifies after relinking and after slicing. Signing any single set on
// its own would produce a signature with a shelf life - correct when written and
// broken by the next reconcile.
//
// Sorted by raw id bytes rather than by insertion order. missingTargets is a Go
// map, and map iteration order is randomised, so an unsorted walk would hash
// differently on successive calls over the same site - the same class of bug the
// commit transaction hit; see the deterministicMarshal comment in
// tx/pb/txpin.go. Tip order is not meaningful anyway: approving A then B is the
// same site as approving B then A.
func approvalTargetIDs(n *Node) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(n.targets)+len(n.slicedTargets)+len(n.missingTargets))
	seen := make(map[uuid.UUID]struct{}, cap(ids))
	add := func(id uuid.UUID) {
		if id == uuid.Nil {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, t := range n.targets {
		if t != nil {
			add(t.id.id)
		}
	}
	for _, id := range n.slicedTargets {
		add(id)
	}
	for k := range n.missingTargets {
		// A key that is not a uuid cannot name a site, so it cannot be an
		// approval. Skipped rather than treated as an error: missingTargets
		// arrives from a peer, and one malformed key should not stop an
		// otherwise good site from being checked.
		if id, err := uuid.Parse(k); err == nil {
			add(id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i][:], ids[j][:]) < 0
	})
	return ids
}

// siteTxHash - the hash of the transaction this site encapsulates.
//
// Marshalled with Deterministic set. pb.Txv1 holds no map today, so this agrees
// byte for byte with the transaction's own GetHash; the option is set because
// the guarantee wanted here is that the same transaction hashes the same way on
// every peer forever, and that should not quietly depend on nobody ever adding a
// map field to Txv1.
func siteTxHash(n *Node) ([]byte, error) {
	if n.tx == nil {
		// A site with no transaction has nothing to attribute. Reported rather
		// than hashed as empty, so a caller cannot end up with a signature over
		// a site whose payload is missing.
		return nil, errors.New("site has no transaction to attribute")
	}
	buf, err := proto.MarshalOptions{Deterministic: true}.Marshal(n.tx.MarshalBinary())
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshal the site's transaction")
	}
	return utils.GetBuilder().Build(crypto.SHA256).Hash(buf), nil
}

// processorPayload - the bytes a processor signature is taken over: this site's
// identity, and nothing about how any particular peer happens to be holding it.
//
// Covered:
//
//   - the site's own id
//   - the hash of its transaction
//   - the ids of the sites it approves
//   - the processor's address
//
// Those four are what "this node built this site out of this transaction on top
// of these approvals" means, and every one of them is fixed at the moment the
// site is built. The address is covered so that the fee payee cannot be swapped
// after the fact; the public key is not, because verification checks the
// signature against whichever key is presented, and verifyProcessor separately
// requires the address to be the one that key produces - so the key is bound
// transitively. That mirrors the reasoning in TxPin.PrototypeHash, which leaves
// the signer's key out of its own payload.
//
// Deliberately NOT covered - cumWeight, txWeight, time, valid, height,
// missingTargets. Every one of them is local, so signing any of them would
// produce a signature that verifies on the builder and fails on every other
// peer:
//
//   - txWeight is a fresh random normal draw per node (genRandomTxWeight), so it
//     is essentially guaranteed to differ.
//   - time is overwritten with time.Now() by addToDag on the builder and again
//     by InsertTxDag on each receiver.
//   - cumWeight is recomputed locally by updateCumWeights as approvers arrive.
//   - valid records the outcome of this peer's own signature check.
//   - height is derived from the tips this peer happened to have.
//   - missingTargets is this peer's private backlog of sites it has not fetched;
//     the approvals themselves are covered via approvalTargetIDs instead.
//
// Framed with an explicit length prefix per component rather than by
// concatenation, so that no two different sites can frame to the same bytes by
// one component borrowing the next one's leading bytes.
//
// Built by hand rather than by cloning a pb.Node and clearing the excluded
// fields, which is the shape TxPin.PrototypeHash uses. The clone-and-clear
// pattern is the better one when the message is stable, but pb.Node is not: it
// is being extended right now, and under clone-and-clear any field added to it
// later would silently join the signed bytes and invalidate every signature
// already on the ledger. Naming the covered fields explicitly costs a little
// more code and makes adding a wire field a safe operation. It also sidesteps
// the fact that pb.Node carries a map, which a hash over the whole message would
// have to marshal deterministically to be reproducible at all.
//
// Takes the address as an argument instead of reading n.processorAddress so that
// signing can compute the payload before stamping anything onto the site: a
// failure part-way through then leaves the site untouched rather than
// half-attributed.
func processorPayload(n *Node, processorAddress []byte) ([]byte, error) {
	txHash, err := siteTxHash(n)
	if err != nil {
		return nil, err
	}
	targets := approvalTargetIDs(n)

	buf := &bytes.Buffer{}
	write := func(component []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(component)))
		buf.Write(length[:])
		buf.Write(component)
	}
	write([]byte(attributionDomain))
	siteID := n.id.id
	write(siteID[:])
	write(txHash)
	// The count is framed too, so a site approving one target cannot frame
	// identically to a site approving none.
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(targets)))
	write(count[:])
	for _, id := range targets {
		target := id
		write(target[:])
	}
	write(processorAddress)

	return utils.GetBuilder().Build(crypto.SHA256).Hash(buf.Bytes()), nil
}

// signProcessor - stamp this node's identity onto the site as its processor and
// sign the site's identity.
//
// Touches processorAddress, processorPk and processorSig and nothing else. In
// particular it does not restamp the site's id or recompute its weights, so it
// is safe to call on a site that is already in the graph.
//
// Must be called once the site's approvals are established - after addToDag has
// linked it - since the approval-target ids are part of what is signed.
func signProcessor(n *Node, w *grape_wallet.Wallet) error {
	if n == nil {
		return errors.New("cannot attribute a nil site")
	}
	if w == nil {
		return errors.New("cannot attribute a site without a wallet to sign with")
	}
	address := grape_wallet.AddressToBytes(w.WalletAddress())
	payload, err := processorPayload(n, address)
	if err != nil {
		return errors.Wrap(err, "cannot build the processor payload to sign")
	}
	signature := grape_wallet.NewDSA().Sign(*w.PrivateKey(), payload)

	// Copied rather than aliased. PublicKey is a slice, and handing the wallet's
	// own backing array to every site it processes would let a later write
	// through any one of them change the others.
	publicKey := append([]byte(nil), (*w.PublicKey())...)

	n.processorAddress = address
	n.processorPk = publicKey
	n.processorSig = signature
	return nil
}

// verifyProcessor - recompute this site's processor payload and check the
// signature over it.
//
// Returns ErrNoProcessorAttribution for a site that claims no processor, which
// is a tolerable state, and an error wrapping ErrBadProcessorAttribution for a
// site whose claim does not hold up, which is not. Match with errors.Is.
//
// Reads only; it does not repair or clear a bad attribution. A caller that wants
// to drop the site should drop the site - silently blanking the fields here
// would destroy the evidence that something upstream is lying.
func verifyProcessor(n *Node) error {
	if n == nil {
		return errors.New("cannot verify the attribution of a nil site")
	}
	haveAddress := len(n.processorAddress) > 0
	havePk := len(n.processorPk) > 0
	haveSig := len(n.processorSig) > 0

	if !haveAddress && !havePk && !haveSig {
		return ErrNoProcessorAttribution
	}
	if !haveAddress || !havePk || !haveSig {
		// Not an older peer: an older peer sends none of the three. Something
		// filled in part of an attribution, which is either corruption or an
		// attempt to have a site look attributed without a checkable claim.
		return errors.Wrap(ErrBadProcessorAttribution, "attribution is incomplete")
	}
	if len(n.processorPk) != ed25519.PublicKeySize {
		// Checked before verifying, not for tidiness: ed25519.Verify panics on a
		// public key of the wrong length, and this key arrives from the network.
		// Without this, a peer could stop a node with a single malformed site.
		return errors.Wrapf(ErrBadProcessorAttribution,
			"processor public key is %d bytes, expected %d", len(n.processorPk), ed25519.PublicKeySize)
	}
	if len(n.processorSig) != ed25519.SignatureSize {
		return errors.Wrapf(ErrBadProcessorAttribution,
			"processor signature is %d bytes, expected %d", len(n.processorSig), ed25519.SignatureSize)
	}

	// The address has to be the one this key produces. The signature alone cannot
	// establish that: a processor is free to sign a payload naming somebody
	// else's address, and that forgery is self-consistent and would verify. It
	// would let a node attribute its sites - and the fees for them - to a wallet
	// it does not control, and pin its own work on an innocent third party.
	expected := grape_wallet.AddressToBytes(
		grape_wallet.AddressFromPulicKey(grape_wallet.PublicKey(n.processorPk)))
	if !bytes.Equal(expected, n.processorAddress) {
		return errors.Wrap(ErrBadProcessorAttribution,
			"processor address is not the address its public key produces")
	}

	payload, err := processorPayload(n, n.processorAddress)
	if err != nil {
		return errors.Wrap(ErrBadProcessorAttribution, err.Error())
	}
	if !grape_wallet.NewDSA().Verify(grape_wallet.PublicKey(n.processorPk), n.processorSig, payload) {
		return errors.Wrap(ErrBadProcessorAttribution, "processor signature does not check out")
	}
	return nil
}

// clearProcessor - remove an attribution claim from a site.
//
// Used when a claim is present but does not verify. The site stays: it is a
// valid part of the graph, and refusing it would let anyone deny the network a
// transaction by attaching a bad claim to it. What must not survive is the
// claim itself, or the liar collects the fee. A stripped site is then
// indistinguishable from one built before attribution existed, which is exactly
// what it should be treated as - work nobody can prove they did.
func clearProcessor(n *Node) {
	if n == nil {
		return
	}
	n.processorAddress = nil
	n.processorPk = nil
	n.processorSig = nil
}

// ProcessorAttribution - the claim a site carries, for the gossip path to
// forward. Exported because a site's attribution has to survive diffusion, and
// the subscribing peer builds its own site from the transaction rather than
// taking the sender's: without a route through the gossip record the claim
// would be lost at the first hop and only the node that built a site would ever
// know it built it.
func ProcessorAttribution(n *Node) (address, pk, sig []byte) {
	if n == nil {
		return nil, nil, nil
	}
	return n.processorAddress, n.processorPk, n.processorSig
}

// SetProcessorAttribution - carry a claim onto a site received over gossip.
//
// Not verified here. The claim is checked where the site joins the graph, which
// is the point at which its approvals are known and therefore the point at
// which the signature can be recomputed at all.
func SetProcessorAttribution(n *Node, address, pk, sig []byte) {
	if n == nil {
		return
	}
	n.processorAddress = address
	n.processorPk = pk
	n.processorSig = sig
}

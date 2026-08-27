package dag

import (
	"encoding/hex"
	"strings"
	"sync"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/pkg/errors"
)

/*
Who is allowed to settle the ledger.

A commit transaction is the ledger's only irrevocable statement: it names the
sites that are settled and the balances that follow from them, and every node
applies it and slices the settled sites out of its graph. Until now nothing
checked where one came from. TxPin.VerifyTx existed and was called from nowhere,
so any peer on the gossip topic could publish a commit transaction of its own
devising and every node would apply it, rewrite its balances and discard the
sites it named.

Calling VerifyTx would not on its own have fixed that, which is worth being
explicit about. It verifies the signature against TxPin.Pk - the public key
carried inside the commit transaction being verified - so a forged one signed
with the forger's own key and carrying the matching public key passes. A
signature is only evidence once it is checked against a key that was authorised
in advance. That is what this file is: the set of keys whose commit transactions
this node will apply.

Where the set comes from, in order of preference:

  - dag.pinsigners, a list of public keys in configuration. This is the strong
    form: the node will apply a commit transaction from these keys and no
    others, whatever it is told.

  - The chain-opening commit transaction, adopted on first use. A node that
    joins learns the chain from a peer's snapshot, so it has to trust something
    on the way in; adopting the signer of the opening statement at least means
    every later commit transaction has to come from whoever opened the chain. A
    peer that lies about the whole chain still succeeds, so this is a weaker
    guarantee and it says so at start-up.

Once the validator set lands this becomes a set with a quorum count rather than
a set with a single member, and the check becomes "at least q of these keys
signed" instead of "one of these keys signed". The seam is deliberate: nothing
outside this file cares how many keys there are.
*/

// pinAuth - the authorised signers for this node's chain.
var pinAuth = newPinAuthority()

type pinAuthority struct {
	mu      sync.RWMutex
	signers map[string]struct{}
	// configured - the set came from configuration, so it is not open to being
	// learned from the chain and a mismatch is fatal to the chain rather than
	// something to adopt.
	configured bool
}

func newPinAuthority() *pinAuthority {
	return &pinAuthority{signers: make(map[string]struct{})}
}

// configurePinSigners - take the authorised set from configuration. A key that
// cannot be decoded is refused loudly rather than silently dropped: a typo that
// quietly left the node trusting nobody, or trusting the first peer to talk to
// it, is the failure this whole file exists to prevent.
func configurePinSigners(raw string) error {
	pinAuth.mu.Lock()
	defer pinAuth.mu.Unlock()
	pinAuth.signers = make(map[string]struct{})
	pinAuth.configured = false

	for _, field := range strings.Split(raw, ",") {
		key := strings.TrimPrefix(strings.TrimSpace(field), "0x")
		if key == "" {
			continue
		}
		if _, err := hex.DecodeString(key); err != nil {
			return errors.Errorf("dag.pinsigners contains %q, which is not hex: %s", field, err.Error())
		}
		pinAuth.signers[strings.ToLower(key)] = struct{}{}
		pinAuth.configured = true
	}
	return nil
}

// adoptChainSigner - learn the authorised signer from the chain-opening commit
// transaction, when configuration did not name one.
//
// Adopts only into an empty set. That check is defence in depth rather than the
// load-bearing one - authoriseChainStart already refuses an opening statement
// once any signer is known, so this is never reached with a non-empty set - but
// it is cheap and this is the function whose job is to widen the trusted set,
// which makes it the wrong place to rely on a caller.
func (a *pinAuthority) adoptChainSigner(pk []byte) {
	if len(pk) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.configured || len(a.signers) != 0 {
		return
	}
	key := strings.ToLower(hex.EncodeToString(pk))
	a.signers[key] = struct{}{}
	logger.Warnf("[pin auth] Adopting %s as the only signer whose commit transactions this node will apply, learned from the chain-opening statement. Set dag.pinsigners to state it in configuration instead - adoption trusts whichever peer supplied the chain.",
		shortKey(key))
}

func (a *pinAuthority) allows(pk []byte) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if len(a.signers) == 0 {
		return false
	}
	_, ok := a.signers[strings.ToLower(hex.EncodeToString(pk))]
	return ok
}

func (a *pinAuthority) known() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.signers)
}

// authorisePin - may this commit transaction be applied? Both questions have to
// be asked, and in this order: a valid signature from an unauthorised key is
// exactly what a forgery looks like.
func authorisePin(pin *pb.TxPin) error {
	if pin == nil {
		return errors.New("no commit transaction to authorise")
	}
	if err := pin.VerifyTx(); err != nil {
		return errors.Errorf("commit transaction pin=%d is not correctly signed: %s", pin.PinNumber, err.Error())
	}
	if !pinAuth.allows(pin.Pk) {
		if pinAuth.known() == 0 {
			return errors.Errorf("commit transaction pin=%d refused: this node has no authorised signer yet, so it has no chain to continue", pin.PinNumber)
		}
		return errors.Errorf("commit transaction pin=%d is signed by %s, which is not an authorised signer",
			pin.PinNumber, shortKey(hex.EncodeToString(pin.Pk)))
	}
	return nil
}

// authoriseChainStart - the opening statement of this node's chain: the genesis
// commit transaction on a node that starts a ledger, or a peer's snapshot on a
// node that joins one.
//
// When configuration named the signers, this is the strongest check the node
// ever makes: a snapshot from anyone else is refused, so a hostile peer cannot
// hand a joining node a chain of its own construction. When it did not, the
// signer is adopted, and the node says so.
func authoriseChainStart(pin *pb.TxPin) error {
	if pin == nil {
		return errors.New("no opening commit transaction to authorise")
	}
	if err := pin.VerifyTx(); err != nil {
		return errors.Errorf("the opening commit transaction pin=%d is not correctly signed: %s", pin.PinNumber, err.Error())
	}
	// A set that already has members is settled, however it got them: a second
	// opening statement from a different signer is refused rather than adopted
	// alongside the first. Without this, a peer that answers a snapshot request
	// after the chain is already open could add its own key.
	if pinAuth.known() > 0 {
		if !pinAuth.allows(pin.Pk) {
			source := "dag.pinsigners does not name that key"
			if !pinAuth.configured {
				source = "that is not the signer already adopted for this chain"
			}
			return errors.Errorf("the opening commit transaction pin=%d is signed by %s, and %s",
				pin.PinNumber, shortKey(hex.EncodeToString(pin.Pk)), source)
		}
		return nil
	}
	pinAuth.adoptChainSigner(pin.Pk)
	return nil
}

// logPinAuthority - say at start-up whose commit transactions will be applied,
// because "nobody has told this node yet" and "anyone" look identical from the
// outside until something goes wrong.
func logPinAuthority() {
	pinAuth.mu.RLock()
	defer pinAuth.mu.RUnlock()
	switch {
	case pinAuth.configured:
		keys := make([]string, 0, len(pinAuth.signers))
		for k := range pinAuth.signers {
			keys = append(keys, shortKey(k))
		}
		utils.ColorizeInfo(logger, "[pin auth] Applying commit transactions from %d configured signer(s): %s",
			len(keys), strings.Join(keys, ", "))
	default:
		logger.Warnf("[pin auth] No dag.pinsigners configured; the signer will be adopted from the chain-opening statement, which trusts whichever peer supplies the chain")
	}
}

// shortKey - enough of a public key to recognise in a log line.
func shortKey(hexKey string) string {
	if len(hexKey) <= 16 {
		return hexKey
	}
	return hexKey[:8] + ".." + hexKey[len(hexKey)-8:]
}

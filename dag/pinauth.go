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

	// validators / quorum - the quorum form. A commit transaction is applied
	// because at least `quorum` of these keys signed it, rather than because one
	// authorised key asserted it. Empty unless dag.consensus is "quorum".
	validators map[string]struct{}
	quorum     int
}

func newPinAuthority() *pinAuthority {
	return &pinAuthority{
		signers:    make(map[string]struct{}),
		validators: make(map[string]struct{}),
	}
}

// quorumMode - is this node applying commit transactions on the strength of a
// validator quorum rather than a single signer?
func (a *pinAuthority) quorumMode() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.quorum > 0 && len(a.validators) > 0
}

// validatorSet - a copy, so a verifier can read it without holding the lock
// across the signature checks.
func (a *pinAuthority) validatorSet() (map[string]struct{}, int) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]struct{}, len(a.validators))
	for k := range a.validators {
		out[k] = struct{}{}
	}
	return out, a.quorum
}

// quorumFor - the number of agreeing validators a commit transaction needs out
// of a set of n: the usual two thirds plus one, which tolerates the largest
// number of faulty members a set of that size can tolerate at all.
func quorumFor(n int) int {
	if n <= 0 {
		return 0
	}
	return (2*n)/3 + 1
}

// configureValidators - take the validator set from configuration and derive the
// quorum from its size.
func configureValidators(raw string) error {
	keys, err := parseKeyList(raw, "dag.validators")
	if err != nil {
		return err
	}
	pinAuth.mu.Lock()
	defer pinAuth.mu.Unlock()
	pinAuth.validators = keys
	pinAuth.quorum = quorumFor(len(keys))
	return nil
}

// parseKeyList - hex public keys, comma separated, 0x optional. A key that
// cannot be decoded is refused loudly rather than silently dropped: a typo that
// quietly shrank the validator set would lower the quorum with it.
func parseKeyList(raw, setting string) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	for _, field := range strings.Split(raw, ",") {
		key := strings.TrimPrefix(strings.TrimSpace(field), "0x")
		if key == "" {
			continue
		}
		if _, err := hex.DecodeString(key); err != nil {
			return nil, errors.Errorf("%s contains %q, which is not hex: %s", setting, field, err.Error())
		}
		out[strings.ToLower(key)] = struct{}{}
	}
	return out, nil
}

// configurePinSigners - take the authorised set from configuration. A key that
// cannot be decoded is refused loudly rather than silently dropped: a typo that
// quietly left the node trusting nobody, or trusting the first peer to talk to
// it, is the failure this whole file exists to prevent.
func configurePinSigners(raw string) error {
	keys, err := parseKeyList(raw, "dag.pinsigners")
	if err != nil {
		return err
	}
	pinAuth.mu.Lock()
	defer pinAuth.mu.Unlock()
	pinAuth.signers = keys
	pinAuth.configured = len(keys) > 0
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
	// In quorum mode the reason to apply a commit transaction is that a quorum of
	// the validator set agreed to it, not that one key asserted it. The
	// proposer's own signature is not consulted: it is one validator's opinion,
	// and it is already inside the certificate if it counts.
	if pinAuth.quorumMode() {
		validators, quorum := pinAuth.validatorSet()
		if err := pin.VerifyQuorum(validators, quorum); err != nil {
			return errors.Errorf("commit transaction pin=%d does not carry a validator quorum: %s",
				pin.PinNumber, err.Error())
		}
		return nil
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

// authoriseStoredChain - the opening statement of a chain read back from this
// node's own disk.
//
// Deliberately more forgiving than the network path, and the threat model is the
// reason. Everything this node knows about its chain comes out of that
// directory: anyone who can rewrite a signature there can rewrite the balances
// next to it, so refusing to start on a signature that does not verify buys
// nothing and costs the node its history. The signature is still checked and a
// failure is still reported loudly, because a failure means something is wrong -
// a chain written before the signing payload was corrected will fail here, and
// so will a corrupted store.
//
// The signer is still held to configuration when there is any: a stored chain
// opened by a key dag.pinsigners does not name is a misconfiguration worth
// stopping for, since continuing would mean following a chain the operator did
// not authorise.
func authoriseStoredChain(pin *pb.TxPin) error {
	if pin == nil {
		return errors.New("no stored opening commit transaction to authorise")
	}
	if err := pin.VerifyTx(); err != nil {
		logger.Warnf("[pin auth] The stored chain's opening commit transaction pin=%d does not verify (%s). Continuing, because the store is already trusted for the balances themselves - but a chain written before the signing payload was corrected will need to be rebuilt before it can be handed to a peer.",
			pin.PinNumber, err.Error())
	}
	if pinAuth.configured {
		if !pinAuth.allows(pin.Pk) {
			return errors.Errorf("the stored chain was opened by %s, which dag.pinsigners does not name",
				shortKey(hex.EncodeToString(pin.Pk)))
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
	case pinAuth.quorum > 0 && len(pinAuth.validators) > 0:
		utils.ColorizeInfo(logger, "[pin auth] Applying commit transactions agreed by %d of %d validator(s)",
			pinAuth.quorum, len(pinAuth.validators))
	case pinAuth.quorum > 0:
		logger.Warnf("[pin auth] dag.consensus is quorum but dag.validators is empty, so no commit transaction can ever be applied")
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

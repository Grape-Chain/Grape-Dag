package dag

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// withPinAuthority - a clean signer set for one test.
func withPinAuthority(t *testing.T) {
	t.Helper()
	prev := pinAuth
	pinAuth = newPinAuthority()
	t.Cleanup(func() { pinAuth = prev })
}

// signedPin - a commit transaction signed by the given wallet, exactly as the
// node signs its own.
func signedPin(t *testing.T, w *grape_wallet.Wallet, number int64) *pb.TxPin {
	t.Helper()
	pin := pb.NewTxPin([]byte{byte(number)})
	pin.PinNumber = number
	pin.Ts = timestamppb.Now()
	pin.SignTx(w)
	return pin
}

// The test that matters most, and the reason calling VerifyTx would not have
// been enough on its own.
//
// VerifyTx checks the signature against TxPin.Pk - the public key carried inside
// the commit transaction being checked. A forgery signed with the forger's own
// key carries the matching public key, so it is perfectly self-consistent and
// VerifyTx passes it. A signature is only evidence once it is checked against a
// key that was authorised beforehand.
func TestAForgeryWithItsOwnValidSignatureIsRefused(t *testing.T) {
	withPinAuthority(t)

	honest := grape_wallet.NewWallet()
	forger := grape_wallet.NewWallet()
	if err := configurePinSigners(hex.EncodeToString(*honest.PublicKey())); err != nil {
		t.Fatalf("configuring the signer: %s", err.Error())
	}

	forged := signedPin(t, forger, 1)

	// Self-consistent, which is the whole point: the old check would pass it.
	if err := forged.VerifyTx(); err != nil {
		t.Fatalf("the forgery should carry a valid signature over its own key, got: %s", err.Error())
	}
	// And it is still refused.
	if err := authorisePin(forged); err == nil {
		t.Fatal("a commit transaction signed by an unauthorised key was accepted")
	}

	// The honest signer's is accepted, so the check is discriminating rather
	// than simply refusing everything.
	if err := authorisePin(signedPin(t, honest, 1)); err != nil {
		t.Fatalf("the authorised signer's commit transaction was refused: %s", err.Error())
	}
}

func TestATamperedPinIsRefused(t *testing.T) {
	withPinAuthority(t)
	w := grape_wallet.NewWallet()
	if err := configurePinSigners(hex.EncodeToString(*w.PublicKey())); err != nil {
		t.Fatalf("configuring the signer: %s", err.Error())
	}

	pin := signedPin(t, w, 1)
	if err := authorisePin(pin); err != nil {
		t.Fatalf("a correctly signed commit transaction was refused: %s", err.Error())
	}

	// Change what it settles, keeping the signature.
	pin.Balance.Balance["0x0000000000000000000000000000000000000001"] = []byte{0xff}
	if err := authorisePin(pin); err == nil {
		t.Fatal("a commit transaction whose contents were changed after signing was accepted")
	}
}

func TestAnUnsignedPinIsRefused(t *testing.T) {
	withPinAuthority(t)
	w := grape_wallet.NewWallet()
	if err := configurePinSigners(hex.EncodeToString(*w.PublicKey())); err != nil {
		t.Fatalf("configuring the signer: %s", err.Error())
	}
	pin := pb.NewTxPin(nil)
	pin.PinNumber = 1
	pin.Ts = timestamppb.Now()
	if err := authorisePin(pin); err == nil {
		t.Fatal("an unsigned commit transaction was accepted")
	}
}

// With no signers at all, everything is refused. The alternative - accepting
// anything until told otherwise - is exactly the hole this closes.
func TestWithNoAuthorisedSignerEverythingIsRefused(t *testing.T) {
	withPinAuthority(t)
	if err := authorisePin(signedPin(t, grape_wallet.NewWallet(), 1)); err == nil {
		t.Fatal("a commit transaction was accepted by a node with no authorised signer")
	}
}

// A joining node learns the chain from a peer, so with nothing configured it
// adopts that peer's signer - and then holds everyone to it.
func TestTheChainOpeningSignerIsAdoptedAndThenEnforced(t *testing.T) {
	withPinAuthority(t)
	leader := grape_wallet.NewWallet()
	other := grape_wallet.NewWallet()

	if err := authoriseChainStart(signedPin(t, leader, 0)); err != nil {
		t.Fatalf("the opening commit transaction was refused: %s", err.Error())
	}
	if got := pinAuth.known(); got != 1 {
		t.Fatalf("expected exactly one adopted signer, got %d", got)
	}
	if err := authorisePin(signedPin(t, leader, 1)); err != nil {
		t.Fatalf("the adopted signer's commit transaction was refused: %s", err.Error())
	}
	if err := authorisePin(signedPin(t, other, 1)); err == nil {
		t.Fatal("a commit transaction from a second signer was accepted after one was adopted")
	}
	// Adoption happens exactly once. A second opening statement from someone
	// else is refused, not adopted alongside the first - otherwise a peer that
	// answers a snapshot request after the chain is already open could add its
	// own key to the set.
	if err := authoriseChainStart(signedPin(t, other, 0)); err == nil {
		t.Fatal("a second opening statement from a different signer was accepted")
	}
	if got := pinAuth.known(); got != 1 {
		t.Fatalf("adoption added a second signer: %d known", got)
	}
	// And the originally adopted signer still works.
	if err := authoriseChainStart(signedPin(t, leader, 0)); err != nil {
		t.Fatalf("the adopted signer's opening statement was refused: %s", err.Error())
	}
}

// The strong form: with the signers named in configuration, a peer cannot hand a
// joining node a chain of its own construction.
func TestAConfiguredSignerRefusesAnotherChainsOpening(t *testing.T) {
	withPinAuthority(t)
	expected := grape_wallet.NewWallet()
	impostor := grape_wallet.NewWallet()
	if err := configurePinSigners("0x" + hex.EncodeToString(*expected.PublicKey())); err != nil {
		t.Fatalf("configuring the signer: %s", err.Error())
	}

	if err := authoriseChainStart(signedPin(t, impostor, 0)); err == nil {
		t.Fatal("a snapshot from an unexpected signer opened the chain")
	}
	if err := authoriseChainStart(signedPin(t, expected, 0)); err != nil {
		t.Fatalf("the configured signer's opening statement was refused: %s", err.Error())
	}
	if got := pinAuth.known(); got != 1 {
		t.Fatalf("expected the configured signer only, got %d", got)
	}
}

func TestSeveralConfiguredSignersAreAllAccepted(t *testing.T) {
	withPinAuthority(t)
	a, b := grape_wallet.NewWallet(), grape_wallet.NewWallet()
	raw := hex.EncodeToString(*a.PublicKey()) + " , 0x" + hex.EncodeToString(*b.PublicKey())
	if err := configurePinSigners(raw); err != nil {
		t.Fatalf("configuring the signers: %s", err.Error())
	}
	if got := pinAuth.known(); got != 2 {
		t.Fatalf("expected two signers, got %d", got)
	}
	for name, w := range map[string]*grape_wallet.Wallet{"a": a, "b": b} {
		if err := authorisePin(signedPin(t, w, 1)); err != nil {
			t.Fatalf("signer %s was refused: %s", name, err.Error())
		}
	}
	if err := authorisePin(signedPin(t, grape_wallet.NewWallet(), 1)); err == nil {
		t.Fatal("a third, unconfigured signer was accepted")
	}
}

// A typo in the signer list must stop the node, not leave it trusting nobody or
// - worse - falling back to adopting whichever peer speaks first.
func TestABadSignerListIsRefused(t *testing.T) {
	withPinAuthority(t)
	err := configurePinSigners("not-hex-at-all")
	if err == nil {
		t.Fatal("a signer list that is not hex was accepted")
	}
	if !strings.Contains(err.Error(), "dag.pinsigners") {
		t.Fatalf("the error should name the setting at fault, got: %s", err.Error())
	}
	if pinAuth.configured {
		t.Fatal("a refused signer list left the authority marked as configured")
	}
}

func TestAnEmptySignerListLeavesTheAuthorityOpenToAdoption(t *testing.T) {
	withPinAuthority(t)
	if err := configurePinSigners("  ,  "); err != nil {
		t.Fatalf("an empty signer list is a valid choice, got: %s", err.Error())
	}
	if pinAuth.configured {
		t.Fatal("an empty signer list should not count as configured")
	}
	leader := grape_wallet.NewWallet()
	if err := authoriseChainStart(signedPin(t, leader, 0)); err != nil {
		t.Fatalf("adoption should still work: %s", err.Error())
	}
	if got := pinAuth.known(); got != 1 {
		t.Fatalf("expected the adopted signer, got %d", got)
	}
}

// The end-to-end property, rather than the unit: a commit transaction from an
// unauthorised signer must not move the chain forward.
//
// This is the one that would have caught the original hole. Everything above
// tests the authorisation function; this tests that the function is actually on
// the path a commit transaction takes.
func TestApplyPinRefusesAnUnauthorisedCommitTransaction(t *testing.T) {
	recoveryFixture(t, filepath.Join(t.TempDir(), "ledger"))
	// Node type 2 keeps the smart-contract stage out of this test: on a full
	// node applying a commit transaction re-executes contracts through the JVM
	// state server, which is not what is under test here.
	peerConfig.NodeType = 2

	// Open the chain with the signer the fixture authorised.
	chainStartCommitted(storedPin(0, nil, map[string]string{addrStr(0xaa): "1000"}))
	before := _pins_.CurrentHeight()

	// A commit transaction from someone else, correctly signed by them, at
	// exactly the height the chain is waiting for.
	forger := grape_wallet.NewWallet()
	forged := pb.NewTxPin(nil)
	forged.PinNumber = int64(before + 1)
	forged.Ts = timestamppb.Now()
	forged.Balance.Balance[addrStr(0xaa)] = []byte{0x01}
	forged.SignTx(forger)

	if applyPin(forged) {
		t.Fatal("a commit transaction from an unauthorised signer was applied")
	}
	if got := _pins_.CurrentHeight(); got != before {
		t.Fatalf("the chain moved from %d to %d on a refused commit transaction", before, got)
	}

	// Positive control: the same height, from the authorised signer, is applied.
	// Without this the test would pass just as well if applyPin refused
	// everything.
	legitimate := storedPin(int64(before+1), nil, map[string]string{addrStr(0xaa): "999"})
	if !applyPin(legitimate) {
		t.Fatal("a commit transaction from the authorised signer was refused")
	}
	if got := _pins_.CurrentHeight(); got != before+1 {
		t.Fatalf("the chain did not advance on an accepted commit transaction: height %d", got)
	}
}

// A chain on disk is trusted differently from one off the wire, and the threat
// model is why: everything the node knows about its chain comes out of that
// directory, so a signature there proves nothing an attacker with write access
// could not also forge. A stored chain whose opening signature does not verify -
// which is what a chain written before the signing payload was corrected looks
// like - must still start, loudly.
func TestAStoredChainStartsEvenIfItsSignatureDoesNotVerify(t *testing.T) {
	withPinAuthority(t)
	leader := grape_wallet.NewWallet()

	stale := signedPin(t, leader, 0)
	// Corrupt the signature the way an older signing payload would have.
	stale.Sign[0] ^= 0xff

	if err := authoriseStoredChain(stale); err != nil {
		t.Fatalf("a stored chain must still open: %s", err.Error())
	}
	if got := pinAuth.known(); got != 1 {
		t.Fatalf("the stored chain's signer should still be adopted, known=%d", got)
	}
	// But the network path refuses exactly the same commit transaction.
	withPinAuthority(t)
	if err := authoriseChainStart(stale); err == nil {
		t.Fatal("the network path accepted an opening statement whose signature does not verify")
	}
}

// Forgiving about the signature is not the same as forgiving about the signer: a
// stored chain opened by a key the operator did not name is a misconfiguration,
// and following it would mean following a chain nobody authorised.
func TestAStoredChainFromAnUnnamedSignerIsRefused(t *testing.T) {
	withPinAuthority(t)
	expected := grape_wallet.NewWallet()
	if err := configurePinSigners(hex.EncodeToString(*expected.PublicKey())); err != nil {
		t.Fatalf("configuring the signer: %s", err.Error())
	}
	if err := authoriseStoredChain(signedPin(t, grape_wallet.NewWallet(), 0)); err == nil {
		t.Fatal("a stored chain opened by an unnamed signer was accepted")
	}
	if err := authoriseStoredChain(signedPin(t, expected, 0)); err != nil {
		t.Fatalf("a stored chain opened by the named signer was refused: %s", err.Error())
	}
}

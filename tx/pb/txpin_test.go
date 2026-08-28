package pb

import (
	"encoding/hex"
	"fmt"
	"testing"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// pinWithBalances - a commit transaction carrying enough balance entries for map
// ordering to matter.
func pinWithBalances(entries int) *TxPin {
	pin := NewTxPin(nil)
	pin.PinNumber = 7
	pin.Ts = timestamppb.Now()
	for i := 0; i < entries; i++ {
		pin.Balance.Balance[fmt.Sprintf("0x%040x", i)] = []byte{byte(i + 1)}
	}
	return pin
}

// A commit transaction's hash has to be the same every time it is taken.
//
// It is not free by default. The hash is taken over the protobuf encoding, a
// commit transaction carries two maps - the balance map, and missingTargets on
// every site it names - and the Go protobuf implementation randomises map
// iteration on purpose so that nobody depends on the order. The result was that
// a correctly signed commit transaction failed its own signature check four
// times out of five, and that the eth RPC reported a different block hash for
// the same commit transaction on successive calls. Neither was noticed, because
// nothing verified a signature and nobody compared two block hashes.
func TestThePinHashIsStable(t *testing.T) {
	pin := pinWithBalances(24)
	first, err := pin.Hash(0)
	if err != nil {
		t.Fatalf("hashing: %s", err.Error())
	}
	for i := 0; i < 200; i++ {
		again, err := pin.Hash(0)
		if err != nil {
			t.Fatalf("hashing: %s", err.Error())
		}
		if hex.EncodeToString(again) != hex.EncodeToString(first) {
			t.Fatalf("the hash of one commit transaction changed between calls on attempt %d", i)
		}
	}
}

// The property that actually matters, and the one whose absence made the
// signature check unusable: sign once, verify as often as you like.
func TestASignedPinVerifiesEveryTime(t *testing.T) {
	w := grape_wallet.NewWallet()
	pin := pinWithBalances(24)
	pin.SignTx(w)

	for i := 0; i < 500; i++ {
		if err := pin.VerifyTx(); err != nil {
			t.Fatalf("verification failed on attempt %d of a correctly signed commit transaction: %s", i, err.Error())
		}
	}
}

// Verifying must not change the commit transaction. The version this replaces
// blanked the signature on the receiver, hashed, and put it back - a mutation of
// an object other goroutines read.
func TestVerifyingDoesNotMutateThePin(t *testing.T) {
	w := grape_wallet.NewWallet()
	pin := pinWithBalances(8)
	pin.SignTx(w)

	before, err := pin.Hash(0)
	if err != nil {
		t.Fatalf("hashing: %s", err.Error())
	}
	sigBefore := hex.EncodeToString(pin.Sign)
	if err := pin.VerifyTx(); err != nil {
		t.Fatalf("verification failed: %s", err.Error())
	}
	after, err := pin.Hash(0)
	if err != nil {
		t.Fatalf("hashing: %s", err.Error())
	}
	if hex.EncodeToString(before) != hex.EncodeToString(after) {
		t.Fatal("verifying changed the commit transaction's hash")
	}
	if sigBefore != hex.EncodeToString(pin.Sign) {
		t.Fatal("verifying changed the commit transaction's signature")
	}
}

// The signed payload excludes the public key, so that more than one signer can
// sign the same bytes - which is what a validator quorum needs. Swapping the key
// is not a way in: verification checks the signature against whichever key is
// presented, so it simply fails.
func TestThePrototypeHashIgnoresTheSignerIdentity(t *testing.T) {
	a, b := grape_wallet.NewWallet(), grape_wallet.NewWallet()
	pin := pinWithBalances(4)

	pin.SignTx(a)
	withA, err := pin.PrototypeHash()
	if err != nil {
		t.Fatalf("hashing: %s", err.Error())
	}
	pin.SignTx(b)
	withB, err := pin.PrototypeHash()
	if err != nil {
		t.Fatalf("hashing: %s", err.Error())
	}
	if hex.EncodeToString(withA) != hex.EncodeToString(withB) {
		t.Fatal("the signed payload changed with the signer, so two signers cannot sign the same bytes")
	}

	// And a key swapped after signing is rejected.
	pin.SignTx(a)
	pin.Pk = *b.PublicKey()
	if err := pin.VerifyTx(); err == nil {
		t.Fatal("a commit transaction whose public key was swapped after signing was accepted")
	}
}

// Changing what a commit transaction settles must invalidate its signature, even
// though the signed payload excludes the signature itself.
func TestChangingWhatAPinSettlesInvalidatesIt(t *testing.T) {
	w := grape_wallet.NewWallet()
	pin := pinWithBalances(8)
	pin.SignTx(w)
	if err := pin.VerifyTx(); err != nil {
		t.Fatalf("verification failed: %s", err.Error())
	}
	pin.Balance.Balance["0x00000000000000000000000000000000000000ff"] = []byte{0xff}
	if err := pin.VerifyTx(); err == nil {
		t.Fatal("a commit transaction with an added balance was still accepted")
	}
}

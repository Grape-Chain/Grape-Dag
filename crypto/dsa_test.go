package grape1crypto

import (
	"crypto/ed25519"
	"testing"
)

// ed25519.Verify panics on a public key that is not exactly 32 bytes - the
// library's own "bad public key length". This function is reached from the
// gossip subscriber with a key that arrived over the wire, so a peer sending a
// transaction with a five-byte sender key killed the receiving node with one
// message. The check that was here rejected only an empty key, which let every
// other wrong length straight through to the panic.
//
// Verification has to return false for these, not panic and not accept.
func TestVerifyRefusesAMalformedPublicKeyInsteadOfPanicking(t *testing.T) {
	message := []byte("a message a peer might send")
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating a key pair: %s", err)
	}
	good := ed25519.Sign(priv, message)

	// The control: the real key and the real signature must still verify, or
	// this test would pass just as well against a Verify that always said no.
	if !(Ed25519DSA{}).Verify(PublicKey(pub), good, message) {
		t.Fatal("a valid signature was refused - the length checks are rejecting good input")
	}

	for _, n := range []int{0, 1, 5, 31, 33, 64} {
		if (Ed25519DSA{}).Verify(PublicKey(make([]byte, n)), good, message) {
			t.Errorf("a %d-byte public key was accepted", n)
		}
	}
}

// A wrong-sized signature does not panic in the library, but it must still be
// refused, and refused as a signature problem rather than silently.
func TestVerifyRefusesAMalformedSignature(t *testing.T) {
	message := []byte("a message a peer might send")
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating a key pair: %s", err)
	}
	for _, n := range []int{0, 1, 63, 65, 128} {
		if (Ed25519DSA{}).Verify(PublicKey(pub), make([]byte, n), message) {
			t.Errorf("a %d-byte signature was accepted", n)
		}
	}
}

// An empty message is refused rather than verified against, which is the
// long-standing behaviour and is kept: nothing in this system signs nothing.
func TestVerifyRefusesAnEmptyMessage(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating a key pair: %s", err)
	}
	if (Ed25519DSA{}).Verify(PublicKey(pub), ed25519.Sign(priv, nil), nil) {
		t.Error("an empty message was accepted")
	}
}

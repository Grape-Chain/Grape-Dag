package grape1crypto

import (
	"crypto"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	golog "github.com/ipfs/go-log/v2"
	"golang.org/x/crypto/ed25519"
)

type PrivateKey []byte

type PublicKey []byte

type DSA interface {
	Sign(key PrivateKey, message []byte) []byte

	Verify(key PublicKey, signature []byte, message []byte) bool
}

type Ed25519DSA struct{}

var logger golog.EventLogger

func init() {
	logger = golog.Logger("grape1crypto")
}

func NewDSA() DSA {
	return Ed25519DSA{}
}

func (dsa Ed25519DSA) Sign(key PrivateKey, message []byte) []byte {
	privKey := ed25519.NewKeyFromSeed(key)
	signature, err := privKey.Sign(rand.Reader, message, crypto.Hash(0))
	if err != nil {
		panic(fmt.Errorf("error during signing message: %s", err.Error()))
	}
	return signature
}

// Verify - check an ed25519 signature.
//
// The lengths are checked exactly, not merely for emptiness. ed25519.Verify
// panics on a public key that is not exactly PublicKeySize bytes - "bad public
// key length" - and this function is reached from the gossip subscriber with a
// key that came off the wire. A peer sending a transaction with a five-byte
// sender key therefore killed the receiving node with a single message, and the
// emptiness check let it straight through. dag/attribution.go already checks the
// processor key this way, for the same reason; this is the same check on the
// path that a transaction's own sender key takes.
//
// The signature length is checked too. That one does not panic - the library
// returns false for a wrong-sized signature - but a caller reading the log
// should be told which of the three inputs was malformed rather than being left
// with a bare false.
func (dsa Ed25519DSA) Verify(key PublicKey, signature []byte, message []byte) bool {
	if len(key) != ed25519.PublicKeySize {
		logger.Errorf("[Verify] Public key is %d bytes, want %d", len(key), ed25519.PublicKeySize)
		return false
	}
	if len(signature) != ed25519.SignatureSize {
		logger.Errorf("[Verify] Signature is %d bytes, want %d", len(signature), ed25519.SignatureSize)
		return false
	}
	if len(message) == 0 {
		logger.Error("[Verify] Message body is invalid")
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(key), message, signature)
}

func ParsePrivateKey(hexstr string) (PrivateKey, error) {
	decoded, err := hex.DecodeString(hexstr)
	if err != nil {
		return nil, err
	}
	return PrivateKey(decoded), nil
}

func ParsePublicKey(hexstr string) (PublicKey, error) {
	decoded, err := hex.DecodeString(hexstr)
	if err != nil {
		return nil, err
	}
	return PublicKey(decoded), nil
}

func GenerateKeys() (PrivateKey, PublicKey) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	return privateKey.Seed(), PublicKey(publicKey)
}

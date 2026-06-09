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

func (dsa Ed25519DSA) Verify(key PublicKey, signature []byte, message []byte) bool {
	if len(key) == 0 {
		logger.Error("[Verify] Public key is invalid")
		return false
	}
	if len(signature) == 0 {
		logger.Error("[Verify] Signature is invalid")
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

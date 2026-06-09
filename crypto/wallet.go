package grape1crypto

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
)

type Wallet struct {
	privateKey    PrivateKey
	publicKey     PublicKey
	walletAddress string // ETH like
}

func LoadWallet(pubKey string, privKey string) *Wallet {
	pubKeyParsed, err := ParsePublicKey(pubKey)
	if err != nil {
		panic(fmt.Errorf("not correct format of public key: %s", err.Error()))
	}
	privKeyParsed, err := ParsePrivateKey(privKey)
	if err != nil {
		panic(fmt.Errorf("not correct format of private key: %s", err.Error()))
	}
	return creatWallet(pubKeyParsed, privKeyParsed)
}

func creatWallet(pubKey PublicKey, privKey PrivateKey) *Wallet {
	// 1. Creating Ed25519 private key (32 bytes) public key (32 bytes)
	w := new(Wallet)
	w.privateKey = privKey
	w.publicKey = pubKey
	w.walletAddress = AddressFromPulicKey(w.publicKey)
	return w
}

func NewWallet() *Wallet {

	privateKey, publicKey := GenerateKeys()
	return creatWallet(publicKey, privateKey)
}

func (w *Wallet) PrivateKey() *PrivateKey {
	return &w.privateKey
}

func (w *Wallet) PrivateKeyStr() string {
	return hex.EncodeToString(w.privateKey)
}

func (w *Wallet) PublicKey() *PublicKey {
	return &w.publicKey
}

func (w *Wallet) PublicKeyStr() string {
	return hex.EncodeToString(w.publicKey)
}

func (w *Wallet) WalletAddress() string {
	return w.walletAddress
}

func (w *Wallet) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		PrivateKey    string `json:"private_key"`
		PublicKey     string `json:"public_key"`
		WalletAddress string `json:"wallet_address"`
	}{
		PrivateKey:    w.PrivateKeyStr(),
		PublicKey:     w.PublicKeyStr(),
		WalletAddress: w.WalletAddress(),
	})
}

func UnmarshalJSON(pbw []byte) *Wallet {
	var (
		privateKey, publicKey string
		walletAddress         string
	)
	wpb := &struct {
		PrivateKey    *string `json:"private_key"`
		PublicKey     *string `json:"public_key"`
		WalletAddress *string `json:"wallet_address"`
	}{
		PrivateKey:    &privateKey,
		PublicKey:     &publicKey,
		WalletAddress: &walletAddress,
	}
	json.Unmarshal(pbw, wpb)
	pk, _ := hex.DecodeString(*wpb.PrivateKey)
	pubk, _ := hex.DecodeString(*wpb.PublicKey)
	return &Wallet{
		privateKey:    PrivateKey(pk),
		publicKey:     PublicKey(pubk),
		walletAddress: *wpb.WalletAddress,
	}
}

func AddressFromPulicKey(pubKey PublicKey) string {
	// 1. Perform SHA-256 hashing on the public key (32 bytes).
	h := NewSHA3Hasher()
	h.Add(pubKey) // public key is 32 bytes long
	digest := h.Digest(nil)
	address := digest[12:]
	return "0x" + hex.EncodeToString(address)
}

func AddressToBytes(address string) []byte {
	address = strings.TrimPrefix(address, "0x")
	if len(address) == 0 {
		return make([]byte, 20)
	}
	bytes, err := hex.DecodeString(address)
	if err != nil {
		panic(err)
	}
	return bytes
}

func HexToBytes(h string) []byte {
	th := strings.TrimPrefix(h, "0x")
	if len(th) == 0 {
		return []byte{}
	}
	if len(th)%2 != 0 {
		th = "0" + th
	}
	bytes, err := hex.DecodeString(th)
	if err != nil {
		panic(err)
	}
	return bytes
}

func HexToBytesNil(h string) []byte {
	th := strings.TrimPrefix(h, "0x")
	if len(th) == 0 {
		return nil
	}
	if len(th)%2 != 0 {
		th = "0" + th
	}
	bytes, err := hex.DecodeString(th)
	if err != nil {
		panic(err)
	}
	return bytes
}

func BytesToAddress(address []byte) string {
	if len(address) != 20 {
		panic(fmt.Errorf("wrong length of address bytes, expected 20 got %d", len(address)))
	}
	return "0x" + hex.EncodeToString(address)
}
func ZeroBytesToAddress(address []byte) string {
	if len(address) == 0 {
		return "0x"
	}
	return BytesToAddress(address)

}

func ZeroHex(length int) string {
	res := "0x"
	for i := 0; i < length; i++ {
		res += "00"
	}
	return res
}

func RandomHex(length int) string {
	res := "0x11"
	for i := 1; i < length; i++ {
		n := rand.Intn(255)
		res += fmt.Sprintf("%X", n)
	}
	return res
}
func LeftPadTo(length int, arr []byte) []byte {
	if len(arr) >= length {
		return arr
	}
	res := make([]byte, length-len(arr), length)
	res = append(res, arr...)
	return res
}

package grape1crypto

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
)

func TestWalletCreation(t *testing.T) {
	wallet := NewWallet()
	fmt.Printf("Private key: %s\n", hex.EncodeToString(*wallet.PrivateKey()))
	fmt.Printf("Public key: %s\n", hex.EncodeToString(*wallet.PublicKey()))
	randomBytes := make([]byte, 120)
	rand.Read(randomBytes)
	fmt.Println(hex.EncodeToString(randomBytes))
	dsa := NewDSA()
	signature := dsa.Sign(*wallet.PrivateKey(), randomBytes)
	fmt.Println(hex.EncodeToString(signature))

	verified := dsa.Verify(*wallet.PublicKey(), signature, randomBytes)

	if !verified {
		t.Errorf("Signature verification failed")
	}
	fmt.Printf("Signature verified for walletid=%s", wallet.WalletAddress())
}

func TestWalletLoading(t *testing.T) {
	wallet := LoadWallet("2bd4e8de88c1578aeb38f8c04f7af4d66c99cc0fe100d06641b4dd4ee0ae3220",
		"8e38269f2cc110b31572e2d9e74aa466c770e82eec34c2f0037e0531822e1e4b")
	fmt.Printf("Private key: %s\n", hex.EncodeToString(*wallet.PrivateKey()))
	fmt.Printf("Public key: %s\n", hex.EncodeToString(*wallet.PublicKey()))
	randomBytes, _ := hex.DecodeString("52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64981855ad8681d0d86d1e91e00167939cb6694d2c422acd208a0072939487f6999eb9d18a44784045d87f3c67cf22746e995af5a25367951baa2ff6cd471c483f15fb90badb37c5821b6d95526a41a9504680b4e7c8b763a1b")
	fmt.Println(hex.EncodeToString(randomBytes))
	dsa := NewDSA()
	signature := dsa.Sign(*wallet.PrivateKey(), randomBytes)
	fmt.Println(hex.EncodeToString(signature))

	verified := dsa.Verify(*wallet.PublicKey(), signature, randomBytes)

	if !verified {
		t.Errorf("Signature verification failed")
	}
	fmt.Printf("Signature verified for walletid=%s", wallet.WalletAddress())
}

//=== RUN   TestWalletCreation
//Private key: 8e38269f2cc110b31572e2d9e74aa466c770e82eec34c2f0037e0531822e1e4b
//Public key: 2bd4e8de88c1578aeb38f8c04f7af4d66c99cc0fe100d06641b4dd4ee0ae3220
//52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64981855ad8681d0d86d1e91e00167939cb6694d2c422acd208a0072939487f6999eb9d18a44784045d87f3c67cf22746e995af5a25367951baa2ff6cd471c483f15fb90badb37c5821b6d95526a41a9504680b4e7c8b763a1b
//b92c458fd8127ac4b9038b1b072da2fad301e1d6265fb9c4fa0231ca5e69cc13a81c647fc5d189d75bb57ef668eb08cd04bfcfc5722682acf62bf3769aa46c0b
//Signature verified: true--- PASS: TestWalletCreation (0.00s)
//PASS

package luna1crypto

import (
	"fmt"
	"testing"
)

func TestGenWallet(t *testing.T) {
	for i := 0; i < 10; i++ {
		w := NewWallet()
		fmt.Println("------------------------------------------")
		fmt.Println("Wallet     : ", w.WalletAddress())
		fmt.Println("Private Key: ", w.PrivateKeyStr())
		fmt.Println("Public Key : ", w.PublicKeyStr())
	}
}

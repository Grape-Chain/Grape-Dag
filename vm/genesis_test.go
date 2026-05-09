package vm

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/VG-Grape/luna/crypto"
)

func TestGenesisAccs(t *testing.T) {
	genesisPubKey := "2bd4e8de88c1578aeb38f8c04f7af4d66c99cc0fe100d06641b4dd4ee0ae3220"
	key, _ := luna1crypto.ParsePublicKey(genesisPubKey)
	fmt.Println(luna1crypto.AddressFromPulicKey(key))
}

func TestGenerateGenesis(t *testing.T) {
	for i := 0; i < 10; i++ {
		wallet := luna1crypto.NewWallet()
		fmt.Println("Private key= ", wallet.PrivateKeyStr())
		fmt.Println("Public key= ", wallet.PublicKeyStr())
		fmt.Println("Acccount=", wallet.WalletAddress())
	}
}

func TestGenerateIds(t *testing.T) {
	publicKeys := []string{
		"940d33cca77608545439c121b679c23b6915f7990ff279cb902c2ccaff2057e3",
		"c13655ea323d8cf8ce392b49a5b5b45ad88f20f53ba41f4a7f2643fa19d69a4f",
		"cf781b50a8f1d79e103126aa885bb43e98de0181a350d48644a65038112901df",
		"7368a34a8b5114db811c5df95e8dd4605567db5ef44783c9a5e073b8cddcecbf",
		"83a6fe0f17a62d85856faf509ac65ae7d88c097a40a3baeb29bfa61f72547652",
		"2fc7d569e485c7616e73918b757117dca042c2db28b3652fe09968c63f583017",
		"753705c4ff27d7f4dff36c49102935665dffa63591d15c827a529e29119cd4aa",
		"6c91f8ca5b683162a111442982a7391108e65e1e15d2bf129c1d1877132bfa50",
		"fa5a823261f413d2ffadf9ce23ea6b023b85a1b797937eab59a21bc50b2daf9f",
		"6fd03841dde673cb166dba06df0ae35a4699f06ee12f0e73367e8908375c97d0"}
	for _, k := range publicKeys {
		byteKey, _ := hex.DecodeString(k)
		address := luna1crypto.AddressFromPulicKey(luna1crypto.PublicKey(byteKey))
		fmt.Println(address)
	}
}

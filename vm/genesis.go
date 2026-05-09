package vm

import (
	"encoding/hex"
	"math/big"
	"strings"
	"time"

	"github.com/VG-Grape/luna/crypto"
)

// Private key=  6f1c1e3f54a6699be61d927f804a191b90912820d89d8d5a8b143e1990fcc0af
// Public key=  940d33cca77608545439c121b679c23b6915f7990ff279cb902c2ccaff2057e3
// Acccount= 0xa074cc0f43bfcd2975089c4adde0415eee305c5b
// Private key=  f2381d878a3fc70342736123298aef78a8f1cd7cfcf8985f267e43d4bdaae330
// Public key=  c13655ea323d8cf8ce392b49a5b5b45ad88f20f53ba41f4a7f2643fa19d69a4f
// Acccount= 0x8825855986980f82d4e7ad8243fcc99bdb14eebf
// Private key=  2cdf28f1c690924087ae585742869fb3096d885d13ff05680a58bec631541642
// Public key=  cf781b50a8f1d79e103126aa885bb43e98de0181a350d48644a65038112901df
// Acccount= 0x68710549f14b7025985d7e4851f9c928e26c118f
// Private key=  017c342c585b8faf18490f3da9bb55051530ae4474ec9d6abc25ae92efa6971c
// Public key=  7368a34a8b5114db811c5df95e8dd4605567db5ef44783c9a5e073b8cddcecbf
// Acccount= 0xb131dc4f56642f42fc4e49cd2cbb7166a56a2c03
// Private key=  2b250205eb4254ff7bd070665c42468d78284f679acd8b340030d54a617a4b4f
// Public key=  83a6fe0f17a62d85856faf509ac65ae7d88c097a40a3baeb29bfa61f72547652
// Acccount= 0xe87e33b61ce3635453cf0d2313cd25143e14685c
// Private key=  0719cbe10d80fbd41bb503916ccc67617f2dccf664a9f3a84927fb26e3913769
// Public key=  2fc7d569e485c7616e73918b757117dca042c2db28b3652fe09968c63f583017
// Acccount= 0x86fa136325c2f9677bfb1cb159bee3ed2b72d464
// Private key=  7759f7c71aa0a39160952d5b97882bbc8456d7ff80e896fea02e04144702d012
// Public key=  753705c4ff27d7f4dff36c49102935665dffa63591d15c827a529e29119cd4aa
// Acccount= 0x91ff42945b42aa9bdb8ae9863b937b90fb4262ab
// Private key=  50f58e6943a81f6acf8ddf53d920fb9779827d93a49bb1ef2921444e65f4f3c5
// Public key=  6c91f8ca5b683162a111442982a7391108e65e1e15d2bf129c1d1877132bfa50
// Acccount= 0xdfb0964466064d451fc0a2dfeaa3415a84eaab0a
// Private key=  b916cac194fbcddcfb297a4f8fa91fea910d5820f17e6d146d71941f5ef5b688
// Public key=  fa5a823261f413d2ffadf9ce23ea6b023b85a1b797937eab59a21bc50b2daf9f
// Acccount= 0x076995fcc7ccbd473fac269814002b8007c739c5
// Private key=  3af24d3cbf08debde55b18aa812eaeb471ee240459a74ae05b4a1403f89fa5c6
// Public key=  6fd03841dde673cb166dba06df0ae35a4699f06ee12f0e73367e8908375c97d0
// Acccount= 0x8bc5a9c61ee2045dfd100751e11b3ff918b2eea4

var GenesisWallets = []GenesisWallet{
	{Id: "0xd09ec4a81cde61b57de012d3fe80beae3f28fb68", PublicKey: "940d33cca77608545439c121b679c23b6915f7990ff279cb902c2ccaff2057e3", PrivateKey: "6f1c1e3f54a6699be61d927f804a191b90912820d89d8d5a8b143e1990fcc0af"},
	{Id: "0x959cc0177d3cf38cb04104f40b1d66d05beb6edb", PublicKey: "c13655ea323d8cf8ce392b49a5b5b45ad88f20f53ba41f4a7f2643fa19d69a4f", PrivateKey: "f2381d878a3fc70342736123298aef78a8f1cd7cfcf8985f267e43d4bdaae330"},
	{Id: "0xa572ee3b050a2c3a287a165a9752cbc8883a45d0", PublicKey: "cf781b50a8f1d79e103126aa885bb43e98de0181a350d48644a65038112901df", PrivateKey: "2cdf28f1c690924087ae585742869fb3096d885d13ff05680a58bec631541642"},
	{Id: "0x4728f954b187e7397b4fedcf9414d167174c3c00", PublicKey: "7368a34a8b5114db811c5df95e8dd4605567db5ef44783c9a5e073b8cddcecbf", PrivateKey: "017c342c585b8faf18490f3da9bb55051530ae4474ec9d6abc25ae92efa6971c"},
	{Id: "0x536aa59b4679c85f2db2d16aab61bd18fd87baf4", PublicKey: "83a6fe0f17a62d85856faf509ac65ae7d88c097a40a3baeb29bfa61f72547652", PrivateKey: "2b250205eb4254ff7bd070665c42468d78284f679acd8b340030d54a617a4b4f"},
	{Id: "0x1c8d0f5c52ae498aaa2f411fd069ca2370915e4d", PublicKey: "2fc7d569e485c7616e73918b757117dca042c2db28b3652fe09968c63f583017", PrivateKey: "0719cbe10d80fbd41bb503916ccc67617f2dccf664a9f3a84927fb26e3913769"},
	{Id: "0x15630412d071b74a1cff19d152b41f7aba2b4acd", PublicKey: "753705c4ff27d7f4dff36c49102935665dffa63591d15c827a529e29119cd4aa", PrivateKey: "7759f7c71aa0a39160952d5b97882bbc8456d7ff80e896fea02e04144702d012"},
	{Id: "0x9ba37f599942fe6b67d46cf210ee95716487cae7", PublicKey: "6c91f8ca5b683162a111442982a7391108e65e1e15d2bf129c1d1877132bfa50", PrivateKey: "50f58e6943a81f6acf8ddf53d920fb9779827d93a49bb1ef2921444e65f4f3c5"},
	{Id: "0xd6756c1b5ac45ba367b15f95708854de527c3002", PublicKey: "fa5a823261f413d2ffadf9ce23ea6b023b85a1b797937eab59a21bc50b2daf9f", PrivateKey: "b916cac194fbcddcfb297a4f8fa91fea910d5820f17e6d146d71941f5ef5b688"},
	{Id: "0xc34f7f7eec92a981d949ef75a624b92b38eff853", PublicKey: "6fd03841dde673cb166dba06df0ae35a4699f06ee12f0e73367e8908375c97d0", PrivateKey: "3af24d3cbf08debde55b18aa812eaeb471ee240459a74ae05b4a1403f89fa5c6"},
}

var GenesisAccounts map[string]LnAccount

type GenesisWallet struct {
	Id         string
	PrivateKey string
	PublicKey  string
}

func (w GenesisWallet) LunaCryptoWallet() *luna1crypto.Wallet {
	return luna1crypto.LoadWallet(w.PublicKey, w.PrivateKey)
}

type LnAccount struct {
	Id        string
	Balance   big.Int
	Created   time.Time
	Nonce     big.Int
	PublicKey string
}

func (ln LnAccount) Exists() bool {
	return ln.Id != ""
}

func (ln LnAccount) ByteAddress() []byte {
	trimmedId := strings.TrimPrefix(ln.Id, "0x")
	bytes, err := hex.DecodeString(trimmedId)
	if err != nil {
		panic(err)
	}
	return bytes
}

func init() {
	balance := big.NewInt(0)
	balance.SetString("1000000000000000000000000000", 10)
	GenesisAccounts = map[string]LnAccount{}
	for _, gw := range GenesisWallets {
		acc := LnAccount{
			Id:        gw.Id,
			Balance:   *balance,
			Created:   time.Now(),
			Nonce:     *big.NewInt(0),
			PublicKey: gw.PublicKey,
		}
		GenesisAccounts[acc.Id] = acc
	}
}

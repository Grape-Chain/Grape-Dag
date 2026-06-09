# GrapeOne Crypto operations
This module incapsulate all cryptographical operations which are being used in GrapeOne project. Including wallets sample impl

## Specification
ECC: Ed25519 RFC 8032

HASH: SHA-256

Address: Bitcoin P2PKH (legacy)


## Overview

`grape1crypto` offers a unified way to sign and verify messages using its own simple API which protects calling code from specific crypto vendor using.

## Import module



```go
import (
    grape1crypto "github.com/Grape-Chain/Grape-Dag/crypto"
)

func main() {
    wallet := grape1crypto.NewWallet()
}
```

Generate Keys

```golang
wallet := grape1crypto.NewWallet()
pk := wallet.PrivateKey() // private key 32 bytes (seed), example: 8e38269f2cc110b31572e2d9e74aa466c770e82eec34c2f0037e0531822e1e4b
pubk := wallet.PublicKey() // public key 32 bytes, example: 2bd4e8de88c1578aeb38f8c04f7af4d66c99cc0fe100d06641b4dd4ee0ae3220
address := wallet.Address()// bitcoin address with checksum, example: 19crp6kZSJMcjiUzd5qotYfiqy9YuMHLZL
```


Load Keys
```golang
// first is public key, second is private
wallet := grape1crypto.LoadWallet("2bd4e8de88c1578aeb38f8c04f7af4d66c99cc0fe100d06641b4dd4ee0ae3220", 
"8e38269f2cc110b31572e2d9e74aa466c770e82eec34c2f0037e0531822e1e4b")
```

Sign
```golang
signature := grape1crypto.NewDSA().Sign(*wallet.PrivateKey(), randomBytes)
```
Verify

```golang
verified := grape1crypto.NewDSA().Verify(*wallet.PublicKey(), signature, randomBytes)
if verified {
    // Hoorah Signature verified successfully
}
```

Hashing

```golang
    // Using SHA-256 grape1crypto wrapper
    bytesToHash, _ := hex.DecodeString("3a48c09448e6d63e8af03838e0eccb5c9e506bddf6b1706b8cb0ba84fe3fa1d23a48c09448e6d63e8af03838e0eccb5c9e506bddf6b1706b8cb0ba84fe3fa1d2")
	hash := grape1crypto.NewHasher()
	hash.Add(bytesToHash)
	actualHash := hex.EncodeToString(hash.Digest(nil)) // hash = 51075006e31a5f33696394ab289af7010c76ee8700e5a74202e9870ee3c8bfa3
```
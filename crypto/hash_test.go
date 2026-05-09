package luna1crypto

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func TestSha256(t *testing.T) {
	bytesToHash, _ := hex.DecodeString("3a48c09448e6d63e8af03838e0eccb5c9e506bddf6b1706b8cb0ba84fe3fa1d23a48c09448e6d63e8af03838e0eccb5c9e506bddf6b1706b8cb0ba84fe3fa1d2")
	hash := NewHasher()
	hash.Add(bytesToHash)
	actualHash := hex.EncodeToString(hash.Digest(nil))
	fmt.Printf("Hash: %s", actualHash)

	expectedHash := "51075006e31a5f33696394ab289af7010c76ee8700e5a74202e9870ee3c8bfa3"
	if actualHash != expectedHash {
		t.Errorf("Calculated hash is not compatible with other languages results, expected: %s, actual: %s",
			expectedHash, actualHash)
	}
}

func TestKeccak256(t *testing.T) {
	bytesToHash, _ := hex.DecodeString("536f6d6520737472696e6720746f2068617368")
	hash := NewSHA3Hasher()
	hash.Add(bytesToHash)
	actualHash := hex.EncodeToString(hash.Digest(nil))
	fmt.Printf("Hash: %s", actualHash)

	expectedHash := "1472341e8646578e6a7a933a157795450e747dbab233b3fca0be0dd6b606802d"
	if actualHash != expectedHash {
		t.Errorf("Calculated hash is not compatible with other languages results, expected: %s, actual: %s",
			expectedHash, actualHash)
	}
}

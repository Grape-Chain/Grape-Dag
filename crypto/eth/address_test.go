package eth

import "testing"

func Test_ParseAddress(t *testing.T) {
	address, err := ParseEthAddress("70207524f48df8de24bfa557c5c1d27831d36afb")
	if err != nil {
		t.Errorf("Unable to parse address from 20 hex-encoded bytes: %s", err.Error())
	}
	if address.Hex() != "0x70207524f48df8de24bfa557c5c1d27831d36afb" {
		t.Errorf("Bad address to hex transformation, got %s, required: %s", address.Hex(), "0x70207524f48df8de24bfa557c5c1d27831d36afb")
	}
}

// Expected values computed independently of go-ethereum, as
// keccak256(rlp([sender, nonce]))[12:]. The previous expectation here was the
// address from the parse test above, which is unrelated to any deployment.
func Test_CreateAddressFromNonce(t *testing.T) {
	caller, err := ParseEthAddress("0xd09ec4a81cde61b57de012d3fe80beae3f28fb68")
	if err != nil {
		t.Fatalf("parsing the caller address: %s", err.Error())
	}
	for _, c := range []struct {
		nonce int64
		want  string
	}{
		{0, "0x8803a8c25ec1cf1325eb1d348b293b5473c04f34"},
		{1, "0x0c57beb251b24884c497f984a12038ec890351b5"},
		{2, "0xd2e659a6dddeb5cdb99dc3506775c93fc535fff3"},
		{5, "0x16289e480ca1b7effe51767ec276416b8c9360ff"},
	} {
		if got := EthAddressFromCaller(caller, c.nonce).Hex(); got != c.want {
			t.Errorf("contract address for nonce %d = %s, want %s", c.nonce, got, c.want)
		}
	}
}

// Every deployment by an account must land on its own address. The nonce used to
// be ignored, so they all collided on the nonce-zero address.
func Test_CreateAddressVariesWithNonce(t *testing.T) {
	caller, _ := ParseEthAddress("0xd09ec4a81cde61b57de012d3fe80beae3f28fb68")
	seen := map[string]int64{}
	for nonce := int64(0); nonce < 8; nonce++ {
		addr := EthAddressFromCaller(caller, nonce).Hex()
		if prev, clash := seen[addr]; clash {
			t.Fatalf("nonce %d and nonce %d both deploy to %s", prev, nonce, addr)
		}
		seen[addr] = nonce
	}
}

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

func Test_CreateAddressFromNonce(t *testing.T) {
	caller, _ := ParseEthAddress("0xd09ec4a81cde61b57de012d3fe80beae3f28fb68")
	nonce := 0
	contractAddress := EthAddressFromCaller(caller, int64(nonce))
	if contractAddress.Hex() != "0x70207524f48df8de24bfa557c5c1d27831d36afb" {
		t.Errorf("Bad contract address creation from caller address and nonce, expected: %s, got %s", "0x70207524f48df8de24bfa557c5c1d27831d36afb", contractAddress.Hex())
	}

}
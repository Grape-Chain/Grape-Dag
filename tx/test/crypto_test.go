package test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestAddressEth(t *testing.T) {
	actual := common.Hex2Bytes("75704b2f3334eea055803fb410995a87d610c5ee")
	newAddr := crypto.CreateAddress(common.BytesToAddress(actual), 0)
	expected := "0x2e57dd414fB6d16B69A8d1D6Cf844a676CD22051"
	if expected != newAddr.Hex() {
		t.Errorf("Generated contract address mismatch: expected %s, actual %s", expected, actual)
	}
}

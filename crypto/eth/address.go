package eth

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type EthAddress []byte

func (a EthAddress) Hex() string {
	return "0x" + hex.EncodeToString(a)
}


func ParseEthAddress(addressString string ) (EthAddress, error){
	addressString = strings.TrimPrefix(addressString, "0x")
	bytes, err := hex.DecodeString(addressString)
	if err != nil {
		return EthAddress{}, err
	}
	if len(bytes) != 20 {
		return EthAddress{}, fmt.Errorf("Wrong eth address length, required %d bytes, got %d", 20, len(bytes))
	}
	return  EthAddress(bytes), nil
}

func EthAddressFromCaller(caller EthAddress, callerNonce int64) EthAddress {
	newAddr := crypto.CreateAddress(common.HexToAddress(caller.Hex()), 0)
	return newAddr.Bytes()
}

func intToBytes(i int64) []byte {
	return big.NewInt(i).Bytes()
}


func leftPad32(bytes []byte) ([]byte, error) {
	if len(bytes) > 32 {
		return nil, fmt.Errorf("Input array has size more than 32")
	}
	res := make([]byte, 32, 32)
	i := 32 - len(bytes)
	for j := 0; j < len(bytes); j++ {
		res[i] = bytes[j]
		i++
	}
	return res, nil
}
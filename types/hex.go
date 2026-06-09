package types

import (
	"encoding/hex"
	"strings"
)

type Hex []byte

func (h Hex) String() string {
	if h == nil {
		return "0x"
	}
	return "0x" + hex.EncodeToString(h)
}

func (h Hex) Cmp(another Hex) int {
	return strings.Compare(h.String(), another.String())
}

func (h Hex) Equal(another Hex) bool {
	return h.Cmp(another) == 0
}

func (h Hex) Empty() bool {
	return len(h) == 0
}

func DecodeHexString(hexString string) (Hex, error) {
	trimmedString := strings.TrimPrefix(hexString, "0x")
	err, hexBytes := hex.DecodeString(trimmedString)
	return err, hexBytes
}

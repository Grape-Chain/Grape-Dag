package services

import (
	"testing"
)

// 0x08c379a0                     // Function selector for Error(string)
// 0x0000000000000000000000000000000000000000000000000000000000000020 // Data offset
// 0x000000000000000000000000000000000000000000000000000000000000001a // String length
// 0x4e6f7420656e6f7567682045746865722070726f76696465642e000000000000 // String data
const solidityErr string = "0x08c379a00000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000001a4e6f7420656e6f7567682045746865722070726f76696465642e000000000000"
func TestParseSolidityVmError(t *testing.T) {
	err := parseVmError(solidityErr)

	expected := "Not enough Ether provided."
	if err.Error() != expected {
		t.Errorf("Parsed solidity error: %s doesn't match to expected %s", err.Error(), expected)
	}
}

func TestParseSystemVmError(t *testing.T) {
	err := parseVmError("unable to write to file")

	expected := "system VM error during tx execution: unable to write to file"
	if err.Error() != expected {
		t.Errorf("Parsed VM error: '%s' doesn't match to expected '%s'", err.Error(), expected)
	}
}
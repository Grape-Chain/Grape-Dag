package config

import "testing"

func TestValidateAddress(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"zero address", ZERO_ADDRESS, false},
		{"20-byte with prefix", "0xd09ec4a81cde61b57de012d3fe80beae3f28fb68", false},
		{"20-byte without prefix", "d09ec4a81cde61b57de012d3fe80beae3f28fb68", false},
		{"uppercase prefix", "0Xd09ec4a81cde61b57de012d3fe80beae3f28fb68", false},
		// the previous default was 15 bytes and made the smart-contract stage fail
		{"15-byte address", "0xac1214a3c58090a516ade112cf1198", true},
		{"too long", "0xd09ec4a81cde61b57de012d3fe80beae3f28fb6812", true},
		{"not hex", "0xzzec4a81cde61b57de012d3fe80beae3f28fb68z", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAddress(tc.addr)
			if tc.wantErr && err == nil {
				t.Fatalf("validateAddress(%q) = nil, want an error", tc.addr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateAddress(%q) = %s, want nil", tc.addr, err.Error())
			}
		})
	}
}

// The coinbase default is parsed as a 20-byte address on every pin; a malformed
// default previously surfaced as a failure deep in the smart-contract stage.
func TestZeroAddressDefaultIsAValidAddress(t *testing.T) {
	if err := validateAddress(ZERO_ADDRESS); err != nil {
		t.Fatalf("ZERO_ADDRESS is not a valid address: %s", err.Error())
	}
	if len(ZERO_ADDRESS) != 2+ADDRESS_BYTE_LEN*2 {
		t.Fatalf("ZERO_ADDRESS has length %d, want %d", len(ZERO_ADDRESS), 2+ADDRESS_BYTE_LEN*2)
	}
}

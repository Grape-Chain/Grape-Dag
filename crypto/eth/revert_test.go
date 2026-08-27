package eth

import (
	"strings"
	"testing"
)

// The canonical Error(string) encoding: selector, offset, length, padded bytes.
const notEnoughEther = "0x08c379a0" +
	"0000000000000000000000000000000000000000000000000000000000000020" +
	"000000000000000000000000000000000000000000000000000000000000001a" +
	"4e6f7420656e6f7567682045746865722070726f76696465642e000000000000"

func TestParseRevertReadsAnErrorString(t *testing.T) {
	got := ParseRevert(notEnoughEther)
	if !got.Decoded {
		t.Fatalf("a canonical Error(string) payload was not decoded")
	}
	if got.Reason != "Not enough Ether provided." {
		t.Fatalf("reason = %q, want %q", got.Reason, "Not enough Ether provided.")
	}
	if got.Error() != got.Reason {
		t.Fatalf("Error() = %q, want the reason", got.Error())
	}
}

func TestParseRevertReadsAPanicCode(t *testing.T) {
	// Panic(uint256) with code 0x11: arithmetic overflow
	payload := "0x4e487b71" + "0000000000000000000000000000000000000000000000000000000000000011"
	got := ParseRevert(payload)
	if !got.Decoded {
		t.Fatalf("a Panic(uint256) payload was not decoded")
	}
	if !strings.Contains(got.Reason, "overflow") {
		t.Fatalf("reason = %q, want it to mention overflow", got.Reason)
	}
}

// Payloads come from the VM, so nothing here may panic, and an unreadable
// payload has to come back as a value rather than an empty reason.
func TestParseRevertSurvivesMalformedPayloads(t *testing.T) {
	for _, payload := range []string{
		"",
		"0x",
		"0xzz",
		"0x08",
		"0x08c379a0",        // selector only
		"0x08c379a0" + "00", // truncated offset
		"0x08c379a0" + strings.Repeat("f", 64) + strings.Repeat("0", 64),        // offset far past the end
		"0x08c379a0" + strings.Repeat("0", 62) + "20" + strings.Repeat("f", 64), // absurd length
		"0x4e487b71",                           // panic selector with no code
		"0xdeadbeef" + strings.Repeat("0", 64), // unknown selector
	} {
		got := ParseRevert(payload)
		if got.Reason == "" {
			t.Errorf("payload %q decoded to an empty reason", payload)
		}
		if got.Raw != strings.TrimSpace(payload) {
			t.Errorf("payload %q lost its raw form (%q)", payload, got.Raw)
		}
	}
}

func TestIsRevertPayload(t *testing.T) {
	if !IsRevertPayload("0x08c379a0") {
		t.Errorf("hex payload not recognised")
	}
	if IsRevertPayload("unable to write to file") {
		t.Errorf("a VM message was mistaken for a revert payload")
	}
}

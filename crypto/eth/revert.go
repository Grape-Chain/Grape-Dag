package eth

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

/*
Decoding revert payloads from the VM.

When a contract reverts with a reason, the EVM returns ABI-encoded call data
rather than a message. Two shapes are standard:

	Error(string)    selector 0x08c379a0, then a 32-byte offset, a 32-byte
	                 length, and the UTF-8 bytes padded to a 32-byte boundary
	Panic(uint256)   selector 0x4e487b71, then a 32-byte code, produced by
	                 compiler-inserted checks such as an assertion or an
	                 arithmetic overflow

This decoding is applied to output from the VM, so it must not panic: a
malformed payload has to come back as a value. An earlier version parsed these
inline and panicked on anything unexpected, which turned bad contract output
into a dead node.
*/

// Error(string) and Panic(uint256) selectors.
const (
	revertStringSelector = "08c379a0"
	revertPanicSelector  = "4e487b71"
)

// panicReasons - the meanings Solidity assigns to Panic(uint256) codes.
var panicReasons = map[uint64]string{
	0x00: "generic compiler panic",
	0x01: "assertion failed",
	0x11: "arithmetic overflow or underflow",
	0x12: "division or modulo by zero",
	0x21: "invalid value converted to an enum",
	0x22: "access to an incorrectly encoded storage byte array",
	0x31: "pop on an empty array",
	0x32: "array index out of bounds",
	0x41: "out of memory",
	0x51: "call to an uninitialised internal function",
}

// RevertError - a revert carrying a decoded reason where one could be read.
type RevertError struct {
	// Reason - the decoded message, or the raw payload when it cannot be read.
	Reason string
	// Raw - the payload as received, for callers that want to re-encode it.
	Raw string
	// Decoded - whether Reason came from a recognised encoding.
	Decoded bool
}

func (e RevertError) Error() string { return e.Reason }

// ParseRevert - decode a hex revert payload. Anything unrecognised comes back
// with Decoded false and the raw payload as the reason, which is more use to
// whoever reads the receipt than an empty string.
func ParseRevert(payload string) RevertError {
	raw := strings.TrimSpace(payload)
	body := strings.TrimPrefix(strings.TrimPrefix(raw, "0x"), "0X")
	out := RevertError{Reason: raw, Raw: raw}

	if body == "" {
		// A revert with no data at all: `revert()` with no reason, or an
		// invalid opcode. There is nothing to decode, but the caller still
		// needs something to put in a receipt.
		out.Reason = "reverted without a reason"
		return out
	}
	data, err := hex.DecodeString(body)
	if err != nil || len(data) < 4 {
		return out
	}
	selector := hex.EncodeToString(data[:4])
	args := data[4:]

	switch selector {
	case revertStringSelector:
		if msg, ok := decodeRevertString(args); ok {
			out.Reason = msg
			out.Decoded = true
		}
	case revertPanicSelector:
		if len(args) >= 32 {
			code := new(big.Int).SetBytes(args[:32])
			if code.IsUint64() {
				if reason, known := panicReasons[code.Uint64()]; known {
					out.Reason = fmt.Sprintf("panic: %s (0x%02x)", reason, code.Uint64())
					out.Decoded = true
					return out
				}
			}
			out.Reason = fmt.Sprintf("panic: code 0x%s", code.Text(16))
			out.Decoded = true
		}
	}
	return out
}

// decodeRevertString - read the offset, length and bytes of an ABI-encoded
// string argument, bounds-checking every step.
func decodeRevertString(args []byte) (string, bool) {
	if len(args) < 64 {
		return "", false
	}
	offset := new(big.Int).SetBytes(args[:32])
	if !offset.IsUint64() {
		return "", false
	}
	at := offset.Uint64()
	if at > uint64(len(args)) || uint64(len(args))-at < 32 {
		return "", false
	}
	length := new(big.Int).SetBytes(args[at : at+32])
	if !length.IsUint64() {
		return "", false
	}
	size := length.Uint64()
	start := at + 32
	if size > uint64(len(args))-start {
		return "", false
	}
	return string(args[start : start+size]), true
}

// IsRevertPayload - whether this looks like hex the VM returned rather than a
// message it wrote.
func IsRevertPayload(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "0x")
}

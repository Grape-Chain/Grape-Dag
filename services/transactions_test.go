package services

import (
	"testing"

	txqueue "github.com/Grape-Chain/Grape-Dag/queues"
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

// The payment path used to report a hard-coded zero fee. It now reports what the
// payment was charged, which while fees are off - the shipped default, and the
// state of a test binary with no configuration - is still nought. The rest of the
// assertion is that the payment is still queued for diffusion and that finding
// the height does not need a ledger: this runs with no DAG at all.
func TestAPaymentIsQueuedAndReportsTheFeeItWasCharged(t *testing.T) {
	before := txqueue.GetPublishQueue().Len()

	execResult, err := executePaymentTx(payment(0, 0))
	if err != nil {
		t.Fatalf("a payment offering no fuel was refused with fees off: %s", err)
	}
	if !execResult.Successful {
		t.Error("a payment queued for diffusion was reported unsuccessful")
	}
	if execResult.GasUsed != 0 {
		t.Errorf("fee reported is %d, want 0 while fees are off", execResult.GasUsed)
	}
	if after := txqueue.GetPublishQueue().Len(); after != before+1 {
		t.Errorf("publish queue went from %d to %d, want one more", before, after)
	}
}

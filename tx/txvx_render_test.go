package tx

import (
	"math/big"
	"testing"
)

// String() is called from log lines on the publish path, which means it is
// called on transactions that arrived over the network and have not been checked
// yet. It used to render the sender through the strict address conversion, which
// panics on anything that is not twenty bytes - so describing a malformed
// transaction crashed the node that received it, and it crashed inside a Debugf
// argument, whatever the log level was set to.
func TestRenderingATransactionWithABadSenderDoesNotPanic(t *testing.T) {
	for _, sender := range [][]byte{nil, {}, make([]byte, 1), make([]byte, 19), make([]byte, 21)} {
		txv := &Txv1{
			Tx_Type:    PAYMENT,
			Chain_Type: PRIVATE_TESTNET,
			Sender:     sender,
			Amount:     big.NewInt(1).Bytes(),
			Fuel_Limit: big.NewInt(0).Bytes(),
			Fuel_Price: big.NewInt(0).Bytes(),
		}
		if got := txv.String(); got == "" {
			t.Errorf("a %d-byte sender rendered as the empty string, so the marshaller failed", len(sender))
		}
	}
}

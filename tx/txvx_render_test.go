package tx

import (
	"bytes"
	"math/big"
	"sync"
	"testing"
	"time"

	grape1crypto "github.com/Grape-Chain/Grape-Dag/crypto"
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

// Verify must be a read. The subscriber checks signatures on several goroutines
// at once now, and while each holds its own record today, a Verify that blanked
// and restored the receiver's signature field would corrupt any transaction two
// goroutines ever shared - and would do it silently, since the window is between
// the blank and the restore.
func TestVerifyingATransactionLeavesItUnchanged(t *testing.T) {
	txv := signedTestTx(t)
	before := append([]byte(nil), txv.Signature...)

	if err := txv.Verify(); err != nil {
		t.Fatalf("a freshly signed transaction did not verify: %s", err)
	}
	if !bytes.Equal(txv.Signature, before) {
		t.Errorf("the signature changed across Verify: %x -> %x", before, txv.Signature)
	}

	// Concurrently, on one shared transaction. Under the blank-and-restore
	// version this reports a race and intermittently fails to verify.
	var wg sync.WaitGroup
	errs := make([]error, 16)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs[i] = txv.Verify() }(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d failed to verify a transaction that verifies on its own: %s", i, err)
		}
	}
	if !bytes.Equal(txv.Signature, before) {
		t.Errorf("the signature did not survive concurrent verification: %x -> %x", before, txv.Signature)
	}
}

// Tampering with any covered field has to be refused. Written as a table rather
// than one case because the payload is built from the whole transaction, and a
// change to any part of it must change the hash.
//
// Note what this does NOT cover: Verify's marshalling-error path, which used to
// return nil - success - for a transaction it could not hash. That path is fixed
// but is not exercised here, because a well-formed Txv1 cannot practically be
// made to fail proto.Marshal, and a test that reached the error some other way
// would be testing the marshaller rather than Verify. An earlier version of this
// test claimed to cover it by setting an out-of-range timestamp; that produced
// an error, but a signature-mismatch error, so it passed for the wrong reason. A
// mutation that put "return nil" back survived it, which is how it was caught.
func TestTamperingWithAnySignedFieldIsRefused(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(*Txv1)
	}{
		{"the amount", func(x *Txv1) { x.Amount = big.NewInt(999).Bytes() }},
		{"the recipient", func(x *Txv1) { x.Recepient = grape1crypto.AddressToBytes(grape1crypto.NewWallet().WalletAddress()) }},
		{"the nonce", func(x *Txv1) { x.Nonce++ }},
		{"the timestamp", func(x *Txv1) { x.Timestamp = x.Timestamp.Add(time.Second) }},
		{"the fuel price", func(x *Txv1) { x.Fuel_Price = big.NewInt(7).Bytes() }},
		{"the signature itself", func(x *Txv1) { x.Signature[0] ^= 0xff }},
	}
	for _, c := range cases {
		txv := signedTestTx(t)
		if err := txv.Verify(); err != nil {
			t.Fatalf("%s: the control failed - a freshly signed transaction did not verify: %s", c.name, err)
		}
		c.break_(txv)
		if err := txv.Verify(); err == nil {
			t.Errorf("a transaction with %s altered after signing was accepted", c.name)
		}
	}
}

func signedTestTx(t *testing.T) *Txv1 {
	t.Helper()
	w := grape1crypto.NewWallet()
	txv := NewTxv1(PRIVATE_TESTNET)
	txv.Tx_Type = PAYMENT
	txv.Sender = grape1crypto.AddressToBytes(w.WalletAddress())
	txv.Recepient = grape1crypto.AddressToBytes(grape1crypto.NewWallet().WalletAddress())
	txv.Sender_Pubk = *w.PublicKey()
	txv.Amount = big.NewInt(1).Bytes()
	txv.Fuel_Limit = big.NewInt(0).Bytes()
	txv.Fuel_Price = big.NewInt(0).Bytes()
	txv.Timestamp = time.Unix(1700000000, 0).UTC()
	txv.Sign(w.PrivateKey())
	return txv
}

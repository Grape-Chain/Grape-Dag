package test

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	grape1crypto "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
)

// A payment produced by web/wallet (cmd/walletwasm compiled to WebAssembly) and
// captured verbatim from a browser-equivalent run under Node. The node must
// accept it exactly as it arrives on POST /api/rest/transactions, so this is the
// contract between the bundled wallet and the ledger: if a change to the tx
// format, the hashing or the signature scheme breaks browser-signed payments,
// this test fails.
//
// Regenerate with: node scripts/wallet_sign_probe.mjs
const (
	wasmSignedPaymentHex = "0x100222204c1da4b40f709db45aefc028532f8d603caa93b7cab313706215da9c5487e815" +
		"2a14db63285447b5225943b9d83e913c46e7a9d2187132144cfd4851853c4c4ca9e7493d67c3c484f13ea66a" +
		"3a080de0b6b3a764000040be97024a0c08f9b0c1d4061080f9f3df026a40e474f75ab7d134ef33f37465f54f" +
		"07a95680ba3a7dc01468b73d9b3bf8ec40a22889fa090a2ae555140e1a6e3850270227880373971d61e8fdf8" +
		"2e4c8d164001"
	wasmSignedPaymentFrom   = "0xdb63285447b5225943b9d83e913c46e7a9d21871"
	wasmSignedPaymentTo     = "0x4cfd4851853c4c4ca9e7493d67c3c484f13ea66a"
	wasmSignedPaymentAmount = "1000000000000000000"
)

func TestWasmSignedPaymentIsAcceptedByTheNode(t *testing.T) {
	raw, err := hex.DecodeString(strings.TrimPrefix(wasmSignedPaymentHex, "0x"))
	if err != nil {
		t.Fatalf("fixture is not valid hex: %s", err.Error())
	}

	// this is the exact decode path SendRawTransaction takes
	parsed, err := tx.ParseTransaction(raw)
	if err != nil {
		t.Fatalf("node cannot parse a wallet-signed payment: %s", err.Error())
	}

	if err := parsed.VerifySignature(); err != nil {
		t.Fatalf("node rejects the wallet's signature: %s", err.Error())
	}

	if got := parsed.GetTransactionType(); got != tx.PAYMENT {
		t.Errorf("tx type = %v, want PAYMENT", got)
	}
	if got := grape1crypto.BytesToAddress(parsed.GetSender()); got != wasmSignedPaymentFrom {
		t.Errorf("sender = %s, want %s", got, wasmSignedPaymentFrom)
	}
	if got := grape1crypto.BytesToAddress(parsed.GetRecipient()); got != wasmSignedPaymentTo {
		t.Errorf("recipient = %s, want %s", got, wasmSignedPaymentTo)
	}
	wantAmount, _ := new(big.Int).SetString(wasmSignedPaymentAmount, 10)
	if got := new(big.Int).SetBytes(parsed.GetAmount().Bytes()); got.Cmp(wantAmount) != 0 {
		t.Errorf("amount = %s, want %s", got.String(), wantAmount.String())
	}

	// payments must carry no fuel today: SendRawTransaction rejects any payment
	// with a non-zero fuel limit or price
	if fl := new(big.Int).SetBytes(parsed.GetFuelLimit().Bytes()); fl.Sign() != 0 {
		t.Errorf("fuel limit = %s, want 0 for a payment", fl.String())
	}
	if fp := new(big.Int).SetBytes(parsed.GetFuelPrice().Bytes()); fp.Sign() != 0 {
		t.Errorf("fuel price = %s, want 0 for a payment", fp.String())
	}
}

// The address the wallet shows must be the one the ledger credits, i.e. it has
// to be derived from the public key the same way on both sides.
func TestWalletAddressDerivationMatchesTheSignedSender(t *testing.T) {
	raw, _ := hex.DecodeString(strings.TrimPrefix(wasmSignedPaymentHex, "0x"))
	parsed, err := tx.ParseTransaction(raw)
	if err != nil {
		t.Fatalf("parsing fixture: %s", err.Error())
	}
	derived := grape1crypto.AddressFromPulicKey(grape1crypto.PublicKey(parsed.GetSenderPubKey()))
	if derived != wasmSignedPaymentFrom {
		t.Fatalf("address derived from the tx public key = %s, want %s", derived, wasmSignedPaymentFrom)
	}
}

// Tampering must invalidate the signature - otherwise the wallet's guarantees
// mean nothing.
func TestTamperedWasmPaymentFailsVerification(t *testing.T) {
	raw, _ := hex.DecodeString(strings.TrimPrefix(wasmSignedPaymentHex, "0x"))
	parsed, err := tx.ParseTransaction(raw)
	if err != nil {
		t.Fatalf("parsing fixture: %s", err.Error())
	}
	native, ok := parsed.(*tx.Txv1)
	if !ok {
		t.Fatalf("fixture decoded as %T, want *tx.Txv1", parsed)
	}
	// pay one unit more to the same recipient
	tampered := new(big.Int).SetBytes(native.Amount)
	tampered.Add(tampered, big.NewInt(1))
	native.Amount = tampered.Bytes()

	if err := native.VerifySignature(); err == nil {
		t.Fatalf("a tampered amount still verified")
	}
}

//go:build js && wasm

// Command walletwasm is the signing core of the bundled web wallet, compiled to
// WebAssembly.
//
// It exists so the browser produces transactions with the node's own code: the
// wire format is a signed protobuf Txv1, and a second implementation in
// JavaScript would have to stay byte-identical to this one by hand. Reusing the
// crypto, tx and wallet packages makes that impossible to get wrong, and private
// keys never leave the browser.
//
// Build:
//
//	GOOS=js GOARCH=wasm go build -o web/wallet/wallet.wasm ./cmd/walletwasm
//
// The API is installed on globalThis.grapeWallet; every call returns an object
// with either the requested fields or an "error" string.
package main

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"syscall/js"

	grape1crypto "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/wallet"
	"golang.org/x/crypto/ed25519"
	"google.golang.org/protobuf/proto"
)

// privateKeySize - Ed25519 seed length; the wallet stores the seed, matching
// grape1crypto.GenerateKeys.
const privateKeySize = 32

func errResult(format string, args ...interface{}) map[string]interface{} {
	return map[string]interface{}{"error": fmt.Sprintf(format, args...)}
}

// guard - turn a panic into an error result. Several helpers in the crypto and
// wallet packages panic on malformed input (LoadWallet, AddressToBytes), which
// would otherwise abort the whole wasm instance and take the page with it.
func guard(fn func() map[string]interface{}) (result map[string]interface{}) {
	defer func() {
		if r := recover(); r != nil {
			result = errResult("%v", r)
		}
	}()
	return fn()
}

// walletFromPrivateKey - rebuild the full wallet from the stored seed. The
// public key is derived rather than stored, so an imported key cannot disagree
// with its own address.
func walletFromPrivateKey(privHex string) (*grape1crypto.Wallet, error) {
	seed, err := hex.DecodeString(strings.TrimPrefix(privHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("private key is not valid hex: %s", err.Error())
	}
	if len(seed) != privateKeySize {
		return nil, fmt.Errorf("private key must be %d bytes, got %d", privateKeySize, len(seed))
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	return grape1crypto.LoadWallet(hex.EncodeToString(pub), hex.EncodeToString(seed)), nil
}

func describe(w *grape1crypto.Wallet) map[string]interface{} {
	return map[string]interface{}{
		"privateKey": w.PrivateKeyStr(),
		"publicKey":  w.PublicKeyStr(),
		"address":    w.WalletAddress(),
	}
}

// newWallet() -> {privateKey, publicKey, address}
func newWallet(js.Value, []js.Value) interface{} {
	return guard(func() map[string]interface{} {
		return describe(grape1crypto.NewWallet())
	})
}

// importPrivateKey(privateKeyHex) -> {privateKey, publicKey, address}
func importPrivateKey(_ js.Value, args []js.Value) interface{} {
	return guard(func() map[string]interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return errResult("importPrivateKey requires the private key as a hex string")
		}
		w, err := walletFromPrivateKey(args[0].String())
		if err != nil {
			return errResult("%s", err.Error())
		}
		return describe(w)
	})
}

// validateAddress(address) -> {valid: bool}
func validateAddress(_ js.Value, args []js.Value) interface{} {
	return guard(func() map[string]interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return errResult("validateAddress requires an address string")
		}
		return map[string]interface{}{"valid": isAddress(args[0].String())}
	})
}

// isAddress - a 20-byte hex address with the 0x prefix. Deliberately stricter
// than wallet.ValidateAddress, whose regexp is unanchored and so accepts an
// address embedded in surrounding text.
func isAddress(addr string) bool {
	if !strings.HasPrefix(addr, "0x") {
		return false
	}
	raw, err := hex.DecodeString(addr[2:])
	return err == nil && len(raw) == 20
}

// signPayment({privateKey, to, amount, chainType}) -> {encodedTx, hash, from, to, amount}
//
// amount is a decimal string in the smallest unit, so the browser never has to
// put a ledger value through a float.
func signPayment(_ js.Value, args []js.Value) interface{} {
	return guard(func() map[string]interface{} {
		if len(args) < 1 || args[0].Type() != js.TypeObject {
			return errResult("signPayment requires an options object")
		}
		opts := args[0]

		w, err := walletFromPrivateKey(opts.Get("privateKey").String())
		if err != nil {
			return errResult("%s", err.Error())
		}

		to := opts.Get("to").String()
		if !isAddress(to) {
			return errResult("recipient %q is not a 20-byte 0x address", to)
		}

		amountStr := strings.TrimSpace(opts.Get("amount").String())
		amount, ok := new(big.Int).SetString(amountStr, 10)
		if !ok {
			return errResult("amount %q is not a decimal integer", amountStr)
		}
		if amount.Sign() <= 0 {
			return errResult("amount must be greater than zero")
		}

		chainType := tx.PRIVATE_TESTNET
		if ct := opts.Get("chainType"); ct.Type() == js.TypeNumber {
			chainType = tx.ChainType(ct.Int())
		}

		payment := tx.NewTxv1(chainType)
		payment.GeneratePayment(wallet.GenPaymentEx(w, to, amount), uint8(chainType))
		if err := payment.Verify(); err != nil {
			return errResult("refusing to send a transaction that does not verify: %s", err.Error())
		}

		raw, marshalErr := proto.Marshal(payment.MarshalBinary())
		if marshalErr != nil {
			return errResult("encoding transaction: %s", marshalErr.Error())
		}

		return map[string]interface{}{
			"encodedTx": "0x" + hex.EncodeToString(raw),
			"hash":      payment.DefaultStringHash(),
			"from":      w.WalletAddress(),
			"to":        to,
			"amount":    amount.String(),
			"nonce":     fmt.Sprintf("%d", payment.Nonce),
		}
	})
}

func main() {
	api := map[string]interface{}{
		"newWallet":        js.FuncOf(newWallet),
		"importPrivateKey": js.FuncOf(importPrivateKey),
		"validateAddress":  js.FuncOf(validateAddress),
		"signPayment":      js.FuncOf(signPayment),
		"privateKeySize":   privateKeySize,
		"defaultChainType": int(tx.PRIVATE_TESTNET),
	}
	js.Global().Set("grapeWallet", js.ValueOf(api))

	// signal readiness, then block forever so the exported functions stay alive
	if ready := js.Global().Get("onGrapeWalletReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	<-make(chan struct{})
}

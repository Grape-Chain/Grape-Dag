package wallet

import (
	"encoding/json"
	"math/big"
	"regexp"

	utils "github.com/VG-Grape/luna/utils"
	luna_wallet "github.com/VG-Grape/luna/crypto"
)

type Transaction struct {
	senderPrivateKey *luna_wallet.PrivateKey
	senderPublicKey  *luna_wallet.PublicKey
	senderAddress    string
	receiverAddress  string
	amount           *big.Int
}

func (t *Transaction) GetSenderPrivK() *luna_wallet.PrivateKey {
	return t.senderPrivateKey
}

func (t *Transaction) GetSenderPubK() *luna_wallet.PublicKey {
	return t.senderPublicKey
}

func (t *Transaction) GetSenderAddress() string {
	return t.senderAddress
}

func (t *Transaction) GetReceiverAddress() string {
	return t.receiverAddress
}

func (t *Transaction) GetAmount() *big.Int {
	return t.amount
}

func NewTransaction(privateKey *luna_wallet.PrivateKey, publicKey *luna_wallet.PublicKey,
	sender string, recipient string, amount *big.Int) *Transaction {
	return &Transaction{privateKey, publicKey, sender, recipient, amount}
}

func (t *Transaction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Sender   string `json:"sender_address"`
		Receiver string `json:"receiver_address"`
		Amount   []byte `json:"amount"`
	}{
		Sender:   t.senderAddress,
		Receiver: t.receiverAddress,
		Amount:   t.amount.Bytes(),
	})
}

type TransactionRequest struct {
	SenderPrivateKey *string `json:"sender_private_key"`
	SenderAddress    *string `json:"sender_address"`
	ReceiverAddress  *string `json:"receiver_address"`
	SenderPublicKey  *string `json:"sender_public_key"`
	Amount           *string `json:"amount"`
}

func (tr *TransactionRequest) Validate() bool {
	if tr.SenderPrivateKey == nil ||
		tr.SenderAddress == nil ||
		tr.ReceiverAddress == nil ||
		tr.SenderPublicKey == nil ||
		tr.Amount == nil {
		return false
	}
	return true
}

func GenRanWallet() *Transaction {
	sender_wallet := luna_wallet.NewWallet()
	receiver_wallet := luna_wallet.NewWallet()
	tx := NewTransaction(
		sender_wallet.PrivateKey(),
		sender_wallet.PublicKey(),
		sender_wallet.WalletAddress(),
		receiver_wallet.WalletAddress(),
		big.NewInt(int64(utils.RandomUint64())),
	)
	return tx
}

func GenServiceTransaction(w *luna_wallet.Wallet) *Transaction {
	tx := NewTransaction(
		w.PrivateKey(),
		w.PublicKey(),
		w.WalletAddress(),
		w.WalletAddress(),
		big.NewInt(0),
	)
	return tx
}

func GenPaymentTransaction(ws *luna_wallet.Wallet, wr *luna_wallet.Wallet, amount *big.Int) *Transaction {
	tx := NewTransaction(
		ws.PrivateKey(),
		ws.PublicKey(),
		ws.WalletAddress(),
		wr.WalletAddress(),
		amount,
	)
	return tx
}

func GenPaymentEx(ws *luna_wallet.Wallet, wr string, amount *big.Int) *Transaction {
	tx := NewTransaction(
		ws.PrivateKey(),
		ws.PublicKey(),
		ws.WalletAddress(),
		wr,
		amount,
	)
	return tx
}

func ValidateAddress(wa string) bool {
	re := regexp.MustCompile("((0x){1}([a-f0-9]){40})")
	return re.Match([]byte(wa))
}

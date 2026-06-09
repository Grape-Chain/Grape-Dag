package tx

import (
	"errors"
)

type Status string

const (
	Failed     Status = "Failed"
	Successful Status = "Successful"
)

type ConfirmedTx struct {
	IdentifiableTx
	UsedFuel          int
	StatusMessage     string
	Status            Status
	PinTxNumber       int
	PinTxHash         string
	TxIndex           int
	CumulativeGasUsed int
}

func (c ConfirmedTx) ToUnified() *UnifiedTx {
	return &UnifiedTx{confirmed: &c}
}

type UnifiedTx struct {
	confirmed   *ConfirmedTx
	unconfirmed *IdentifiableTx
}

func (tx UnifiedTx) GetRawTx() Transaction {
	if tx.confirmed != nil {
		return tx.confirmed.Transaction
	}
	if tx.unconfirmed != nil {
		return tx.unconfirmed.Transaction
	}
	panic(errors.New("transaction isn't set"))
}

func (tx UnifiedTx) GetUnconfirmed() *IdentifiableTx {
	if tx.unconfirmed == nil {
		panic(errors.New("no unconfirmed tx present"))
	}
	return tx.unconfirmed
}

func (tx UnifiedTx) GetConfirmed() *ConfirmedTx {
	if tx.confirmed == nil {
		panic(errors.New("no confirmed tx present"))
	}
	return tx.confirmed
}

func (tx UnifiedTx) IsConfirmed() bool {
	return tx.confirmed != nil
}

func NewUnifiedConfirmedTx(transaction ConfirmedTx) *UnifiedTx {
	return &UnifiedTx{confirmed: &transaction}
}

func NewUnifiedConfirmedTxRaw(transaction Transaction, pinTxNumber int, pinTxHash string, txIndex int, usedGas int) *UnifiedTx {
	return &UnifiedTx{confirmed: &ConfirmedTx{IdentifiableTx: IdentifiableTx{transaction}, StatusMessage: "Executed",
		Status: Successful, PinTxNumber: pinTxNumber, UsedFuel: usedGas, PinTxHash: pinTxHash, CumulativeGasUsed: 0, TxIndex: txIndex}}
}

func NewUnifiedUnconfirmedTxRaw(transaction Transaction) *UnifiedTx {
	return &UnifiedTx{unconfirmed: &IdentifiableTx{transaction}}
}

func NewUnifiedUnconfirmedTx(transaction IdentifiableTx) *UnifiedTx {
	return &UnifiedTx{unconfirmed: &transaction}
}

func NewPaymentConfirmedTx(tx Transaction, pinTxNumber int, pinTxHash string, txIndex int) (ConfirmedTx, error) {
	if tx.GetTransactionType() != PAYMENT {
		return ConfirmedTx{}, errors.New("only payment tx supported for mapping into confirmed")
	}
	return ConfirmedTx{IdentifiableTx: IdentifiableTx{tx}, StatusMessage: "Executed", Status: Successful, PinTxNumber: pinTxNumber,
		UsedFuel: 0, PinTxHash: pinTxHash, TxIndex: txIndex, CumulativeGasUsed: 0}, nil
}

type IdentifiableTx struct {
	Transaction
}

func (i IdentifiableTx) ToUnified() *UnifiedTx {
	return &UnifiedTx{unconfirmed: &i}
}

func (tx *IdentifiableTx) Id() string {

	return tx.GetHash().String()
}

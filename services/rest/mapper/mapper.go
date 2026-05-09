package mapper

import (
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/VG-Grape/luna/dag"
	"github.com/VG-Grape/luna/services/rest/api"
	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/vm"
	"github.com/VG-Grape/luna/crypto"
)

func LogsToDto(logs []vm.Log) []api.Log {
	res := []api.Log{}
	for _, l := range logs {
		res = append(res, LogToDto(l))
	}
	return res
}

func LogToDto(l vm.Log) api.Log {
	mapped := api.Log{}
	mapped.Address = pointerOnString("0x" + hex.EncodeToString(l.ContractAddress))
	mapped.PinTxNumber = pointerOnString(strconv.FormatInt(int64(l.PinTxNumber), 10))
	topics := []string{}
	for _, t := range l.Topics {
		mt := "0x" + hex.EncodeToString(t)
		topics = append(topics, mt)
	}
	mapped.Topics = &topics
	mapped.Data = pointerOnString("0x" + hex.EncodeToString(l.Data))
	mapped.TransactionHash = pointerOnString("0x" + hex.EncodeToString(l.TransactionHash))
	return mapped
}

func AccountToDto(account *vm.LnAccount) api.Account {
	balanceString := account.Balance.Text(10)
	// balance for non-SC accounts is more recent in wallet_cache instead of
	//
	if !vm.IsSCAccount(account.Id) {
		byteAddress := luna1crypto.AddressToBytes(account.Id)
		balance, err := dag.GetPin().GetBalance(byteAddress)
		if err != nil {
			balanceString = "0"
		} else {
			balanceString = balance.Text(10)
		}
	}
	var accountId string
	if strings.HasPrefix(account.Id, "0x") {
		accountId = account.Id
	} else {
		accountId = "0x" + account.Id
	}
	nonceInt := account.Nonce.Int64()
	return api.Account{
		Balance:   &balanceString,
		Created:   &account.Created,
		Id:        &accountId,
		Nonce:     &nonceInt,
		PublicKey: &account.PublicKey}
}

func TxToDto(transaction *tx.UnifiedTx) api.UnifiedTransaction {
	txToMap := transaction.GetRawTx()
	amount := txToMap.GetAmount().String()
	var chainType string
	switch txToMap.GetChainType() {
	case tx.MAINNET:
		chainType = "MAINNET"
	case tx.PUBLIC_TESTNET:
		chainType = "TESTNET"
	case tx.PRIVATE_TESTNET:
		chainType = "TESTNET0"
	default:
		panic(errors.New("unable to determine network"))
	}

	chainId := api.UnifiedTransactionChainId(chainType)
	dataString := hex.EncodeToString(txToMap.GetData())
	fuelLimit := txToMap.GetFuelLimit().String()
	fee := "0"
	usedFuel := "-1"
	if transaction.IsConfirmed() {
		fee = big.NewInt(0).Mul(big.NewInt(int64(transaction.GetConfirmed().UsedFuel)), txToMap.GetFuelPrice()).String()
		usedFuel = strconv.FormatInt(int64(transaction.GetConfirmed().UsedFuel), 10)
	} else {
		fee = big.NewInt(0).Mul(txToMap.GetFuelLimit(), txToMap.GetFuelPrice()).String()
	}
	fuelPrice := txToMap.GetFuelPrice().String()
	nonce := int(txToMap.GetNonce())
	recepient := ""
	if len(txToMap.GetRecipient()) > 0 {
		recepient = luna1crypto.BytesToAddress(txToMap.GetRecipient())
	}
	sender := txToMap.GetSender().String()
	signature := txToMap.GetSignature().String()
	timestamp := time.UnixMilli(int64(txToMap.GetTimestamp())).Format(time.RFC3339)
	txHash := txToMap.GetHash()
	hash := "0x" + hex.EncodeToString(txHash)
	version := 1
	tx_type := int(txToMap.GetTransactionType())
	status := api.SUCCESSFULLYEXECUTED
	if transaction.IsConfirmed() {
		if transaction.GetConfirmed().Status != tx.Successful {
			status = api.FAILED
		}
	} else {
		status = api.NOTEXECUTED
	}
	statusMessage := ""
	if transaction.IsConfirmed() {
		statusMessage = transaction.GetConfirmed().StatusMessage
	} else {
		statusMessage = "Not yet executed & confirmed"
	}
	pinTxNumber := -1
	if transaction.IsConfirmed() {
		pinTxNumber = transaction.GetConfirmed().PinTxNumber
	}
	confirmedTx := api.UnifiedTransaction{
		Amount:        &amount,
		ChainId:       &chainId,
		Data:          &dataString,
		Fee:           &fee,
		FuelLimit:     &fuelLimit,
		FuelPrice:     &fuelPrice,
		FuelUsed:      &usedFuel,
		Nonce:         &nonce,
		Recipient:     &recepient,
		Sender:        &sender,
		Signature:     &signature,
		Status:        &status,
		StatusMessage: &statusMessage,
		Timestamp:     &timestamp,
		TxHash:        &hash,
		Type:          &tx_type,
		Version:       &version,
		PinTxNumber:   &pinTxNumber,
	}
	return confirmedTx
}

func pointerOnString(s string) *string {
	return &s
}

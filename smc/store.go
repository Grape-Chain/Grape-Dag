package smc

import (
	"encoding/hex"
	"math/big"

	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/vm"
	"github.com/VG-Grape/luna/crypto"
	golog "github.com/ipfs/go-log/v2"
)

var logger golog.EventLogger = golog.Logger("smc-pool")

var unconfirmedPool Pool[*tx.IdentifiableTx]
var confirmedPool Pool[*tx.ConfirmedTx]

func init() {
	unconfirmedPool = Pool[*tx.IdentifiableTx]{}
	unconfirmedPool.objects = make(map[string]*tx.IdentifiableTx)

	confirmedPool = Pool[*tx.ConfirmedTx]{}
	confirmedPool.objects = make(map[string]*tx.ConfirmedTx)
}

func AddConfirmed(tx tx.ConfirmedTx) {
	id := tx.Id()
	unconfirmedPool.Remove(id)
	confirmedPool.Put(&tx)
}

func RemoveUnconfirmed(transaction tx.Transaction) {
	idTx := tx.IdentifiableTx{Transaction: transaction}
	unconfirmedPool.Remove(idTx.Id())
}

func AddUnconfirmed(transaction tx.Transaction) {
	unconfirmed := tx.IdentifiableTx{Transaction: transaction}
	unconfirmedPool.Put(&unconfirmed)
	confirmedPool.Remove(unconfirmed.Id())
}

func FindConfirmed(hash string) (*tx.ConfirmedTx, bool) {
	return confirmedPool.Get(hash)
}

func FindAny(hash string) (*tx.UnifiedTx, bool) {
	confirmedTx, found := confirmedPool.Get(hash)
	if found {
		return confirmedTx.ToUnified(), true
	}
	unconfirmedTx, found := unconfirmedPool.Get(hash)
	if !found {
		return nil, false
	}
	return unconfirmedTx.ToUnified(), found
}

func GetAllUncofirmed(fuelLimit int) []tx.Transaction {
	selectedSenders := map[string]bool{}
	fuelLimitLeft := fuelLimit
	selectedTxs := unconfirmedPool.Find(func(transaction *tx.IdentifiableTx) bool {
		accountAddress := luna1crypto.BytesToAddress(transaction.GetSender())
		account := vm.SearchAccount(luna1crypto.BytesToAddress(transaction.GetSender()))
		if account == nil {
			logger.Warnf("Account %s doesn't exist, skip it while selecting unconfirmed smc txs", accountAddress)
			return false
		}
		txFuelLimit := int(big.NewInt(0).SetBytes(transaction.GetFuelLimit().Bytes()).Int64())
		if account.Nonce.Uint64() == transaction.GetNonce() &&
			!selectedSenders[accountAddress] &&
			fuelLimitLeft >= txFuelLimit {
			logger.Infof("Selected unconfirmed tx from=%s,fuelLimit=%d,nonce=%d", accountAddress, txFuelLimit, transaction.GetNonce())
			selectedSenders[accountAddress] = true
			fuelLimitLeft -= txFuelLimit
			return true
		} else {
			hash := transaction.GetHash()
			logger.Infof("Skipped unconfirmed tx, hash=0x%s, from=%s,fuelLimit=%d,nonce=%d, accountNonce=%d, alreadySelectedTxForSenderAccount=%t, leftFuelLimit=%d",
				hex.EncodeToString(hash), accountAddress, txFuelLimit, transaction.GetNonce(), account.Nonce.Uint64(), selectedSenders[accountAddress], fuelLimitLeft)
			return false
		}
	})
	resultTxs := []tx.Transaction{}
	for _, idTx := range selectedTxs {
		resultTxs = append(resultTxs, idTx)
	}
	return resultTxs
}

func FilterConfirmed(f Filter[*tx.ConfirmedTx]) []*tx.ConfirmedTx {
	return confirmedPool.Find(f)
}

func FilterUnconfirmed(f Filter[*tx.IdentifiableTx]) []*tx.IdentifiableTx {
	return unconfirmedPool.Find(f)
}

func FilterAny(fConfirmed Filter[*tx.ConfirmedTx], fUnconfirmed Filter[*tx.IdentifiableTx]) []*tx.UnifiedTx {
	confirmed := confirmedPool.Find(fConfirmed)
	unconfirmed := unconfirmedPool.Find(fUnconfirmed)
	result := []*tx.UnifiedTx{}
	for _, c := range confirmed {
		result = append(result, c.ToUnified())
	}
	for _, u := range unconfirmed {
		result = append(result, u.ToUnified())
	}
	return result
}

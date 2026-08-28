package dag

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"

	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/smc"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/types"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
)

var accountFilter = func(transaction tx.Transaction, accounts []string, txType int, directionIsSent *bool) bool {
	if txType != -1 && txType != int(transaction.GetTransactionType()) {
		return false
	}
	if len(accounts) == 0 {
		return true
	}
	for _, account := range accounts {
		accountId := grape1crypto.AddressToBytes(account)
		senderId := transaction.GetSender()
		if (directionIsSent == nil || *directionIsSent) && bytes.Equal(senderId, accountId) {
			return true
		} else if (directionIsSent == nil || !*directionIsSent) && bytes.Equal(transaction.GetRecipient(), accountId) {
			return true
		}
	}

	return false
}

func SearchTx(hash types.Hex) (*tx.ConfirmedTx, error) {
	for _, pin := range _pins_.snapshotPins() { // select confirmed Payments
		pinHash, _ := pin.Hash(crypto.SHA256)
		pinHashStr := "0x" + hex.EncodeToString(pinHash)
		for nodeIdx, node := range pin.Nodes { // confirmed payment
			realTx := tx.UnmarshalBinary(node.Tx)
			txHash := realTx.GetHash()
			if bytes.Equal(txHash, hash) {
				paymentTx, err := tx.NewPaymentConfirmedTx(realTx, int(pin.PinNumber), pinHashStr, nodeIdx)
				return &paymentTx, err
			}
		}
	}
	confirmedSmcTx, found := smc.FindConfirmed(hash.String())
	if !found {
		return nil, fmt.Errorf("tx not found in dag")
	}
	return confirmedSmcTx, nil
}

func SearchAnyTx(hash types.Hex) (*tx.UnifiedTx, error) {
	confirmedTx, err := SearchTx(hash)
	if err == nil {
		return tx.NewUnifiedConfirmedTx(*confirmedTx), nil
	}

	for _, node := range _dag_.SnapshotNodes() { // select unconfirmed Payments
		realTx := node.tx
		txHash := realTx.GetHash()
		if bytes.Equal(txHash, hash) {
			paymentTx := tx.NewUnifiedUnconfirmedTx(tx.IdentifiableTx{Transaction: realTx})
			return paymentTx, err
		}
	}
	smcTx, found := smc.FindAny(hash.String())
	if !found {
		return nil, fmt.Errorf("tx not found in dag/smcpool/blockxhain")
	}
	return smcTx, nil
}

func SearchTxs(accounts []string, txType int, limit int, offset int, ascSort bool, confirmed *bool, directionIsSent *bool) []*tx.UnifiedTx {
	var accTxs []*tx.UnifiedTx
	processedPayments := map[string]bool{}
	// Snapshot the shared collections under their own locks, sequentially, then
	// iterate the copies. Nesting the two locks here would invert the order used
	// by the insert path (dag lock first) and could deadlock.
	pinsSnapshot := _pins_.snapshotPins()
	for _, pin := range pinsSnapshot { // select confirmed SMC txs and Payments
		pinTxNumber := int(pin.PinNumber)
		pinHashStr := pin.GetHash().String()
		if confirmed == nil || *confirmed {
			gasUsed := 0
			for smcTxIndex, smcTx := range pin.SmcTxs { // SMC transactions
				gasUsed += int(smcTx.GetReceipt().FuelUsed)
				realTx := tx.UnmarshalBinary(smcTx.Tx)
				status := tx.Successful
				if smcTx.Receipt.Status == pb.TxReceipt_FAILED {
					status = tx.Failed
				}
				hash := realTx.GetHash()
				if accountFilter(realTx, accounts, txType, directionIsSent) {
					accTxs = append(accTxs,
						tx.ConfirmedTx{IdentifiableTx: tx.IdentifiableTx{Transaction: realTx},
							UsedFuel:          int(smcTx.Receipt.FuelUsed),
							StatusMessage:     smcTx.Receipt.StatusMessage,
							Status:            status,
							PinTxNumber:       pinTxNumber,
							PinTxHash:         pinHashStr,
							TxIndex:           len(pin.Nodes) + smcTxIndex,
							CumulativeGasUsed: gasUsed,
						}.
							ToUnified())
					logger.Debugf("Found confirmed SC tx=%s,pinTx=%s, from=0x%s,to=0x%s,amount=%s,status=%s,data=0x%s",
						hash, pinTxNumber, hex.EncodeToString(realTx.GetSender()), hex.EncodeToString(realTx.GetRecipient()),
						big.NewInt(0).SetBytes(realTx.GetAmount().Bytes()).String(), status, hex.EncodeToString(realTx.GetData()))
				}
			}
		}
		for nodeIdx, node := range pin.Nodes { // confirmed payment
			realTx := tx.UnmarshalBinary(node.Tx)
			if accountFilter(realTx, accounts, txType, directionIsSent) {
				id := &uuid.UUID{}
				id.UnmarshalBinary(node.Id.Id)
				processedPayments[id.String()] = true // get confirmed txs data even when confirmed txs aren't requested to distinguish them from unconfirmed later
				if confirmed == nil || *confirmed {
					accTxs = append(accTxs, tx.NewUnifiedConfirmedTxRaw(realTx, pinTxNumber, pinHashStr, nodeIdx, 0))
					hash := realTx.GetHash()
					logger.Debugf("Found confirmed payment nodeId=%s,tx=0x%s, from=0x%s,to=0x%s,amount=%s",
						id.String(), hex.EncodeToString(hash), hex.EncodeToString(realTx.GetSender()), hex.EncodeToString(realTx.GetRecipient()),
						big.NewInt(0).SetBytes(realTx.GetAmount().Bytes()).String())
				}
			}
		}
	}

	if confirmed == nil || !*confirmed {
		unconfirmedSmc := smc.FilterUnconfirmed( // unconfirmed SMC txs
			func(transaction *tx.IdentifiableTx) bool {
				return accountFilter(transaction.Transaction, accounts, txType, directionIsSent)
			})
		for _, uSc := range unconfirmedSmc {
			accTxs = append(accTxs, uSc.ToUnified())
			hash := uSc.GetHash()
			logger.Debugf("Found unconfirmed SC tx=0x%s,from=0x%s,to=0x%s,amount=%s,data=0x%s",
				hex.EncodeToString(hash), hex.EncodeToString(uSc.GetSender()), hex.EncodeToString(uSc.GetRecipient()),
				big.NewInt(0).SetBytes(uSc.GetAmount().Bytes()).String(), hex.EncodeToString(uSc.GetData()))
		}

		for _, n := range _dag_.SnapshotNodes() { // unconfirmed payments
			if processedPayments[n.Id()] { // skip confirmed txs collected before
				continue
			}
			if accountFilter(n.tx, accounts, txType, directionIsSent) {
				accTxs = append(accTxs, tx.NewUnifiedUnconfirmedTxRaw(n.tx))
				hash := n.tx.GetHash()
				logger.Debugf("Found unconfirmed payment nodeId=%s,tx=0x%s, from=0x%s,to=0x%s,amount=%s",
					n.Id(), hex.EncodeToString(hash), hex.EncodeToString(n.tx.GetSender()), hex.EncodeToString(n.tx.GetRecipient()),
					big.NewInt(0).SetBytes(n.tx.GetAmount().Bytes()).String())
			}
		}
	}
	accTxs = deduplicate(accTxs)
	sort.SliceStable(accTxs, func(i int, j int) bool {
		var first, second tx.Transaction
		if !ascSort {
			first = accTxs[j].GetRawTx()
			second = accTxs[i].GetRawTx()
		} else {
			first = accTxs[i].GetRawTx()
			second = accTxs[j].GetRawTx()
		}

		if first.GetTimestamp() <= second.GetTimestamp() {
			return true
		} else {
			return false
		}
	})
	if len(accTxs) <= offset {
		return []*tx.UnifiedTx{}
	}
	accTxs = accTxs[offset:]
	if len(accTxs) < limit {
		return accTxs
	} else {
		return accTxs[:limit]
	}
}

func deduplicate(txs []*tx.UnifiedTx) []*tx.UnifiedTx {
	uniqueTxs := map[string]*tx.UnifiedTx{}
	result := []*tx.UnifiedTx{}
	for _, tx := range txs {
		hash := tx.GetRawTx().GetHash()
		stringHash := hex.EncodeToString(hash)
		_, exists := uniqueTxs[stringHash]
		if exists {
			logger.Warnf("Duplicate tx 0x%s found when retrieving txs list. Will use latest tx", stringHash)
		}
		uniqueTxs[stringHash] = tx
	}
	for _, tx := range uniqueTxs {
		result = append(result, tx)
	}
	logger.Debugf("Before deduplication:%d, after deduplication: %d", len(txs), len(result))
	return result
}

func (n *Node) isVisited(visited []*Node) bool {
	if _, _, e := goterators.Find(visited, func(node *Node) bool {
		return n.id.id == node.id.id
	}); e != nil {
		return false
	}
	return true
}

func (n *Node) isTarget(targets []*Node) bool {
	if _, _, e := goterators.Find(targets, func(node *Node) bool {
		return n.Equal(node)
	}); e != nil {
		return false
	}
	return true
}

func (n *Node) visit(targets []*Node, visited []*Node) ([]*Node, []*Node) {
	if n == nil {
		return targets, visited
	}
	if n.isVisited(visited) {
		return targets, visited
	}
	visited = append(visited, n)
	// let's return ourselves since we are working with 2 references
	// but have only one - we are a tip
	if len(n.sources) <= 1 {
		targets = append(targets, n)
	}
	// going towards the tips
	for _, v := range n.sources {
		targets, visited = v.visit(targets, visited)
	}

	return targets, visited
}

// dfSearch - depth first search: find tips for the current site
// returns:
//
//	[]*Node a list of sites that are tips for the given node
func (n *Node) dfSearch() []*Node {
	targets := []*Node{}
	visited := []*Node{}
	if n != nil {
		targets, _ = n.visit(targets, visited)
	}

	return targets
}

func (n *Node) traverseX(res []*SiteTips) []*SiteTips {
	if n == nil {
		return nil
	}
	nTips := n.dfSearch()
	// this site may be its own tip, make sure we do not count it
	if len(nTips) > 0 && nTips[0].id.id != n.id.id {
		// make sure the entry is not present in the slice
		// if present replace with a new value only if the current value
		// is greater than the value already obtained
		t, i, e := goterators.Find(res, func(s *SiteTips) bool {
			return bytes.Compare(s.id, n.id.id[:]) == 0
		})
		if e != nil {
			// not found - insert
			res = append(res, &SiteTips{
				id:   n.id.id[:],
				tips: len(nTips),
				cw:   n.cumWeight.Load(),
				n:    n,
			})
		} else {
			if t.tips < len(nTips) {
				res = append(res[:i], res[i+1:]...)
				res = append(res, &SiteTips{
					id:   n.id.id[:],
					tips: len(nTips),
					cw:   n.cumWeight.Load(),
					n:    n,
				})
			}
		} // else leave as is
	}
	// if the current site has sites that reference it, get their tips as well
	goterators.ForEach(n.sources, func(x *Node) {
		res = x.traverseX(res)
	})
	return res
}

func (n *Node) traverse(siteTips []*SiteTips, visited []*Node, threshold int) ([]*SiteTips, []*Node) {
	if n == nil {
		return siteTips, visited
	}
	if n.isVisited(visited) {
		return siteTips, visited
	}
	visited = append(visited, n)
	nTips := n.dfSearch()
	if len(nTips) < threshold {
		return siteTips, visited
	}
	// this site may be its own tip, make sure we do not count it
	if len(nTips) > 0 && nTips[0].id.id != n.id.id {
		// make sure the entry is not present in the slice
		// if present replace with a new value only if the current value
		// is greater than the value already obtained
		t, i, e := goterators.Find(siteTips, func(s *SiteTips) bool {
			return bytes.Compare(s.id, n.id.id[:]) == 0
		})
		if e != nil {
			// not found - insert
			siteTips = append(siteTips, &SiteTips{
				id:   n.id.id[:],
				tips: len(nTips),
				cw:   n.cumWeight.Load(),
				n:    n,
			})
		} else {
			if t.tips < len(nTips) {
				siteTips = append(siteTips[:i], siteTips[i+1:]...)
				siteTips = append(siteTips, &SiteTips{
					id:   n.id.id[:],
					tips: len(nTips),
					cw:   n.cumWeight.Load(),
					n:    n,
				})
			}
		} // else leave as is
	}
	// if the current site has sites that reference it, get their tips as well
	goterators.ForEach(n.sources, func(x *Node) {
		siteTips, visited = x.traverse(siteTips, visited, threshold)
	})
	return siteTips, visited
}

func (n *Node) traverseEx(siteTips []*SiteTips, threshold int) []*SiteTips {
	// traverse until threshold is reached
	// assume direction from genesis to tips
	visited := []*Node{}
	siteTips, _ = n.traverse(siteTips, visited, threshold)
	return siteTips
}

type SiteTips struct {
	id   []byte
	cw   float64
	tips int
	n    *Node
}

// bfTraverse - return a slice of tips from the given site
//
// returns:
//
//	[]*Node - a slice of tips for the given site
func (n *Node) bfTraverse(theshold int) []*SiteTips {
	// check for tip condition
	if n == nil || len(n.sources) == 0 {
		return nil
	}
	res := []*SiteTips{}

	return n.traverseEx(res, theshold)
}

package dag

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/smc"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/Grape-Chain/Grape-Dag/vm"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/crypto/eth"
	"github.com/enescakir/emoji"
	"github.com/golang-collections/collections/set"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
)

var storageNode NodeTxPin

type NodeTxPin struct {
	NodeType int
	pins     []*pb.TxPin
	mu       sync.Mutex
	last     string
	ready    bool
	// view - an immutable copy of the chain published on every mutation, so
	// readers (REST, eth-RPC, balance lookups) never take p.mu. Taking the
	// mutex on the read path would serialise every query behind pin
	// application, which re-executes smart contracts against the VM over gRPC
	// and can therefore block for a long time - or indefinitely, if the VM is
	// unreachable.
	view           atomic.Pointer[[]*pb.TxPin]
	downloadedPins chan *pb.TxPin
}

// chain - the current published view of the pin chain. Lock-free; the returned
// slice is never mutated in place, only replaced.
func (p *NodeTxPin) chain() []*pb.TxPin {
	if v := p.view.Load(); v != nil {
		return *v
	}
	return nil
}

// unsafe_publish - republish the reader view after mutating p.pins.
// Caller must hold p.mu.
func (p *NodeTxPin) unsafe_publish() {
	cp := make([]*pb.TxPin, len(p.pins))
	copy(cp, p.pins)
	p.view.Store(&cp)
}

// unsafe_appendPin - add a pin to the chain and republish the reader view.
// Every mutation of p.pins must go through here (or call unsafe_publish
// itself), otherwise readers keep seeing a stale chain. Caller must hold p.mu.
func (p *NodeTxPin) unsafe_appendPin(pin *pb.TxPin) {
	p.pins = append(p.pins, pin)
	p.unsafe_publish()
}

func (p *NodeTxPin) GetPin(number int) *pb.TxPin {
	pins := p.chain()
	// Prefer the pin whose number matches. Indexing by position only works
	// while the chain starts at zero: a node that joined via a balance snapshot
	// holds pins numbered from wherever the leader was.
	if number >= 0 && number < len(pins) && pins[number].PinNumber == int64(number) {
		return pins[number]
	}
	for _, pin := range pins {
		if pin.PinNumber == int64(number) {
			return pin
		}
	}
	return nil
}

func (p *NodeTxPin) GetLastPin() *pb.TxPin {
	pins := p.chain()
	if len(pins) > 0 {
		return pins[len(pins)-1]
	}
	return nil
}

// unsafe_getLastPin - caller must hold p.mu
func (p *NodeTxPin) unsafe_getLastPin() *pb.TxPin {
	if len(p.pins) > 0 {
		return p.pins[len(p.pins)-1]
	}
	return nil
}

// unsafe_nextPinNumber - the number the next pin appended to the chain must
// carry: one past the current head, or 0 for an empty chain (the genesis pin).
// Caller must hold p.mu.
func (p *NodeTxPin) unsafe_nextPinNumber() int64 {
	if last := p.unsafe_getLastPin(); last != nil {
		return last.PinNumber + 1
	}
	return 0
}

func (p *NodeTxPin) unlock() {
	// clear the diagnostic before releasing, or concurrent lock/unlock pairs
	// race on the field itself
	p.last = ""
	p.mu.Unlock()
}

func (p *NodeTxPin) lock(caller string) {
	p.mu.Lock()
	p.last = caller
}

func (p *NodeTxPin) LockPin() {
	p.mu.Lock()
}

func (p *NodeTxPin) UnlockPin() {
	p.mu.Unlock()
}

func (p *NodeTxPin) IsReady() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ready
}

func (p *NodeTxPin) InPin(vertex *Node) bool {
	p.lock("InPin")
	defer p.unlock()
	for l := len(p.pins) - 1; l >= 0; l-- {
		_, _, err := goterators.Find(p.pins[l].Sites, func(site *pb.SiteID) bool {
			return bytes.Equal(site.Id, vertex.id.id[:])
		})
		if err == nil {
			return true
		}
	}
	return false
}

func newNodeTxPin() *NodeTxPin {
	return &NodeTxPin{
		pins:           []*pb.TxPin{},
		mu:             sync.Mutex{},
		last:           string(""),
		ready:          false,
		downloadedPins: make(chan *pb.TxPin, 1000),
	}
}

func (p *NodeTxPin) set(genesis *Node, wallet string) {
	p.lock("set")
	defer p.unlock()
	pin := pb.NewTxPin([]byte{})
	s := &pb.SiteID{
		Id:      genesis.id.id[:],
		Address: genesis.id.address,
		IdMajor: genesis.id.idMajor,
		IdMinor: genesis.id.idMinor,
	}
	pin.Balance.Balance[wallet] = genesis.tx.GetAmount().Bytes()
	pin.Sites = append(pin.Sites, s)
	// The genesis pin is number 0. This must be set before signing: the
	// signature covers the marshalled pin, PinNumber included.
	pin.PinNumber = p.unsafe_nextPinNumber()
	pin.SignTx(_dag_.Wallet())

	p.unsafe_appendPin(pin)
	// The genesis site reaches a pin without going through the confirmed pool,
	// so record it as harvested here: otherwise the first two sites to approve
	// genesis promote it and it lands in a second pin.
	if confirmationCounter != nil {
		confirmationCounter.markHarvested(genesis.id.id)
	}
}

// snapshotPins - a shallow copy of the pin chain, taken under the pin lock.
// Readers iterate the copy so that an append (which may reallocate the backing
// array) cannot race them.
func (p *NodeTxPin) snapshotPins() []*pb.TxPin {
	return p.chain()
}

func (p *NodeTxPin) getLast() *pb.TxPin {
	p.lock("getLast")
	defer p.unlock()
	if len(p.pins) > 0 {
		return p.pins[len(p.pins)-1]
	}
	return nil
}

func (p *NodeTxPin) getAllFrom(fromPinNumber int) []*pb.TxPin {
	p.lock("getAllFrom")
	defer p.unlock()
	if len(p.pins) > 0 && len(p.pins) > fromPinNumber && fromPinNumber >= 0 {
		return p.pins[fromPinNumber:]
	}
	return nil
}

func (p *NodeTxPin) openPinDownloading() {
	p.downloadedPins = make(chan *pb.TxPin, 1000)
}

func (p *NodeTxPin) GetBalance(wallet []byte) (*big.Int, error) {
	balances, err := p.GetBalances([][]byte{wallet})
	if err != nil {
		return big.NewInt(0), err
	}
	return big.NewInt(0).SetBytes(balances[0]), nil
}

func (p *NodeTxPin) AddDownloadedPin(pin *pb.TxPin) {
	p.downloadedPins <- pin
}

func (p *NodeTxPin) ClosePinDownloading() {
	close(p.downloadedPins)
}

// Method <GetLatestBalance> - for the given wallet address return the latest known balance
// args:
//
//	wallet_address - wallet address for each to retrieve the latest balance
//
// returns:
//
//	*big.Int - if wallet address is found, return is last known balance
//	error - if an error occurs, such as there is no information on the wallet
func (p *NodeTxPin) unsafe_getLatestBalance(wallet_address string) (*big.Int, error) {
	// Reads the published view rather than p.pins, so callers that hold only
	// their own lock (the wallet cache, on the payment hot path) do not race
	// the appends made under p.mu.
	pins := p.chain()
	// lookups take place in reverce order to that we always find the latest first
	for ri := len(pins) - 1; ri >= 0; ri-- {
		if v, ok := pins[ri].Balance.Balance[wallet_address]; ok {
			return big.NewInt(0).SetBytes(v), nil
		}
	}
	return nil, fmt.Errorf("unknown wallet %s, cannot obtain balance", wallet_address)
}

func (p *NodeTxPin) insertIfNotFound(ptx *pb.TxPin) {
	p.lock("inserIfNotFound")
	defer p.unlock()
	// this function splices p.pins in several branches; republish the reader
	// view once, whichever branch ran
	defer p.unsafe_publish()
	logger.Infof("%s  ~ %s Inserting pin tx \nS:%s P:%s",
		emoji.RoundPushpin,
		emoji.ClockwiseVerticalArrows,
		hex.EncodeToString(ptx.Sign)[:10],
		func() string {
			if len(ptx.Prev) > 0 {
				return hex.EncodeToString(ptx.Prev)[:10]
			} else {
				return "NIL"
			}
		}(),
	)
	if _, _, err := goterators.Find(p.pins, func(p *pb.TxPin) bool {
		return bytes.Equal(p.Sign, ptx.Sign)
	}); err != nil {
		if len(ptx.Prev) == 0 {
			// the new transaction is a genesis tx, insert it if it does not exist
			if len(p.pins) > 0 {
				if len(p.pins[0].Prev) == 0 {
					// is this a duplicate of the genesis tx?
					if bytes.Equal(p.pins[0].Sign, ptx.Sign) {
						// this is a duplicate, can ignore
						logger.Infof("%s  ~ genesis pin tx already exists", emoji.RoundPushpin)
					} else {
						logger.Warnf("%s  ~ %s different versions of genesis tx", emoji.RoundPushpin, emoji.Warning)
						// replace with the new genesis
						logger.Infof("%s  ~ insert pin tx as genesis S:%s", emoji.RoundPushpin, hex.EncodeToString(ptx.Sign)[:10])
						p.pins[0] = ptx
					}
				} else {
					// let's see where we need to insert this pin tx
					for i, x := range p.pins {
						if bytes.Equal(x.Prev, ptx.Sign) {
							// we found the place
							p.pins = append(p.pins[:i+1], p.pins[i:]...)
							logger.Infof("%s  ~ insert pin tx at %d S:%s", emoji.RoundPushpin, i, hex.EncodeToString(ptx.Sign)[:10])
							p.pins[i] = ptx
							break
						}
					}
				}
			} else {
				// it's empty - add as first
				logger.Infof("%s  ~ insert pin tx as last S:%s", emoji.RoundPushpin, hex.EncodeToString(ptx.Sign)[:10])
				p.pins = append(p.pins, ptx)
			}
		} else {
			// new pin tx needs to be inserted in the right place
			// let's find a pin tx that references this tx
			inserted := false
			for i, x := range p.pins {
				if bytes.Equal(x.Prev, ptx.Sign) {
					// we found the place
					p.pins = append(p.pins[:i+1], p.pins[i:]...)
					logger.Infof("%s  ~ insert pin tx at %d S:%s", emoji.RoundPushpin, i, hex.EncodeToString(ptx.Sign)[:10])
					p.pins[i] = ptx
					inserted = true
					break
				}
			}
			if !inserted {
				//add at the end
				logger.Infof("%s  ~ insert pin tx as last S:%s", emoji.RoundPushpin, hex.EncodeToString(ptx.Sign)[:10])
				p.pins = append(p.pins, ptx)
			}
		}

		// // insert only when we can
		// if ptx.Prev == nil {
		// 	// this is the genesis pin
		// 	logger.Infof("%s  ~ insert genesis pin %s", emoji.RoundPushpin, emoji.MoneyBag)
		// 	l := len(p.pins)
		// 	if l > 1 {
		// 		p.pins = append(p.pins[:1], p.pins[:]...)
		// 		p.pins[0] = ptx
		// 	} else if l == 0 {
		// 		p.pins = append(p.pins, ptx)
		// 	}
		// } else {
		// 	found := false
		// 	for idx, v := range p.pins {
		// 		if bytes.Equal(v.Sign, ptx.Prev) {
		// 			logger.Infof("%s  ~ insert in order %d pin %s", emoji.RoundPushpin, idx, emoji.Coin)
		// 			p.pins = append(p.pins[:idx+1], p.pins[idx:]...)
		// 			p.pins[idx] = ptx
		// 			found = true
		// 			break
		// 		}
		// 	}
		// 	if !found {
		// 		// append as last - the pin sync will request missing...
		// 		logger.Infof("%s  ~ insert as last pin %s", emoji.RoundPushpin, emoji.Coin)
		// 		p.pins = append(p.pins, ptx)
		// 	}
		// }
	} else {
		logger.Infof("%s  ~  pin tx S:%s already exits. Ignore", emoji.RoundPushpin, hex.EncodeToString(ptx.Sign)[:10])
	}
}

func (p *NodeTxPin) unsafe_getBalanceForWallet(wallet string) (*big.Int, error) {
	sb, err := walletCache.get(wallet)
	if err != nil && sb == nil {
		sb, err = _pins_.unsafe_getLatestBalance(wallet)
		if err != nil {
			// we did not have luck finding this wallet in pin txs
			err = fmt.Errorf("[pin tx] Balance for wallet %s cannot be found. Out of sync or DS", wallet)
			sb = big.NewInt(0)
		}
	}
	return sb, err
}

func (p *NodeTxPin) getById(id uuid.UUID) *Node {
	p.lock("findById")
	defer p.unlock()
	idb, _ := id.MarshalBinary()

	for l := len(p.pins) - 1; l >= 0; l-- {
		n, _, err := goterators.Find(p.pins[l].Nodes, func(node *pb.Node) bool {
			return bytes.Equal(node.Id.Id, idb)
		})
		if err == nil {
			node := &Node{}
			node.FromPbNode(n)
			return node
		}
	}
	return nil
}

// Method <add>  -  adds a new pinning tx with a list of sites that are confirmed and limited list of executed smart contracts
// args:
//
//		a slice of confirmed sites (Nodes) in dag
//
//	    a list of smart contract transactions to execute and add to pin tx
//
// returns:
//
//	error if a balancing error occurs

func (p *NodeTxPin) add(sites []*Node, smcTxs []tx.Transaction) error {
	p.lock("add")
	defer p.unlock()
	var prev []byte = []byte{}

	if len(p.pins) > 0 {
		prev = p.pins[len(p.pins)-1].Sign
	}

	// migrate the wallet cache balance to pin tx - reconcile balances,
	// detect conflicts, update balances for confirmed tx
	// Note: pin tx do not reflect the latest balances,
	// only the balances for the confirmed txs.
	// Wallet cache may still have new balance information for the unconfirmed tx
	// Keep that in mind when updating balances here
	// The most important aspect: detect balance conflicts

	// sort sites based on time in an ascending order - from past to future
	sort.SliceStable(sites, func(i, j int) bool {
		return sites[i].time.Before(sites[j].time)
	})

	// get the latest balances from cache first,
	// and when missing, from prev pin tx
	senders := map[string][]*big.Int{}
	receivers := map[string][]*big.Int{}
	for _, vertex := range sites {
		// Get all senders' balances first
		// Look in cache first
		wallet := grape1crypto.BytesToAddress(vertex.tx.GetSender())
		senderBalance, err := p.unsafe_getBalanceForWallet(wallet)
		if err != nil {
			logger.Error(err.Error())
			// this is a fatal error - perhaps it's related to double spending, fraud, etc.
			logger.Warn("[@DEVNOTE] This is not normal. Decide what to do with this.")
			senderBalance = big.NewInt(0)
		}
		senders[wallet] = append(senders[wallet], senderBalance)
		// get all receivers' balances
		// Look in cache first
		wallet = grape1crypto.BytesToAddress(vertex.tx.GetRecipient())
		receiverBalance, err := p.unsafe_getBalanceForWallet(wallet)
		if err != nil {
			logger.Error(err.Error())
			// this is a fatal error - perhaps it's related to double spending, fraud, etc.
			logger.Warn("[@DEVNOTE] Receiver balance not found. Decide what to do with this.")
			receiverBalance = big.NewInt(0)
		}
		receivers[wallet] = append(receivers[wallet], receiverBalance)
	}

	pin := pb.NewTxPin(prev)
	// each sites indicates a new transaction - process all transactions
	for _, val := range sites {
		// store sites in pin [for now] - need for synch
		pin.Nodes = append(pin.Nodes, val.ToPbNode())
		s := &pb.SiteID{
			Id:      val.id.id[:],
			Address: val.id.address,
			IdMajor: val.id.idMajor,
			IdMinor: val.id.idMinor,
		}
		// for each site update the balance, when valid
		// 1. Find the latest balance update for the sender
		senderBalanceTx := senders[grape1crypto.BytesToAddress(val.tx.GetSender())]
		senderBalance := senderBalanceTx[len(senderBalanceTx)-1]
		if senderBalance.Cmp(big.NewInt(0)) < 0 {
			// this transaction cannot be processed
			val.valid = false
			logger.Warnf("[@DEVNOTE] Need to revert balances in cache")
			logger.Errorf("Sender wallet %s balance is %d", grape1crypto.BytesToAddress(val.tx.GetSender()), senderBalance.Uint64())
		}
		// 2. Find the latest balance update for the recipient
		// receiver may not be known to us at all at this point
		receiverBalanceTx := receivers[grape1crypto.BytesToAddress(val.tx.GetRecipient())]
		receiverBalance := receiverBalanceTx[len(receiverBalanceTx)-1]
		pin.Sites = append(pin.Sites, s)
		// allow tx balance update
		pin.Balance.Balance[grape1crypto.BytesToAddress(val.tx.GetSender())] = senderBalance.Bytes()
		pin.Balance.Balance[grape1crypto.BytesToAddress(val.tx.GetRecipient())] = receiverBalance.Bytes()
		// remove from wallet cache
		walletCache.remove(grape1crypto.BytesToAddress(val.tx.GetSender()), []string{val.Id()})
		walletCache.remove(grape1crypto.BytesToAddress(val.tx.GetRecipient()), []string{val.Id()})
	}
	// Derive the number from the chain head rather than a process-local counter,
	// so it stays correct across restarts and after syncing pins from a peer.
	// Set before signing and before the smart-contract stage, which reports it
	// to the VM as the block number.
	pin.PinNumber = p.unsafe_nextPinNumber()
	p.runSmartContractStage(pin, smcTxs)

	// now that all the information has been collected, sign it and store
	pin.SignTx(dagWallet)

	// append the latest pinning transaction to the slice of all pinning tx
	p.unsafe_appendPin(pin)
	// b, _ := pin.MarshalJSON()
	// logger.Infof("%s", string(b))
	return nil
}

func (p *NodeTxPin) runSmartContractStage(pin *pb.TxPin, smcTxs []tx.Transaction) {
	// Smart contracts execution stage
	// First step is balances synchronization - put updated balances from this pin tx into VM state store
	// Second step - execute smart contract transactions and get receipts (execution results) with logs produced during execution
	// Third step - collect changes applied to the state
	// Fourth step - synchronize balances affected during sc execution back to pin.Balance.Balace
	// Fifth step - remove temporary cached balances within wallet_cache (affected during sc execution)
	// to force it to update balance on next lookup
	logger.Infof("Smart contract stage started, pin=%d, txs=%d", pin.PinNumber, len(smcTxs))
	startTime := time.Now()
	vm.SyncBalances(pin.Balance.Balance)   // sync balances to storage
	vm.CaptureStateStoreDiffs()            // enable state store changes capture
	defer vm.ResetCaptureStateStoreDiffs() // disable state store changes capture even when this method fails for some reason
	executedTxs := []*pb.ExecutedSmcTx{}
	pinHash, _ := pin.Hash(crypto.SHA256)
	pinHashStr := "0x" + hex.EncodeToString(pinHash)
	timestamp := pin.Ts.AsTime().Unix()
	gasUsed := 0
	for _, node := range pin.GetNodes() {
		if node.Tx.GetRlpEthTx() != nil { // increase nonce for non-payment eth txs
			vm.NonceIncr(node.Tx.Sender)
		}
	}
	for smcTxIdx, smcTx := range smcTxs {

		execResult, err := p.executeSMCTx(smcTx, int64(timestamp), pin.PinNumber)
		if err != nil {
			hash := smcTx.GetHash()
			logger.Warnf("Transaction %s is invalid (during execution), removing from smc pool", "0x"+hex.EncodeToString(hash))
			smc.RemoveUnconfirmed(smcTx)
			continue
		}
		status := tx.Successful
		if !execResult.Successful {
			status = tx.Failed
		}
		gasUsed += int(execResult.GasUsed)
		smc.AddConfirmed(tx.ConfirmedTx{IdentifiableTx: tx.IdentifiableTx{Transaction: smcTx},
			Status: status, StatusMessage: execResult.Output, PinTxNumber: int(pin.PinNumber), UsedFuel: int(execResult.GasUsed),
			PinTxHash: pinHashStr, TxIndex: len(pin.Nodes) + smcTxIdx, CumulativeGasUsed: gasUsed})
		receipt := execResultToReceipt(execResult)
		execSmc := pb.ExecutedSmcTx{Tx: smcTx.MarshalBinary(), Receipt: receipt}
		executedTxs = append(executedTxs, &execSmc)
	}
	pin.SmcTxs = executedTxs
	diffSlice := vm.GetStateStoreDiffs()
	pin.Diffs = diffToPb(diffSlice)

	for _, diff := range diffSlice {
		if diff.HasAccount() {
			address := "0x" + diff.Account.Address
			balance := new(big.Int)
			balance, _ = balance.SetString(diff.Account.Balance, 10)
			pin.Balance.Balance[address] = balance.Bytes()
			walletCache.remove(address, []string{}) // remove from cache to force balance update on next lookup
			logger.Infof("Account %s balance was synchronized with dag, now balance -  %v", address, balance)
		}
	}
	logger.Infof("Smart contract stage finished in %d mcs", time.Since(startTime).Microseconds())
}

func (pin *NodeTxPin) runSmartContractStageFullNode(balances map[string][]byte, recentPin *pb.TxPin) error {
	logger.Infof("Smart contract stage started at full node, pin=%d, txs=%d", recentPin.PinNumber, len(recentPin.SmcTxs))
	vm.SyncBalances(balances)
	vm.CaptureStateStoreDiffs()
	defer vm.ResetCaptureStateStoreDiffs()
	executedTxs := []*pb.ExecutedSmcTx{}
	timestamp := recentPin.Ts.AsTime().Unix()
	execTx := recentPin.SmcTxs
	for _, smcTx := range execTx {
		realTx := tx.UnmarshalBinary(smcTx.Tx)
		realJson := realTx.String()
		logger.Infof("Transaction info:  %s", realJson)
		execResult, err := pin.executeSMCTx(realTx, int64(timestamp), recentPin.PinNumber)
		if err != nil {
			hash := realTx.GetHash()
			logger.Warnf("Synchronization failed! Transaction %s is invalid (during execution)", "0x"+hex.EncodeToString(hash))
			return errors.New("Fullnode sync failed")
		}
		status := tx.Successful
		statusMsg := "SUCCESSFUL"
		if !execResult.Successful {
			status = tx.Failed
			statusMsg = "FAILED"
		}

		receipt := execResultToReceipt(execResult)
		execSmc := pb.ExecutedSmcTx{Tx: realTx.MarshalBinary(), Receipt: receipt}
		executedTxs = append(executedTxs, &execSmc)

		logger.Debugf("Number of pin tx - %d, Smc execution result is %s, %s, fuel used - %d ", len(pin.pins), status, receipt, int(execResult.GasUsed))

		if int32(execResult.GasUsed) != smcTx.Receipt.FuelUsed {
			logger.Warnf("Fullnode is out of sync! Fuel used expected %d, used - %d", int32(execResult.GasUsed), smcTx.Receipt.FuelUsed)
			return errors.New("Fullnode sync failed")

		}
		if tx.Status(statusMsg) != tx.Status(smcTx.Receipt.Status.String()) {
			logger.Warnf("Fullnode is out of sync! Expected status - %s, real status - %s ", tx.Status(statusMsg), tx.Status(smcTx.Receipt.Status.String()))
			return errors.New("Fullnode sync failed")
		}
		if execResult.Output != smcTx.Receipt.StatusMessage {
			logger.Warnf("Fullnode is out of sync! Execution results are not the same! Expected - %s, real - %s", execResult.Output, smcTx.Receipt.StatusMessage)
			return errors.New("Fullnode sync failed")
		}
		logs := execResultToReceipt(execResult).Logs
		for i, log := range logs {
			if !bytes.Equal(log.GetData(), smcTx.Receipt.Logs[i].GetData()) {
				return errors.New("fullnode sync failed! Sync failed by logs")
			}
		}
	}
	diffs := vm.GetStateStoreDiffs()
	for _, diff := range diffs {
		if diff.HasAccount() {
			address := "0x" + diff.Account.Address
			balance := new(big.Int)
			balance, _ = balance.SetString(diff.Account.Balance, 10)
			walletCacheConfirmed.cache[address] = append(walletCacheConfirmed.cache[address], newPair[string, *big.Int]("sc", balance))
			cacheBalance, _ := walletCacheConfirmed.get(address)
			pinBalance := big.NewInt(0).SetBytes(recentPin.Balance.Balance[address])
			logger.Infof("Account %s balance updated according to VM execution results, now balance -  %s, in confirmed cache - %s, in received pin %s", address, balance.String(), cacheBalance.String(), pinBalance)

		}

	}

	logger.Info("Sync by diffs is successfull")
	return nil
}

func (p *NodeTxPin) CurrentHeight() int {
	pins := p.chain()
	if len(pins) == 0 {
		return 0
	}
	return int(pins[len(pins)-1].PinNumber)
}

// unsafe_currentHeight - the pin number at the head of the chain.
// Caller must hold p.mu.
func (p *NodeTxPin) unsafe_currentHeight() int {
	if len(p.pins) == 0 {
		return 0
	}
	return int(p.pins[len(p.pins)-1].PinNumber)
}

func (p *NodeTxPin) CurrentTS() int64 {
	pins := p.chain()
	if len(pins) == 0 {
		return 0
	}
	return pins[len(pins)-1].Ts.AsTime().Unix()
}

func (p *NodeTxPin) UpdateIfValid(wallet string, amount *big.Int) bool {
	p.lock("UpdateIfValid")
	defer p.unlock()
	valid := false
	// start looking for wallets in reverse
	known_balance := big.NewInt(0)
	for l := len(p.pins) - 1; l >= 0; l-- {
		txmap := p.pins[l].Balance.Balance
		if b, ok := txmap[wallet]; ok {
			balance := big.NewInt(0).SetBytes(b)
			balance = balance.Sub(balance, amount)
			if balance.Cmp(big.NewInt(0)) >= 0 {
				p.pins[l].Balance.Balance[wallet] = balance.Bytes()
				known_balance = balance
				valid = true
				break
			}
		}
	}
	logger.Infof("[$] Update balance for %s. valid %t, amount %d", wallet, valid, known_balance.Int64())
	return valid
}

// GetWallets - return all known wallets
func (p *NodeTxPin) GetWallets() ([][]byte, error) {
	p.lock("GetWallets")
	defer p.unlock()
	resp := [][]byte{}
	wallet_set := set.New()
	// genesis wallet should be the first, if not only wallet known at start up time
	for _, pin := range p.pins {
		for wallet, _ := range pin.Balance.Balance {
			wallet_set.Insert(wallet)
		}
	}
	// add what we have in cache
	walletCache.lock()
	for wallet, _ := range walletCache.cache {
		wallet_set.Insert(wallet)
	}
	walletCache.unlock()

	wallet_set.Do(func(i interface{}) {
		wallet_address := i.(string)
		resp = append(resp, grape1crypto.AddressToBytes(wallet_address))
	})

	return resp, nil
}

func (p *NodeTxPin) GetPinnedBalances(pinNumber int64) (map[string][]byte, error) {
	p.lock("GetPinnedBalances")
	defer p.unlock()
	resp := map[string][]byte{}

	// genesis wallet should be the first, if not only wallet known at start up time
	for _, pin := range p.pins {
		if pin.PinNumber > pinNumber {
			break
		}
		for wallet, balance := range pin.Balance.Balance {
			resp[wallet] = balance
		}
	}
	return resp, nil
}

func (p *NodeTxPin) GetBalances(wallets [][]byte) ([][]byte, error) {
	p.lock("GetBalances")
	defer p.unlock()
	resp := [][]byte{}
	// go through all the wallets we are interested in
	for _, wallet := range wallets {
		// go through the pins in a descending order until we either find
		// or not the desired balance
		success := false
		// var bs []*Pair[string, *big.Int]
		// var ok bool
		walletCache.lock()
		bs, ok := walletCache.cache[grape1crypto.BytesToAddress(wallet)]
		walletCache.unlock()
		if !ok || len(bs) == 0 {
			for l := len(p.pins) - 1; l >= 0; l-- {
				wallet_str := grape1crypto.BytesToAddress(wallet)
				if balance, ok := p.pins[l].Balance.Balance[wallet_str]; ok {
					logger.Debugf("[GetBalance] *** Wallet %s found in pin %d", wallet_str, l)
					resp = append(resp, balance)
					success = true
					break
				}
			}
		} else {
			if len(bs) == 0 {
				// wallet is present, but the balance is unknown.
				logger.Warnf("[@DEVNOTE] Wallet %s is in cache but balance unknown", wallet)
				resp = append(resp, big.NewInt(0).Bytes())
			} else {
				resp = append(resp, bs[len(bs)-1].second.Bytes())
			}
			success = true
		}
		if !success {
			return nil, fmt.Errorf("Balance for wallet %s not found", grape1crypto.BytesToAddress(wallet))
		}
	}
	return resp, nil
}

func diffToPb(diffs []vm.Diff) []*pb.Diff {
	result := []*pb.Diff{}
	for _, diff := range diffs {
		if diff.HasAccount() {
			accountData := pb.AccountData{}
			accountData.Address = diff.Account.AddressBytes()
			balance, _ := big.NewInt(0).SetString(diff.Account.Balance, 10)
			nonce, _ := big.NewInt(0).SetString(diff.Account.Nonce, 10)
			accountData.Balance = balance.Bytes()
			accountData.Nonce = nonce.Int64()
			accountData.Codehash, _ = hex.DecodeString(diff.Account.CodeHash)
			accountData.Code, _ = hex.DecodeString(diff.Account.Code)
			result = append(result, &pb.Diff{MappingOrAccount: &pb.Diff_AccountDiff{AccountDiff: &pb.AccountDiff{NewValue: &accountData}}})
		} else {
			mappingDiff := pb.Mapping{}
			mappingDiff.Address = diff.MappingDiffValue.Address
			mappingDiff.Key = diff.MappingDiffValue.Key
			mappingDiff.Value = diff.MappingDiffValue.Value
			result = append(result, &pb.Diff{MappingOrAccount: &pb.Diff_MappingDiff{MappingDiff: &mappingDiff}})
		}
	}
	return result
}

func execResultToReceipt(r ExecutionResult) *pb.TxReceipt {
	hash, err := hex.DecodeString(r.Hash)
	if err != nil {
		panic(err)
	}
	logs := vm.GetLogsForTx(hash)
	execLogs := []*pb.TxExecLog{}
	for _, v := range logs {
		topics := []*pb.LogTopic{}
		for _, t := range v.Topics {
			topics = append(topics, &pb.LogTopic{Hash: t})
		}
		execLogs = append(execLogs, &pb.TxExecLog{ContractAddress: v.ContractAddress, Topics: topics})
	}
	status := pb.TxReceipt_SUCCESSFUL
	if !r.Successful {
		status = pb.TxReceipt_FAILED
	}
	receipt := pb.TxReceipt{FuelUsed: int32(r.GasUsed), Status: status, StatusMessage: r.Output, Logs: execLogs}
	return &receipt
}

func parseVmError(err string) error {
	if strings.HasPrefix(err, "0x") {
		return newSolidityError(err)
	}
	return fmt.Errorf("system VM error during tx execution: %s", err)
}

type solidityError struct {
	signature string
	offset    uint64
	dataLen   uint64
	data      string
	raw       string
}

func (err solidityError) Error() string {
	return err.data
}

func newSolidityError(hexErr string) solidityError {
	if !strings.HasPrefix(hexErr, "0x") {
		panic(fmt.Errorf("solidity error must be in hex format starting with 0x, got %s", hexErr))
	}
	trimmedErr := strings.TrimPrefix(hexErr, "0x")
	bytes, err := hex.DecodeString(trimmedErr)
	if err != nil {
		panic(fmt.Errorf("solidity error must be in hex format got %s", hexErr))
	}
	if len(bytes) < 4 {
		panic(fmt.Errorf("hex error has size %d, but solidity error has minimum size 4 bytes", len(bytes)))
	}
	sErr := solidityError{}
	if len(bytes) < 100 {
		//no parameters
		sErr.signature = trimmedErr[:8]
		sErr.offset = 0
		sErr.dataLen = 0
		//panic(fmt.Errorf("hex error has size %d, but solidity error has minimum size %d (signature 4 bytes, offset 32 bytes, length min 32 bytes, data min 32 bytes)", len(bytes), 100))
	} else {
		offset, offsetParseErr := strconv.ParseUint(trimmedErr[8:72], 16, 64)
		if offsetParseErr != nil {
			panic(fmt.Errorf("unable to convert value %s to uint64 for solidity error offset, hexErr=%s", trimmedErr[8:72], hexErr))
		}
		length, lengthParseErr := strconv.ParseUint(trimmedErr[72:72+offset*2], 16, 64)
		if lengthParseErr != nil {
			panic(fmt.Errorf("unable to convert value %s to uint64 for solidity error offset, hexErr=%s", trimmedErr[72:72+offset*2], hexErr))
		}

		errData, dataParseErr := hex.DecodeString(trimmedErr[72+offset*2 : 72+offset*2+length*2])
		if dataParseErr != nil {
			// mustn't reach this line
			panic(fmt.Errorf("program logic error! Unexpected error happened, %s is not a hex for solidity error", hexErr))
		}
		errDataString := string(errData)

		sErr.signature = trimmedErr[:8]
		sErr.offset = offset
		sErr.dataLen = length
		sErr.data = errDataString
		sErr.raw = hexErr
	}
	return sErr
}

func emptyHex(hex string) bool {
	return hex == "" || hex == "0x" || len(hex) <= 2
}

type ExecutionResult struct {
	Hash       string
	Successful bool
	Output     string
	GasUsed    int64
}

// executeSMCTx - run one smart-contract tx on the VM in the context of the pin
// identified by pinNumber (reported to the VM as the block number).
func (p *NodeTxPin) executeSMCTx(transaction tx.Transaction, timestamp int64, pinNumber int64) (ExecutionResult, error) {
	hash := transaction.GetHash()
	execResult := ExecutionResult{}
	client, err := vm.ConnectToVm()
	if err != nil {
		logger.Errorf("Unable to connect to VM: %s", err.Error())
		return execResult, err
	}
	coinbaseAddrBytes, err := eth.ParseEthAddress(config.GetConfig().Dag.Coinbaseaccount)
	if err != nil {
		return execResult, fmt.Errorf("invalid coinbase account %q in configuration: %w",
			config.GetConfig().Dag.Coinbaseaccount, err)
	}
	txPin := &pb.PinTxHeader{CoinbaseAccountAddress: &pb.Address{AddBytes: coinbaseAddrBytes},
		Timestamp: timestamp, TxNumber: int32(pinNumber)}
	response, vmErr := client.RunCall(context.TODO(), &pb.WriteContractRequest{Tx: transaction.MarshalBinary(), Header: txPin})
	if vmErr != nil {
		logger.Errorf("VM error occurred during tx=%v execution: %s", transaction, vmErr.Error())
		return execResult, vmErr
	}
	if response.Status == -2 { // system error occurred
		logger.Debug("Smc execution has gone wrong, vm response status is -2")
		return execResult, parseVmError(response.Error) // bad tx, abandon
	} else {
		gasUsed, err := strconv.ParseInt(response.GasUsed, 10, 64)
		if err != nil {
			return execResult, err
		}
		if response.Status > 0 || response.Status == -1 { // solidity error or general VM error occurred (out of gas, bad instruction, etc), keep tx
			parsedErr := ""
			if emptyHex(response.Msg) {
				parsedErr = parseVmError(response.Error).Error()
			} else {
				parsedErr = parseVmError(response.Msg).Error()
			}
			execResult.Successful = false
			execResult.Output = parsedErr
			execResult.GasUsed = gasUsed
		} else {
			execResult.Output = response.Msg
			execResult.Successful = true
			execResult.GasUsed = gasUsed
		}
		status := tx.Successful
		if !execResult.Successful {
			status = tx.Failed
		}

		format := "Transaction %s executed, status = %s, output=%s, gas=%d"
		var colorizedFormat string
		if execResult.Successful {
			colorizedFormat = string(utils.Green) + format + string(utils.Reset)
		} else {
			colorizedFormat = string(utils.Red) + format + string(utils.Reset)
		}
		logger.Infof(colorizedFormat, hex.EncodeToString(hash), status, execResult.Output, execResult.GasUsed)
		execResult.Hash = hex.EncodeToString(hash)
		return execResult, nil
	}
}

func (pin *NodeTxPin) SyncPins(recentPin *pb.TxPin) {
	logger.Debug("Executing slices at fullnode started")
	balances := make(map[string][]byte)
	sites := recentPin.Nodes
	//here we adding balances to fill walletcacheConfirmed

	for _, payment := range sites {
		senderBalance, err := walletCacheConfirmed.get(grape1crypto.BytesToAddress(payment.Tx.Sender))
		if err != nil {
			senderBalance, err = pin.unsafe_getLatestBalance(grape1crypto.BytesToAddress(payment.Tx.Sender))
			if err != nil {
				logger.Errorf("Sender's wallet %s does not exist", grape1crypto.BytesToAddress(payment.Tx.Sender))

			}
		}
		transferAmount := new(big.Int)
		transferAmount.SetBytes(payment.Tx.Amount)
		walletCacheConfirmed.sub(grape1crypto.BytesToAddress(payment.Tx.Sender), payment.Tx.String(), transferAmount)

		walletCacheConfirmed.add(grape1crypto.BytesToAddress(payment.Tx.Recepient), payment.Tx.String(), transferAmount)
		receiverBalance, _ := walletCacheConfirmed.get(grape1crypto.BytesToAddress(payment.Tx.Recepient))
		// update sender's balance after sub operation
		senderBalance, _ = walletCacheConfirmed.get(grape1crypto.BytesToAddress(payment.Tx.Sender))
		balances[grape1crypto.BytesToAddress(payment.Tx.Sender)] = senderBalance.Bytes()
		balances[grape1crypto.BytesToAddress(payment.Tx.Recepient)] = receiverBalance.Bytes()
	}
	if config.GetConfig().Peer.NodeType == 0 {
		logger.Info("Dump new balances to vm state store")
		pin.runSmartContractStageFullNode(balances, recentPin)
	} else if config.GetConfig().Peer.NodeType == 1 {
		//here balances from diffs are dumping to vm.state store
		//We take balances from latest pin because peernode doesn't proceed smc
		vm.SyncBalances(balances)
		diffsPinBalances := make(map[string][]byte)
		diffsFromPin := recentPin.Diffs

		for _, diff := range diffsFromPin {
			if diff.GetAccountDiff() != nil {
				address := "0x" + hex.EncodeToString(diff.GetAccountDiff().NewValue.Address)
				acDiffBalance := new(big.Int).SetBytes(diff.GetAccountDiff().NewValue.Balance)
				walletCacheConfirmed.cache[address] = append(walletCacheConfirmed.cache[address], newPair[string, *big.Int]("sc", acDiffBalance))
				diffsPinBalances[address] = acDiffBalance.Bytes()
				cacheBalance, _ := walletCacheConfirmed.get(address)
				pinBalance := big.NewInt(0).SetBytes(recentPin.Balance.Balance[address])
				if acDiffBalance.Cmp(cacheBalance) != 0 || acDiffBalance.Cmp(pinBalance) != 0 {
					logger.Warnf("Sync at peernode is failed by address - %s,  now balance -  %s, in confirmed cache - %s, in received pin %s", address, acDiffBalance.String(), cacheBalance.String(), pinBalance)
				}
			}
			if diff.GetMappingDiff() != nil {
				vm.DumpMappingDiffs(diff)

			}
		}
		if len(diffsPinBalances) > 0 {
			vm.SyncBalances(diffsPinBalances)
		}

		pin.checkBalances(recentPin)

	}
}

func (pin *NodeTxPin) checkBalances(recentPin *pb.TxPin) error {
	for address, balanceBytes := range recentPin.Balance.Balance {
		cachedBalance, _ := walletCacheConfirmed.get(address)
		pinBalance := big.NewInt(0).SetBytes(balanceBytes)
		vmAcc := vm.SearchAccount(address)
		if vmAcc != nil {
			vmBalance := vmAcc.Balance
			if cachedBalance.Cmp(pinBalance) != 0 || pinBalance.Cmp(&vmBalance) != 0 {
				logger.Warnf("[Applied Pin Post-Check] Balances are different for address %s! In received pin - %d, in comfirmed cache - %d, stored in vm - %s", address, pinBalance.String(), cachedBalance.String(), &vmBalance)
				return errors.New("node sync failed")
			}
		} else {
			if cachedBalance.Cmp(pinBalance) != 0 {
				logger.Warnf("[Applied Pin Post-Check] Balances are different for address %s! In received pin - %d, in comfirmed cache - %d", address, pinBalance.String(), cachedBalance.String())
				return errors.New("node sync failed")

			}
		}
	}
	return nil
}

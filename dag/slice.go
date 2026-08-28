package dag

import (
	"fmt"
	"math/big"
	"sync"
)

/*
This file contains logic that handles wallet balances in the current dag slice.
When a new pin tx is formed, the balances associated with the tx included in
the pin tx are removed, but the balances that are associated with the unfonfirmed
tx are left in cache.
*/

// Pair stores a pair of tx id and the latest balance
type Pair[T1, T2 any] struct {
	first  T1
	second T2
}

func newPair[T1, T2 any](f1 T1, s2 T2) *Pair[T1, T2] {
	return &Pair[T1, T2]{
		first:  f1,
		second: s2,
	}
}

// Define cache struct
type WalletCache struct {
	mu sync.Mutex
	// contains wallet as key, and a slice of pairs: tx id, updated balance
	cache map[string][]*Pair[string, *big.Int]
}

func newWalletCache() *WalletCache {
	return &WalletCache{
		mu:    sync.Mutex{},
		cache: make(map[string][]*Pair[string, *big.Int]),
	}
}

func (wc *WalletCache) lock() {
	wc.mu.Lock()
}

func (wc *WalletCache) unlock() {
	wc.mu.Unlock()
}

// add an amount to the latest balance for the given wallet
func (wc *WalletCache) add(wallet string, txId string, amount *big.Int) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	// @Note: tx to dag should only be added in order, hence
	// we assume that all tx are in order and add linearly
	// without consequences
	var loadedBalance *big.Int
	var err error
	if balances, ok := wc.cache[wallet]; !ok || len(balances) == 0 {
		//  not found in cache
		wc.cache[wallet] = []*Pair[string, *big.Int]{}
		// check if pins have the last known balance for this wallet
		loadedBalance, err = _pins_.unsafe_getLatestBalance(wallet)
		if err != nil {
			// we did not have luck finding this wallet in pin txs
			loadedBalance = big.NewInt(0)
		}
	} else {
		walletLen := len(wc.cache[wallet])
		if walletLen == 0 {
			// this is a new tx balance, set it to 0
			loadedBalance = big.NewInt(0)
		} else {
			loadedBalance = wc.cache[wallet][len(wc.cache[wallet])-1].second
		}
	}
	// don't run sub/add operations on real reference to latest balance
	// since the operation may fail and we do nothing to revert the balance
	// to previous state
	tmpBalance := big.NewInt(0)
	if traceBalances {
		logger.Infof("[wallet cache] Add %s to %s, current balance %s", amount.String(), wallet, loadedBalance.String())
	}
	tmpBalance.Add(loadedBalance, amount)
	if traceBalances {
		logger.Infof("[wallet cache] Result %s balance %s", wallet, tmpBalance.String())
	}
	if tmpBalance.Sign() == -1 {
		return fmt.Errorf("Wallet %s: tx %s negative balance %s", wallet, txId, tmpBalance.String())
	}
	// append to cache
	wc.cache[wallet] = append(wc.cache[wallet], newPair(txId, tmpBalance))
	if traceBalances {
		logger.Infof("[wallet cache] Final balance for %s is %s", wallet, wc.cache[wallet][len(wc.cache[wallet])-1].second.String())
	}
	return nil
}

// subtract an amount from the latest balance for the given wallet
func (wc *WalletCache) sub(wallet string, txId string, amount *big.Int) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	// @Note: tx to dag should only be added in order, hence
	// we assume that all tx are in order and add linearly
	// without consequences
	var loadedBalance *big.Int
	var err error
	if balances, ok := wc.cache[wallet]; !ok || len(balances) == 0 {
		//  not found in cache
		wc.cache[wallet] = []*Pair[string, *big.Int]{}
		// check if pins have the last known balance for this wallet
		loadedBalance, err = _pins_.unsafe_getLatestBalance(wallet)
		if err != nil {
			// we did not have luck finding this wallet in pin txs
			loadedBalance = big.NewInt(0)
		}
	} else {
		loadedBalance = wc.cache[wallet][len(wc.cache[wallet])-1].second
	}
	// don't run sub/add operations on real reference to latest balance
	// since the operation may fail and we do nothing to revert the balance
	// to previous state
	tmpBalance := big.NewInt(0)
	tmpBalance = tmpBalance.Sub(loadedBalance, amount)
	if tmpBalance.Sign() == -1 {
		return fmt.Errorf("Wallet %s: tx %s negative balance %s", wallet, txId, tmpBalance.String())
	}
	wc.cache[wallet] = append(wc.cache[wallet], newPair[string, *big.Int](txId, tmpBalance))
	return nil
}

// get the latest balance for the given wallet
func (wc *WalletCache) get(wallet string) (*big.Int, error) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	if _, ok := wc.cache[wallet]; ok {
		if len(wc.cache[wallet]) > 0 {
			return big.NewInt(0).Set(wc.cache[wallet][len(wc.cache[wallet])-1].second), nil
		}
	}
	return nil, fmt.Errorf("Wallet %s not in cache", wallet)
}

// when a pin tx is formed, we need to remove the balances from cache
// that correspond to the confirmed txs
func (wc *WalletCache) remove(wallet string, txIds []string) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	if _, ok := wc.cache[wallet]; !ok {
		return nil
	} else {
		delete(wc.cache, wallet)
		// b := wc.cache[wallet]
		// newB := goterators.Filter(b, func(p *Pair[string, *big.Int]) bool {
		// 	_, _, err := goterators.Find(txIds, func(id string) bool {
		// 		return id == p.first
		// 	})
		// 	return err != nil
		// })
		// if newB != nil {
		// 	wc.cache[wallet] = newB
		// } else {
		// 	wc.cache[wallet] = []*Pair[string, *big.Int]{}
		// }
	}
	return nil
}

// setBalance - replace what the cache holds for a wallet, without arithmetic.
// Used when rebuilding from the commit-transaction chain, where each pin states
// the balance outright rather than a delta.
func (wc *WalletCache) setBalance(wallet string, balance *big.Int) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	wc.cache[wallet] = []*Pair[string, *big.Int]{newPair("recovered", big.NewInt(0).Set(balance))}
}

// snapshot - a copy of what this cache holds, taken under its own lock.
func (wc *WalletCache) snapshot() map[string][]*Pair[string, *big.Int] {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	out := make(map[string][]*Pair[string, *big.Int], len(wc.cache))
	for address, balances := range wc.cache {
		out[address] = copyBalances(balances)
	}
	return out
}

// copyFrom - replace what this cache holds with what another one holds.
//
// The source is read through snapshot(), under the source's own lock. Reading
// it directly took only the destination's lock, which is not the lock that
// guards the map being iterated: a copy taken while any other goroutine wrote
// to the source crashed the process with "concurrent map iteration and map
// write". That was survivable while the only copy happened under the pin lock;
// a validator snapshotting the live cache before building a commit transaction
// made it reachable from the ordinary path.
//
// The two locks are never held at once, so there is no order to get wrong.
func (wc *WalletCache) copyFrom(another *WalletCache) error {
	if another == nil {
		return nil
	}
	copied := another.snapshot()
	wc.mu.Lock()
	defer wc.mu.Unlock()
	wc.cache = copied
	return nil
}

func copyBalances(balances []*Pair[string, *big.Int]) []*Pair[string, *big.Int] {
	copy := []*Pair[string, *big.Int]{}
	copy = append(copy, balances...)
	return copy
}

package dag

import (
	"math/big"
	"sync"

	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
)

/*
The settled balances: what every account holds according to the commit
transactions alone, with nothing unconfirmed folded in.

Why this exists rather than reading the balance map a commit transaction
carries. That map is written from the live cache at the moment the commit
transaction is formed, so it also reflects transactions that are still
unconfirmed - sites that will land in some later commit transaction, or in none
at all. It is therefore not a statement of the ledger at that point, and
rebuilding from it lands a restarted node on balances the transactions do not
support. cmd/ledgercheck reports exactly that discrepancy on a stored chain.

What the transactions do support is unambiguous, so that is what is tracked
here: the initial offering as the first commit transaction states it, then every
settled payment applied in order, then contract execution's account diffs, which
state balances outright rather than as transfers. This is the same arithmetic
receiving nodes already perform when they apply a commit transaction, so a
recovered node agrees with a node that never restarted.

It is also maintained identically by whichever node formed the commit
transaction and by every node that applied it - the leader used to leave the
confirmed cache empty, because only the receive path filled it.
*/
type settledLedger struct {
	mu       sync.RWMutex
	balances map[string]*big.Int
	// upTo - the last commit transaction folded in, so recovery knows where to
	// resume and never applies one twice.
	upTo   int64
	seeded bool
}

func newSettledLedger() *settledLedger {
	return &settledLedger{balances: make(map[string]*big.Int), upTo: -1}
}

// applyPin - fold a commit transaction into the settled balances. Idempotent by
// pin number: applying the same one twice is a no-op.
func (l *settledLedger) applyPin(pin *pb.TxPin) {
	if pin == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if pin.PinNumber <= l.upTo {
		return
	}

	// The first commit transaction of a chain is the only statement of where
	// the money came from - the initial offering for a chain that starts at
	// genesis, or the leader's snapshot for a node that joined later. There are
	// no transactions to derive it from, so it is taken as given.
	if !l.seeded {
		if pin.Balance != nil {
			for wallet, raw := range pin.Balance.Balance {
				l.balances[wallet] = new(big.Int).SetBytes(raw)
			}
		}
		l.seeded = true
		l.upTo = pin.PinNumber
		return
	}

	for _, node := range pin.Nodes {
		if node == nil || node.Tx == nil {
			continue
		}
		realTx := tx.UnmarshalBinary(node.Tx)
		if realTx == nil {
			continue
		}
		if realTx.GetTransactionType() != tx.PAYMENT {
			continue
		}
		// Addresses come off the chain, so they are checked rather than
		// trusted: BytesToAddress panics on anything that is not 20 bytes, and
		// this runs on the commit path and on recovery.
		sender, ok := addressOf(realTx.GetSender())
		if !ok {
			continue
		}
		recipient, ok := addressOf(realTx.GetRecipient())
		if !ok {
			continue
		}
		if sender == recipient {
			continue
		}
		amount := new(big.Int).SetBytes(realTx.GetAmount().Bytes())
		l.creditLocked(sender, new(big.Int).Neg(amount))
		l.creditLocked(recipient, amount)
	}

	// Contract execution reports the resulting account state rather than a
	// transfer, so those balances replace whatever the payments implied.
	for _, diff := range pin.Diffs {
		acct := diff.GetAccountDiff()
		if acct == nil || acct.NewValue == nil {
			continue
		}
		addr, ok := addressOf(acct.NewValue.Address)
		if !ok {
			continue
		}
		l.balances[addr] = new(big.Int).SetBytes(acct.NewValue.Balance)
	}

	l.upTo = pin.PinNumber
}

func (l *settledLedger) creditLocked(wallet string, delta *big.Int) {
	cur, ok := l.balances[wallet]
	if !ok {
		cur = big.NewInt(0)
		l.balances[wallet] = cur
	}
	cur.Add(cur, delta)
}

// seed - install a persisted snapshot as the starting point.
func (l *settledLedger) seed(upTo int64, balances map[string][]byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.balances = make(map[string]*big.Int, len(balances))
	for wallet, raw := range balances {
		l.balances[wallet] = new(big.Int).SetBytes(raw)
	}
	l.upTo = upTo
	l.seeded = true
}

// snapshot - the settled balances in the form the store keeps them.
func (l *settledLedger) snapshot() (int64, map[string][]byte) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[string][]byte, len(l.balances))
	for wallet, v := range l.balances {
		out[wallet] = v.Bytes()
	}
	return l.upTo, out
}

func (l *settledLedger) get(wallet string) (*big.Int, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	v, ok := l.balances[wallet]
	if !ok {
		return nil, false
	}
	return new(big.Int).Set(v), true
}

// total - the sum of every settled balance, for conservation checks.
func (l *settledLedger) total() *big.Int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	sum := big.NewInt(0)
	for _, v := range l.balances {
		sum.Add(sum, v)
	}
	return sum
}

func (l *settledLedger) appliedUpTo() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.upTo
}

// installAsConfirmed - publish the settled balances as the node's confirmed
// cache, and start the working cache from them.
func (l *settledLedger) installAsConfirmed() {
	l.mu.RLock()
	snapshot := make(map[string]*big.Int, len(l.balances))
	for wallet, v := range l.balances {
		snapshot[wallet] = new(big.Int).Set(v)
	}
	l.mu.RUnlock()

	for wallet, v := range snapshot {
		walletCacheConfirmed.setBalance(wallet, v)
	}
	walletCache.copyFrom(walletCacheConfirmed)
}

// addressOf - an account address as a string, reporting whether it was usable.
// Deliberately not grape1crypto.BytesToAddress, which panics on any length
// other than 20; nothing read off the chain should be able to do that.
func addressOf(raw []byte) (string, bool) {
	if len(raw) != 20 {
		return "", false
	}
	return grape1crypto.BytesToAddress(raw), true
}

// ledgercheck reads a node's stored commit-transaction chain and reports
// whether it adds up, without needing the node to be running.
//
// It computes the balances two ways and compares them:
//
//	stated    what each commit transaction says the balances are
//	replayed  what they must be if you start from the initial offering and
//	          apply every transaction the chain settled
//
// The two agreeing is the property that matters: a commit transaction is meant
// to be a faithful statement of the ledger at that point, and anything that
// rebuilds from the chain - a restarting node, an explorer, an auditor - is
// entitled to believe it.
//
//	ledgercheck -path ~/.grap3/data/ledger [-v]
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"math/big"
	"os"
	"sort"

	"github.com/Grape-Chain/Grape-Dag/store"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
)

func main() {
	path := flag.String("path", "", "ledger store directory")
	verbose := flag.Bool("v", false, "report every pin")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "usage: ledgercheck -path <store dir> [-v]")
		os.Exit(2)
	}

	s, err := store.Open(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open the store: %s\n", err.Error())
		os.Exit(1)
	}
	defer s.Close()

	head, err := s.Head()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read the store head: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("chain head: pin %d, %d pins, network %d, schema %d\n",
		head.LastPinNumber, head.PinCount, head.Network, head.SchemaVersion)

	stated := map[string]*big.Int{}   // as the chain states them
	replayed := map[string]*big.Int{} // as the transactions imply
	opening := big.NewInt(0)
	pins, sites, payments := 0, 0, 0
	seeded := false

	err = s.LoadPins(func(pin *pb.TxPin) error {
		pins++
		if pin.Balance != nil {
			for w, raw := range pin.Balance.Balance {
				stated[w] = new(big.Int).SetBytes(raw)
			}
		}

		// The first commit transaction of the chain is the opening statement:
		// the initial offering for a chain that starts at genesis, or the
		// leader's snapshot for a node that joined later. There are no
		// transactions to derive it from, so it is taken as given - the same
		// rule a recovering node follows.
		if !seeded {
			if pin.Balance != nil {
				for w, raw := range pin.Balance.Balance {
					replayed[w] = new(big.Int).SetBytes(raw)
					opening.Add(opening, replayed[w])
				}
			}
			seeded = true
			if *verbose {
				fmt.Printf("  pin %-5d opening statement: %d wallet(s), total %s\n",
					pin.PinNumber, len(pin.Balance.Balance), opening.String())
			}
			return nil
		}

		for _, node := range pin.Nodes {
			sites++
			if node == nil || node.Tx == nil {
				continue
			}
			realTx := tx.UnmarshalBinary(node.Tx)
			if realTx == nil {
				continue
			}
			amount := new(big.Int).SetBytes(realTx.GetAmount().Bytes())
			sender := "0x" + hex.EncodeToString(realTx.GetSender())
			recipient := "0x" + hex.EncodeToString(realTx.GetRecipient())

			if realTx.GetTransactionType() != tx.PAYMENT || sender == recipient {
				continue
			}
			payments++
			credit(replayed, sender, new(big.Int).Neg(amount))
			credit(replayed, recipient, amount)
		}

		// Contract execution states account balances outright rather than as
		// transfers, so take those as given.
		for _, diff := range pin.Diffs {
			if acct := diff.GetAccountDiff(); acct != nil && acct.NewValue != nil {
				addr := "0x" + hex.EncodeToString(acct.NewValue.Address)
				replayed[addr] = new(big.Int).SetBytes(acct.NewValue.Balance)
			}
		}

		if *verbose {
			fmt.Printf("  pin %-5d sites=%-4d balances=%d\n", pin.PinNumber, len(pin.Nodes), len(pin.Balance.Balance))
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read the chain: %s\n", err.Error())
		os.Exit(1)
	}

	fmt.Printf("read %d pins, %d settled sites (%d payments)\n", pins, sites, payments)
	fmt.Printf("wallets: %d stated, %d implied by the transactions\n", len(stated), len(replayed))
	fmt.Printf("opening total:  %s\n", opening.String())
	fmt.Printf("replayed total: %s\n", total(replayed).String())
	fmt.Printf("stated total:   %s\n", total(stated).String())

	// Conservation is the property that has to hold: payments move value
	// between accounts, so replaying them can neither create nor destroy any.
	conserved := total(replayed).Cmp(opening) == 0
	if conserved {
		fmt.Println("value is conserved: replaying every settled payment returns the opening total")
	} else {
		fmt.Printf("VALUE IS NOT CONSERVED: replaying the payments is out by %s\n",
			new(big.Int).Sub(total(replayed), opening).String())
	}

	mismatches := compare(stated, replayed)
	if len(mismatches) == 0 {
		if !conserved {
			os.Exit(1)
		}
		fmt.Println("the chain adds up: every stated balance matches the transactions")
		return
	}
	fmt.Printf("\n%d wallet(s) where the chain states a balance the transactions do not support.\n", len(mismatches))
	fmt.Println("A commit transaction's balance map is written from the live cache, so it can")
	fmt.Println("include transactions that were still unconfirmed at the time. Nodes rebuild")
	fmt.Println("balances by replaying the settled payments instead, which is why this does not")
	fmt.Println("affect them - but anything that reads those maps directly inherits it.")
	for i, m := range mismatches {
		if i == 20 {
			fmt.Printf("  ... and %d more\n", len(mismatches)-20)
			break
		}
		fmt.Printf("  %s stated %s, transactions imply %s (out by %s)\n", m.wallet, m.stated, m.replayed, m.delta)
	}
	os.Exit(1)
}

func credit(m map[string]*big.Int, wallet string, amount *big.Int) {
	if _, ok := m[wallet]; !ok {
		m[wallet] = big.NewInt(0)
	}
	m[wallet].Add(m[wallet], amount)
}

func total(m map[string]*big.Int) *big.Int {
	sum := big.NewInt(0)
	for _, v := range m {
		sum.Add(sum, v)
	}
	return sum
}

type mismatch struct{ wallet, stated, replayed, delta string }

func compare(stated, replayed map[string]*big.Int) []mismatch {
	out := []mismatch{}
	seen := map[string]bool{}
	for w := range stated {
		seen[w] = true
	}
	for w := range replayed {
		seen[w] = true
	}
	wallets := make([]string, 0, len(seen))
	for w := range seen {
		wallets = append(wallets, w)
	}
	sort.Strings(wallets)

	for _, w := range wallets {
		a, b := stated[w], replayed[w]
		if a == nil {
			a = big.NewInt(0)
		}
		if b == nil {
			b = big.NewInt(0)
		}
		if a.Cmp(b) != 0 {
			out = append(out, mismatch{w, a.String(), b.String(), new(big.Int).Sub(a, b).String()})
		}
	}
	return out
}

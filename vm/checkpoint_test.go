package vm

import (
	"math/big"
	"testing"
)

// A validator that proposes a commit transaction executes its smart-contract
// transactions to know what the commit transaction says - before knowing
// whether the round will be agreed. If it loses the round it has to be able to
// get back to where it was, or it carries state no other node has.

func TestRevertUndoesEverythingSinceTheCheckpoint(t *testing.T) {
	AttachInMemoryStateStore()

	const address = "aabbccddeeff00112233445566778899aabbccdd"
	server.storage.putAccount(StoredAccount{Address: address, Balance: "100", Nonce: "1"})

	Checkpoint()
	server.storage.putAccount(StoredAccount{Address: address, Balance: "999", Nonce: "2"})
	if got, _ := server.storage.getAccount(mustHex(t, address)); got.Balance != "999" {
		t.Fatalf("the speculative change did not apply, balance is %s", got.Balance)
	}

	RevertCheckpoint()
	got, ok := server.storage.getAccount(mustHex(t, address))
	if !ok {
		t.Fatal("reverting removed an account that existed before the checkpoint")
	}
	if got.Balance != "100" {
		t.Fatalf("balance after revert is %s, want the pre-checkpoint 100", got.Balance)
	}
}

func TestDropKeepsEverythingSinceTheCheckpoint(t *testing.T) {
	AttachInMemoryStateStore()

	const address = "1122334455667788990011223344556677889900"
	server.storage.putAccount(StoredAccount{Address: address, Balance: "5", Nonce: "1"})

	Checkpoint()
	server.storage.putAccount(StoredAccount{Address: address, Balance: "42", Nonce: "2"})
	DropCheckpoint()

	got, ok := server.storage.getAccount(mustHex(t, address))
	if !ok {
		t.Fatal("dropping the checkpoint lost the account")
	}
	if got.Balance != "42" {
		t.Fatalf("balance after dropping the checkpoint is %s, want 42", got.Balance)
	}
}

// A round is won or lost after the balances a commit transaction carries have
// been synced into the store, so the revert has to cover those too.
func TestRevertUndoesSyncedBalances(t *testing.T) {
	AttachInMemoryStateStore()

	const address = "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	SyncBalances(map[string][]byte{address: big.NewInt(70).Bytes()})
	before, _ := server.storage.getAccount(mustHex(t, address[2:]))

	Checkpoint()
	SyncBalances(map[string][]byte{address: big.NewInt(4000).Bytes()})
	RevertCheckpoint()

	after, ok := server.storage.getAccount(mustHex(t, address[2:]))
	if !ok {
		t.Fatal("reverting removed the account")
	}
	if after.Balance != before.Balance {
		t.Fatalf("balance after revert is %s, want the pre-checkpoint %s", after.Balance, before.Balance)
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var b int
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			switch {
			case c >= '0' && c <= '9':
				b = b<<4 | int(c-'0')
			case c >= 'a' && c <= 'f':
				b = b<<4 | int(c-'a'+10)
			default:
				t.Fatalf("%q is not hex", s)
			}
		}
		out[i] = byte(b)
	}
	return out
}

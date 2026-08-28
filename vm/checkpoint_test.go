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

// The undo log exists so a change made inside a checkpoint can be taken back.
// A change made with no checkpoint open can never be taken back, so recording
// one is pure cost - and commit() never trimmed the log, so those unreachable
// entries accumulated for the life of the process. On a loaded node that was a
// hundred megabytes, a third of the live heap, and the garbage collector walked
// all of it on every cycle.
func TestNoUndoIsRecordedWithNoCheckpointOpen(t *testing.T) {
	s := NewStorage()
	for i := 0; i < 500; i++ {
		s.putAccount(StoredAccount{Address: acctAddr(i), Balance: "1", Nonce: "1"})
	}
	if got := len(s.modifications); got != 0 {
		t.Fatalf("the undo log holds %d entry(s) after 500 writes with nothing to revert to", got)
	}
}

// Committing the outermost checkpoint puts its changes beyond recall, so the
// log can go back to empty. An inner commit must NOT clear it: a checkpoint
// still open outside can revert those changes yet.
func TestTheUndoLogEmptiesOnTheOutermostCommit(t *testing.T) {
	s := NewStorage()

	s.checkpoint()
	s.putAccount(StoredAccount{Address: acctAddr(1), Balance: "1", Nonce: "1"})
	s.checkpoint()
	s.putAccount(StoredAccount{Address: acctAddr(2), Balance: "2", Nonce: "1"})

	s.commit() // inner
	if len(s.modifications) == 0 {
		t.Fatal("an inner commit cleared the undo log, so the outer checkpoint can no longer revert")
	}
	s.commit() // outermost
	if got := len(s.modifications); got != 0 {
		t.Fatalf("the undo log holds %d entry(s) after the outermost commit", got)
	}
}

// The outer checkpoint has to be able to revert changes an inner commit
// accepted - which is what stops the inner commit from simply clearing the log.
func TestAnOuterRevertUndoesInnerCommittedChanges(t *testing.T) {
	s := NewStorage()
	const addr = "00000000000000000000000000000000000000a1"
	s.putAccount(StoredAccount{Address: addr, Balance: "100", Nonce: "1"})

	s.checkpoint()
	s.checkpoint()
	s.putAccount(StoredAccount{Address: addr, Balance: "999", Nonce: "2"})
	s.commit() // inner: accepted, but the outer checkpoint still stands
	s.revert() // outer: takes it back

	got, ok := s.getAccount(mustHex(t, addr))
	if !ok {
		t.Fatal("the account is gone")
	}
	if got.Balance != "100" {
		t.Fatalf("balance is %s after the outer revert, want the pre-checkpoint 100", got.Balance)
	}
}

// A slot written twice inside one checkpoint must come back to the value it had
// when the checkpoint was taken, not to the value it held between the two
// writes. That is what fixes the order the undo entries are applied in.
func TestRevertUndoesRepeatedWritesToTheValueAtTheCheckpoint(t *testing.T) {
	s := NewStorage()
	const addr = "00000000000000000000000000000000000000b2"
	s.putAccount(StoredAccount{Address: addr, Balance: "10", Nonce: "1"})

	s.checkpoint()
	s.putAccount(StoredAccount{Address: addr, Balance: "20", Nonce: "2"})
	s.putAccount(StoredAccount{Address: addr, Balance: "30", Nonce: "3"})
	s.revert()

	got, _ := s.getAccount(mustHex(t, addr))
	if got.Balance != "10" {
		t.Fatalf("balance is %s after revert, want the value at the checkpoint (10)", got.Balance)
	}
	if got.Nonce != "1" {
		t.Fatalf("nonce is %s after revert, want the value at the checkpoint (1)", got.Nonce)
	}
}

// Contract storage slots go through the same log, so they get the same
// treatment.
func TestNoUndoIsRecordedForContractSlotsWithNoCheckpointOpen(t *testing.T) {
	s := NewStorage()
	addr := mustHex(t, "00000000000000000000000000000000000000c3")
	s.putAccount(StoredAccount{Address: "00000000000000000000000000000000000000c3", Balance: "1", Nonce: "1"})
	for i := 0; i < 200; i++ {
		s.putValue(addr, []byte{byte(i), byte(i >> 8)}, []byte{1, 2, 3})
	}
	if got := len(s.modifications); got != 0 {
		t.Fatalf("the undo log holds %d entry(s) after 200 contract writes with nothing to revert to", got)
	}
}

func acctAddr(i int) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 40)
	for j := range out {
		out[j] = '0'
	}
	out[38] = hexDigits[(i>>4)&0xf]
	out[39] = hexDigits[i&0xf]
	out[36] = hexDigits[(i>>12)&0xf]
	out[37] = hexDigits[(i>>8)&0xf]
	return string(out)
}

// The guard in putAccount is an allocation guard: record() would drop the entry
// anyway, so no behavioural test can see it. What it saves is building an undo
// record - and a copy of the account it replaces - for every write on a node
// that is not proposing anything. At a thousand writes a second that is the
// difference the profile was pointing at, so it is measured rather than
// described.
func TestWritingWithNoCheckpointOpenDoesNotAllocateUndoRecords(t *testing.T) {
	s := NewStorage()
	const addr = "00000000000000000000000000000000000000d4"
	// Seeded first, so the write under test is a replacement - the case that
	// copies the existing account into an undo record.
	s.putAccount(StoredAccount{Address: addr, Balance: "1", Nonce: "1"})
	account := StoredAccount{Address: addr, Balance: "2", Nonce: "2"}

	outside := testing.AllocsPerRun(200, func() { s.putAccount(account) })

	s.checkpoint()
	inside := testing.AllocsPerRun(200, func() { s.putAccount(account) })
	s.revert()

	if !(outside < inside) {
		t.Fatalf("a write outside a checkpoint allocates %.0f, inside %.0f: the undo record is being built either way",
			outside, inside)
	}
}

package vm

import (
	"testing"
	"time"
)

func TestRevertTwoAccountsChangeBalanceAndNonce(t *testing.T) {
	storage := NewStorage()

	acc1 := StoredAccount{
		Address:   "0x00004001",
		PublicKey: "0x48656c6c",
		Balance:   "100000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}

	acc2 := StoredAccount{
		Address:   "fb68",
		PublicKey: "ab68",
		Balance:   "120000000000000000000000000000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}
	storage.checkpoint()
	storage.putAccount(acc1)
	storage.putAccount(acc2)

	storage.checkpoint()
	acc1.Balance, acc2.Balance = "80000000000000000000", "19000000"
	acc1.Nonce, acc2.Nonce = "11", "1"
	storage.putAccount(acc1)
	storage.putAccount(acc2)
	storage.revert()
	acc1, _ = storage.getAccount(acc1.AddressBytes())
	acc2, _ = storage.getAccount(acc2.AddressBytes())

	// Reverting the second checkpoint restores what the first one stored.
	// These conditions were joined with && and expected "10000" for a balance
	// that was stored as "100000", so the assertion could not fail.
	if acc1.Balance != "100000" {
		t.Errorf("after revert acc1 balance is %s, want 100000", acc1.Balance)
	}
	if acc2.Balance != "120000000000000000000000000000" {
		t.Errorf("after revert acc2 balance is %s, want 120000000000000000000000000000", acc2.Balance)
	}
	if acc1.Nonce != "0" {
		t.Errorf("after revert acc1 nonce is %s, want 0", acc1.Nonce)
	}
	if acc2.Nonce != "0" {
		t.Errorf("after revert acc2 nonce is %s, want 0", acc2.Nonce)
	}
}

// An address carrying the 0x prefix must not take the process down: putAccount
// strips the prefix when storing, so callers routinely still hold it.
func TestAddressBytesAcceptsThePrefix(t *testing.T) {
	prefixed := StoredAccount{Address: "0x00004001"}
	plain := StoredAccount{Address: "00004001"}
	got, want := prefixed.AddressBytes(), plain.AddressBytes()
	if len(want) == 0 {
		t.Fatalf("unprefixed address did not decode")
	}
	if string(got) != string(want) {
		t.Fatalf("prefixed address decoded to %x, want %x", got, want)
	}
	if bad := (StoredAccount{Address: "nothex"}).AddressBytes(); bad != nil {
		t.Fatalf("a non-hex address decoded to %x, want nil", bad)
	}
}

func TestRevertChangesAfterCommit(t *testing.T) {
	storage := NewStorage()

	acc1 := StoredAccount{
		Address:   "00004001",
		PublicKey: "48656c6c",
		Balance:   "100000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}

	acc2 := StoredAccount{
		Address:   "fb68",
		PublicKey: "ab68",
		Balance:   "120000000000000000000000000000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}

	storage.putAccount(acc1)
	storage.putAccount(acc2)
	storage.checkpoint()

	acc1.Balance, acc2.Balance = "80000000000000000000", "19000000"
	acc1.Nonce, acc2.Nonce = "11", "1"
	storage.putAccount(acc1)
	storage.putAccount(acc2)
	storage.checkpoint()

	acc1.Balance, acc2.Balance = "1000000000", "189839200000"
	acc1.Nonce, acc2.Nonce = "12", "2"
	storage.putAccount(acc1)
	storage.putAccount(acc2)
	storage.checkpoint()

	acc1.Balance, acc2.Balance = "300000000000", "78000000000000000000000"
	acc1.Nonce, acc2.Nonce = "13", "3"
	storage.putAccount(acc1)
	storage.putAccount(acc2)
	storage.commit()

	storage.revert()
	storage.revert()

	acc1, _ = storage.getAccount(acc1.AddressBytes())
	acc2, _ = storage.getAccount(acc2.AddressBytes())

	// Reverting back past every checkpoint restores the state the accounts were
	// first stored with. These were joined with && and expected "10000" for a
	// balance stored as "100000", so they could not fail.
	if acc1.Balance != "100000" {
		t.Errorf("after revert acc1 balance is %s, want 100000", acc1.Balance)
	}
	if acc2.Balance != "120000000000000000000000000000" {
		t.Errorf("after revert acc2 balance is %s, want 120000000000000000000000000000", acc2.Balance)
	}
	if acc1.Nonce != "0" {
		t.Errorf("after revert acc1 nonce is %s, want 0", acc1.Nonce)
	}
	if acc2.Nonce != "0" {
		t.Errorf("after revert acc2 nonce is %s, want 0", acc2.Nonce)
	}

}

func TestRecoverAfterRevert(t *testing.T) {
	storage := NewStorage()

	acc1 := StoredAccount{
		Address:   "00004001",
		PublicKey: "48656c6c",
		Balance:   "100000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}

	acc2 := StoredAccount{
		Address:   "fb68",
		PublicKey: "ab68",
		Balance:   "120000000000000000000000000000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}

	storage.putAccount(acc1)
	storage.putAccount(acc2)
	storage.checkpoint()

	acc1.Balance, acc2.Balance = "80000000000000000000", "19000000"
	acc1.Nonce, acc2.Nonce = "11", "1"
	storage.putAccount(acc1)
	storage.putAccount(acc2)
	storage.checkpoint()

	storage.revert()
	storage.revert()
	acc1, _ = storage.getAccount(acc1.AddressBytes())
	acc2, _ = storage.getAccount(acc2.AddressBytes())
	// Reverting back past every checkpoint restores the state the accounts were
	// first stored with. These were joined with && and expected "10000" for a
	// balance stored as "100000", so they could not fail.
	if acc1.Balance != "100000" {
		t.Errorf("after revert acc1 balance is %s, want 100000", acc1.Balance)
	}
	if acc2.Balance != "120000000000000000000000000000" {
		t.Errorf("after revert acc2 balance is %s, want 120000000000000000000000000000", acc2.Balance)
	}
	if acc1.Nonce != "0" {
		t.Errorf("after revert acc1 nonce is %s, want 0", acc1.Nonce)
	}
	if acc2.Nonce != "0" {
		t.Errorf("after revert acc2 nonce is %s, want 0", acc2.Nonce)
	}
	acc1.Balance, acc2.Balance = "8000000", "9000000000000"
	acc1.Nonce, acc2.Nonce = "11", "1"
	storage.putAccount(acc1)
	storage.putAccount(acc2)
	acc1, _ = storage.getAccount(acc1.AddressBytes())
	acc2, _ = storage.getAccount(acc2.AddressBytes())
	// Writes made after a revert are ordinary writes and must be readable.
	if acc1.Balance != "8000000" {
		t.Errorf("acc1 balance is %s, want 8000000", acc1.Balance)
	}
	if acc2.Balance != "9000000000000" {
		t.Errorf("acc2 balance is %s, want 9000000000000", acc2.Balance)
	}
	if acc1.Nonce != "11" {
		t.Errorf("acc1 nonce is %s, want 11", acc1.Nonce)
	}
	if acc2.Nonce != "1" {
		t.Errorf("acc2 nonce is %s, want 1", acc2.Nonce)
	}

}

// checkpoint1
//    -- checkpoint 2
//    -- commit 2
//    -- checkpoint 3
//    -- revert 3
// revert 1
// state will be on checkpoint1

func TestNestedRevert(t *testing.T) {
	storage := NewStorage()

	acc1 := StoredAccount{
		Address:   "acc1",
		PublicKey: "acc2",
		Balance:   "100000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}
	acc2 := StoredAccount{
		Address:   "acc2",
		PublicKey: "ab68",
		Balance:   "120000000000000000000000000000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}

	storage.putAccount(acc1)
	storage.checkpoint()

	acc1.Balance = "80000000000000000000"
	acc1.Nonce = "1"
	storage.putAccount(acc1)
	acc1.Balance = "19000000"
	acc1.Nonce = "2"
	storage.putAccount(acc1)
	storage.checkpoint()

	storage.putAccount(acc2)
	storage.commit()

	acc2.Balance = "4444444444444444444444"
	acc2.Nonce = "1"
	storage.putAccount(acc2)
	storage.checkpoint()

	storage.revert()
	storage.revert()

	acc1, _ = storage.getAccount(acc1.AddressBytes())
	acc2, acc2Exists := storage.getAccount(acc2.AddressBytes())

	// acc1 goes back to what it held at the first checkpoint. acc2 was created
	// inside the reverted range - committing the inner checkpoint does not make
	// it survive an outer revert - so it must be gone entirely.
	if acc1.Balance != "100000" {
		t.Errorf("after revert acc1 balance is %s, want 100000", acc1.Balance)
	}
	if acc1.Nonce != "0" {
		t.Errorf("after revert acc1 nonce is %s, want 0", acc1.Nonce)
	}
	if acc2Exists {
		t.Errorf("acc2 was created inside the reverted range but is still stored: %v", acc2)
	}

}

func TestRevertNestedCommitedAccount(t *testing.T) {
	storage := NewStorage()

	acc1 := StoredAccount{
		Address:   "acc1",
		PublicKey: "acc2",
		Balance:   "100000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}
	acc2 := StoredAccount{
		Address:   "acc2",
		PublicKey: "ab68",
		Balance:   "120000000000000000000000000000",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}
	acc3 := StoredAccount{
		Address:   "acc3",
		PublicKey: "acf3",
		Balance:   "0",
		Nonce:     "0",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now().Local(),
	}

	storage.putAccount(acc1)
	storage.checkpoint()

	acc1.Balance = "80000000000000000000"
	acc1.Nonce = "1"
	storage.putAccount(acc1)
	acc1.Balance = "19000000"
	acc1.Nonce = "2"
	storage.putAccount(acc1)
	storage.checkpoint()

	storage.putAccount(acc2)
	storage.commit()

	storage.checkpoint()

	storage.putAccount(acc3)
	acc1.Balance = "28000000"
	acc1.Nonce = "3"
	storage.putAccount(acc1)
	storage.checkpoint()

	acc3.Balance = "11111111111111111111"
	acc1.Nonce = "1"

	storage.revert()
	storage.revert()
	storage.revert()

	acc1, _ = storage.getAccount(acc1.AddressBytes())
	acc2, acc2Exist := storage.getAccount(acc2.AddressBytes())
	acc3, acc3Exist := storage.getAccount(acc3.AddressBytes())

	if acc2Exist {
		t.Errorf("Account2 should be reverted and mustn't be present in storage, actual acc2 state is %v", acc2)
	}

	if acc3Exist {
		t.Errorf("Account2 should be reverted and mustn't be present in storage, actual acc2 state is %v", acc3)
	}

	if acc1.Balance != "100000" {
		t.Errorf("after revert acc1 balance is %s, want 100000", acc1.Balance)
	}
	if acc1.Nonce != "0" {
		t.Errorf("after revert acc1 nonce is %s, want 0", acc1.Nonce)
	}

}

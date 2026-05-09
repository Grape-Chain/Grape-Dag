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
	if acc1.Balance != "10000" && acc2.Balance != "120000000000000000000000000000" {
		t.Errorf("Revert is not  working, balance were %s and %s,but expected were 10000 and 120000000000000000000000000000", acc1.Balance, acc2.Balance)
	}

	if acc1.Nonce != "0" && acc2.Nonce != "0" {
		t.Errorf("Revert is not  working, t.Errorf, expected nonce were 0 and 0, but were %s and %s", acc1.Nonce, acc2.Nonce)
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

	if acc1.Balance != "10000" && acc2.Balance != "120000000000000000000000000000" {
		t.Errorf("Revert is not  working, balance were %s and %s,but expected were 10000 and 120000000000000000000000000000", acc1.Balance, acc2.Balance)
	}

	if acc1.Nonce != "0" && acc2.Nonce != "0" {
		t.Errorf("Revert is not  working, t.Errorf, expected nonce were 0 and 0, but were %s and %s", acc1.Nonce, acc2.Nonce)
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
	if acc1.Balance != "10000" && acc2.Balance != "120000000000000000000000000000" {
		t.Errorf("Revert is not  working, balance were %s and %s,but expected were 10000 and 120000000000000000000000000000", acc1.Balance, acc2.Balance)
	}

	if acc1.Nonce != "0" && acc2.Nonce != "0" {
		t.Errorf("Revert is not  working, t.Errorf, expected nonce were 0 and 0, but were %s and %s", acc1.Nonce, acc2.Nonce)
	}
	acc1.Balance, acc2.Balance = "8000000", "9000000000000"
	acc1.Nonce, acc2.Nonce = "11", "1"
	storage.putAccount(acc1)
	storage.putAccount(acc2)
	acc1, _ = storage.getAccount(acc1.AddressBytes())
	acc2, _ = storage.getAccount(acc2.AddressBytes())
	if acc1.Balance != "8000000" && acc2.Balance != "9000000000000" {
		t.Errorf("Revert is not  working, balance were %s and %s,but expected were 8000000 and 19000000000000", acc1.Balance, acc2.Balance)
	}

	if acc1.Nonce != "11" && acc2.Nonce != "1" {
		t.Errorf("Revert is not  working, t.Errorf, expected nonce were 11 and 1, but were %s and %s", acc1.Nonce, acc2.Nonce)
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
	acc2, _ = storage.getAccount(acc2.AddressBytes())

	if acc1.Balance != "10000" && acc2.Balance != "" {
		t.Errorf("Revert is not  working, balance were %s and %s,but expected were 10000 and ", acc1.Balance, acc2.Balance)
	}

	if acc1.Nonce != "0" && acc2.Nonce != "" {
		t.Errorf("Revert is not  working, t.Errorf, expected nonce were 0 and 0, but were %s and %s", acc1.Nonce, acc2.Nonce)
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

	if acc1.Balance != "10000" && acc1.Nonce != "0" {
		t.Errorf("Revert is not  working expected balance was 10000, but was %s and expected nonce was 0, but was %s", acc1.Balance, acc1.Nonce)
	}

	if acc1.Nonce != "0" && acc2.Nonce != "0" && acc3.Nonce != "0" {
		t.Errorf("Revert is not  working, t.Errorf, expected nonce were 0, 0 and 0, but were %s, %s and %s", acc1.Nonce, acc2.Nonce, acc3.Nonce)
	}

}

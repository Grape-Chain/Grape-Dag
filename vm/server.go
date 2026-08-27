package vm

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/types"
	golog "github.com/ipfs/go-log/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var slogger golog.EventLogger = golog.Logger("state-server")
var emptyArray []byte = make([]byte, 32)

const keccak256NullHash = "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"

type Log struct {
	ContractAddress []byte   `json:"contractAddress,omitempty"`
	Topics          [][]byte `json:"topics,omitempty"`
	Data            []byte   `json:"data,omitempty"`
	PinTxNumber     int      `json:"pinTxNumber,omitempty"`
	TransactionHash []byte   `json:"transactionHash,omitempty"`
}

func (l Log) fromPb(pbl *pb.Log) Log {
	l.ContractAddress = pbl.ContractAddress.AddBytes
	topics := [][]byte{}
	for _, t := range pbl.Topics {
		topics = append(topics, t.Hash)
	}
	l.Topics = topics
	l.Data = pbl.Data
	l.PinTxNumber = int(pbl.Block)
	l.TransactionHash, _ = CurrentExecutingTx().Hash(crypto.SHA256)
	return l
}

type Mapping struct {
	Address []byte
	Key     []byte
	Value   []byte
}

type Diff struct {
	Account          *StoredAccount
	MappingDiffValue *Mapping
}

func (d Diff) HasAccount() bool {
	return d.Account != nil
}

type Diffs struct {
	accountDiffs map[string]StoredAccount
	mappingDiffs map[string]Mapping
}

func (d Diffs) reset() {
	for key, _ := range d.accountDiffs {
		delete(d.accountDiffs, key)
	}
	for key, _ := range d.mappingDiffs {
		delete(d.mappingDiffs, key)
	}
}

func (d Diffs) size() int {
	return len(d.accountDiffs) + len(d.mappingDiffs)
}

func (d Diffs) toDiffSlice() []Diff {
	result := []Diff{}
	for _, acc := range d.accountDiffs {
		accAddr := acc
		result = append(result, Diff{Account: &accAddr})
	}
	for _, mapping := range d.mappingDiffs {
		mapAddr := mapping
		result = append(result, Diff{MappingDiffValue: &mapAddr})
	}
	return result
}

func (d Diffs) putAccount(s StoredAccount) {
	d.accountDiffs[s.Address] = s
}

func (d Diffs) removeAccount(s StoredAccount) {
	delete(d.accountDiffs, s.Address)
}

func (d Diffs) removeMapping(address []byte, key []byte) {
	mappingKey := hex.EncodeToString(address) + ":" + hex.EncodeToString(key)
	delete(d.mappingDiffs, mappingKey)
}

func (d Diffs) putMapping(address []byte, key []byte, value []byte) {
	mappingKey := hex.EncodeToString(address) + ":" + hex.EncodeToString(key)
	d.mappingDiffs[mappingKey] = Mapping{Address: address, Key: key, Value: value}
}

type Storage struct {
	mappings map[string]map[string][]byte
	accounts map[string]StoredAccount
	logs     map[string][]Log

	// transactional mechanism supporting nested transactionals
	checkPoints   []int
	modifications []Modification

	// capture diffs of the state
	collectDiffs bool
	diffs        Diffs
}

func NewStorage() *Storage {
	s := Storage{}
	s.accounts = map[string]StoredAccount{}
	s.mappings = map[string]map[string][]byte{}
	s.logs = map[string][]Log{}
	s.diffs = Diffs{accountDiffs: map[string]StoredAccount{}, mappingDiffs: map[string]Mapping{}}
	return &s
}

func (s *Storage) getAccount(address []byte) (StoredAccount, bool) {
	account, exists := s.accounts[hex.EncodeToString(address)]
	return account, exists
}

func (s *Storage) getValue(address []byte, key []byte) ([]byte, bool) {
	account, exists := s.accounts[hex.EncodeToString(address)]
	if !exists {
		slogger.Warnf("Account %s doesn't exist, unable to get storage for it by key %s", hex.EncodeToString(address), hex.EncodeToString(key))
		return emptyArray, false
	}
	accountStore, exists := s.mappings[account.Address]
	if !exists {
		slogger.Warnf("Account store (mapping) doesn't exist for address=%s")
		return emptyArray, false
	}
	value, exists := accountStore[hex.EncodeToString(key)]
	if !exists {
		slogger.Warnf("Value for address=%s and key=%s wasn't found in mapping", hex.EncodeToString(address), hex.EncodeToString(key))
		return emptyArray, false
	}
	return value, true
}

func (s *Storage) putValue(address []byte, key []byte, value []byte) {
	hexAddress := hex.EncodeToString(address)
	_, exists := s.accounts[hexAddress]
	keyHex := hex.EncodeToString(key)
	if !exists {
		slogger.Warnf("Account %s doesn't exist, creating new to put storage %s", hex.EncodeToString(address), hex.EncodeToString(key))
		newAccount := StoredAccount{Address: hex.EncodeToString(address), Balance: "0", Nonce: "0", CodeHash: "", Code: ""}
		s.putAccount(newAccount)
	}
	accountStore, exists := s.mappings[hexAddress]

	if !exists {
		slogger.Warnf("Account store (mapping) doesn't exist for address=%s, to successfully put new value into a storage will create mapping for this account", hexAddress)
		s.mappings[hexAddress] = make(map[string][]byte)
		accountStore = s.mappings[hexAddress]
	}

	preValue, exists := accountStore[keyHex]
	if !exists {
		slogger.Infof("Previous value didn't exist, replace it by empty value, key=%s", hex.EncodeToString(key))
		preValue = emptyArray
	}

	s.modifications = append([]Modification{{mapping: &ModifiedMapping{address: hexAddress, key: hex.EncodeToString(key), value: preValue}}}, s.modifications...)
	slogger.Infof("Put contract=%s storage, key=%s, value=%s, preValue=%s", hexAddress, hex.EncodeToString(key), hex.EncodeToString(value), hex.EncodeToString(preValue))
	accountStore[keyHex] = value
	if s.collectDiffs {
		slogger.Infof("Capture new mapping change, contract=%s, key=%s, value=%s", hexAddress, hex.EncodeToString(key), hex.EncodeToString(value))
		s.diffs.putMapping(address, key, value)
	}
}

func (s *Storage) putAccount(account StoredAccount) {
	account.Address = strings.TrimPrefix(account.Address, "0x")
	existingAccc, exists := s.accounts[account.Address]
	modification := Modification{}
	if exists {
		modification.accountState = &existingAccc
	} else {
		modification.accountState = &StoredAccount{Address: account.Address}
	}
	s.accounts[account.Address] = account
	if s.collectDiffs {
		s.diffs.putAccount(account)
	}
	s.modifications = append([]Modification{modification}, s.modifications...)
	slogger.Infof("Put account, address=%s, balance%s, nonce=%s, codeHash=%s into state store", account.Address, account.Balance, account.Nonce, account.CodeHash)
	_, exists = s.mappings[account.Address]
	if !exists {
		slogger.Infof("Initialize account state store (mappings) for address %s", account.Address)
		s.mappings[account.Address] = make(map[string][]byte)
		s.logs[account.Address] = make([]Log, 0)
	}
}

func (s *Storage) putAccountPb(account *pb.Account) {
	s.putAccount(StoredAccount{
		Address:  hex.EncodeToString(account.Address.AddBytes),
		Balance:  account.Balance,
		Nonce:    strconv.FormatInt(account.Nonce, 10),
		CodeHash: account.CodeHash,
	})
}

func (s *Storage) putLogs(logs []*pb.Log) error {
	if len(logs) == 0 {
		slogger.Warn("Nothing to store, no logs")
		return errors.New("no logs")
	}
	logsToStore := []Log{}
	for _, l := range logs {
		address := l.ContractAddress.AddBytes
		addressString := hex.EncodeToString(address)
		_, exists := s.accounts[addressString]
		if !exists {
			err := fmt.Sprintf("Account %s doesn't exist, unable to store logs", addressString)
			slogger.Warn(err)
			return errors.New(err)
		}
		_, logsExist := s.logs[addressString]
		if !logsExist {
			slogger.Infof("Creating logs storage for account %s", addressString)
			s.logs[addressString] = []Log{}
		}
		logToStore := Log{}.fromPb(l)
		logsToStore = append(logsToStore, logToStore)
		s.logs[addressString] = append(s.logs[addressString], logToStore.fromPb(l))
		slogger.Debugf("Added log %v for contract=%s", s.logs[addressString], addressString)
	}
	slogger.Infof("Saved %d logs", len(logs))
	s.logEvents(logs)
	TriggerCallbacks(logsToStore)
	return nil
}

func (*Storage) logEvents(logs []*pb.Log) {
	for _, l := range logs {
		topicsString := ""
		for _, t := range l.Topics {
			topicsString += hex.EncodeToString(t.Hash) + ","
		}
		slogger.Debugf("Log -> Address=%s, Block=%d, Topics=%s, Data=%s",
			hex.EncodeToString(l.ContractAddress.AddBytes),
			l.Block,
			topicsString,
			hex.EncodeToString(l.Data))
	}
}

func (s *Storage) checkpoint() {
	slogger.Infof("Checkpoint on %d changes", len(s.modifications))
	s.checkPoints = append([]int{len(s.modifications)}, s.checkPoints...)
}

func (s *Storage) revert() {
	if len(s.checkPoints) == 0 {
		panic("Nothing to revert")
	}
	modificationsToRevert := s.modifications[:len(s.modifications)-s.checkPoints[0]]

	for _, modification := range modificationsToRevert {
		if modification.mapping != nil {
			s.mappings[modification.mapping.address][modification.mapping.key] = modification.mapping.value
			if s.collectDiffs {
				addressBytes, _ := hex.DecodeString(modification.mapping.address)
				keyBytes, _ := hex.DecodeString(modification.mapping.key)
				if bytes.Equal(emptyArray, modification.mapping.value) {
					s.diffs.removeMapping(addressBytes, keyBytes)
				} else {
					// TODO add removal of unchanged mappings since snapshot
					s.diffs.putMapping(addressBytes, keyBytes, modification.mapping.value)
				}
			}
		} else {
			if modification.accountState.IsEmpty() {
				delete(s.accounts, modification.accountState.Address)
				if s.collectDiffs {
					s.diffs.removeAccount(*modification.accountState)
				}
			} else {
				s.accounts[modification.accountState.Address] = *modification.accountState
				if s.collectDiffs {
					s.diffs.putAccount(*modification.accountState)
				}
			}
		}
	}

	slogger.Infof("Revert %d changes", len(modificationsToRevert))
	s.modifications = s.modifications[len(modificationsToRevert):]
	s.checkPoints = s.checkPoints[1:]
}

func (s *Storage) commit() {
	if len(s.checkPoints) == 0 {
		panic("Nothing to commit")
	}
	popCount := s.checkPoints[0]
	commited := len(s.modifications) - popCount
	slogger.Infof("Commit %d changes, left %d", commited, len(s.modifications))
	s.checkPoints = s.checkPoints[1:]
}

type StoredAccount struct {
	Address   string
	PublicKey string
	Balance   string
	Nonce     string
	CodeHash  string
	Code      string
	Created   time.Time
}

func (sa StoredAccount) IsEmpty() bool {
	return sa.Nonce == ""
}

// AddressBytes - the account address as raw bytes.
//
// Addresses reach this type both with and without the 0x prefix: putAccount
// strips it when storing, so a caller holding the value it passed in still has
// the prefix. Decoding used to panic on that - and on any other malformed
// address - which is a poor trade in a type reached from gRPC handlers that run
// without a recovery interceptor. Unreadable addresses now come back nil, which
// callers see as a lookup miss.
func (sa StoredAccount) AddressBytes() []byte {
	address := strings.TrimPrefix(strings.TrimPrefix(sa.Address, "0x"), "0X")
	bytes, err := hex.DecodeString(address)
	if err != nil {
		slogger.Warnf("Account address %q is not hex, treating it as empty", sa.Address)
		return nil
	}
	return bytes
}

type Modification struct {
	mapping      *ModifiedMapping
	accountState *StoredAccount
}
type ModifiedMapping struct {
	address string
	key     string
	value   []byte
}

type InMemoryNodeStorageServer struct {
	storage *Storage
	lock    sync.Mutex
	pb.UnimplementedNodeStorageServiceServer
}

func (s *InMemoryNodeStorageServer) GetAccount(ctx context.Context, addr *pb.Address) (*pb.Account, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	storedAcc, exists := s.storage.getAccount(addr.AddBytes)
	if exists {
		slogger.Infof("Account %s loaded from state.", hex.EncodeToString(addr.AddBytes))
		slogger.Debugf("Loaded Account from memory state %v", storedAcc)
		nonceParsed, err := strconv.ParseInt(storedAcc.Nonce, 10, 64)
		if err != nil {
			slogger.Errorf("Unable to convert string nonce to int64, maybe value %s exceeds the limit of int64", storedAcc.Nonce)
			panic(err)
		}
		return &pb.Account{
			Address:  &pb.Address{AddBytes: storedAcc.AddressBytes()},
			Balance:  storedAcc.Balance,
			Nonce:    nonceParsed,
			CodeHash: storedAcc.CodeHash,
		}, nil
	} else {
		return nil, status.Errorf(codes.NotFound, "Account by address %s was not found", hex.EncodeToString(addr.AddBytes))
	}
}
func (s *InMemoryNodeStorageServer) CreateAccount(ctx context.Context, addr *pb.Address) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.storage.putAccount(StoredAccount{
		Address:   hex.EncodeToString(addr.AddBytes),
		PublicKey: "",
		CodeHash:  "",
		Code:      "",
		Created:   time.Now(),
		Balance:   "0",
		Nonce:     "0",
	})
	return &pb.OpResult{Status: 0, Message: "Successfully created new account"}, nil
}

func (s *InMemoryNodeStorageServer) PutContractCode(ctx context.Context, req *pb.PutContractCodeRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	contractAccount, exists := s.storage.getAccount(req.ContractAddress.AddBytes)
	if !exists {
		return nil, fmt.Errorf("Account for address %s doesn't exist, no place to put code", hex.EncodeToString(req.ContractAddress.AddBytes))
	}
	contractAccount.Code = hex.EncodeToString(req.ContractCode.Bytecode)
	hasher := grape1crypto.NewSHA3Hasher()
	codeHash := hasher.Digest(req.ContractCode.Bytecode)
	contractAccount.CodeHash = hex.EncodeToString(codeHash)
	s.storage.putAccount(contractAccount)
	slogger.Infof("Put contract code for account %s with codeHash=%s, code=%s",
		contractAccount.Address, contractAccount.CodeHash, contractAccount.Code)
	return &pb.OpResult{Status: 0, Message: "Successfully put account code"}, nil
}

func (s *InMemoryNodeStorageServer) AddBalance(ctx context.Context, req *pb.AddBalanceRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	account, exists := s.storage.getAccount(req.AccountAddress.AddBytes)
	if !exists {
		return nil, fmt.Errorf("Account %s doesn't exist", hex.EncodeToString(req.AccountAddress.AddBytes))
	}
	amount := new(big.Int)
	amount.SetString(req.GetAmount(), 10)
	balance := new(big.Int)
	balance.SetString(account.Balance, 10)
	sum := new(big.Int)
	sum.Add(balance, amount)
	slogger.Infof("Add %s to account=%s balance=%s, resulting balance=%s", req.Amount, account.Address, account.Balance, sum.String())
	account.Balance = sum.String()
	s.storage.putAccount(account)

	return &pb.OpResult{Status: 0, Message: "Balance added"}, nil
}
func (s *InMemoryNodeStorageServer) SubBalance(ctx context.Context, req *pb.SubBalanceRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	account, exists := s.storage.getAccount(req.AccountAddress.AddBytes)
	if !exists {
		return nil, fmt.Errorf("Account %s doesn't exist", hex.EncodeToString(req.AccountAddress.AddBytes))
	}
	amount := new(big.Int)
	amount.SetString(req.GetAmount(), 10)
	balance := new(big.Int)
	balance.SetString(account.Balance, 10)
	sum := new(big.Int)
	sum.Sub(balance, amount)
	slogger.Infof("Substract %s from account=%s balance=%s, resulting balance=%s", req.Amount, account.Address, account.Balance, sum.String())
	account.Balance = sum.String()
	s.storage.putAccount(account)

	return &pb.OpResult{Status: 0, Message: "Balance substracted"}, nil
}
func (s *InMemoryNodeStorageServer) GetContractCode(ctx context.Context, addr *pb.Address) (*pb.ContractCode, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	contractAccount, exists := s.storage.getAccount(addr.AddBytes)
	if !exists {
		return nil, fmt.Errorf("Contract account %s doesn't exist", hex.EncodeToString(addr.AddBytes))
	}
	slogger.Infof("Get code for contract address=%s, codeHash=%s", contractAccount.Address, contractAccount.CodeHash)
	codeBytes, _ := hex.DecodeString(contractAccount.Code)
	return &pb.ContractCode{Bytecode: codeBytes}, nil
}

func (s *InMemoryNodeStorageServer) GetContractCodeStorage(ctx context.Context, req *pb.GetFromStorageByKeyRequest) (*pb.Value, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	value, _ := s.storage.getValue(req.ContractAddress.AddBytes, req.KeyToQuery.Content)
	return &pb.Value{Content: value}, nil
}

func (s *InMemoryNodeStorageServer) PutContractStorage(ctx context.Context, req *pb.PutIntoStorageRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.storage.putValue(req.ContractAddress.AddBytes, req.KeyToPut.Content, req.ValueToPut.Content)
	slogger.Infof("Save to contract storage, contract=%s, key=%s, value=%s",
		hex.EncodeToString(req.ContractAddress.AddBytes), hex.EncodeToString(req.KeyToPut.Content), hex.EncodeToString(req.ValueToPut.Content))
	return &pb.OpResult{Status: 0, Message: "Value saved"}, nil
}

func (s *InMemoryNodeStorageServer) StateCheckpoint(context.Context, *pb.TransactionalRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.storage.checkpoint()
	return &pb.OpResult{Status: 0, Message: "OK"}, nil
}
func (s *InMemoryNodeStorageServer) StateCommit(context.Context, *pb.TransactionalRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.storage.commit()
	return &pb.OpResult{Status: 0, Message: "OK"}, nil
}
func (s *InMemoryNodeStorageServer) StateRevert(context.Context, *pb.TransactionalRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.storage.revert()
	return &pb.OpResult{Status: 0, Message: "OK"}, nil
}

func (s *InMemoryNodeStorageServer) SetNonce(ctx context.Context, req *pb.SetNonceRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	account, exists := s.storage.getAccount(req.ContractAddress.AddBytes)
	if !exists {
		return nil, fmt.Errorf("unable to set nonce for non-existent account=%s", hex.EncodeToString(req.ContractAddress.AddBytes))
	}
	slogger.Infof("Set nonce=%d for contract account=%s, prevNonce=%s", req.Nonce, account.Address, account.Nonce)
	account.Nonce = strconv.FormatInt(req.Nonce, 10)
	s.storage.putAccount(account)
	return &pb.OpResult{Status: 0, Message: "OK"}, nil
}

func (s *InMemoryNodeStorageServer) SaveLogs(ctx context.Context, req *pb.SaveLogsRequest) (*pb.OpResult, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.storage.putLogs(req.Logs)
	if err != nil {
		return nil, err
	}
	return &pb.OpResult{Status: 0, Message: "OK"}, nil
}

// const stateServerPort int = 39399
const stateServerHost string = "localhost"

var server *InMemoryNodeStorageServer

func StartStateServer() error {
	serverURI := fmt.Sprintf("%s:%d", stateServerHost, config.GetConfig().Peer.StateServerPort)
	lis, err := net.Listen("tcp", serverURI)
	if err != nil {
		return fmt.Errorf("[gRPC VM State Service] failed to start using URI=%s, maybe port is busy: %v", serverURI, err)
	}
	grpcServer := grpc.NewServer()
	storage := NewStorage()

	server = &InMemoryNodeStorageServer{storage: storage}
	pb.RegisterNodeStorageServiceServer(grpcServer, server)
	slogger.Infof("[gRPC VM State Service] Started on port %s", lis.Addr().String())
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			slogger.Fatalf("[gRPC VM State Service] Failed serving gRPC request %v", err)
		}
		slogger.Infof("[gRPC VM State Service] Stopped.")
	}()
	return nil
}

// func putGenesisAccounts(storage *Storage) {
// 	for _, gw := range GenesisAccounts {

// 		accountToStore := StoredAccount{
// 			Address:   gw.Id,
// 			Balance:   gw.Balance.String(),
// 			PublicKey: gw.PublicKey,
// 			Nonce:     gw.Nonce.String(),
// 			CodeHash:  "",
// 			Created:   time.Now(),
// 		}
// 		storage.putAccount(accountToStore)
// 	}
// 	storage.putAccount(StoredAccount{ // put zero account into state
// 		Address:  "0x0000000000000000000000000000000000000000",
// 		Balance:  "0",
// 		Nonce:    "0",
// 		CodeHash: "",
// 		Created:  time.Now(),
// 	})
// }

func CaptureStateStoreDiffs() {
	server.lock.Lock()
	defer server.lock.Unlock()
	if server.storage.collectDiffs {
		panic(errors.New("diffs collection is already enabled"))
	}
	server.storage.collectDiffs = true
	slogger.Info("Diffs collection from state store is enabled")
}

func NonceIncr(account types.Hex) {
	server.lock.Lock()
	defer server.lock.Unlock()

	acc, exist := server.storage.getAccount(account)

	prevNonce := "0"
	if !exist {
		server.storage.putAccount(StoredAccount{
			Address:   hex.EncodeToString(account),
			PublicKey: "",
			CodeHash:  "",
			Code:      "",
			Created:   time.Now(),
			Balance:   "0",
			Nonce:     "1",
		})
	} else {
		nonce, _ := big.NewInt(0).SetString(acc.Nonce, 10)
		prevNonce = acc.Nonce
		nonce = nonce.Add(nonce, big.NewInt(1))
		acc.Nonce = nonce.String()
		server.storage.putAccount(acc)
	}
	slogger.Infof("Increase prevNonce=%s for account=%s manually", prevNonce, account.String())
}

func GetStateStoreDiffs() []Diff {
	server.lock.Lock()
	defer server.lock.Unlock()
	if !server.storage.collectDiffs {
		panic(errors.New("diffs collection wasn't enabled, nothing to retrieve"))
	}
	slogger.Infof("Retrieve %d collected diffs from state store", server.storage.diffs.size())
	return server.storage.diffs.toDiffSlice()
}

func ResetCaptureStateStoreDiffs() {
	server.lock.Lock()
	defer server.lock.Unlock()
	slogger.Infof("Reset capture of state store diffs, total collected=%d", server.storage.diffs.size())
	server.storage.collectDiffs = false
	server.storage.diffs.reset()
	slogger.Debugf("Reset capture of state store diffs finished, current size = %d", server.storage.diffs.size())
}

func SubBalance(pubKey []byte, amount *big.Int) error {
	server.lock.Lock()
	defer server.lock.Unlock()
	address := grape1crypto.AddressFromPulicKey(grape1crypto.PublicKey(pubKey))
	acc, exists := server.storage.getAccount(grape1crypto.AddressToBytes(address))
	if !exists {
		return fmt.Errorf("account doesn't exist %s", address)
	}
	balance := big.NewInt(0)
	balance.SetString(acc.Balance, 10)
	if balance.Cmp(amount) < 0 {
		return fmt.Errorf("account %s has not enough funds %s, required %s", address, acc.Balance, amount.String())
	}
	acc.PublicKey = hex.EncodeToString(pubKey)
	acc.Balance = balance.Sub(balance, amount).String()
	slogger.Infof("Account %s has spent %s, resulting balance %s, nonce %s", address, amount.String(), acc.Balance, acc.Nonce)
	server.storage.putAccount(acc)
	return nil
}

func IncrementNonce(address string) error {
	server.lock.Lock()
	defer server.lock.Unlock()
	acc, exists := server.storage.getAccount(grape1crypto.AddressToBytes(address))
	if !exists {
		return fmt.Errorf("account doesn't exist %s", address)
	}
	nonce := big.NewInt(0)
	nonce.SetString(acc.Nonce, 10)
	nonce = nonce.Add(nonce, big.NewInt(1))
	acc.Nonce = nonce.String()
	slogger.Debugf("Set nonce=%s for address %s", acc.Nonce, address)
	server.storage.putAccount(acc)
	return nil
}

func SearchAccount(address string) *LnAccount {
	server.lock.Lock()
	defer server.lock.Unlock()
	acc, exists := server.storage.getAccount(grape1crypto.AddressToBytes(address))
	if !exists {
		return nil
	}
	account := mapToLnAccount(acc)

	return account
}

func GetCodeHash(address string) string {
	server.lock.Lock()
	defer server.lock.Unlock()
	acc, exists := server.storage.getAccount(grape1crypto.AddressToBytes(address))
	if !exists {
		return keccak256NullHash
	}
	return acc.CodeHash

}

func GetContractCode(address string) string {
	server.lock.Lock()
	defer server.lock.Unlock()
	acc, exists := server.storage.getAccount(grape1crypto.AddressToBytes(address))
	if !exists {
		return ""
	}
	return acc.Code
}

func GetStorageAt(address string, key string) string {
	server.lock.Lock()
	defer server.lock.Unlock()
	value, exists := server.storage.getValue(grape1crypto.AddressToBytes(address), grape1crypto.LeftPadTo(32, grape1crypto.AddressToBytes(key)))
	if !exists {
		return grape1crypto.ZeroHex(32)
	}
	return "0x" + hex.EncodeToString(value)
}

type SearchLogRequest struct {
	Address string
	Topics  []string
	Limit   int
	Offset  int
	PinFrom int
	PinTo   int
}

func GetLogsForTx(hash []byte) []Log {
	server.lock.Lock()
	defer server.lock.Unlock()
	result := []Log{}
	for _, logs := range server.storage.logs {
		for _, l := range logs {
			if bytes.Equal(l.TransactionHash, hash) {
				result = append(result, l)
			}
		}
	}
	return result
}

func SearchLogs(r SearchLogRequest) []Log {
	server.lock.Lock()
	defer server.lock.Unlock()
	r.Address = strings.TrimPrefix(r.Address, "0x")
	logsForContract, exists := server.storage.logs[r.Address]
	var result []Log
	if !exists {
		return result
	}
OUTER:
	for i := len(logsForContract) - 1; i >= 0; i-- {
		log := logsForContract[i]
		if log.PinTxNumber > r.PinTo || log.PinTxNumber < r.PinFrom {
			continue
		}
		for tIndex, rTopic := range r.Topics {
			if rTopic == "null" {
				continue
			}
			if tIndex >= len(log.Topics) {
				continue OUTER
			}
			if strings.TrimPrefix(rTopic, "0x") != hex.EncodeToString(log.Topics[tIndex]) {
				continue OUTER
			}
		}
		// log matches request
		result = append(result, log)
	}
	// apply pagination
	if len(result) <= r.Offset {
		return []Log{}
	}
	result = result[r.Offset:]
	if len(result) < r.Limit {
		return result
	} else {
		return result[:r.Limit]
	}
}

func SyncBalances(balances map[string][]byte) {
	server.lock.Lock()
	defer server.lock.Unlock()
	slogger.Infof("Run sync of %d balances from pin tx", len(balances))
	for key, value := range balances {
		address := grape1crypto.AddressToBytes(key)
		account, exists := server.storage.getAccount(address)
		newBalance := big.NewInt(0).SetBytes(value).String()
		if !exists {
			newAccount := StoredAccount{
				Address:   key,
				PublicKey: "", Balance: newBalance, Nonce: "0", CodeHash: "", Code: "", Created: time.Now()}
			server.storage.putAccount(newAccount)
			slogger.Infof("Create account=%s from pin tx balance sync, newBalance=%s", newAccount.Address, newAccount.Balance)
		} else {
			slogger.Infof("Update existing account=%s balance, prev=%s, new=%s", account.Address, account.Balance, newBalance)
			account.Balance = newBalance
			server.storage.putAccount(account)
		}
	}
}

func DumpMappingDiffs(d *pb.Diff) {
	server.lock.Lock()
	defer server.lock.Unlock()
	slogger.Infof("Dumping mapping diffs started")
	address := hex.EncodeToString(d.GetMappingDiff().Address)
	key := hex.EncodeToString(d.GetMappingDiff().Key)
	server.storage.mappings[address][key] = d.GetMappingDiff().Value

}

func IsSCAccount(walletId string) bool {
	trimmedId := strings.TrimPrefix(walletId, "0x")
	idBytes, err := hex.DecodeString(trimmedId)
	if err != nil {
		panic(err)
	}
	acc, found := server.storage.getAccount(idBytes)
	if !found {
		return false
	}
	return acc.CodeHash != ""
}

func mapToLnAccount(acc StoredAccount) *LnAccount {
	balance := big.NewInt(0)
	balance.SetString(acc.Balance, 10)
	nonce := big.NewInt(0)
	nonce.SetString(acc.Nonce, 10)
	account := LnAccount{
		Id:        acc.Address,
		Balance:   *balance,
		Created:   acc.Created,
		Nonce:     *nonce,
		PublicKey: acc.PublicKey,
	}
	return &account
}

func SearchAccounts(limit int, offset int, ascSort bool) []*LnAccount {
	server.lock.Lock()
	defer server.lock.Unlock()
	accsArray := make([]*LnAccount, 0, len(server.storage.accounts))
	for _, v := range server.storage.accounts {
		accsArray = append(accsArray, mapToLnAccount(v))
	}

	sort.SliceStable(accsArray, func(i int, j int) bool {
		var first, second *LnAccount
		if !ascSort {
			first = accsArray[j]
			second = accsArray[i]
		} else {
			first = accsArray[i]
			second = accsArray[j]
		}
		if first.Created.Before(second.Created) {
			return true
		} else {
			return false
		}
	})
	if len(accsArray) <= offset {
		return []*LnAccount{}
	}
	accsArray = accsArray[offset:]
	if len(accsArray) < limit {
		return accsArray
	} else {
		return accsArray[:limit]
	}
}

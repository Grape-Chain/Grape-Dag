package rest

import (
	"fmt"
	"sync"
)

var store map[string]string = map[string]string{}
var lock sync.Mutex

func GetABI(contractAddress string) string {
	lock.Lock()
	defer lock.Unlock()
	abi, exists := store[contractAddress]
	if !exists {
		panic(fmt.Errorf("abi for contract %s not found", contractAddress))
	}
	return abi
}

func PutABI(contractAddress string, abi string) {
	lock.Lock()
	defer lock.Unlock()
	store[contractAddress] = abi
}
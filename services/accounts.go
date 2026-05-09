package services

import (
	"fmt"

	"github.com/VG-Grape/luna/vm"
)

type AccountService interface {
	GetAccounts(sort Sort, p Page) []*vm.LnAccount

	GetAccountById(id string) *vm.LnAccount
}

func NewAccountService() AccountService {
	service := accountServiceImpl{}
	return &service
}

type accountServiceImpl struct {
}

func (a *accountServiceImpl) GetAccounts(sort Sort, p Page) []*vm.LnAccount {
	return vm.SearchAccounts(p.Size, p.Offset(), sort.isAsc())
}

func (a *accountServiceImpl) GetAccountById(id string) *vm.LnAccount {
	if id == "" {
		panic(fmt.Errorf("Id must not be empty"))
	}
	return vm.SearchAccount(id)
}

package rest

import (
	"crypto"
	"io"
	"math/big"
	"strconv"
	"sync"

	// "crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Grape-Chain/Grape-Dag/app"
	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/dag"
	"github.com/Grape-Chain/Grape-Dag/services"
	"github.com/Grape-Chain/Grape-Dag/services/rest/api"
	"github.com/Grape-Chain/Grape-Dag/services/rest/mapper"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/vm"
	"github.com/Grape-Chain/Grape-Dag/wallet"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/google/martian/log"
	"github.com/ledongthuc/goterators"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/peer"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"google.golang.org/protobuf/proto"
)

type NodeApiServer struct {
	config *RestAPIConfig
}

// Defaults documented in api/openapi.yml for the shared optional query params.
// oapi-codegen v1.11 does not materialize a schema `default:` into the generated
// server, so an omitted param arrives as a nil pointer and must be defaulted
// here rather than dereferenced.
const (
	defaultPage      = 0
	defaultPageSize  = 15
	defaultSortOrder = "DESC"
)

// derefOr - the value p points to, or def when p is nil.
func derefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

func (nas NodeApiServer) GetAccounts(w http.ResponseWriter, r *http.Request, params api.GetAccountsParams) {
	accounts := accService.GetAccounts(
		services.Sort(derefOr(params.SortOrder, api.GetAccountsParamsSortOrder(defaultSortOrder))),
		services.Page{
			Size:   derefOr(params.PageSize, defaultPageSize),
			Number: derefOr(params.Page, defaultPage),
		})
	var responseAccounts []api.Account
	for _, account := range accounts {
		responseAccounts = append(responseAccounts, mapper.AccountToDto(account))
	}
	w.WriteHeader(200)

	_ = json.NewEncoder(w).Encode(api.Accounts{Accounts: &responseAccounts})
}
func (nas NodeApiServer) GetAccountsAccountId(w http.ResponseWriter, r *http.Request, accountId string) {
	requireHexWithPrefixOfSize(accountId, 20, "accountId")
	foundAccount := accService.GetAccountById(accountId)
	if foundAccount == nil {
		foundAccount = &vm.LnAccount{Id: accountId, Balance: *big.NewInt(0), Nonce: *big.NewInt(0), PublicKey: "", Created: time.Now()}
	}
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(mapper.AccountToDto(foundAccount))
}

func (nas NodeApiServer) GetContractsContractId(w http.ResponseWriter, r *http.Request, contractId string, params api.GetContractsContractIdParams) {
	contractId = requireHexWithPrefixOfSize(contractId, 20, "Contract Address")
	if params.Params == nil {
		panic(errors.New("params is required: packed calldata (method hash and arguments)"))
	}
	allParams := requireHexWithPrefixOfMinSize(*params.Params, 4, "Hash of method without params")
	methodId := allParams[0:8]
	inParams := allParams[8:]
	sender := ""
	if params.Sender != nil && *params.Sender != "" {
		sender = requireHex(*params.Sender, "Sender")
	}
	res, err := txService.CallReadContract(contractId, methodId, inParams, sender)
	if err != nil {
		panic(err)
	} else {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(api.ContractMethodCallResult{Result: &res})
	}
}

func (nas NodeApiServer) GetContractsContractIdMethodsMethodId(w http.ResponseWriter, r *http.Request, contractId string,
	methodId string, params api.GetContractsContractIdMethodsMethodIdParams) {
	contractId = requireHexWithPrefixOfSize(contractId, 20, "Contract Address")
	methodId = requireHexWithPrefixOfSize(methodId, 4, "Method Id")
	allParamsMerged := ""
	// params is optional: a method taking no arguments is called without it.
	methodParams := derefOr(params.Params, []string{})
	if len(methodParams) == 1 && len(methodParams[0]) >= 66 {
		allParamsMerged = requireHex(methodParams[0], "Param")
	} else {
		for _, param := range methodParams {
			param = requireHex(param, "Param")
			allParamsMerged += pad32(param)
		}
	}
	sender := ""
	if params.Sender != nil && *params.Sender != "" {
		sender = requireHex(*params.Sender, "Sender")
	}
	res, err := txService.CallReadContract(contractId, methodId, allParamsMerged, sender)
	if err != nil {
		panic(err)
	} else {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(api.ContractMethodCallResult{Result: &res})
	}
}

func (nas NodeApiServer) GetContractsContractIdAbi(w http.ResponseWriter, r *http.Request, contractId string) {
	contractId = requireHexWithPrefixOfSize(contractId, 20, "Contract Address")
	abi := GetABI(contractId)
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(api.ABI{Content: &abi})
}

func (nas NodeApiServer) PutContractsContractIdAbi(w http.ResponseWriter, r *http.Request, contractId string) {
	contractId = requireHexWithPrefixOfSize(contractId, 20, "Contract Address")
	decodedReq := api.ABI{}
	err := json.NewDecoder(r.Body).Decode(&decodedReq)
	if err != nil {
		panic("Bad ABI json")
	}
	PutABI(contractId, *decodedReq.Content)
	w.WriteHeader(200)
	success := true
	_ = json.NewEncoder(w).Encode(api.ABISaveResponse{Success: &success})
}

func (nas NodeApiServer) GetNetworkInfo(w http.ResponseWriter, r *http.Request) {
	chainIdInt := config.GetConfig().Peer.Network
	var chainId api.NetworkInfoChainId
	if chainIdInt == 0 {
		chainId = api.NetworkInfoChainIdMAINNET
	} else if chainIdInt == 1 {
		chainId = api.NetworkInfoChainIdTESTNET1
	} else if chainIdInt == 2 {
		chainId = api.NetworkInfoChainIdTESTNET2
	} else {
		panic(fmt.Errorf("wrong network, allowed 0,1,2 got %d", chainIdInt))
	}

	_ = json.NewEncoder(w).Encode(api.NetworkInfo{ChainId: &chainId})
}

func (nas NodeApiServer) GetContractsContractIdCode(w http.ResponseWriter, r *http.Request, contractId string) {
	contractId = requireHexWithPrefixOfSize(contractId, 20, "Contract Address")
	codeHash := vm.GetCodeHash(contractId)
	codeContract := vm.GetContractCode(contractId)
	_ = json.NewEncoder(w).Encode(api.Code{CodeHash: &codeHash, CodeContract: &codeContract})

}

func (nas NodeApiServer) GetPinNumber(w http.ResponseWriter, r *http.Request) {
	currentPin := dag.GetPin().CurrentHeight()
	if currentPin < 0 {
		panic(fmt.Errorf("couldn't find any pinning tx"))
	}
	_ = json.NewEncoder(w).Encode(api.PinNumber{PinNumber: &currentPin})

}

type peerResponse struct {
	Error   bool     `json:"error"`
	Message string   `json:"message"`
	Data    []string `json:"data,omitempty"`
}

func (nas NodeApiServer) GetPeers(w http.ResponseWriter, r *http.Request, params api.GetPeersParams) {
	logger.Infof("[rest api] %s:%s[%s] from %s", r.Method, r.URL, r.UserAgent(), r.RemoteAddr)
	var options []discovery.Option
	options = append(options, discovery.Limit(10))
	options = append(options, discovery.TTL(time.Second*30))
	foundPeers, err := dutil.FindPeers(nas.config.ctx, nas.config.rd, config.RENDEZVOUS[0], options...)

	payload := peerResponse{
		Error:   err != nil,
		Message: config.RENDEZVOUS[0],
		Data:    []string{},
	}
	if err == nil {
		goterators.ForEach(foundPeers, func(p peer.AddrInfo) {
			payload.Data = append(payload.Data, p.String())
		})
	} else {
		payload.Data = append(payload.Data, err.Error())
	}

	out, _ := json.MarshalIndent(payload, "", "\t")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write(out)
}
func (nas NodeApiServer) GetSystem(w http.ResponseWriter, r *http.Request) {

}
func (nas NodeApiServer) GetPubsub(w http.ResponseWriter, r *http.Request) {
	success := app.GetApp().App_dagsyncmgr.HaveJoined.Load()
	if success {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(api.PubsubReady{Ready: &success})
	} else {
		w.WriteHeader(404)
	}

}
func (nas NodeApiServer) GetFilteredTransactions(w http.ResponseWriter, r *http.Request, params api.GetFilteredTransactionsParams) {
	var txType int
	if params.TxType == nil {
		txType = -1
	} else {
		txType = int(*params.TxType)
	}
	if txType > int(pb.TransactionType_CALL_CONTRACT) || txType < -1 {
		panic(fmt.Errorf("wrong txType for filtering, allowed -1,0,1,2 got %d", txType))
	}
	var confirmed *bool
	if params.ConfirmationStatus != nil {
		requiredStatus := strings.ToUpper(string(*params.ConfirmationStatus))
		if requiredStatus == "CONFIRMED" {
			confirmed = wrapBool(true)
		} else if requiredStatus == "UNCONFIRMED" {
			confirmed = wrapBool(false)
		}
	}
	var directionIsSent *bool
	if params.Direction != nil {
		requiredStatus := strings.ToUpper(string(*params.Direction))
		if requiredStatus == "SENT" {
			directionIsSent = wrapBool(true)
		} else if requiredStatus == "RECEIVED" {
			directionIsSent = wrapBool(false)
		}
	}
	accounts := []string{}
	if params.Accounts != nil {
		accounts = *params.Accounts
	}
	foundTxs := txService.GetTransactions(accounts, txType,
		services.Sort(derefOr(params.SortOrder, api.GetFilteredTransactionsParamsSortOrder(defaultSortOrder))),
		services.Page{
			Number: derefOr(params.Page, defaultPage),
			Size:   derefOr(params.PageSize, defaultPageSize),
		}, confirmed, directionIsSent)

	dtoTxs := make([]api.UnifiedTransaction, 0)

	for _, foundTx := range foundTxs {
		confirmedTx := mapper.TxToDto(foundTx)
		dtoTxs = append(dtoTxs, confirmedTx)
	}
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(api.UnifiedTransactions{Transactions: &dtoTxs})
}

func (nas NodeApiServer) SendRawTransaction(w http.ResponseWriter, r *http.Request) {
	var rt api.RawTransaction
	err := json.NewDecoder(r.Body).Decode(&rt)
	if err != nil || rt.EncodedTx == "" {
		panic(errors.New("unable to decode RawTransaction, bad Json scheme"))
	}
	logger.Debugf("Received new raw transaction=%v", rt)
	execResult, err := txService.SendRawTransaction(strings.TrimPrefix(rt.EncodedTx, "0x"))
	if err != nil {
		panic(err)
	}
	if execResult.Successful {
		successStatus := "SUCCESSFUL"
		_ = json.NewEncoder(w).Encode(api.TxReceipt{TxHash: &execResult.Hash, ExecutionStatus: &successStatus, FuelUsed: &execResult.GasUsed})
	} else {
		_ = json.NewEncoder(w).Encode(api.TxReceipt{TxHash: &execResult.Hash, ExecutionStatus: &execResult.Output, FuelUsed: &execResult.GasUsed})
	}
}

type CallMessageExtender api.CallMessage

func (m CallMessageExtender) isValid() bool {
	return m.Data != nil && len(*m.Data) != 0 && m.Amount != nil && *m.Amount != ""
}

func (nas NodeApiServer) EstimateTxFuel(w http.ResponseWriter, r *http.Request) {
	estimationRequest := parseEstimationRequest(r)

	logger.Debugf("Received new estimation request=%v for transaction", estimationRequest)
	execResult, err := txService.EstimateTxFuel(estimationRequest)
	if err != nil {
		panic(err)
	}
	if execResult.Successful {
		successStatus := "SUCCESSFUL"
		_ = json.NewEncoder(w).Encode(api.TxReceipt{TxHash: &execResult.Hash, ExecutionStatus: &successStatus, FuelUsed: &execResult.GasUsed})
	} else {
		_ = json.NewEncoder(w).Encode(api.TxReceipt{TxHash: &execResult.Hash, ExecutionStatus: &execResult.Output, FuelUsed: &execResult.GasUsed})
	}
}
func parseEstimationRequest(r *http.Request) services.FuelEstimationRequest {
	var rt api.RawTransaction
	var jmsg CallMessageExtender
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		panic(errors.New("Unable to read request body: " + err.Error()))
	}
	err = json.Unmarshal(bodyBytes, &jmsg)
	if err != nil || !jmsg.isValid() {
		err := json.Unmarshal(bodyBytes, &rt)
		if err != nil || rt.EncodedTx == "" {
			panic(errors.New("unable to decode RawTransaction and CallMessage, bad Json supplied"))
		} else {
			return services.FuelEstimationRequest{RawTx: parseHex(rt.EncodedTx, "encodedTx")}
		}
	} else {
		var from []byte
		if jmsg.Sender != nil {
			from = parseHex(*jmsg.Sender, "sender")
		}
		var to []byte
		if jmsg.Recipient != nil {
			to = parseHex(*jmsg.Recipient, "recipient")
		}
		amount := big.NewInt(0)
		amount, _ = amount.SetString(*jmsg.Amount, 10)
		if amount == nil {
			panic(errors.New("amount is not a number in decimal form"))
		}
		return services.FuelEstimationRequest{Message: &services.Message{From: from,
			To:     to,
			Data:   parseHex(*jmsg.Data, "data"),
			Amount: amount.Bytes()}}
	}
}

func (nas NodeApiServer) GetTransactionByHash(w http.ResponseWriter, r *http.Request, txHash string) {
	transactionByHash, exist := txService.GetTransactionByHash(txHash)
	if txHash == "" {
		panic(errors.New("txHash can not be empty"))
	}
	if !exist {
		err := "Confirmed transaction with hash '" + txHash + "' wasn't found"
		logger.Info(err)
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(api.ApiError{Error: &err})
	} else {
		_ = json.NewEncoder(w).Encode(mapper.TxToDto(transactionByHash.ToUnified()))
	}
}

type getLogsParamsExtended api.GetLogsParams

func (p getLogsParamsExtended) validate() api.GetLogsParams {
	requireHexWithPrefixOfSize(p.Address, 20, "address")
	for i, t := range p.Topics {
		if t != "null" {
			requireHexWithPrefixOfSize(t, 32, "topic"+strconv.FormatInt(int64(i), 10))
		}
	}
	if p.Page == nil {
		p.Page = new(int)
		*p.Page = 0
	}
	if *p.Page < 0 {
		panic(errors.New("page must be 0 or positive"))
	}
	if p.PageSize == nil {
		p.PageSize = new(int)
		*p.PageSize = 15
	}
	if *p.PageSize <= 0 {
		panic(errors.New("pageSize must be positive"))
	}
	lastPinTx := dag.GetPin().CurrentHeight()
	if p.PinFrom == nil {
		p.PinFrom = new(int)
		*p.PinFrom = 0
	}
	if *p.PinFrom < 0 {
		p.PinFrom = new(int)
		*p.PinFrom = 0
		logger.Warn("Number of pinning tx must be positive")
	}
	if *p.PinFrom > lastPinTx {
		p.PinFrom = &lastPinTx
		logger.Warn("Number of pinning tx can not be bigger than current height")
	}
	if p.PinTo == nil {
		p.PinTo = new(int)
		p.PinTo = &lastPinTx
	}
	if *p.PinTo < 0 {
		p.PinTo = new(int)
		p.PinTo = &lastPinTx
		logger.Warn("Number of pinning tx must be positive")
	}
	if *p.PinTo > lastPinTx {
		p.PinTo = &lastPinTx
		logger.Warn("Number of pinning tx can not be bigger than current hight")
	}
	if *p.PinTo < *p.PinFrom {
		panic(errors.New("incorrect input, pinTo  is smaller than pinFrom "))
	}
	return api.GetLogsParams{
		Address:  p.Address,
		Topics:   p.Topics,
		Page:     p.Page,
		PageSize: p.PageSize,
		PinFrom:  p.PinFrom,
		PinTo:    p.PinTo,
	}
}

func (nas NodeApiServer) GetLogs(w http.ResponseWriter, r *http.Request, params api.GetLogsParams) {
	validParams := getLogsParamsExtended(params).validate()
	logs := vm.SearchLogs(vm.SearchLogRequest{Address: params.Address, Topics: validParams.Topics, Offset: *validParams.Page * *validParams.PageSize, Limit: *validParams.PageSize, PinFrom: *validParams.PinFrom, PinTo: *validParams.PinTo})
	mappedLogs := mapper.LogsToDto(logs)
	_ = json.NewEncoder(w).Encode(api.LogsResponse{Logs: &mappedLogs})
}

type ErrorRecovery struct {
	next http.Handler
}

// panicMessage - render a recovered panic value as a string. Not every panic in
// the handlers carries an error value (some panic with a plain string), and a
// bare type assertion here would panic again inside the recover, escaping this
// middleware and dropping the connection with no response at all.
func panicMessage(r interface{}) string {
	switch v := r.(type) {
	case error:
		return v.Error()
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (e *ErrorRecovery) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			apiErrorString := panicMessage(r)
			debug.PrintStack()
			logger.Infof("Error handling request %s - %s: %s", req.Method, req.URL.RequestURI(), apiErrorString)
			if strings.Contains(apiErrorString, "not found") {
				rw.WriteHeader(404)
			} else {
				rw.WriteHeader(400)
			}
			_ = json.NewEncoder(rw).Encode(api.ApiError{Error: &apiErrorString})
		} else {
			logger.Debugf("Handling request %s - %s OK", req.Method, req.URL.RequestURI())
		}
	}()
	e.next.ServeHTTP(rw, req)
}

func requireHexWithPrefixOfSize(s string, size int, name string) string {
	s = requireHex(s, name)
	decoded, _ := hex.DecodeString(s)
	if len(decoded) != size {
		panic(fmt.Errorf("%s must be of size %d, got %d", name, size, len(decoded)))
	}
	return s
}

func requireHexWithPrefixOfMinSize(s string, size int, name string) string {
	s = requireHex(s, name)
	decoded, _ := hex.DecodeString(s)
	if len(decoded) < size {
		panic(fmt.Errorf("%s must be at least of size %d, got %d", name, size, len(decoded)))
	}
	return s
}

func requireHex(s string, name string) string {
	if s == "" {
		panic(fmt.Errorf("%s must not be empty", name))
	}
	if !strings.HasPrefix(s, "0x") {
		panic(fmt.Errorf("%s must have a 0x prefix", name))
	}
	s = strings.TrimPrefix(s, "0x")
	_, err := hex.DecodeString(s)
	if err != nil {
		logger.Debugf("Invalid %s hex, value=%s, reason:%s", name, s, err.Error())
		panic(fmt.Errorf("%s is not a valid hex string", name))
	}
	return s
}

func parseHex(s string, name string) []byte {
	s = requireHex(s, name)
	decoded, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return decoded
}

func pad32(s string) string {
	if len(s) > 64 {
		panic(errors.New("input string is longer that 32 bytes"))
	}
	additionalZeros := 64 - len(s)
	for i := 0; i < additionalZeros; i++ {
		s = "0" + s
	}
	return s
}

func wrapBool(b bool) *bool {
	return &b
}

// Faucet impl
// TODO move to separate service (low-priority)
var received map[string]time.Time = map[string]time.Time{}
var faucetLock sync.Mutex

func Faucet(w http.ResponseWriter, r *http.Request) {
	if config.GetConfig().Dag.FaucetPrivatekey == "" || config.GetConfig().Dag.FaucetPublickey == "" || config.GetConfig().Dag.FaucetWallet == "" {
		panic(errors.New("Faucet Wallet/PublicKey/PrivateKey aren't configured. Faucet is disabled"))
	}
	faucetLock.Lock()
	defer faucetLock.Unlock()
	addresses, ok := r.URL.Query()["address"]

	if !ok || len(addresses) < 1 {
		w.WriteHeader(400)
		err := "Address not specified"
		json.NewEncoder(w).Encode(api.ApiError{Error: &err})
		return
	}

	address := addresses[0]
	address = requireHexWithPrefixOfSize(address, 20, "address")
	key, eligible := findKey(r)
	if !eligible {
		w.WriteHeader(400)
		err := fmt.Sprintf("You've already received coins this day, try again after %s", received[key].AddDate(0, 0, 1).String())
		json.NewEncoder(w).Encode(api.ApiError{Error: &err})
		return
	}
	privKey, _ := grape1crypto.ParsePrivateKey(config.GetConfig().Dag.FaucetPrivatekey)
	pubKey, _ := grape1crypto.ParsePublicKey(config.GetConfig().Dag.FaucetPublickey)
	amount, _ := big.NewInt(0).SetString("1000000000000000000000", 10)
	senderAcc := accService.GetAccountById(config.GetConfig().Dag.FaucetWallet)
	if senderAcc == nil {
		panic(errors.New("Faucet account hasn't been created yet"))
	}
	if senderAcc.Balance.Cmp(amount) < 0 {
		panic(errors.New("not enough money on faucet account, try again later"))
	}
	data := wallet.NewTransaction(&privKey, &pubKey, config.GetConfig().Dag.FaucetWallet, address, amount)
	t := &tx.Txv1{}
	t.GeneratePayment(data, uint8(config.GetConfig().Peer.Network))
	txBytes, err := proto.Marshal(t.MarshalBinary())
	if err != nil {
		panic(err)
	}
	_, err = txService.SendRawTransaction(hex.EncodeToString(txBytes))
	if err != nil {
		panic(err)
	}
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(mapper.TxToDto(tx.NewUnifiedUnconfirmedTx(tx.IdentifiableTx{Transaction: t})))
	received[key] = time.Now()
	hash, _ := t.Hash(crypto.SHA256)
	log.Infof("Sent %s coins to %s from faucet via tx 0x%s", amount.String(), key, hex.EncodeToString(hash))

}

func findKey(r *http.Request) (string, bool) {
	ipAndPort := strings.Split(r.RemoteAddr, ":")
	if len(ipAndPort) < 2 {
		panic(errors.New("IpAndPort from request.RemoteAddr aren't in valid format"))
	}
	ip := ipAndPort[0]
	key := ip
	logger.Debugf("Address + IP verification for getting money from faucet: %s", key)
	receivedTime, exist := received[key]
	if !exist || time.Since(receivedTime).Hours() > 24 {
		logger.Debugf("Address + IP (remote addr) is eligible for getting money from faucet: %s", key)
		return key, true
	}
	// inspect chained proxy
	proxiesStr := r.Header.Get("X-Forwarded-For")
	if proxiesStr == "" {
		logger.Debugf("No chained proxy specified. Account is not eligible for getting money from faucet: %s", key)
		return key, false
	}
	proxies := strings.Split(proxiesStr, ", ")
	initiator := proxies[0]
	key = initiator
	receivedTime, exist = received[key]
	if !exist || time.Since(receivedTime).Hours() > 24 {
		logger.Debugf("Address + IP (from chained proxy list) is eligible for getting money from faucet: %s", key)
		return key, true
	}
	return key, false
}

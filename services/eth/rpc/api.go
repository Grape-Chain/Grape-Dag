package rpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/dag"
	"github.com/Grape-Chain/Grape-Dag/services"
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/types"
	"github.com/Grape-Chain/Grape-Dag/vm"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/crypto/eth"
	"github.com/ethereum/go-ethereum/common"
	golog "github.com/ipfs/go-log/v2"
)

var logger golog.EventLogger

func init() {
	logger = golog.Logger("json-rpc")

}

type BlockResponse struct {
	Number           string                `json:"number"`
	Hash             string                `json:"hash"`
	ParentHash       string                `json:"parentHash"`
	Nonce            string                `json:"nonce"`
	Sha3Uncles       string                `json:"sha3Uncles"`
	LogsBloom        string                `json:"logsBloom"`
	TransactionsRoot string                `json:"transactionsRoot"`
	StateRoot        string                `json:"stateRoot"`
	ReceiptsRoot     string                `json:"receiptsRoot"`
	Miner            string                `json:"miner"`
	Difficulty       string                `json:"difficulty"`
	TotalDifficulty  string                `json:"totalDifficulty"`
	ExtraData        string                `json:"extraData"`
	Size             string                `json:"size"`
	GasLimit         string                `json:"gasLimit"`
	GasUsed          string                `json:"gasUsed"`
	Timestamp        string                `json:"timestamp"`
	Transactions     []TransactionResponse `json:"transactions"`
	Uncles           []common.Hash         `json:"uncles"`
}

type RPCRequest struct {
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	JsonRpc string        `json:"jsonrpc"`
}

type RPCResponse struct {
	Result  interface{} `json:"result"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      int         `json:"id"`
	JsonRpc string      `json:"jsonrpc"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type CallParams struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Gas      string `json:"gas"`
	GasPrice string `json:"gasPrice"`
	Value    string `json:"value"`
	Data     string `json:"data"`
}

type TxReceipt struct {
	TransactionHash   string  `json:"transactionHash"`
	TransactionIndex  string  `json:"transactionIndex"`
	BlockHash         string  `json:"blockHash"`
	BlockNumber       string  `json:"blockNumber"`
	From              string  `json:"from"`
	To                *string `json:"to"`
	CumulativeGasUsed string  `json:"cumulativeGasUsed"` // gas used at the moment of tx execution in block
	EffectiveGasPrice string  `json:"effectiveGasPrice"`
	GasUsed           string  `json:"gasUsed"`
	ContractAddress   *string `json:"contractAddress"` // address of the created contracts (or null if no any)
	Logs              []Log   `json:"logs"`
	LogsBloom         string  `json:"logsBloom"`
	Type              string  `json:"type"`   // 0 for legacy transactions, 1 - for access list, 2 - dynamic fees
	Status            string  `json:"status"` // 1 - success, 0 - failed
}

type TransactionResponse struct {
	TransactionHash  string  `json:"hash"`
	TransactionIndex *string `json:"transactionIndex"`
	BlockHash        *string `json:"blockHash"`
	BlockNumber      *string `json:"blockNumber"`
	From             string  `json:"from"`
	To               *string `json:"to"`
	Input            *string `json:"input"`
	Value            string  `json:"value"`
	Nonce            string  `json:"nonce"`
	Gas              string  `json:"gas"`
	GasPrice         string  `json:"gasPrice"`
	V                *string `json:"v"`
	R                *string `json:"r"`
	S                *string `json:"s"`
}

type Log struct {
	LogIndex         string   `json:"logIndex"`
	TransactionIndex string   `json:"transactionIndex"`
	TransactionHash  string   `json:"transactionHash"`
	BlockHash        string   `json:"blockHash"`
	BlockNumber      string   `json:"blockNumber"`
	Address          string   `json:"address"` // address from which this log was created
	Data             string   `json:"data"`
	Topics           []string `json:"topics"`
}

func RpcHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}

		var request RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if request.JsonRpc != "2.0" {
			http.Error(w, "Only JsonRpc 2.0 is supported", http.StatusBadRequest)
			return
		}

		var response RPCResponse
		switch request.Method {
		case "net_version":
			response = handleNetVersion(request.Params)
		case "eth_coinbase":
			response = handleEthCoinbase(request.Params)
		case "eth_chainId":
			response = handleEthChainID(request.Params)
		case "eth_gasPrice":
			response = handleEthGasPrice(request.Params)
		case "eth_blockNumber":
			response = handleEthBlockNumber(request.Params)
		case "eth_getBalance":
			response = handleEthGetBalance(request.Params)
		case "eth_getStorageAt":
			response = handleEthGetStorageAt(request.Params)
		case "eth_getCode":
			response = handleEthGetCode(request.Params)
		case "eth_sendRawTransaction":
			response = handleEthSendRawTransaction(request.Params)
		case "eth_call":
			response = handleEthCall(request.Params)
		case "eth_estimateGas":
			response = handleEthEstimateGas(request.Params)
		case "eth_getTransactionReceipt":
			response = handleEthGetTransactionReceipt(request.Params)
		case "eth_getBlockByNumber":
			response = handleEthGetBlockByNumber(request.Params)
		case "eth_getTransactionCount":
			response = handleEthGetTransactionCount(request.Params)

		case "eth_getTransactionByHash":
			response = handleEthGetTransactionByHash(request.Params)

		default:
			response.Error = &RPCError{
				Code:    http.StatusNotFound,
				Message: "Method not found",
			}
		}
		response.JsonRpc = "2.0"
		response.ID = request.ID
		respJson, _ := json.MarshalIndent(response, "", "  ")
		requestJson, _ := json.MarshalIndent(request, "", "  ")
		logger.Infof("Eth request: %s Eth response: %s", requestJson, respJson)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}

}

func handleNetVersion(params []interface{}) RPCResponse {
	net_id := config.GetConfig().Peer.Network

	return RPCResponse{Result: intToHex(net_id + 1)} // +1 because eth has reserved legacy txs for chainId=0
}

func handleEthCoinbase(params []interface{}) RPCResponse {
	result := config.GetConfig().Dag.Coinbaseaccount

	return RPCResponse{Result: result}
}

func handleEthChainID(params []interface{}) RPCResponse {
	chainId := config.GetConfig().Peer.Network
	return RPCResponse{Result: intToHex(chainId + 1)} // +1 because eth has reserved legacy txs for chainId=0

}

func handleEthGasPrice(params []interface{}) RPCResponse {
	return RPCResponse{Result: "0x1"}
}

func handleEthBlockNumber(params []interface{}) RPCResponse {
	height := dag.GetPin().CurrentHeight()
	return RPCResponse{Result: intToHex(height)}
}

func handleEthGetBalance(params []interface{}) RPCResponse {
	if len(params) < 1 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Address must be specified in params",
		}}
	}
	addressStr, ok := params[0].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid address supplied",
		}}
	}
	account := vm.SearchAccount(addressStr)
	balance := big.NewInt(0)
	if account != nil {
		balance = &account.Balance
		if !vm.IsSCAccount(addressStr) {
			byteAddress := grape1crypto.AddressToBytes(account.Id)
			cachedBalance, err := dag.GetPin().GetBalance(byteAddress)
			if err != nil {
				balance = big.NewInt(0)
			} else {
				balance = cachedBalance
			}
		}
	} else { // load from cache for non-sc accounts
		balance = big.NewInt(0)
	}

	return RPCResponse{Result: "0x" + balance.Text(16)}
}

func handleEthGetTransactionCount(params []interface{}) RPCResponse {
	if len(params) < 1 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Address must be specified in params",
		}}
	}
	addressStr, ok := params[0].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid address supplied",
		}}
	}
	account := vm.SearchAccount(addressStr)
	nonce := int64(0)
	if account != nil {
		nonce = account.Nonce.Int64()
	}

	return RPCResponse{Result: int64ToHex(nonce)}
}

func handleEthGetStorageAt(params []interface{}) RPCResponse {
	if len(params) < 2 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid parameters. At least 2 parameters (address, position) must be provided",
		}}
	}
	addressHex, ok := params[0].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid address",
		}}
	}

	positionHex, ok := params[1].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid positoin",
		}}
	}

	value := vm.GetStorageAt(addressHex, positionHex)

	return RPCResponse{Result: value}
}

func handleEthGetCode(params []interface{}) RPCResponse {
	if len(params) < 1 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Address must be specified in params",
		}}
	}

	addressStr, ok := params[0].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid address supplied",
		}}
	}
	code := vm.GetContractCode(addressStr)

	return RPCResponse{Result: "0x" + code}
}

func handleEthSendRawTransaction(params []interface{}) RPCResponse {
	if len(params) < 1 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Tx bytes must be specified in parameters in hex format",
		}}
	}

	txHexStr, ok := params[0].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid transaction bytes supplied",
		}}
	}

	res, err := services.NewTransactionService().SendRawTransaction(txHexStr[2:])
	if err != nil {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Failed to send tx: %s", err.Error()),
		}}
	}

	return RPCResponse{Result: res.Hash}
}

func handleEthCall(params []interface{}) RPCResponse {

	if len(params) < 1 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Call data isn't supplied",
		}}
	}

	p := CallParams{}
	callParamMap := params[0].(map[string]interface{})
	if callParamMap["data"] != nil {
		p.Data = callParamMap["data"].(string)
	}
	if callParamMap["from"] != nil {
		p.From = callParamMap["from"].(string)
	}
	if callParamMap["value"] != nil {
		p.Value = callParamMap["value"].(string)
	}
	if callParamMap["gas"] != nil {
		p.Gas = callParamMap["gas"].(string)
	}
	if callParamMap["gasPrice"] != nil {
		p.GasPrice = callParamMap["gasPrice"].(string)
	}

	p.To = callParamMap["to"].(string)

	methodId := p.Data[2:10]
	data := p.Data[10:]
	res, err := services.NewTransactionService().CallReadContract(p.To, methodId, data, p.From)
	if err != nil {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusInternalServerError,
			Message: fmt.Sprintf("Call wasn't successful: %s", err.Error()),
		}}
	}
	if res == "null" {
		return RPCResponse{Result: "0x"}
	}

	return RPCResponse{Result: res}
}

func handleEthEstimateGas(params []interface{}) RPCResponse {
	if len(params) < 1 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Call data isn't supplied",
		}}
	}

	p := CallParams{}
	callParamMap := params[0].(map[string]interface{})
	if callParamMap["data"] != nil {
		p.Data = callParamMap["data"].(string)
	}
	if callParamMap["from"] != nil {
		p.From = callParamMap["from"].(string)
	}
	if callParamMap["value"] != nil {
		p.Value = callParamMap["value"].(string)
	}
	if callParamMap["gas"] != nil {
		p.Gas = callParamMap["gas"].(string)
	}
	if callParamMap["gasPrice"] != nil {
		p.GasPrice = callParamMap["gasPrice"].(string)
	}

	if callParamMap["to"] != nil {
		p.To = callParamMap["to"].(string)
	}

	res, err := services.NewTransactionService().EstimateTxFuel(services.FuelEstimationRequest{Message: &services.Message{
		From:   grape1crypto.AddressToBytes(p.From),
		To:     grape1crypto.AddressToBytes(p.To),
		Data:   grape1crypto.HexToBytesNil(p.Data),
		Amount: grape1crypto.HexToBytesNil(p.Value),
	}})
	if err != nil {
		return RPCResponse{Result: "null", Error: &RPCError{
			Code:    32000,
			Message: "RPC: gas estimation failed (invalid tx?): " + err.Error(),
		}}
	}
	increasedGas := res.GasUsed * 11 / 10
	if increasedGas > int64(config.GetConfig().Tx.Maxfuellimit) {
		increasedGas = int64(config.GetConfig().Tx.Maxfuellimit)
	}
	return RPCResponse{Result: intToHex(int(increasedGas))}
}

func handleEthGetTransactionReceipt(params []interface{}) RPCResponse {
	if len(params) < 1 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "TxHash must be specified in parameters in hex format",
		}}
	}

	txHashStr, ok := params[0].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid transaction hash supplied",
		}}
	}
	confirmedTx, found := services.NewTransactionService().GetTransactionByHash(txHashStr)
	if !found {
		return RPCResponse{Result: nil}
	}
	var to *string
	if len(confirmedTx.GetRecipient()) != 0 {
		recipientAddress := grape1crypto.BytesToAddress(confirmedTx.GetRecipient())
		to = &recipientAddress
	}
	status := 1
	if confirmedTx.Status == tx.Failed {
		status = 0
	}
	var contractAddress *string
	if confirmedTx.GetTransactionType() == 1 {
		generatedEthAddress := eth.EthAddressFromCaller(eth.EthAddress(confirmedTx.GetSender()), int64(confirmedTx.GetNonce())).Hex()
		contractAddress = &generatedEthAddress
	}
	hashBytes := confirmedTx.GetHash()
	logs := vm.GetLogsForTx(hashBytes)
	result := TxReceipt{
		TransactionHash:   confirmedTx.GetHash().String(),
		TransactionIndex:  intToHex(confirmedTx.TxIndex),
		BlockHash:         confirmedTx.PinTxHash,
		BlockNumber:       intToHex(confirmedTx.PinTxNumber),
		From:              grape1crypto.BytesToAddress(confirmedTx.GetSender()),
		To:                to,
		CumulativeGasUsed: intToHex(confirmedTx.CumulativeGasUsed),
		EffectiveGasPrice: "0x" + big.NewInt(0).SetBytes(confirmedTx.GetFuelPrice().Bytes()).Text(16),
		GasUsed:           intToHex(confirmedTx.UsedFuel),
		ContractAddress:   contractAddress,
		Status:            intToHex(status),
		Type:              intToHex(0),
		LogsBloom:         grape1crypto.ZeroHex(32),
		Logs:              mapLogs(logs, *confirmedTx),
	}

	return RPCResponse{Result: result}
}

func handleEthGetTransactionByHash(params []interface{}) RPCResponse {
	if len(params) < 1 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "TxHash must be specified in parameters in hex format",
		}}
	}

	txHashStr, ok := params[0].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid transaction hash supplied",
		}}
	}
	foundTx, found := services.NewTransactionService().GetAnyTransactionByHash(txHashStr)
	if !found {
		return RPCResponse{Result: nil}
	}
	result := mapTx(foundTx)
	return RPCResponse{Result: result}
}

func mapTx(trx *tx.UnifiedTx) TransactionResponse {
	var to, idx, blockHash, blockNumber, r, s, v, input *string
	if len(trx.GetRawTx().GetRecipient()) != 0 {
		recipientAddress := trx.GetRawTx().GetRecipient().String()
		to = &recipientAddress
	}
	hashBytes := trx.GetRawTx().GetHash()
	if trx.IsConfirmed() {
		confirmed := trx.GetConfirmed()
		idx = intToHexPointer(confirmed.TxIndex)
		blockHash = &confirmed.PinTxHash
		blockNumber = int64ToHexPointer(int64(confirmed.PinTxNumber))
	}
	if trx.GetRawTx().GetType() == 1 { // extract R, S, V for eth tx
		sig := trx.GetRawTx().GetSignature()
		R, S, V := tx.DecodeSignature(sig, int(trx.GetRawTx().GetChainType()))
		r = stringToPointer(R.String())
		s = stringToPointer(S.String())
		v = stringToPointer(V.String())

	} else {
		// mock signature
		r = stringToPointer(grape1crypto.RandomHex(32))
		s = stringToPointer(grape1crypto.RandomHex(32))
		v = stringToPointer("0x26")
	}
	if len(trx.GetRawTx().GetData()) != 0 {
		input = stringToPointer(trx.GetRawTx().GetData().String())
	} else {
		input = stringToPointer("0x0")
	}

	result := TransactionResponse{
		TransactionHash:  hashBytes.String(),
		TransactionIndex: idx,
		BlockHash:        blockHash,
		BlockNumber:      blockNumber,
		From:             grape1crypto.BytesToAddress(trx.GetRawTx().GetSender()),
		To:               to,
		Input:            input,
		Value:            bigIntToHex(trx.GetRawTx().GetAmount()),
		Nonce:            int64ToHex(int64(trx.GetRawTx().GetNonce())),
		Gas:              bigIntToHex(trx.GetRawTx().GetFuelLimit()),
		GasPrice:         bigIntToHex(trx.GetRawTx().GetFuelPrice()),
		V:                v,
		R:                r,
		S:                s,
	}
	return result
}

func handleEthGetBlockByNumber(params []interface{}) RPCResponse {
	if len(params) < 2 {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Block number and load tx flag must be specified in parameters",
		}}
	}

	blockNumStr, ok := params[0].(string)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid block number supplied",
		}}
	}

	loadTxs, ok := params[1].(bool)
	if !ok {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusBadRequest,
			Message: "Invalid flag to fetch txs supplied",
		}}
	}
	var pin *pb.TxPin
	if blockNumStr == "latest" {
		pin = dag.GetPin().GetLastPin()
	} else {
		err, blockNum := hexToInt(blockNumStr)
		if err != nil {
			return RPCResponse{Error: &RPCError{
				Code:    http.StatusBadRequest,
				Message: "Invalid block number: 'latest' or valid hex required ",
			}}
		}
		pin = dag.GetPin().GetPin(int(blockNum))
	}
	if pin == nil {
		return RPCResponse{Error: &RPCError{
			Code:    http.StatusNotFound,
			Message: fmt.Sprintf("Block: %s doesn't exist", blockNumStr),
		}}
	}

	prevPin := dag.GetPin().GetPin(int(pin.PinNumber - 1))
	var prevHash types.Hex
	if prevPin != nil {
		prevHash = prevPin.GetHash()
	}

	result := BlockResponse{
		Number:           intToHex(int(pin.PinNumber)),
		Hash:             pin.GetHash().String(),
		ParentHash:       prevHash.String(),
		Nonce:            "0x2867e9f4173be91e", // real but magic numbers
		Sha3Uncles:       "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
		LogsBloom:        "0xa5b9d60f32436310afebcfda832817a68921beb782fabf7915cc0460b443116a",
		TransactionsRoot: "0xd6054e3cec7d20477a3ff91d438b2806af40ecda39303612035589e86e069f46",
		StateRoot:        "0xd6054e3cec7d20477a3ff91d438b2806af40ecda39303612035589e86e069f46",
		Miner:            config.GetConfig().Dag.Coinbaseaccount,
		Difficulty:       "0x800667E2F7342",
		TotalDifficulty:  "0x2FF1CEC0D6B079CD42B",
		ExtraData:        "0x0",
		Size:             intToHex(len(pin.Sites) + len(pin.SmcTxs)),
		GasLimit:         "0x" + big.NewInt(int64(config.GetConfig().Tx.Maxfuellimit)).Text(16),
		GasUsed:          "0x" + big.NewInt(int64(config.GetConfig().Tx.Maxfuellimit)).Text(16),
		Timestamp:        int64ToHex(pin.Ts.AsTime().Unix()),
		Transactions:     []TransactionResponse{},
		Uncles:           []common.Hash{},
	}
	if loadTxs {
		for idx, payment := range pin.Nodes {
			pinHash := pin.GetHash()
			confirmedTx, _ := tx.NewPaymentConfirmedTx(tx.UnmarshalBinary(payment.Tx), int(pin.PinNumber), pinHash.String(), idx)
			result.Transactions = append(result.Transactions, mapTx(confirmedTx.ToUnified()))
		}
		gas := 0
		for idx, smcTx := range pin.SmcTxs {
			var status tx.Status = tx.Successful
			if smcTx.Receipt.Status == 1 {
				status = tx.Failed
			}
			pinHash := pin.GetHash()
			confirmedTx := tx.NewUnifiedConfirmedTx(tx.ConfirmedTx{IdentifiableTx: tx.IdentifiableTx{Transaction: tx.UnmarshalBinary(smcTx.Tx)},
				UsedFuel:          int(smcTx.Receipt.FuelUsed),
				TxIndex:           idx + len(pin.Nodes),
				PinTxNumber:       int(pin.GetPinNumber()),
				PinTxHash:         pinHash.String(),
				StatusMessage:     smcTx.Receipt.StatusMessage,
				Status:            status,
				CumulativeGasUsed: gas,
			})

			result.Transactions = append(result.Transactions, mapTx(confirmedTx))
			gas = gas + int(smcTx.Receipt.GetFuelUsed())
		}
	}
	return RPCResponse{Result: result}
}

func mapLogs(logs []vm.Log, confirmedTx tx.ConfirmedTx) []Log {
	res := []Log{}
	for idx, l := range logs {
		res = append(res,
			Log{
				LogIndex:         intToHex(idx),
				TransactionIndex: intToHex(confirmedTx.TxIndex),
				TransactionHash:  confirmedTx.GetHash().String(),
				BlockHash:        confirmedTx.PinTxHash,
				BlockNumber:      intToHex(confirmedTx.PinTxNumber),
				Address:          grape1crypto.BytesToAddress(l.ContractAddress),
				Data:             "0x" + hex.EncodeToString(l.Data),
				Topics:           mapTopic(l.Topics),
			})
	}
	return res
}

func mapTopic(topics [][]byte) []string {
	res := []string{}
	for _, topic := range topics {
		res = append(res, "0x"+hex.EncodeToString(topic))
	}
	return res
}
func bigIntToHex(i *big.Int) string {
	if i.Cmp(big.NewInt(0)) == 0 {
		return "0x0"
	}
	return "0x" + i.Text(16)
}

func intToHex(i int) string {
	if i == 0 {
		return "0x0"
	}
	return "0x" + fmt.Sprintf("%X", i)
}
func intToHexPointer(i int) *string {
	res := intToHex(i)
	return &res
}

func stringToPointer(s string) *string {
	return &s
}

func int64ToHex(i int64) string {
	if i == 0 {
		return "0x0"
	}
	return "0x" + fmt.Sprintf("%X", i)
}

func int64ToHexPointer(i int64) *string {
	res := int64ToHex(i)
	return &res
}
func hexToInt(h string) (error, int) {
	b := grape1crypto.HexToBytes(h)

	return nil, int(big.NewInt(0).SetBytes(b).Int64())

}

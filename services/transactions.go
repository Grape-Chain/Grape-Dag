package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/VG-Grape/luna/config"
	"github.com/VG-Grape/luna/dag"
	txqueue "github.com/VG-Grape/luna/queues"
	"github.com/VG-Grape/luna/smc"
	"github.com/VG-Grape/luna/tx"
	"github.com/VG-Grape/luna/tx/pb"
	"github.com/VG-Grape/luna/types"
	"github.com/VG-Grape/luna/utils"
	"github.com/VG-Grape/luna/vm"
	"github.com/VG-Grape/luna/crypto"
	"github.com/VG-Grape/luna/crypto/eth"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/protobuf/proto"
)

type TransactionService interface {
	SendRawTransaction(pbTx string) (ExecutionResult, error)

	EstimateTxFuel(req FuelEstimationRequest) (ExecutionResult, error)

	CallReadContract(contractAddress string, contractMethod string, methodParameters string, sender string) (string, error)

	GetTransactionByHash(hash string) (*tx.ConfirmedTx, bool)

	GetAnyTransactionByHash(hash string) (*tx.UnifiedTx, bool)

	GetTransactions(accounts []string, txType int, s Sort, p Page, confirmed *bool, direction *bool) []*tx.UnifiedTx
}

type TransactionServiceImpl struct {
}

func NewTransactionService() TransactionService {
	impl := TransactionServiceImpl{}

	return &impl
}

func (ts *TransactionServiceImpl) CallReadContract(contractAddress string, contractMethod string, methodParameters string, sender string) (string, error) {
	utils.ColorizeInfo(logger, "Execute read call on contract %s, method=%s, parameters=%s, sender=%s", contractAddress, contractMethod, methodParameters, sender)
	client, err := vm.ConnectToVm()
	if err != nil {
		logger.Errorf("Unable to connect to VM: %s", err.Error())
		return "", err
	}
	if contractAddress == "" {
		panic(fmt.Errorf("Contract adress is nil"))
	}
	if contractMethod == "" {
		panic(fmt.Errorf("Contract method is nil"))
	}
	coinbaseAddrBytes, err := eth.ParseEthAddress(config.GetConfig().Dag.Coinbaseaccount)
	if err != nil {
		panic(err)
	}
	header := &pb.PinTxHeader{CoinbaseAccountAddress: &pb.Address{AddBytes: coinbaseAddrBytes},
		Timestamp: dag.GetPin().CurrentTS(), TxNumber: int32(dag.GetPin().CurrentHeight())}
	decodedAddress := luna1crypto.AddressToBytes(contractAddress)
	response, vmErr := client.RunCode(context.TODO(), &pb.ReadContractRequest{Header: header, ContractAddress: &pb.Address{AddBytes: decodedAddress}, MethodSignature: contractMethod, MethodParameters: methodParameters, Sender: sender})
	if vmErr != nil {
		logger.Errorf("VM error occurred during calling method %s on contract %s with parameters = %s and sender = %s: %s", contractMethod, contractAddress, methodParameters, sender, vmErr.Error())
		return "", vmErr
	}
	if response.Error != "" && response.Error != "0x" {
		errStr := fmt.Sprintf("Error read method execution: %s", response.Error)
		logger.Error(errStr)
		return "", errors.New(errStr)
	} else {
		logger.Infof("Executed successfully with result %s: read call on contract %s, method=%s, parameters=%s, sender=%s", response.Msg, contractAddress, contractMethod, methodParameters, sender)
	}
	return response.Msg, nil
}

func parseVmError(err string) error {
	return fmt.Errorf("system VM error during tx execution: %s", err)
}

func (ts *TransactionServiceImpl) SendRawTransaction(pbTx string) (ExecutionResult, error) {
	startProcessingTime := time.Now()
	utils.ColorizeInfo(logger, "Process transaction %s", shortenString(pbTx, 16))
	defer func() {
		utils.ColorizeInfo(logger, "Tx %s processing finished in %s", shortenString(pbTx, 16), time.Since(startProcessingTime))
	}()
	if pbTx == "" {
		panic(fmt.Errorf("Tx is empty"))
	}

	txBytes, err := hex.DecodeString(pbTx)
	execResult := ExecutionResult{}
	if err != nil {
		return execResult, err
	}
	transaction, err := tx.ParseTransaction(txBytes)
	if err != nil {
		return execResult, err
	}

	errSig := transaction.VerifySignature()
	network := config.GetConfig().Peer.Network
	if int(transaction.GetChainType()) != network {
		err := fmt.Errorf(" Tx %s came from a different network. We are on: %d, got %d", transaction.String(), network, int(transaction.GetChainType()))
		return execResult, err
	}
	hash := transaction.GetHash()
	if errSig != nil {
		logger.Errorf("Bad tx signature for: hash %s, account %s", hex.EncodeToString(hash), transaction.GetSender().String())
		return execResult, errSig
	}
	var txExecErr error
	if transaction.GetTransactionType() == tx.CALL_CONTRACT || transaction.GetTransactionType() == tx.PUBLISH_CONTRACT {
		fuelPrice := transaction.GetFuelPrice()
		if fuelPrice.Cmp(big.NewInt(0)) <= 0 {
			err := fmt.Errorf("fuel price must be greater than zero but got %s", fuelPrice.String())
			return execResult, err
		}
		maxAllowedFuelLimit := big.NewInt(int64(config.GetConfig().Tx.Maxfuellimit))
		if transaction.GetFuelLimit().CmpAbs(maxAllowedFuelLimit) > 0 {
			return execResult, fmt.Errorf("max allowed fuel limit for smart contract transaction is %s, got %s",
				maxAllowedFuelLimit.String(), transaction.GetFuelLimit().String())
		}
		maxAllowedPrice := big.NewInt(int64(config.GetConfig().Tx.Maxfuelprice))
		maxComission := new(big.Int).Mul(maxAllowedFuelLimit, maxAllowedPrice)
		price := new(big.Int).Mul(fuelPrice, transaction.GetFuelLimit())
		if price.Cmp(maxComission) > 0 {
			return execResult, fmt.Errorf("price  is %s, but must be less than max comission %s", price.String(), maxComission.String())
		}
		senderId := transaction.GetSender().String()
		sender := vm.SearchAccount(senderId)
		if sender == nil {
			return execResult, errors.New("sender's account doesn't exist")
		}
		txTotal := new(big.Int).Add(transaction.GetAmount(), new(big.Int).Mul(transaction.GetFuelLimit(), transaction.GetFuelPrice()))
		if sender.Balance.Cmp(txTotal) < 0 {
			return execResult, fmt.Errorf("not enough funds: required %d, got %s", txTotal, sender.Balance.Text(10))
		}

		smc.AddUnconfirmed(transaction)
		execResult.GasUsed = int64(transaction.GetFuelLimit().Uint64())
		execResult.Output = "0x"
		execResult.Successful = true
	} else {
		sender := transaction.GetSender()
		senderBalance, err := dag.GetPin().GetBalance(sender)
		if err != nil {
			return execResult, err
		}
		txTotal := new(big.Int).Add(transaction.GetAmount(), new(big.Int).Mul(transaction.GetFuelLimit(), transaction.GetFuelPrice()))
		if senderBalance.Cmp(txTotal) < 0 {
			return execResult, fmt.Errorf("not enough funds: required %d, got %s", txTotal, senderBalance.Text(10))
		}
		if transaction.GetFuelLimit().String() != "0" || transaction.GetFuelPrice().String() != "0" {
			return execResult, fmt.Errorf("payment transaction must have zero fuelLimit and fuelPrice, got %s and %s correspondingly", transaction.GetFuelLimit().String(), transaction.GetFuelPrice().String())
		}
		execResult, txExecErr = executePaymentTx(transaction)
	}
	if txExecErr != nil {
		return execResult, txExecErr
	}
	execResult.Hash = "0x" + hex.EncodeToString(hash)
	return execResult, nil
}

func executePaymentTx(transaction tx.Transaction) (ExecutionResult, error) {
	hash := transaction.GetHash()
	execResult := ExecutionResult{}
	txqueue.GetPublishQueue().Enqueue(transaction)
	// set zero fee here
	execResult.GasUsed = 0
	execResult.Successful = true
	logger.Infof("Added tx %s to publish queue", hash.String())
	return execResult, nil
}

func (ts *TransactionServiceImpl) EstimateTxFuel(r FuelEstimationRequest) (ExecutionResult, error) {
	r.validate()
	execResult := ExecutionResult{}
	grpcReq := &pb.EstimateGasRequest{}
	coinbaseAddrBytes, err := eth.ParseEthAddress(config.GetConfig().Dag.Coinbaseaccount)
	if err != nil {
		panic(err)
	}
	header := &pb.PinTxHeader{CoinbaseAccountAddress: &pb.Address{AddBytes: coinbaseAddrBytes},
		Timestamp: dag.GetPin().CurrentTS(), TxNumber: int32(dag.GetPin().CurrentHeight())}
	grpcReq.Header = header
	if r.isMessageBased() {
		grpcReq.TxOrMessage = &pb.EstimateGasRequest_Message{Message: r.Message.toPb()}
	} else {
		pbTxo := pb.Txv1{}

		pbErr := proto.Unmarshal(r.RawTx, &pbTxo)
		if pbErr != nil {
			return execResult, pbErr
		}
		grpcReq.TxOrMessage = &pb.EstimateGasRequest_Tx{Tx: &pbTxo}
	}

	return ts.doEstimate(grpcReq)
}

func (*TransactionServiceImpl) doEstimate(grpcReq *pb.EstimateGasRequest) (ExecutionResult, error) {
	execResult := ExecutionResult{}
	client, err := vm.ConnectToVm()
	if err != nil {
		logger.Errorf("Unable to connect to VM: %s", err.Error())
		return execResult, err
	}
	response, vmErr := client.EstimateGas(context.TODO(), grpcReq)
	if vmErr != nil {
		logger.Errorf("VM error occurred during running estimation=%v: %s", grpcReq, vmErr.Error())
		return execResult, vmErr
	}
	execResult, err = analyzeCallResponse(response)
	if err != nil {
		return execResult, err
	}
	logger.Infof("%s estimated successfully, status = %d, output=%s, err=%s", grpcReq, response.Status, response.Msg, response.Error)
	return execResult, nil
}

func analyzeCallResponse(response *pb.CallResponse) (ExecutionResult, error) {
	execResult := ExecutionResult{}
	if response.Status == -2 { // system error occurred
		return execResult, parseVmError(response.Error) // bad tx, abandon
	} else {
		gasUsed, err := strconv.ParseInt(response.GasUsed, 10, 64)
		if err != nil {
			return execResult, err
		}
		if response.Status == 0 {
			execResult.Output = response.Msg
			execResult.Successful = true
			execResult.GasUsed = gasUsed
		} else { // Status > 0 || Status == -1 // solidity error or general VM error occurred (out of gas, bad instruction, etc), keep tx
			execResult.Successful = false
			execResult.Output = response.Error + ":" + response.Msg
			execResult.GasUsed = gasUsed
		}
	}
	return execResult, nil
}

func (ts *TransactionServiceImpl) GetTransactionByHash(hash string) (*tx.ConfirmedTx, bool) {
	h, err := types.DecodeHexString(hash)
	if err != nil {
		return nil, false
	}
	found, err := dag.SearchTx(h)
	if err != nil {
		logger.Warnf("Tx by hash %s was not found %s", hash, err.Error())
		return nil, false
	} else {
		return found, true
	}
}

func (ts *TransactionServiceImpl) GetAnyTransactionByHash(hash string) (*tx.UnifiedTx, bool) {
	h, err := types.DecodeHexString(hash)
	if err != nil {
		return nil, false
	}
	found, err := dag.SearchAnyTx(h)
	if err != nil {
		logger.Warnf("Tx by hash %s was not found %s", hash, err.Error())
		return nil, false
	} else {
		return found, true
	}
}

func (ts *TransactionServiceImpl) GetTransactions(accounts []string, txType int, s Sort, p Page, confirmed *bool, direction *bool) []*tx.UnifiedTx {

	return dag.SearchTxs(accounts, txType, p.Size, p.Offset(), s.isAsc(), confirmed, direction)
}

func shortenString(s string, size int) string {
	if size < 2 {
		return ""
	}
	if size >= len(s) {
		return s
	}
	return s[:size/2] + "..." + s[len(s)-size/2:]
}

func emptyHex(hex string) bool {
	return hex == "" || hex == "0x" || len(hex) <= 2
}

type ExecutionResult struct {
	Hash       string
	Successful bool
	Output     string
	GasUsed    int64
}

type Message struct {
	From                  []byte
	To                    []byte
	Data                  []byte
	Amount                []byte
	CoinbaseAccountAdress []byte
	Timestamp             timestamp.Timestamp
	FixingTx              []byte
}

func (m *Message) toPb() *pb.CallMessage {
	pbMessage := pb.CallMessage{}
	if m.From != nil {
		pbMessage.From = &pb.Address{AddBytes: m.From}
	}
	if m.To != nil {
		pbMessage.To = &pb.Address{AddBytes: m.To}
	}
	pbMessage.Amount = m.Amount
	pbMessage.Data = m.Data
	if m.CoinbaseAccountAdress != nil {
		pbMessage.CoinbaseAccountAddress = &pb.Address{AddBytes: m.CoinbaseAccountAdress}
	}
	currentTime := time.Now().Unix()
	pbMessage.Timestamp = &timestamp.Timestamp{
		Seconds: currentTime,
	}
	pbMessage.FixingTx = m.FixingTx
	return &pbMessage
}

type FuelEstimationRequest struct {
	RawTx   []byte
	Message *Message
}

func (r FuelEstimationRequest) isMessageBased() bool {
	return r.Message != nil
}

func (r FuelEstimationRequest) validate() {
	if r.RawTx != nil && r.Message != nil {
		panic(errors.New("invalid FuelEstimationRequest, either rawTx or message can be present simultaneously"))
	}
}

func (r FuelEstimationRequest) String() string {
	if r.isMessageBased() {
		return "Estimate: from=0x" + hex.EncodeToString(r.Message.From) + ", to=0x" + hex.EncodeToString(r.Message.To) + ", amount=" + big.NewInt(0).SetBytes(r.Message.Amount).String() + ", data=0x" + hex.EncodeToString(r.Message.Data) + ", coinbaseAccountAddress=0x" + hex.EncodeToString(r.Message.CoinbaseAccountAdress) + ", timestamp=" + r.Message.Timestamp.String()
	}
	pbTx := pb.Txv1{}
	proto.Unmarshal(r.RawTx, &pbTx)
	tx := tx.Txv1{}
	tx.UnmarshalBinary(&pbTx)
	return tx.String()
}

// SPDX-License-Identifier: Apache-2.0

package tx

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"math/big"
	"math/rand"
	"reflect"
	"strings"
	"time"

	"github.com/Grape-Chain/Grape-Dag/crypto"
	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	pb "github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/types"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/Grape-Chain/Grape-Dag/wallet"
	eth "github.com/ethereum/go-ethereum/core/types"
	ethCrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
	proto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type VersionType uint8

const (
	TXV0 VersionType = iota
	TVX1
	TXV2
)

type TransactionType uint8

const (
	PAYMENT TransactionType = iota
	PUBLISH_CONTRACT
	CALL_CONTRACT
	SERVICE_GENESIS
	SERVICE
	SERVICE_PIN
)

func (t TransactionType) Name() string {
	return []string{"PAYMENT", "PUBLISH_CONTRACT", "CALL_CONTRACT", "SERVICE_GENESIS", "SERVICE", "SERVICE_PIN"}[t]
}

type ChainType uint8

const (
	MAINNET ChainType = iota
	PUBLIC_TESTNET
	PRIVATE_TESTNET
)

func ParseTransaction(txBytes []byte) (Transaction, error) {

	ethTx := new(eth.Transaction)
	err := rlp.DecodeBytes(txBytes, ethTx)
	if err == nil {
		return &EthTx{Transaction: *ethTx}, nil
	} else {
		logger.Infof("Attempt to unmarshal eth rlp tx failed: %s", err.Error())
		pbTxo := pb.Txv1{}
		err := proto.Unmarshal(txBytes, &pbTxo)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal grape raw transaction: %s", err.Error())
		}
		transaction := Txv1{}
		transaction.UnmarshalBinary(&pbTxo)
		return &transaction, nil
	}
}

func UnmarshalBinary(txPb *pb.Txv1) Transaction {
	if txPb.RlpEthTx != nil {
		ethTx := &EthTx{}
		ethTx.UnmarshalBinary(txPb)
		return ethTx
	} else {
		transaction := Txv1{}
		transaction.UnmarshalBinary(txPb)
		return &transaction
	}
}

type Transaction interface {
	GetTransactionType() TransactionType

	GetVersion() uint8

	GetChainType() ChainType

	GetAmount() *big.Int

	GetFuelLimit() *big.Int

	GetFuelPrice() *big.Int

	GetSenderPubKey() types.Hex

	GetSender() types.Hex

	GetRecipient() types.Hex

	GetNonce() uint64

	GetTimestamp() uint64

	GetData() types.Hex

	GetSignature() types.Hex

	VerifySignature() error

	GetHash() types.Hex

	MarshalBinary() *pb.Txv1

	UnmarshalBinary(*pb.Txv1)

	String() string

	IsService() bool

	IsPayload(payload string) bool

	GetType() int
}

type EthTx struct {
	eth.Transaction
}

func (tx *EthTx) GetTransactionType() TransactionType {
	if tx.To() == nil {
		return PUBLISH_CONTRACT
	}
	if len(tx.Data()) > 0 {
		return CALL_CONTRACT
	}
	return PAYMENT
}

func (tx *EthTx) GetVersion() uint8 {
	return 1
}
func (tx *EthTx) GetChainType() ChainType {
	chainType := tx.ChainId()
	if chainType.Cmp(big.NewInt(1)) == 0 {
		return MAINNET
	}
	if chainType.Cmp(big.NewInt(2)) == 0 {
		return PUBLIC_TESTNET
	}
	if chainType.Cmp(big.NewInt(3)) == 0 {
		return PRIVATE_TESTNET
	}
	panic(fmt.Errorf("unknown eth tx chain type %s", chainType.String()))
}

func (tx *EthTx) GetSenderPubKey() types.Hex {
	sig := tx.GetSignature()
	signer := eth.NewEIP2930Signer(tx.ChainId())
	msgHash := signer.Hash(&tx.Transaction)

	recoveredPubKey, err := ethCrypto.Ecrecover(msgHash.Bytes(), sig)
	if err != nil {
		panic(fmt.Errorf("failed to recover public key: %s", err.Error()))
	}
	return recoveredPubKey
}

func (tx *EthTx) GetSender() types.Hex {
	signer := eth.NewEIP155Signer(tx.ChainId())

	sender, err := eth.Sender(signer, &tx.Transaction)
	if err != nil {
		panic(err)
	}
	return sender.Bytes()
}

func (tx *EthTx) GetRecipient() types.Hex {
	if tx.To() == nil {
		return []byte{}
	} else {
		return tx.To().Bytes()
	}
}

func (tx *EthTx) GetNonce() uint64 {
	return tx.Nonce()
}

func (tx *EthTx) GetTimestamp() uint64 {
	// no such field
	return uint64(time.Now().UnixMilli())
}
func (tx *EthTx) GetData() types.Hex {
	return tx.Data()
}
func (tx *EthTx) GetSignature() types.Hex {
	return encodeSignature(tx)
}

func encodeSignature(tx *EthTx) types.Hex {
	V, R, S := tx.RawSignatureValues()

	v := byte(V.Uint64() - 27)
	if tx.ChainId().Cmp(big.NewInt(0)) != 0 {
		v = byte(V.Uint64()) - 35
		v = v - 2*byte(tx.ChainId().Int64())
	}

	// encode the signature in uncompressed format
	r, s := R.Bytes(), S.Bytes()
	sig := make([]byte, 65)
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):64], s)
	sig[64] = v

	return sig
}

func DecodeSignature(sig types.Hex, chainId int) (types.Hex, types.Hex, types.Hex) {
	r := sig[:32]
	for i, b := range r {
		if b == 0 {
			continue
		} else {
			r = r[i+1:]
			break
		}
	}

	s := sig[32:64]
	for i, b := range s {
		if b == 0 {
			continue
		} else {
			s = s[i+1:]
			break
		}
	}
	v := byte(35 + (chainId+1)*2)

	return r, s, []byte{v}
}

func (tx *EthTx) VerifySignature() error {
	sig := tx.GetSignature()
	signer := eth.NewEIP2930Signer(tx.ChainId())
	msgHash := signer.Hash(&tx.Transaction)

	// Recover the public key from the signature
	pubKey, err := ethCrypto.Ecrecover(msgHash.Bytes(), sig)
	if err != nil {
		return fmt.Errorf("failed to recover public key: %s", err.Error())
	}
	if !ethCrypto.VerifySignature(pubKey, msgHash[:], sig[:64]) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func (tx *EthTx) GetHash() types.Hex {
	return tx.Hash().Bytes()
}

func (tx *EthTx) GetAmount() *big.Int {
	return tx.Value()
}

func (tx *EthTx) GetFuelLimit() *big.Int {
	return big.NewInt(int64(tx.Gas()))
}

func (tx *EthTx) GetFuelPrice() *big.Int {
	return tx.GasPrice()
}

func (tx *EthTx) MarshalBinary() *pb.Txv1 {
	tpb := &pb.Txv1{}
	tpb.TxType = pb.TransactionType(tx.GetTransactionType())
	// tpb.Depth = t.Depth
	tpb.ChainType = pb.ChainType(tx.GetChainType())
	tpb.SenderPubk = tx.GetSenderPubKey()
	tpb.Sender = tx.GetSender()
	tpb.Recepient = tx.GetRecipient()
	tpb.Amount = tx.GetAmount().Bytes()
	tpb.Nonce = tx.GetNonce()
	tpb.Timestamp = timestamppb.New(time.UnixMilli(int64(tx.GetTimestamp())))
	tpb.FuelLimit = tx.GetFuelLimit().Bytes()
	tpb.FuelPrice = tx.GetFuelPrice().Bytes()
	tpb.Data = tx.GetData()
	tpb.Signature = tx.GetSignature()
	bytes, err := rlp.EncodeToBytes(tx)
	if err != nil {
		panic(err)
	}
	tpb.RlpEthTx = bytes
	return tpb
}

func (tx *EthTx) UnmarshalBinary(txPb *pb.Txv1) {
	if txPb.RlpEthTx != nil {
		err := rlp.DecodeBytes(txPb.RlpEthTx, tx)
		if err != nil {
			panic(err)
		}
	} else {
		panic("unmarshalling eth tx without rlp data")
	}
}

func (tx *EthTx) String() string {
	type JSONTransaction struct {
		Nonce    uint64    `json:"nonce"`
		GasPrice *big.Int  `json:"gasPrice"`
		GasLimit uint64    `json:"gasLimit"`
		To       types.Hex `json:"to"`
		Value    *big.Int  `json:"value"`
		Data     []byte    `json:"data"`
		V        *big.Int  `json:"v"`
		R        *big.Int  `json:"r"`
		S        *big.Int  `json:"s"`
		Hash     types.Hex `json:"hash"`
	}

	var to types.Hex
	if tx.To() != nil {
		to = tx.To().Bytes()
	}
	v, r, s := tx.RawSignatureValues()
	jsonTx := JSONTransaction{
		Nonce:    tx.Nonce(),
		GasPrice: tx.GasPrice(),
		GasLimit: tx.Gas(),
		To:       to,
		Value:    tx.Value(),
		Data:     tx.Data(),
		V:        v,
		R:        r,
		S:        s,
		Hash:     tx.GetHash(),
	}

	jsonData, err := json.MarshalIndent(jsonTx, "", "    ")
	if err != nil {
		logger.Fatal("Failed to marshal to JSON:", err)
	}

	return string(jsonData)
}

func (tx *EthTx) IsService() bool {
	return false
}

func (tx *EthTx) IsPayload(payload string) bool {

	if len(tx.GetData()) > 0 {
		return strings.Compare(string(tx.GetData()), payload) == 0
	}
	return false

}

func (tx *EthTx) GetType() int {
	return 1
}

type Txv1 struct {
	Tx_Type    TransactionType
	Chain_Type ChainType
	//	Depth       uint64 // used mostly by pinning transaction for synchronization
	Sender_Pubk []byte // sender's public key: ed25519 32 bytes, compressed
	Sender      []byte // sender's wallet address, 20 bytes, without prefix
	Recepient   []byte // recipient’s address, 20 bytes without prefix
	Amount      []byte
	Nonce       uint64
	Timestamp   time.Time
	Fuel_Limit  []byte
	Fuel_Price  []byte
	Data        []byte // optional for payments; mandatory for contracts
	Signature   []byte // Signature of the serialized transaction bytes by user’s private key
}

func (tx *Txv1) GetTransactionType() TransactionType {
	return tx.Tx_Type
}

func (tx *Txv1) GetVersion() uint8 {
	return 1
}
func (tx *Txv1) GetChainType() ChainType {
	return tx.Chain_Type
}

func (tx *Txv1) GetSenderPubKey() types.Hex {
	return tx.Sender_Pubk
}

func (tx *Txv1) GetSender() types.Hex {
	return tx.Sender
}
func (tx *Txv1) GetRecipient() types.Hex {
	return tx.Recepient
}
func (tx *Txv1) GetNonce() uint64 {
	return tx.Nonce
}
func (tx *Txv1) GetTimestamp() uint64 {
	return uint64(tx.Timestamp.UnixMilli())
}
func (tx *Txv1) GetData() types.Hex {
	return tx.Data
}
func (tx *Txv1) GetSignature() types.Hex {
	return tx.Signature
}
func (tx *Txv1) VerifySignature() error {
	return tx.Verify()
}
func (tx *Txv1) GetHash() types.Hex {
	hashBytes, err := tx.Hash(crypto.SHA256)
	if err != nil {
		panic(err)
	}
	return hashBytes
}

func (tx *Txv1) GetAmount() *big.Int {
	if tx != nil {
		return big.NewInt(0).SetBytes(tx.Amount)
	}
	return nil
}

func (tx *Txv1) SetAmount(amount *big.Int) {
	if tx != nil {
		copy(tx.Amount, amount.Bytes())
	}
}

func (tx *Txv1) GetFuelLimit() *big.Int {
	if tx != nil {
		return big.NewInt(0).SetBytes(tx.Fuel_Limit)
	}
	return nil
}

func (tx *Txv1) SetFuelLimit(fuelLimit *big.Int) {
	if tx != nil {
		copy(tx.Fuel_Limit, fuelLimit.Bytes())
	}
}

func (tx *Txv1) GetFuelPrice() *big.Int {
	if tx != nil {
		return big.NewInt(0).SetBytes(tx.Fuel_Price)
	}
	return nil
}

func (tx *Txv1) SetFuelPrice(fuelPrice *big.Int) {
	if tx != nil {
		copy(tx.Fuel_Price, fuelPrice.Bytes())
	}
}

type Txv2 struct {
}

func NewTxv1(ct ChainType) *Txv1 {
	return &Txv1{
		Chain_Type: ct,
	}
}

func NewServiceTxv1(ct ChainType, payload string, w *grape_wallet.Wallet) Txv1 {
	t := Txv1{
		Tx_Type:    SERVICE,
		Chain_Type: ct,
		Data:       []byte(payload),
	}
	t.Sender_Pubk = *w.PublicKey()
	t.Sender = grape1crypto.AddressToBytes(w.WalletAddress())
	t.Recepient = grape1crypto.AddressToBytes(w.WalletAddress())
	return t
}

func NewGenesisTxv1(ct ChainType, w *grape_wallet.Wallet) Txv1 {
	t := Txv1{
		Tx_Type:    SERVICE_GENESIS,
		Chain_Type: ct,
	}
	t.Sender_Pubk = *w.PublicKey()
	t.Sender = grape1crypto.AddressToBytes(w.WalletAddress())
	t.Recepient = grape1crypto.AddressToBytes(w.WalletAddress())
	return t
}

func NewPinTxv1(ct ChainType, w *grape_wallet.Wallet) Txv1 {
	t := Txv1{
		Tx_Type:    SERVICE_PIN,
		Chain_Type: ct,
	}
	t.Sender_Pubk = *w.PublicKey()
	t.Sender = grape1crypto.AddressToBytes(w.WalletAddress())
	t.Recepient = grape1crypto.AddressToBytes(w.WalletAddress())
	return t
}

func (thisTx *Txv1) IsService() bool {
	return thisTx.Tx_Type == SERVICE
}

func (thisTx *Txv1) IsPayload(payload string) bool {
	if thisTx.Data != nil {
		return strings.Compare(string(thisTx.Data), payload) == 0
	}
	return false
}

func (thisTx *Txv1) Equal(thatTx *Txv1) bool {
	switch {
	case thisTx == thatTx:
		fallthrough
	case slices.Equal(thisTx.Signature, thatTx.Signature):
		return true
	default:
		return false
	}
}

func (t *Txv1) Size() uint32 {
	x := reflect.ValueOf(*t).Type().Size()
	return uint32(x)
}

func (t *Txv1) Hash(algo crypto.Hash) ([]byte, error) {
	// Get wire format byte sequence from the transaction
	buf, err := proto.Marshal(t.MarshalBinary())
	if err != nil {
		logger.Errorf("[hash] Getting wire byte sequence error: %s", err.Error())
		return nil, err
	}
	return utils.GetBuilder().Build(crypto.SHA256).Hash(buf), nil
}

// Return SHA-256 hash of the tx with 0x prefix
func (t *Txv1) DefaultStringHash() string {
	hash, err := t.Hash(crypto.SHA256)
	if err != nil {
		panic(err)
	}
	return "0x" + hex.EncodeToString(hash)
}

func (t *Txv1) MarshalBinary() *pb.Txv1 {
	tpb := &pb.Txv1{}
	tpb.TxType = pb.TransactionType(t.Tx_Type)
	// tpb.Depth = t.Depth
	tpb.ChainType = pb.ChainType(t.Chain_Type)
	tpb.SenderPubk = t.Sender_Pubk
	tpb.Sender = t.Sender
	tpb.Recepient = t.Recepient
	tpb.Amount = t.Amount
	tpb.Nonce = t.Nonce
	tpb.Timestamp = timestamppb.New(t.Timestamp)
	tpb.FuelLimit = t.Fuel_Limit
	tpb.FuelPrice = t.Fuel_Price
	tpb.Data = t.Data
	tpb.Signature = t.Signature
	return tpb
}

func (t *Txv1) UnmarshalBinary(tpb *pb.Txv1) {
	t.Tx_Type = TransactionType(tpb.TxType)
	t.Chain_Type = ChainType(tpb.ChainType)
	// t.Depth = tpb.Depth
	t.Sender_Pubk = tpb.SenderPubk
	t.Sender = tpb.Sender
	t.Recepient = tpb.Recepient
	t.Amount = tpb.Amount
	t.Nonce = tpb.Nonce
	t.Timestamp = tpb.GetTimestamp().AsTime()
	t.Fuel_Limit = tpb.FuelLimit
	t.Fuel_Price = tpb.FuelPrice
	t.Data = tpb.Data
	t.Signature = tpb.Signature
}

func (t *Txv1) GeneratePayment(tx *wallet.Transaction, chaintype uint8) {
	t.Tx_Type = TransactionType(0)
	t.Amount = tx.GetAmount().Bytes()
	t.Chain_Type = ChainType(chaintype)
	t.Fuel_Limit = big.NewInt(0).Bytes()
	t.Fuel_Price = big.NewInt(0).Bytes()
	// Public Key - BEGIN
	t.Sender_Pubk = *tx.GetSenderPubK()
	// Public Key - END
	t.Nonce = utils.RandomUint64() % 200000
	t.Sender = grape1crypto.AddressToBytes(tx.GetSenderAddress())
	t.Recepient = grape1crypto.AddressToBytes(tx.GetReceiverAddress())
	t.Timestamp = time.Now()
	t.Tx_Type = PAYMENT
	t.Signature = []byte{}
	t.Signature = t.generateSignature(tx.GetSenderPrivK())
}

func (t *Txv1) GenerateRandom(maxFuelLimit, maxFuelPrice uint64, tx *wallet.Transaction, chaintype uint8) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	t.Tx_Type = TransactionType(0)
	t.Amount = big.NewInt(r.Int63()).Bytes()
	t.Chain_Type = ChainType(chaintype)
	t.Fuel_Limit = big.NewInt(r.Int63n(int64(maxFuelLimit))).Bytes()
	t.Fuel_Price = big.NewInt(r.Int63n(int64(maxFuelPrice))).Bytes()
	// t.Fuel_Limit = uint64(r.NormFloat64()/float64(maxFuelLimit) + float64(maxFuelLimit/2))
	// t.Fuel_Price = uint64(r.NormFloat64()/float64(maxFuelPrice) + float64(maxFuelPrice/2))
	// Public Key - BEGIN
	t.Sender_Pubk = *tx.GetSenderPubK()
	// Public Key - END
	t.Nonce = utils.RandomUint64() % 200000
	t.Sender = grape1crypto.AddressToBytes(tx.GetSenderAddress())
	t.Recepient = grape1crypto.AddressToBytes(tx.GetReceiverAddress())
	t.Timestamp = time.Now()
	if t.Tx_Type == CALL_CONTRACT || t.Tx_Type == PUBLISH_CONTRACT {
		rs := rand.New(rand.NewSource(time.Now().UnixMilli()))
		t.Data = make([]byte, rand.Intn(512))
		rs.Read(t.Data)
	}
	t.Signature = []byte{}
	t.Signature = t.generateSignature(tx.GetSenderPrivK())
}

func (t *Txv1) Sign(pk *grape_wallet.PrivateKey) {
	t.Signature = []byte{}
	t.Signature = t.generateSignature(pk)
}

func (t *Txv1) generateSignature(pk *grape_wallet.PrivateKey) []byte {
	// generate tx hash
	payload, err := t.Hash(crypto.SHA256)
	if err != nil {
		logger.Errorf("Tx marshal binary error: %s", err.Error())
		return nil
	}
	// Sign the hash
	t.Signature = grape_wallet.NewDSA().Sign(*pk, payload)
	if err != nil {
		logger.Errorf("Failed to sign transaction. %v", err)
		return nil
	}
	// Get the signature
	return t.Signature
}

// Verify - check the sender's signature over this transaction.
//
// The payload is the transaction with its signature field empty, because that is
// what was signed. This used to be done by blanking t.Signature on the receiver,
// hashing, and putting it back - which made verification a write. That is a
// hazard nobody had written down: the subscriber now verifies on several
// goroutines at once, and while each holds its own freshly unmarshalled record
// today, anything that ever verified one shared transaction from two goroutines
// would have them blanking and restoring the same field against each other. The
// hash is taken on a copy instead, so verifying is a read and the question does
// not arise.
//
// It also used to return nil - success - when the marshalling failed, having
// already blanked the signature and not yet restored it. A transaction that
// could not be hashed was therefore reported as verified, and lost its signature
// on the way through.
func (t *Txv1) Verify() error {
	sz := len(t.Signature)
	if sz < 64 {
		return errors.Errorf("Invalid tx signature of length %d", sz)
	}
	// A shallow copy is enough: the only field that has to differ is Signature,
	// and assigning to the copy's field cannot reach the original's bytes.
	unsigned := *t
	unsigned.Signature = nil
	payload, err := unsigned.Hash(crypto.SHA256)
	if err != nil {
		return errors.Wrap(err, "cannot hash the transaction to verify it")
	}

	valid := grape_wallet.NewDSA().Verify(t.Sender_Pubk, t.Signature, payload)
	if !valid {
		return errors.Errorf("Cannot verify transaction: %s", t.String())
	}
	return nil
}

func (t *Txv1) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		TxType     uint8     `json:"tx_type"`
		ChainType  uint8     `json:"chain_type"`
		Depth      uint64    `json:"depth"`
		SenderPubk string    `json:"sender_pubk"`
		Sender     string    `json:"sender"`
		Recepient  string    `json:"recepient"`
		Amount     uint64    `json:"amount"`
		Nonce      uint64    `json:"nonce"`
		Timestamp  time.Time `json:"timestamp"`
		FuelLimit  uint64    `json:"fuel_limit"`
		FuelPrice  uint64    `json:"fuel_price"`
		Data       []byte    `json:"data"`
	}{
		TxType:    uint8(t.Tx_Type),
		ChainType: uint8(t.Chain_Type),
		// Depth:      t.Depth,
		SenderPubk: hex.EncodeToString(t.Sender_Pubk),
		// Rendered rather than converted. Both addresses used to go through the
		// crypto package's conversions, which panic on anything that is not
		// twenty bytes long - and String() is called from log lines on the
		// publish path, on transactions that arrived over the network and have
		// not been checked yet. So describing a malformed transaction crashed the
		// node that received it. A malformed transaction has to be describable:
		// describing it is how it gets refused.
		Sender:    renderAddress(t.Sender),
		Recepient: renderAddress(t.Recepient),
		Amount:    big.NewInt(0).SetBytes(t.Amount).Uint64(),
		Nonce:     t.Nonce,
		Timestamp: t.Timestamp,
		FuelLimit: big.NewInt(0).SetBytes(t.Fuel_Limit).Uint64(),
		FuelPrice: big.NewInt(0).SetBytes(t.Fuel_Price).Uint64(),
		Data:      t.Data,
	})

}

// renderAddress - an address as it should appear in a log line or an error,
// whatever is actually in the field.
//
// Deliberately not crypto.BytesToAddress: that function asserts the twenty-byte
// length and panics otherwise, which is the right behaviour for code that has
// already established the address is valid and the wrong behaviour for code
// whose job is to describe something that might not be. A length that is not
// twenty is stated rather than hidden, so a log line about a refused transaction
// says what was wrong with it.
func renderAddress(address []byte) string {
	switch {
	case len(address) == 0:
		return "0x"
	case len(address) == grape1crypto.AddressLength:
		return grape1crypto.BytesToAddress(address)
	default:
		return fmt.Sprintf("0x%s (%d bytes, not %d)", hex.EncodeToString(address), len(address), grape1crypto.AddressLength)
	}
}

func (t *Txv1) String() string {
	var out bytes.Buffer
	var in []byte
	in, err := t.MarshalJSON()
	if err != nil {
		logger.Errorf("Failed to marshal tx to json. %v", err)
		return ""
	}
	err = json.Indent(&out, in, "", "\t")
	if err != nil {
		logger.Errorf("Failed to indent json payload. %v", err)
		return ""
	}
	return out.String()
}

func (t *Txv1) UnmarshalJSON(data []byte) error {
	var tx_type uint8
	var chain_type uint8
	var amount uint64
	var fuel_limit uint64
	var fuel_price uint64
	spk, snd, rcp := string(""), string(""), string("")
	v := &struct {
		TxType     *uint8     `json:"tx_type"`
		ChainType  *uint8     `json:"chain_type"`
		Depth      *uint64    `json:"depth"`
		SenderPubk *string    `json:"sender_pubk"`
		Sender     *string    `json:"sender"`
		Recepient  *string    `json:"recepient"`
		Amount     *uint64    `json:"amount"`
		Nonce      *uint64    `json:"nonce"`
		Timestamp  *time.Time `json:"timestamp"`
		FuelLimit  *uint64    `json:"fuel_limit"`
		FuelPrice  *uint64    `json:"fuel_price"`
		Data       *[]byte    `json:"data"`
	}{
		TxType:     &tx_type,
		ChainType:  &chain_type,
		SenderPubk: &spk,
		Sender:     &snd,
		Recepient:  &rcp,
		Amount:     &amount,
		Nonce:      &t.Nonce,
		Timestamp:  &t.Timestamp,
		FuelLimit:  &fuel_limit,
		FuelPrice:  &fuel_price,
		Data:       &t.Data,
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	t.Tx_Type = TransactionType(tx_type)
	t.Chain_Type = ChainType(chain_type)
	t.Amount = big.NewInt(0).SetUint64(amount).Bytes()
	t.Fuel_Limit = big.NewInt(0).SetUint64(fuel_limit).Bytes()
	t.Fuel_Price = big.NewInt(0).SetUint64(fuel_limit).Bytes()
	t.Sender_Pubk = []byte(spk)
	t.Sender = grape1crypto.AddressToBytes(snd)
	t.Recepient = grape1crypto.AddressToBytes(rcp)
	return nil
}

func (tx *Txv1) GetType() int {
	return 0
}

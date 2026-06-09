package test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"google.golang.org/protobuf/proto"
)

func TestAmountBigInt(t *testing.T) {
	rand.Seed(time.Now().UnixMicro())
	sender_wallet := grape1crypto.NewWallet()
	recipient_wallet := grape1crypto.NewWallet()

	transaction := tx.Txv1{
		Tx_Type:     tx.TransactionType(pb.TransactionType_PAYMENT),
		Chain_Type:  tx.MAINNET,
		Sender_Pubk: *sender_wallet.PublicKey(),
		Sender:      grape1crypto.AddressToBytes(sender_wallet.WalletAddress()),
		Recepient:   grape1crypto.AddressToBytes(grape1crypto.AddressFromPulicKey(*recipient_wallet.PublicKey())),
		Amount:      big.NewInt(rand.Int63n(20000)).Bytes(),
		Nonce:       uint64(rand.Int()),
		Timestamp:   time.Now(),
		Fuel_Limit:  big.NewInt(rand.Int63n(30000)).Bytes(),
		Fuel_Price:  big.NewInt(rand.Int63n(20)).Bytes(),
		Data:        nil,
		Signature:   big.NewInt(rand.Int63()).FillBytes(make([]byte, 64)),
	}

	fmt.Printf("Tx Signature: %s\n", hex.EncodeToString(transaction.Signature))

	transaction.Sign(sender_wallet.PrivateKey())
	pbBytes, err := proto.Marshal(transaction.MarshalBinary())

	if err != nil {
		t.Errorf("Failed to proto marshal tx: %s", err.Error())
	}

	fmt.Printf("Tx bytes: %s\n", hex.EncodeToString(pbBytes))

	if err = transaction.Verify(); err != nil {
		t.Errorf("Transaction signature verification failed: %s", err)
	}
	fmt.Println(t.Name(), " + OK")
}

func TestAmountBigIntMarshal(t *testing.T) {
	rand.Seed(time.Now().UnixMicro())
	sender_wallet := grape1crypto.NewWallet()
	recipient_wallet := grape1crypto.NewWallet()

	transaction := tx.Txv1{
		Tx_Type:     tx.TransactionType(pb.TransactionType_PAYMENT),
		Chain_Type:  tx.MAINNET,
		Sender_Pubk: *sender_wallet.PublicKey(),
		Sender:      grape1crypto.AddressToBytes(sender_wallet.WalletAddress()),
		Recepient:   grape1crypto.AddressToBytes(grape1crypto.AddressFromPulicKey(*recipient_wallet.PublicKey())),
		Amount:      big.NewInt(rand.Int63n(20000)).Bytes(),
		Nonce:       uint64(rand.Int()),
		Timestamp:   time.Now(),
		Fuel_Limit:  big.NewInt(rand.Int63n(30000)).Bytes(),
		Fuel_Price:  big.NewInt(rand.Int63n(20)).Bytes(),
		Data:        nil,
		Signature:   big.NewInt(rand.Int63()).FillBytes(make([]byte, 64)),
	}

	_, err := transaction.MarshalJSON()

	if err != nil {
		t.Errorf("Marshal Tx to JSON failed: %s", err.Error())
	}

	fmt.Println(t.Name(), " + OK")
}

func TestAmountBigIntMarshalUnmarshal(t *testing.T) {
	rand.Seed(time.Now().UnixMicro())
	sender_wallet := grape1crypto.NewWallet()
	recipient_wallet := grape1crypto.NewWallet()

	transaction := tx.Txv1{
		Tx_Type:     tx.TransactionType(pb.TransactionType_PAYMENT),
		Chain_Type:  tx.MAINNET,
		Sender_Pubk: *sender_wallet.PublicKey(),
		Sender:      grape1crypto.AddressToBytes(sender_wallet.WalletAddress()),
		Recepient:   grape1crypto.AddressToBytes(grape1crypto.AddressFromPulicKey(*recipient_wallet.PublicKey())),
		Amount:      big.NewInt(rand.Int63n(20000)).Bytes(),
		Nonce:       uint64(rand.Int()),
		Timestamp:   time.Now(),
		Fuel_Limit:  big.NewInt(rand.Int63n(30000)).Bytes(),
		Fuel_Price:  big.NewInt(rand.Int63n(20)).Bytes(),
		Data:        nil,
		Signature:   big.NewInt(rand.Int63()).FillBytes(make([]byte, 64)),
	}

	pbBytes1, err := proto.Marshal(transaction.MarshalBinary())
	if err != nil {
		t.Errorf("Failed to marshal binary tx: %s", err.Error())
	}

	payload, err := transaction.MarshalJSON()

	if err != nil {
		t.Errorf("Marshal Tx to JSON failed: %s", err.Error())
	}

	transaction2 := tx.Txv1{}
	if err = transaction2.UnmarshalJSON(payload); err != nil {
		t.Errorf("Failed to unmarshal Tx from JSON: %s", err)
	}

	pbBytes2, err := proto.Marshal(transaction.MarshalBinary())

	if err != nil {
		t.Errorf("Failed to marshal re-marshaled tx: %s", err.Error())
	}

	if !bytes.Equal(pbBytes1, pbBytes2) {
		t.Errorf("Failed to compare marshaled and unmarshaled bytes")
	}

	fmt.Println(t.Name(), " + OK")
}

func TestDepthInData(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixMicro()))
	depth := uint64(r.Int63())
	buf := make([]byte, 1024)
	r.Read(buf)
	data := []byte{}
	depth_data := make([]byte, 8)
	binary.BigEndian.PutUint64(depth_data, depth)
	length_data := make([]byte, 2)
	binary.BigEndian.PutUint16(length_data, uint16(32))
	data = append(data, depth_data...)
	data = append(data, length_data...)
	data = append(data, buf...)

	depth2 := binary.BigEndian.Uint64(data[:8])
	if depth2 != depth {
		t.Errorf("Incorrect depth value retrieved: %d. Expected %d", depth2, depth)
	}
	length2 := binary.BigEndian.Uint16(data[8:10])
	if length2 != 32 {
		t.Errorf("Incorrect length value retrieved: %d. Expected %d", length2, 32)
	}
	fmt.Printf("Smart contract: %s", hex.EncodeToString(data[10:]))
}

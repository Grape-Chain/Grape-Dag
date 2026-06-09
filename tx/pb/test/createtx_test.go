package test

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProtobuffTxCreationSerializationSigningAndVerification(t *testing.T) {
	// Part 1
	tm := pb.Txv1{}

	//  Load Sender's account Wallet (Private + Public key) Ed25519 curve
	wallet := grape1crypto.LoadWallet("2bd4e8de88c1578aeb38f8c04f7af4d66c99cc0fe100d06641b4dd4ee0ae3220",
		"8e38269f2cc110b31572e2d9e74aa466c770e82eec34c2f0037e0531822e1e4b")
	anotherWallet := grape1crypto.NewWallet()

	// Create Transaction by setting all fields of Txv1 protobuff-generated struct
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	tm.Amount = big.NewInt(rand.Int63n(1000000)).Bytes()
	tm.ChainType = pb.ChainType_PUBLIC_TESTNET
	tm.FuelLimit = big.NewInt(r.Int63n(10) + 10).Bytes()
	tm.FuelPrice = big.NewInt(r.Int63n(10) + 1).Bytes()
	tm.SenderPubk = []byte(*wallet.PublicKey()) // set sender's PublicKey for verification
	tm.Nonce = 0
	tm.Recepient = grape1crypto.AddressToBytes(anotherWallet.WalletAddress())
	tm.Timestamp = timestamppb.New(time.Now())

	// Get unsigned bytes of transaction by Marshaling it using ProtoBuff
	unsignedBytes, _ := proto.Marshal(&tm)
	fmt.Printf("Generated tx: %v\n", &tm)
	fmt.Printf("My unsigned tx ProtoBuff bytes: %s\n", hex.EncodeToString(unsignedBytes))

	// Sign unsigned bytes of transaction via grape1crypto API using sender's Private Key
	signature := grape1crypto.NewDSA().Sign(*wallet.PrivateKey(), unsignedBytes)
	fmt.Printf("Signature: %s\n", hex.EncodeToString(signature))

	// Set signature to usigned transaction object to create a Signed Transaction
	tm.Signature = signature

	// Serialize signed Transaction and broadcast to the network in hexadecimal format
	signedBytes, _ := proto.Marshal(&tm)

	fmt.Printf("Signed transaction bytes: %s\n", hex.EncodeToString(signedBytes))

	// Part 2
	// Parse Transaction back from ProtoBuff bytes
	deserializedSignedTx := pb.Txv1{}
	proto.Unmarshal(signedBytes, &deserializedSignedTx)

	// Make an Unsigned Tx to Get Unsigned Bytes (clone to avoid copying
	// the protobuf message's internal lock).
	deserializedUnsignedTx := proto.Clone(&deserializedSignedTx).(*pb.Txv1)
	deserializedUnsignedTx.Signature = nil

	// Get Unsigned Bytes
	serializedPbBytesToVerify, _ := proto.Marshal(deserializedUnsignedTx)
	// Verify Signature over Unsigned Bytes
	var verified = grape1crypto.NewDSA().Verify(deserializedSignedTx.SenderPubk, deserializedSignedTx.Signature, serializedPbBytesToVerify)
	if !verified {
		t.Errorf("Bad transaction %v, signature verfication failed", t)
	}
	fmt.Println("Signature verified OK")

}

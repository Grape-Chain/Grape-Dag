package main

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"

	"github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/enescakir/emoji"
	"golang.org/x/crypto/scrypt"
	"google.golang.org/protobuf/proto"
)

func main() {
	var ico_value string
	var leader_id string
	var leader_pk string
	flag.StringVar(&ico_value, "ico", "0", "Initial coin offering")
	flag.StringVar(&leader_id, "leader_id", "", "Leader key")
	flag.StringVar(&leader_pk, "wallet_pk", "", "Leader wallet's private key")
	flag.Parse()

	if len(leader_id) == 0 {
		fmt.Printf("%s\tleader_id must be provided\n", emoji.StopSign)
		flag.PrintDefaults()
		os.Exit(0)
	}

	if len(leader_pk) == 0 {
		fmt.Printf("%s\tleader %s private key must be provided\n", emoji.StopSign, emoji.Purse)
		flag.PrintDefaults()
		os.Exit(0)
	}

	pk, err := grape1crypto.ParsePrivateKey(leader_pk)
	if err != nil {
		fmt.Printf("%s\tleader %s private key is invalid %s\n", emoji.StopSign, emoji.Purse, err.Error())
		flag.PrintDefaults()
		os.Exit(0)
	}

	ico, status := big.NewInt(0).SetString(ico_value, 10)
	if !status {
		fmt.Printf("%s\tFailed to parse the ICO value\n", emoji.StopSign)
		flag.PrintDefaults()
		os.Exit(0)
	}
	if ico.Cmp(big.NewInt(0)) <= 0 {
		fmt.Printf("%s\tIncorrect or insufficient value for ICO: %s", emoji.Warning, ico.String())
		os.Exit(0)
	}

	secret_seed := make([]byte, 256)
	_, err = io.ReadFull(rand.Reader, secret_seed)
	if err != nil {
		panic(err.Error())
	}
	secret_str := base64.StdEncoding.EncodeToString(secret_seed)

	salt := make([]byte, 32)
	_, err = io.ReadFull(rand.Reader, salt)
	if err != nil {
		panic(err.Error())
	}

	dk, err := scrypt.Key([]byte(secret_str), salt, 1<<15, 8, 1, 1<<5)
	if err != nil {
		panic(err.Error())
	}

	// prepare activation file
	secret := &pb.Secret{
		Id:   leader_id,
		Ico:  ico.Bytes(),
		Sign: []byte{},
	}

	msg, err := proto.Marshal(secret)

	privKey := ed25519.NewKeyFromSeed(pk)
	signature, err := privKey.Sign(rand.Reader, msg, crypto.Hash(0))
	pubKey := privKey.Public()
	z := pubKey.(ed25519.PublicKey)
	if !ed25519.Verify(z, msg, signature) {
		fmt.Printf("Failed to verify the signature %s", emoji.WorriedFace)
		os.Exit(0)
	}

	secret.Sign = signature
	msg, _ = proto.Marshal(secret)

	block, err := aes.NewCipher(dk)
	if err != nil {
		panic(err)
	}

	// The IV needs to be unique, but not secure and included at the beginning of the ciphertext
	ciphertext := make([]byte, aes.BlockSize+len(msg))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], msg)

	// Verify

	verify_ciphertext := make([]byte, len(ciphertext))

	copy(verify_ciphertext, ciphertext)

	verify_ciphertext = verify_ciphertext[aes.BlockSize:]

	verify_stream := cipher.NewCFBDecrypter(block, iv)

	// XORKeyStream can work in-place if the two arguments are the same.
	verify_stream.XORKeyStream(verify_ciphertext, verify_ciphertext)

	payload := &pb.Secret{}

	proto.Unmarshal(verify_ciphertext, payload)

	payload.Sign = []byte{}

	payload_pb, _ := proto.Marshal(payload)

	if !ed25519.Verify(z, payload_pb, signature) {
		fmt.Printf("Failed to verify the signature %s\n", emoji.WorriedFace)
		os.Exit(0)
	}

	// let's print out the keys, hmac and save the file
	secret_key := base64.StdEncoding.EncodeToString(dk)
	fmt.Printf("%s => %s \n", emoji.Key, secret_key)
	fn := fmt.Sprintf("%s.activation", leader_id)
	fd, err := os.Create(fn)
	if err != nil {
		fmt.Printf("Failed to create activation file. err: %s\n", err.Error())
		if err = os.Remove(fn); err != nil {
			os.Exit(0)
		}
		fd, err = os.Create(fn)
	}
	n, err := fd.Write(ciphertext)
	if err != nil || len(ciphertext) != n {
		fmt.Printf("Failed to write to activation file. err: %s", err.Error())
	}
	fd.Chmod(0400)
	fd.Close()

	fmt.Printf("%s has been successfully created\n", fn)
	fmt.Printf("Use this file to active a %s Leader %s\n", emoji.Grapes, emoji.Grapes)
}

func validate(fn string, secret string, pr_key string) ([]byte, error) {

	fd, err := os.OpenFile(fn, os.O_RDONLY, fs.ModeExclusive)
	if err != nil {
		return nil, err
	}
	act := []byte{}
	buf := make([]byte, 1024)
	for {
		buf = buf[:cap(buf)]
		n, err := fd.Read(buf)
		if err == io.EOF {
			if n > 0 {
				act = append(act, buf[:n]...)
			}
			break
		} else if err != nil {
			return nil, err
		}
		act = append(act, buf[:n]...)
	}

	dk, err := base64.StdEncoding.DecodeString(secret)

	block, err := aes.NewCipher(dk)
	if err != nil {
		panic(err)
	}

	iv := act[:aes.BlockSize]
	verify_stream := cipher.NewCFBDecrypter(block, iv)
	ciphertext := act[aes.BlockSize:]
	// XORKeyStream can work in-place if the two arguments are the same.
	verify_stream.XORKeyStream(ciphertext, ciphertext)

	payload := &pb.Secret{}

	proto.Unmarshal(ciphertext, payload)
	signature := payload.Sign
	payload.Sign = []byte{}

	payload_pb, _ := proto.Marshal(payload)

	pk, err := grape1crypto.ParsePrivateKey(pr_key)
	if err != nil {
		fmt.Printf("%s\tleader %s private key is invalid %s\n", emoji.StopSign, emoji.Purse, err.Error())
		flag.PrintDefaults()
		os.Exit(0)
	}

	privKey := ed25519.NewKeyFromSeed(pk)
	pubKey := privKey.Public()
	z := pubKey.(ed25519.PublicKey)
	if !ed25519.Verify(z, payload_pb, signature) {
		fmt.Printf("Failed to verify the signature %s", emoji.WorriedFace)
		os.Exit(0)
	}

	return nil, nil
}

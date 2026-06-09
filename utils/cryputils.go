package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	mrand "math/rand"
)

type Signature struct {
	R *big.Int
	S *big.Int
}

func (s *Signature) Marshal() []byte {
	buf := []byte{}
	br, _ := s.R.MarshalJSON()
	buf = append(buf, byte(len(br)))
	buf = append(buf, br...)
	bs, _ := s.S.MarshalJSON()
	buf = append(buf, byte(len(bs)))
	buf = append(buf, bs...)
	return buf
}

func (s *Signature) Unmarshal(b []byte) {
	offset := b[0]
	err := s.R.UnmarshalJSON(b[1 : offset+1])
	if err != nil {
		logger.Errorf("UnmarshalJSON error: %s", err.Error())
	}
	err = s.S.UnmarshalJSON(b[offset+2:])
	if err != nil {
		logger.Errorf("UnmarshalJSON error: %s", err.Error())
	}
}

func (s *Signature) String() string {
	return fmt.Sprintf("%064x%064x", s.R, s.S)
}

func String2BigIntTuple(s string) (big.Int, big.Int) {
	bx, _ := hex.DecodeString(s[:64])
	by, _ := hex.DecodeString(s[64:])

	var bix big.Int
	var biy big.Int

	_ = bix.SetBytes(bx)
	_ = biy.SetBytes(by)

	return bix, biy
}

func SignatureFromString(s string) *Signature {
	x, y := String2BigIntTuple(s)
	return &Signature{&x, &y}
}

func PublicKeyFromString(s string) *ecdsa.PublicKey {
	x, y := String2BigIntTuple(s)
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: &x, Y: &y}
}

func PrivateKeyFromString(s string, publicKey *ecdsa.PublicKey) *ecdsa.PrivateKey {
	b, _ := hex.DecodeString(s[:])
	var bi big.Int
	_ = bi.SetBytes(b)
	return &ecdsa.PrivateKey{PublicKey: *publicKey, D: &bi}
}

func RandomUint64() uint64 {
	// 8 byte long buffer to accept a cryptosecure sequence of bytes
	var rbuf [8]byte
	// read in the random sequnece
	crand.Read(rbuf[:])
	// convert the sequence of bytes into an uint64 var and use it as a seed
	mrand.Seed(int64(binary.LittleEndian.Uint64(rbuf[:])))
	// nicely seeded, let's generate a psuedo perfect random uint64
	return mrand.Uint64()
}

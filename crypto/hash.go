package grape1crypto

import (
	"crypto/sha256"
	"hash"

	"golang.org/x/crypto/sha3"
)

func NewHasher() Hasher {
	hasher := sha256Hasher{}
	hasher.internalHasher = sha256.New()
	return &hasher
}

func NewSHA3Hasher() Hasher {
	hasher := keccak256Hasher{}
	hasher.internalHasher = sha3.NewLegacyKeccak256()
	return &hasher
}

type Hasher interface {
	Add(data []byte)
	Digest(data []byte) []byte
}

type sha256Hasher struct {
	internalHasher hash.Hash
}

func (s *sha256Hasher) Add(data []byte) {
	s.internalHasher.Write(data)
}

func (s *sha256Hasher) Digest(data []byte) []byte {
	return s.internalHasher.Sum(nil)
}

type keccak256Hasher struct {
	internalHasher hash.Hash
}

func (s *keccak256Hasher) Add(data []byte) {
	s.internalHasher.Write(data)
}

func (s *keccak256Hasher) Digest(data []byte) []byte {
	return s.internalHasher.Sum(nil)
}

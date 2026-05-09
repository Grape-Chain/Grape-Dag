package utils

import (
	"crypto"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"

	"golang.org/x/crypto/sha3"
)

type Stringable interface {
	String() string
}

type Hashable interface {
	Stringable
	Hash([]byte) []byte
}

type HashBuffer struct {
	t   string
	buf []byte
}

type HashableBuilder interface {
	Build(crypto.Hash) Hashable
}

type HashingBuilder struct{}

func (b *HashingBuilder) Build(algo crypto.Hash) Hashable {
	switch algo {
	case crypto.MD5:
		return newMD5Hash()
	case crypto.SHA1:
		return newSHA1Hash()
	case crypto.SHA224:
		return newSHA224Hash()
	case crypto.SHA256:
		return newSHA256Hash()
	case crypto.SHA384:
		return newSHA384Hash()
	case crypto.SHA512:
		return newSHA512Hash()
	case crypto.SHA3_224:
		return newSHA3_224Hash()
	case crypto.SHA3_384:
		return newSHA3_384Hash()
	case crypto.SHA3_512:
		return newSHA3_512Hash()
	}
	return nil
}

func GetBuilder() HashableBuilder {
	return &HashingBuilder{}
}

func newMD5Hash() Hashable {
	return &MD5Hashing{HashBuffer{t: "MD5"}}
}

type MD5Hashing struct {
	HashBuffer
}

func (h *MD5Hashing) Hash(in []byte) []byte {
	bs := md5.Sum(in)
	h.buf = bs[:]
	return h.buf
}

func (h *MD5Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

func newSHA1Hash() Hashable {
	return &SHA1Hashing{HashBuffer{t: "SHA1"}}
}

type SHA1Hashing struct {
	HashBuffer
}

func (h *SHA1Hashing) Hash(in []byte) []byte {
	h.buf = crypto.SHA1.New().Sum(in)
	return h.buf
}

func (h *SHA1Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

func newSHA224Hash() Hashable {
	return &SHA224Hashing{HashBuffer{t: "SHA224"}}
}

type SHA224Hashing struct {
	HashBuffer
}

func (h *SHA224Hashing) Hash(in []byte) []byte {
	h.buf = crypto.SHA224.New().Sum(in)
	return h.buf
}

func (h *SHA224Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

func newSHA256Hash() Hashable {
	return &SHA256Hashing{HashBuffer{t: "SHA256"}}
}

type SHA256Hashing struct {
	HashBuffer
}

func (h *SHA256Hashing) Hash(in []byte) []byte {
	bs := sha256.Sum256(in)
	h.buf = bs[:]
	return h.buf
}

func (h *SHA256Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

func newSHA384Hash() Hashable {
	return &SHA384Hashing{HashBuffer{t: "SHA384"}}
}

type SHA384Hashing struct {
	HashBuffer
}

func (h *SHA384Hashing) Hash(in []byte) []byte {
	bs := sha512.Sum384(in)
	h.buf = bs[:]
	return h.buf
}

func (h *SHA384Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

func newSHA512Hash() Hashable {
	return &SHA512Hashing{HashBuffer{t: "SHA512"}}
}

type SHA512Hashing struct {
	HashBuffer
}

func (h *SHA512Hashing) Hash(in []byte) []byte {
	bs := sha512.Sum512(in)
	h.buf = bs[:]
	return h.buf
}

func (h *SHA512Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

func newSHA3_224Hash() Hashable {
	return &SHA3_224Hashing{HashBuffer{t: "SHA3-224"}}
}

type SHA3_224Hashing struct {
	HashBuffer
}

func (h *SHA3_224Hashing) Hash(in []byte) []byte {
	bs := sha3.Sum224(in)
	h.buf = bs[:]
	return h.buf
}

func (h *SHA3_224Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

func newSHA3_384Hash() Hashable {
	return &SHA3_384Hashing{HashBuffer{t: "SHA3-384"}}
}

type SHA3_384Hashing struct {
	HashBuffer
}

func (h *SHA3_384Hashing) Hash(in []byte) []byte {
	bs := sha3.Sum384(in)
	h.buf = bs[:]
	return h.buf
}

func (h *SHA3_384Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

func newSHA3_512Hash() Hashable {
	return &SHA3_512Hashing{HashBuffer{t: "SHA3-512"}}
}

type SHA3_512Hashing struct {
	HashBuffer
}

func (h *SHA3_512Hashing) Hash(in []byte) []byte {
	bs := sha3.Sum512(in)
	h.buf = bs[:]
	return h.buf
}

func (h *SHA3_512Hashing) String() string {
	return fmt.Sprintf("%s: %x", h.t, h.buf)
}

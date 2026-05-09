package utils

import (
	"crypto/sha256"
	"hash/crc64"
	"os"
	"os/signal"

	"github.com/google/uuid"
)

func WaitOnSignal(signals []os.Signal) {
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs, signals...)
	<-sigs
}

func UuidToUint64(id uuid.UUID) uint64 {
	if id == uuid.Nil {
		return 0
	}
	v, _ := id.MarshalBinary()
	h := sha256.New()
	h.Write([]byte(v))
	sha2_hash := h.Sum(nil)
	t := crc64.MakeTable(crc64.ECMA)
	h64 := crc64.Checksum(sha2_hash, t)
	return h64
}

package auth

import (
	crand "crypto/rand"
	"encoding/binary"
)

// CryptoRandSource represents a source of uniformly-distributed random int64 values in the range [0, 1<<63).
type CryptoRandSource struct{}

// Int63 returns a non-negative random 63-bit integer as an int64 from CryptoRandSource.
func (CryptoRandSource) Int63() int64 {
	var b [8]byte

	if _, err := crand.Read(b[:]); err != nil {
		panic(err) // fail - can't continue
	}

	return int64(binary.LittleEndian.Uint64(b[:]) & (1<<63 - 1))
}

// Seed is a fake implementation for rand.Source interface from math/rand.
func (CryptoRandSource) Seed(int64) {}

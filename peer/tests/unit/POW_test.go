package unit

import (
	"crypto/sha256"
	"math/big"
	"testing"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
)

func TestCheckWorkBasic(t *testing.T) {
	header := []byte("hello")
	hash := sha256Sum(header)

	// target just above hash -> valid
	targetOK := new(big.Int).Add(new(big.Int).SetBytes(hash[:]), big.NewInt(1))
	if !impl.CheckWork(header, targetOK) {
		t.Fatalf("expected work to be valid")
	}

	// target just below hash -> invalid
	targetBad := new(big.Int).Sub(new(big.Int).SetBytes(hash[:]), big.NewInt(1))
	if impl.CheckWork(header, targetBad) {
		t.Fatalf("expected work to be invalid")
	}

	if impl.CheckWork(nil, targetOK) {
		t.Fatalf("expected empty header to fail")
	}
}

func TestMineNonceUsesEasyTarget(t *testing.T) {
	headerFor := func(n uint64, ts int64) []byte {
		return append(uint64ToBytes(n), uint64ToBytes(uint64(ts))...)
	}

	// Compute an easy target that accepts nonce 0.
	firstHeader := headerFor(0, 1)
	firstHash := sha256Sum(firstHeader)
	// Use a target just above the hash of the first header so nonce 0 passes PoW.
	easyTarget := new(big.Int).Add(new(big.Int).SetBytes(firstHash[:]), big.NewInt(1))

	cfg := peer.PoWConfig{
		Target:     easyTarget,
		MaxNonce:   10,
		TimeSource: func() time.Time { return time.Unix(1, 0) },
	}
	nonce, ok := callMineNonce(headerFor, cfg, nil)
	if !ok {
		t.Fatalf("expected mining success with easy target")
	}
	if nonce != 0 {
		t.Fatalf("expected nonce 0, got %d", nonce)
	}
	ts := cfg.TimeSource().Unix()
	winningHeader := headerFor(nonce, ts)
	winningHash := sha256Sum(winningHeader)
	if new(big.Int).SetBytes(winningHash[:]).Cmp(easyTarget) >= 0 {
		t.Fatalf("hash not under easy target")
	}
}

func TestMineNonceHonorsMaxNonce(t *testing.T) {
	headerFor := func(n uint64, ts int64) []byte {
		return append(uint64ToBytes(n), uint64ToBytes(uint64(ts))...)
	}
	cfg := peer.PoWConfig{
		Target:     big.NewInt(1), // effectively impossible to hit
		MaxNonce:   2,
		TimeSource: func() time.Time { return time.Unix(1, 0) },
	}
	nonce, ok := callMineNonce(headerFor, cfg, nil)
	if ok {
		t.Fatalf("expected mining to fail with too low target")
	}
	if nonce != 0 {
		t.Fatalf("expected nonce 0 on failure, got %d", nonce)
	}
}

func TestMineNonceStopsEarly(t *testing.T) {
	cfg := peer.PoWConfig{
		Target:     big.NewInt(1), // impossible to reach quickly
		MaxNonce:   1000,
		TimeSource: func() time.Time { return time.Unix(1, 0) },
	}
	buildCalls := 0
	headerFor := func(n uint64, ts int64) []byte {
		buildCalls++
		return append(uint64ToBytes(n), uint64ToBytes(uint64(ts))...)
	}
	stop := make(chan struct{})
	close(stop)
	_, ok := callMineNonce(headerFor, cfg, stop)
	if ok {
		t.Fatalf("expected mining to stop without success")
	}
	if buildCalls != 0 {
		t.Fatalf("expected mining to stop before building headers, got %d calls", buildCalls)
	}
}

// Helpers

func sha256Sum(b []byte) [32]byte {
	return sha256.Sum256(b)
}

func uint64ToBytes(v uint64) []byte {
	var out [8]byte
	for i := uint(0); i < 8; i++ {
		out[7-i] = byte(v >> (i * 8))
	}
	return out[:]
}

// callMineNonce wraps the unexported mineNonce for test use.
func callMineNonce(build impl.PoWHeaderBuilder, cfg peer.PoWConfig, stop <-chan struct{}) (uint64, bool) {
	return impl.MineNonce(build, cfg, stop)
}

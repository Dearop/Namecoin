package unit

import (
	"crypto/sha256"
	"math/big"
	"testing"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func TestCheckWork_Basic(t *testing.T) {
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

func TestCheckWork_UsesDefaultTargetWhenNil(t *testing.T) {
	header := []byte("hello")

	// When target is nil, CheckWork should behave like using the default target.
	defaultTarget := new(big.Int).Lsh(big.NewInt(1), 240)
	gotNil := impl.CheckWork(header, nil)
	gotDefault := impl.CheckWork(header, defaultTarget)
	if gotNil != gotDefault {
		t.Fatalf("expected nil target to use default target semantics")
	}
}

func TestCheckWork_ZeroTargetAlwaysFails(t *testing.T) {
	header := []byte("hello")

	// When target is explicitly zero, CheckWork must always fail.
	if impl.CheckWork(header, big.NewInt(0)) {
		t.Fatalf("expected zero target to always fail")
	}
}

func TestMineNonce_UsesEasyTarget(t *testing.T) {
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

func TestMineNonce_HonorsMaxNonce(t *testing.T) {
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

func TestMineNonce_StopsEarly(t *testing.T) {
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

func TestMineNonce_DegenerateConfig(t *testing.T) {
	// Nil header builder should immediately fail.
	cfg := peer.PoWConfig{
		Target:     big.NewInt(1),
		MaxNonce:   10,
		TimeSource: func() time.Time { return time.Unix(1, 0) },
	}
	nonce, ok := callMineNonce(nil, cfg, nil)
	if ok {
		t.Fatalf("expected mining to fail with nil header builder")
	}
	if nonce != 0 {
		t.Fatalf("expected nonce 0 on failure, got %d", nonce)
	}

	// Zero target is degenerate and should also fail without doing useful work.
	headerFor := func(n uint64, ts int64) []byte {
		return append(uint64ToBytes(n), uint64ToBytes(uint64(ts))...)
	}
	cfg = peer.PoWConfig{
		Target:     big.NewInt(0),
		MaxNonce:   10,
		TimeSource: func() time.Time { return time.Unix(1, 0) },
	}
	nonce, ok = callMineNonce(headerFor, cfg, nil)
	if ok {
		t.Fatalf("expected mining to fail with zero target")
	}
	if nonce != 0 {
		t.Fatalf("expected nonce 0 on failure, got %d", nonce)
	}
}

func TestIsBlockComplexityValid(t *testing.T) {
	// Build a simple block with a deterministic hash and a very high target so it must pass.
	var hdr types.BlockHeader
	block := types.Block{
		Header: hdr,
	}
	block.Hash = block.ComputeHash()

	// Target = 2^256 ensures any 256-bit hash is below the target.
	highTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	if !impl.IsBlockComplexityValid(block, highTarget) {
		t.Fatalf("expected block complexity to be valid under high target")
	}

	// With a zero target, the same block should be invalid (hash != 0 with overwhelming probability).
	if impl.IsBlockComplexityValid(block, big.NewInt(0)) {
		t.Fatalf("expected block complexity to be invalid under zero target")
	}
}

func TestEncodeDecodeDifficulty(t *testing.T) {
	target := big.NewInt(1234567890)
	enc := impl.EncodeDifficulty(target)
	if len(enc) != 32 {
		t.Fatalf("expected fixed width encoding, got %d", len(enc))
	}
	dec := impl.DecodeDifficulty(enc)
	if target.Cmp(dec) != 0 {
		t.Fatalf("expected decoded target %v, got %v", target, dec)
	}
}

func TestAdjustDifficultyClamps(t *testing.T) {
	prev := big.NewInt(1000)
	higher := impl.AdjustDifficulty(prev, 100*time.Second, 10*time.Second, 4, 0.25)
	if higher.Cmp(big.NewInt(4000)) != 0 {
		t.Fatalf("expected adjustment capped to 4x, got %v", higher)
	}

	lower := impl.AdjustDifficulty(prev, time.Second, 10*time.Second, 4, 0.25)
	if lower.Cmp(big.NewInt(250)) != 0 {
		t.Fatalf("expected adjustment capped to 0.25x, got %v", lower)
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
	nonce, _, ok := impl.MineNonce(build, cfg, stop)
	return nonce, ok
}

package impl

import (
	"crypto/sha256"
	"math"
	"math/big"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/types"
)

// PoWHeaderBuilder builds the byte-encoded header for hashing given a nonce and
// timestamp. Callers must ensure the encoding is deterministic and includes the
// nonce; timestamp can be used to refresh the header during mining.
type PoWHeaderBuilder func(nonce uint64, ts int64) []byte

var defaultTarget = new(big.Int).Lsh(big.NewInt(1), 240) // Easy default, overridden by config.
const (
	defaultTargetBlockTime = 10 * time.Second
	defaultMaxAdjustUp     = 4.0
	defaultMaxAdjustDown   = 0.1
)

// CheckWork returns true if the given header bytes hash below the target.
func CheckWork(header []byte, target *big.Int) bool {
	if len(header) == 0 {
		return false
	}
	t := effectiveTarget(target)
	if t.Sign() == 0 {
		return false
	}
	hash := sha256.Sum256(header)
	num := new(big.Int).SetBytes(hash[:])
	return num.Cmp(t) <= 0
}

// EncodeDifficulty renders the target as a fixed-width big-endian slice to be
// stored in the block header. Nil targets are encoded using the default target.
func EncodeDifficulty(target *big.Int) []byte {
	t := effectiveTarget(target)
	out := make([]byte, 32)
	b := t.Bytes()
	switch {
	case len(b) > len(out):
		// If the target exceeds 256 bits, encode the max 256-bit value so the
		// header represents the easiest possible target.
		for i := range out {
			out[i] = 0xFF
		}
	case len(b) == len(out):
		copy(out, b)
	default:
		copy(out[len(out)-len(b):], b)
	}
	return out
}

// DecodeDifficulty parses a big-endian target stored in the block header.
// Returns nil if the slice is empty.
func DecodeDifficulty(b []byte) *big.Int {
	if len(b) == 0 {
		return nil
	}
	return new(big.Int).SetBytes(b)
}

// AdjustDifficulty computes the next target given the previous target and the
// observed spacing between blocks. The adjustment is bounded to avoid large
// oscillations.
func AdjustDifficulty(
	prevTarget *big.Int,
	spacing time.Duration,
	targetBlockTime time.Duration,
	maxAdjustUp, maxAdjustDown float64,
) *big.Int {
	if prevTarget == nil {
		prevTarget = defaultTarget
	}
	if targetBlockTime <= 0 {
		targetBlockTime = defaultTargetBlockTime
	}
	if maxAdjustUp <= 0 {
		maxAdjustUp = defaultMaxAdjustUp
	}
	if maxAdjustDown <= 0 {
		maxAdjustDown = defaultMaxAdjustDown
	}

	// Normalize spacing; avoid zero/negative durations.
	if spacing <= 0 {
		spacing = time.Second
	}

	ratio := new(big.Rat).SetFloat64(float64(spacing) / float64(targetBlockTime))
	// Clamp ratio between maxAdjustDown (harder) and maxAdjustUp (easier).
	minFactor := maxAdjustDown
	maxFactor := maxAdjustUp
	if ratioNum, _ := ratio.Float64(); ratioNum < minFactor {
		ratio = new(big.Rat).SetFloat64(minFactor)
	} else if ratioNum > maxFactor {
		ratio = new(big.Rat).SetFloat64(maxFactor)
	}

	num := new(big.Rat).SetInt(prevTarget)
	num.Mul(num, ratio)

	out := new(big.Int).Div(num.Num(), num.Denom())
	if out.Sign() == 0 {
		out = big.NewInt(1)
	}
	return out
}

// MineNonce runs a simple PoW search by iterating nonces until the header hash
// is below the effective target or the stop channel is signaled. It returns the
// winning nonce, the timestamp used for that header, and a boolean indicating success.
func MineNonce(buildHeader PoWHeaderBuilder, cfg peer.PoWConfig, stop <-chan struct{}) (uint64, int64, bool) {
	if buildHeader == nil {
		return 0, 0, false
	}
	target := effectiveTarget(cfg.Target)
	if target.Sign() == 0 {
		return 0, 0, false
	}
	maxNonce := cfg.MaxNonce
	if maxNonce == 0 {
		maxNonce = math.MaxUint64
	}
	now := cfg.TimeSource
	if now == nil {
		now = time.Now
	}
	for nonce := uint64(0); nonce < maxNonce; nonce++ {
		// Allow the caller to abort quickly when the head changes.
		if stop != nil {
			select {
			case <-stop:
				return 0, 0, false
			default:
			}
		}
		ts := now().Unix()
		header := buildHeader(nonce, ts)
		hash := sha256.Sum256(header)
		if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
			return nonce, ts, true
		}
	}
	return 0, 0, false
}

func effectiveTarget(normal *big.Int) *big.Int {
	if normal != nil {
		return normal
	}
	return defaultTarget
}

func IsBlockComplexityValid(blk types.Block, target *big.Int) bool {
	// 1. PoW validation
	hash := blk.ComputeHash()

	t := effectiveTarget(target)

	return new(big.Int).SetBytes(hash[:]).Cmp(t) <= 0
}

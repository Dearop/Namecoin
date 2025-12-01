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

// MineNonce runs a simple PoW search by iterating nonces until the header hash
// is below the effective target or the stop channel is signaled. It returns the
// winning nonce, the corresponding hash, and a boolean indicating success.
func MineNonce(buildHeader PoWHeaderBuilder, cfg peer.PoWConfig, stop <-chan struct{}) (uint64, bool) {
	if buildHeader == nil {
		return 0, false
	}
	target := effectiveTarget(cfg.Target)
	if target.Sign() == 0 {
		return 0, false
	}
	maxNonce := cfg.MaxNonce
	if maxNonce == 0 {
		maxNonce = math.MaxUint64
	}
	now := cfg.TimeSource
	if now == nil {
		now = time.Now
	}
	var hash [32]byte
	for nonce := uint64(0); nonce < maxNonce; nonce++ {
		// Allow the caller to abort quickly when the head changes.
		if stop != nil {
			select {
			case <-stop:
				return 0, false
			default:
			}
		}
		header := buildHeader(nonce, now().Unix())
		hash = sha256.Sum256(header)
		if new(big.Int).SetBytes(hash[:]).Cmp(target) <= 0 {
			return nonce, true
		}
	}
	return 0, false
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
	if new(big.Int).SetBytes(hash[:]).Cmp(t) > 0 {
		return false
	}

	return true
}

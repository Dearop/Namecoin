package peer

import (
	"math/big"
	"time"
)

// PoWConfig holds the tunables for Namecoin Proof-of-Work. Difficulty
// is static per run; tests can override Target to use an easy value.
type PoWConfig struct {
	// Target is the difficulty target for the PoW (hash must be < Target).
	Target *big.Int
	// DisableDifficultyAdjustment turns off dynamic retargeting; the target stays
	// fixed at the configured value (useful for tests).
	DisableDifficultyAdjustment bool
	// TargetBlockTime controls the expected time between blocks for difficulty
	// retargeting. If zero, a default of 10s is used.
	TargetBlockTime time.Duration
	// MaxAdjustUp bounds how much easier the difficulty can become in one step
	// (as a multiplicative factor). If zero, a default of 4.0 is used.
	MaxAdjustUp float64
	// MaxAdjustDown bounds how much harder the difficulty can become in one step
	// (as a multiplicative factor). If zero, a default of 0.25 is used.
	MaxAdjustDown float64
	// MaxNonce bounds the search space; 0 means uint64 max.
	MaxNonce uint64
	// TimeSource lets tests inject a deterministic clock; defaults to time.Now.
	TimeSource func() time.Time
	PubKey     string
}

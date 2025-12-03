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
	// MaxNonce bounds the search space; 0 means uint64 max.
	MaxNonce uint64
	// TimeSource lets tests inject a deterministic clock; defaults to time.Now.
	TimeSource func() time.Time
	PubKey     string
}

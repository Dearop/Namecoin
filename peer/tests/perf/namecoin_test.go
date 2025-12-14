//go:build performance
// +build performance

package perf

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
)

func Test_Mining_Perf(t *testing.T) {
	runMiningPerf(t)
}

func runMiningPerf(t *testing.T) {
	transp := channelFac()

	node := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0",
		z.WithPoWConfig(peer.PoWConfig{
			Target: new(big.Int).Lsh(big.NewInt(1), 234),//this will change with Paul's Dynamic Difficulty
			PubKey: "miner",
		}),
		z.WithEnableMiner(true),
	)
	defer node.Stop()

	chain := node.NamecoinChainState()
	require.NotNil(t, chain)

	// Mine 10 blocks and record time for each
	targetBlocks := uint64(10)
	blockTimes := make([]time.Duration, 0, targetBlocks)

	var previousHeight uint64 = 0
	blockStart := time.Now()

	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		chain := node.NamecoinChainState()
		currentHeight := chain.HeadHeight()

		// New block mined
		if currentHeight > previousHeight {
			blockTime := time.Since(blockStart)
			blockTimes = append(blockTimes, blockTime)
			t.Logf("Block %d mined in %v", currentHeight, blockTime)

			previousHeight = currentHeight
			blockStart = time.Now()

			// Stop when we reach target
			if currentHeight >= targetBlocks {
				break
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	require.GreaterOrEqual(t, previousHeight, targetBlocks, "Failed to mine %d blocks", targetBlocks)

	// Log statistics
	var total time.Duration
	for _, bt := range blockTimes {
		total += bt
	}
	avgTime := total / time.Duration(len(blockTimes))
	t.Logf("Mined %d blocks, Average time per block: %v, Total time: %v", len(blockTimes), avgTime, total)
}

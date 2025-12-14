//go:build performance
// +build performance

package perf

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func Test_Mining_Perf(t *testing.T) {
	runMiningPerf(t)
}

func runMiningPerf(t *testing.T) {
	transp := channelFac()

	node := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0",
		z.WithPoWConfig(peer.PoWConfig{
			Target: new(big.Int).Lsh(big.NewInt(1), 234), //this will change with Paul's Dynamic Difficulty
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

func Test_Transaction_Throughput_Perf(t *testing.T) {
	testCases := []struct {
		name     string
		numTxs   int
		numNodes int
	}{
		{"100_txs_3_nodes", 100, 3},
		{"500_txs_3_nodes", 500, 3},
		//{"1000_txs_5_nodes", 1000, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runTransactionThroughput(t, tc.numTxs, tc.numNodes)
		})
	}
}

func runTransactionThroughput(t *testing.T, numTxs, numNodes int) {
	transp := channelFac()
	easyTarget := new(big.Int).Lsh(big.NewInt(1), 245)

	// Create nodes with mining enabled
	nodes := make([]z.TestNode, numNodes)
	for i := 0; i < numNodes; i++ {
		node := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0",
			z.WithPoWConfig(peer.PoWConfig{
				Target: easyTarget,
				PubKey: fmt.Sprintf("miner-%d", i),
			}),
			z.WithEnableMiner(true),
			z.WithAntiEntropy(400*time.Millisecond),
			z.WithHeartbeat(300*time.Millisecond),
		)
		nodes[i] = node
	}
	defer func() {
		for _, node := range nodes {
			node.Stop()
		}
	}()

	// Connect all nodes
	for _, n1 := range nodes {
		for _, n2 := range nodes {
			n1.AddPeer(n2.GetAddr())
		}
	}

	// Wait for initial mining to start
	time.Sleep(500 * time.Millisecond)

	// Track transaction submissions
	type txSubmission struct {
		txID       string
		submitTime time.Time
		domain     string
	}
	submissions := make([]txSubmission, 0, numTxs)
	var submissionMu sync.Mutex

	startTime := time.Now()

	// Submit transactions concurrently from multiple nodes
	var wg sync.WaitGroup
	txsPerNode := numTxs / numNodes
	remainder := numTxs % numNodes

	for nodeIdx := 0; nodeIdx < numNodes; nodeIdx++ {
		txCount := txsPerNode
		if nodeIdx < remainder {
			txCount++
		}

		wg.Add(1)
		go func(nIdx int, count int) {
			defer wg.Done()
			node := nodes[nIdx]

			for i := 0; i < count; i++ {
				domain := fmt.Sprintf("domain-%d-%d.bit", nIdx, i)
				salt := fmt.Sprintf("salt-%d-%d", nIdx, i)
				commitment := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt))

				// Create name_new transaction
				nameNew := impl.NameNew{
					Commitment: commitment,
					TTL:        impl.DefaultDomainTTLBlocks,
				}
				payload, err := json.Marshal(nameNew)
				require.NoError(t, err)

				minerID := node.GetMinerID()
				tx := types.Tx{
					From:    minerID,
					Type:    impl.NameNewCommandName,
					Amount:  1,
					Payload: json.RawMessage(payload),
					Outputs: []types.TxOutput{{To: minerID, Amount: 1}},
				}

				txID, err := impl.BuildTransactionID(&tx)
				require.NoError(t, err)

				// Broadcast transaction
				msg := types.NamecoinTransactionMessage{
					TxID: txID,
					Tx:   tx,
				}
				marshaled, err := node.GetRegistry().MarshalMessage(msg)
				require.NoError(t, err)

				submitTime := time.Now()
				err = node.Broadcast(marshaled)
				require.NoError(t, err)

				submissionMu.Lock()
				submissions = append(submissions, txSubmission{
					txID:       txID,
					submitTime: submitTime,
					domain:     domain,
				})
				submissionMu.Unlock()

				time.Sleep(5 * time.Millisecond)

			}
		}(nodeIdx, txCount)
	}

	wg.Wait()
	submissionEndTime := time.Now()
	submissionDuration := submissionEndTime.Sub(startTime)

	t.Logf("Submitted %d transactions in %v (%.2f tx/s)",
		len(submissions), submissionDuration, float64(len(submissions))/submissionDuration.Seconds())

	// Wait for transactions to be included in blocks
	t.Logf("Waiting for transactions to be included in blocks...")

	deadline := time.Now().Add(120 * time.Second)
	includedCount := 0
	checkInterval := 500 * time.Millisecond
	lastCheck := time.Now()

	for time.Now().Before(deadline) {
		// Check one node's chain state
		chain := nodes[0].NamecoinChainState()
		if chain == nil {
			time.Sleep(checkInterval)
			continue
		}

		state := chain.State()
		if state == nil {
			time.Sleep(checkInterval)
			continue
		}

		// Count how many transactions have been applied
		newIncludedCount := 0
		for _, sub := range submissions {
			if state.IsTxApplied(sub.txID) {
				newIncludedCount++
			}
		}

		if newIncludedCount > includedCount {
			includedCount = newIncludedCount
			t.Logf("Progress: %d/%d transactions included (%.1f%%)",
				includedCount, len(submissions), float64(includedCount)*100/float64(len(submissions)))
		}

		// Check if all transactions are included
		if includedCount >= len(submissions) {
			break
		}

		// Only check every checkInterval
		if time.Since(lastCheck) < checkInterval {
			time.Sleep(checkInterval - time.Since(lastCheck))
		}
		lastCheck = time.Now()
	}

	totalDuration := time.Now().Sub(startTime)

	// Calculate metrics
	t.Logf("\n=== Transaction Throughput Results ===")
	t.Logf("Total transactions: %d", len(submissions))
	t.Logf("Transactions included: %d (%.1f%%)", includedCount, float64(includedCount)*100/float64(len(submissions)))
	t.Logf("Submission rate: %.2f tx/s", float64(len(submissions))/submissionDuration.Seconds())
	t.Logf("Inclusion rate: %.2f tx/s", float64(includedCount)/totalDuration.Seconds())
	t.Logf("Total time: %v", totalDuration)

	if includedCount > 0 {
		avgLatency := totalDuration / time.Duration(includedCount)
		t.Logf("Average latency (submit to include): %v", avgLatency)
	}

	// Require at least 80% inclusion
	require.GreaterOrEqual(t, includedCount, int(float64(len(submissions))*0.8),
		"Expected at least 80%% of transactions to be included in blocks")
}

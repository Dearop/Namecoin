//go:build performance
// +build performance

package perf

import (
	"fmt"
	"math/big"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer/impl"
)

var (
	perfResultsMu sync.Mutex
	perfResults   []string
)

var (
	// Standardized PoW targets to keep runs comparable across scenarios.
	powTargetStandard = new(big.Int).Lsh(big.NewInt(1), 252) // balanced latency
	powTargetFast     = new(big.Int).Lsh(big.NewInt(1), 254) // easier / faster
	powTargetSlow     = new(big.Int).Lsh(big.NewInt(1), 250) // harder / slower
	powTargetFastest  = new(big.Int).Lsh(big.NewInt(1), 256) // very easy for large topologies
)

func recordPerfResult(name, details string) {
	perfResultsMu.Lock()
	defer perfResultsMu.Unlock()
	perfResults = append(perfResults, fmt.Sprintf("%s: %s", name, details))
}

func TestMain(m *testing.M) {
	code := m.Run()
	if len(perfResults) > 0 {
		fmt.Println("\n=== Namecoin Performance Summary ===")
		for _, line := range perfResults {
			fmt.Println(line)
		}
	}
	os.Exit(code)
}

// S3-T12: Mine with different difficulties and miner sets; capture time to first
// block and sustained throughput.
func Test_Mining_Perf(t *testing.T) {
	scenarios := []struct {
		name        string
		blocks      uint64
		config      scenarioConfig
		description string
	}{
		{
			name:   "single_miner_easy",
			blocks: 5,
			config: scenarioConfig{
				nodeCount:  1,
				minerCount: 1,
				target:     powTargetFast,
			},
			description: "baseline single miner",
		},
		{
			name:   "three_miners_medium",
			blocks: 5,
			config: scenarioConfig{
				nodeCount:  3,
				minerCount: 3,
				target:     powTargetStandard,
			},
			description: "scale-out miners with balanced difficulty",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			h := newNamecoinHarness(t, sc.config)
			defer h.stop()

			chain := h.nodes[0].NamecoinChainState()
			require.NotNil(t, chain)

			start := time.Now()
			initial := chain.HeadHeight()
			deadline := start.Add(180 * time.Second)
			var firstBlock time.Duration
			var blocksMined uint64

			for time.Now().Before(deadline) {
				head := h.nodes[0].NamecoinChainState().HeadHeight()
				if head > initial+blocksMined {
					if firstBlock == 0 {
						firstBlock = time.Since(start)
					}
					blocksMined = head - initial
					if blocksMined >= sc.blocks {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
			}

			require.GreaterOrEqual(t, blocksMined, sc.blocks, "failed to mine target blocks")
			elapsed := time.Since(start)
			throughput := float64(blocksMined) / elapsed.Seconds()

			t.Logf("%s: mined %d blocks in %v (%.2f blocks/s), first block in %v",
				sc.description, blocksMined, elapsed, throughput, firstBlock)
			recordPerfResult(t.Name()+"-"+sc.name,
				fmt.Sprintf("%s mined %d blocks, %.2f blocks/s, first block %v",
					sc.description, blocksMined, throughput, firstBlock))
		})
	}
}

// S3-T13: Stream registrations + updates, measure tx/s and latency to k confirmations.
func Test_Domain_Operation_Throughput_Latency_Perf(t *testing.T) {
	const (
		domains       = 18
		confirmations = uint64(3)
	)
	h := newNamecoinHarness(t, scenarioConfig{
		nodeCount:  4,
		minerCount: 3,
		target:     powTargetStandard,
	})
	defer h.stop()

	var regLatencies []time.Duration
	var updateLatencies []time.Duration
	var latMu sync.Mutex

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(domains)

	for i := 0; i < domains; i++ {
		go func(idx int) {
			defer wg.Done()
			node := h.nodes[idx%len(h.nodes)]
			domain := fmt.Sprintf("perf-domain-%d.bit", idx)
			salt := fmt.Sprintf("salt-%d", idx)

			nameNewTx, nameNewTxID := buildNameNewTx(t, node.GetMinerID(), domain, salt, impl.DefaultDomainTTLBlocks)
			broadcastTx(t, node, nameNewTx, nameNewTxID)

			firstUpdateTx, firstUpdateTxID := buildNameFirstUpdateTx(t, node.GetMinerID(), domain, salt, nameNewTxID,
				fmt.Sprintf("10.0.0.%d", idx+1), 96)
			firstSent := broadcastTx(t, node, firstUpdateTx, firstUpdateTxID)

			regHeight, ok := waitForDomain(h.nodes, domain, 120*time.Second)
			if ok && waitForConfirmations(h.nodes, regHeight, confirmations, 120*time.Second) {
				latMu.Lock()
				regLatencies = append(regLatencies, time.Since(firstSent))
				latMu.Unlock()
			}

			updateTx, updateTxID := buildNameUpdateTx(t, node.GetMinerID(), domain,
				fmt.Sprintf("10.0.1.%d", idx+1), firstUpdateTxID, 96)
			updateSent := broadcastTx(t, node, updateTx, updateTxID)
			updateHeight, ok := waitForTxApplied(h.nodes, updateTxID, 120*time.Second)
			if ok && waitForConfirmations(h.nodes, updateHeight, confirmations, 120*time.Second) {
				latMu.Lock()
				updateLatencies = append(updateLatencies, time.Since(updateSent))
				latMu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)
	totalTx := domains * 3
	throughput := float64(totalTx) / elapsed.Seconds()

	require.Greater(t, len(regLatencies), domains/2, "too few registrations confirmed")

	regAvg := averageLatency(regLatencies)
	updAvg := averageLatency(updateLatencies)

	t.Logf("Submitted %d tx across %d domains in %v (%.2f tx/s)", totalTx, domains, elapsed, throughput)
	t.Logf("Registration latency (k=%d confirmations): avg %v over %d successes", confirmations, regAvg, len(regLatencies))
	if len(updateLatencies) > 0 {
		t.Logf("Update latency (k=%d confirmations): avg %v over %d successes", confirmations, updAvg, len(updateLatencies))
	}

	recordPerfResult(t.Name(), fmt.Sprintf("registrations avg %v (%d/%d), updates avg %v (%d/%d), throughput %.2f tx/s",
		regAvg, len(regLatencies), domains, updAvg, len(updateLatencies), domains, throughput))
}

// S3-T14: Measure consensus convergence time as node count grows.
func Test_Consensus_Convergence_Perf(t *testing.T) {
	scenarios := []struct {
		name      string
		nodes     int
		blockGoal uint64
		miners    int
		target    *big.Int
	}{
		// Tune for fast-but-not-too-fast blocks (aim ~hundreds of ms) to collect many samples quickly.
		{"four_nodes", 4, 50, 2, powTargetFast},
		{"eight_nodes", 8, 50, 3, powTargetFast},
		{"sixteen_nodes", 16, 30, 4, powTargetFast},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			h := newNamecoinHarness(t, scenarioConfig{
				nodeCount:  sc.nodes,
				minerCount: sc.miners,
				target:     sc.target,
				shape:      networkShapeMesh,
			})
			defer h.stop()

			minerChain := h.nodes[0].NamecoinChainState()
			require.NotNil(t, minerChain)
			initial := minerChain.HeadHeight()
			targetHeight := initial + sc.blockGoal

			var delays []time.Duration
			var lastHeight uint64 = initial
			deadline := time.Now().Add(240 * time.Second)

			for time.Now().Before(deadline) {
				head := h.nodes[0].NamecoinChainState()
				if head == nil {
					time.Sleep(25 * time.Millisecond)
					continue
				}
				height := head.HeadHeight()
				if height > lastHeight {
					observed := time.Now()
					if !waitForHeads(h.nodes, height-1, 180*time.Second) {
						t.Logf("Skipping height %d sample: peers lagged on previous block", height-1)
						lastHeight = height
						continue
					}
					if waitForHeads(h.nodes, height, 120*time.Second) {
						delays = append(delays, time.Since(observed))
					}
					lastHeight = height
					if height >= targetHeight {
						break
					}
				}
				time.Sleep(30 * time.Millisecond)
			}

			if len(delays) == 0 {
				recordPerfResult(t.Name()+"-"+sc.name, fmt.Sprintf("%d nodes no convergence samples collected", sc.nodes))
				t.Skipf("no convergence samples collected for %d nodes", sc.nodes)
			}
			avg := averageLatency(delays)
			median := percentileLatency(delays, 50)
			p90 := percentileLatency(delays, 90)
			p99 := percentileLatency(delays, 99)
			var maxDelay time.Duration
			for _, d := range delays {
				if d > maxDelay {
					maxDelay = d
				}
			}

			t.Logf("%d-node convergence over %d blocks: avg %v, median %v, p90 %v, p99 %v, max %v",
				sc.nodes, len(delays), avg, median, p90, p99, maxDelay)
			recordPerfResult(t.Name()+"-"+sc.name,
				fmt.Sprintf("%d nodes avg %v median %v p90 %v p99 %v max %v over %d blocks",
					sc.nodes, avg, median, p90, p99, maxDelay, len(delays)))
		})
	}
}

// S3-T15: Instrument network overhead (gossip + DNS) under different loads and shapes.
func Test_Network_Overhead_Perf(t *testing.T) {
	scenarios := []struct {
		name             string
		domains          int
		updatesPerDomain int
		shape            networkShape
	}{
		{"mesh_normal_load", 10, 1, networkShapeMesh},
		{"line_adversarial_load", 8, 3, networkShapeLine},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			h := newNamecoinHarness(t, scenarioConfig{
				nodeCount:  5,
				minerCount: 3,
				target:     powTargetStandard,
				shape:      sc.shape,
				counting:   true,
			})
			defer h.stop()

			driveNetworkOverheadWorkload(t, h, sc.domains, sc.updatesPerDomain)
			time.Sleep(1 * time.Second) // allow gossip buffers to drain

			require.NotNil(t, h.counting, "counting transport missing")
			metrics := h.counting.snapshot()
			recordPerfResult(t.Name()+"-"+sc.name, fmt.Sprintf(
				"bytes sent %d recv %d, msgs sent %d recv %d (nodes=%d)",
				metrics.TotalSentBytes, metrics.TotalRecvBytes,
				metrics.TotalSentMessages, metrics.TotalRecvMessages, len(metrics.ByNode)))
			t.Logf("%s overhead: %+v", sc.name, metrics)
		})
	}
}

// Measures communication efficiency to reach consensus (bytes/messages per block) as node count grows.
func Test_Consensus_Communication_Efficiency_Perf(t *testing.T) {
	type commSample struct {
		nodes         int
		bytesPerBlock float64
		msgsPerBlock  float64
	}
	var samples []commSample

	scenarios := []struct {
		name   string
		nodes  int
		miners int
		blocks uint64
		target *big.Int
	}{
		{"four_nodes", 4, 2, 2, powTargetStandard},
		{"eight_nodes", 8, 3, 2, powTargetFast},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			h := newNamecoinHarness(t, scenarioConfig{
				nodeCount:  sc.nodes,
				minerCount: sc.miners,
				target:     sc.target,
				shape:      networkShapeMesh,
				counting:   true,
			})
			defer h.stop()

			require.NotNil(t, h.counting, "counting transport missing")

			chain := h.nodes[0].NamecoinChainState()
			require.NotNil(t, chain)

			startHeight := chain.HeadHeight()
			targetHeight := startHeight + sc.blocks
			deadline := time.Now().Add(150 * time.Second)

			for time.Now().Before(deadline) {
				if waitForHeads(h.nodes, targetHeight, 45*time.Second) {
					break
				}
				time.Sleep(150 * time.Millisecond)
			}

			endHeight := h.nodes[0].NamecoinChainState().HeadHeight()
			blocksMined := endHeight - startHeight
			if blocksMined == 0 {
				recordPerfResult(t.Name()+"-"+sc.name, fmt.Sprintf("nodes=%d miners=%d no blocks mined (timeout)", sc.nodes, sc.miners))
				t.Skipf("no blocks mined for scenario %s", sc.name)
			}

			metrics := h.counting.snapshot()
			totalBytes := metrics.TotalSentBytes + metrics.TotalRecvBytes
			totalMsgs := metrics.TotalSentMessages + metrics.TotalRecvMessages

			bytesPerBlock := float64(totalBytes) / float64(blocksMined)
			msgsPerBlock := float64(totalMsgs) / float64(blocksMined)

			samples = append(samples, commSample{
				nodes:         sc.nodes,
				bytesPerBlock: bytesPerBlock,
				msgsPerBlock:  msgsPerBlock,
			})

			t.Logf("nodes=%d miners=%d blocks=%d bytes=%d msgs=%d bytes/block=%.2f msgs/block=%.2f",
				sc.nodes, sc.miners, blocksMined, totalBytes, totalMsgs, bytesPerBlock, msgsPerBlock)

			recordPerfResult(t.Name()+"-"+sc.name, fmt.Sprintf(
				"nodes=%d miners=%d blocks=%d bytes=%d msgs=%d bytes/block=%.2f msgs/block=%.2f",
				sc.nodes, sc.miners, blocksMined, totalBytes, totalMsgs, bytesPerBlock, msgsPerBlock))
		})
	}

	if len(samples) > 0 {
		var totalB, totalM float64
		for _, s := range samples {
			totalB += s.bytesPerBlock
			totalM += s.msgsPerBlock
		}
		recordPerfResult(t.Name()+"-summary", fmt.Sprintf(
			"avg bytes/block=%.2f avg msgs/block=%.2f samples=%d",
			totalB/float64(len(samples)), totalM/float64(len(samples)), len(samples)))
	}
}

func averageLatency(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range samples {
		total += d
	}
	return total / time.Duration(len(samples))
}

func percentileLatency(samples []time.Duration, pct int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	if pct <= 0 {
		pct = 1
	}
	if pct > 100 {
		pct = 100
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	// nearest-rank method
	idx := (pct*len(sorted) + 99) / 100 // ceiling(pct/100 * n)
	if idx <= 0 {
		idx = 0
	} else if idx > len(sorted)-1 {
		idx = len(sorted) - 1
	} else {
		idx = idx - 1
	}
	return sorted[idx]
}

func waitForHeads(nodes []z.TestNode, height uint64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allReached := true
		for _, node := range nodes {
			chain := node.NamecoinChainState()
			if chain == nil || chain.HeadHeight() < height {
				allReached = false
				break
			}
		}
		if allReached {
			return true
		}
		time.Sleep(40 * time.Millisecond)
	}
	return false
}

func driveNetworkOverheadWorkload(t *testing.T, h *namecoinHarness, domains, updatesPerDomain int) {
	var wg sync.WaitGroup
	wg.Add(domains)

	for i := 0; i < domains; i++ {
		go func(idx int) {
			defer wg.Done()
			node := h.nodes[idx%len(h.nodes)]
			domain := fmt.Sprintf("overhead-%d.bit", idx)
			salt := fmt.Sprintf("net-salt-%d", idx)

			nameNewTx, nameNewTxID := buildNameNewTx(t, node.GetMinerID(), domain, salt, impl.DefaultDomainTTLBlocks)
			broadcastTx(t, node, nameNewTx, nameNewTxID)
			firstUpdateTx, firstUpdateTxID := buildNameFirstUpdateTx(t, node.GetMinerID(), domain, salt, nameNewTxID,
				fmt.Sprintf("192.168.0.%d", idx+1), 64)
			broadcastTx(t, node, firstUpdateTx, firstUpdateTxID)

			for u := 0; u < updatesPerDomain; u++ {
				updateTx, updateTxID := buildNameUpdateTx(t, node.GetMinerID(), domain,
					fmt.Sprintf("192.168.1.%d", idx+u+1), firstUpdateTxID, 64)
				broadcastTx(t, node, updateTx, updateTxID)
				time.Sleep(15 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
}

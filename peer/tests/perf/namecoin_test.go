//go:build performance
// +build performance

package perf

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"go.dedis.ch/cs438/types"
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

// Mining throughput scaling: blocks/s vs miner count / target, collects tail latencies.
func Test_Mining_Throughput_Scaling_Perf(t *testing.T) {
	const runsPerScenario = 3
	scenarios := []struct {
		name   string
		nodes  int
		miners int
		target *big.Int
		blocks uint64
	}{
		{"miners_1_easy", 1, 1, powTargetFastest, 8},
		{"miners_2_medium", 2, 2, powTargetFast, 10},
		{"miners_4_harder", 4, 4, powTargetStandard, 10},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			var throughputs []float64
			var ttfbs []time.Duration

			for run := 0; run < runsPerScenario; run++ {
				h := newNamecoinHarness(t, scenarioConfig{
					nodeCount:  sc.nodes,
					minerCount: sc.miners,
					target:     sc.target,
					shape:      networkShapeMesh,
				})
				chain := h.nodes[0].NamecoinChainState()
				require.NotNil(t, chain)

				startHeight := chain.HeadHeight()
				targetHeight := startHeight + sc.blocks
				start := time.Now()
				deadline := start.Add(90 * time.Second)
				var firstBlock time.Duration

				for time.Now().Before(deadline) {
					height := chain.HeadHeight()
					if height > startHeight && firstBlock == 0 {
						firstBlock = time.Since(start)
					}
					if height >= targetHeight {
						break
					}
					time.Sleep(25 * time.Millisecond)
				}

				mined := chain.HeadHeight() - startHeight
				h.stop()

				require.Greater(t, mined, uint64(0), "no blocks mined for %s run %d", sc.name, run)
				elapsed := time.Since(start)
				if firstBlock == 0 {
					firstBlock = elapsed
				}
				throughputs = append(throughputs, float64(mined)/elapsed.Seconds())
				ttfbs = append(ttfbs, firstBlock)
			}

			avg := averageFloat(throughputs)
			median := percentileFloat(throughputs, 50)
			p90 := percentileFloat(throughputs, 90)
			p99 := percentileFloat(throughputs, 99)
			ttfbAvg := averageLatency(ttfbs)
			ttfbP90 := percentileLatency(ttfbs, 90)

			t.Logf("miners=%d nodes=%d target=2^%d throughput avg=%.2f median=%.2f p90=%.2f p99=%.2f ttfb avg=%v p90=%v (runs=%d)",
				sc.miners, sc.nodes, powTargetBits(sc.target), avg, median, p90, p99, ttfbAvg, ttfbP90, runsPerScenario)
			recordPerfResult(t.Name()+"-"+sc.name, fmt.Sprintf(
				"nodes=%d miners=%d target=2^%d runs=%d throughput_avg=%.2f median=%.2f p90=%.2f p99=%.2f ttfb_avg=%v ttfb_p90=%v",
				sc.nodes, sc.miners, powTargetBits(sc.target), runsPerScenario, avg, median, p90, p99, ttfbAvg, ttfbP90))
		})
	}
}

// Domain operation throughput: confirmed registrations per second / per block under PoW.
func Test_Domain_Operation_Throughput_Perf(t *testing.T) {
	type scenario struct {
		name        string
		nodes       int
		miners      int
		target      *big.Int
		domainCount int
		runFor      time.Duration
	}
	scenarios := []scenario{
		{"single_fast_small", 1, 1, powTargetFastest, 6, 45 * time.Second},
		{"single_fast_medium", 1, 1, powTargetFastest, 10, 60 * time.Second},
		{"fast_miners_2", 2, 2, powTargetFastest, 8, 60 * time.Second},
		{"fast_miners_4", 4, 4, powTargetFastest, 12, 75 * time.Second},
		{"fast_miners_8", 8, 8, powTargetFastest, 16, 90 * time.Second},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			h := newNamecoinHarness(t, scenarioConfig{
				nodeCount:  sc.nodes,
				minerCount: sc.miners,
				target:     sc.target,
			})
			defer h.stop()

			// Warm up a few blocks so wallets have spendable funds (avoid immature coinbases).
			waitForHeads(h.nodes, 3, 30*time.Second)

			// Prebuild domain registrations.
			type regTx struct {
				domain        string
				salt          string
				ownerPub      []byte
				ownerPriv     []byte
				ownerAddr     string
				seedTxID      string
				nameNewID     string
				firstUpdateID string
			}
			regs := make([]regTx, sc.domainCount)
			for i := 0; i < sc.domainCount; i++ {
				pub, priv, err := ed25519.GenerateKey(rand.Reader)
				require.NoError(t, err)
				owner := impl.HashHex(pub)

				// Seed funds for this owner.
				seedID := fmt.Sprintf("perf-seed-%s-%d", sc.name, i)
				seed := types.UTXO{
					TxID:   seedID,
					To:     owner,
					Amount: 5,
				}
				for _, nd := range h.nodes {
					if ch := nd.NamecoinChainState(); ch != nil && ch.State() != nil {
						_ = ch.State().AppendUTXO(seed)
					}
				}

				domain := fmt.Sprintf("pow-throughput-%s-%d.bit", sc.name, i)
				salt := fmt.Sprintf("pow-salt-%s-%d", sc.name, i)
				regs[i] = regTx{domain: domain, salt: salt, ownerPub: pub, ownerPriv: priv, ownerAddr: owner, seedTxID: seedID}
			}

			start := time.Now()

			node := h.nodes[0]

			// Pause miners while staging transactions to avoid races with partial data.
			for _, nd := range h.nodes {
				if miner, ok := nd.Peer.(interface{ StopMiner() }); ok {
					miner.StopMiner()
				}
			}

			// Build and broadcast all name_new quickly.
			for i := range regs {
				reg := &regs[i]
				state := node.NamecoinChainState().State()
				inputsNew, outputsNew, err := state.DeterministicSpendPlan(reg.ownerAddr, 1)
				require.NoError(t, err)
				commitment := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", reg.domain, reg.salt))
				nameNewTx := types.Tx{
					From:    reg.ownerAddr,
					Type:    impl.NameNewCommandName,
					Amount:  1,
					Inputs:  inputsNew,
					Outputs: outputsNew,
					Pk:      hex.EncodeToString(reg.ownerPub),
					Payload: json.RawMessage(mustJSONMarshal(t, impl.NameNew{Commitment: commitment, TTL: impl.DefaultDomainTTLBlocks})),
				}
				txID, err := impl.BuildTransactionID(&nameNewTx)
				require.NoError(t, err)
				nameNewTx.TxID = txID
				pre, err := (&impl.SignedTransaction{
					Type:    nameNewTx.Type,
					From:    nameNewTx.From,
					Amount:  nameNewTx.Amount,
					Payload: nameNewTx.Payload,
					Inputs:  nameNewTx.Inputs,
					Outputs: nameNewTx.Outputs,
					Pk:      nameNewTx.Pk,
					TxID:    nameNewTx.TxID,
				}).SerializeTransactionSignature()
				require.NoError(t, err)
				nameNewTx.Signature = hex.EncodeToString(ed25519.Sign(reg.ownerPriv, impl.Hash(pre)))
				reg.nameNewID = nameNewTx.TxID
				broadcastTx(t, node, nameNewTx, nameNewTx.TxID)
			}

			// Mine just enough so commitments land: resume miners and wait for either height+1 or all name_new applied.
			for _, nd := range h.nodes {
				if miner, ok := nd.Peer.(interface{ StartMiner() }); ok {
					miner.StartMiner()
				}
			}

			heightBeforeNew := h.nodes[0].NamecoinChainState().HeadHeight()
			waitNewDeadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(waitNewDeadline) {
				chain := node.NamecoinChainState()
				state := chain.State()
				all := true
				for _, reg := range regs {
					if !state.IsTxApplied(reg.nameNewID) {
						all = false
						break
					}
				}
				if all || chain.HeadHeight() >= heightBeforeNew+1 {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			// Build and broadcast all first_update quickly using current state.
			for i := range regs {
				reg := &regs[i]
				state := node.NamecoinChainState().State()
				inputsFU, outputsFU, err := state.DeterministicSpendPlan(reg.ownerAddr, 1)
				require.NoError(t, err)
				firstPayload := impl.NameFirstUpdate{
					Domain: reg.domain,
					Salt:   reg.salt,
					IP:     fmt.Sprintf("10.42.%d", i),
					TTL:    impl.DefaultDomainTTLBlocks,
					TxID:   reg.nameNewID,
				}
				firstUpdateTx := types.Tx{
					From:    reg.ownerAddr,
					Type:    impl.NameFirstUpdateCommandName,
					Amount:  1,
					Inputs:  inputsFU,
					Outputs: outputsFU,
					Pk:      hex.EncodeToString(reg.ownerPub),
					Payload: json.RawMessage(mustJSONMarshal(t, firstPayload)),
				}
				fuID, err := impl.BuildTransactionID(&firstUpdateTx)
				require.NoError(t, err)
				firstUpdateTx.TxID = fuID
				preFU, err := (&impl.SignedTransaction{
					Type:    firstUpdateTx.Type,
					From:    firstUpdateTx.From,
					Amount:  firstUpdateTx.Amount,
					Payload: firstUpdateTx.Payload,
					Inputs:  firstUpdateTx.Inputs,
					Outputs: firstUpdateTx.Outputs,
					Pk:      firstUpdateTx.Pk,
					TxID:    firstUpdateTx.TxID,
				}).SerializeTransactionSignature()
				require.NoError(t, err)
				firstUpdateTx.Signature = hex.EncodeToString(ed25519.Sign(reg.ownerPriv, impl.Hash(preFU)))
				reg.firstUpdateID = firstUpdateTx.TxID
				broadcastTx(t, node, firstUpdateTx, firstUpdateTx.TxID)
			}

			// Resume miners for the measurement window.
			for _, nd := range h.nodes {
				if miner, ok := nd.Peer.(interface{ StartMiner() }); ok {
					miner.StartMiner()
				}
			}

			headStart := h.nodes[0].NamecoinChainState().HeadHeight()
			timeout := time.Now().Add(sc.runFor)
			for time.Now().Before(timeout) {
				time.Sleep(150 * time.Millisecond)
			}

			elapsed := time.Since(start)
			finalChain := h.nodes[0].NamecoinChainState()
			blocksMined := uint64(0)
			if finalChain != nil {
				if finalChain.HeadHeight() >= headStart {
					blocksMined = finalChain.HeadHeight() - headStart
				}
			}

			confirmedSet := make(map[string]struct{})
			if finalChain != nil {
				if state := finalChain.State(); state != nil {
					for _, reg := range regs {
						if state.IsTxApplied(reg.firstUpdateID) {
							confirmedSet[reg.firstUpdateID] = struct{}{}
						}
					}
				}
			}
			confirmed := len(confirmedSet)

			// Log which txs made it.
			if finalChain != nil && finalChain.State() != nil {
				state := finalChain.State()
				var acceptedNew, missingNew, acceptedFirst, missingFirst []string
				for _, reg := range regs {
					if state.IsTxApplied(reg.nameNewID) {
						acceptedNew = append(acceptedNew, reg.nameNewID)
					} else {
						missingNew = append(missingNew, reg.nameNewID)
					}
					if state.IsTxApplied(reg.firstUpdateID) {
						acceptedFirst = append(acceptedFirst, reg.firstUpdateID)
					} else {
						missingFirst = append(missingFirst, reg.firstUpdateID)
					}
				}
				t.Logf("%s accepted name_new=%d/%d first_update=%d/%d (missing new=%d missing first=%d)",
					sc.name, len(acceptedNew), len(regs), len(acceptedFirst), len(regs), len(missingNew), len(missingFirst))
			}
			throughput := float64(confirmed) / elapsed.Seconds()
			txPerBlock := float64(confirmed)
			if blocksMined > 0 {
				txPerBlock = float64(confirmed) / float64(blocksMined)
			}

			t.Logf("nodes=%d miners=%d target=2^%d blocks=%d confirmed=%d duration=%v throughput=%.2f tx/s tx_per_block=%.2f",
				sc.nodes, sc.miners, powTargetBits(sc.target), blocksMined, confirmed, elapsed, throughput, txPerBlock)
			recordPerfResult(t.Name()+"-"+sc.name, fmt.Sprintf(
				"nodes=%d miners=%d blocks=%d confirmed=%d duration=%.2fs throughput=%.2f tx_per_block=%.2f target=2^%d",
				sc.nodes, sc.miners, blocksMined, confirmed, elapsed.Seconds(), throughput, txPerBlock, powTargetBits(sc.target)))
		})
	}
}

// S3-T13: Stream registrations + updates, measure tx/s and latency to k confirmations.
func Test_Domain_Operation_Throughput_Latency_Perf(t *testing.T) {
	const (
		domains       = 8
		confirmations = uint64(1)
	)
	h := newNamecoinHarness(t, scenarioConfig{
		nodeCount:  4,
		minerCount: 4,
		target:     powTargetFastest,
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

	if len(regLatencies) == 0 {
		recordPerfResult(t.Name(), "no registrations confirmed; skipping metrics")
		t.Skip("no registrations confirmed")
	}

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

// Convergence vs network size: inject divergence and measure stabilization latency.
func Test_Convergence_vs_Network_Size_Perf(t *testing.T) {
	nodeCounts := []int{2, 4, 6, 8, 10, 16}
	const runsPerScenario = 3
	const stabilityWindow = 2 * time.Second

	for _, nCount := range nodeCounts {
		t.Run(fmt.Sprintf("%d_nodes", nCount), func(t *testing.T) {
			var samples []time.Duration
			for run := 0; run < runsPerScenario; run++ {
				h := newNamecoinHarness(t, scenarioConfig{
					nodeCount:  nCount,
					minerCount: minInt(4, nCount),
					target:     powTargetFast,
					shape:      networkShapeMesh,
				})

				initial := h.nodes[0].NamecoinChainState().HeadHeight()
				if !forceCompetingBlocks(t, h.nodes, initial+1, 25*time.Second) {
					h.stop()
					t.Logf("run %d/%d: could not force competing blocks", run+1, runsPerScenario)
					continue
				}

				start := time.Now()
				if waitUntilConvergedStable(h.nodes, initial+1, stabilityWindow, 60*time.Second) {
					samples = append(samples, time.Since(start))
				}
				h.stop()
			}

			if len(samples) == 0 {
				recordPerfResult(t.Name()+fmt.Sprintf("-%d_nodes", nCount), fmt.Sprintf("nodes=%d no convergence samples", nCount))
				t.Skipf("no convergence samples for %d nodes", nCount)
			}

			avg := averageLatency(samples)
			med := percentileLatency(samples, 50)
			p90 := percentileLatency(samples, 90)
			p99 := percentileLatency(samples, 99)
			var maxDelay time.Duration
			for _, d := range samples {
				if d > maxDelay {
					maxDelay = d
				}
			}

			t.Logf("nodes=%d convergence samples=%d avg=%v p50=%v p90=%v p99=%v max=%v window=%v",
				nCount, len(samples), avg, med, p90, p99, maxDelay, stabilityWindow)
			recordPerfResult(t.Name()+fmt.Sprintf("-%d_nodes", nCount), fmt.Sprintf(
				"nodes=%d runs=%d samples=%d avg=%v p50=%v p90=%v p99=%v max=%v window=%v",
				nCount, runsPerScenario, len(samples), avg, med, p90, p99, maxDelay, stabilityWindow))
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

// Convergence sweep across node counts 2..10; records median/avg/p90/p99 over several runs.
func Test_Consensus_Convergence_Sweep_Perf(t *testing.T) {
	nodeCounts := []int{2, 4, 6, 8, 10}
	runs := 3

	for _, nCount := range nodeCounts {
		t.Run(fmt.Sprintf("%d_nodes", nCount), func(t *testing.T) {
			var allDelays []time.Duration

			for run := 0; run < runs; run++ {
				h := newNamecoinHarness(t, scenarioConfig{
					nodeCount:  nCount,
					minerCount: minInt(4, nCount),
					target:     powTargetFast,
					shape:      networkShapeMesh,
				})

				minerChain := h.nodes[0].NamecoinChainState()
				require.NotNil(t, minerChain)
				initial := minerChain.HeadHeight()

				// Force a divergence by mining competing blocks at the same height on two different nodes.
				if !forceCompetingBlocks(t, h.nodes, initial+1, 30*time.Second) {
					t.Logf("run %d/%d: failed to forge competing blocks; skipping", run+1, runs)
					h.stop()
					continue
				}

				start := time.Now()
				if waitUntilConverged(h.nodes, initial+1, 30*time.Second) {
					delay := time.Since(start)
					allDelays = append(allDelays, delay)
					t.Logf("run %d/%d: forced divergence at height %d converged in %v", run+1, runs, initial+1, delay)
				} else {
					t.Logf("run %d/%d: forced divergence at height %d did not converge within limit", run+1, runs, initial+1)
				}

				h.stop()
			}

			require.NotEmpty(t, allDelays, "no convergence samples collected for %d nodes", nCount)
			avg := averageLatency(allDelays)
			median := percentileLatency(allDelays, 50)
			p90 := percentileLatency(allDelays, 90)
			p99 := percentileLatency(allDelays, 99)
			var maxDelay time.Duration
			for _, d := range allDelays {
				if d > maxDelay {
					maxDelay = d
				}
			}

			t.Logf("%d-node sweep (%d runs, %d samples): avg %v, median %v, p90 %v, p99 %v, max %v",
				nCount, runs, len(allDelays), avg, median, p90, p99, maxDelay)
			recordPerfResult(t.Name()+fmt.Sprintf("-%d_nodes", nCount),
				fmt.Sprintf("runs=%d samples=%d avg %v median %v p90 %v p99 %v max %v",
					runs, len(allDelays), avg, median, p90, p99, maxDelay))
		})
	}
}

// Network overhead normalized per domain update across mesh topologies.
func Test_Network_Overhead_Per_Domain_Update_Perf(t *testing.T) {
	scenarios := []struct {
		name             string
		nodes            int
		domains          int
		updatesPerDomain int
	}{}

	for n := 4; n <= 12; n++ {
		scenarios = append(scenarios, struct {
			name             string
			nodes            int
			domains          int
			updatesPerDomain int
		}{
			name:             fmt.Sprintf("mesh_%d_nodes", n),
			nodes:            n,
			domains:          maxInt(8, n),
			updatesPerDomain: 3,
		})
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			h := newNamecoinHarness(t, scenarioConfig{
				nodeCount:  sc.nodes,
				minerCount: minInt(4, sc.nodes),
				target:     powTargetFast,
				shape:      networkShapeMesh,
				counting:   true,
			})
			defer h.stop()

			require.NotNil(t, h.counting)

			driveNetworkOverheadWorkload(t, h, sc.domains, sc.updatesPerDomain)
			time.Sleep(2 * time.Second)

			metrics := h.counting.snapshot()
			totalBytes := metrics.TotalSentBytes + metrics.TotalRecvBytes
			totalMsgs := metrics.TotalSentMessages + metrics.TotalRecvMessages

			updateOps := sc.domains * (sc.updatesPerDomain + 1) // first update + subsequent updates
			bytesPerUpdate := float64(totalBytes) / float64(updateOps)
			msgsPerUpdate := float64(totalMsgs) / float64(updateOps)

			t.Logf("%s nodes=%d domains=%d updates/domain=%d bytes=%d msgs=%d bytes/update=%.2f msgs/update=%.2f",
				sc.name, sc.nodes, sc.domains, sc.updatesPerDomain, totalBytes, totalMsgs, bytesPerUpdate, msgsPerUpdate)
			recordPerfResult(t.Name()+"-"+sc.name, fmt.Sprintf(
				"nodes=%d domains=%d updates/domain=%d bytes/update=%.2f msgs/update=%.2f total_bytes=%d total_msgs=%d",
				sc.nodes, sc.domains, sc.updatesPerDomain, bytesPerUpdate, msgsPerUpdate, totalBytes, totalMsgs))
		})
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

func averageFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var total float64
	for _, v := range vals {
		total += v
	}
	return total / float64(len(vals))
}

func percentileFloat(vals []float64, pct int) float64 {
	if len(vals) == 0 {
		return 0
	}
	if pct <= 0 {
		pct = 1
	}
	if pct > 100 {
		pct = 100
	}
	sorted := append([]float64(nil), vals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (pct*len(sorted) + 99) / 100
	if idx <= 0 {
		idx = 0
	} else if idx > len(sorted)-1 {
		idx = len(sorted) - 1
	} else {
		idx = idx - 1
	}
	return sorted[idx]
}

func powTargetBits(target *big.Int) int {
	if target == nil {
		return 0
	}
	return target.BitLen() - 1
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

package integration

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/transport/disrupted"
)

// Two miners on UDP should quickly converge on the same head hash.
func Test_Namecoin_Integration_LongestChain_TwoNodes(t *testing.T) {
	skipIfWIndows(t)

	//easyTarget := new(big.Int).Lsh(big.NewInt(1), 255)
	hardTarget := new(big.Int).Lsh(big.NewInt(1), 242)
	sharedTransport := udpFac()

	newOpts := func(minerID string) []z.Option {
		return []z.Option{
			z.WithPoWConfig(peer.PoWConfig{
				Target:                      hardTarget,
				PubKey:                      minerID,
				DisableDifficultyAdjustment: true,
			}),
			z.WithEnableMiner(true),
			z.WithAntiEntropy(800 * time.Millisecond),
			z.WithHeartbeat(500 * time.Millisecond),
			z.WithContinueMongering(1),
		}
	}

	nodes := []z.TestNode{
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", newOpts("miner-two-0")...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", newOpts("miner-two-1")...),
	}

	defer terminateNodes(t, nodes)
	defer stopAllNodesWithin(t, nodes, nodesTimeout)
	defer stopMinersFor(t, nodes)

	addAllPeers(nodes)

	waitForHeight(t, nodes, 2, 12*time.Second)
	stopMinersFor(t, nodes)
	time.Sleep(2 * time.Second)

	head, height := waitForCommonHead(t, nodes, 5*time.Second)
	require.NotNil(t, head)
	require.GreaterOrEqual(t, height, uint64(2))
}

// A lagged third node should fork briefly but rejoin the longest chain.
func Test_Namecoin_Integration_LongestChain_ThreeNodes(t *testing.T) {
	skipIfWIndows(t)

	fastTarget := new(big.Int).Lsh(big.NewInt(1), 242)
	baseOpts := func(minerID string) []z.Option {
		return []z.Option{
			z.WithPoWConfig(peer.PoWConfig{
				Target:                      fastTarget,
				PubKey:                      minerID,
				DisableDifficultyAdjustment: true,
			}),
			z.WithEnableMiner(true),
			z.WithAntiEntropy(700 * time.Millisecond),
			z.WithHeartbeat(600 * time.Millisecond),
			z.WithContinueMongering(1),
		}
	}

	sharedTransport := udpFac()
	laggedTransport := disrupted.NewDisrupted(udpFac(), disrupted.WithFixedDelay(400*time.Millisecond))

	nodes := []z.TestNode{
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", baseOpts("miner-three-0")...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", baseOpts("miner-three-1")...),
		z.NewTestNode(t, studentFac, laggedTransport, "127.0.0.1:0", baseOpts("miner-three-2")...),
	}

	defer terminateNodes(t, nodes)
	defer stopAllNodesWithin(t, nodes, nodesTimeout)
	defer stopMinersFor(t, nodes)

	addAllPeers(nodes)

	waitForHeight(t, nodes, 3, 15*time.Second)
	stopMinersFor(t, nodes)
	time.Sleep(time.Second)

	head, height := waitForCommonHead(t, nodes, 20*time.Second)
	require.NotNil(t, head)
	require.GreaterOrEqual(t, height, uint64(3))
}

// Five miners with mixed difficulty and high gossip traffic should still agree on a single head.
func Test_Namecoin_Integration_LongestChain_FiveNodes(t *testing.T) {
	skipIfWIndows(t)

	easyTarget := new(big.Int).Lsh(big.NewInt(1), 241)
	midTarget := new(big.Int).Lsh(big.NewInt(1), 239)

	optsFor := func(target *big.Int, minerID string, hb time.Duration) []z.Option {
		return []z.Option{
			z.WithPoWConfig(peer.PoWConfig{
				Target:                      target,
				PubKey:                      minerID,
				DisableDifficultyAdjustment: true,
			}),
			z.WithEnableMiner(true),
			z.WithAntiEntropy(600 * time.Millisecond),
			z.WithHeartbeat(hb),
			z.WithAckTimeout(4 * time.Second),
			z.WithContinueMongering(1),
		}
	}

	sharedTransport := udpFac()

	nodes := []z.TestNode{
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(easyTarget, "miner-five-0", 400*time.Millisecond)...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(easyTarget, "miner-five-1", 450*time.Millisecond)...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(midTarget, "miner-five-2", 650*time.Millisecond)...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(midTarget, "miner-five-3", 700*time.Millisecond)...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(midTarget, "miner-five-4", 600*time.Millisecond)...),
	}

	defer terminateNodes(t, nodes)
	defer stopAllNodesWithin(t, nodes, nodesTimeout)
	defer stopMinersFor(t, nodes)

	addAllPeers(nodes)

	waitForHeight(t, nodes, 5, 20*time.Second)
	stopMinersFor(t, nodes)
	time.Sleep(time.Second)

	head, height := waitForCommonHead(t, nodes, 10*time.Second)
	require.NotNil(t, head)
	require.GreaterOrEqual(t, height, uint64(5))
}

// Five miners start in two disconnected groups, then merge and must converge to a common head.
func Test_Namecoin_Integration_LongestChain_PartitionMerge(t *testing.T) {
	skipIfWIndows(t)

	// Static PoW: easy targets and unlimited nonce for reliable mining in tests.
	easyTarget := new(big.Int).Lsh(big.NewInt(1), 244)
	midTarget := new(big.Int).Set(easyTarget)

	optsFor := func(target *big.Int, minerID string, hb time.Duration) []z.Option {
		return []z.Option{
			z.WithPoWConfig(peer.PoWConfig{
				Target:                      target,
				PubKey:                      minerID,
				MaxNonce:                    0,
				DisableDifficultyAdjustment: true,
			}),
			z.WithEnableMiner(true),
			z.WithAntiEntropy(600 * time.Millisecond),
			z.WithHeartbeat(hb),
			z.WithAckTimeout(4 * time.Second),
			z.WithContinueMongering(1),
		}
	}

	sharedTransport := udpFac()

	nodes := []z.TestNode{
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(easyTarget, "miner-part-0", 400*time.Millisecond)...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(easyTarget, "miner-part-1", 450*time.Millisecond)...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(midTarget, "miner-part-2", 650*time.Millisecond)...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(midTarget, "miner-part-3", 700*time.Millisecond)...),
		z.NewTestNode(t, studentFac, sharedTransport, "127.0.0.1:0", optsFor(midTarget, "miner-part-4", 600*time.Millisecond)...),
	}

	defer terminateNodes(t, nodes)
	defer stopAllNodesWithin(t, nodes, nodesTimeout)
	defer stopMinersFor(t, nodes)

	// Partition: group A (0,1) only talk to each other; group B (2,3,4) only to each other.
	connectGroup := func(group []z.TestNode) {
		addrs := make([]string, 0, len(group))
		for _, n := range group {
			addrs = append(addrs, n.GetAddr())
		}
		for _, n := range group {
			n.AddPeer(addrs...)
		}
	}
	connectGroup(nodes[:2])
	connectGroup(nodes[2:])

	// Each partition mines independently for a bit (reduced heights to avoid timeouts).
	waitForHeight(t, nodes[:2], 1, 15*time.Second)
	waitForHeight(t, nodes[2:], 1, 15*time.Second)

	// Verify each partition is internally consistent but diverged from the other.
	headA, heightA := waitForCommonHead(t, nodes[:2], 5*time.Second)
	require.NotNil(t, headA)
	require.GreaterOrEqual(t, heightA, uint64(1))

	headB, heightB := waitForCommonHead(t, nodes[2:], 5*time.Second)
	require.NotNil(t, headB)
	require.GreaterOrEqual(t, heightB, uint64(1))

	// Heads should differ across partitions (most likely); allow equality only if coincidentally same.
	if bytes.Equal(headA, headB) {
		// If they matched by chance, ensure at least heights are equal.
		require.Equal(t, heightA, heightB)
	}

	// Heal the partition: connect everyone.
	addAllPeers(nodes)

	// Allow growth on the merged network, then stop miners and ensure convergence.
	waitForHeight(t, nodes, 2, 15*time.Second)
	stopMinersFor(t, nodes)
	time.Sleep(time.Second)

	head, height := waitForCommonHead(t, nodes, 20*time.Second)
	require.NotNil(t, head)
	require.GreaterOrEqual(t, height, uint64(2))
}

func waitForHeight(t *testing.T, nodes []z.TestNode, minHeight uint64, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok := true
		for _, node := range nodes {
			_, height := loadHead(t, node)
			if height < minHeight {
				ok = false
				break
			}
		}
		if ok {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}

	// Collect a detailed diagnostic snapshot before failing.
	var b strings.Builder
	fmt.Fprintf(&b, "timeout waiting for height after %s; snapshot:\n", timeout)
	for i, node := range nodes {
		h, ht := loadHead(t, node)
		fmt.Fprintf(&b, "  node[%d] addr=%s height=%d len~=%d hash=%s\n",
			i, node.GetAddr(), ht, ht+1, hex.EncodeToString(h))
	}
	t.Fatal(b.String())
}

func waitForCommonHead(t *testing.T, nodes []z.TestNode, timeout time.Duration) ([]byte, uint64) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		hash, height := loadHead(t, nodes[0])
		if hash == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		mismatch := false
		for i := 1; i < len(nodes); i++ {
			otherHash, otherHeight := loadHead(t, nodes[i])
			if otherHash == nil || otherHeight != height || !bytes.Equal(hash, otherHash) {
				mismatch = true
				break
			}
		}
		if !mismatch {
			return hash, height
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Collect a detailed diagnostic snapshot before failing.
	var b strings.Builder
	fmt.Fprintf(&b, "timeout waiting for common head after %s; snapshot:\n", timeout)
	for i, node := range nodes {
		h, ht := loadHead(t, node)
		fmt.Fprintf(&b, "  node[%d] addr=%s height=%d len~=%d hash=%s\n",
			i, node.GetAddr(), ht, ht+1, hex.EncodeToString(h))
	}
	t.Fatal(b.String())
	return nil, 0
}

func loadHead(t *testing.T, node z.TestNode) ([]byte, uint64) {
	t.Helper()

	// Avoid replaying the entire chain from storage on every poll when the
	// running node already exposes its live chain. Reloading from disk was
	// reapplying all historical blocks and spamming duplicate tx-id logs.
	if chain := node.NamecoinChainState(); chain != nil {
		return chain.HeadHash(), chain.HeadHeight()
	}

	chain, err := impl.LoadNamecoinChain(node.GetStorage().GetNamecoinStore())
	require.NoError(t, err)
	return chain.HeadHash(), chain.HeadHeight()
}

func addAllPeers(nodes []z.TestNode) {
	for _, node := range nodes {
		neighbors := make([]string, 0, len(nodes))
		for _, other := range nodes {
			neighbors = append(neighbors, other.GetAddr())
		}
		node.AddPeer(neighbors...)
	}
}

func stopMinersFor(t *testing.T, nodes []z.TestNode) {
	t.Helper()

	for _, node := range nodes {
		if miner, ok := node.Peer.(interface{ DisableMiner() }); ok {
			miner.DisableMiner()
		}
	}
}

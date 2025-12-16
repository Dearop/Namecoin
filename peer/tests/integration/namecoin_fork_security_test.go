package integration

import (
	"bytes"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/channel"
	"go.dedis.ch/cs438/types"
)

var maxTarget = new(big.Int).Lsh(big.NewInt(1), 256)

// Helpers (copied minimally from unit helpers)
type mapStore struct{ data map[string][]byte }

func newMapStore() *mapStore { return &mapStore{data: make(map[string][]byte)} }
func (s *mapStore) Get(key string) []byte {
	if s.data == nil {
		return nil
	}
	if v, ok := s.data[key]; ok {
		return impl.CloneBytes(v)
	}
	return nil
}
func (s *mapStore) Set(key string, val []byte) { s.data[key] = impl.CloneBytes(val) }
func (s *mapStore) Delete(key string)          { delete(s.data, key) }
func (s *mapStore) Len() int                   { return len(s.data) }
func (s *mapStore) ForEach(f func(key string, val []byte) bool) {
	for k, v := range s.data {
		if !f(k, v) {
			return
		}
	}
}

func newTestChain(store storage.Store) *impl.NamecoinChain {
	c := impl.NewNamecoinChain(store)
	// Accept any PoW for test blocks.
	c.SetPowTarget(new(big.Int).Lsh(big.NewInt(1), 256))
	return c
}

func makeBlock(height uint64, prev []byte, txs []types.Tx) *types.Block {
	root, err := impl.ComputeTxRoot(txs)
	if err != nil {
		panic(err)
	}
	b := types.Block{
		Header: types.BlockHeader{
			Height:   height,
			PrevHash: impl.CloneBytes(prev),
			TxRoot:   root,
			Difficulty: maxTarget.Bytes(), // ensure PoW passes for synthetic blocks
		},
		Transactions: txs,
	}
	b.Hash = b.ComputeHash()
	return &b
}

func rewardTx(to string, amount uint64) types.Tx {
	tx := types.Tx{
		Type:   impl.RewardCommandName,
		From:   to,
		Amount: amount,
	}
	tx.TxID, _ = impl.BuildTransactionID(&tx)
	return tx
}

// S3-T2/S3-T3: equivocation and state stability.
func Test_NamecoinForkChoice_EquivocationConvergesAndStateFollowsWinner(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil, nil)
	require.NoError(t, chain.ApplyBlock(genesis))

	svc := impl.NewChainService(chain)

	// Two conflicting height-1 blocks (different miner rewards).
	blkA1 := makeBlock(1, genesis.Hash, []types.Tx{rewardTx("minerA", 1)})
	changed, err := svc.AppendBlockToLongestChain(blkA1)
	require.NoError(t, err)
	require.True(t, changed)

	blkB1 := makeBlock(1, genesis.Hash, []types.Tx{rewardTx("minerB", 1)})
	changed, err = svc.AppendBlockToLongestChain(blkB1)
	require.NoError(t, err)
	require.False(t, changed, "equal length fork should not immediately promote")

	// Extend chain A to win.
	blkA2 := makeBlock(2, blkA1.Hash, []types.Tx{rewardTx("minerA", 2)})
	changed, err = svc.AppendBlockToLongestChain(blkA2)
	require.NoError(t, err)
	require.True(t, changed)

	longest := svc.GetLongestChain()
	require.Equal(t, uint64(2), longest.HeadHeight())
	require.Equal(t, longest.HeadHash(), blkA2.Hash)

	// State should reflect only winning rewards (minerA has 2, minerB none).
	state := longest.State()
	utxosA := state.UTXOMap["minerA"]
	require.NotNil(t, utxosA)
	require.Len(t, utxosA, 2)
	_, minerBPresent := state.UTXOMap["minerB"]
	require.False(t, minerBPresent, "losing fork rewards must not persist")
}

// S3-T4/S3-T5: withheld blocks released later should still converge.
func Test_NamecoinForkChoice_WithheldBlocksEventuallyConverge(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil, nil)
	require.NoError(t, chain.ApplyBlock(genesis))
	svc := impl.NewChainService(chain)

	// Honest chain grows to height 2.
	main1 := makeBlock(1, genesis.Hash, []types.Tx{rewardTx("main", 1)})
	_, _ = svc.AppendBlockToLongestChain(main1)
	main2 := makeBlock(2, main1.Hash, []types.Tx{rewardTx("main", 2)})
	_, _ = svc.AppendBlockToLongestChain(main2)

	// Withheld fork at height 1 (not delivered yet).
	withheld1 := makeBlock(1, genesis.Hash, []types.Tx{rewardTx("withheld", 1)})
	withheld2 := makeBlock(2, withheld1.Hash, []types.Tx{rewardTx("withheld", 2)})
	withheld3 := makeBlock(3, withheld2.Hash, []types.Tx{rewardTx("withheld", 3)})

	// Deliver withheld blocks later; fork should win once longer.
	_, _ = svc.AppendBlockToLongestChain(withheld1) // still shorter (tie height 2)
	_, _ = svc.AppendBlockToLongestChain(withheld2) // tie with main (height 2), no change
	changed, err := svc.AppendBlockToLongestChain(withheld3)
	require.NoError(t, err)
	require.True(t, changed, "longer withheld fork should promote")

	longest := svc.GetLongestChain()
	require.Equal(t, withheld3.Hash, longest.HeadHash())
	require.Equal(t, uint64(3), longest.HeadHeight())

	// State reflects withheld branch (withheld rewards present, main removed).
	state := longest.State()
	require.NotNil(t, state.UTXOMap["withheld"])
	_, mainPresent := state.UTXOMap["main"]
	require.False(t, mainPresent, "main branch rewards should not persist after reorg")
}

// S3-T6: malformed block scenarios (TxRoot mismatch) are rejected without side effects.
func Test_NamecoinRejectsMalformedBlock_NoStateChange(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil, nil)
	require.NoError(t, chain.ApplyBlock(genesis))

	svc := impl.NewChainService(chain)

	// Malformed block: TxRoot incorrect (should not match transactions).
	badTx := rewardTx("miner", 1)
	badBlock := &types.Block{
		Header: types.BlockHeader{
			Height:   1,
			PrevHash: genesis.Hash,
			TxRoot:   []byte{0x00, 0x01}, // wrong root
		},
		Transactions: []types.Tx{badTx},
	}
	badBlock.Hash = badBlock.ComputeHash()

	changed, err := svc.AppendBlockToLongestChain(badBlock)
	require.Error(t, err, "malformed TxRoot should be rejected")
	require.False(t, changed)

	// State and head remain unchanged.
	longest := svc.GetLongestChain()
	require.Equal(t, uint64(0), longest.HeadHeight())
	require.Equal(t, genesis.Hash, longest.HeadHash())
	require.Nil(t, longest.State().UTXOMap["miner"])
}

// Malicious miner equivocation across subsets of nodes; eventual convergence on longest chain.
func Test_Namecoin_Integration_EquivocationAcrossSubsets(t *testing.T) {
	skipIfWIndows(t)

	trans := channel.NewTransport()
	easyTarget := new(big.Int).Lsh(big.NewInt(1), 252)
	opts := []z.Option{
		z.WithEnableMiner(false),
		z.WithPoWConfig(peer.PoWConfig{Target: easyTarget, PubKey: "malicious"}),
	}

	nodes := []z.TestNode{
		z.NewTestNode(t, studentFac, trans, "127.0.0.1:0", opts...),
		z.NewTestNode(t, studentFac, trans, "127.0.0.1:0", opts...),
		z.NewTestNode(t, studentFac, trans, "127.0.0.1:0", opts...),
	}
	defer terminateNodes(t, nodes)

	addAllPeers(nodes)

	// Common genesis to all nodes.
	genesis := makeBlock(0, nil, []types.Tx{rewardTx("genesis", 1)})
	for _, n := range nodes {
		deliverBlock(t, n, genesis)
	}

	// Malicious miner creates two conflicting height-1 blocks.
	blkA1 := makeBlock(1, genesis.Hash, []types.Tx{rewardTx("evil", 1)})
	blkB1 := makeBlock(1, genesis.Hash, []types.Tx{rewardTx("evil", 2)})

	// Send to disjoint subsets.
	deliverBlock(t, nodes[0], blkA1)
	deliverBlock(t, nodes[1], blkA1)
	deliverBlock(t, nodes[2], blkB1)

	// Deliver blkA1 to node2 (gossip eventually) then a longer extension blkA2.
	deliverBlock(t, nodes[2], blkA1)
	blkA2 := makeBlock(2, blkA1.Hash, []types.Tx{rewardTx("evil", 3)})
	for _, n := range nodes {
		deliverBlock(t, n, blkA2)
	}

	// All nodes should converge on blkA2 as head.
	require.Eventually(t, func() bool {
		for _, n := range nodes {
			chain := n.NamecoinChainState()
			if chain == nil || chain.HeadHeight() != 2 || !bytes.Equal(chain.HeadHash(), blkA2.Hash) {
				return false
			}
		}
		return true
	}, 3*time.Second, 50*time.Millisecond, "nodes did not converge on longest chain after equivocation")

	// State: only the winning chain rewards should exist (amounts 1 and 3).
	for _, n := range nodes {
		state := n.NamecoinChainState().State()
		require.NotNil(t, state.UTXOMap["evil"])
		require.Len(t, state.UTXOMap["evil"], 2)
	}
}

func deliverBlock(t *testing.T, node z.TestNode, blk *types.Block) {
	t.Helper()
	reg := node.GetRegistry()
	wire, err := reg.MarshalMessage(types.NamecoinBlockMessage{Block: *blk})
	require.NoError(t, err)
	header := transport.NewHeader(node.GetAddr(), node.GetAddr(), node.GetAddr())
	require.NoError(t, reg.ProcessPacket(transport.Packet{Header: &header, Msg: &wire}))
}

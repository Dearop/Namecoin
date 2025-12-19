package integration

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/storage/file"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/channel"
	"go.dedis.ch/cs438/types"
)

// Single-node chain growth with easy PoW.
func Test_Namecoin_Integration_SingleNodeChainGrowth(t *testing.T) {
	transp := channel.NewTransport()

	tmpFolder, err := os.MkdirTemp("", "namecoin_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpFolder)

	storage, err := file.NewPersistency(tmpFolder)
	require.NoError(t, err)

	// Very easy target so mining is fast in tests.
	easyTarget := new(big.Int).Lsh(big.NewInt(1), 252)
	powCfg := peer.PoWConfig{
		Target:                      easyTarget,
		MaxNonce:                    0,
		PubKey:                      "miner-single",
		DisableDifficultyAdjustment: true,
	}

	node := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0",
		z.WithStorage(storage),
		z.WithPoWConfig(powCfg),
		z.WithEnableMiner(true),
	)
	defer node.Stop()

	// Wait until at least one block has been mined and applied.
	var (
		headHash   []byte
		headHeight uint64
	)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		chain, err := impl.LoadNamecoinChain(storage.GetNamecoinStore())
		require.NoError(t, err)
		headHash = chain.HeadHash()
		headHeight = chain.HeadHeight()
		if headHash != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NotNil(t, headHash, "expected at least one mined block")

	// Wait for at least one more block so that height increases.
	initialHeight := headHeight
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		chain, err := impl.LoadNamecoinChain(storage.GetNamecoinStore())
		require.NoError(t, err)
		if chain.HeadHeight() > initialHeight {
			headHeight = chain.HeadHeight()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Greater(t, headHeight, initialHeight, "expected chain height to grow")

	// Verify miner has received some reward UTXOs in reconstructed state.
	finalChain, err := impl.LoadNamecoinChain(storage.GetNamecoinStore())
	require.NoError(t, err)
	state := finalChain.State()
	require.NotNil(t, state)
	require.NotEmpty(t, state.UTXOMap["miner-single"], "expected miner to have non-empty UTXO set")
}

// Restart + state reconstruction from persistent storage.
func Test_Namecoin_Integration_RestartRebuildsState(t *testing.T) {
	transp := channel.NewTransport()

	tmpFolder, err := os.MkdirTemp("", "namecoin_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpFolder)

	storage, err := file.NewPersistency(tmpFolder)
	require.NoError(t, err)

	easyTarget := new(big.Int).Lsh(big.NewInt(1), 252)
	powCfg := peer.PoWConfig{
		Target:                      easyTarget,
		MaxNonce:                    0,
		PubKey:                      "miner-restart",
		DisableDifficultyAdjustment: true,
	}

	// First node mines a few blocks.
	node1 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0",
		z.WithStorage(storage),
		z.WithPoWConfig(powCfg),
		z.WithEnableMiner(true),
	)

	// Wait for some blocks to be mined.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		chain, err := impl.LoadNamecoinChain(storage.GetNamecoinStore())
		require.NoError(t, err)
		if chain.HeadHash() != nil && chain.HeadHeight() >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Stop first node.
	node1.Stop()

	chainBefore, err := impl.LoadNamecoinChain(storage.GetNamecoinStore())
	require.NoError(t, err)
	headHeightBefore := chainBefore.HeadHeight()
	headHashBefore := chainBefore.HeadHash()
	stateBefore := chainBefore.State().Clone()

	require.NotNil(t, headHashBefore)
	require.GreaterOrEqual(t, headHeightBefore, uint64(1))

	// Start a new node on the same storage; do not mine further.
	transp2 := channel.NewTransport()
	node2 := z.NewTestNode(t, studentFac, transp2, "127.0.0.1:0",
		z.WithStorage(storage),
		z.WithPoWConfig(powCfg),
	)
	defer node2.Stop()

	chainAfter, err := impl.LoadNamecoinChain(storage.GetNamecoinStore())
	require.NoError(t, err)
	require.Equal(t, headHeightBefore, chainAfter.HeadHeight())
	require.Equal(t, headHashBefore, chainAfter.HeadHash())

	stateAfter := chainAfter.State()
	require.Equal(t, stateBefore.Domains, stateAfter.Domains)
	require.Equal(t, stateBefore.Commitments, stateAfter.Commitments)
	require.Equal(t, stateBefore.UTXOMap, stateAfter.UTXOMap)
}

// Bad PoW rejection: ensure a block that does not satisfy the configured target
// does not advance the chain or mutate storage.
func Test_Namecoin_Integration_BadPoWBlockRejected(t *testing.T) {
	transp := channel.NewTransport()

	tmpFolder, err := os.MkdirTemp("", "namecoin_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpFolder)

	storage, err := file.NewPersistency(tmpFolder)
	require.NoError(t, err)

	// Zero target makes every block invalid under IsBlockComplexityValid.
	strictTarget := big.NewInt(0)
	powCfg := peer.PoWConfig{
		Target:                      strictTarget,
		MaxNonce:                    0,
		PubKey:                      "miner-badpow",
		DisableDifficultyAdjustment: true,
	}

	node := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0",
		z.WithStorage(storage),
		z.WithPoWConfig(powCfg),
	)
	defer node.Stop()

	store := storage.GetNamecoinStore()
	chainBefore, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	headHeightBefore := chainBefore.HeadHeight()
	headHashBefore := chainBefore.HeadHash()

	// Build a syntactically valid genesis block that will fail PoW under strictTarget.
	header := types.BlockHeader{
		Height:   0,
		PrevHash: nil,
		Miner:    powCfg.PubKey,
		// Difficulty must match the chain's expected target.
		Difficulty: impl.EncodeDifficulty(powCfg.Target),
	}
	block := impl.AssembleBlock(&header, nil, powCfg.PubKey)

	msg := types.NamecoinBlockMessage{Block: block}
	wire, err := node.GetRegistry().MarshalMessage(msg)
	require.NoError(t, err)

	hdr := transport.NewHeader(node.GetAddr(), node.GetAddr(), node.GetAddr())
	err = node.GetRegistry().ProcessPacket(transport.Packet{
		Header: &hdr,
		Msg:    &wire,
	})
	require.Error(t, err, "expected bad PoW block to trigger handler error")

	// Reload chain from storage and ensure head did not change and no blocks were persisted.
	chainAfter, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, headHeightBefore, chainAfter.HeadHeight())
	require.Equal(t, headHashBefore, chainAfter.HeadHash())
	require.Equal(t, 0, store.Len(), "expected no blocks persisted for bad PoW")
}

// Valid PoW but structurally invalid block (e.g., txRoot mismatch) must be rejected.
func Test_Namecoin_Integration_ValidPoWInvalidStructureRejected(t *testing.T) {
	transp := channel.NewTransport()

	tmpFolder, err := os.MkdirTemp("", "namecoin_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpFolder)

	storage, err := file.NewPersistency(tmpFolder)
	require.NoError(t, err)

	// Very high target so any hash is considered valid PoW.
	highTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	powCfg := peer.PoWConfig{
		Target:                      highTarget,
		MaxNonce:                    0,
		PubKey:                      "miner-struct",
		DisableDifficultyAdjustment: true,
	}

	node := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0",
		z.WithStorage(storage),
		z.WithPoWConfig(powCfg),
	)
	defer node.Stop()

	store := storage.GetNamecoinStore()
	chainBefore, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	headHeightBefore := chainBefore.HeadHeight()
	headHashBefore := chainBefore.HeadHash()
	target := chainBefore.NextPowTarget()

	// Build a structurally invalid block: txRoot in header doesn't match transactions.
	header := types.BlockHeader{
		Height:     0,
		PrevHash:   nil,
		Miner:      powCfg.PubKey,
		Difficulty: impl.EncodeDifficulty(target),
	}
	// Start from a valid block, then mutate transactions without updating TxRoot.
	validBlock := impl.AssembleBlock(&header, nil, powCfg.PubKey)
	block := validBlock
	// Add a dummy transaction; txRoot now mismatches header.TxRoot but PoW remains "valid"
	// because it only depends on the header.
	block.Transactions = append(block.Transactions, types.Tx{
		From: powCfg.PubKey,
		Type: impl.NameNew{}.Name(),
	})

	msg := types.NamecoinBlockMessage{Block: block}
	wire, err := node.GetRegistry().MarshalMessage(msg)
	require.NoError(t, err)

	hdr := transport.NewHeader(node.GetAddr(), node.GetAddr(), node.GetAddr())
	err = node.GetRegistry().ProcessPacket(transport.Packet{
		Header: &hdr,
		Msg:    &wire,
	})
	require.Error(t, err, "expected structurally invalid block to be rejected")

	chainAfter, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, headHeightBefore, chainAfter.HeadHeight())
	require.Equal(t, headHashBefore, chainAfter.HeadHash())
	require.Equal(t, 0, store.Len(), "expected no blocks persisted for structurally invalid block")
}

// When a valid block at height 0 has been accepted, a second block at the same
// height must be rejected because it does not extend the current head.
func Test_Namecoin_Integration_SecondBlockSameHeightRejected(t *testing.T) {
	tmpFolder, err := os.MkdirTemp("", "namecoin_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpFolder)

	storage, err := file.NewPersistency(tmpFolder)
	require.NoError(t, err)

	highTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	store := storage.GetNamecoinStore()
	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.NotNil(t, chain)
	chain.SetPowTarget(highTarget)

	miner := "miner-fork"

	// First genesis-like block (height 0, prevHash nil) should be accepted.
	header1 := types.BlockHeader{
		Height:     0,
		PrevHash:   nil,
		Miner:      miner,
		Difficulty: impl.EncodeDifficulty(chain.NextPowTarget()),
	}
	block1 := impl.AssembleBlock(&header1, nil, miner)
	require.NoError(t, chain.ApplyBlock(&block1), "expected first genesis block to be accepted")

	// Snapshot head after first block.
	chainAfterFirst, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, uint64(0), chainAfterFirst.HeadHeight())
	require.NotNil(t, chainAfterFirst.HeadHash())

	genesisKey := impl.NamecoinBlockPrefix + fmt.Sprintf("%020d", 0)
	firstGenesisBytes := store.Get(genesisKey)
	require.NotEmpty(t, firstGenesisBytes, "expected genesis block persisted after first genesis")

	// Second block at the same height 0 should be rejected because the chain
	// head is no longer nil; it does not extend the current head.
	header2 := types.BlockHeader{
		Height:     0,
		PrevHash:   nil,
		Miner:      miner,
		Difficulty: impl.EncodeDifficulty(chain.NextPowTarget()),
	}
	block2 := impl.AssembleBlock(&header2, nil, miner)
	err = chain.ApplyBlock(&block2)
	require.Error(t, err, "expected second block at same height to be rejected")

	// Chain should still have the first block as head, with no additional blocks.
	chainFinal, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, uint64(0), chainFinal.HeadHeight())
	require.Equal(t, chainAfterFirst.HeadHash(), chainFinal.HeadHash())
	finalGenesisBytes := store.Get(genesisKey)
	require.NotEmpty(t, finalGenesisBytes, "expected genesis block to remain persisted after rejecting second genesis")
	require.Equal(t, firstGenesisBytes, finalGenesisBytes, "expected genesis block data to be unchanged")
}

// Commitment from a name_new in a prior block must leave a usable commitment.
func Test_Namecoin_Integration_NameNewCommitmentPersistsAcrossBlocks(t *testing.T) {
	tmpFolder, err := os.MkdirTemp("", "namecoin_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpFolder)

	storage, err := file.NewPersistency(tmpFolder)
	require.NoError(t, err)

	highTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	store := storage.GetNamecoinStore()
	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.NotNil(t, chain)
	chain.SetPowTarget(highTarget)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	owner := impl.HashHex(pub)
	miner := owner
	domain := "example.bit"
	salt := "secret"
	commitment := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt))

	// Block 0 with a single name_new spending the coinbase output.
	payloadNew, err := json.Marshal(impl.NameNew{Commitment: commitment})
	require.NoError(t, err)

	reward0 := types.Tx{
		From:    miner,
		Type:    impl.RewardCommandName,
		Amount:  1,
		Payload: json.RawMessage(fmt.Sprintf(`{"height":%d}`, 0)),
		Outputs: []types.TxOutput{{To: miner, Amount: 1}},
	}
	reward0ID, err := impl.BuildTransactionID(&reward0)
	require.NoError(t, err)

	txNew := types.Tx{
		From:    owner,
		Type:    impl.NameNew{}.Name(),
		Amount:  1,
		Payload: json.RawMessage(payloadNew),
		Inputs:  []types.TxInput{{TxID: reward0ID, Index: 0}},
		Outputs: []types.TxOutput{},
		Pk:      hex.EncodeToString(pub),
	}
	txNewID, err := impl.BuildTransactionID(&txNew)
	require.NoError(t, err)
	txNew.TxID = txNewID

	preimage, err := (&impl.SignedTransaction{
		Type:    txNew.Type,
		From:    txNew.From,
		Amount:  txNew.Amount,
		Payload: txNew.Payload,
		Inputs:  txNew.Inputs,
		Outputs: txNew.Outputs,
	}).SerializeTransactionSignature()
	require.NoError(t, err)
	txNew.Signature = hex.EncodeToString(ed25519.Sign(priv, impl.Hash(preimage)))
	header0 := types.BlockHeader{
		Height:     0,
		PrevHash:   nil,
		Miner:      miner,
		Difficulty: impl.EncodeDifficulty(chain.NextPowTarget()),
	}
	block0 := impl.AssembleBlock(&header0, []types.Tx{txNew}, miner)
	require.NoError(t, chain.ApplyBlock(&block0), "expected name_new block to be accepted")

	chainAfterFirst, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, uint64(0), chainAfterFirst.HeadHeight())
	state := chainAfterFirst.State()
	key := impl.OutpointKey(txNewID, 0)
	commit, ok := state.GetCommitment(key)
	require.True(t, ok)
	require.Equal(t, commitment, commit.Commit, "expected commitment to persist after first block")

	// Block 1 (empty pending set) should keep the commitment intact.
	header1 := types.BlockHeader{
		Height:     1,
		PrevHash:   chainAfterFirst.HeadHash(),
		Miner:      miner,
		Difficulty: impl.EncodeDifficulty(chain.NextPowTarget()),
	}
	block1 := impl.AssembleBlock(&header1, nil, miner)
	require.NoError(t, chain.ApplyBlock(&block1), "expected second block to be accepted")

	chainFinal, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, uint64(1), chainFinal.HeadHeight())
	stateFinal := chainFinal.State()
	commitFinal, ok := stateFinal.GetCommitment(key)
	require.True(t, ok)
	require.Equal(t, commitment, commitFinal.Commit, "expected commitment to persist across blocks")
}

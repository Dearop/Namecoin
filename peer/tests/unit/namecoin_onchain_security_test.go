package unit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

// These tests encode security properties we want (Bitcoin/Namecoin-style):
// - blocks should not be able to spend coins without an owner-authorizing signature, and
// - the block TxRoot should commit to the spend (inputs/outputs) so miners can't tamper.

func Test_Security_OnChainRejectsUnsignedSpend(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	victim := "victim"
	attacker := "attacker"

	// Fund the victim via a genesis reward UTXO.
	genesisReward := types.Tx{
		Type:    impl.Reward{}.Name(),
		From:    victim,
		Amount:  10,
		Outputs: []types.TxOutput{{To: victim, Amount: 10}},
	}
	genesisTxRoot, err := impl.ComputeTxRoot([]types.Tx{genesisReward})
	require.NoError(t, err)
	genesis := &types.Block{
		Header: types.BlockHeader{
			Height: 0,
			TxRoot: genesisTxRoot,
		},
		Transactions: []types.Tx{genesisReward},
	}
	genesis.Hash = genesis.ComputeHash()
	require.NoError(t, chain.ApplyBlock(genesis))

	victimUTXO, err := impl.BuildTransactionID(&genesisReward)
	require.NoError(t, err)

	// Malicious miner constructs an unsigned tx that spends the victim's UTXO and
	// pays the attacker. There is no signature on-chain to verify.
	malicious := types.Tx{
		From:   victim,
		Type:   impl.NameNew{}.Name(),
		Amount: 0,
		Inputs: []types.TxInput{{TxID: victimUTXO, Index: 0}},
		Outputs: []types.TxOutput{{
			To:     attacker,
			Amount: 10,
		}},
		Payload: mustPayload(t, impl.NameNew{Commitment: "evil"}),
	}
	blkRoot, err := impl.ComputeTxRoot([]types.Tx{malicious})
	require.NoError(t, err)
	blk := &types.Block{
		Header: types.BlockHeader{
			Height:   1,
			PrevHash: genesis.Hash,
			TxRoot:   blkRoot,
		},
		Transactions: []types.Tx{malicious},
	}
	blk.Hash = blk.ComputeHash()

	// Security property: the chain should reject this block because the "owner"
	// did not authorize the spend (no signature/script data exists on-chain).
	require.Error(t, chain.ApplyBlock(blk), "expected unauthorized spend to be rejected on-chain")
}

func Test_Security_TxRootCommitsToInputsAndOutputs(t *testing.T) {
	// Two transactions that differ only by inputs/outputs should yield different
	// TxRoot if TxRoot commits to the spend. Today, SerializeTransaction excludes
	// inputs/outputs, so roots can collide and miners can tamper.
	base := types.Tx{
		From:    "someone",
		Type:    impl.NameNew{}.Name(),
		Amount:  1,
		Payload: mustPayload(t, impl.NameNew{Commitment: "same"}),
	}

	txA := base
	txA.Inputs = []types.TxInput{{TxID: "utxo-a", Index: 0}}
	txA.Outputs = []types.TxOutput{{To: "alice", Amount: 1}}

	txB := base
	txB.Inputs = []types.TxInput{{TxID: "utxo-b", Index: 0}}
	txB.Outputs = []types.TxOutput{{To: "bob", Amount: 1}}

	rootA, err := impl.ComputeTxRoot([]types.Tx{txA})
	require.NoError(t, err)
	rootB, err := impl.ComputeTxRoot([]types.Tx{txB})
	require.NoError(t, err)

	// Security property: roots must differ because the spends differ.
	require.NotEqual(t, rootA, rootB, "expected TxRoot to commit to inputs/outputs")
}

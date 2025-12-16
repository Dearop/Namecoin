package unit

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

// Demonstrates pre-mining malleability: miner can swap which UTXO is burned under
// the same TxID, leaving the user's intended UTXO spendable.
func Test_InputSwapSameTxID_IsRejected_OptionB(t *testing.T) {
	chain := newTestChain(newMapStore())
	st := chain.State()

	pub, priv := mustMakeKeyPair(t)
	aliceAddr := hex.EncodeToString(impl.Hash(pub))

	// Fund alice with deterministic keys
	st.UTXOMap[aliceAddr] = map[string]types.UTXO{
		"utxo-A": {TxID: "utxo-A", To: aliceAddr, Amount: 5},
		"utxo-B": {TxID: "utxo-B", To: aliceAddr, Amount: 5},
	}

	payloadBytes, _ := json.Marshal(impl.NameNew{Commitment: "mal-commit"})
	intendedInputs, intendedOutputs, err := st.DeterministicSpendPlan(aliceAddr, 5)
	require.NoError(t, err)

	// Intent-signing: TxID/signature over {type,from,amount,payload}
	signed := impl.SignedTransaction{
		Type:    impl.NameNew{}.Name(),
		From:    aliceAddr,
		Amount:  5,
		Payload: payloadBytes,
		Pk:      hex.EncodeToString(pub),
		Inputs:  intendedInputs,
		Outputs: intendedOutputs,
	}
	unsigned, err := signed.SerializeTransaction()
	require.NoError(t, err)
	signed.TxID = impl.HashHex(unsigned)
	sig := ed25519.Sign(priv, impl.Hash(unsigned))
	signed.Signature = hex.EncodeToString(sig)

	// Canonical spend would pick utxo-A (sorted order), but miner swaps to utxo-B
	tx := types.Tx{
		Type:      signed.Type,
		From:      signed.From,
		Inputs:    []types.TxInput{{TxID: "utxo-B", Index: 0}}, // mutated
		Outputs:   nil,
		Amount:    signed.Amount,
		Payload:   signed.Payload,
		Pk:        signed.Pk,
		TxID:      signed.TxID,
		Signature: signed.Signature,
	}

	root, err := impl.ComputeTxRoot([]types.Tx{tx})
	require.NoError(t, err)

	blk := &types.Block{
		Header:       types.BlockHeader{Height: 0, PrevHash: nil, TxRoot: root},
		Transactions: []types.Tx{tx},
	}
	blk.Hash = blk.ComputeHash()

	// Should reject because inputs are not canonical
	require.Error(t, chain.ApplyBlock(blk))

	// State unchanged
	_, okA := st.UTXOMap[aliceAddr]["utxo-A"]
	_, okB := st.UTXOMap[aliceAddr]["utxo-B"]
	require.True(t, okA)
	require.True(t, okB)
}

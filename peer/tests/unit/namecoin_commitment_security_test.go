package unit

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func onChainTxFromSigned(stx impl.SignedTransaction, inputs []types.TxInput, outputs []types.TxOutput) types.Tx {
	return types.Tx{
		From:      stx.From,
		Type:      stx.Type,
		Inputs:    inputs,
		Outputs:   outputs,
		Amount:    stx.Amount,
		Payload:   stx.Payload,
		Pk:        stx.Pk,
		TxID:      stx.TxID,
		Signature: stx.Signature,
	}
}

func applyBlock(t *testing.T, chain *impl.NamecoinChain, height uint64, prevHash []byte, txs []types.Tx) *types.Block {
	t.Helper()

	root, err := impl.ComputeTxRoot(txs)
	require.NoError(t, err)

	blk := &types.Block{
		Header: types.BlockHeader{
			Height:   height,
			PrevHash: impl.CloneBytes(prevHash),
			TxRoot:   root,
		},
		Transactions: txs,
	}
	blk.Hash = blk.ComputeHash()
	require.NoError(t, chain.ApplyBlock(blk))
	return blk
}

func makeKeyedAddress(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv := mustMakeKeyPair(t)
	from := fmt.Sprintf("%x", impl.Hash(pub))
	return pub, priv, from
}

// Stale commitment reveal:
// If the commit (name_new) is too old, a later name_firstupdate should be rejected.
//
// Namecoin enforces this with MAX_FIRSTUPDATE_DEPTH; we now track commit height per outpoint.
func Test_Security_RejectsStaleCommitmentReveal(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	pub, priv, from := makeKeyedAddress(t)

	const (
		domain = "stale.bit"
		salt   = "stale-salt"
	)
	commit := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt))

	// Fund the sender with 1 coin.
	genesisReward := types.Tx{
		Type:   impl.RewardCommandName,
		From:   from,
		Amount: 1,
	}
	genesis := applyBlock(t, chain, 0, nil, []types.Tx{genesisReward})
	fundingUTXO, err := impl.BuildTransactionID(&genesisReward)
	require.NoError(t, err)

	// Create the commitment.
	nameNewSigned := buildSignedTransaction(
		t,
		from,
		priv,
		impl.NameNewCommandName,
		0,
		impl.NameNew{Commitment: commit, TTL: 10},
		pub,
	)
	nameNewTx := onChainTxFromSigned(
		nameNewSigned,
		[]types.TxInput{{TxID: fundingUTXO, Index: 0}},
		[]types.TxOutput{{To: from, Amount: 1}},
	)
	commitBlk := applyBlock(t, chain, 1, genesis.Hash, []types.Tx{nameNewTx})
	commitTxID := nameNewTx.TxID

	// "Mine many blocks" after the commit (no txs).
	const maxFirstUpdateDepth = uint64(25)
	prev := commitBlk.Hash
	for h := uint64(2); h <= 1+maxFirstUpdateDepth+1; h++ {
		blk := applyBlock(t, chain, h, prev, nil)
		prev = blk.Hash
	}

	// Attempt to reveal long after the commit.
	revealSigned := buildSignedTransaction(
		t,
		from,
		priv,
		impl.NameFirstUpdateCommandName,
		0,
		impl.NameFirstUpdate{
			Domain: domain,
			Salt:   salt,
			IP:     "203.0.113.1",
			TTL:    0,
			TxID:   commitTxID,
		},
		pub,
	)
	revealTx := onChainTxFromSigned(revealSigned, nil, nil)

	// Desired security property: reveal should be rejected once the commitment is stale.
	// Today this is accepted because we don't track commit height/max-depth.
	root, err := impl.ComputeTxRoot([]types.Tx{revealTx})
	require.NoError(t, err)
	revealBlk := &types.Block{
		Header: types.BlockHeader{
			Height:   1 + maxFirstUpdateDepth + 2,
			PrevHash: prev,
			TxRoot:   root,
		},
		Transactions: []types.Tx{revealTx},
	}
	revealBlk.Hash = revealBlk.ComputeHash()
	require.Error(t, chain.ApplyBlock(revealBlk), "expected stale commitment reveal to be rejected")
}

// TTL skew:
// If two name_new transactions reuse the same commitment hash but choose different TTL values,
// a later firstupdate referencing the first outpoint should not "inherit" the newer TTL.
//
// TTL preference is now stored per outpoint to avoid skew.
func Test_Security_NoTTLSkewAcrossNameNew(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	pub, priv, from := makeKeyedAddress(t)

	const (
		domain = "skew.bit"
		salt   = "skew-salt"
	)
	commit := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", domain, salt))

	// Fund with 1 coin.
	genesisReward := types.Tx{
		Type:   impl.RewardCommandName,
		From:   from,
		Amount: 1,
	}
	genesis := applyBlock(t, chain, 0, nil, []types.Tx{genesisReward})
	fundingUTXO, err := impl.BuildTransactionID(&genesisReward)
	require.NoError(t, err)

	ttl1 := uint64(10)
	ttl2 := uint64(20)

	// First name_new writes TTL preference ttl1.
	nameNew1Signed := buildSignedTransaction(
		t,
		from,
		priv,
		impl.NameNewCommandName,
		0,
		impl.NameNew{Commitment: commit, TTL: ttl1},
		pub,
	)
	nameNew1Tx := onChainTxFromSigned(
		nameNew1Signed,
		[]types.TxInput{{TxID: fundingUTXO, Index: 0}},
		[]types.TxOutput{{To: from, Amount: 1}},
	)
	blk1 := applyBlock(t, chain, 1, genesis.Hash, []types.Tx{nameNew1Tx})

	// Second name_new reuses the same commitment but overwrites TTL preference to ttl2.
	nameNew2Signed := buildSignedTransaction(
		t,
		from,
		priv,
		impl.NameNewCommandName,
		0,
		impl.NameNew{Commitment: commit, TTL: ttl2},
		pub,
	)
	nameNew2Tx := onChainTxFromSigned(
		nameNew2Signed,
		[]types.TxInput{{TxID: nameNew1Tx.TxID, Index: 0}},
		[]types.TxOutput{{To: from, Amount: 1}},
	)
	blk2 := applyBlock(t, chain, 2, blk1.Hash, []types.Tx{nameNew2Tx})

	// Reveal using the first name_new outpoint, without specifying TTL in firstupdate.
	revealSigned := buildSignedTransaction(
		t,
		from,
		priv,
		impl.NameFirstUpdateCommandName,
		0,
		impl.NameFirstUpdate{
			Domain: domain,
			Salt:   salt,
			IP:     "203.0.113.2",
			TTL:    0, // force using stored TTL preference
			TxID:   nameNew1Tx.TxID,
		},
		pub,
	)
	revealTx := onChainTxFromSigned(revealSigned, nil, nil)
	_ = applyBlock(t, chain, 3, blk2.Hash, []types.Tx{revealTx})

	rec, ok := chain.State().NameLookup(domain)
	require.True(t, ok, "expected domain record to exist after firstupdate")

	// Desired security property: the TTL used should be the one associated with the referenced name_new outpoint.
	require.Equal(t, uint64(3)+ttl1, rec.ExpiresAt, "expected firstupdate to use the TTL from the referenced name_new")
}

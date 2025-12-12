package integration

import (
	"crypto/sha256"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/storage/inmemory"
	"go.dedis.ch/cs438/types"
)

func txRoot(t *testing.T, txs []types.Tx) []byte {
	t.Helper()
	h := sha256.New()
	for i := range txs {
		enc, err := impl.SerializeTransaction(&txs[i])
		require.NoError(t, err)
		_, err = h.Write(enc)
		require.NoError(t, err)
	}
	return h.Sum(nil)
}

func rewardTx(amount uint64, to string) types.Tx {
	return types.Tx{
		From:   to,
		Type:   impl.RewardCommandName,
		Amount: amount,
		Outputs: []types.TxOutput{{
			To:     to,
			Amount: amount,
		}},
	}
}

func TestNamecoinBlockValidationRejectsBadTxRoot(t *testing.T) {
	store := inmemory.NewPersistency().GetNamecoinStore()
	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	chain.SetPowTarget(new(big.Int).Lsh(big.NewInt(1), 256))

	txs := []types.Tx{rewardTx(10, "miner")}

	blk := types.Block{
		Header: types.BlockHeader{
			Height:    0,
			TxRoot:    []byte("bogus-root"),
			Timestamp: time.Now().Unix(),
		},
		Transactions: txs,
	}
	blk.Hash = blk.ComputeHash()

	err = chain.ApplyBlock(&blk)
	require.Error(t, err, "tx root mismatch should reject block")

	require.Nil(t, chain.HeadHash())
	require.Equal(t, uint64(0), chain.HeadHeight())
	require.Equal(t, 0, countBlocks(store), "block store should remain empty on failure")
}

func TestNamecoinBlockValidationRejectsInvalidWork(t *testing.T) {
	store := inmemory.NewPersistency().GetNamecoinStore()
	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	chain.SetPowTarget(new(big.Int).Lsh(big.NewInt(1), 256))

	txs := []types.Tx{rewardTx(10, "miner")}
	root := txRoot(t, txs)

	genesis := types.Block{
		Header: types.BlockHeader{
			Height:    0,
			TxRoot:    root,
			Timestamp: time.Now().Unix(),
		},
		Transactions: txs,
	}
	genesis.Hash = genesis.ComputeHash()

	require.NoError(t, chain.ApplyBlock(&genesis))

	// Next block with impossible PoW target.
	chain.SetPowTarget(big.NewInt(1))
	badWork := types.Block{
		Header: types.BlockHeader{
			Height:    1,
			PrevHash:  genesis.Hash,
			TxRoot:    root,
			Timestamp: time.Now().Unix(),
		},
		Transactions: txs,
	}
	badWork.Hash = badWork.ComputeHash()

	err = chain.ApplyBlock(&badWork)
	require.Error(t, err, "block above target should be rejected")

	require.Equal(t, genesis.Hash, chain.HeadHash(), "head hash should remain genesis after rejection")
	require.Equal(t, uint64(0), chain.HeadHeight(), "head height should not advance")
	require.Equal(t, 1, countBlocks(store), "only genesis block should be persisted")
}

func countBlocks(store storage.Store) int {
	var blocks int
	store.ForEach(func(key string, _ []byte) bool {
		if strings.HasPrefix(key, impl.NamecoinBlockPrefix) {
			blocks++
		}
		return true
	})
	return blocks
}

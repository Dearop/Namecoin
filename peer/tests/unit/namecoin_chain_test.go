package unit

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/storage/inmemory"
	"go.dedis.ch/cs438/types"
)

func newTestChain(t *testing.T) (*impl.NamecoinChain, storage.Store) {
	t.Helper()
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()
	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.NotNil(t, chain)
	require.NotNil(t, chain.State())
	chain.SetPowTarget(new(big.Int).Lsh(big.NewInt(1), 260))
	return chain, store
}

// ---- LoadNamecoinChain tests ----

func Test_Namecoin_LoadNamecoinChain_EmptyStore(t *testing.T) {
	chain, store := newTestChain(t)

	require.Empty(t, chain.State().Domains)
	require.Nil(t, chain.HeadHash())
	require.Equal(t, uint64(0), chain.HeadHeight())
	require.Empty(t, store.Get(impl.NamecoinLastBlockKey))
}

func Test_Namecoin_LoadNamecoinChain_SingleBlock(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	genesis := mustBlock(t, 0, nil)
	data, err := genesis.Marshal()
	require.NoError(t, err)
	store.Set(encodeNamecoinBlockKey(genesis.Header.Height), data)

	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, genesis.Header.Height, chain.HeadHeight())
	require.Equal(t, genesis.Hash, chain.HeadHash())
	require.Equal(t, genesis.Hash, store.Get(impl.NamecoinLastBlockKey))
}

func Test_Namecoin_LoadNamecoinChain_MultipleBlocksAndPrevHashMismatch(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	b0 := mustBlock(t, 0, nil)
	data0, _ := b0.Marshal()
	store.Set(encodeNamecoinBlockKey(0), data0)

	b1Bad := mustBlock(t, 1, []byte("not-prev"))
	data1, _ := b1Bad.Marshal()
	store.Set(encodeNamecoinBlockKey(1), data1)

	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, b0.Header.Height, chain.HeadHeight())
	require.Equal(t, b0.Hash, chain.HeadHash())
}

func Test_Namecoin_LoadNamecoinChain_RespectsStoredHeadHash(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	b0 := mustBlock(t, 0, nil)
	data0, _ := b0.Marshal()
	store.Set(encodeNamecoinBlockKey(0), data0)

	b1 := mustBlock(t, 1, b0.Hash)
	data1, _ := b1.Marshal()
	store.Set(encodeNamecoinBlockKey(1), data1)

	store.Set(impl.NamecoinLastBlockKey, b0.Hash)

	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, b0.Header.Height, chain.HeadHeight())
	require.Equal(t, b0.Hash, chain.HeadHash())
}

func Test_Namecoin_LoadNamecoinChain_NilStoreError(t *testing.T) {
	var store storage.Store
	chain, err := impl.LoadNamecoinChain(store)
	require.Error(t, err)
	require.Nil(t, chain)
}

func Test_Namecoin_LoadNamecoinChain_StoredHeadNotFoundUpdatesPointer(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	b0 := mustBlock(t, 0, nil)
	data0, _ := b0.Marshal()
	store.Set(encodeNamecoinBlockKey(0), data0)

	b1 := mustBlock(t, 1, b0.Hash)
	data1, _ := b1.Marshal()
	store.Set(encodeNamecoinBlockKey(1), data1)

	store.Set(impl.NamecoinLastBlockKey, []byte("missing-head"))

	chain, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, b1.Header.Height, chain.HeadHeight())
	require.Equal(t, b1.Hash, chain.HeadHash())
	require.Equal(t, b1.Hash, store.Get(impl.NamecoinLastBlockKey))
}

// ---- ValidateBlock tests ----

func Test_Namecoin_ValidateBlock_GenesisValidAndInvalid(t *testing.T) {
	chain, _ := newTestChain(t)

	genesis := mustBlock(t, 0, nil)

	err := chain.ValidateBlock(chain.HeadHeight(), chain.HeadHash(), chain.State().Clone(), &genesis)
	require.NoError(t, err)

	t.Run("bad height", func(t *testing.T) {
		bad := genesis
		bad.Header.Height = 1
		err := chain.ValidateBlock(0, nil, chain.State().Clone(), &bad)
		require.Error(t, err)
	})

	t.Run("non-empty prev hash", func(t *testing.T) {
		bad := genesis
		bad.Header.PrevHash = []byte("non-empty")
		err := chain.ValidateBlock(0, nil, chain.State().Clone(), &bad)
		require.Error(t, err)
	})
}

func Test_Namecoin_ValidateBlock_TxRootAndPrevHashChecks(t *testing.T) {
	chain, _ := newTestChain(t)

	genesis := mustBlock(t, 0, nil)
	require.NoError(t, chain.ApplyBlock(&genesis))

	b1 := mustBlock(t, 1, chain.HeadHash())
	require.NoError(t, chain.ValidateBlock(chain.HeadHeight(), chain.HeadHash(), chain.State().Clone(), &b1))

	bBadHeight := b1
	bBadHeight.Header.Height = 2
	require.Error(t, chain.ValidateBlock(chain.HeadHeight(), chain.HeadHash(), chain.State().Clone(), &bBadHeight))

	bBadPrev := b1
	bBadPrev.Header.PrevHash = []byte("wrong-prev")
	require.Error(t, chain.ValidateBlock(chain.HeadHeight(), chain.HeadHash(), chain.State().Clone(), &bBadPrev))

	bBadRoot := b1
	bBadRoot.Header.TxRoot = make([]byte, len(b1.Header.TxRoot))
	require.Error(t, chain.ValidateBlock(chain.HeadHeight(), chain.HeadHash(), chain.State().Clone(), &bBadRoot))
}

func Test_Namecoin_ValidateBlock_ErrorsOnNilInputs(t *testing.T) {
	var nilChain *NamecoinChain
	require.Error(t, nilChain.ValidateBlock(0, nil, impl.NewState(), nil))

	chain, _ := newTestChain(t)
	require.Error(t, chain.ValidateBlock(chain.HeadHeight(), chain.HeadHash(), chain.State().Clone(), nil))
}

// ---- ApplyBlock tests ----

func Test_Namecoin_ApplyBlock_PersistsBlockAndUpdatesState(t *testing.T) {
	chain, store := newTestChain(t)

	genesis := mustBlock(t, 0, nil)
	require.NoError(t, chain.ApplyBlock(&genesis))
	require.Equal(t, genesis.Header.Height, chain.HeadHeight())
	require.Equal(t, genesis.Hash, chain.HeadHash())

	key := encodeNamecoinBlockKey(genesis.Header.Height)
	raw := store.Get(key)
	require.NotEmpty(t, raw, "expected stored block")

	var decoded types.Block
	require.NoError(t, decoded.Unmarshal(raw))
	require.Equal(t, genesis.Header.Height, decoded.Header.Height)
	require.Equal(t, genesis.Hash, decoded.Hash)
	require.Equal(t, genesis.Hash, store.Get(impl.NamecoinLastBlockKey))
}

func Test_Namecoin_ApplyBlock_InvalidBlockDoesNotMutateStateOrStore(t *testing.T) {
	chain, store := newTestChain(t)

	genesis := mustBlock(t, 0, nil)
	require.NoError(t, chain.ApplyBlock(&genesis))

	beforeLen := store.Len()
	beforeLast := store.Get(impl.NamecoinLastBlockKey)

	b1 := mustBlock(t, 1, chain.HeadHash())
	b1.Header.TxRoot = make([]byte, len(b1.Header.TxRoot)) // invalidate root

	require.Error(t, chain.ApplyBlock(&b1))

	require.Equal(t, genesis.Header.Height, chain.HeadHeight())
	require.Equal(t, genesis.Hash, chain.HeadHash())
	require.Equal(t, beforeLen, store.Len())
	require.Equal(t, beforeLast, store.Get(impl.NamecoinLastBlockKey))
}

func Test_Namecoin_LoadNamecoinChain_RestartRebuildsState(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	chain1, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)

	b0 := mustBlock(t, 0, nil)
	require.NoError(t, chain1.ApplyBlock(&b0))

	b1 := mustBlock(t, 1, chain1.HeadHash())
	require.NoError(t, chain1.ApplyBlock(&b1))

	chain2, err := impl.LoadNamecoinChain(store)
	require.NoError(t, err)

	require.Equal(t, chain1.HeadHeight(), chain2.HeadHeight())
	require.Equal(t, chain1.HeadHash(), chain2.HeadHash())
}

// -----------------------------------------------------------------------------//
// Helpers

func mustBlock(t *testing.T, height uint64, prevHash []byte, txs ...types.Tx) types.Block {
	t.Helper()

	root := mustTxRoot(t, txs)
	blk := types.Block{
		Header: types.BlockHeader{
			Height:   height,
			PrevHash: append([]byte(nil), prevHash...),
			TxRoot:   root,
		},
		Transactions: txs,
	}

	h := sha256.New()
	h.Write([]byte{byte(height)})
	h.Write(root)
	h.Write(blk.Header.PrevHash)
	blk.Hash = h.Sum(nil)

	return blk
}

func mustTxRoot(t *testing.T, txs []types.Tx) []byte {
	t.Helper()

	h := sha256.New()
	for i := range txs {
		b, err := impl.SerializeTransaction(&txs[i])
		require.NoError(t, err)
		_, err = h.Write(b)
		require.NoError(t, err)
	}
	return h.Sum(nil)
}

func encodeNamecoinBlockKey(height uint64) string {
	return impl.NamecoinBlockPrefix + fmt.Sprintf("%020d", height)
}

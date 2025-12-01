package unit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/storage/inmemory"
	"go.dedis.ch/cs438/types"
)

func mustBlock(t *testing.T, height uint64, prevHash []byte, txs ...types.Tx) types.Block {
	t.Helper()
	root, err := computeTxRoot(txs) // this is now calling chain.computeTxRoot
	require.NoError(t, err)

	b := types.Block{
		Header: types.BlockHeader{
			Height:   height,
			PrevHash: append([]byte(nil), prevHash...),
			TxRoot:   root,
		},
		Transactions: txs,
	}

	// Simple deterministic hash for testing: H(height || txRoot)
	h := sha256.New()
	h.Write([]byte{byte(height)})
	h.Write(root)
	b.Header.Hash = h.Sum(nil)

	return b
}

func newTestChain(t *testing.T) (*NamecoinChain, storage.Store) {
	t.Helper()
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()
	chain, err := LoadNamecoinChain(store)
	require.NoError(t, err)
	require.NotNil(t, chain)
	require.NotNil(t, chain.State())
	return chain, store
}

// --- LoadNamecoinChain tests ---

func Test_Namecoin_LoadNamecoinChain_EmptyStore(t *testing.T) {
	chain, store := newTestChain(t)

	require.Empty(t, chain.State().Domains)
	require.Nil(t, chain.HeadHash())
	require.Equal(t, uint64(0), chain.HeadHeight())
	require.Empty(t, store.Get(NamecoinLastBlockKey))
}

func Test_Namecoin_LoadNamecoinChain_SingleBlock(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	// Build a genesis block that registers one domain
	tx := types.Tx{
		ID: []byte("tx1"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "example.bit",
			Value: "1.2.3.4",
		},
	}
	genesis := mustBlock(t, 0, nil, tx)

	data, err := genesis.Marshal()
	require.NoError(t, err)
	store.Set(encodeNamecoinBlockKey(genesis.Header.Height), data)

	chain, err := LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, genesis.Header.Height, chain.HeadHeight())
	require.Equal(t, genesis.Header.Hash, chain.HeadHash())

	rec, ok := chain.State().Domains["example.bit"]
	require.True(t, ok, "expected domain example.bit in state")
	require.Equal(t, "addr1", rec.Owner)
	require.Equal(t, "1.2.3.4", rec.Value)

	require.Equal(t, genesis.Header.Hash, store.Get(NamecoinLastBlockKey))
}

func Test_Namecoin_LoadNamecoinChain_MultipleBlocksAndPrevHashMismatch(t *testing.T) {
	_, store := newTestChain(t)

	tx0 := types.Tx{
		ID: []byte("tx0"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "example.bit",
			Value: "value0",
		},
	}
	b0 := mustBlock(t, 0, nil, tx0)
	data0, _ := b0.Marshal()
	store.Set(encodeNamecoinBlockKey(b0.Header.Height), data0)

	tx1 := types.Tx{
		ID: []byte("tx1"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "update",
			Name:  "example.bit",
			Value: "value1",
		},
	}
	wrongPrev := []byte("not-the-prev-hash")
	b1Bad := mustBlock(t, 1, wrongPrev, tx1)
	data1Bad, _ := b1Bad.Marshal()
	store.Set(encodeNamecoinBlockKey(b1Bad.Header.Height), data1Bad)

	chain, err := LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, b0.Header.Height, chain.HeadHeight())
	require.Equal(t, b0.Header.Hash, chain.HeadHash())

	rec, ok := chain.State().Domains["example.bit"]
	require.True(t, ok, "expected domain example.bit in state")
	require.Equal(t, "value0", rec.Value, "block 1 must be skipped")
}

func Test_Namecoin_LoadNamecoinChain_RespectsStoredHeadHash(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	tx0 := types.Tx{
		ID: []byte("tx0"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "d.bit",
			Value: "value0",
		},
	}
	b0 := mustBlock(t, 0, nil, tx0)
	data0, _ := b0.Marshal()
	store.Set(encodeNamecoinBlockKey(b0.Header.Height), data0)

	tx1 := types.Tx{
		ID: []byte("tx1"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "update",
			Name:  "d.bit",
			Value: "value1",
		},
	}
	b1 := mustBlock(t, 1, b0.Header.Hash, tx1)
	data1, _ := b1.Marshal()
	store.Set(encodeNamecoinBlockKey(b1.Header.Height), data1)

	store.Set(NamecoinLastBlockKey, b0.Header.Hash)

	chain, err := LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, b0.Header.Height, chain.HeadHeight())
	require.Equal(t, b0.Header.Hash, chain.HeadHash())

	rec, ok := chain.State().Domains["d.bit"]
	require.True(t, ok, "expected domain d.bit in state")
	require.Equal(t, "value0", rec.Value, "stored head should stop before b1")
}

func Test_Namecoin_LoadNamecoinChain_NilStoreError(t *testing.T) {
	var store storage.Store
	chain, err := LoadNamecoinChain(store)
	require.Error(t, err)
	require.Nil(t, chain)
}

func Test_Namecoin_LoadNamecoinChain_StoredHeadNotFoundUpdatesPointer(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	tx0 := types.Tx{
		ID: []byte("tx0"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "fallback.bit",
			Value: "v0",
		},
	}
	b0 := mustBlock(t, 0, nil, tx0)
	data0, _ := b0.Marshal()
	store.Set(encodeNamecoinBlockKey(b0.Header.Height), data0)

	tx1 := types.Tx{
		ID: []byte("tx1"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "update",
			Name:  "fallback.bit",
			Value: "v1",
		},
	}
	b1 := mustBlock(t, 1, b0.Header.Hash, tx1)
	data1, _ := b1.Marshal()
	store.Set(encodeNamecoinBlockKey(b1.Header.Height), data1)

	store.Set(NamecoinLastBlockKey, []byte("missing-head"))

	chain, err := LoadNamecoinChain(store)
	require.NoError(t, err)
	require.Equal(t, b1.Header.Height, chain.HeadHeight())
	require.Equal(t, b1.Header.Hash, chain.HeadHash())
	require.Equal(t, b1.Header.Hash, store.Get(NamecoinLastBlockKey))
}

// --- ApplyNamecoinTx / ApplyNamecoinBlock tests ---

func Test_Namecoin_ApplyNamecoinTx_RegisterUpdateTransferFlow(t *testing.T) {
	state := NewNamecoinState()

	register := types.Tx{
		ID: []byte("tx-register"),
		Payload: types.TxPayload{
			From:  "owner1",
			Op:    "register",
			Name:  "flow.bit",
			Value: "ip1",
		},
	}
	require.NoError(t, ApplyNamecoinTx(state, &register))

	rec := state.Domains["flow.bit"]
	require.Equal(t, "owner1", rec.Owner)
	require.Equal(t, "ip1", rec.Value)

	update := types.Tx{
		ID: []byte("tx-update"),
		Payload: types.TxPayload{
			From:  "owner1",
			Op:    "update",
			Name:  "flow.bit",
			Value: "ip2",
		},
	}
	require.NoError(t, ApplyNamecoinTx(state, &update))
	require.Equal(t, "ip2", state.Domains["flow.bit"].Value)

	transfer := types.Tx{
		ID: []byte("tx-transfer"),
		Payload: types.TxPayload{
			From: "owner1",
			To:   "owner2",
			Op:   "transfer",
			Name: "flow.bit",
		},
	}
	require.NoError(t, ApplyNamecoinTx(state, &transfer))
	require.Equal(t, "owner2", state.Domains["flow.bit"].Owner)

	require.Error(t, ApplyNamecoinTx(nil, &register))
	require.Error(t, ApplyNamecoinTx(state, &types.Tx{}))
}

func Test_Namecoin_ApplyNamecoinBlock_SkipsInvalidTxsButContinues(t *testing.T) {
	state := NewNamecoinState()

	bad := types.Tx{
		ID: []byte("tx-bad"),
		Payload: types.TxPayload{
			From:  "addr",
			Op:    "update",
			Name:  "ghost.bit",
			Value: "noop",
		},
	}
	blk := types.Block{
		Header: types.BlockHeader{Height: 3},
		Transactions: []types.Tx{
			bad,
		},
	}

	require.NoError(t, ApplyNamecoinBlock(state, &blk))
	require.Empty(t, state.Domains)

	require.Error(t, ApplyNamecoinBlock(nil, &blk))
	require.Error(t, ApplyNamecoinBlock(state, nil))
}

// --- ValidateBlock tests ---

func Test_Namecoin_ValidateBlock_GenesisValidAndInvalid(t *testing.T) {
	chain, _ := newTestChain(t)

	tx := types.Tx{
		ID: []byte("txg"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "g.bit",
			Value: "1.2.3.4",
		},
	}
	genesis := mustBlock(t, 0, nil, tx)

	newState, err := chain.ValidateBlock(&genesis)
	require.NoError(t, err)

	_, okLive := chain.State().Domains["g.bit"]
	require.False(t, okLive, "ValidateBlock must not mutate live state")

	rec, okClone := newState.Domains["g.bit"]
	require.True(t, okClone)
	require.Equal(t, "1.2.3.4", rec.Value)

	cases := []struct {
		name   string
		mutate func(b *types.Block)
	}{
		{"bad index", func(b *types.Block) { b.Header.Height = 1 }},
		{"non-empty prevhash", func(b *types.Block) { b.Header.PrevHash = []byte("non-empty") }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			invalid := genesis
			tc.mutate(&invalid)
			_, err := chain.ValidateBlock(&invalid)
			require.Error(t, err)
		})
	}
}

func Test_Namecoin_ValidateBlock_TxRootAndPrevHashChecks(t *testing.T) {
	chain, _ := newTestChain(t)

	tx0 := types.Tx{
		ID: []byte("tx0"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "x.bit",
			Value: "value0",
		},
	}
	genesis := mustBlock(t, 0, nil, tx0)
	require.NoError(t, chain.ApplyBlock(&genesis))

	tx1 := types.Tx{
		ID: []byte("tx1"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "update",
			Name:  "x.bit",
			Value: "value1",
		},
	}
	b1 := mustBlock(t, 1, genesis.Header.Hash, tx1)

	_, err := chain.ValidateBlock(&b1)
	require.NoError(t, err)

	bBadHeight := b1
	bBadHeight.Header.Height = 2
	_, err = chain.ValidateBlock(&bBadHeight)
	require.Error(t, err)

	bBadPrev := b1
	bBadPrev.Header.PrevHash = []byte("wrong-prev")
	_, err = chain.ValidateBlock(&bBadPrev)
	require.Error(t, err)

	bBadRoot := b1
	bBadRoot.Header.TxRoot = make([]byte, len(b1.Header.TxRoot))
	_, err = chain.ValidateBlock(&bBadRoot)
	require.Error(t, err)
}

func Test_Namecoin_ValidateBlock_ErrorsOnNilInputs(t *testing.T) {
	var nilChain *NamecoinChain
	_, err := nilChain.ValidateBlock(&types.Block{})
	require.Error(t, err)

	chain, _ := newTestChain(t)
	_, err = chain.ValidateBlock(nil)
	require.Error(t, err)
}

// --- ApplyBlock tests ---

func Test_Namecoin_ApplyBlock_PersistsBlockAndUpdatesState(t *testing.T) {
	chain, store := newTestChain(t)

	tx := types.Tx{
		ID: []byte("txg"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "z.bit",
			Value: "1.2.3.4",
		},
	}
	genesis := mustBlock(t, 0, nil, tx)

	require.NoError(t, chain.ApplyBlock(&genesis))
	require.Equal(t, genesis.Header.Height, chain.HeadHeight())
	require.Equal(t, genesis.Header.Hash, chain.HeadHash())

	key := encodeNamecoinBlockKey(genesis.Header.Height)
	raw := store.Get(key)
	require.NotEmpty(t, raw, "expected stored block")

	var decoded types.Block
	require.NoError(t, decoded.Unmarshal(raw))
	require.Equal(t, genesis.Header.Height, decoded.Header.Height)
	require.Equal(t, genesis.Header.Hash, decoded.Header.Hash)

	require.Equal(t, genesis.Header.Hash, store.Get(NamecoinLastBlockKey))

	rec, ok := chain.State().Domains["z.bit"]
	require.True(t, ok)
	require.Equal(t, "1.2.3.4", rec.Value)
}

func Test_Namecoin_ApplyBlock_DropsPendingTxs(t *testing.T) {
	chain, _ := newTestChain(t)

	tx0 := types.Tx{
		ID: []byte("tx0"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "p.bit",
			Value: "v0",
		},
	}
	genesis := mustBlock(t, 0, nil, tx0)
	require.NoError(t, chain.ApplyBlock(&genesis))

	tx1 := types.Tx{
		ID: []byte("tx1"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "update",
			Name:  "p.bit",
			Value: "v1",
		},
	}

	addPendingTxForTest(t, chain, &tx1)

	b1 := mustBlock(t, 1, genesis.Header.Hash, tx1)
	require.NoError(t, chain.ApplyBlock(&b1))

	stillPending := hasPendingTxForTest(chain, tx1.ID)
	rec, ok := chain.State().Domains["p.bit"]

	require.False(t, stillPending, "tx must be dropped from pending pool")
	require.True(t, ok)
	require.Equal(t, "v1", rec.Value)
}

func Test_Namecoin_ApplyBlock_InvalidBlockDoesNotMutateStateOrStore(t *testing.T) {
	chain, store := newTestChain(t)

	tx0 := types.Tx{
		ID: []byte("tx0"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "safe.bit",
			Value: "v0",
		},
	}
	genesis := mustBlock(t, 0, nil, tx0)
	require.NoError(t, chain.ApplyBlock(&genesis))

	beforeLen := store.Len()
	beforeLast := store.Get(NamecoinLastBlockKey)

	tx1 := types.Tx{
		ID: []byte("tx1"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "update",
			Name:  "safe.bit",
			Value: "v1",
		},
	}
	b1 := mustBlock(t, 1, genesis.Header.Hash, tx1)
	b1.Header.TxRoot = make([]byte, len(b1.Header.TxRoot)) // invalidate root

	require.Error(t, chain.ApplyBlock(&b1))

	rec, ok := chain.State().Domains["safe.bit"]
	require.True(t, ok)
	require.Equal(t, "v0", rec.Value)
	require.Equal(t, genesis.Header.Height, chain.HeadHeight())
	require.Equal(t, genesis.Header.Hash, chain.HeadHash())

	require.Equal(t, beforeLen, store.Len())
	require.Equal(t, beforeLast, store.Get(NamecoinLastBlockKey))
}

func Test_Namecoin_ApplyBlock_ErrorsOnNilInputs(t *testing.T) {
	var nilChain *NamecoinChain
	err := nilChain.ApplyBlock(&types.Block{})
	require.Error(t, err)

	chain, _ := newTestChain(t)
	err = chain.ApplyBlock(nil)
	require.Error(t, err)
}

func Test_Namecoin_LoadNamecoinChain_RestartRebuildsState(t *testing.T) {
	p := inmemory.NewPersistency()
	store := p.GetBlockchainStore()

	chain1, err := LoadNamecoinChain(store)
	require.NoError(t, err)

	tx0 := types.Tx{
		ID: []byte("tx0"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "register",
			Name:  "restart.bit",
			Value: "v0",
		},
	}
	b0 := mustBlock(t, 0, nil, tx0)
	require.NoError(t, chain1.ApplyBlock(&b0))

	tx1 := types.Tx{
		ID: []byte("tx1"),
		Payload: types.TxPayload{
			From:  "addr1",
			Op:    "update",
			Name:  "restart.bit",
			Value: "v1",
		},
	}
	b1 := mustBlock(t, 1, b0.Header.Hash, tx1)
	require.NoError(t, chain1.ApplyBlock(&b1))

	chain2, err := LoadNamecoinChain(store)
	require.NoError(t, err)

	require.Equal(t, chain1.HeadHeight(), chain2.HeadHeight())
	require.Equal(t, chain1.HeadHash(), chain2.HeadHash())

	rec1, ok1 := chain1.State().Domains["restart.bit"]
	rec2, ok2 := chain2.State().Domains["restart.bit"]
	require.True(t, ok1 && ok2)
	require.Equal(t, rec1.Owner, rec2.Owner)
	require.Equal(t, rec1.Value, rec2.Value)
}

func addPendingTxForTest(t *testing.T, chain *NamecoinChain, tx *types.Tx) {
	t.Helper()
	require.NotNil(t, chain)
	require.NotNil(t, tx)

	st := chain.State()
	require.NotNil(t, st)
	require.NotNil(t, st.Pending, "pending pool must exist")
	require.NoError(t, st.Pending.Add(tx))
}

func hasPendingTxForTest(chain *NamecoinChain, txID []byte) bool {
	if chain == nil || txID == nil {
		return false
	}
	st := chain.State()
	if st == nil || st.Pending == nil {
		return false
	}
	target := string(txID)
	for _, pending := range st.Pending.SnapshotPending() {
		if pending != nil && string(pending.ID) == target {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Utility functions

// computeTxRoot recomputes the txRoot from the block's transactions
// MVP: SHA-256 over the JSON encoding of each tx in order.
func computeTxRoot(txs []types.Tx) ([]byte, error) {
	h := sha256.New()
	for i := range txs {
		b, err := json.Marshal(txs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tx %d for root: %w", i, err)
		}
		if _, err := h.Write(b); err != nil {
			return nil, fmt.Errorf("failed to hash tx %d: %w", i, err)
		}
	}
	return h.Sum(nil), nil
}

// encodeNamecoinBlockKey builds the key for a given height
func encodeNamecoinBlockKey(height uint64) string {
	return NamecoinBlockPrefix + fmt.Sprintf("%020d", height)
}

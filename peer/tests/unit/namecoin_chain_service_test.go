package unit

import (
	"bytes"
	"math/big"
	"sync"
	"testing"

	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/types"
)

// mapStore is a simple in-memory store for tests.
type mapStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMapStore() *mapStore {
	return &mapStore{data: make(map[string][]byte)}
}

func (s *mapStore) Get(key string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return impl.CloneBytes(s.data[key])
}

func (s *mapStore) Set(key string, val []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = impl.CloneBytes(val)
}

func (s *mapStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *mapStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data)
}

func (s *mapStore) ForEach(f func(key string, val []byte) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.data {
		if !f(k, v) {
			return
		}
	}
}

func Test_NamecoinChainServicePromotesBranchAndCommitsOverlay(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil)
	if err := chain.ApplyBlock(genesis); err != nil {
		t.Fatalf("apply genesis: %v", err)
	}
	mainBlock := makeBlock(1, genesis.Hash)
	if err := chain.ApplyBlock(mainBlock); err != nil {
		t.Fatalf("apply main block: %v", err)
	}

	svc := impl.NewChainService(chain)

	if _, err := svc.ReplayLongestChainBack(genesis.Hash); err != nil {
		t.Fatalf("expected genesis to be indexed, got error: %v", err)
	}

	// Competing fork that grows longer should be promoted.
	forkBlock := makeBlock(1, genesis.Hash, 1)
	if _, err := svc.AppendBlockToChain(0, forkBlock); err != nil {
		t.Fatalf("append fork block: %v", err)
	}
	if len(svc.Chains) != 2 {
		t.Fatalf("expected fork chain to be created, got %d chains", len(svc.Chains))
	}
	forkBlock2 := makeBlock(2, forkBlock.Hash, 2)
	changed, err := svc.AppendBlockToChain(0, forkBlock2)
	if err != nil {
		t.Fatalf("append fork block2: %v", err)
	}
	if !changed {
		t.Fatalf("expected promotion after longer fork (head=%x height=%d)", svc.GetLongestChain().HeadHash(), svc.GetLongestChain().HeadHeight())
	}

	if svc.LongestChainIndex == 0 {
		h0 := svc.Chains[0].HeadHash()
		hh0 := svc.Chains[0].HeadHeight()
		var h1 []byte
		var hh1 uint64
		if len(svc.Chains) > 1 {
			h1 = svc.Chains[1].HeadHash()
			hh1 = svc.Chains[1].HeadHeight()
		}
		t.Fatalf("expected fork chain to be promoted (chains=%d idx=0 head=%x h=%d idx1 head=%x h=%d)", len(svc.Chains), h0, hh0, h1, hh1)
	}
	longest := svc.Chains[svc.LongestChainIndex]
	if !bytes.Equal(longest.HeadHash(), forkBlock2.Hash) {
		t.Fatalf("expected longest head to be fork block2, got %x", longest.HeadHash())
	}

	// The overlay must have been committed so the base store now contains the fork block.
	data := store.Get(impl.EncodeNamecoinBlockKey(2))
	if len(data) == 0 {
		t.Fatalf("expected block at height 2 in base store")
	}
	var stored types.Block
	if err := stored.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal stored block: %v", err)
	}
	if !bytes.Equal(stored.Hash, forkBlock2.Hash) {
		t.Fatalf("expected stored block hash %x, got %x", forkBlock2.Hash, stored.Hash)
	}
}

func Test_NamecoinChainServiceKeepsBaseOnShorterFork(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil)
	if err := chain.ApplyBlock(genesis); err != nil {
		t.Fatalf("apply genesis: %v", err)
	}
	mainBlock1 := makeBlock(1, genesis.Hash)
	if err := chain.ApplyBlock(mainBlock1); err != nil {
		t.Fatalf("apply main block1: %v", err)
	}

	svc := impl.NewChainService(chain)

	mainBlock2 := makeBlock(2, mainBlock1.Hash)
	if err := chain.ApplyBlock(mainBlock2); err != nil {
		t.Fatalf("apply main block2: %v", err)
	}

	// Shorter competing fork at height 1 should not be promoted.
	forkBlock := makeBlock(1, genesis.Hash, 1)
	changed, err := svc.AppendBlockToChain(0, forkBlock)
	if err != nil {
		t.Fatalf("append fork block: %v", err)
	}
	if changed {
		t.Fatalf("expected no promotion for shorter fork")
	}

	if svc.LongestChainIndex != 0 {
		t.Fatalf("unexpected promotion of shorter fork")
	}

	// Base store should still hold the original main block at height 1.
	data := store.Get(impl.EncodeNamecoinBlockKey(1))
	if len(data) == 0 {
		t.Fatalf("expected block at height 1 in base store")
	}
	var stored types.Block
	if err := stored.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal stored block: %v", err)
	}
	if !bytes.Equal(stored.Hash, mainBlock1.Hash) {
		t.Fatalf("expected stored hash %x, got %x", mainBlock1.Hash, stored.Hash)
	}
}

func Test_NamecoinGenesisForkCreatesBranchWithoutPromotion(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil)
	if err := chain.ApplyBlock(genesis); err != nil {
		t.Fatalf("apply genesis: %v", err)
	}
	mainBlock := makeBlock(1, genesis.Hash)
	if err := chain.ApplyBlock(mainBlock); err != nil {
		t.Fatalf("apply main block: %v", err)
	}

	svc := impl.NewChainService(chain)

	altGenesis := makeBlock(0, nil, 1) // competing genesis
	if changed, err := svc.AppendBlockToChain(0, altGenesis); err != nil {
		t.Fatalf("append competing genesis: %v", err)
	} else if changed {
		t.Fatalf("unexpected canonical change on competing genesis")
	}

	if len(svc.Chains) != 2 {
		t.Fatalf("expected a new branch chain, got %d", len(svc.Chains))
	}
	if svc.LongestChainIndex != 0 {
		t.Fatalf("unexpected promotion on shorter/equal genesis fork")
	}
	if svc.Chains[1].HeadHeight() != 0 {
		t.Fatalf("expected branch head height 0, got %d", svc.Chains[1].HeadHeight())
	}

	// Base store should still contain the main block at height 1.
	data := store.Get(impl.EncodeNamecoinBlockKey(1))
	if len(data) == 0 {
		t.Fatalf("expected main block at height 1 in base store")
	}
	var stored types.Block
	if err := stored.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal main block: %v", err)
	}
	if !bytes.Equal(stored.Hash, mainBlock.Hash) {
		t.Fatalf("expected main block hash %x, got %x", mainBlock.Hash, stored.Hash)
	}
}

func Test_NamecoinReplayLongestChainBackFailsWhenOriginMissing(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil)
	if err := chain.ApplyBlock(genesis); err != nil {
		t.Fatalf("apply genesis: %v", err)
	}

	svc := impl.NewChainService(chain)

	if _, err := svc.ReplayLongestChainBack([]byte("missing")); err == nil {
		t.Fatalf("expected error when origin not found")
	}
	if len(svc.Chains) != 1 {
		t.Fatalf("unexpected chain added on failure")
	}
}

func Test_NamecoinPromotedBranchUsesBaseStoreAfterCommit(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil)
	if err := chain.ApplyBlock(genesis); err != nil {
		t.Fatalf("apply genesis: %v", err)
	}
	mainBlock := makeBlock(1, genesis.Hash)
	if err := chain.ApplyBlock(mainBlock); err != nil {
		t.Fatalf("apply main block: %v", err)
	}

	svc := impl.NewChainService(chain)

	forkBlock1 := makeBlock(1, genesis.Hash, 1)
	if _, err := svc.AppendBlockToChain(0, forkBlock1); err != nil {
		t.Fatalf("append fork block: %v", err)
	}
	branchIdx := svc.LongestChainIndex

	forkBlock2 := makeBlock(2, forkBlock1.Hash, 2)
	if changed, err := svc.AppendBlockToChain(branchIdx, forkBlock2); err != nil {
		t.Fatalf("append fork block2: %v", err)
	} else if !changed {
		t.Fatalf("expected canonical change after deeper fork")
	}

	// After promotion, writes should hit the base store; check block at height 2.
	data := store.Get(impl.EncodeNamecoinBlockKey(2))
	if len(data) == 0 {
		t.Fatalf("expected block at height 2 in base store after promotion")
	}
	var stored types.Block
	if err := stored.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal block2: %v", err)
	}
	if !bytes.Equal(stored.Hash, forkBlock2.Hash) {
		t.Fatalf("expected stored hash %x, got %x", forkBlock2.Hash, stored.Hash)
	}
}

func Test_NamecoinChainServiceProcessesOrphans(t *testing.T) {
	store := newMapStore()
	chain := newTestChain(store)

	genesis := makeBlock(0, nil)
	if err := chain.ApplyBlock(genesis); err != nil {
		t.Fatalf("apply genesis: %v", err)
	}

	svc := impl.NewChainService(chain)

	parent := makeBlock(1, genesis.Hash, 1)
	child := makeBlock(2, parent.Hash, 2)

	// First, deliver child before parent; should be stored as orphan and not change canonical head.
	changed, err := svc.AppendBlockToChain(0, child)
	if err != nil {
		t.Fatalf("append orphan child: %v", err)
	}
	if changed {
		t.Fatalf("unexpected canonical change when parent missing")
	}
	if svc.GetLongestChain().HeadHeight() != 0 {
		t.Fatalf("expected head to remain at genesis, got %d", svc.GetLongestChain().HeadHeight())
	}

	// Now deliver the parent; it should attach and drain the orphan child.
	changed, err = svc.AppendBlockToChain(0, parent)
	if err != nil {
		t.Fatalf("append parent: %v", err)
	}
	if !changed {
		t.Fatalf("expected canonical change after attaching parent and child")
	}

	if svc.GetLongestChain().HeadHeight() != 2 {
		t.Fatalf("expected head height 2 after draining orphan, got %d", svc.GetLongestChain().HeadHeight())
	}
	if !bytes.Equal(svc.GetLongestChain().HeadHash(), child.Hash) {
		t.Fatalf("expected head hash to be child, got %x", svc.GetLongestChain().HeadHash())
	}
}

func newTestChain(store storage.Store) *impl.NamecoinChain {
	c := impl.NewNamecoinChain(store)
	c.SetPowTarget(new(big.Int).Lsh(big.NewInt(1), 256)) // accept any hash
	return c
}

func makeBlock(height uint64, prev []byte, nonce ...uint64) *types.Block {
	n := uint64(0)
	if len(nonce) > 0 {
		n = nonce[0]
	}
	b := types.Block{
		Header: types.BlockHeader{
			Height:   height,
			PrevHash: impl.CloneBytes(prev),
			Nonce:    n,
		},
	}
	txRoot, err := impl.ComputeTxRoot(nil)
	if err != nil {
		panic(err)
	}
	b.Header.TxRoot = txRoot
	b.Hash = b.ComputeHash()
	return &b
}

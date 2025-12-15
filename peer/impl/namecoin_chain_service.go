package impl

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/types"
)

func NewChainService(chain *NamecoinChain) *ChainService {
	cs := &ChainService{
		Chains:            []*NamecoinChain{chain},
		LongestChainIndex: 0,
		orphansByPrev:     make(map[string][]*types.Block),
		blockIndex:        make(map[string]*blockMeta),
	}
	cs.indexChainUnlocked(chain)
	return cs
}

type ChainService struct {
	Chains            []*NamecoinChain
	LongestChainIndex int
	mu                sync.RWMutex

	// orphansByPrev maps PrevHash(hex) -> blocks waiting for that parent.
	orphansByPrev map[string][]*types.Block

	// blockIndex keeps every known block by hash (across forks) with its height and chains.
	blockIndex map[string]*blockMeta
}

type blockMeta struct {
	height uint64
	chains map[*NamecoinChain]struct{}
}

// -------------------- Public API --------------------

func (s *ChainService) AppendChain(chain *NamecoinChain) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Chains = append(s.Chains, chain)
	s.indexChainUnlocked(chain)
}

func (s *ChainService) AppendBlockToLongestChain(block *types.Block) (changedCanonical bool, err error) {
	s.mu.RLock()
	idx := s.LongestChainIndex
	s.mu.RUnlock()
	return s.AppendBlockToChain(idx, block)
}

func (s *ChainService) GetLongestChain() *NamecoinChain {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Chains[s.LongestChainIndex]
}

func (s *ChainService) HeadSnapshot() (hash []byte, height uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chain := s.Chains[s.LongestChainIndex]
	return CloneBytes(chain.headHash), chain.headHeight
}

// ReplayLongestChainBack forks the current longest chain back to the given origin hash.
// Returns ErrUnknownParent if the origin hash is not known.
func (s *ChainService) ReplayLongestChainBack(originHash []byte) (*NamecoinChain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if originHash == nil {
		return nil, ErrUnknownParent
	}

	meta, ok := s.blockIndex[hashKeyHex(originHash)]
	if !ok || meta == nil {
		return nil, ErrUnknownParent
	}

	var base *NamecoinChain
	for ch := range meta.chains {
		base = ch
		break
	}
	if base == nil && s.LongestChainIndex < len(s.Chains) {
		base = s.Chains[s.LongestChainIndex]
	}
	if base == nil {
		return nil, ErrUnknownParent
	}

	// If the head already matches, return it directly.
	if bytes.Equal(base.headHash, originHash) {
		return base, nil
	}

	fork, err := base.forkUpToHeight(meta.height, newOverlayStore(base.store))
	if err != nil {
		return nil, err
	}
	s.Chains = append(s.Chains, fork)
	s.indexChainUnlocked(fork)
	return fork, nil
}

// AppendBlockToChain tries to incorporate block into our known DAG.
// Returns changedCanonical=true iff the *canonical head* (longest chain head) changed.
func (s *ChainService) AppendBlockToChain(
	index int,
	block *types.Block,
) (bool, error) {

	s.mu.Lock()
	defer s.mu.Unlock()

	beforeHash := CloneBytes(s.Chains[s.LongestChainIndex].headHash)
	beforeHeight := s.Chains[s.LongestChainIndex].headHeight

	firstErr := s.processPendingBlocksUnlocked(index, block)

	changed := s.canonicalChangedUnlocked(beforeHash, beforeHeight)
	return changed, firstErr
}

func (s *ChainService) processPendingBlocksUnlocked(
	index int,
	initial *types.Block,
) error {

	pending := []*types.Block{initial}
	var firstErr error

	for len(pending) > 0 {
		blk := pending[0]
		pending = pending[1:]

		children, err := s.processSingleBlockUnlocked(index, blk)
		if err != nil && firstErr == nil {
			firstErr = err
		}

		pending = append(pending, children...)
	}

	return firstErr
}

func (s *ChainService) processSingleBlockUnlocked(
	index int,
	blk *types.Block,
) ([]*types.Block, error) {

	if blk == nil {
		return nil, nil
	}

	// Already known → just drain waiting children
	if s.blockKnownUnlocked(blk.Hash, blk.Header.Height) {
		return s.popWaitingChildrenUnlocked(blk.Hash), nil
	}

	origin, err := s.ensureChainForParentUnlocked(index, blk)
	if err != nil {
		return s.handleParentErrorUnlocked(blk, err)
	}

	if err := origin.ApplyBlock(blk); err != nil {
		return s.handleApplyErrorUnlocked(blk, err)
	}

	// Successfully attached
	s.registerBlockUnlocked(origin, blk)
	s.maybePromoteLongest(origin)

	return s.popWaitingChildrenUnlocked(blk.Hash), nil
}

func (s *ChainService) handleParentErrorUnlocked(
	blk *types.Block,
	err error,
) ([]*types.Block, error) {

	if errors.Is(err, ErrUnknownParent) {
		s.storeOrphanUnlocked(blk)
		return nil, nil
	}
	return nil, err
}

func (s *ChainService) handleApplyErrorUnlocked(
	blk *types.Block,
	err error,
) ([]*types.Block, error) {

	var fe *ChainFollowError
	if errors.As(err, &fe) {
		s.storeOrphanUnlocked(blk)
		return nil, nil
	}
	return nil, err
}

func (s *ChainService) canonicalChangedUnlocked(beforeHash []byte, beforeHeight uint64) bool {
	after := s.Chains[s.LongestChainIndex]
	if beforeHeight != after.headHeight {
		return true
	}
	return !bytes.Equal(beforeHash, after.headHash)
}

// -------------------- Orphans --------------------

var ErrUnknownParent = errors.New("unknown parent")

func hashKeyHex(b []byte) string {
	if b == nil {
		return ""
	}
	return fmt.Sprintf("%x", b)
}

func (s *ChainService) storeOrphanUnlocked(block *types.Block) {
	// Genesis blocks shouldn't be treated as orphans.
	if block == nil || block.Header.Height == 0 || block.Header.PrevHash == nil {
		return
	}

	// IMPORTANT: do not store caller-owned pointer; make a copy.
	bc := cloneBlock(block)

	k := hashKeyHex(bc.Header.PrevHash)
	s.orphansByPrev[k] = append(s.orphansByPrev[k], bc)
}

// popWaitingChildrenUnlocked returns and clears children that were waiting for parentHash.
func (s *ChainService) popWaitingChildrenUnlocked(parentHash []byte) []*types.Block {
	if parentHash == nil {
		return nil
	}
	k := hashKeyHex(parentHash)
	waiting := s.orphansByPrev[k]
	if len(waiting) == 0 {
		return nil
	}
	delete(s.orphansByPrev, k)
	return waiting
}

// -------------------- Parent lookup / fork creation --------------------

// ensureChainForParentUnlocked returns a chain whose head hash equals block.Header.PrevHash,
// creating a fork if the parent exists in the middle of some chain. Returns ErrUnknownParent
// if we don't know the parent yet.
func (s *ChainService) ensureChainForParentUnlocked(
	index int,
	blk *types.Block,
) (*NamecoinChain, error) {

	if blk == nil {
		return nil, fmt.Errorf("nil block")
	}

	if blk.Header.Height == 0 {
		return s.handleGenesisUnlocked(index)
	}

	parentHash := blk.Header.PrevHash
	if parentHash == nil {
		return nil, ErrUnknownParent
	}

	// 1) Parent already a chain head
	if ch := s.findHeadByHashUnlocked(parentHash); ch != nil {
		return ch, nil
	}

	// 2) Parent must be indexed
	meta, err := s.getParentMetaUnlocked(parentHash)
	if err != nil {
		return nil, err
	}

	// 3) Reuse or fork deterministically
	return s.forkFromParentMetaUnlocked(meta)
}

// -------------------- Longest-chain selection --------------------

func (s *ChainService) newEmptyChainFrom(ref *NamecoinChain) *NamecoinChain {
	state := NewState()
	state.SetLogger(ref.state.CloneLogger())
	return &NamecoinChain{
		store:      newOverlayStore(ref.store),
		state:      state,
		headHash:   nil,
		headHeight: 0,
		powTarget:  ref.powTargetSnapshot(),
	}
}

func (s *ChainService) maybePromoteLongest(candidate *NamecoinChain) {
	current := s.Chains[s.LongestChainIndex]

	candHeight := candidate.HeadHeight()
	currHeight := current.HeadHeight()

	// Promote if strictly longer, or if same height but with a lexicographically
	// smaller hash to deterministically break ties and allow convergence.
	if candHeight < currHeight {
		return
	}
	if candHeight == currHeight {
		candHash := candidate.HeadHash()
		currHash := current.HeadHash()
		if currHash != nil && candHash != nil && bytes.Compare(candHash, currHash) >= 0 {
			return
		}
		// If either hash is nil, fall through and promote to make progress.
	}

	if ov, ok := candidate.store.(*overlayStore); ok {
		ov.Commit()
		candidate.store = ov.base
	}

	for i, ch := range s.Chains {
		if ch == candidate {
			s.LongestChainIndex = i
			return
		}
	}
}

// -------------------- Block indexing helpers --------------------

func (s *ChainService) registerBlockUnlocked(chain *NamecoinChain, blk *types.Block) {
	if blk == nil || blk.Hash == nil {
		return
	}

	key := hashKeyHex(blk.Hash)
	meta, ok := s.blockIndex[key]
	if !ok || meta == nil {
		meta = &blockMeta{
			height: blk.Header.Height,
			chains: make(map[*NamecoinChain]struct{}),
		}
		s.blockIndex[key] = meta
	}
	meta.chains[chain] = struct{}{}
}

func (s *ChainService) blockKnownUnlocked(hash []byte, height uint64) bool {
	if hash == nil {
		return false
	}
	meta, ok := s.blockIndex[hashKeyHex(hash)]
	if !ok || meta == nil {
		return false
	}
	return meta.height == height
}

func (s *ChainService) indexChainUnlocked(chain *NamecoinChain) {
	if chain == nil {
		return
	}
	head := chain.HeadHeight()
	for h := uint64(0); h <= head; h++ {
		blk, err := chain.blockAtHeight(h)
		if err != nil || blk == nil {
			continue
		}
		s.registerBlockUnlocked(chain, blk)
	}
}

func (s *ChainService) handleGenesisUnlocked(index int) (*NamecoinChain, error) {
	for _, c := range s.Chains {
		if c.headHash == nil && c.headHeight == 0 {
			return c, nil
		}
	}

	if index < 0 || index >= len(s.Chains) {
		return nil, ErrUnknownParent
	}

	origin := s.newEmptyChainFrom(s.Chains[index])
	s.Chains = append(s.Chains, origin)
	return origin, nil
}

func (s *ChainService) findHeadByHashUnlocked(hash []byte) *NamecoinChain {
	for _, c := range s.Chains {
		if bytes.Equal(c.headHash, hash) {
			return c
		}
	}
	return nil
}

func (s *ChainService) getParentMetaUnlocked(
	parentHash []byte,
) (*blockMeta, error) {

	meta, ok := s.blockIndex[hashKeyHex(parentHash)]
	if !ok || meta == nil {
		return nil, ErrUnknownParent
	}
	return meta, nil
}

func (s *ChainService) forkFromParentMetaUnlocked(
	meta *blockMeta,
) (*NamecoinChain, error) {

	parentHeight := meta.height

	// Pick deterministic base (longest chain containing parent)
	var base *NamecoinChain
	for ch := range meta.chains {
		if base == nil || ch.HeadHeight() > base.HeadHeight() {
			base = ch
		}
	}

	if base == nil {
		return nil, ErrUnknownParent
	}

	origin, err := base.forkUpToHeight(
		parentHeight,
		newOverlayStore(base.store),
	)
	if err != nil {
		return nil, err
	}

	s.Chains = append(s.Chains, origin)
	s.indexChainUnlocked(origin)

	return origin, nil
}

func cloneBlock(b *types.Block) *types.Block {
	if b == nil {
		return nil
	}
	cp := *b
	cp.Header.PrevHash = CloneBytes(b.Header.PrevHash)
	cp.Header.TxRoot = CloneBytes(b.Header.TxRoot)
	cp.Header.Difficulty = CloneBytes(b.Header.Difficulty)
	cp.Hash = CloneBytes(b.Hash)
	cp.Transactions = append([]types.Tx(nil), b.Transactions...)
	return &cp
}

// -------------------- overlayStore --------------------

// overlayStore reads through to base but keeps branch writes in-memory until committed.
type overlayStore struct {
	base storage.Store
	mu   sync.Mutex
	data map[string][]byte
	del  map[string]struct{}
}

func newOverlayStore(base storage.Store) *overlayStore {
	return &overlayStore{
		base: base,
		data: make(map[string][]byte),
		del:  make(map[string]struct{}),
	}
}

func (s *overlayStore) Get(key string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.del[key]; ok {
		return nil
	}
	if v, ok := s.data[key]; ok {
		return CloneBytes(v)
	}
	return s.base.Get(key)
}

func (s *overlayStore) Set(key string, val []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.del, key)
	s.data[key] = CloneBytes(val)
}

func (s *overlayStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	s.del[key] = struct{}{}
}

func (s *overlayStore) Len() int {
	count := 0
	s.ForEach(func(key string, val []byte) bool {
		count++
		return true
	})
	return count
}

func (s *overlayStore) ForEach(f func(key string, val []byte) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := make(map[string]struct{}, len(s.data)+len(s.del))
	for k, v := range s.data {
		seen[k] = struct{}{}
		if !f(k, CloneBytes(v)) {
			return
		}
	}
	s.base.ForEach(func(key string, val []byte) bool {
		if _, skip := s.del[key]; skip {
			return true
		}
		if _, done := seen[key]; done {
			return true
		}
		seen[key] = struct{}{}
		return f(key, val)
	})
}

// Commit flushes overlay writes to base and clears the overlay.
func (s *overlayStore) Commit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.del {
		s.base.Delete(k)
	}
	for k, v := range s.data {
		s.base.Set(k, v)
	}
	s.data = make(map[string][]byte)
	s.del = make(map[string]struct{})
}

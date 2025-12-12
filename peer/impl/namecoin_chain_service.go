package impl

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/types"
)

type ChainService struct {
	Chains            []*NamecoinChain
	LongestChainIndex int
	mu                sync.RWMutex
}

func (s *ChainService) AppendChain(chain *NamecoinChain) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Chains = append(s.Chains, chain)
}

func (s *ChainService) AppendBlockToLongestChain(block *types.Block) error {
	return s.AppendBlockToChain(s.LongestChainIndex, block)
}

func (s *ChainService) AppendBlockToChain(index int, block *types.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.Chains[index].ApplyBlock(block)

	var ve *ChainFollowError
	if !errors.As(err, &ve) {
		return err
	}

	// if we receive ChainFollowError then we assume that there was another branch created.
	// it can have an undefined number of blocks after the branch was discovered (network partition).
	// we check if there is a branch, head block of which is ancestor for incoming.

	originChain, err := s.GetOriginChain(index, block)
	if err != nil {
		return err
	}

	if err := originChain.ApplyBlock(block); err != nil {
		return err
	}

	s.maybePromoteLongest(originChain)
	return nil
}

func (s *ChainService) GetOriginChain(index int, block *types.Block) (*NamecoinChain, error) {
	if block.Header.Height == 0 {
		// New competing genesis: spin up an empty chain to attach this block.
		originChain := s.newEmptyChainFrom(s.Chains[index])
		s.Chains = append(s.Chains, originChain)

		return originChain, nil
	}

	for _, chain := range s.Chains {
		if bytes.Equal(chain.headHash, block.Header.PrevHash) {
			return chain, nil
		}
	}

	// replay back main branch
	originChain, err := s.ReplayLongestChainBack(block.Header.PrevHash)

	if originChain == nil {
		return nil, err
	}

	return originChain, nil
}

func (s *ChainService) ReplayLongestChainBack(prevHash []byte) (*NamecoinChain, error) {
	longestChain := s.Chains[s.LongestChainIndex]
	startHeight := longestChain.HeadHeight()

	if startHeight == 0 {
		return nil, fmt.Errorf("height is already 0")
	}

	for h := startHeight - 1; ; {
		blkAtHeight, blkErr := longestChain.blockAtHeight(h)
		if blkErr != nil {
			return nil, blkErr
		}

		if bytes.Equal(blkAtHeight.Hash, prevHash) {
			originChain, blkErr := longestChain.forkUpToHeight(h, newOverlayStore(longestChain.store))
			if blkErr != nil {
				return nil, blkErr
			}
			s.Chains = append(s.Chains, originChain)
			return originChain, nil
		}

		if h == 0 {
			break
		}
		h--
	}

	return nil, fmt.Errorf("block origin has not been found")
}

func (s *ChainService) newEmptyChainFrom(ref *NamecoinChain) *NamecoinChain {
	return &NamecoinChain{
		store:      newOverlayStore(ref.store),
		state:      NewState(),
		headHash:   nil,
		headHeight: 0,
		powTarget:  ref.powTargetSnapshot(),
	}
}

func (s *ChainService) maybePromoteLongest(candidate *NamecoinChain) {
	currentLongest := s.Chains[s.LongestChainIndex]
	if candidate.HeadHeight() < currentLongest.HeadHeight() {
		return
	}

	if ov, ok := candidate.store.(*overlayStore); ok {
		ov.Commit()
		candidate.store = ov.base
	}

	for i, chain := range s.Chains {
		if chain == candidate {
			s.LongestChainIndex = i
			break
		}
	}
}

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

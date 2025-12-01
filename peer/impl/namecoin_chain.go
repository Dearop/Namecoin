package impl

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/types"
)

// Isolation prefixes for Namecoin in the blockchain store
var (
	NamecoinBlockPrefix = "namecoin:block:"

	// NamecoinLastBlockKey stores the hash of the last applied Namecoin block
	// at the time of the last successful replay
	NamecoinLastBlockKey = "namecoin:last_block"
)

// NamecoinChain represents the local Namecoin chain for a node
type NamecoinChain struct {
	mu         sync.RWMutex
	store      storage.Store
	state      *NamecoinState
	headHash   []byte
	headHeight uint64
}

func (c *NamecoinChain) HeadHash() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]byte(nil), c.headHash...)
}

func (c *NamecoinChain) HeadHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.headHeight
}

// ---- Helper utils ----
// loadedBlock is used only during chain replay to sort blocks by height and
// keep both the key and raw bytes on hand
type loadedBlock struct {
	Height uint64
	Key    string
	Raw    []byte
}

// quality of life - warnf logs only if GLOG != "no"
func warnf(format string, args ...interface{}) {
	if os.Getenv("GLOG") == "no" {
		return
	}
	log.Printf(format, args...)
}

// encodeNamecoinBlockKey builds the key for a given height
func encodeNamecoinBlockKey(height uint64) string {
	return NamecoinBlockPrefix + fmt.Sprintf("%020d", height)
}

// parseNamecoinBlockHeight extracts the height from a block key
func parseNamecoinBlockHeight(key string) (uint64, error) {
	if !strings.HasPrefix(key, NamecoinBlockPrefix) {
		return 0, fmt.Errorf("not a namecoin block key")
	}

	suffix := strings.TrimPrefix(key, NamecoinBlockPrefix)
	if len(suffix) == 0 {
		return 0, fmt.Errorf("invalid namecoin block key length %d", len(key))
	}

	h, err := strconv.ParseUint(suffix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid height suffix %q: %w", suffix, err)
	}
	return h, nil
}

// ---- Helpers ----
// computeTxRoot recomputes the txRoot from the block's transactions
// MVP: SHA-256 over the JSON encoding of each tx in order.
func computeTxRoot(txs []types.Tx) ([]byte, error) {
	h := sha256.New()
	for i := range txs {
		b, err := SerializeTransaction(&txs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tx %d for root: %w", i, err)
		}
		if _, err = h.Write(b); err != nil {
			return nil, fmt.Errorf("failed to hash tx %d: %w", i, err)
		}
	}
	return h.Sum(nil), nil
}

// ---- Core Chain Loader ----
// LoadNamecoinChain replays all Namecoin blocks from the blockchain store
// and reconstructs local NamecoinState
func LoadNamecoinChain(store storage.Store) (*NamecoinChain, error) {

	state := NewState()

	// Read stored head hash - can be empty
	storedHeadHash := store.Get(NamecoinLastBlockKey)

	// Scan all Namecoin blocks
	blocks := make([]loadedBlock, 0, store.Len())
	store.ForEach(func(key string, val []byte) bool {
		if !strings.HasPrefix(key, NamecoinBlockPrefix) {
			return true
		}

		h, err := parseNamecoinBlockHeight(key)
		if err != nil {
			warnf("load namecoin chain: skipping malformed block key %q: %v", key, err)
			return true
		}

		blocks = append(blocks, loadedBlock{
			Height: h,
			Key:    key,
			Raw:    append([]byte(nil), val...),
		})
		return true
	})

	// Nothing to replay.
	if len(blocks) == 0 {
		return &NamecoinChain{
			store:      store,
			state:      state,
			headHash:   nil,
			headHeight: 0,
		}, nil
	}

	// Order by height from genesis to head
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Height < blocks[j].Height
	})

	// replay
	headHash, foundHead, headHeight := ReplayBlocks(state, blocks, storedHeadHash)

	// If we had a stored head but never encountered it, we use the
	// last valid block we replayed and overwrite NamecoinLastBlockKey below
	if storedHeadHash != nil && !foundHead {
		warnf("namecoin: stored head hash not found in chain; using last valid block at height %d", headHeight)
	}

	// Persist the new head hash
	if headHash != nil {
		store.Set(NamecoinLastBlockKey, headHash)
	}

	return &NamecoinChain{
		store:      store,
		state:      state,
		headHash:   headHash,
		headHeight: headHeight,
	}, nil
}

func ReplayBlocks(state *NamecoinState, blocks []loadedBlock, storedHeadHash []byte) ([]byte, bool, uint64) {
	// replay vars
	var (
		prevHash   []byte
		headHash   []byte
		headHeight uint64
		foundHead  bool
	)

	// Replay blocks in order
	for _, lb := range blocks {
		var blk types.Block
		if err := blk.Unmarshal(lb.Raw); err != nil {
			warnf("load namecoin chain: skipping block at height %d: cannot unmarshal: %v",
				lb.Height, err)
			continue
		}

		// Basic height sanity: header height should match key height if set
		if blk.Header.Height != 0 && blk.Header.Height != lb.Height {
			warnf("load namecoin chain: block key height %d != header height %d; skipping",
				lb.Height, blk.Header.Height)
			continue
		}
		if blk.Header.Height == 0 {
			blk.Header.Height = lb.Height
		}

		// Basic prevHash linkage
		if blk.Header.Height == 0 {
			// Genesis prevHash should be empty
			if len(blk.Header.PrevHash) != 0 {
				warnf("load namecoin chain: genesis block with non-empty prevHash; continuing anyway")
			}
		} else if prevHash != nil && !bytes.Equal(blk.Header.PrevHash, prevHash) {
			warnf("load namecoin chain: skipping block at height %d: prevHash mismatch", blk.Header.Height)
			continue
		}

		// Apply block transactions to state
		if err := state.ApplyBlock(&blk); err != nil {
			warnf("namecoin: error applying block at height %d: %v", blk.Header.Height, err)
			// for robustness, we keep going; bad blocks don't kill replay
			continue
		}

		// Track head and linkage for the next iteration
		prevHash = append([]byte(nil), blk.Hash...)
		headHash = prevHash
		headHeight = blk.Header.Height

		if storedHeadHash != nil && bytes.Equal(blk.Hash, storedHeadHash) {
			foundHead = true
			break
		}
	}

	return headHash, foundHead, headHeight
}

// ---- Core Validate Block ----
// ValidateAndApply validates a candidate Namecoin block against the current
// chain head and state, without mutating the live state.
//
// Checks:
//   - prevHash linkage
//   - monotonic height
//   - txRoot consistency (recomputed from txs)
//   - that all txs apply cleanly on a temporary state
//
// On success, it returns the resulting (cloned) state that includes the
// block's effects. On failure, it returns a non-nil error and leaves
// the live chain state untouched
func (c *NamecoinChain) ValidateAndApply(currentHeadHeight uint64, currentHeadHash []byte, currentState *NamecoinState, blk *types.Block) error {
	if blk == nil {
		return fmt.Errorf("validate namecoin blk: nil block")
	}

	// Basic height / prevHash linkage
	if currentHeadHash == nil {
		// Empty chain: expect genesis block
		if blk.Header.Height != 0 {
			return fmt.Errorf("invalid genesis height %d; expected 0", blk.Header.Height)
		}
		if len(blk.Header.PrevHash) != 0 {
			return fmt.Errorf("genesis block must have empty prevHash")
		}
	} else {
		expectedHeight := currentHeadHeight + 1
		if blk.Header.Height != expectedHeight {
			return fmt.Errorf("invalid height %d; expected %d", blk.Header.Height, expectedHeight)
		}
		if !bytes.Equal(blk.Header.PrevHash, currentHeadHash) {
			return fmt.Errorf("prevHash mismatch")
		}
	}

	// txRoot consistency
	computedRoot, err := computeTxRoot(blk.Transactions)
	if err != nil {
		return fmt.Errorf("failed to compute tx root: %w", err)
	}
	if !bytes.Equal(blk.Header.TxRoot, computedRoot) {
		return fmt.Errorf("txRoot mismatch")
	}

	// Replay txs on a cloned state to ensure they all apply cleanly
	if err = currentState.ApplyBlock(blk); err != nil {
		return fmt.Errorf("block tx replay failed: %w", err)
	}

	return nil
}

func (c *NamecoinChain) ApplyBlock(blk *types.Block) error {
	if blk == nil {
		return fmt.Errorf("nil block")
	}

	// take snapshot of blockchain state
	c.mu.Lock()
	currentHeadHeight := c.headHeight
	currentHeadHash := append([]byte(nil), c.headHash...)
	clonedState := c.state.Clone()
	c.mu.Unlock()

	// validate on state copy
	err := c.ValidateAndApply(currentHeadHeight, currentHeadHash, clonedState, blk)
	if err != nil {
		return err
	}

	// serialize block
	data, err := blk.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal block at height %d: %w", blk.Header.Height, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	err = c.ValidateAndApply(c.headHeight, c.headHash, c.state, blk)
	if err != nil {
		return err
	}

	// Persist block to block store
	blockKey := encodeNamecoinBlockKey(blk.Header.Height)
	c.store.Set(blockKey, data)

	// Update last-block pointer
	c.store.Set(NamecoinLastBlockKey, blk.Hash)

	c.headHash = append([]byte(nil), blk.Hash...)
	c.headHeight = blk.Header.Height

	return nil
}

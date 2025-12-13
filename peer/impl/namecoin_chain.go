package impl

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"log"
	"math/big"
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

func NewNamecoinChain(store storage.Store) *NamecoinChain {
	return &NamecoinChain{
		store:      store,
		state:      NewState(),
		headHash:   nil,
		headHeight: 0,
	}
}

// NamecoinChain represents the local Namecoin chain for a node
type NamecoinChain struct {
	mu         sync.RWMutex
	store      storage.Store
	state      *NamecoinState
	headHash   []byte
	headHeight uint64
	powTarget  *big.Int
}

func (c *NamecoinChain) HeadHash() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CloneBytes(c.headHash)
}

func (c *NamecoinChain) HeadHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.headHeight
}

// SnapshotDomains returns a copy of the current domains map along with the chain height.
func (c *NamecoinChain) SnapshotDomains() (map[string]types.NameRecord, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state == nil {
		return nil, c.headHeight
	}
	return c.state.SnapshotDomains()
}

// ---- Helper utils ----
// loadedBlock is used only during chain replay to sort blocks by height and
// keep both the key and raw bytes on hand
type loadedBlock struct {
	Height uint64
	Key    string
	Raw    []byte
}

// CloneBytes returns a defensive copy of the provided slice.
func CloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	return append([]byte(nil), src...)
}

// quality of life - warnf logs only if GLOG != "no"
func warnf(format string, args ...interface{}) {
	if os.Getenv("GLOG") == "no" {
		return
	}
	log.Printf(format, args...)
}

// EncodeNamecoinBlockKey builds the key for a given height
func EncodeNamecoinBlockKey(height uint64) string {
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
// ComputeTxRoot recomputes the txRoot from the block's transactions
// MVP: SHA-256 over the JSON encoding of each tx in order.
func ComputeTxRoot(txs []types.Tx) ([]byte, error) {
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

// decodeLoadedBlock unmarshals a stored block and ensures the header height
// matches the key height (defaulting to the key height when unspecified).
func decodeLoadedBlock(lb loadedBlock) (*types.Block, error) {
	var blk types.Block
	if err := blk.Unmarshal(lb.Raw); err != nil {
		return nil, fmt.Errorf("cannot unmarshal: %w", err)
	}

	if blk.Header.Height == 0 {
		blk.Header.Height = lb.Height
	} else if blk.Header.Height != lb.Height {
		return nil, fmt.Errorf("block key height %d != header height %d", lb.Height, blk.Header.Height)
	}

	return &blk, nil
}

// ensureNextBlockFollows validates that blk properly extends the provided head.
// When headHash is nil, blk must be the genesis block.
func ensureNextBlockFollows(currentHeadHeight uint64, currentHeadHash []byte, blk *types.Block) error {
	if currentHeadHash == nil {
		if blk.Header.Height != 0 {
			return &ChainFollowError{fmt.Sprintf("invalid genesis height %d; expected 0", blk.Header.Height)}
		}
		if len(blk.Header.PrevHash) != 0 {
			return &ChainFollowError{"genesis block must have empty prevHash"}
		}
		return nil
	}

	expectedHeight := currentHeadHeight + 1
	if blk.Header.Height != expectedHeight {
		return &ChainFollowError{fmt.Sprintf("invalid height %d; expected %d", blk.Header.Height, expectedHeight)}
	}
	if !bytes.Equal(blk.Header.PrevHash, currentHeadHash) {
		return &ChainFollowError{"prevHash mismatch"}
	}

	return nil
}

// LoadNamecoinChain replays all Namecoin blocks from the blockchain store
// and reconstructs local NamecoinState
func LoadNamecoinChain(store storage.Store) (*NamecoinChain, error) {
	if store == nil {
		return nil, fmt.Errorf("nil blockchain store")
	}

	state := NewState()

	// Read stored head hash - can be empty
	storedHeadHash := CloneBytes(store.Get(NamecoinLastBlockKey))

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
			Raw:    CloneBytes(val),
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

// SetPowTarget stores the PoW target used when validating incoming blocks.
func (c *NamecoinChain) SetPowTarget(target *big.Int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if target == nil {
		c.powTarget = nil
		return
	}
	c.powTarget = cloneBigInt(target)
}

func cloneBigInt(src *big.Int) *big.Int {
	if src == nil {
		return nil
	}
	return new(big.Int).Set(src)
}

func (c *NamecoinChain) powTargetSnapshot() *big.Int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneBigInt(c.powTarget)
}

// blockAtHeight loads a block from storage for the given height.
func (c *NamecoinChain) blockAtHeight(height uint64) (*types.Block, error) {
	key := EncodeNamecoinBlockKey(height)
	raw := c.store.Get(key)
	if raw == nil {
		return nil, fmt.Errorf("no block at height %d", height)
	}
	lb := loadedBlock{
		Height: height,
		Key:    key,
		Raw:    CloneBytes(raw),
	}
	return decodeLoadedBlock(lb)
}

// forkUpToHeight reconstructs a chain whose head is the block at the provided height.
// If storeOverride is nil, the current chain's store is used.
func (c *NamecoinChain) forkUpToHeight(height uint64, storeOverride storage.Store) (*NamecoinChain, error) {
	c.mu.RLock()
	if height > c.headHeight {
		c.mu.RUnlock()
		return nil, fmt.Errorf("cannot fork to height %d above head %d", height, c.headHeight)
	}
	powTarget := cloneBigInt(c.powTarget)
	c.mu.RUnlock()

	forkStore := c.store
	if storeOverride != nil {
		forkStore = storeOverride
	}

	blocks := make([]loadedBlock, 0, height+1)
	for h := uint64(0); h <= height; h++ {
		key := EncodeNamecoinBlockKey(h)
		raw := forkStore.Get(key)
		if raw == nil {
			return nil, fmt.Errorf("missing block at height %d", h)
		}
		blocks = append(blocks, loadedBlock{
			Height: h,
			Key:    key,
			Raw:    CloneBytes(raw),
		})
	}

	state := NewState()
	state.SetLogger(c.state.CloneLogger())
	headHash, _, headHeight := ReplayBlocks(state, blocks, nil)
	if headHeight != height {
		return nil, fmt.Errorf("replay stopped at height %d (expected %d)", headHeight, height)
	}

	return &NamecoinChain{
		store:      forkStore,
		state:      state,
		headHash:   headHash,
		headHeight: headHeight,
		powTarget:  powTarget,
	}, nil
}

func ReplayBlocks(state *NamecoinState, blocks []loadedBlock, storedHeadHash []byte) ([]byte, bool, uint64) {
	// replay vars
	var (
		headHash   []byte
		headHeight uint64
		foundHead  bool
	)

	// Replay blocks in order
	for _, lb := range blocks {
		blk, err := decodeLoadedBlock(lb)
		if err != nil {
			warnf("load namecoin chain: skipping block at height %d: %v", lb.Height, err)
			continue
		}

		if err := ensureNextBlockFollows(headHeight, headHash, blk); err != nil {
			warnf("load namecoin chain: skipping block at height %d: %v", lb.Height, err)
			continue
		}

		// Apply block transactions to state
		if err := state.ApplyBlock(blk); err != nil {
			warnf("namecoin: error applying block at height %d: %v", blk.Header.Height, err)
			// for robustness, we keep going; bad blocks don't kill replay
			continue
		}

		// Track head and linkage for the next iteration
		headHash = CloneBytes(blk.Hash)
		headHeight = blk.Header.Height

		if storedHeadHash != nil && bytes.Equal(blk.Hash, storedHeadHash) {
			foundHead = true
			break
		}
	}

	return headHash, foundHead, headHeight
}

// ValidateBlock validates a candidate Namecoin block against the current
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

func (c *NamecoinChain) ApplyBlock(blk *types.Block) error {
	if blk == nil {
		return fmt.Errorf("nil block")
	}

	// take snapshot of blockchain state
	target := c.powTargetSnapshot()
	c.mu.Lock()
	currentHeadHeight := c.headHeight
	currentHeadHash := CloneBytes(c.headHash)
	clonedState := c.state.Clone()
	c.mu.Unlock()

	// validate on state copy
	err := c.ValidateBlock(currentHeadHeight, currentHeadHash, blk, target)
	if err != nil {
		return err
	}

	// try to apply on a state clone to ensure safety
	if err = clonedState.ApplyBlock(blk); err != nil {
		return err
	}

	// serialize block
	data, err := blk.Marshal()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	err = c.ValidateBlock(c.headHeight, c.headHash, blk, c.powTarget)
	if err != nil {
		return err
	}

	if err = c.state.ApplyBlock(blk); err != nil {
		return err
	}

	// Persist block to block store
	blockKey := EncodeNamecoinBlockKey(blk.Header.Height)
	c.store.Set(blockKey, data)

	// Update last-block pointer
	c.store.Set(NamecoinLastBlockKey, blk.Hash)

	c.headHash = CloneBytes(blk.Hash)
	c.headHeight = blk.Header.Height

	return nil
}

func (c *NamecoinChain) ValidateBlock(
	currentHeadHeight uint64,
	currentHeadHash []byte,
	blk *types.Block,
	target *big.Int) error {
	if blk == nil {
		return fmt.Errorf("validate namecoin blk: nil block")
	}

	if err := ensureNextBlockFollows(currentHeadHeight, currentHeadHash, blk); err != nil {
		return err
	}

	computedRoot, err := ComputeTxRoot(blk.Transactions)
	if err != nil {
		return fmt.Errorf("failed to compute tx root: %w", err)
	}
	if !bytes.Equal(blk.Header.TxRoot, computedRoot) {
		return fmt.Errorf("txRoot mismatch")
	}

	validationTarget := selectPowTarget(blk, target)
	if err = validateWorkForTarget(blk, validationTarget); err != nil {
		return err
	}

	return nil
}

func validateWorkForTarget(blk *types.Block, target *big.Int) error {
	if !IsBlockComplexityValid(*blk, target) {
		return fmt.Errorf("block hash above target")
	}
	return nil
}

// selectPowTarget picks the target to validate work against, preferring the block's
// encoded difficulty when present, falling back to the chain's target.
func selectPowTarget(blk *types.Block, chainTarget *big.Int) *big.Int {
	if blk != nil && len(blk.Header.Difficulty) > 0 {
		return new(big.Int).SetBytes(blk.Header.Difficulty)
	}
	return chainTarget
}

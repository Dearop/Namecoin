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
	"time"

	"go.dedis.ch/cs438/peer"
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
	powTarget  *big.Int
	powParams  powParams
	headTS     int64
}

type powParams struct {
	targetBlockTime time.Duration
	maxAdjustUp     float64
	maxAdjustDown   float64
}

func powParamsFromConfig(cfg peer.PoWConfig) powParams {
	p := powParams{
		targetBlockTime: cfg.TargetBlockTime,
		maxAdjustUp:     cfg.MaxAdjustUp,
		maxAdjustDown:   cfg.MaxAdjustDown,
	}
	if p.targetBlockTime <= 0 {
		p.targetBlockTime = defaultTargetBlockTime
	}
	if p.maxAdjustUp <= 0 {
		p.maxAdjustUp = defaultMaxAdjustUp
	}
	if p.maxAdjustDown <= 0 {
		p.maxAdjustDown = defaultMaxAdjustDown
	}
	return p
}

func loadStoredNamecoinBlocks(store storage.Store) ([]loadedBlock, []byte) {
	storedHeadHash := cloneBytes(store.Get(NamecoinLastBlockKey))
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
			Raw:    cloneBytes(val),
		})
		return true
	})

	// Order by height from genesis to head
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Height < blocks[j].Height
	})

	return blocks, storedHeadHash
}

func rebuildPowState(blocks []loadedBlock, headHeight uint64) (*big.Int, powParams, int64) {
	powTarget := effectiveTarget(nil)
	params := powParamsFromConfig(peer.PoWConfig{})
	var (
		headTS   int64
		havePrev bool
	)

	for _, lb := range blocks {
		if lb.Height > headHeight {
			break
		}
		blk, err := decodeLoadedBlock(lb)
		if err != nil {
			warnf("load namecoin chain: skipping block for difficulty rebuild at height %d: %v", lb.Height, err)
			continue
		}
		blkTarget := DecodeDifficulty(blk.Header.Difficulty)
		if blkTarget == nil {
			blkTarget = powTarget
		}

		if !havePrev {
			powTarget = cloneBigInt(blkTarget)
		} else if powTarget != nil && blkTarget != nil && powTarget.Cmp(blkTarget) != 0 {
			warnf("load namecoin chain: header difficulty mismatch at height %d", blk.Header.Height)
			powTarget = cloneBigInt(blkTarget)
		}

		if havePrev {
			spacing := time.Duration(blk.Header.Timestamp-headTS) * time.Second
			powTarget = AdjustDifficulty(
				powTarget,
				spacing,
				params.targetBlockTime,
				params.maxAdjustUp,
				params.maxAdjustDown,
			)
		}
		headTS = blk.Header.Timestamp
		havePrev = true
	}
	if powTarget == nil {
		powTarget = effectiveTarget(nil)
	}
	return powTarget, params, headTS
}

func (c *NamecoinChain) HeadHash() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneBytes(c.headHash)
}

func (c *NamecoinChain) HeadHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.headHeight
}

// NextPowTarget returns a copy of the target difficulty the chain expects for
// the next block to be mined.
func (c *NamecoinChain) NextPowTarget() *big.Int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneBigInt(c.powTarget)
}

// HeadTimestamp returns the timestamp of the current head block (0 if none).
func (c *NamecoinChain) HeadTimestamp() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.headTS
}

// ConfigurePow sets the tuning parameters for PoW adjustment. If the chain has
// no blocks yet, it also seeds the target from the provided config.
func (c *NamecoinChain) ConfigurePow(cfg peer.PoWConfig) {
	params := powParamsFromConfig(cfg)
	seed := effectiveTarget(cfg.Target)

	c.mu.Lock()
	c.powParams = params
	if c.powTarget == nil || (c.headHash == nil && c.headHeight == 0) {
		c.powTarget = cloneBigInt(seed)
	}
	c.mu.Unlock()
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

// cloneBytes returns a defensive copy of the provided slice.
func cloneBytes(src []byte) []byte {
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
			return fmt.Errorf("invalid genesis height %d; expected 0", blk.Header.Height)
		}
		if len(blk.Header.PrevHash) != 0 {
			return fmt.Errorf("genesis block must have empty prevHash")
		}
		return nil
	}

	expectedHeight := currentHeadHeight + 1
	if blk.Header.Height != expectedHeight {
		return fmt.Errorf("invalid height %d; expected %d", blk.Header.Height, expectedHeight)
	}
	if !bytes.Equal(blk.Header.PrevHash, currentHeadHash) {
		return fmt.Errorf("prevHash mismatch")
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
	blocks, storedHeadHash := loadStoredNamecoinBlocks(store)

	// Nothing to replay.
	if len(blocks) == 0 {
		return &NamecoinChain{
			store:      store,
			state:      state,
			headHash:   nil,
			headHeight: 0,
			powTarget:  effectiveTarget(nil),
			powParams:  powParamsFromConfig(peer.PoWConfig{}),
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

	powTarget, params, headTS := rebuildPowState(blocks, headHeight)

	return &NamecoinChain{
		store:      store,
		state:      state,
		headHash:   headHash,
		headHeight: headHeight,
		powTarget:  powTarget,
		powParams:  params,
		headTS:     headTS,
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
	t := c.powTarget
	if t == nil {
		t = effectiveTarget(nil)
	}
	return cloneBigInt(t)
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
		headHash = cloneBytes(blk.Hash)
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
	currentHeadHash := cloneBytes(c.headHash)
	clonedState := c.state.Clone()
	prevTS := c.headTS
	c.mu.Unlock()

	blockTarget := DecodeDifficulty(blk.Header.Difficulty)
	// validate on state copy
	err := c.ValidateBlock(currentHeadHeight, currentHeadHash, blk, target)
	if err != nil {
		return err
	}

	// try to apply on a state clone to ensure safety
	if err = clonedState.applyBlockInPlace(blk); err != nil {
		return err
	}

	// serialize block
	data, err := blk.Marshal()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err = c.ValidateBlock(c.headHeight, c.headHash, blk, c.powTarget); err != nil {
		return err
	}

	// Swap in the validated state rather than reapplying to avoid partial writes
	// if a later error bubbles up.
	c.state.replaceWith(clonedState)

	// Persist block to block store
	blockKey := encodeNamecoinBlockKey(blk.Header.Height)
	c.store.Set(blockKey, data)

	// Update last-block pointer
	c.store.Set(NamecoinLastBlockKey, blk.Hash)

	c.headHash = cloneBytes(blk.Hash)
	c.headHeight = blk.Header.Height
	c.headTS = blk.Header.Timestamp
	// Prepare target for the next block using observed spacing.
	if prevTS != 0 {
		spacing := time.Duration(blk.Header.Timestamp-prevTS) * time.Second
		c.powTarget = AdjustDifficulty(
			blockTarget, 
			spacing, 
			c.powParams.targetBlockTime, 
			c.powParams.maxAdjustUp, 
			c.powParams.maxAdjustDown,
		)
	} else {
		c.powTarget = cloneBigInt(blockTarget)
	}

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

	blockTarget := DecodeDifficulty(blk.Header.Difficulty)
	if blockTarget == nil {
		return fmt.Errorf("validate namecoin blk: missing difficulty")
	}
	// If the caller provided an expected target and it differs, we still allow
	// the block but rely on the header-encoded target for PoW validation.

	computedRoot, err := computeTxRoot(blk.Transactions)
	if err != nil {
		return fmt.Errorf("failed to compute tx root: %w", err)
	}
	if !bytes.Equal(blk.Header.TxRoot, computedRoot) {
		return fmt.Errorf("txRoot mismatch")
	}

	if err = validateWorkForTarget(blk, blockTarget); err != nil {
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

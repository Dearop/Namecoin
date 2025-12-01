package impl

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
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

// TODO: move these and NamecoinChain struct into a separate package
func (c *NamecoinChain) State() *NamecoinState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
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

// SnapshotDomains returns a shallow copy of the domain map and the current height.
// Used by read-only components like DNS resolvers to avoid holding locks.
func (c *NamecoinChain) SnapshotDomains() (map[string]types.NameRecord, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]types.NameRecord, len(c.state.Domains))
	for k, v := range c.state.Domains {
		out[k] = v
	}
	return out, c.headHeight
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

// ApplyNamecoinTx implements minimal Namecoin semantics
// We can harden this later (ownership checks, expiries, coins, etc).
func ApplyNamecoinTx(st *NamecoinState, transaction *types.Tx) error {
	if st == nil {
		return fmt.Errorf("apply namecoin tx: nil state")
	}
	if transaction == nil || transaction.ID == nil {
		return fmt.Errorf("apply namecoin tx: nil tx")
	}
	// TODO: add payload validation
	id := transaction.ID
	tx := transaction.Payload

	switch tx.Op {
	case "register":
		// Simple MVP semantics:
		// - overwrites any existing record for this name
		// - assigns ownership to tx.From
		if tx.Name == "" {
			return fmt.Errorf("register tx without name")
		}
		st.Domains[tx.Name] = types.NameRecord{
			Owner:     tx.From,
			Value:     tx.Value,
			ExpiresAt: 0,
		}
	case "update":
		if tx.Name == "" {
			return fmt.Errorf("update tx without name")
		}
		rec, ok := st.Domains[tx.Name]
		if !ok {
			return fmt.Errorf("update non-existent name %s", tx.Name)
		}
		// NOTE: no ownership checks
		rec.Value = tx.Value
		st.Domains[tx.Name] = rec
	case "transfer":
		// NOTE: only domain ownership transfer, no coins
		if tx.Name == "" {
			return fmt.Errorf("transfer tx without name")
		}
		rec, ok := st.Domains[tx.Name]
		if !ok {
			return fmt.Errorf("transfer non-existent name %s", tx.Name)
		}
		rec.Owner = tx.To
		st.Domains[tx.Name] = rec

	// TODO: Add "pay", "coinbase", etc. later

	default:
		// Unknown op: treat as no-op
		warnf("namecoin: unknown tx op %q in tx %s", tx.Op, id)
	}
	return nil
}

// ApplyNamecoinBlock applies all txs and prunes included pending txs
// NOTE: kept simple for now but later we can refactor into
// dedicated modules (e.g., domain, coin, mempool)
func ApplyNamecoinBlock(st *NamecoinState, blk *types.Block) error {
	if st == nil {
		return fmt.Errorf("apply namecoin block: nil state")
	}
	if blk == nil {
		return fmt.Errorf("apply namecoin block: nil block")
	}
	for i := range blk.Transactions {
		tx := &blk.Transactions[i]
		if err := ApplyNamecoinTx(st, tx); err != nil {
			// for robustness, we log and continue
			warnf("namecoin: failed to apply tx %s at height %d: %v",
				tx.ID, blk.Header.Height, err)
		}
		if st.Pending != nil && tx.ID != nil {
			st.Pending.Remove(tx.ID)
		}
	}
	return nil
}

// ---- Core Chain Loader ----
// LoadNamecoinChain replays all Namecoin blocks from the blockchain store
// and reconstructs local NamecoinState
func LoadNamecoinChain(store storage.Store) (*NamecoinChain, error) {
	if store == nil {
		return nil, fmt.Errorf("nil blockchain store")
	}

	state := NewState()
	storedHeadHash := store.Get(NamecoinLastBlockKey)

	blocks := collectNamecoinBlocks(store)
	if len(blocks) == 0 {
		return &NamecoinChain{
			store: store, state: state,
		}, nil
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Height < blocks[j].Height
	})

	headHash, headHeight := replayNamecoinBlocks(state, blocks, storedHeadHash)
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

func collectNamecoinBlocks(store storage.Store) []loadedBlock {
	var blocks []loadedBlock
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
	return blocks
}

func replayNamecoinBlocks(state *NamecoinState, blocks []loadedBlock, storedHeadHash []byte) ([]byte, uint64) {
	var (
		prevHash   []byte
		headHash   []byte
		headHeight uint64
		foundHead  bool
	)

	for _, lb := range blocks {
		blk, ok := decodeLoadedBlock(lb)
		if !ok {
			continue
		}
		if !validateLoadedBlockHeight(lb, &blk) {
			continue
		}
		if !validateLoadedPrev(prevHash, &blk) {
			continue
		}
		if err := ApplyNamecoinBlock(state, &blk); err != nil {
			warnf("namecoin: error applying block at height %d: %v", blk.Header.Height, err)
			continue
		}
		prevHash = append([]byte(nil), blk.Header.Hash...)
		headHash = prevHash
		headHeight = blk.Header.Height

		if storedHeadHash != nil && bytes.Equal(blk.Header.Hash, storedHeadHash) {
			foundHead = true
			break
		}
	}

	if storedHeadHash != nil && !foundHead {
		warnf("namecoin: stored head hash not found in chain; using last valid block at height %d", headHeight)
	}
	return headHash, headHeight
}

func decodeLoadedBlock(lb loadedBlock) (types.Block, bool) {
	var blk types.Block
	if err := blk.Unmarshal(lb.Raw); err != nil {
		warnf("load namecoin chain: skipping block at height %d: cannot unmarshal: %v",
			lb.Height, err)
		return blk, false
	}
	return blk, true
}

func validateLoadedBlockHeight(lb loadedBlock, blk *types.Block) bool {
	if blk.Header.Height != 0 && blk.Header.Height != lb.Height {
		warnf("load namecoin chain: block key height %d != header height %d; skipping",
			lb.Height, blk.Header.Height)
		return false
	}
	if blk.Header.Height == 0 {
		blk.Header.Height = lb.Height
	}
	return true
}

func validateLoadedPrev(prevHash []byte, blk *types.Block) bool {
	if blk.Header.Height == 0 {
		if len(blk.Header.PrevHash) != 0 {
			warnf("load namecoin chain: genesis block with non-empty prevHash; continuing anyway")
		}
		return true
	}
	if prevHash != nil && !bytes.Equal(blk.Header.PrevHash, prevHash) {
		warnf("load namecoin chain: skipping block at height %d: prevHash mismatch", blk.Header.Height)
		return false
	}
	return true
}

// ---- Core Validate Block ----
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
func (c *NamecoinChain) ValidateBlock(blk *types.Block) (*NamecoinState, error) {
	if c == nil {
		return nil, fmt.Errorf("validate namecoin blk: nil NamecoinChain")
	}
	if blk == nil {
		return nil, fmt.Errorf("validate namecoin blk: nil block")
	}

	// Snapshot current head and state
	c.mu.RLock()
	currentHeadHash := append([]byte(nil), c.headHash...)
	currentHeadHeight := c.headHeight
	currentState := c.state
	c.mu.RUnlock()

	if err := validateLinkage(currentHeadHash, currentHeadHeight, blk); err != nil {
		return nil, err
	}

	// txRoot consistency
	computedRoot, err := computeTxRoot(blk.Transactions)
	if err != nil {
		return nil, fmt.Errorf("failed to compute tx root: %w", err)
	}
	if !bytes.Equal(blk.Header.TxRoot, computedRoot) {
		return nil, fmt.Errorf("txRoot mismatch")
	}

	// Replay txs on a cloned state to ensure they all apply cleanly
	tmp := currentState.Clone()
	if err := ApplyNamecoinBlock(tmp, blk); err != nil {
		return nil, fmt.Errorf("block tx replay failed: %w", err)
	}

	return tmp, nil
}

func validateLinkage(headHash []byte, headHeight uint64, blk *types.Block) error {
	if headHash == nil {
		if blk.Header.Height != 0 {
			return fmt.Errorf("invalid genesis height %d; expected 0", blk.Header.Height)
		}
		if len(blk.Header.PrevHash) != 0 {
			return fmt.Errorf("genesis block must have empty prevHash")
		}
		return nil
	}

	expectedHeight := headHeight + 1
	if blk.Header.Height != expectedHeight {
		return fmt.Errorf("invalid height %d; expected %d", blk.Header.Height, expectedHeight)
	}
	if !bytes.Equal(blk.Header.PrevHash, headHash) {
		return fmt.Errorf("prevHash mismatch")
	}
	return nil
}

// ---- Core Apply Block ----
// ApplyBlock validates and then applies a block:
//  1. ValidateBlock(block) – if it fails, return error, no state/store changes.
//  2. Persist the block under NamecoinBlockPrefix + height.
//  3. Update NamecoinLastBlockKey to block.Header.Hash.
//  4. Commit the validated state to the chain (including pruning pending txs).
//
// If any persistence step fails, the in-memory state/head is NOT updated,
// so there is no partial commit at the logical chain level.
func (c *NamecoinChain) ApplyBlock(blk *types.Block) error {
	if c == nil {
		return fmt.Errorf("nil NamecoinChain")
	}
	if blk == nil {
		return fmt.Errorf("nil block")
	}
	if c.store == nil {
		return fmt.Errorf("nil blockchain store")
	}

	// validate
	newState, err := c.ValidateBlock(blk)
	if err != nil {
		return err
	}

	// serialise block
	data, err := blk.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal block at height %d: %w", blk.Header.Height, err)
	}

	// Persist block to block store
	blockKey := encodeNamecoinBlockKey(blk.Header.Height)
	c.store.Set(blockKey, data)

	// Update last-block pointer
	c.store.Set(NamecoinLastBlockKey, blk.Header.Hash)

	// Commit validated state + head into memory
	c.mu.Lock()
	c.state = newState
	c.headHash = append([]byte(nil), blk.Header.Hash...)
	c.headHeight = blk.Header.Height
	c.mu.Unlock()

	return nil
}

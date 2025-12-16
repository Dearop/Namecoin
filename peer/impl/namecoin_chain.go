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
	powParams  powParams
	headTS     int64
}

type powParams struct {
	targetBlockTime time.Duration
	maxAdjustUp     float64
	maxAdjustDown   float64
	dynamic         bool
}

func powParamsFromConfig(cfg peer.PoWConfig) powParams {
	p := powParams{
		targetBlockTime: cfg.TargetBlockTime,
		maxAdjustUp:     cfg.MaxAdjustUp,
		maxAdjustDown:   cfg.MaxAdjustDown,
		dynamic:         !cfg.DisableDifficultyAdjustment,
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

		if havePrev && params.dynamic {
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
	return CloneBytes(c.headHash)
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
// MVP: SHA-256 over the canonical JSON encoding of each tx in order.
func ComputeTxRoot(txs []types.Tx) ([]byte, error) {
	h := sha256.New()
	for i := range txs {
		b, err := SerializeTransactionForTxRoot(&txs[i])
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

		// Recompute-and-verify (hash, linkage, txRoot)
		canonicalHash, err := validateBlockOnReplay(headHeight, headHash, blk)
		if err != nil {
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
		headHash = CloneBytes(canonicalHash)
		headHeight = blk.Header.Height

		if storedHeadHash != nil && bytes.Equal(canonicalHash, storedHeadHash) {
			foundHead = true
			break
		}
	}

	return headHash, foundHead, headHeight
}

func validateBlockOnReplay(
	currentHeadHeight uint64,
	currentHeadHash []byte,
	blk *types.Block,
) ([]byte, error) {
	// 1) Canonical hash must be derived from header
	computedArr := blk.ComputeHash()
	computed := computedArr[:] // []byte

	// If blk.Hash is present in storage, it must match.
	// Since stored blocks always include Hash, this catches tampering/corruption.
	if len(blk.Hash) != 0 && !bytes.Equal(blk.Hash, computed) {
		return nil, fmt.Errorf("stored blk.Hash != ComputeHash()")
	}

	// Normalise in-memory to computed canonical value to prevent later misuse.
	blk.Hash = CloneBytes(computed)

	// 2) Linkage/height check (uses currentHeadHash that we maintain as canonical)
	if err := ensureNextBlockFollows(currentHeadHeight, currentHeadHash, blk); err != nil {
		return nil, err
	}

	// 3) TxRoot check
	root, err := ComputeTxRoot(blk.Transactions)
	if err != nil {
		return nil, fmt.Errorf("compute tx root: %w", err)
	}
	if !bytes.Equal(blk.Header.TxRoot, root) {
		return nil, fmt.Errorf("txRoot mismatch on replay")
	}

	return computed, nil
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
	prevTS := c.headTS
	c.mu.Unlock()

	blockTarget := DecodeDifficulty(blk.Header.Difficulty)
	if blockTarget == nil {
		blockTarget = cloneBigInt(c.powTarget)
	}
	// validate + apply on state copy
	err := c.ValidateBlock(currentHeadHeight, currentHeadHash, blk, target, clonedState)
	if err != nil {
		return err
	}

	// serialize block
	data, err := blk.Marshal()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure the chain head didn't change since we validated/applied on the clone.
	if c.headHeight != currentHeadHeight || !bytes.Equal(c.headHash, currentHeadHash) {
		return fmt.Errorf("chain head changed during ApplyBlock")
	}

	err = c.ValidateBlockHeader(c.headHeight, c.headHash, blk, c.powTarget)
	if err != nil {
		return err
	}

	// Swap in the validated state rather than reapplying to avoid partial writes
	// if a later error bubbles up.
	c.state.replaceWith(clonedState)

	// Persist block to block store
	blockKey := EncodeNamecoinBlockKey(blk.Header.Height)
	c.store.Set(blockKey, data)

	// Recompute blk hash for safety
	canonical := blk.ComputeHash()

	// Update last-block pointer
	c.store.Set(NamecoinLastBlockKey, canonical)

	c.headHash = CloneBytes(canonical)
	c.headHeight = blk.Header.Height
	c.headTS = blk.Header.Timestamp
	// Prepare target for the next block using observed spacing.
	if c.powParams.dynamic && prevTS != 0 {
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

// ValidateBlockHeader validates only block header-level properties (linkage, TxRoot, PoW),
// without applying transactions.
func (c *NamecoinChain) ValidateBlockHeader(
	currentHeadHeight uint64,
	currentHeadHash []byte,
	blk *types.Block,
	target *big.Int) error {
	if blk == nil {
		return fmt.Errorf("validate namecoin blk: nil block")
	}

	// Canonical block id must be derived from the header.
	computed := blk.ComputeHash() // must hash exactly the header fields used for PoW
	if len(blk.Hash) != 0 && !bytes.Equal(blk.Hash, computed) {
		return fmt.Errorf("block hash mismatch: payload hash != computed header hash")
	}
	// Take as memory copy to prevent later misuse.
	blk.Hash = CloneBytes(computed)

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

	validationTarget, err := enforceDifficultyTarget(blk, target)
	if err != nil {
		return err
	}
	if err = validateWorkForTarget(blk, validationTarget); err != nil {
		return err
	}

	return nil
}

// ValidateBlock validates a candidate Namecoin block against the current chain head and
// applies all transactions onto the provided state snapshot (which must be a clone).
func (c *NamecoinChain) ValidateBlock(
	currentHeadHeight uint64,
	currentHeadHash []byte,
	blk *types.Block,
	target *big.Int,
	stateSnapshot *NamecoinState,
) error {
	if stateSnapshot == nil {
		return fmt.Errorf("validate namecoin blk: nil state snapshot")
	}

	if err := c.ValidateBlockHeader(currentHeadHeight, currentHeadHash, blk, target); err != nil {
		return err
	}

	if err := applyBlockStrict(stateSnapshot, blk); err != nil {
		return err
	}

	return nil
}

func applyBlockStrict(st *NamecoinState, blk *types.Block) error {
	if blk == nil {
		return fmt.Errorf("apply block: nil")
	}

	st.setHeight(blk.Header.Height)
	st.pruneExpired(blk.Header.Height)

	for i := range blk.Transactions {
		tx := &blk.Transactions[i]
		txID, err := BuildTransactionID(tx)
		if err != nil {
			return err
		}

		if st.IsTxApplied(txID) {
			return fmt.Errorf("tx already applied: %s", txID)
		}

		if err := validateTxStrict(st, txID, tx); err != nil {
			return fmt.Errorf("tx %d (%s) invalid: %w", i, txID, err)
		}

		if err := st.ApplyCommandUTXO(txID, tx); err != nil {
			return fmt.Errorf("apply tx %d (%s) UTXO: %w", i, txID, err)
		}
		if err := st.ApplyCommandState(tx); err != nil {
			return fmt.Errorf("apply tx %d (%s) state: %w", i, txID, err)
		}

		st.MarkAsApplied(txID)
	}

	return nil
}

func validateTxStrict(st *NamecoinState, txID string, tx *types.Tx) error {
	if tx == nil {
		return fmt.Errorf("nil tx")
	}

	// Validate command payload fields on the signed-tx view.
	stx := &SignedTransaction{
		Type:      tx.Type,
		From:      tx.From,
		Amount:    tx.Amount,
		Payload:   tx.Payload,
		Inputs:    tx.Inputs,
		Outputs:   tx.Outputs,
		Pk:        tx.Pk,
		TxID:      tx.TxID,
		Signature: tx.Signature,
	}
	if err := st.ValidateCommand(stx); err != nil {
		return err
	}

	// Reward is special: no signature required.
	if tx.Type == RewardCommandName {
		if tx.TxID != "" && tx.TxID != txID {
			return fmt.Errorf("reward txid mismatch: expected %s, got %s", txID, tx.TxID)
		}
		if len(tx.Outputs) > 1 {
			return fmt.Errorf("reward tx: expected 0 or 1 output, got %d", len(tx.Outputs))
		}
		return nil
	}

	// Require signature and ownership binding for all non-reward txs.
	if err := verifyTxAuth(tx, txID); err != nil {
		return err
	}

	// Enforce single-output MVP.
	if len(tx.Outputs) > 1 {
		return fmt.Errorf("expected 0 or 1 output, got %d", len(tx.Outputs))
	}
	for _, in := range tx.Inputs {
		if in.Index != 0 {
			return fmt.Errorf("unsupported input index %d (MVP only supports vout=0)", in.Index)
		}
	}

	// Enforce input availability and deterministic change output.
	// Recompute canonical inputs/outputs and require exact match.
	expIn, expOut, err := st.DeterministicSpendPlan(tx.From, tx.Amount)
	if err != nil {
		return err
	}
	if !equalTxInputs(tx.Inputs, expIn) {
		return fmt.Errorf("inputs not canonical for (from=%s, amount=%d)", tx.From, tx.Amount)
	}
	if !equalTxOutputs(tx.Outputs, expOut) {
		return fmt.Errorf("outputs not canonical for (from=%s, amount=%d)", tx.From, tx.Amount)
	}

	// Validate any command invariants that depend on formed inputs/outputs.
	return st.ValidateCommandWithInputs(tx)
}

func verifyTxAuth(tx *types.Tx, txID string) error {
	if tx.Pk == "" || tx.Signature == "" || tx.TxID == "" {
		return fmt.Errorf("missing on-chain signature metadata")
	}
	if tx.TxID != txID {
		return fmt.Errorf("txid mismatch: expected %s, got %s", txID, tx.TxID)
	}

	pubKeyBytes, err := decodeHex(tx.Pk)
	if err != nil {
		return fmt.Errorf("invalid public key format")
	}
	if tx.From != HashHex(pubKeyBytes) {
		return fmt.Errorf("from does not match pk hash")
	}

	sigPreimage, err := (&SignedTransaction{
		Type:    tx.Type,
		From:    tx.From,
		Amount:  tx.Amount,
		Payload: tx.Payload,
		Inputs:  tx.Inputs,
		Outputs: tx.Outputs,
	}).SerializeTransactionSignature()
	if err != nil {
		return fmt.Errorf("failed to serialize signature preimage: %w", err)
	}
	return VerifySignature(pubKeyBytes, sigPreimage, tx.Signature)
}

func validateWorkForTarget(blk *types.Block, target *big.Int) error {
	if !IsBlockComplexityValid(*blk, target) {
		return fmt.Errorf("block hash above target")
	}
	return nil
}

// enforceDifficultyTarget returns the target to validate against and rejects
// blocks that try to claim an easier difficulty than the chain expects.
func enforceDifficultyTarget(blk *types.Block, chainTarget *big.Int) (*big.Int, error) {
	if blk == nil {
		return nil, fmt.Errorf("nil block")
	}
	blkTarget := DecodeDifficulty(blk.Header.Difficulty)
	if blkTarget == nil {
		// If no difficulty is set on the block, fall back to chain target.
		return chainTarget, nil
	}
	if chainTarget != nil && blkTarget.Cmp(chainTarget) > 0 {
		// Larger target = easier work -> reject difficulty spoofing.
		return nil, fmt.Errorf("block difficulty easier than expected")
	}
	// Accept blocks that are equal or harder (smaller/equal target).
	return blkTarget, nil
}

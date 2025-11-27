package impl

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// NamecoinState abstracts the state machine that applies Namecoin txs. The
// concrete implementation can enforce ownership, balances, and expirations.
type State interface {
	// ApplyTx validates and applies a tx against the provided block height.
	ApplyTx(tx types.Tx, height uint64) error
	// Clone returns a deep copy suitable for speculative validation.
	Clone() State
}

// NamecoinBlockValidator validates Namecoin blocks against chain rules and
// state. It should not mutate the consensus state on failure.
type BlockValidator interface {
	ValidateBlock(block types.Block) error
}

// NamecoinBlockApplier applies a validated block to consensus state and
// persistence.
type BlockApplier interface {
	ApplyBlock(block types.Block) error
}

// PoWHeaderBuilderFactory builds a PoWHeaderBuilder for a given base header.
// The returned PoWHeaderBuilder must encode nonce and timestamp deterministically.
type PoWHeaderBuilderFactory func(base types.BlockHeader) PoWHeaderBuilder

// NamecoinConsensus wires PoW mining with block validation/apply. It is an
// abstraction layer so PoW can be reused with different state/chain
// implementations.
type NamecoinConsensus struct {
	powCfg        peer.PoWConfig
	baseHeader    types.BlockHeader
	buildHeader   PoWHeaderBuilderFactory
	assembleBlock func(h types.BlockHeader) types.Block
	applier       BlockApplier
	validator     BlockValidator
}

// NamecoinConsensusFactory creates a consensus helper. The caller provides how to
// build headers for hashing, how to construct full blocks when a nonce is
// found, and how to validate/apply them. Returns an error if required
// dependencies are missing.
func NamecoinConsensusFactory(
	powCfg peer.PoWConfig,
	baseHeader types.BlockHeader,
	buildHeader PoWHeaderBuilderFactory,
	assembler func(h types.BlockHeader) types.Block,
	validator BlockValidator,
	applier BlockApplier,
) (*NamecoinConsensus, error) {
	if err := validateConsensusDeps(powCfg, buildHeader, assembler, validator, applier); err != nil {
		return nil, err
	}
	return &NamecoinConsensus{
		powCfg:        powCfg,
		baseHeader:    baseHeader,
		buildHeader:   buildHeader,
		assembleBlock: assembler,
		applier:       applier,
		validator:     validator,
	}, nil
}

// MineAndApply mines a block using the provided stop channel, validates it, and
// applies it. It returns the mined block and any error from validation/apply.
func (c *NamecoinConsensus) MineAndApply(stop <-chan struct{}) (types.Block, error) {
	if c.buildHeader == nil || c.assembleBlock == nil || c.applier == nil || c.validator == nil {
		return types.Block{}, ErrNotInitialized
	}
	// Build the header builder for PoW hashing using the current base header.
	headerBuilder := c.buildHeader(c.baseHeader)
	nonce, _, ok := MineNonce(headerBuilder, c.powCfg, stop)
	if !ok {
		return types.Block{}, ErrMiningAborted
	}
	// Rebuild the full block with the winning nonce/timestamp. Timestamp is
	// baked inside the header builder via the PowHeaderBuilder call.
	ts := time.Now().Unix()
	if c.powCfg.TimeSource != nil {
		ts = c.powCfg.TimeSource().Unix()
	}
	hdr := c.baseHeader
	hdr.Nonce = nonce
	hdr.Timestamp = ts
	block := c.assembleBlock(hdr)
	if err := c.validator.ValidateBlock(block); err != nil {
		return block, err
	}
	if err := c.applier.ApplyBlock(block); err != nil {
		return block, err
	}
	return block, nil
}

// UpdatePoWConfig replaces the PoW configuration (used for tests).
func (c *NamecoinConsensus) UpdatePoWConfig(cfg peer.PoWConfig) {
	c.powCfg = cfg
}

// SetBaseHeader updates the base header (height, prevHash, tx root, etc.) used
// for subsequent mining attempts.
func (c *NamecoinConsensus) SetBaseHeader(h types.BlockHeader) {
	c.baseHeader = h
}

// Errors used by the consensus helper.
var (
	ErrNotInitialized = xerrors.New("namecoin consensus not fully initialized")
	ErrMiningAborted  = xerrors.New("mining aborted")
	ErrInvalidConfig  = xerrors.New("namecoin consensus invalid configuration")
)

func validateConsensusDeps(
	cfg peer.PoWConfig,
	buildHeader PoWHeaderBuilderFactory,
	assembler func(h types.BlockHeader) types.Block,
	validator BlockValidator,
	applier BlockApplier,
) error {
	if cfg.Target == nil || buildHeader == nil || assembler == nil || validator == nil || applier == nil {
		return ErrInvalidConfig
	}
	return nil
}

func (n *node) SubmitTransaction(tx types.SignedTransaction) (string, error) {
	//dummy implementation. I need it to be able to run the gui
	valid, err := veriifySignature(tx)
	if err != nil {
		return "", xerrors.Errorf("failed to verify signature: %v", err)
	}
	if !valid {
		return "", xerrors.New("invalid signature")
	}
	return tx.Tx.TransactionID, nil
}

func veriifySignature(tx types.SignedTransaction) (bool, error) {
	pK := tx.Tx.Source
	pKBytes, err := hex.DecodeString(pK)
	if err != nil {
		return false, xerrors.New("invalid public key format")
	}

	if len(pKBytes) != ed25519.PublicKeySize {
		return false, xerrors.New("invalid public key size")
	}

	publicKey := ed25519.PublicKey(pKBytes)

	message := fmt.Sprintf("%s%s%d%s%d%s",
		tx.Tx.Type,
		tx.Tx.Source,
		tx.Tx.Fee,
		tx.Tx.Payload,
		tx.Tx.Nonce,
		tx.Tx.TransactionID,
	)
	messageBytes := []byte(message)

	// Decode the signature from hex
	signatureBytes, err := hex.DecodeString(tx.Signature)
	if err != nil {
		return false, fmt.Errorf("invalid signature hex: %w", err)
	} // Verify the signature
	valid := ed25519.Verify(publicKey, messageBytes, signatureBytes)

	return valid, nil
}

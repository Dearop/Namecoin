package impl

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// PoWHeaderBuilderFactory builds a PoWHeaderBuilder for a given base header.
// The returned PoWHeaderBuilder must encode nonce and timestamp deterministically.
type PoWHeaderBuilderFactory func(baseBytes []byte) PoWHeaderBuilder

// NamecoinConsensus wires PoW mining with block validation/apply. It is an
// abstraction layer so PoW can be reused with different state/chain
// implementations.
type NamecoinConsensus struct {
	powCfg      peer.PoWConfig
	buildHeader PoWHeaderBuilderFactory
	txBuffer    *TxBuffer
}

// NewNamecoinConsensus creates a consensus helper. The caller provides how to
// build headers for hashing, how to construct full blocks when a nonce is
// found, and how to validate/apply them. Returns an error if required
// dependencies are missing.
func NewNamecoinConsensus(
	powCfg peer.PoWConfig,
	txBuffer *TxBuffer,
) (*NamecoinConsensus, error) {
	if err := validateConsensusDeps(powCfg, txBuffer); err != nil {
		return nil, err
	}
	return &NamecoinConsensus{
		powCfg:      powCfg,
		txBuffer:    txBuffer,
		buildHeader: HeaderBuilderFactory(),
	}, nil
}

func HeaderBuilderFactory() PoWHeaderBuilderFactory {
	return func(baseBytes []byte) PoWHeaderBuilder {
		return func(nonce uint64, ts int64) []byte {
			buf := bytes.Buffer{}
			buf.Write(baseBytes)

			b := make([]byte, 8)
			binary.LittleEndian.PutUint64(b, uint64(ts))
			buf.Write(b)

			binary.LittleEndian.PutUint64(b, nonce)
			buf.Write(b)

			return buf.Bytes()
		}
	}
}

// MineAndApply mines a block using the provided stop channel, validates it, and
// applies it. It returns the mined block and any error from validation/apply.
func (c *NamecoinConsensus) MineAndApply(stop <-chan struct{}, baseHeader *types.BlockHeader) (types.Block, error) {
	// Build the header builder for PoW hashing using the current base header.
	if baseHeader == nil {
		return types.Block{}, xerrors.Errorf("invalid base header for namecoin miner")
	}

	headerBuilder := c.buildHeader(baseHeader.SerializeBase())
	nonce, ok := MineNonce(headerBuilder, c.powCfg, stop)
	if !ok {
		return types.Block{}, ErrMiningAborted
	}
	// Rebuild the full block with the winning nonce/timestamp. Timestamp is
	// baked inside the header builder via the PowHeaderBuilder call.
	ts := time.Now().Unix()
	if c.powCfg.TimeSource != nil {
		ts = c.powCfg.TimeSource().Unix()
	}
	hdr := baseHeader
	hdr.Nonce = nonce
	hdr.Timestamp = ts
	// we need to ensure that the order of the transaction is not changed in case of block complexity is less than expected
	// Requeuing the transaction and Draining them should preserve order otherwise
	//Hash will be inconsistent between 2 Hash executions
	pending, order, snapshot := c.txBuffer.Drain()
	block := AssembleBlock(hdr, pending, c.powCfg.PubKey)
	isComplexityValid := IsBlockComplexityValid(block, c.powCfg.Target)
	if !isComplexityValid {
		c.txBuffer.Requeue(order, snapshot)
		return block, xerrors.Errorf("block complexity is not valid")
	}

	return block, nil
}

// UpdatePoWConfig replaces the PoW configuration (used for tests).
func (c *NamecoinConsensus) UpdatePoWConfig(cfg peer.PoWConfig) {
	c.powCfg = cfg
}

// Errors used by the consensus helper.
var (
	ErrMiningAborted = xerrors.New("mining aborted")
	ErrInvalidConfig = xerrors.New("namecoin consensus invalid configuration")
)

func validateConsensusDeps(cfg peer.PoWConfig, buffer *TxBuffer) error {
	if cfg.Target == nil || buffer == nil {
		return ErrInvalidConfig
	}
	return nil
}

func AssembleBlock(h *types.BlockHeader, pending []types.Tx, minersPubKey string) types.Block {
	rewardTx := types.Tx{
		From:    minersPubKey,
		Type:    RewardCommandName,
		Amount:  1,
		Payload: json.RawMessage(fmt.Sprintf(`{"height":%d}`, h.Height)), // to make every block unique
		Outputs: []types.TxOutput{
			{
				To:     minersPubKey,
				Amount: 1,
			},
		},
	}

	txs := append([]types.Tx{rewardTx}, pending...)

	root, err := computeTxRoot(txs)
	if err != nil {
		panic(fmt.Sprintf("failed to compute TxRoot: %v", err))
	}
	h.TxRoot = root

	block := types.Block{
		Header:       *h,
		Transactions: txs,
	}

	block.Hash = block.ComputeHash()

	return block
}

package impl

import (
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"go.dedis.ch/cs438/types"
)

const (
	DefaultMaxPending          = 10_000
	DefaultMaxPerSender        = 100
	MaxNameLength              = 255
	MaxValueLength             = 4 * 1024
	MaxFeePerTx         uint64 = 1_000_000_000 // arbitrary sanity bound
)

// ---- State ----

// State is the in-memory state rebuilt from the Namecoin chain
type NamecoinState struct {
	// Domain name -> record
	Domains map[string]types.NameRecord

	// Simple coin balances per address
	Balances map[string]uint64

	// Pending transactions that have not yet been included in any block
	// (cleaned up during replay when we see them in a block)
	Pending *PendingPool // map[string]*Tx
}

// NewState creates an empty state with fresh maps and a pending pool
func NewState() *NamecoinState {
	return &NamecoinState{
		Domains:  make(map[string]types.NameRecord),
		Balances: make(map[string]types.Balance),
		Pending:  NewPendingPool(DefaultMaxPending, DefaultMaxPerSender),
	}
}

func (s *NamecoinState) Clone() *NamecoinState {
	if s == nil {
		return NewState()
	}
	out := &NamecoinState{
		Domains:  maps.Clone(s.Domains),
		Balances: maps.Clone(s.Balances),
	}

	// Clone pending pool (optional but keeps behaviour close to before).
	if s.Pending != nil {
		out.Pending = NewPendingPool(s.Pending.maxTotal, s.Pending.maxPerFrom)
		for _, tx := range s.Pending.SnapshotPending() {
			// Ignore errors here – pending pool is best-effort.
			_ = out.Pending.Add(tx)
		}
	} else {
		out.Pending = NewPendingPool(DefaultMaxPending, DefaultMaxPerSender)
	}
	return out
}

// ---- Pending Pool ----

// PendingPool is a bounded mempool with per-sender limits
type PendingPool struct {
	mu         sync.Mutex
	byID       map[string]*types.Tx
	perSender  map[string]int
	maxTotal   int
	maxPerFrom int
}

func NewPendingPool(maxTotal, maxPerSender int) *PendingPool {
	return &PendingPool{
		byID:       make(map[string]*types.Tx),
		perSender:  make(map[string]int),
		maxTotal:   maxTotal,
		maxPerFrom: maxPerSender,
	}
}

// Add a tx if it passes basic anti-spam checks
func (p *PendingPool) Add(tx *types.Tx) error {
	if tx == nil || tx.ID == nil {
		return errors.New("pending: nil tx or id")
	}
	id := string(tx.ID)
	from := tx.Payload.From

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.byID[id]; exists {
		return nil // idempotent
	}
	if len(p.byID) >= p.maxTotal {
		return fmt.Errorf("pending: full (max %d)", p.maxTotal)
	}
	if from != "" && p.perSender[from] >= p.maxPerFrom {
		return fmt.Errorf("pending: too many tx from %s", from)
	}

	// Guardrail size checks
	if len(tx.Payload.Name) > MaxNameLength {
		return fmt.Errorf("pending: name too long")
	}
	if len(tx.Payload.Value) > MaxValueLength {
		return fmt.Errorf("pending: value too long")
	}
	if tx.Payload.Fee > MaxFeePerTx {
		return fmt.Errorf("pending: fee too large")
	}

	p.byID[id] = tx
	if from != "" {
		p.perSender[from]++
	}
	return nil
}

// Remove drops a tx from the pool by id
func (p *PendingPool) Remove(id []byte) {
	if id == nil {
		return
	}
	key := string(id)

	p.mu.Lock()
	defer p.mu.Unlock()

	tx, ok := p.byID[key]
	if !ok {
		return
	}
	delete(p.byID, key)
	from := tx.Payload.From
	if from != "" {
		if n := p.perSender[from]; n > 1 {
			p.perSender[from] = n - 1
		} else {
			delete(p.perSender, from)
		}
	}
}

// SnapshotPending returns a shallow copy of all pending tx for iteration
func (p *PendingPool) SnapshotPending() []*types.Tx {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]*types.Tx, 0, len(p.byID))
	for _, tx := range p.byID {
		out = append(out, tx)
	}
	return out
}

// ---- Helpers ----

const (
	DomainTTLBlocks  uint64 = 36_000
	BlockSubsidyBase uint64 = 50 * 1e8
	MinTxFee         uint64 = 1e4
	MaxClockSkewSecs        = 2 * 60
	opCoinbase              = "coinbase"
)

// BlockContext is supplied by the chain when applying a block
type BlockContext struct {
	Height    uint64
	Timestamp int64  // unix seconds
	Miner     string // miner address from header
}

// Credit / Debit helpers.

func (s *NamecoinState) credit(addr string, amount uint64) {
	if addr == "" || amount == 0 {
		return
	}
	s.Balances[addr] += amount
}

func (s *NamecoinState) debit(addr string, amount uint64) error {
	if addr == "" {
		return fmt.Errorf("missing address")
	}
	cur := s.Balances[addr]
	if cur < amount {
		return fmt.Errorf("insufficient balance: have %d, need %d", cur, amount)
	}
	s.Balances[addr] = cur - amount
	return nil
}

// pruneExpired removes expired domains for the given height.
func (s *NamecoinState) pruneExpired(height uint64) {
	for name, rec := range s.Domains {
		if rec.ExpiresAt != 0 && rec.ExpiresAt <= height {
			delete(s.Domains, name)
		}
	}
}

// ---- Core Apply Tx ----

// ApplyTx applies a single tx to the state under a given block context
// ApplyTx validates ownership, fees, expiry, and updates domain/balance state
// for each tx type. Balance debits/credits deferred to block reward/fees
// nolint:funlen // validation logic is kept inline for readability
func (s *NamecoinState) ApplyTx(tx *types.Tx, ctx BlockContext) error {
	if s == nil {
		return fmt.Errorf("apply tx: nil NamecoinState")
	}
	if tx == nil || tx.ID == nil {
		return fmt.Errorf("apply tx: nil tx")
	}

	p := tx.Payload
	op := p.Op

	// Very basic timestamp sanity (optional).
	if ctx.Timestamp > 0 {
		now := time.Now().Unix()
		if ctx.Timestamp > now+MaxClockSkewSecs {
			return fmt.Errorf("apply tx: block timestamp too far in future")
		}
	}

	// Apply fees first (except coinbase)
	if op != opCoinbase && p.Fee > 0 {
		if p.Fee < MinTxFee {
			return fmt.Errorf("fee below minimum")
		}
		if err := s.debit(p.From, p.Fee); err != nil {
			return fmt.Errorf("fee debit failed: %w", err)
		}
	}

	// Prune expired domains before applying name ops
	s.pruneExpired(ctx.Height)

	switch op {

	case opCoinbase:
		// Block subsidy + fees will be handled at block level; here we assume
		// this payload just transfers Amount to Miner or To
		if p.Amount == 0 {
			return nil
		}
		target := p.To
		if target == "" {
			target = ctx.Miner
		}
		if target == "" {
			return fmt.Errorf("coinbase without target address")
		}
		s.credit(target, p.Amount)
		return nil

	case "pay":
		if p.Name != "" {
			return fmt.Errorf("pay must not carry name")
		}
		if p.Amount == 0 {
			return fmt.Errorf("pay with zero amount")
		}
		if err := s.debit(p.From, p.Amount); err != nil {
			return fmt.Errorf("pay debit failed: %w", err)
		}
		s.credit(p.To, p.Amount)
		return nil

	case "register":
		// Simple name_new/commit stand-in:
		// - name must not exist (or must be expired)
		// TODO: add commit logic
		if p.Name == "" {
			return fmt.Errorf("register without name")
		}
		if rec, ok := s.Domains[p.Name]; ok && rec.ExpiresAt == 0 || rec.ExpiresAt > ctx.Height {
			return fmt.Errorf("name %s already registered", p.Name)
		}
		s.Domains[p.Name] = types.NameRecord{
			Owner:     p.From,
			Value:     p.Value,
			ExpiresAt: ctx.Height + DomainTTLBlocks,
		}
		return nil

	case "firstupdate":
		// TODO: In real Namecoin this reveals a commit
		if p.Name == "" {
			return fmt.Errorf("firstupdate without name")
		}
		rec, ok := s.Domains[p.Name]
		if !ok {
			return fmt.Errorf("firstupdate for unknown name %s", p.Name)
		}
		if rec.Owner != p.From {
			return fmt.Errorf("firstupdate: not owner")
		}

		// Update value and refresh expiry
		rec.Value = p.Value
		rec.ExpiresAt = ctx.Height + DomainTTLBlocks
		s.Domains[p.Name] = rec
		return nil

	case "update":
		if p.Name == "" {
			return fmt.Errorf("update without name")
		}
		rec, ok := s.Domains[p.Name]
		if !ok {
			return fmt.Errorf("update non-existent name %s", p.Name)
		}
		if rec.ExpiresAt != 0 && rec.ExpiresAt <= ctx.Height {
			return fmt.Errorf("update on expired name %s", p.Name)
		}
		if rec.Owner != p.From {
			return fmt.Errorf("update: not owner")
		}
		rec.Value = p.Value
		// Expand expiry on update
		rec.ExpiresAt = ctx.Height + DomainTTLBlocks
		s.Domains[p.Name] = rec
		return nil

	case "transfer":
		if p.Name == "" {
			return fmt.Errorf("transfer without name")
		}
		if p.To == "" {
			return fmt.Errorf("transfer without recipient")
		}
		rec, ok := s.Domains[p.Name]
		if !ok {
			return fmt.Errorf("transfer non-existent name %s", p.Name)
		}
		if rec.ExpiresAt != 0 && rec.ExpiresAt <= ctx.Height {
			return fmt.Errorf("transfer on expired name %s", p.Name)
		}
		if rec.Owner != p.From {
			return fmt.Errorf("transfer: not owner")
		}
		rec.Owner = p.To
		s.Domains[p.Name] = rec
		return nil

	default:
		// Unknown op: treat as invalid
		return fmt.Errorf("unknown op %q", op)
	}
}

// ApplyBlock applies all txs in a block to the state
// - Validates each tx via ApplyTx
// - Aggregates fees and optional block subsidy to the miner
func (s *NamecoinState) ApplyBlock(b *types.Block) error {
	if s == nil {
		return fmt.Errorf("apply block: nil state")
	}
	if b == nil {
		return fmt.Errorf("apply block: nil block")
	}

	ctx := BlockContext{
		Height:    b.Header.Height,
		Timestamp: b.Header.Timestamp,
		Miner:     b.Header.Miner,
	}

	var totalFees uint64
	for i := range b.Transactions {
		tx := &b.Transactions[i]
		p := tx.Payload

		// Track fees for miner, except coinbase
		if p.Op != "coinbase" {
			totalFees += p.Fee
		}

		if err := s.ApplyTx(tx, ctx); err != nil {
			return fmt.Errorf("block %d tx %d failed: %w", b.Header.Height, i, err)
		}

		// Remove from pending pool
		if s.Pending != nil && tx.ID != nil {
			s.Pending.Remove(tx.ID)
		}
	}

	// Reward miner: basic subsidy + totalFees
	if b.Header.Miner != "" {
		s.credit(b.Header.Miner, BlockSubsidyBase+totalFees)
	}

	return nil
}

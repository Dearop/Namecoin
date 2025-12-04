package impl

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"go.dedis.ch/cs438/types"
	"maps"
	"os"
	"sync"
)

// DomainTTLBlocks is the default number of blocks a domain will remain registered for.
// DefaultDomainTTLBlocks defines the default number of blocks a domain stays valid.
var DefaultDomainTTLBlocks uint64 = 36_000

// NamecoinState is the in-memory state rebuilt from the Namecoin chain
type NamecoinState struct {
	// Domain name -> record
	Domains map[string]types.NameRecord
	// Expiry index: height -> domains expiring at that height
	expires map[uint64][]string
	// Tracks current height processed
	currentHeight uint64
	// Configurable TTL for domains
	domainTTL uint64

	// Commitment -> hashed Domain and Salt
	Commitments map[string]string
	// Optional TTL preference keyed by commitment hash
	commitmentTTLs map[string]uint64

	// Simple coin balances per address
	UTXOMap map[string]map[string]types.UTXO

	// To deduplicate transactions. Subject to discuss, but now suggest as a temp solution
	txMap map[string]struct{}

	mu sync.RWMutex
}

func (c *NamecoinChain) State() *NamecoinState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// NewState creates an empty state with fresh maps and a pending pool
func NewState() *NamecoinState {
	return &NamecoinState{
		Domains:        make(map[string]types.NameRecord),
		expires:        make(map[uint64][]string),
		Commitments:    make(map[string]string),
		commitmentTTLs: make(map[string]uint64),
		UTXOMap:        make(map[string]map[string]types.UTXO),
		txMap:          make(map[string]struct{}),
		domainTTL:      DefaultDomainTTLBlocks,
	}
}

func (st *NamecoinState) GetDomainOwner(domain string) string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	res, ok := st.Domains[domain]

	if !ok {
		return ""
	}

	return res.Owner
}

func (st *NamecoinState) IsDomainExists(domain string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	_, ok := st.Domains[domain]
	return ok
}

func (st *NamecoinState) GetCommitment(from string) string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Commitments[from]
}

func (st *NamecoinState) GetCommitmentTTL(commitment string) uint64 {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.commitmentTTLs[commitment]
}

// EnsureAccount initialises an empty UTXO map entry for an address to allow zero-input txs in tests.
func (st *NamecoinState) EnsureAccount(addr string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.UTXOMap == nil {
		st.UTXOMap = make(map[string]map[string]types.UTXO)
	}
	if st.UTXOMap[addr] == nil {
		st.UTXOMap[addr] = make(map[string]types.UTXO)
	}
}

func (st *NamecoinState) getDomain(name string) (types.NameRecord, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	rec, ok := st.Domains[name]
	return rec, ok
}

func (st *NamecoinState) SetDomain(record types.NameRecord) {
	st.mu.Lock()
	defer st.mu.Unlock()
	// Remove from old expiry bucket if exists
	if existing, ok := st.Domains[record.Domain]; ok && existing.ExpiresAt != 0 {
		st.removeFromExpiryLocked(record.Domain, existing.ExpiresAt)
	}
	st.Domains[record.Domain] = record
	if record.ExpiresAt != 0 {
		st.addExpiryLocked(record.Domain, record.ExpiresAt)
	}
}

func (st *NamecoinState) SetCommitment(from, commitment string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.Commitments[from] = commitment
}

func (st *NamecoinState) SetCommitmentTTL(commitment string, ttl uint64) {
	if ttl == 0 {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.commitmentTTLs[commitment] = ttl
}

func (st *NamecoinState) Clone() *NamecoinState {
	if st == nil {
		return NewState()
	}
	clone := &NamecoinState{
		Domains:        maps.Clone(st.Domains),
		expires:        make(map[uint64][]string, len(st.expires)),
		Commitments:    maps.Clone(st.Commitments),
		commitmentTTLs: maps.Clone(st.commitmentTTLs),
		UTXOMap:        make(map[string]map[string]types.UTXO, len(st.UTXOMap)),
		txMap:          make(map[string]struct{}, len(st.txMap)),
		currentHeight:  st.currentHeight,
		domainTTL:      st.domainTTL,
	}
	for h, names := range st.expires {
		clone.expires[h] = append([]string(nil), names...)
	}
	for addr, utxos := range st.UTXOMap {
		inner := make(map[string]types.UTXO, len(utxos))
		for txID, utxo := range utxos {
			inner[txID] = utxo
		}
		clone.UTXOMap[addr] = inner
	}

	for id := range st.txMap {
		clone.txMap[id] = struct{}{}
	}

	return clone
}

// SnapshotDomains returns a shallow copy of the domain map for safe read-only use.
func (st *NamecoinState) SnapshotDomains() (map[string]types.NameRecord, uint64) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make(map[string]types.NameRecord, len(st.Domains))
	for k, v := range st.Domains {
		out[k] = v
	}
	return out, st.currentHeight
}

// SnapshotDomainsMap returns only the domain map (without height) for callers that only need records.
func (st *NamecoinState) SnapshotDomainsMap() map[string]types.NameRecord {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make(map[string]types.NameRecord, len(st.Domains))
	for k, v := range st.Domains {
		out[k] = v
	}
	return out
}

func (st *NamecoinState) CurrentHeight() uint64 {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.currentHeight
}

func (st *NamecoinState) setHeight(h uint64) {
	st.mu.Lock()
	st.currentHeight = h
	st.mu.Unlock()
}

func (st *NamecoinState) isExpired(rec types.NameRecord, height uint64) bool {
	return rec.ExpiresAt != 0 && rec.ExpiresAt <= height
}

func (st *NamecoinState) addExpiryLocked(domain string, height uint64) {
	st.expires[height] = append(st.expires[height], domain)
}

func (st *NamecoinState) effectiveTTL(ttl uint64) uint64 {
	if ttl == 0 {
		return st.domainTTL
	}
	return ttl
}

func (st *NamecoinState) removeFromExpiryLocked(domain string, height uint64) {
	if height == 0 {
		return
	}
	names, ok := st.expires[height]
	if !ok {
		return
	}
	for i, name := range names {
		if name == domain {
			// remove without preserving order
			names[i] = names[len(names)-1]
			names = names[:len(names)-1]
			break
		}
	}
	if len(names) == 0 {
		delete(st.expires, height)
	} else {
		st.expires[height] = names
	}
}

func (st *NamecoinState) pruneExpired(height uint64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for expHeight, names := range st.expires {
		if expHeight > height {
			continue
		}
		for _, name := range names {
			if rec, ok := st.Domains[name]; ok && rec.ExpiresAt == expHeight {
				delete(st.Domains, name)
			}
		}
		delete(st.expires, expHeight)
	}
}

// ApplyTx implements minimal Namecoin semantics
// We can harden this later (ownership checks, expires, coins, etc).
func (st *NamecoinState) ApplyTx(txID string, tx *types.Tx) error {

	if st.IsTxApplied(txID) {
		// tx has already been applied
		return nil
	}

	if os.Getenv("GLOG") != "no" {
		log.Debug().Msgf("applying tx type: %s with ID: %s", tx.Type, txID)
	}
	// Reduce noise in tests: log at debug level for normal tx application.
	log.Debug().Msgf("applying tx type: %s with ID: %s", tx.Type, txID)

	// BurnUTXOs first making sure the user hasn't burned the same UTXOs already
	// First transaction in Block is always Reward to ensure that miner gets reward even if user transaction is reverted
	err := st.ProcessCommandTransactionStateUpdate(txID, tx)
	if err != nil {
		return err
	}

	err = st.ProcessCommandStateUpdate(tx)
	if err != nil {
		return err
	}

	st.MarkAsApplied(txID)
	return nil
}

// ApplyBlock applies all txs and prunes included pending txs
// NOTE: kept simple for now, but later we can refactor into
// dedicated modules (e.g., domain, coin, mempool)
func (st *NamecoinState) ApplyBlock(blk *types.Block) error {
	if blk == nil {
		return fmt.Errorf("apply namecoin block: nil block")
	}
	st.setHeight(blk.Header.Height)
	st.pruneExpired(blk.Header.Height)
	for i := range blk.Transactions {
		tx := &blk.Transactions[i]
		txID, err := BuildTransactionID(tx)
		if err != nil {
			return err
		}

		if err = st.ApplyTx(txID, tx); err != nil {
			// for robustness, we log and continue
			warnf("namecoin: failed to apply tx %s at height %d: %v",
				txID, blk.Header.Height, err)
		}
	}
	return nil
}

// IsTxApplied returns true if Tx is already in the state
func (st *NamecoinState) IsTxApplied(txID string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	_, ok := st.txMap[txID]

	return ok
}

func (st *NamecoinState) MarkAsApplied(txID string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.txMap[txID] = struct{}{}
}

func (st *NamecoinState) ProcessCommandTransactionStateUpdate(txID string, tx *types.Tx) error {
	cmd, err := ResolveCommand(tx.Type, tx.Payload)
	if err != nil {
		return err
	}
	return cmd.ProcessTxState(st, txID, tx)
}

func (st *NamecoinState) ProcessCommandStateUpdate(tx *types.Tx) error {
	cmd, err := ResolveCommand(tx.Type, tx.Payload)
	if err != nil {
		return err
	}
	return cmd.ProcessState(st, tx)
}

// ValidateCommand verifies the payload of a transaction based on its type
func (st *NamecoinState) ValidateCommand(tx *SignedTransaction) error {
	cmd, err := ResolveCommand(tx.Type, tx.Payload)
	if err != nil {
		return err
	}
	return cmd.Validate(st, tx)
}

package impl

import (
	"fmt"
	"maps"
	"sync"

	"go.dedis.ch/cs438/types"
)

// NamecoinState is the in-memory state rebuilt from the Namecoin chain
type NamecoinState struct {
	// Domain name -> record
	Domains map[string]types.NameRecord

	// Commitment -> hashed Domain and Salt
	Commitments map[string]string

	// Simple coin balances per address
	UTXOMap map[string]map[string]types.UTXO

	Transactions map[string]map[string]types.Tx

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
		Domains:      make(map[string]types.NameRecord),
		Commitments:  make(map[string]string),
		UTXOMap:      make(map[string]map[string]types.UTXO),
		Transactions: make(map[string]map[string]types.Tx),
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

func (st *NamecoinState) SetDomain(record types.NameRecord) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.Domains[record.Domain] = record
}

func (st *NamecoinState) SetCommitment(from, commitment string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.Commitments[from] = commitment
}

func (st *NamecoinState) Clone() *NamecoinState {
	if st == nil {
		return NewState()
	}
	out := &NamecoinState{
		Domains: maps.Clone(st.Domains),
		UTXOMap: maps.Clone(st.UTXOMap),
	}

	return out
}

// ApplyTx implements minimal Namecoin semantics
// We can harden this later (ownership checks, expires, coins, etc).
func (st *NamecoinState) ApplyTx(txID string, tx *types.Tx) error {

	if st.IsTxApplied(tx.From, txID) {
		// tx has already been applied
		return nil
	}

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
	return nil
}

// ApplyBlock applies all txs and prunes included pending txs
// NOTE: kept simple for now, but later we can refactor into
// dedicated modules (e.g., domain, coin, mempool)
func (st *NamecoinState) ApplyBlock(blk *types.Block) error {
	if blk == nil {
		return fmt.Errorf("apply namecoin block: nil block")
	}
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
func (st *NamecoinState) IsTxApplied(from, txID string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	_, ok := st.UTXOMap[from][txID]

	return ok
}

package impl

import (
	"fmt"
	"maps"
	"strings"
	"sync"

	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
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

// BurnUTXO function deletes UTXOs for corresponding pub Key "from"
func (st *NamecoinState) BurnUTXO(from string, txIDs []string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, txID := range txIDs {
		// burn UTXOs corresponding to TxID
		if _, ok := st.UTXOMap[from][txID]; !ok {
			return xerrors.Errorf("burn txID %s not found", from)
		}

		delete(st.UTXOMap[from], txID)
	}

	return nil
}

// AppendUTXO appends UTXO to users storage aka "balance top up"
func (st *NamecoinState) AppendUTXO(utxo types.UTXO) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if _, ok := st.UTXOMap[utxo.To][utxo.TxID]; ok {
		return xerrors.New("tx already exists")
	}

	st.UTXOMap[utxo.To][utxo.TxID] = utxo
	return nil

}

// GetUTXOsToBurn returns utxo IDs to burn and leftover UTXO.
func (st *NamecoinState) GetUTXOsToBurn(txID, from string, amount uint64) ([]string, []types.UTXO, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	userUTXOs := st.UTXOMap[from]
	UTXOsToBurn := make([]string, 0)

	// deduct until we burn enough UTXOs to pay
	for key, utxo := range userUTXOs {
		amount -= utxo.Amount
		UTXOsToBurn = append(UTXOsToBurn, key)

		if amount <= 0 {
			break
		}
	}

	// if amount still > 0 means that the user has not enough UTXOs to burn, revert
	if amount > 0 {
		return make([]string, 0), make([]types.UTXO, 0), xerrors.New("insufficient funds")
	}

	leftOver := 0 - amount

	if leftOver == 0 {
		return UTXOsToBurn, make([]types.UTXO, 0), nil
	}

	leftOverUTXO := types.UTXO{
		TxID:   txID,
		To:     from,
		Amount: leftOver,
	}

	// safe to return transactionIDS because we create only one UTXO per transaction
	// example, miner A mined a block; transaction is created - as a result, UTXO with corresponding txID is created.
	return UTXOsToBurn, append(make([]types.UTXO, 0, 1), leftOverUTXO), nil
}

var (
	NameNewCommandName         = NameNew{}.Name()
	NameFirstUpdateCommandName = NameFirstUpdate{}.Name()
	NameUpdateCommandName      = NameUpdate{}.Name()
	RewardCommandName          = Reward{}.Name()
)

// ApplyTx implements minimal Namecoin semantics
// We can harden this later (ownership checks, expires, coins, etc).
func (st *NamecoinState) ApplyTx(txID string, tx *types.Tx) error {

	if st.IsTxApplied(tx.From, txID) {
		// tx has already been applied
		return nil
	}

	// BurnUTXOs first making sure the user hasn't burned the same UTXOs already
	// First transaction in Block is always Reward to ensure that miner gets reward even if user transaction is reverted
	switch tx.Type {
	case NameNewCommandName, NameFirstUpdateCommandName, NameUpdateCommandName:
		txIDs := make([]string, len(tx.Inputs))
		for i, value := range tx.Inputs {
			txIDs[i] = value.TxID
		}

		err := st.BurnUTXO(tx.From, txIDs)
		if err != nil {
			return err
		}

		// 1 or 0 UTXOs
		for _, value := range tx.Outputs {
			utxo := types.UTXO{
				TxID:   txID,
				To:     value.To,
				Amount: value.Amount,
			}
			err = st.AppendUTXO(utxo)
		}
	case RewardCommandName:
		// On Reward - always 1 UTXO
		utxo := types.UTXO{
			TxID:   txID,
			To:     tx.Outputs[0].To,
			Amount: tx.Outputs[0].Amount,
		}

		// save UTXO that rewards miner
		err := st.AppendUTXO(utxo)

		if err != nil {
			return err
		}
	}

	switch tx.Type {
	case NameNewCommandName:
		cmd, err := ResolveNameCoinCommand[NameNew](tx.Type, tx.Payload)

		if err != nil {
			return err
		}

		// we don't reveal the name on the initial domain creation, look at the project description
		st.SetCommitment(tx.From, cmd.Commitment)
	case NameFirstUpdateCommandName:
		cmd, err := ResolveNameCoinCommand[NameFirstUpdate](tx.Type, tx.Payload)
		if err != nil {
			return err
		}

		if st.IsDomainExists(cmd.Domain) {
			return xerrors.New("Domain already exists")
		}

		st.SetDomain(types.NameRecord{
			Owner:     tx.From,
			IP:        cmd.IP,
			Domain:    cmd.Domain,
			Salt:      cmd.Salt,
			ExpiresAt: 0, // todo: Add expiration
		})
	case NameUpdateCommandName:
		cmd, err := ResolveNameCoinCommand[NameUpdate](tx.Type, tx.Payload)
		if err != nil {
			return err
		}

		// rec is copy, changing it without a lock, then updating with lock.
		rec, ok := st.Domains[cmd.Domain]
		if !ok {
			return fmt.Errorf("updating non-existent domain %s", cmd.Domain)
		}

		// update only if the value is set. If value equals "", no updates have been made
		if len(strings.TrimSpace(cmd.Domain)) != 0 {
			rec.Domain = cmd.Domain
		}
		if len(strings.TrimSpace(cmd.IP)) != 0 {
			rec.IP = cmd.IP
		}

		st.SetDomain(rec)
		// todo: refresh domain lifetime

	case RewardCommandName:
		// nothing to verify specifically.
	default:
		// Unknown op: treat as no-op
		//todo: remove difference between Namecoin block and other, it should be one block that contains different transactions.
		warnf("namecoin: unknown tx op %q in tx %s", tx.Type, txID)
	}

	return nil
}

// ApplyBlockToState applies all txs and prunes included pending txs
// NOTE: kept simple for now, but later we can refactor into
// dedicated modules (e.g., domain, coin, mempool)
func ApplyBlockToState(st *NamecoinState, blk *types.Block) error {
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

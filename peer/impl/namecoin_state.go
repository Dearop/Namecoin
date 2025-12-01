package impl

import (
	"encoding/json"
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

// -- helper
func BuildTransactionID(tx *types.Tx) (string, error) {
	b, err := SerializeTransaction(tx)

	if err != nil {
		return "", err
	}

	return HashHex(b), nil
}

// -- helper
func SerializeTransaction(tx *types.Tx) ([]byte, error) {
	data := map[string]interface{}{
		"type":    tx.Type,
		"from":    tx.From,
		"amount":  tx.Amount,
		"payload": tx.Payload,
	}

	b, err := json.Marshal(data)
	if err != nil {
		return make([]byte, 0), err
	}

	return b, err
}

// Add a tx if it passes basic anti-spam checks
func (p *PendingPool) Add(tx *types.Tx) error {
	txID, err := BuildTransactionID(tx)
	if err != nil {
		return err
	}

	from := tx.From

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.byID[txID]; exists {
		return nil // idempotent
	}
	if len(p.byID) >= p.maxTotal {
		return fmt.Errorf("pending: full (max %d)", p.maxTotal)
	}
	if from != "" && p.perSender[from] >= p.maxPerFrom {
		return fmt.Errorf("pending: too many tx from %s", from)
	}

	p.byID[txID] = tx
	if from != "" {
		p.perSender[from]++
	}
	return nil
}

// Remove drops a tx from the pool by id
func (p *PendingPool) Remove(txID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	tx, ok := p.byID[txID]
	if !ok {
		return
	}
	delete(p.byID, txID)
	from := tx.From
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
)

// BlockContext is supplied by the chain when applying a block
type BlockContext struct {
	Height    uint64
	Timestamp int64  // unix seconds
	Miner     string // miner address from header
}

// pruneExpired removes expired domains for the given height.
func (st *NamecoinState) pruneExpired(height uint64) {
	for name, rec := range st.Domains {
		if rec.ExpiresAt != 0 && rec.ExpiresAt <= height {
			delete(st.Domains, name)
		}
	}
}

// ---- Core Apply Tx ----

// BurnUTXO applies a single tx to the state under a given block context
// BurnUTXO validates ownership, fees, expiry, and updates domain/balance state
// for each tx type. Balance debits/credits deferred to block reward/fees

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
func (st *NamecoinState) GetUTXOsToBurn(txID, from string, amount uint64) ([]string, *types.UTXO, error) {
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
		return make([]string, 0), nil, xerrors.New("insufficient funds")
	}

	leftOver := 0 - amount

	leftOverUTXO := types.UTXO{
		TxID:   txID,
		To:     from,
		Amount: leftOver,
	}

	// safe to return transactionIDS because we create only one UTXO per transaction
	// example, miner A mined a block; transaction is created - as a result, UTXO with corresponding txID is created.
	return UTXOsToBurn, &leftOverUTXO, nil
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

	switch tx.Type {
	case NameNewCommandName:
		cmd, err := ResolveNameCoinCommand[NameNew](tx.Type, tx.Payload)

		if err != nil {
			return err
		}

		// we don't reveal name on the initial domain creation, look at project description
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
			ExpiresAt: 0,
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

	case RewardCommandName:
		// nothing to verify specifically.
	default:
		// Unknown op: treat as no-op
		//todo: remove difference between Namecoin block and other, it should be one block that contains different transactions.
		warnf("namecoin: unknown tx op %q in tx %s", tx.Type, txID)
	}

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
	case RewardCommandName:
		utxo := types.UTXO{
			TxID:   txID,
			To:     tx.Output.To,
			Amount: tx.Output.Amount,
		}

		// save UTXO that rewards miner
		err := st.AppendUTXO(utxo)

		if err != nil {
			return err
		}
	}
	return nil
}

func (st *NamecoinState) IsTxApplied(from, txID string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	_, ok := st.UTXOMap[from][txID]

	return ok
}

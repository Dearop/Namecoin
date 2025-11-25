package impl

import (
	"fmt"
	"sync"

	"go.dedis.ch/cs438/types"
)

func NewState() *TmpState {
	return &TmpState{
		transactions: map[string]*types.Tx{},
		// todo: not sure, teacher said that we don't need MemPool
		MemPool:      make([]*SignedTransaction, 0),
		Commitments:  map[string]string{},
		DomainOwners: map[string]string{}}
}

// TmpState todo: replace it with actual blockchain storage.
type TmpState struct {
	transactions map[string]*types.Tx
	// todo: not sure, teacher said that we don't need MemPool
	MemPool      []*SignedTransaction
	Commitments  map[string]string
	DomainOwners map[string]string

	mu sync.RWMutex
}

// GetPendingTransactions returns all transactions waiting to be mined.
func (t *TmpState) GetPendingTransactions() []*SignedTransaction {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.MemPool
}

// DropTransaction removes a transaction from the pending pool.
func (t *TmpState) DropTransaction(txID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, tx := range t.MemPool {
		if tx.TxID == txID {
			t.MemPool = append(t.MemPool[:i], t.MemPool[i+1:]...)
			return
		}
	}
}

// DropTransactions removes all transactions from the pending pool.
func (t *TmpState) DropTransactions() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.MemPool = make([]*SignedTransaction, 0)
}

// ApplyTransaction stores transaction in the state
func (t *TmpState) ApplyTransaction(txID string, tx types.Tx) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.transactions[txID] = &tx
}

// GetTransaction returns a transaction by its txId if known otherwise nil.
func (t *TmpState) GetTransaction(txID string) *types.Tx {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.transactions[txID]
}

// NewTransactionService creates a new TransactionService instance.
func NewTransactionService(blockchain *TmpState) *TransactionService {
	return &TransactionService{
		TokenManager: NewTokenWalletManager(),
		state:        blockchain}
}

// TransactionService implements ITransactionService
type TransactionService struct {
	TokenManager *TokenWalletManager
	state        *TmpState
}

func (t *TransactionService) ApplyTransaction(txID string, tx types.Tx) (uint64, error) {
	// todo: replace with actual storing on blockchain
	balance, err := t.TokenManager.ChargeAndGet(tx.From, tx.Fee)
	if err != nil {
		return 0, err
	}

	t.state.ApplyTransaction(txID, tx)

	// return balance as it will be used in response to frontend to dynamically update balance
	return balance, err
}

func (t *TransactionService) ValidateTransaction(tx *SignedTransaction) error {
	// 1. Decode public key
	pubKeyBytes, err := decodeHex(tx.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key format")
	}

	// 2. Check that H(publicKey) == From to ensure the sender is indeed the owner of the public key
	err = t.TokenManager.VerifyOwnership(tx.From, pubKeyBytes)
	if err != nil {
		return err
	}

	// 3. Recompute TxID from unsigned transaction
	unsignedBytes := BuildUnsignedTxBytes(tx)
	computedTxID := HashHex(unsignedBytes)

	if computedTxID != tx.TxID {
		return fmt.Errorf("txId mismatch: expected %s, got %s", computedTxID, tx.TxID)
	}

	// 4. Verify signature
	err = VerifySignature(pubKeyBytes, unsignedBytes, tx.Signature)
	if err != nil {
		return err
	}

	// 5. Check user balance use TokenWalletManager
	// balance deduction happens on submitting transaction
	balance := t.TokenManager.GetBalance(tx.From)
	if balance < tx.Fee {
		return fmt.Errorf("insufficient funds")
	}

	// 6. Validate payload based on transaction type
	err = t.verifyCommand(tx)
	if err != nil {
		return err
	}

	return nil
}

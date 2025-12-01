package impl

import (
	"encoding/json"
)

// SignedTransaction is what the backend receives from the frontend wallet.
// Model that is received from frontend.
type SignedTransaction struct {
	Type    string          `json:"type"`    // e.g. "name_new", "name_firstupdate", "name_update"
	From    string          `json:"from"`    // wallet address (public key hash / base58 / hex)
	Amount  uint64          `json:"amount"`  // token fee
	Payload json.RawMessage `json:"payload"` // raw payload, variant by type

	// Non-hash-related properties
	TxID      string `json:"txId"`      // hash of unsigned transaction data
	Signature string `json:"signature"` // signature of txHash
}

// NewTransactionService creates a new TransactionService instance.
func NewTransactionService(blockchain *NamecoinState) *TransactionService {
	return &TransactionService{
		TokenManager: NewBalanceManager(blockchain),
		state:        blockchain}
}

// TransactionService implements ITransactionService
type TransactionService struct {
	TokenManager *BalanceManager
	state        *NamecoinState
}

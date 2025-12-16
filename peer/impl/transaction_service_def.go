package impl

import (
	"encoding/json"

	"go.dedis.ch/cs438/types"
)

// SignedTransaction is what the backend receives from the frontend wallet.
// Model that is received from frontend.
type SignedTransaction struct {
	Type    string           `json:"type"`    // e.g. "name_new", "name_firstupdate", "name_update", "Reward"
	From    string           `json:"from"`    // wallet address (public key hash / base58 / hex)
	Amount  uint64           `json:"amount"`  // token fee
	Payload json.RawMessage  `json:"payload"` // raw payload, variant by type
	Inputs  []types.TxInput  `json:"inputs"`
	Outputs []types.TxOutput `json:"outputs"`
	Pk      string           `json:"pk"` // public key
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

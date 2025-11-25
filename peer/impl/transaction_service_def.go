package impl

import (
	"encoding/json"

	"go.dedis.ch/cs438/types"
)

type ITransactionService interface {
	// SubmitTransaction handles a signed transaction from the UI.
	// It verifies signature, nonce, balance, and payload.
	SubmitTransaction(tx types.Tx) error

	// ValidateTransaction checks tx correctness before broadcasting.
	ValidateTransaction(tx *SignedTransaction) error

	// GetTransaction returns a transaction by its txId if known.
	GetTransaction(txId string) *SignedTransaction

	// GetPendingTransactions returns all transactions waiting to be mined.
	GetPendingTransactions() []*SignedTransaction

	// DropTransaction removes a transaction from the pending pool.
	DropTransaction(txId string)

	// DropTransactions removes all transactions from the pending pool.
	DropTransactions()
}

// SignedTransaction is what the backend receives from the frontend wallet.
// Model that is received from frontend.
type SignedTransaction struct {
	Type    string          `json:"type"`    // e.g. "name_new", "name_firstupdate", "name_update"
	From    string          `json:"from"`    // wallet address (public key hash / base58 / hex)
	Fee     uint64          `json:"fee"`     // token fee
	Nonce   uint64          `json:"nonce"`   // anti-replay counter
	Payload json.RawMessage `json:"payload"` // raw payload, variant by type

	// Non-hash-related properties
	PublicKey string `json:"publicKey"` // Users Public Key
	TxID      string `json:"txId"`      // hash of unsigned transaction data
	Signature string `json:"signature"` // signature of txHash
}

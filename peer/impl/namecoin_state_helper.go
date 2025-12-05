package impl

import (
	"encoding/json"

	"go.dedis.ch/cs438/types"
)

// BuildTransactionID hashes Tx that represents ID
func BuildTransactionID(tx *types.Tx) (string, error) {
	b, err := SerializeTransaction(tx)

	if err != nil {
		return "", err
	}

	return HashHex(b), nil
}

// SerializeTransaction serializes Tx
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

//--------------------------------------------------
// Domain Expiration
// linter unused error
//// BlockContext is supplied by the chain when applying a block
//type BlockContext struct {
//	Height      uint64
//	Timestamp   int64
//	MinerPubKey string
//}
//
//// pruneExpired removes expired domains for the given height.
//func (st *NamecoinState) pruneExpired(height uint64) {
//	for name, rec := range st.Domains {
//		if rec.ExpiresAt != 0 && rec.ExpiresAt <= height {
//			delete(st.Domains, name)
//		}
//	}
//}

package impl

import (
	"fmt"

	canonicaljson "github.com/gibson042/canonicaljson-go"
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

// SerializeTransaction serializes Tx using canonical JSON (matches frontend)
func SerializeTransaction(tx *types.Tx) ([]byte, error) {
	data := map[string]interface{}{
		"type":    tx.Type,
		"from":    tx.From,
		"amount":  tx.Amount,
		"payload": tx.Payload,
	}

	b, err := canonicaljson.Marshal(data)
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

// outpointKey builds a canonical outpoint string "txid:index" from the
// referenced transaction ID (hex string or raw) and output index.
func outpointKey(txid string, index uint32) string {
	if b, err := decodeHex(txid); err == nil {
		return fmt.Sprintf("%x:%d", b, index)
	}
	return fmt.Sprintf("%x:%d", []byte(txid), index)
}

// OutpointKey exposes outpointKey for use outside the impl package (e.g., tests).
func OutpointKey(txid string, index uint32) string {
	return outpointKey(txid, index)
}

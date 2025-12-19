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
		"inputs":  tx.Inputs,
		"outputs": tx.Outputs,
	}

	b, err := canonicaljson.Marshal(data)
	if err != nil {
		return make([]byte, 0), err
	}

	return b, err
}

// SerializeTransactionForTxRoot serializes the full on-chain transaction data that
// must be committed to by the block TxRoot (including inputs/outputs and auth fields).
func SerializeTransactionForTxRoot(tx *types.Tx) ([]byte, error) {
	inputs := make([]map[string]interface{}, len(tx.Inputs))
	for i, in := range tx.Inputs {
		inputs[i] = map[string]interface{}{
			"txid":  in.TxID,
			"index": in.Index,
		}
	}

	outputs := make([]map[string]interface{}, len(tx.Outputs))
	for i, out := range tx.Outputs {
		outputs[i] = map[string]interface{}{
			"to":     out.To,
			"amount": out.Amount,
		}
	}

	data := map[string]interface{}{
		"type":      tx.Type,
		"from":      tx.From,
		"amount":    tx.Amount,
		"payload":   tx.Payload,
		"inputs":    inputs,
		"outputs":   outputs,
		"pk":        tx.Pk,
		"txid":      tx.TxID,
		"signature": tx.Signature,
	}

	b, err := canonicaljson.Marshal(data)
	if err != nil {
		return make([]byte, 0), err
	}

	return b, err
}

func equalTxInputs(a, b []types.TxInput) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].TxID != b[i].TxID || a[i].Index != b[i].Index {
			return false
		}
	}
	return true
}

func equalTxOutputs(a, b []types.TxOutput) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].To != b[i].To || a[i].Amount != b[i].Amount {
			return false
		}
	}
	return true
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

package impl

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func BuildUnsignedTxBytes(t *SignedTransaction) []byte {
	data := map[string]interface{}{
		"type":    t.Type,
		"from":    t.From,
		"fee":     t.Fee,
		"nonce":   t.Nonce,
		"payload": t.Payload,
	}

	b, _ := json.Marshal(data)
	return b
}

func Hash(bytes []byte) []byte {
	h := sha256.Sum256(bytes)
	return h[:]
}

func HashHex(b []byte) string {
	h := Hash(b)
	return hex.EncodeToString(h[:])
}

func HashString(s string) string {
	h := Hash([]byte(s))
	return hex.EncodeToString(h[:])
}

func decodeHex(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

func VerifySignature(publicKey, unsignedBytes []byte, signature string) error {
	txHash := Hash(unsignedBytes)
	sigBytes, err := decodeHex(signature)
	if err != nil {
		return fmt.Errorf("invalid signature format")
	}

	if !ed25519.Verify(publicKey, txHash, sigBytes) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

func (t *TransactionService) verifyCommand(tx *SignedTransaction) error {
	switch tx.Type {

	case NameNew{}.Name():
		p, wErr := ResolveNameCoinCommand[NameNew](tx.Type, tx.Payload)

		if wErr != nil {
			return wErr
		}

		if len(p.Commitment) == 0 {
			return fmt.Errorf("name_new commitment empty")
		}

	case NameFirstUpdate{}.Name():
		p, wErr := ResolveNameCoinCommand[NameFirstUpdate](tx.Type, tx.Payload)
		if wErr != nil {
			return wErr
		}

		// Must match earlier commitment
		storedCommit := t.state.Commitments[tx.From]
		if HashString(p.Salt+p.Domain) != storedCommit {
			return fmt.Errorf("commitment mismatch for domain %s", p.Domain)
		}

	case NameUpdate{}.Name():
		p, wErr := ResolveNameCoinCommand[NameUpdate](tx.Type, tx.Payload)
		if wErr != nil {
			return wErr
		}

		owner := t.state.DomainOwners[p.Domain]
		if owner != tx.From {
			return fmt.Errorf("cannot update domain you do not own")
		}

	default:
		return fmt.Errorf("unsupported transaction type: %s", tx.Type)
	}

	return nil
}

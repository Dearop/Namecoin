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

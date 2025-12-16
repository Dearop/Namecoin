package impl

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	canonicaljson "github.com/gibson042/canonicaljson-go"
)

func (t *SignedTransaction) SerializeTransaction() ([]byte, error) {
	data := map[string]interface{}{
		"type":    t.Type,
		"from":    t.From,
		"amount":  t.Amount,
		"payload": t.Payload,
		"inputs":  t.Inputs,
		"outputs": t.Outputs,
	}

	b, err := canonicaljson.Marshal(data)
	return b, err
}

func (t *SignedTransaction) SerializeTransactionSignature() ([]byte, error) {
	data := map[string]interface{}{
		"type":    t.Type,
		"from":    t.From,
		"amount":  t.Amount,
		"payload": t.Payload,
		"inputs":  t.Inputs,
		"outputs": t.Outputs,
	}

	b, err := canonicaljson.Marshal(data)
	return b, err
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

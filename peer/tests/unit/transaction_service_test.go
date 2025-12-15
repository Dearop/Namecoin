package unit

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func TestTransactionServiceValidateTransactionSignatureMismatch(t *testing.T) {
	service := impl.NewTransactionService(impl.NewState())
	pub, priv := mustMakeKeyPair(t)

	from := hex.EncodeToString(impl.Hash(pub))
	tx := buildSignedTransaction(t, from, priv, impl.NameNew{}.Name(), 2, impl.NameNew{Commitment: "commitment"}, pub)
	tx.Signature = "deadbeef"

	if err := service.ValidateTransaction(&tx); err == nil {
		t.Fatalf("expected validation to fail due to signature mismatch")
	}
}

func TestTransactionServiceValidateTransactionSignatureMatch(t *testing.T) {
	service := impl.NewTransactionService(impl.NewState())
	pub, priv := mustMakeKeyPair(t)

	from := hex.EncodeToString(impl.Hash(pub))
	tx := buildSignedTransaction(t, from, priv, impl.NameNew{}.Name(), 2, impl.NameNew{Commitment: "commitment"}, pub)
	if err := service.ValidateTransaction(&tx); err != nil {
		t.Fatalf("expected validation to succeed, got %v", err)
	}
}

func TestTransactionServiceValidateTransactionFirstUpdateCommitmentMismatch(t *testing.T) {
	state := impl.NewState()
	service := impl.NewTransactionService(state)
	pub, priv := mustMakeKeyPair(t)
	from := hex.EncodeToString(impl.Hash(pub))

	payload := impl.NameFirstUpdate{
		Domain: "example.test",
		Salt:   "random",
		IP:     "10.0.0.1",
	}

	tx := buildSignedTransaction(t, from, priv, impl.NameFirstUpdate{}.Name(), 3, payload, pub)
	state.UTXOMap[tx.From] = map[string]types.UTXO{
		"funds-1": {TxID: "funds-1", To: tx.From, Amount: tx.Amount},
	}

	// Set up the correct commitment for initial validation to pass
	correctCommit := impl.HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", payload.Domain, payload.Salt))
	state.SetCommitment(tx.From, correctCommit, 0, 0)

	require.NoError(t, service.ValidateTransaction(&tx))

	inputs, outputs, err := service.VerifyBalance(tx.TxID, tx.From, tx.Amount)
	require.NoError(t, err)

	txToValidate := types.Tx{
		From:    tx.From,
		Type:    tx.Type,
		Inputs:  inputs,
		Outputs: outputs,
		Amount:  tx.Amount,
		Payload: tx.Payload,
	}

	commitKey := impl.OutpointKey(inputs[0].TxID, inputs[0].Index)
	state.SetCommitment(commitKey, impl.HashString("different"), 0, 0)

	if err := service.ValidateTxCommand(&txToValidate); err == nil {
		t.Fatalf("expected validation to fail when commitment does not match")
	}
}

func TestTransactionServiceValidateTransactionNameUpdateWrongOwner(t *testing.T) {
	state := impl.NewState()
	service := impl.NewTransactionService(state)
	pub, priv := mustMakeKeyPair(t)
	from := hex.EncodeToString(impl.Hash(pub))
	payload := impl.NameUpdate{
		Domain: "foo.test",
		IP:     "192.0.2.1",
	}

	state.Domains[payload.Domain] = types.NameRecord{Owner: "other-owner"}
	tx := buildSignedTransaction(t, from, priv, impl.NameUpdate{}.Name(), 1, payload, pub)

	if err := service.ValidateTransaction(&tx); err == nil {
		t.Fatalf("expected validation to fail when sender does not own the domain")
	}
}

func TestTransactionServiceVerifyBalanceInsufficientFunds(t *testing.T) {
	service := impl.NewTransactionService(impl.NewState())

	if _, _, err := service.VerifyBalance("tx-1", "carol", 10); err == nil {
		t.Fatalf("expected insufficient funds error")
	}
}

func TestTransactionServiceVerifyBalanceReturnsInputsAndOutputs(t *testing.T) {
	state := impl.NewState()
	state.UTXOMap["carol"] = map[string]types.UTXO{
		"utxo-1": {TxID: "tx-2", To: "carol", Amount: 10},
	}
	service := impl.NewTransactionService(state)

	inputs, outputs, err := service.VerifyBalance("tx-2", "carol", 6)
	if err != nil {
		t.Fatalf("expected verify balance to succeed, got %v", err)
	}

	if len(inputs) != 1 || inputs[0].TxID != "utxo-1" {
		t.Fatalf("unexpected inputs returned: %+v", inputs)
	}

	if len(outputs) != 1 {
		t.Fatalf("expected single leftover UTXO, got %d", len(outputs))
	}

	if outputs[0].To != "carol" || outputs[0].Amount != 4 {
		t.Fatalf("unexpected leftover output %+v", outputs[0])
	}
}

func mustMakeKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}
	return pub, priv
}

func buildSignedTransaction(t *testing.T, from string, privateKey ed25519.PrivateKey,
	txType string, amount uint64, payload interface{}, publicKey ed25519.PublicKey) impl.SignedTransaction {
	t.Helper()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	tx := impl.SignedTransaction{
		Type:    txType,
		From:    from,
		Amount:  amount,
		Payload: json.RawMessage(payloadBytes),
		Pk:      hex.EncodeToString(publicKey),
	}

	unsignedBytes, err := tx.SerializeTransaction()
	if err != nil {
		t.Fatalf("failed to serialize unsigned tx: %v", err)
	}
	tx.TxID = impl.HashHex(unsignedBytes)

	sig := ed25519.Sign(privateKey, impl.Hash(unsignedBytes))
	tx.Signature = hex.EncodeToString(sig)
	return tx
}

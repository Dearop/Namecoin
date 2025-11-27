package unit

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"testing"

	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func TestTransactionServiceApplyTransaction(t *testing.T) {
	state := impl.NewState()
	service := impl.NewTransactionService(state)
	const addr = "carol"
	fundAddress(service, addr, 200)

	tx := types.Tx{
		Type:    "custom",
		From:    addr,
		Fee:     30,
		Payload: json.RawMessage("{}"),
	}

	balance, err := service.ApplyTransaction("tx-1", tx)
	if err != nil {
		t.Fatalf("ApplyTransaction returned error: %v", err)
	}
	if balance != 200 {
		t.Fatalf("expected balance before charge to be 200, got %d", balance)
	}

	stored := state.GetTransaction("tx-1")
	if stored == nil {
		t.Fatalf("expected transaction to be stored")
	}
	if stored.From != tx.From || stored.Fee != tx.Fee || stored.Type != tx.Type {
		t.Fatalf("unexpected stored transaction %+v", stored)
	}

	if updated := service.TokenManager.GetBalance(addr); updated != 170 {
		t.Fatalf("expected balance deducted to 170, got %d", updated)
	}
}

func TestTransactionServiceValidateTransactionSignatureMismatch(t *testing.T) {
	service := impl.NewTransactionService(impl.NewState())
	pub, priv := mustMakeKeyPair(t)

	tx := buildSignedTransaction(t, pub, priv, impl.NameNew{}.Name(), 2, impl.NameNew{Commitment: "commitment"})
	fundAddress(service, tx.From, 5)
	tx.Signature = "deadbeef"

	if err := service.ValidateTransaction(&tx); err == nil {
		t.Fatalf("expected validation to fail due to signature mismatch")
	}
}

func TestTransactionServiceValidateTransactionSignatureMatch(t *testing.T) {
	service := impl.NewTransactionService(impl.NewState())
	pub, priv := mustMakeKeyPair(t)

	tx := buildSignedTransaction(t, pub, priv, impl.NameNew{}.Name(), 2, impl.NameNew{Commitment: "commitment"})
	fundAddress(service, tx.From, 5)

	if err := service.ValidateTransaction(&tx); err != nil {
		t.Fatalf("expected validation to success due to signature match")
	}
}

func TestTransactionServiceValidateTransactionFirstUpdateCommitmentMismatch(t *testing.T) {
	state := impl.NewState()
	service := impl.NewTransactionService(state)
	pub, priv := mustMakeKeyPair(t)

	payload := impl.NameFirstUpdate{
		Domain: "example.test",
		Salt:   "random",
		IP:     "10.0.0.1",
	}

	tx := buildSignedTransaction(t, pub, priv, impl.NameFirstUpdate{}.Name(), 3, payload)
	fundAddress(service, tx.From, 5)

	state.Commitments[tx.From] = "different"

	if err := service.ValidateTransaction(&tx); err == nil {
		t.Fatalf("expected validation to fail when commitment mismatch occurs")
	}
}

func TestTransactionServiceValidateTransactionNameUpdateWrongOwner(t *testing.T) {
	state := impl.NewState()
	service := impl.NewTransactionService(state)
	pub, priv := mustMakeKeyPair(t)
	payload := impl.NameUpdate{
		Domain: "foo.test",
		IP:     "192.0.2.1",
	}

	tx := buildSignedTransaction(t, pub, priv, impl.NameUpdate{}.Name(), 1, payload)
	fundAddress(service, tx.From, 3)
	state.DomainOwners[payload.Domain] = "other-owner"

	if err := service.ValidateTransaction(&tx); err == nil {
		t.Fatalf("expected validation to fail when sender is not domain owner")
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

func buildSignedTransaction(t *testing.T, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey, txType string, fee uint64, payload interface{}) impl.SignedTransaction {
	t.Helper()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	tx := impl.SignedTransaction{
		Type:      txType,
		From:      hex.EncodeToString(impl.Hash(publicKey)),
		Fee:       fee,
		Payload:   json.RawMessage(payloadBytes),
		PublicKey: hex.EncodeToString(publicKey),
	}

	unsignedBytes := impl.BuildUnsignedTxBytes(&tx)
	tx.TxID = impl.HashHex(unsignedBytes)

	sig := ed25519.Sign(privateKey, impl.Hash(unsignedBytes))
	tx.Signature = hex.EncodeToString(sig)
	return tx
}

func fundAddress(service *impl.TransactionService, addr string, balance uint64) {
	service.TokenManager.SetBalance(addr, balance)
}

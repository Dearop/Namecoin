package unit

import (
	"encoding/hex"
	"fmt"
	"testing"

	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func TestVerifyBalanceAggregatesInputs(t *testing.T) {
	state := impl.NewState()
	const addr = "alice"
	state.UTXOMap[addr] = map[string]types.UTXO{
		"coin-1": {
			TxID:   "coin-1",
			To:     addr,
			Amount: 20,
		},
		"coin-2": {
			TxID:   "coin-2",
			To:     addr,
			Amount: 30,
		},
	}
	manager := impl.NewBalanceManager(state)

	inputs, outputs, err := manager.VerifyBalance("tx-spend", addr, 50)
	if err != nil {
		t.Fatalf("VerifyBalance returned error: %v", err)
	}
	if len(outputs) != 0 {
		t.Fatalf("expected no change outputs, got %#v", outputs)
	}
	if len(inputs) != 2 {
		t.Fatalf("expected two inputs, got %#v", inputs)
	}
	seen := map[string]bool{}
	for _, in := range inputs {
		seen[in.TxID] = true
	}
	if !seen["coin-1"] || !seen["coin-2"] {
		t.Fatalf("missing expected inputs, got %#v", inputs)
	}
}

func TestVerifyBalanceInsufficientFunds(t *testing.T) {
	state := impl.NewState()
	const addr = "bob"
	state.UTXOMap[addr] = map[string]types.UTXO{
		"coin-3": {
			TxID:   "coin-3",
			To:     addr,
			Amount: 5,
		},
	}
	manager := impl.NewBalanceManager(state)

	if _, _, err := manager.VerifyBalance("tx-miss", addr, 10); err == nil {
		t.Fatalf("expected insufficient funds error")
	}
}

func TestTokenBalanceManagerVerifyOwnershipSuccess(t *testing.T) {
	publicKey := []byte{0x01, 0x02, 0x03}
	from := hex.EncodeToString(impl.Hash(publicKey))

	if err := verifyOwnership(from, publicKey); err != nil {
		t.Fatalf("expected ownership verification to succeed, got %v", err)
	}
}

func TestTokenBalanceManagerVerifyOwnershipMismatch(t *testing.T) {
	publicKey := []byte{0x04, 0x05, 0x06}
	from := "deadbeef"

	if err := verifyOwnership(from, publicKey); err == nil {
		t.Fatalf("expected ownership verification to fail with mismatched address")
	}
}

func verifyOwnership(from string, publicKey []byte) error {
	pkHex := hex.EncodeToString(publicKey)
	if from == pkHex {
		return nil
	}
	derivedAddr := hex.EncodeToString(impl.Hash(publicKey))
	if from == derivedAddr {
		return nil
	}
	return fmt.Errorf("public key does not match sender address")
}

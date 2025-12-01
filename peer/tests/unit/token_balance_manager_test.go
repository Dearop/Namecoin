package unit

import (
	"encoding/hex"
	"testing"

	"go.dedis.ch/cs438/peer/impl"
)

func TestTokenBalanceManagerChargeAndGet(t *testing.T) {
	manager := impl.NewBalanceManager(nil)
	const addr = "alice"

	manager.SetBalance(addr, 100)

	balance, err := manager.ChargeAndGet(addr, 40)
	if err != nil {
		t.Fatalf("ChargeAndGet returned error: %v", err)
	}
	if balance != 100 {
		t.Fatalf("expected returned balance 100, got %d", balance)
	}
	if got := manager.VerifyBalance(addr); got != 60 {
		t.Fatalf("expected balance to be deducted to 60, got %d", got)
	}
}

func TestTokenBalanceManagerChargeAndGetInsufficientFunds(t *testing.T) {
	manager := impl.NewBalanceManager(nil)
	const addr = "bob"

	manager.SetBalance(addr, 5)

	if _, err := manager.ChargeAndGet(addr, 6); err == nil {
		t.Fatalf("expected insufficient funds error")
	}
}

func TestTokenBalanceManagerVerifyOwnershipSuccess(t *testing.T) {
	manager := impl.NewBalanceManager(nil)

	publicKey := []byte{0x01, 0x02, 0x03}
	from := hex.EncodeToString(impl.Hash(publicKey))

	if err := manager.VerifyOwnership(from, publicKey); err != nil {
		t.Fatalf("expected ownership verification to succeed, got %v", err)
	}
}

func TestTokenBalanceManagerVerifyOwnershipMismatch(t *testing.T) {
	manager := impl.NewBalanceManager(nil)

	publicKey := []byte{0x04, 0x05, 0x06}
	from := "deadbeef"

	if err := manager.VerifyOwnership(from, publicKey); err == nil {
		t.Fatalf("expected ownership verification to fail with mismatched address")
	}
}

package impl

import (
	"encoding/hex"
	"fmt"
	"sync"
)

// TokenWalletManager implements ITokenWalletManager
type TokenWalletManager struct {
	// todo: replace balances with actual blockchain storage.
	// todo: calculating balances should be done Bitcoin (UTXO) alike or Ethereum (state) alike?
	balances map[string]uint64
	mu       sync.RWMutex
}

func NewTokenWalletManager() *TokenWalletManager {
	// todo: replace balances with actual blockchain storage.
	return &TokenWalletManager{balances: make(map[string]uint64)}
}

// GetBalance returns the balance of the given address
func (t *TokenWalletManager) GetBalance(from string) uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.balances[from]
}

// ChargeAndGet deducts the given amount from the balance of the given address
func (t *TokenWalletManager) ChargeAndGet(from string, amount uint64) (uint64, error) {
	t.mu.Lock()
	balance := t.GetBalance(from)
	defer t.mu.Unlock()
	if balance < amount {
		return 0, fmt.Errorf("insufficient funds")
	}

	t.balances[from] = balance - amount

	return balance, nil
}

// VerifyOwnership verifies that the given public key matches the given address
func (t *TokenWalletManager) VerifyOwnership(from string, publicKey []byte) error {
	derivedAddr := hex.EncodeToString(Hash(publicKey))
	if derivedAddr != from {
		return fmt.Errorf("public key does not match sender address")
	}

	return nil
}

// SetBalance sets the balance of the given address
func (t *TokenWalletManager) SetBalance(from string, amount uint64) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.balances[from] = amount

	return t.balances[from]
}

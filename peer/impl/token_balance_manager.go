package impl

import (
	"encoding/hex"
	"fmt"

	"go.dedis.ch/cs438/types"
)

// BalanceManager implements ITokenWalletManager
type BalanceManager struct {
	state *NamecoinState
}

func NewBalanceManager(state *NamecoinState) *BalanceManager {
	return &BalanceManager{state: state}
}

// VerifyBalance returns the canonical inputs/outputs for spending `amount` from `from`.
func (t *BalanceManager) VerifyBalance(txID, from string, amount uint64) ([]types.TxInput, []types.TxOutput, error) {
	inp, out, err := t.state.DeterministicSpendPlan(from, amount)
	if err != nil {
		return []types.TxInput{}, []types.TxOutput{}, err
	}

	return inp, out, nil
}

// VerifyOwnership verifies that the given public key matches the given address
func (t *BalanceManager) VerifyOwnership(from string, publicKey []byte) error {
	derivedAddr := hex.EncodeToString(Hash(publicKey))
	if derivedAddr != from {
		return fmt.Errorf("public key does not match sender address")
	}

	return nil
}

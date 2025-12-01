package impl

import (
	"encoding/hex"
	"fmt"

	"go.dedis.ch/cs438/types"
)

// TokenManager implements ITokenWalletManager
type TokenManager struct {
	state *NamecoinState
}

func NewTokenWalletManager(state *NamecoinState) *TokenManager {
	return &TokenManager{state: state}
}

// VerifyBalance returns the balance of the given address
func (t *TokenManager) VerifyBalance(txID, from string, amount uint64) ([]types.TxInput, types.TxOutput, error) {
	inp, out, err := t.state.GetUTXOsToBurn(txID, from, amount)
	if err != nil {
		return make([]types.TxInput, 0), types.TxOutput{}, err
	}

	inputs := make([]types.TxInput, 0)
	for _, value := range inp {
		inputs = append(inputs, types.TxInput{
			TxID: value,
		})
	}

	output := types.TxOutput{
		To:     out.To,
		Amount: out.Amount,
	}

	return inputs, output, nil
}

// VerifyOwnership verifies that the given public key matches the given address
func (t *TokenManager) VerifyOwnership(from string, publicKey []byte) error {
	derivedAddr := hex.EncodeToString(Hash(publicKey))
	if derivedAddr != from {
		return fmt.Errorf("public key does not match sender address")
	}

	return nil
}

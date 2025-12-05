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

// VerifyBalance returns the balance of the given address
func (t *BalanceManager) VerifyBalance(txID, from string, amount uint64) ([]types.TxInput, []types.TxOutput, error) {
	inp, out, err := t.state.GetUTXOsToBurn(txID, from, amount)
	if err != nil {
		return make([]types.TxInput, 0), make([]types.TxOutput, 0), err
	}

	inputs := make([]types.TxInput, 0)
	for _, value := range inp {
		inputs = append(inputs, types.TxInput{
			TxID: value,
		})
	}

	// Max 1 output UTXO
	outputs := make([]types.TxOutput, 0, 1)
	for _, value := range out {
		output := types.TxOutput{
			To:     value.To,
			Amount: value.Amount,
		}
		outputs = append(outputs, output)
	}

	return inputs, outputs, nil
}

// VerifyOwnership verifies that the given public key matches the given address
func (t *BalanceManager) VerifyOwnership(from string, publicKey []byte) error {
	derivedAddr := hex.EncodeToString(Hash(publicKey))
	if derivedAddr != from {
		return fmt.Errorf("public key does not match sender address")
	}

	return nil
}

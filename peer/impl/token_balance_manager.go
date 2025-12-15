package impl

import (
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

// VerifyOwnership ensures the provided address matches the hash of the public key.
func (t *BalanceManager) VerifyOwnership(from string, pubKeyBytes []byte) error {
	if from == "" {
		return fmt.Errorf("missing from address")
	}
	if len(pubKeyBytes) == 0 {
		return fmt.Errorf("missing public key bytes")
	}
	expected := HashHex(pubKeyBytes)
	if from != expected {
		return fmt.Errorf("from does not match public key hash")
	}
	return nil
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
			// Single-output transactions map to index 0 for now.
			Index: 0,
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

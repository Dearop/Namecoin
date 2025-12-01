package impl

import (
	"fmt"

	"go.dedis.ch/cs438/types"
)

func (t *TransactionService) ValidateTransaction(tx *SignedTransaction) error {
	// 1. Decode public key
	pubKeyBytes, err := decodeHex(tx.From)
	if err != nil {
		return fmt.Errorf("invalid public key format")
	}

	// 2. Recompute TxID from unsigned transaction
	unsignedBytes := tx.SerializeTransaction()
	computedTxID := HashHex(unsignedBytes)

	if computedTxID != tx.TxID {
		return fmt.Errorf("txId mismatch: expected %s, got %s", computedTxID, tx.TxID)
	}

	// 3. Verify signature
	err = VerifySignature(pubKeyBytes, unsignedBytes, tx.Signature)
	if err != nil {
		return err
	}

	// 4. Validate payload based on transaction type
	err = t.state.ValidateCommand(tx)
	if err != nil {
		return err
	}

	return nil
}

func (t *TransactionService) VerifyBalance(
	txID,
	from string,
	amount uint64) ([]types.TxInput, []types.TxOutput, error) {
	// 5. Check user balance use BalanceManager
	// generating UTXOs to burn and one for leftovers
	inputs, output, err := t.TokenManager.VerifyBalance(txID, from, amount)
	if err != nil {
		return make([]types.TxInput, 0), make([]types.TxOutput, 0), err
	}

	return inputs, output, nil
}

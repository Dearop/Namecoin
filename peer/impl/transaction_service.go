package impl

import (
	"fmt"

	"go.dedis.ch/cs438/types"
)

func (t *TransactionService) ValidateTransaction(tx *SignedTransaction) error {
	// 1. Decode public key
	pubKeyBytes, err := decodeHex(tx.Pk)
	if err != nil {
		return fmt.Errorf("invalid public key format")
	}

	// 1b. Enforce address ownership: From must be the hash of the public key.
	// address := hex(sha256(pkBytes)).
	if err := t.TokenManager.VerifyOwnership(tx.From, pubKeyBytes); err != nil {
		return err
	}

	// 2. Derive canonical inputs/outputs and ensure they match what was signed.
	canonicalInputs, canonicalOutputs, err := t.state.DeterministicSpendPlan(tx.From, tx.Amount)
	if err != nil {
		return err
	}
	if !equalTxInputs(tx.Inputs, canonicalInputs) {
		return fmt.Errorf("inputs not canonical for (from=%s, amount=%d)", tx.From, tx.Amount)
	}
	if !equalTxOutputs(tx.Outputs, canonicalOutputs) {
		return fmt.Errorf("outputs not canonical for (from=%s, amount=%d)", tx.From, tx.Amount)
	}

	// 3. Recompute TxID from unsigned transaction (which now includes inputs/outputs).
	unsignedBytes, err := tx.SerializeTransaction()
	if err != nil {
		return fmt.Errorf("failed to serialize transaction: %w", err)
	}
	computedTxID := HashHex(unsignedBytes)

	if computedTxID != tx.TxID {
		return fmt.Errorf("txId mismatch: expected %s, got %s", computedTxID, tx.TxID)
	}

	// 4. Verify signature over same preimage
	allUnsignedBytes, err := tx.SerializeTransactionSignature()
	if err != nil {
		return fmt.Errorf("failed to serialize signature preimage: %w", err)
	}
	if err := VerifySignature(pubKeyBytes, allUnsignedBytes, tx.Signature); err != nil {
		return err
	}

	// 5. Validate payload based on transaction type (signed transaction view)
	return t.state.ValidateCommand(tx)
}

func (t *TransactionService) ValidateTxCommand(tx *types.Tx) error {
	return t.state.ValidateCommandWithInputs(tx)
}

func (t *TransactionService) VerifyBalance(
	txID,
	from string,
	amount uint64) ([]types.TxInput, []types.TxOutput, error) {
	// Canonical spend plan
	return t.TokenManager.VerifyBalance(txID, from, amount)
}

func (t *TransactionService) GetSpendPlan(from string, amount uint64) ([]types.TxInput, []types.TxOutput, error) {
	return t.state.DeterministicSpendPlan(from, amount)
}

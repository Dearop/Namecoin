package impl

import (
	"fmt"

	"go.dedis.ch/cs438/types"
)

// NewTransactionService creates a new TransactionService instance.
func NewTransactionService(blockchain *NamecoinState) *TransactionService {
	return &TransactionService{
		TokenManager: NewTokenWalletManager(blockchain),
		state:        blockchain}
}

// TransactionService implements ITransactionService
type TransactionService struct {
	TokenManager *TokenManager
	state        *NamecoinState
}

func (t *TransactionService) ValidateTransaction(tx *SignedTransaction) error {
	// 1. Decode public key
	pubKeyBytes, err := decodeHex(tx.From)
	if err != nil {
		return fmt.Errorf("invalid public key format")
	}

	// 2. Recompute TxID from unsigned transaction
	unsignedBytes := BuildUnsignedTxBytes(tx)
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
	err = t.verifyCommand(tx)
	if err != nil {
		return err
	}

	return nil
}

func (t *TransactionService) VerifyBalance(txID, from string, amount uint64) ([]types.TxInput, types.TxOutput, error) {
	// 5. Check user balance use TokenManager
	// generating UTXOs to burn and one for leftovers
	inputs, output, err := t.TokenManager.VerifyBalance(txID, from, amount)
	if err != nil {
		return make([]types.TxInput, 0), types.TxOutput{}, err
	}

	return inputs, output, nil
}

// verifyCommand verifies the payload of a transaction based on its type
func (t *TransactionService) verifyCommand(tx *SignedTransaction) error {
	switch tx.Type {

	case NameNew{}.Name():
		p, wErr := ResolveNameCoinCommand[NameNew](tx.Type, tx.Payload)

		if wErr != nil {
			return wErr
		}

		if len(p.Commitment) == 0 {
			return fmt.Errorf("name_new commitment empty")
		}

	case NameFirstUpdate{}.Name():
		p, wErr := ResolveNameCoinCommand[NameFirstUpdate](tx.Type, tx.Payload)
		if wErr != nil {
			return wErr
		}

		// Must match earlier commitment
		storedCommit := t.state.GetCommitment(tx.From)

		//todo: Update, to avoid collisions.
		if HashString(p.Salt+p.Domain) != storedCommit {
			return fmt.Errorf("commitment mismatch for domain %s", p.Domain)
		}

	case NameUpdate{}.Name():
		p, wErr := ResolveNameCoinCommand[NameUpdate](tx.Type, tx.Payload)
		if wErr != nil {
			return wErr
		}

		owner := t.state.GetDomainOwner(p.Domain)
		if owner != tx.From {
			return fmt.Errorf("cannot update domain you do not own")
		}

	default:
		return fmt.Errorf("unsupported transaction type: %s", tx.Type)
	}

	return nil
}

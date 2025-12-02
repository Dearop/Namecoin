package impl

import (
	"reflect"

	"go.dedis.ch/cs438/types"
)

// Reward is a transaction type for rewarding a MinerPubKey. No payload here.
type Reward struct {
}

// Name implements NamecoinCommand
func (r Reward) Name() string {
	return reflect.TypeOf(&Reward{}).Elem().Name()
}

// Validate is a no-op for this transaction type
func (r Reward) Validate(_ *NamecoinState, _ *SignedTransaction) error {
	return nil
}

// ProcessState is a no-op for this transaction type
func (r Reward) ProcessState(_ *NamecoinState, _ *types.Tx) error {
	return nil
}

func (r Reward) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	// On Reward - always 1 UTXO
	// todo: when block mined, output is not actually created, Should it be or we can create it here based on the data?

	utxo := types.UTXO{
		TxID:   txID,
		To:     tx.Outputs[0].To,
		Amount: tx.Outputs[0].Amount,
	}

	// save UTXO that rewards miner
	err := st.AppendUTXO(utxo)

	return err
}

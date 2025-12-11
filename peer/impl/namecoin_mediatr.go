package impl

import (
	"encoding/json"
	"fmt"

	"go.dedis.ch/cs438/types"
)

var (
	RewardCommandName          = Reward{}.Name()
	NameNewCommandName         = NameNew{}.Name()
	NameFirstUpdateCommandName = NameFirstUpdate{}.Name()
	NameUpdateCommandName      = NameUpdate{}.Name()
)

type NamecoinCommand interface {
	Name() string
	ProcessState(st *NamecoinState, tx *types.Tx) error
	ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error
	Validate(st *NamecoinState, tx *SignedTransaction) error
}

type CommandType interface {
	NameNew | NameFirstUpdate | NameUpdate | Reward
}

func ResolveCommand(command string, payload json.RawMessage) (NamecoinCommand, error) {
	switch command {
	case NameNewCommandName:
		var cmd NameNew
		return cmd, json.Unmarshal(payload, &cmd)
	case NameFirstUpdateCommandName:
		var cmd NameFirstUpdate
		return cmd, json.Unmarshal(payload, &cmd)
	case NameUpdateCommandName:
		var cmd NameUpdate
		return cmd, json.Unmarshal(payload, &cmd)
	case RewardCommandName:
		var cmd Reward
		return cmd, nil
	}
	return nil, fmt.Errorf("unknown command %s", command)
}

func ProcessTxStateGeneric(st *NamecoinState, txID string, tx *types.Tx) error {
	txIDs := make([]string, len(tx.Inputs))
	for i, value := range tx.Inputs {
		txIDs[i] = value.TxID
	}

	var err error
	if len(txIDs) > 0 {
		err = st.BurnUTXO(tx.From, txIDs)
		if err != nil {
			return err
		}
	}

	// 1 or 0 UTXOs
	for _, value := range tx.Outputs {
		utxo := types.UTXO{
			TxID:   txID,
			To:     value.To,
			Amount: value.Amount,
		}

		err = st.AppendUTXO(utxo)
		if err != nil {
			return err
		}
	}

	return nil
}

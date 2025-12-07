package impl

import (
	"fmt"
	"reflect"

	"go.dedis.ch/cs438/types"
)

type NameNew struct {
	Commitment string `json:"commitment"` // H(salt + domain)
}

// Name implements NamecoinCommand
func (n NameNew) Name() string {
	return reflect.TypeOf(&NameNew{}).Elem().Name()
}

func (n NameNew) Validate(_ *NamecoinState, _ *SignedTransaction) error {
	if len(n.Commitment) == 0 {
		return fmt.Errorf("name_new commitment empty")
	}

	return nil
}

func (n NameNew) ProcessState(st *NamecoinState, tx *types.Tx) error {
	return nil
}

func (n NameNew) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	if err := ProcessTxStateGeneric(st, txID, tx); err != nil {
		return err
	}

	key := outpointKey(txID, 0)
	st.SetCommitment(key, n.Commitment)

	return nil
}

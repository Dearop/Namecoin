package impl

import (
	"fmt"
	"reflect"

	"go.dedis.ch/cs438/types"
)

type NameNew struct {
	Commitment string `json:"commitment"` // H(salt + domain)
	TTL        uint64 `json:"ttl,omitempty"`
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
	// we don't reveal the name on the initial domain creation, look at the project description
	st.SetCommitment(tx.From, n.Commitment)
	// Store TTL preference keyed by commitment; applied during firstupdate
	st.SetCommitmentTTL(n.Commitment, n.TTL)

	return nil
}

func (n NameNew) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	return ProcessTxStateGeneric(st, txID, tx)
}

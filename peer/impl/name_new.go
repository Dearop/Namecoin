package impl

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"go.dedis.ch/cs438/types"
	"reflect"
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

// ValidateWithInputs enforces that name_new spends at least one UTXO (no free mints).
func (n NameNew) ValidateWithInputs(_ *NamecoinState, tx *types.Tx) error {
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("name_new requires at least one input UTXO")
	}
	return nil
}

func (n NameNew) ApplyState(st *NamecoinState, _ *types.Tx) error {
	// Store TTL preference keyed by commitment; applied during firstupdate
	if n.TTL != 0 {
		st.SetCommitmentTTL(n.Commitment, n.TTL)
	}
	return nil
}

func (n NameNew) ApplyUTXO(st *NamecoinState, txID string, tx *types.Tx) error {
	if err := ApplyUTXOGeneric(st, txID, tx); err != nil {
		return err
	}

	key := OutpointKey(txID, 0)
	log.Info().Msgf("NameNew ApplyUTXO: setting commitment for key %s to %s", key, n.Commitment)
	st.SetCommitment(key, n.Commitment)

	return nil
}

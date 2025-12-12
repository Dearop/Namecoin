package impl

import (
	"fmt"
	"reflect"
	"github.com/rs/zerolog/log"
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

// ValidateWithInputs enforces that name_new spends at least one UTXO (no free mints).
func (n NameNew) ValidateWithInputs(_ *NamecoinState, tx *types.Tx) error {
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("name_new requires at least one input UTXO")
	}
	return nil
}

func (n NameNew) ProcessState(st *NamecoinState, tx *types.Tx) error {
	// we don't reveal the name on the initial domain creation, look at the project description
	st.SetCommitment(tx.From, n.Commitment)

	// Convert 0 to default TTL before storing
    ttlToStore := n.TTL
    if ttlToStore == 0 {
        ttlToStore = DefaultDomainTTLBlocks  // or DefaultDomainTTLBlocks
    }
	// Store TTL preference keyed by commitment; applied during firstupdate
	st.SetCommitmentTTL(n.Commitment, ttlToStore)

	return nil
}

func (n NameNew) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	if err := ProcessTxStateGeneric(st, txID, tx); err != nil {
		return err
	}

	key := OutpointKey(txID, 0)
	//log that we are setting the commitment
	log.Info().Msgf("NameNew ProcessTxState: setting commitment for key %s to %s", key, n.Commitment)
	st.SetCommitment(key, n.Commitment)

	return nil
}

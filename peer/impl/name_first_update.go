package impl

import (
	"fmt"
	"reflect"

	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

type NameFirstUpdate struct {
	Domain string `json:"domain"` // The real domain name being registered
	Salt   string `json:"salt"`   // Must match the original commitment
	IP     string `json:"ip"`     // IP address the user wants to bind
}

// Name implements NamecoinCommand
func (n NameFirstUpdate) Name() string {
	return reflect.TypeOf(&NameFirstUpdate{}).Elem().Name()
}

func (n NameFirstUpdate) Validate(_ *NamecoinState, _ *SignedTransaction) error {
	// Basic payload sanity; full commit–reveal checks happen in ValidateWithInputs.
	if n.Domain == "" || n.Salt == "" || n.IP == "" {
		return fmt.Errorf("invalid name_firstupdate payload")
	}
	return nil
}

func (n NameFirstUpdate) ProcessState(st *NamecoinState, tx *types.Tx) error {
	key, err := n.resolveCommitment(st, tx)
	if err != nil {
		return err
	}

	// Reject if already claimed/active.
	if st.IsDomainExists(n.Domain) || st.IsDomainClaimed(n.Domain) {
		return xerrors.New("Domain already exists")
	}

	// Commitment matched, proceed to reveal and consume it.
	st.SetDomain(types.NameRecord{
		Owner:     tx.From,
		IP:        n.IP,
		Domain:    n.Domain,
		Salt:      n.Salt,
		ExpiresAt: 0, // todo: Add expiration
	})
	st.DeleteCommitment(key)

	return nil
}

func (n NameFirstUpdate) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	return ProcessTxStateGeneric(st, txID, tx)
}

// ValidateWithInputs performs commitment checks requiring full tx inputs.
func (n NameFirstUpdate) ValidateWithInputs(st *NamecoinState, tx *types.Tx) error {
	_, err := n.resolveCommitment(st, tx)
	return err
}

func (n NameFirstUpdate) resolveCommitment(st *NamecoinState, tx *types.Tx) (string, error) {
	if len(tx.Inputs) == 0 {
		return "", fmt.Errorf("name_firstupdate requires at least one input")
	}
	in := tx.Inputs[0]
	key := outpointKey(in.TxID, in.Index)
	commit, ok := st.GetCommitment(key)
	if !ok {
		return "", fmt.Errorf("no matching name_new commitment")
	}
	if HashString(n.Domain+n.Salt) != commit {
		return "", fmt.Errorf("commitment mismatch for domain %s", n.Domain)
	}
	return key, nil
}

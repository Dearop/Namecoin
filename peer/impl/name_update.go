package impl

import (
	"fmt"
	"reflect"
	"strings"

	"go.dedis.ch/cs438/types"
)

type NameUpdate struct {
	Domain string `json:"domain"` // Registered domain
	IP     string `json:"ip"`     // Handle IP address
}

// Name implements NamecoinCommand
func (n NameUpdate) Name() string {
	return reflect.TypeOf(&NameUpdate{}).Elem().Name()
}

func (n NameUpdate) Validate(st *NamecoinState, tx *SignedTransaction) error {
	owner := st.GetDomainOwner(n.Domain)
	if owner != tx.From {
		return fmt.Errorf("cannot update domain you do not own")
	}

	return nil
}

func (n NameUpdate) ProcessState(st *NamecoinState, _ *types.Tx) error {
	rec, ok := st.NameLookup(n.Domain)
	if !ok {
		return fmt.Errorf("updating non-existent domain %s", n.Domain)
	}

	// update only if the value is set. If value equals "", no updates have been made
	if len(strings.TrimSpace(n.Domain)) != 0 {
		rec.Domain = n.Domain
	}
	if len(strings.TrimSpace(n.IP)) != 0 {
		rec.IP = n.IP
	}

	st.SetDomain(rec)
	// todo: refresh domain lifetime

	return nil
}

func (n NameUpdate) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	return ProcessTxStateGeneric(st, txID, tx)
}

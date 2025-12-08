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

func (n NameFirstUpdate) Validate(st *NamecoinState, tx *SignedTransaction) error {
	// Must match earlier commitment
	storedCommit := st.GetCommitment(tx.From)

	//todo: Update, to avoid collisions.
	if HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", n.Domain, n.Salt)) != storedCommit {
		return fmt.Errorf("commitment mismatch for domain %s with the following values %s : %s", n.Domain, HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", n.Domain, n.Salt)), storedCommit)
	}

	return nil
}

func (n NameFirstUpdate) ProcessState(st *NamecoinState, tx *types.Tx) error {
	if st.IsDomainExists(n.Domain) {
		return xerrors.New("Domain already exists")
	}

	st.SetDomain(types.NameRecord{
		Owner:     tx.From,
		IP:        n.IP,
		Domain:    n.Domain,
		Salt:      n.Salt,
		ExpiresAt: 0, // todo: Add expiration
	})

	return nil
}

func (n NameFirstUpdate) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	return ProcessTxStateGeneric(st, txID, tx)
}

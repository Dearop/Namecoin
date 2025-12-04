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
	TTL    uint64 `json:"ttl,omitempty"`
}

// Name implements NamecoinCommand
func (n NameFirstUpdate) Name() string {
	return reflect.TypeOf(&NameFirstUpdate{}).Elem().Name()
}

func (n NameFirstUpdate) Validate(st *NamecoinState, tx *SignedTransaction) error {
	// Must match earlier commitment
	storedCommit := st.GetCommitment(tx.From)
	//todo: Update, to avoid collisions.
	if HashString(n.Domain+n.Salt) != storedCommit {
		return fmt.Errorf("commitment mismatch for domain %s", n.Domain)
	}

	if rec, ok := st.getDomain(n.Domain); ok && !st.isExpired(rec, st.CurrentHeight()) {
		return xerrors.New("domain already exists")
	}
	return nil
}

func (n NameFirstUpdate) ProcessState(st *NamecoinState, tx *types.Tx) error {
	// remove expired if present
	if rec, ok := st.getDomain(n.Domain); ok && st.isExpired(rec, st.CurrentHeight()) {
		st.mu.Lock()
		delete(st.Domains, n.Domain)
		st.removeFromExpiryLocked(n.Domain, rec.ExpiresAt)
		st.mu.Unlock()
	} else if ok {
		return xerrors.New("domain already exists")
	}

	st.SetDomain(types.NameRecord{
		Owner:     tx.From,
		IP:        n.IP,
		Domain:    n.Domain,
		Salt:      n.Salt,
		ExpiresAt: st.CurrentHeight() + st.effectiveTTL(resolveTTLPreference(n.TTL, st.GetCommitment(tx.From), st)),
	})

	return nil
}

func resolveTTLPreference(txTTL uint64, commitment string, st *NamecoinState) uint64 {
	if txTTL != 0 {
		return txTTL
	}
	if commitment != "" {
		if pref := st.GetCommitmentTTL(commitment); pref != 0 {
			return pref
		}
	}
	return 0
}

func (n NameFirstUpdate) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	return ProcessTxStateGeneric(st, txID, tx)
}

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
	TTL    uint64 `json:"ttl"`    // TTL override; if zero, state default is used
}

// Name implements NamecoinCommand
func (n NameUpdate) Name() string {
	return reflect.TypeOf(&NameUpdate{}).Elem().Name()
}

func (n NameUpdate) Validate(st *NamecoinState, tx *SignedTransaction) error {
	rec, ok := st.getDomain(n.Domain)
	if !ok || st.isExpired(rec, st.CurrentHeight()) {
		return fmt.Errorf("cannot update non-existent or expired domain")
	}
	if rec.Owner != tx.From {
		// TODO : Bubble error up to caller to send message to frontend.
		return fmt.Errorf("cannot update domain you do not own")
	}

	return nil
}

func (n NameUpdate) ProcessState(st *NamecoinState, tx *types.Tx) error {
	// rec is copy, changing it without a lock, then updating with lock.
	rec, ok := st.getDomain(n.Domain)
	if !ok {
		return fmt.Errorf("updating non-existent domain %s", n.Domain)
	}
	// TODO : Bubble errors up to caller to send message to frontend.
	if st.isExpired(rec, st.CurrentHeight()) {
		return fmt.Errorf("updating expired domain %s", n.Domain)
	}
	if tx != nil && rec.Owner != tx.From {
		return fmt.Errorf("update: not owner")
	}

	// update only if the value is set. If value equals "", no updates have been made
	if len(strings.TrimSpace(n.Domain)) != 0 {
		rec.Domain = n.Domain
	}
	if len(strings.TrimSpace(n.IP)) != 0 {
		rec.IP = n.IP
	}
	rec.ExpiresAt = st.CurrentHeight() + st.effectiveTTL(n.TTL)

	st.SetDomain(rec)

	return nil
}

func (n NameUpdate) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	return ProcessTxStateGeneric(st, txID, tx)
}

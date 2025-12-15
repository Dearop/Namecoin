package impl

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/rs/zerolog/log"
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

func (n NameUpdate) ApplyState(st *NamecoinState, _ *types.Tx) error {
	// rec is copy, changing it without a lock, then updating with lock.
	rec, ok := st.NameLookup(n.Domain)
	if !ok {
		return fmt.Errorf("updating non-existent domain %s", n.Domain)
	}
	if st.isExpired(rec, st.CurrentHeight()) {
		return fmt.Errorf("updating expired domain %s", n.Domain)
	}

	// update only if the value is set. If value equals "", no updates have been made
	if len(strings.TrimSpace(n.Domain)) != 0 {
		rec.Domain = n.Domain
	}
	if len(strings.TrimSpace(n.IP)) != 0 {
		rec.IP = n.IP
	}

	oldExpiresAt := rec.ExpiresAt
	effectiveTTLValue := st.effectiveTTL(n.TTL)
	newExpiresAt := st.CurrentHeight() + effectiveTTLValue
	rec.ExpiresAt = newExpiresAt

	log.Info().Str("domain", n.Domain).Uint64("old_expires_at", oldExpiresAt).Uint64("ttl_blocks",
		effectiveTTLValue).Uint64("new_expires_at", newExpiresAt).Msg("Domain TTL updated")

	st.SetDomain(rec)

	return nil
}

func (n NameUpdate) ApplyUTXO(st *NamecoinState, txID string, tx *types.Tx) error {
	return ApplyUTXOGeneric(st, txID, tx)
}

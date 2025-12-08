package impl

import (
	"fmt"
	"reflect"

	"github.com/rs/zerolog/log"
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
	if HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", n.Domain, n.Salt)) != storedCommit {
		return fmt.Errorf("commitment mismatch for domain %s with the following values %s : %s",
		 n.Domain, HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", n.Domain, n.Salt)), storedCommit)
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
		// TODO: Bubble error up to caller to send message to frontend.
		return xerrors.New("domain already exists")
	}

	effectiveTTLValue := st.effectiveTTL(resolveTTLPreference(n.TTL, st.GetCommitment(tx.From), st))
	newExpiresAt := st.CurrentHeight() + effectiveTTLValue
	
	log.Info().Str("domain", n.Domain).Uint64("ttl_blocks", effectiveTTLValue).Uint64("new_expires_at", newExpiresAt).Msg("Domain registered with TTL")
	
	st.SetDomain(types.NameRecord{
		Owner:     tx.From,
		IP:        n.IP,
		Domain:    n.Domain,
		Salt:      n.Salt,
		ExpiresAt: newExpiresAt,
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

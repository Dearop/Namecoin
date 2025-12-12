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
	if n.Domain == "" || n.Salt == "" || n.IP == "" {
		return fmt.Errorf("invalid name_firstupdate payload")
	}

	if rec, ok := st.getDomain(n.Domain); ok && !st.isExpired(rec, st.CurrentHeight()) {
		return xerrors.New("domain already exists")
	}
	return nil
}

func (n NameFirstUpdate) ApplyState(st *NamecoinState, tx *types.Tx) error {
	// remove expired if present
	if rec, ok := st.getDomain(n.Domain); ok && st.isExpired(rec, st.CurrentHeight()) {
		st.mu.Lock()
		delete(st.Domains, n.Domain)
		st.removeFromExpiryLocked(n.Domain, rec.ExpiresAt)
		st.mu.Unlock()
	}

	//generate txID of the name_new transaction, i.e. with type name_new and commitment

	commit, key, err := n.resolveCommitment(st, tx)
	if err != nil {
		return err
	}

	// Reject if already claimed/active.
	if st.IsDomainExists(n.Domain) || st.IsDomainClaimed(n.Domain) {
		return xerrors.New("Domain already exists")
	}

	effectiveTTLValue := st.effectiveTTL(resolveTTLPreference(n.TTL, commit, st))
	newExpiresAt := st.CurrentHeight() + effectiveTTLValue

	log.Info().Str("domain", n.Domain).Uint64("ttl_blocks", effectiveTTLValue).Uint64("new_expires_at",
		newExpiresAt).Msg("Domain registered with TTL")

	st.SetDomain(types.NameRecord{
		Owner:     tx.From,
		IP:        n.IP,
		Domain:    n.Domain,
		Salt:      n.Salt,
		ExpiresAt: newExpiresAt,
	})
	st.DeleteCommitment(key)

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

func (n NameFirstUpdate) ApplyUTXO(st *NamecoinState, txID string, tx *types.Tx) error {
	return ApplyUTXOGeneric(st, txID, tx)
}

// ValidateWithInputs performs commitment checks requiring full tx inputs.
func (n NameFirstUpdate) ValidateWithInputs(st *NamecoinState, tx *types.Tx) error {
	_, _, err := n.resolveCommitment(st, tx)
	if rec, ok := st.getDomain(n.Domain); ok && !st.isExpired(rec, st.CurrentHeight()) {
		return xerrors.New("domain already exists")
	}
	return err
}

func (n NameFirstUpdate) resolveCommitment(st *NamecoinState, tx *types.Tx) (string, string, error) {
	if len(tx.Inputs) == 0 {
		return "", "", fmt.Errorf("name_firstupdate requires at least one input")
	}

	commitment := HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", n.Domain, n.Salt))
	// Use the referenced name_new outpoint (txid:0 in the single-output MVP).
	in := tx.Inputs[0]
	key := OutpointKey(in.TxID, 0)

	storedCommit, ok := st.GetCommitment(key)
	if !ok {
		return "", "", fmt.Errorf("no matching name_new commitment with key %s", key)
	}
	if commitment != storedCommit {
		return "", "", fmt.Errorf("commitment mismatch for domain %s", n.Domain)
	}
	return storedCommit, key, nil
}

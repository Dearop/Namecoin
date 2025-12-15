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
	TxID   string `json:"txid"` // The txid of the corresponding name_new transaction
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

	_, rec, key, err := n.resolveCommitment(st, tx)
	if err != nil {
		return err
	}

	// Reject if already claimed/active.
	if st.IsDomainExists(n.Domain) || st.IsDomainClaimed(n.Domain) {
		return xerrors.New("Domain already exists")
	}

	effectiveTTLValue := st.effectiveTTL(resolveTTLPreference(n.TTL, rec.TTL))
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

func resolveTTLPreference(txTTL uint64, storedTTL uint64) uint64 {
	if txTTL != 0 {
		return txTTL
	}
	return storedTTL
}

func (n NameFirstUpdate) ApplyUTXO(st *NamecoinState, txID string, tx *types.Tx) error {
	return ApplyUTXOGeneric(st, txID, tx)
}

// ValidateWithInputs performs commitment checks requiring full tx inputs.
func (n NameFirstUpdate) ValidateWithInputs(st *NamecoinState, tx *types.Tx) error {
	_, rec, _, err := n.resolveCommitment(st, tx)
	if err != nil {
		return err
	}
	// Reject if the commitment is too old.
	if rec.Height > 0 && st.CurrentHeight() > rec.Height {
		if st.CurrentHeight()-rec.Height > MaxFirstUpdateDepth {
			return fmt.Errorf("commitment too old for firstupdate")
		}
	}
	if rec, ok := st.getDomain(n.Domain); ok && !st.isExpired(rec, st.CurrentHeight()) {
		return xerrors.New("domain already exists")
	}
	return nil
}

func (n NameFirstUpdate) resolveCommitment(st *NamecoinState, tx *types.Tx) (string, CommitmentRecord, string, error) {
	commitment := HashString(fmt.Sprintf("DOMAIN_HASH_v1:%s:%s", n.Domain, n.Salt))

	commitTxID := n.TxID
	commitVout := uint32(0)
	if commitTxID == "" {
		if len(tx.Inputs) == 0 {
			return "", CommitmentRecord{}, "", fmt.Errorf("missing name_new reference: no payload txid and no inputs")
		}
		// Backwards-compatible fallback: older clients encode the commitment reference
		// as the first input outpoint.
		commitTxID = tx.Inputs[0].TxID
		commitVout = tx.Inputs[0].Index
	}

	// Use the referenced name_new outpoint (txid:vout).
	log.Info().Msgf("NameFirstUpdate: resolving commitment for input %s", commitTxID)
	key := OutpointKey(commitTxID, commitVout)

	rec, ok := st.GetCommitment(key)
	if !ok {
		return "", CommitmentRecord{}, "", fmt.Errorf("no matching name_new commitment with key %s", key)
	}
	if commitment != rec.Commit {
		return "", CommitmentRecord{}, "", fmt.Errorf("commitment mismatch for domain %s", n.Domain)
	}
	return rec.Commit, rec, key, nil
}

const MaxFirstUpdateDepth uint64 = 25

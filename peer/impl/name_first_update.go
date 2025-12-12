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

func (n NameFirstUpdate) Validate(_ *NamecoinState, _ *SignedTransaction) error {
	// Basic payload sanity; full commit–reveal checks happen in ValidateWithInputs.
	if n.Domain == "" || n.Salt == "" || n.IP == "" {
		return fmt.Errorf("invalid name_firstupdate payload")
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
	}

	commit, key, err := n.resolveCommitment(st, tx)
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
		ExpiresAt: st.CurrentHeight() + st.effectiveTTL(resolveTTLPreference(n.TTL, commit, st)),
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

func (n NameFirstUpdate) ProcessTxState(st *NamecoinState, txID string, tx *types.Tx) error {
	return ProcessTxStateGeneric(st, txID, tx)
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
	// Preferred path: follow the referenced input back to the name_new outpoint.
	if len(tx.Inputs) > 0 {
		in := tx.Inputs[0]
		// MVP: commitments are keyed by the name_new transaction's first (and only)
		// output. Inputs here reference UTXOs being spent, but the commitment was
		// stored at txid:0 of the original name_new, so we fix the index to 0.
		key := OutpointKey(in.TxID, 0)
		commit, ok := st.GetCommitment(key)
		if !ok {
			return "", "", fmt.Errorf("no matching name_new commitment")
		}
		if HashString(n.Domain+n.Salt) != commit {
			return "", "", fmt.Errorf("commitment mismatch for domain %s", n.Domain)
		}
		return commit, key, nil
	}

	// Fallback path: tests may bundle name_new + name_firstupdate in the same
	// block without explicit inputs; locate the commitment by value instead.
	commit := HashString(n.Domain + n.Salt)
	if key, ok := st.FindCommitmentByValue(commit); ok {
		return commit, key, nil
	}

	return "", "", fmt.Errorf("no matching name_new commitment")
}

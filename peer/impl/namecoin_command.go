package impl

import (
	"fmt"
	"strings"

	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

var (
	NameNewCommandName         = NameNew{}.Name()
	NameFirstUpdateCommandName = NameFirstUpdate{}.Name()
	NameUpdateCommandName      = NameUpdate{}.Name()
	RewardCommandName          = Reward{}.Name()
)

func (st *NamecoinState) ProcessCommandTransactionStateUpdate(txID string, tx *types.Tx) error {
	switch tx.Type {
	case NameNewCommandName, NameFirstUpdateCommandName, NameUpdateCommandName:
		txIDs := make([]string, len(tx.Inputs))
		for i, value := range tx.Inputs {
			txIDs[i] = value.TxID
		}

		err := st.BurnUTXO(tx.From, txIDs)
		if err != nil {
			return err
		}

		// 1 or 0 UTXOs
		for _, value := range tx.Outputs {
			utxo := types.UTXO{
				TxID:   txID,
				To:     value.To,
				Amount: value.Amount,
			}
			err = st.AppendUTXO(utxo)
		}
	case RewardCommandName:
		// On Reward - always 1 UTXO
		utxo := types.UTXO{
			TxID:   txID,
			To:     tx.Outputs[0].To,
			Amount: tx.Outputs[0].Amount,
		}

		// save UTXO that rewards miner
		err := st.AppendUTXO(utxo)

		if err != nil {
			return err
		}
	}

	return nil
}

func (st *NamecoinState) ProcessCommandStateUpdate(tx *types.Tx) error {
	switch tx.Type {
	case NameNewCommandName:
		cmd, err := ResolveNameCoinCommand[NameNew](tx.Type, tx.Payload)

		if err != nil {
			return err
		}

		// we don't reveal the name on the initial domain creation, look at the project description
		st.SetCommitment(tx.From, cmd.Commitment)
	case NameFirstUpdateCommandName:
		cmd, err := ResolveNameCoinCommand[NameFirstUpdate](tx.Type, tx.Payload)
		if err != nil {
			return err
		}

		if st.IsDomainExists(cmd.Domain) {
			return xerrors.New("Domain already exists")
		}

		st.SetDomain(types.NameRecord{
			Owner:     tx.From,
			IP:        cmd.IP,
			Domain:    cmd.Domain,
			Salt:      cmd.Salt,
			ExpiresAt: 0, // todo: Add expiration
		})
	case NameUpdateCommandName:
		cmd, err := ResolveNameCoinCommand[NameUpdate](tx.Type, tx.Payload)
		if err != nil {
			return err
		}

		// rec is copy, changing it without a lock, then updating with lock.
		rec, ok := st.Domains[cmd.Domain]
		if !ok {
			return fmt.Errorf("updating non-existent domain %s", cmd.Domain)
		}

		// update only if the value is set. If value equals "", no updates have been made
		if len(strings.TrimSpace(cmd.Domain)) != 0 {
			rec.Domain = cmd.Domain
		}
		if len(strings.TrimSpace(cmd.IP)) != 0 {
			rec.IP = cmd.IP
		}

		st.SetDomain(rec)
		// todo: refresh domain lifetime

	case RewardCommandName:
		// nothing to verify specifically.
	default:
		return xerrors.New("unknown command")
	}

	return nil
}

// ValidateCommand verifies the payload of a transaction based on its type
func (st *NamecoinState) ValidateCommand(tx *SignedTransaction) error {
	switch tx.Type {

	case NameNewCommandName:
		p, wErr := ResolveNameCoinCommand[NameNew](tx.Type, tx.Payload)

		if wErr != nil {
			return wErr
		}

		if len(p.Commitment) == 0 {
			return fmt.Errorf("name_new commitment empty")
		}

	case NameFirstUpdateCommandName:
		p, wErr := ResolveNameCoinCommand[NameFirstUpdate](tx.Type, tx.Payload)
		if wErr != nil {
			return wErr
		}

		// Must match earlier commitment
		storedCommit := st.GetCommitment(tx.From)

		//todo: Update, to avoid collisions.
		if HashString(p.Salt+p.Domain) != storedCommit {
			return fmt.Errorf("commitment mismatch for domain %s", p.Domain)
		}

	case NameUpdateCommandName:
		p, wErr := ResolveNameCoinCommand[NameUpdate](tx.Type, tx.Payload)
		if wErr != nil {
			return wErr
		}

		owner := st.GetDomainOwner(p.Domain)
		if owner != tx.From {
			return fmt.Errorf("cannot update domain you do not own")
		}

	default:
		return fmt.Errorf("unsupported transaction type: %s", tx.Type)
	}

	return nil
}

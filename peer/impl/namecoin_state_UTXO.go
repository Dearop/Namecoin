package impl

import (
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// BurnUTXO function deletes UTXOs for corresponding pub Key "from"
func (st *NamecoinState) BurnUTXO(from string, txIDs []string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, txID := range txIDs {
		// burn UTXOs corresponding to TxID
		if _, ok := st.UTXOMap[from][txID]; !ok {
			return xerrors.Errorf("burn txID %s not found", from)
		}

		delete(st.UTXOMap[from], txID)
	}

	return nil
}

// AppendUTXO appends UTXO to users storage aka "balance top up"
func (st *NamecoinState) AppendUTXO(utxo types.UTXO) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if _, ok := st.UTXOMap[utxo.To][utxo.TxID]; ok {
		return xerrors.New("tx already exists")
	}

	st.UTXOMap[utxo.To][utxo.TxID] = utxo
	return nil

}

// GetUTXOsToBurn returns utxo IDs to burn and leftover UTXO.
func (st *NamecoinState) GetUTXOsToBurn(txID, from string, amount uint64) ([]string, []types.UTXO, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	userUTXOs := st.UTXOMap[from]
	UTXOsToBurn := make([]string, 0)

	// deduct until we burn enough UTXOs to pay
	for key, utxo := range userUTXOs {
		amount -= utxo.Amount
		UTXOsToBurn = append(UTXOsToBurn, key)

		if amount <= 0 {
			break
		}
	}

	// if amount still > 0 means that the user has not enough UTXOs to burn, revert
	if amount > 0 {
		return make([]string, 0), make([]types.UTXO, 0), xerrors.New("insufficient funds")
	}

	leftOver := 0 - amount

	if leftOver == 0 {
		return UTXOsToBurn, make([]types.UTXO, 0), nil
	}

	leftOverUTXO := types.UTXO{
		TxID:   txID,
		To:     from,
		Amount: leftOver,
	}

	// safe to return transactionIDS because we create only one UTXO per transaction
	// example, miner A mined a block; transaction is created - as a result, UTXO with corresponding txID is created.
	return UTXOsToBurn, append(make([]types.UTXO, 0, 1), leftOverUTXO), nil
}

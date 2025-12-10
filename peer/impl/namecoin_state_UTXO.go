package impl

import (
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// BurnUTXO function deletes UTXOs for corresponding pub Key "from"
func (st *NamecoinState) BurnUTXO(from string, txIDs []string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.UTXOMap == nil {
		return xerrors.Errorf("burn txID %s not found", from)
	}

	userUTXOs, ok := st.UTXOMap[from]
	if !ok {
		return xerrors.Errorf("burn txID %s not found", from)
	}

	seen := make(map[string]struct{}, len(txIDs))
	for _, txID := range txIDs {
		if _, dup := seen[txID]; dup {
			return xerrors.Errorf("duplicate utxo input %s", txID)
		}
		seen[txID] = struct{}{}

		// burn UTXOs corresponding to TxID
		if _, ok := userUTXOs[txID]; !ok {
			return xerrors.Errorf("burn txID %s not found", txID)
		}
	}

	for txID := range seen {
		delete(userUTXOs, txID)
	}

	return nil
}

// AppendUTXO appends UTXO to users storage aka "balance top up"
func (st *NamecoinState) AppendUTXO(utxo types.UTXO) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.UTXOMap == nil {
		st.UTXOMap = make(map[string]map[string]types.UTXO)
	}
	if st.UTXOMap[utxo.To] == nil {
		st.UTXOMap[utxo.To] = make(map[string]types.UTXO)
	}

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

	userUTXOs, ok := st.UTXOMap[from]
	if !ok || len(userUTXOs) == 0 {
		return make([]string, 0), make([]types.UTXO, 0), xerrors.New("insufficient funds")
	}
	UTXOsToBurn := make([]string, 0)

	total := int(amount)
	// deduct until we burn enough UTXOs to pay
	for key, utxo := range userUTXOs {
		total -= int(utxo.Amount)
		UTXOsToBurn = append(UTXOsToBurn, key)

		if total <= 0 {
			break
		}
	}

	// if amount still > 0 means that the user has not enough UTXOs to burn, revert
	if total > 0 {
		return make([]string, 0), make([]types.UTXO, 0), xerrors.New("insufficient funds")
	}

	leftOver := 0 - total

	if leftOver == 0 {
		return UTXOsToBurn, make([]types.UTXO, 0), nil
	}

	leftOverUTXO := types.UTXO{
		TxID:   txID,
		To:     from,
		Amount: uint64(leftOver),
	}

	// safe to return transactionIDS because we create only one UTXO per transaction
	// example, miner A mined a block; transaction is created - as a result, UTXO with corresponding txID is created.
	return UTXOsToBurn, append(make([]types.UTXO, 0, 1), leftOverUTXO), nil
}

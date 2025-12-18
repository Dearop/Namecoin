package impl

import (
	"fmt"
	"sort"

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
	inputs, outputs, err := st.DeterministicSpendPlan(from, amount)
	if err != nil {
		return []string{}, []types.UTXO{}, err
	}

	burn := make([]string, 0, len(inputs))
	for _, in := range inputs {
		burn = append(burn, in.TxID)
	}

	// outputs is either empty or change-back-to-sender (MVP)
	if len(outputs) == 0 {
		return burn, []types.UTXO{}, nil
	}

	leftOverUTXO := types.UTXO{
		TxID:   txID,
		To:     outputs[0].To,
		Amount: outputs[0].Amount,
	}
	return burn, []types.UTXO{leftOverUTXO}, nil
}

func (st *NamecoinState) DeterministicSpendPlan(from string, amount uint64) ([]types.TxInput, []types.TxOutput, error) {
	if amount == 0 {
		return []types.TxInput{}, []types.TxOutput{}, nil
	}

	// Snapshot
	type utxoEntry struct {
		txid   string
		amount uint64
	}

	st.mu.RLock()
	if st.UTXOMap == nil {
		st.mu.RUnlock()
		return nil, nil, fmt.Errorf("no utxos for sender")
	}
	userUTXOs := st.UTXOMap[from]
	if userUTXOs == nil {
		st.mu.RUnlock()
		return nil, nil, fmt.Errorf("no utxos for sender")
	}
	entries := make([]utxoEntry, 0, len(userUTXOs))
	for txid, utxo := range userUTXOs {
		entries = append(entries, utxoEntry{txid: txid, amount: utxo.Amount})
	}
	st.mu.RUnlock()

	// Sort UTXO keys deterministically.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].txid < entries[j].txid
	})

	var (
		inputs []types.TxInput
		total  uint64
	)
	for _, utxoKey := range entries {
		inputs = append(inputs, types.TxInput{
			TxID:  utxoKey.txid,
			Index: 0, // MVP single-output
		})
		total += utxoKey.amount
		if total >= amount {
			break
		}
	}

	if total < amount {
		return nil, nil, fmt.Errorf("insufficient funds")
	}

	leftover := total - amount
	outputs := make([]types.TxOutput, 0, 1)
	if leftover > 0 {
		// MVP: deterministic change back to sender only
		outputs = append(outputs, types.TxOutput{
			To:     from,
			Amount: leftover,
		})
	}

	return inputs, outputs, nil
}

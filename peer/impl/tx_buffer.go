package impl

import (
	"sync"

	"go.dedis.ch/cs438/types"
)

type TxBuffer struct {
	mu    sync.Mutex
	order []string
	txs   map[string]types.Tx
}

func NewTxBuffer() *TxBuffer {
	return &TxBuffer{
		order: make([]string, 0),
		txs:   make(map[string]types.Tx),
	}
}

func (b *TxBuffer) Add(tx types.Tx, txID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.txs == nil {
		b.txs = make(map[string]types.Tx)
	}
	if _, exists := b.txs[txID]; exists {
		return
	}
	b.txs[txID] = tx
	b.order = append(b.order, txID)
}

func (b *TxBuffer) Remove(txID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.txs == nil {
		return
	}
	if _, exists := b.txs[txID]; !exists {
		return
	}
	delete(b.txs, txID)

	// compact order lazily
	for i, id := range b.order {
		if id == txID {
			b.order = append(b.order[:i], b.order[i+1:]...)
			break
		}
	}
}

func (b *TxBuffer) Drain() ([]types.Tx, []string, map[string]types.Tx) {
	b.mu.Lock()
	defer b.mu.Unlock()

	orderCopy := append([]string(nil), b.order...)
	drained := make([]types.Tx, 0, len(orderCopy))
	txMap := make(map[string]types.Tx, len(orderCopy))
	for _, id := range orderCopy {
		if tx, ok := b.txs[id]; ok {
			drained = append(drained, tx)
			txMap[id] = tx
		}
	}
	b.order = nil
	b.txs = make(map[string]types.Tx)
	return drained, orderCopy, txMap
}

func (b *TxBuffer) Requeue(order []string, txs map[string]types.Tx) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.txs == nil {
		b.txs = make(map[string]types.Tx)
	}
	for _, id := range order {
		tx, ok := txs[id]
		if !ok {
			continue
		}
		if _, exists := b.txs[id]; !exists {
			b.order = append(b.order, id)
		}
		b.txs[id] = tx
	}
}

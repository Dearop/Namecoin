package peer

import "go.dedis.ch/cs438/types"

type Namecoin interface {
	HandleNamecoinCommand(buf []byte) error
	GetMinerID() string
	GetDomains() []types.NameRecord
	SetMinerID(minerID string) error
	GetSpendPlan(from string, amount uint64) ([]types.TxInput, []types.TxOutput, error)
}

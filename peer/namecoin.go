package peer

import "go.dedis.ch/cs438/types"

type Namecoin interface {
	HandleNamecoinCommand(buf []byte) error
	GetMinerID() string
	GetDomains() []types.NameRecord
}

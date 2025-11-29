package types

import (
	"encoding/json"
)

// Tx is a generic on-chain transaction container. Concrete Namecoin
// operations (such as name_new, name_firstupdate, name_update, etc.) will be
// implemented using this basic form for inclusion in blocks.
type Tx struct {
	// ID uniquely identifies the transaction
	ID []byte
	// Raw payload (opaque for now)
	Payload TxPayload
}

type TxPayload struct {
	From   string
	To     string
	Op     string // "register", "firstupdate", "update", "transfer", "pay", "coinbase"
	Name   string // domain name (for name ops)
	Value  string // stored value for the domain
	Amount uint64 // coins transferred (pay/coinbase)
	Fee    uint64 // tx fee burned
}

// BlockHeader captures the PoW header fields that are hashed when
// mining, following the planned architecture (height, prev hash, tx root,
// timestamp, nonce, miner, difficulty).
type BlockHeader struct {
	Height     uint64
	PrevHash   []byte
	Hash       []byte
	TxRoot     []byte
	Timestamp  int64
	Nonce      uint64
	Miner      string // coinbase address
	Difficulty []byte
}

// Block ties the header with its transaction list.
type Block struct {
	Header       BlockHeader
	Transactions []Tx
}

// NameRecord is a minimal name -> value mapping
type NameRecord struct {
	Owner     string
	Value     string // TODO: we need to update this to salted and unexported commitment
	ExpiresAt uint64 // block height; 0 if never expires
}

type Balance = uint64

// Serialisation
// Marshal marshals the NamecoinBlock into bytes for the blockchain store
func (b *Block) Marshal() ([]byte, error) {
	return json.Marshal(b)
}

// Unmarshal unmarshals data into this NamecoinBlock
func (b *Block) Unmarshal(data []byte) error {
	return json.Unmarshal(data, b)
}

package types

import "encoding/json"

// Tx is a generic on-chain transaction container. Concrete Namecoin
// operations (such as name_new, name_firstupdate, name_update, etc.) will be
// implemented using this basic form for inclusion in blocks.
// txID - is a transaction hash constructed with these properties. No need to store it here
// because storage itself will refer to this transaction by using txID as a key.
type Tx struct {
	Type    string
	From    string
	Fee     uint64
	Payload json.RawMessage
}

// BlockHeader captures the PoW header fields that are hashed when
// mining, following the planned architecture (height, prev hash, tx root,
// timestamp, nonce, miner, difficulty).
type BlockHeader struct {
	Height     uint64
	PrevHash   []byte
	TxRoot     []byte
	Timestamp  int64
	Nonce      uint64
	Miner      string
	Difficulty []byte
}

// Block ties the header with its transaction list.
type Block struct {
	Header       BlockHeader
	Transactions []Tx
}

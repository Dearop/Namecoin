package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type UTXO struct {
	TxID   string
	To     string
	Amount uint64
	Order  uint64
}

type TxInput struct {
	TxID string
	// Index of the output being spent from the referenced transaction.
	Index uint32
	// TODO: replace with following in case multiple outputs
	// OutPoint string = TxID + Index
}

type TxOutput struct {
	To     string
	Amount uint64
}

// Tx is a generic on-chain transaction container. Concrete Namecoin
// operations (such as name_new, name_firstupdate, name_update, etc.) will be
// implemented using this basic form for inclusion in blocks.
type Tx struct {
	From string
	Type string

	// UTXOs inputs/outputs
	Inputs []TxInput

	// we can produce at MAX one UTXO to corresponding transaction
	// stored as an array for simplicity
	Outputs []TxOutput

	// primary amount
	Amount  uint64
	Payload json.RawMessage

	// On-chain authentication metadata (MVP).
	// For Reward transactions these may be empty.
	Pk        string
	TxID      string
	Signature string
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
	Miner      string // coinbase address
	Difficulty []byte
}

// Block ties the header with its transaction list.
type Block struct {
	Header       BlockHeader
	Transactions []Tx

	Hash []byte
}

type NameRecord struct {
	Owner  string
	IP     string
	Domain string
	Salt   string

	ExpiresAt uint64 // block height; 0 if never expires
}

// Serialisation
func (b *Block) Marshal() ([]byte, error) {
	// Serialize the header using the custom method to respect determinism
	headerBytes := b.Header.SerializeHeader()
	headerHex := hex.EncodeToString(headerBytes)

	// Marshal transactions normally
	txData := b.Transactions

	// Marshal hash as hex string too
	hashHex := hex.EncodeToString(b.Hash)

	// Create JSON structure
	data := map[string]interface{}{
		"header":       headerHex,
		"transactions": txData,
		"hash":         hashHex,
	}

	return json.Marshal(data)
}

func (b *Block) Unmarshal(data []byte) error {
	var raw struct {
		Header       string `json:"header"`
		Transactions []Tx   `json:"transactions"`
		Hash         string `json:"hash"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	headerBytes, err := hex.DecodeString(raw.Header)
	if err != nil {
		return err
	}
	var header BlockHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return err
	}
	hashBytes, err := hex.DecodeString(raw.Hash)
	if err != nil {
		return err
	}

	b.Header = header
	b.Transactions = raw.Transactions
	b.Hash = hashBytes

	return nil
}

// NamecoinTransactionMessage message for broadcasting UTXO across the nodes
type NamecoinTransactionMessage struct {
	Type      string
	From      string
	Amount    uint64
	Payload   json.RawMessage
	Inputs    []TxInput
	Outputs   []TxOutput
	Tx        Tx
	Pk        string
	TxID      string
	Signature string
}

type NamecoinBlockMessage struct {
	Block Block
}

func (b *Block) ComputeHash() []byte {
	headerBytes := b.Header.SerializeHeader()
	h := sha256.Sum256(headerBytes)
	return h[:]
}

func (h *BlockHeader) SerializeHeader() []byte {
	data := struct {
		Height     uint64 `json:"height"`
		PrevHash   []byte `json:"prevHash"`
		TxRoot     []byte `json:"txRoot"`
		Timestamp  int64  `json:"timestamp"`
		Nonce      uint64 `json:"nonce"`
		Miner      string `json:"miner"`
		Difficulty []byte `json:"difficulty"`
	}{
		Height:     h.Height,
		PrevHash:   h.PrevHash,
		TxRoot:     h.TxRoot,
		Timestamp:  h.Timestamp,
		Nonce:      h.Nonce,
		Miner:      h.Miner,
		Difficulty: h.Difficulty,
	}

	b, _ := json.Marshal(data)
	return b
}

func (h *BlockHeader) SerializeBase() []byte {
	data := struct {
		Height     uint64 `json:"height"`
		PrevHash   []byte `json:"prevHash"`
		TxRoot     []byte `json:"txRoot"`
		Miner      string `json:"miner"`
		Difficulty []byte `json:"difficulty"`
	}{
		Height:     h.Height,
		PrevHash:   h.PrevHash,
		TxRoot:     h.TxRoot,
		Miner:      h.Miner,
		Difficulty: h.Difficulty,
	}

	b, _ := json.Marshal(data)
	return b
}

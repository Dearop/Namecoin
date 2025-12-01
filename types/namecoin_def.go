package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
)

type UTXO struct {
	TxID   string
	To     string
	Amount uint64
}

type TxInput struct {
	TxID string
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
	Output TxOutput

	Amount  uint64
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
	Miner      string // coinbase address
	Difficulty []byte
}

// Block ties the header with its transaction list.
type Block struct {
	Header       BlockHeader
	Transactions []Tx

	Hash []byte
}

// NameRecord is a minimal name -> value mapping
type NameRecord struct {
	Owner  string
	IP     string
	Domain string
	Salt   string

	ExpiresAt uint64 // block height; 0 if never expires
}

// Serialisation
// Marshal marshals the NamecoinBlock into bytes for the blockchain store
func (b *Block) Marshal() ([]byte, error) {

	return json.Marshal(b)
}

// Unmarshal unmarshals data into this NamecoinBlock
func (b *Block) Unmarshal(data []byte) error {
	return json.Unmarshal(data, b)
}

// NamecoinTransactionMessage message for broadcasting UTXO across the nodes
type NamecoinTransactionMessage struct {
	Tx   Tx
	TxID string
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
	buf := new(bytes.Buffer)

	_ = binary.Write(buf, binary.LittleEndian, h.Height)

	_ = binary.Write(buf, binary.LittleEndian, uint32(len(h.PrevHash)))
	buf.Write(h.PrevHash)

	_ = binary.Write(buf, binary.LittleEndian, uint32(len(h.TxRoot)))
	buf.Write(h.TxRoot)

	_ = binary.Write(buf, binary.LittleEndian, h.Timestamp)

	_ = binary.Write(buf, binary.LittleEndian, h.Nonce)

	minerBytes := []byte(h.Miner)
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(minerBytes)))
	buf.Write(minerBytes)

	_ = binary.Write(buf, binary.LittleEndian, uint32(len(h.Difficulty)))
	buf.Write(h.Difficulty)

	return buf.Bytes()
}

func (h *BlockHeader) SerializeBase() []byte {
	buf := new(bytes.Buffer)

	_ = binary.Write(buf, binary.LittleEndian, h.Height)

	_ = binary.Write(buf, binary.LittleEndian, uint32(len(h.PrevHash)))
	buf.Write(h.PrevHash)

	_ = binary.Write(buf, binary.LittleEndian, uint32(len(h.TxRoot)))
	buf.Write(h.TxRoot)

	minerBytes := []byte(h.Miner)
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(minerBytes)))
	buf.Write(minerBytes)

	_ = binary.Write(buf, binary.LittleEndian, uint32(len(h.Difficulty)))
	buf.Write(h.Difficulty)

	return buf.Bytes()
}

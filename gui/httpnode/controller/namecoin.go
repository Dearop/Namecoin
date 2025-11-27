package controller

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/types"
)

// NewNamecoinCtrl returns a new namecoin controller
func NewNamecoinCtrl(peer peer.Peer, log *zerolog.Logger) namecoinctrl {
	return namecoinctrl{
		peer: peer,
		log:  log,
	}
}

type namecoinctrl struct {
	peer peer.Peer
	log  *zerolog.Logger
}

type TransactionRequest struct {
	Transaction struct {
		Type          string `json:"type"`
		Source        string `json:"source"`
		Fee           int    `json:"fee"`
		Payload       string `json:"payload"`
		Nonce         int    `json:"nonce"`
		TransactionID string `json:"transactionID"`
	} `json:"transaction"`
	Signature string `json:"signature"`
}

type TransactionResponse struct {
	Success bool   `json:"success"`
	TxID    string `json:"txID,omitempty"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (n namecoinctrl) SubmitTransactionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			n.submitTransactionPost(w, r)
		case http.MethodOptions:
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			return
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
			return
		}
	}
}

func (n namecoinctrl) submitTransactionPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Content-Type", "application/json")

	var req TransactionRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		n.log.Error().Err(err).Msg("failed to decode request")
		json.NewEncoder(w).Encode(TransactionResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	n.log.Info().
		Str("type", req.Transaction.Type).
		Str("source", req.Transaction.Source).
		Int("fee", req.Transaction.Fee).
		Str("payload", req.Transaction.Payload).
		Msg("Received transaction")

	// Build the transaction struct
	tx := types.Transaction{
		Type:          req.Transaction.Type,
		Source:        req.Transaction.Source,
		Fee:           req.Transaction.Fee,
		Payload:       req.Transaction.Payload,
		Nonce:         req.Transaction.Nonce,
		TransactionID: req.Transaction.TransactionID,
	}

	signedTx := types.SignedTransaction{
		Tx:        tx,
		Signature: req.Signature,
	}

	// Submit the transaction
	txID, err := n.peer.SubmitTransaction(signedTx)
	if err != nil {
		n.log.Error().Err(err).Msg("SubmitTransaction failed")
		json.NewEncoder(w).Encode(TransactionResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	n.log.Info().
		Str("txID", txID).
		Msg("Successfully submitted transaction")

	response := TransactionResponse{
		Success: true,
		TxID:    txID,
		Status:  "confirmed",
	}

	json.NewEncoder(w).Encode(response)
}

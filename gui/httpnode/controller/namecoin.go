package controller

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"
	"go.dedis.ch/cs438/peer"
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
		Msg("Received transaction")

	// TODO: Verify signature and process transaction
	// For now, just acknowledge receipt
	response := TransactionResponse{
		Success: true,
		TxID:    req.Transaction.TransactionID,
		Status:  "pending",
	}

	json.NewEncoder(w).Encode(response)
}

package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/types"
)

func NewNamecoinController(node peer.Peer, transactionService *impl.TransactionService, l *zerolog.Logger) Namecoin {
	return Namecoin{node, transactionService, l}
}

type Namecoin struct {
	node               peer.Peer
	transactionService *impl.TransactionService
	log                *zerolog.Logger
}

func (n Namecoin) NewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			n.Handle(w, r)
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

func (n Namecoin) Handle(w http.ResponseWriter, r *http.Request) {
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err),
			http.StatusInternalServerError)
		return
	}

	var transaction impl.SignedTransaction
	err = json.Unmarshal(buf, &transaction)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to unmarshal transaction: %v", err),
			http.StatusInternalServerError)
		return
	}

	err = n.transactionService.ValidateTransaction(&transaction)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to validate transaction: %v", err),
			http.StatusBadRequest)
		return
	}

	inputs, output, err := n.transactionService.VerifyBalance(transaction.TxID, transaction.From, transaction.Amount)

	msg := types.NamecoinTransactionMessage{
		TxID: transaction.TxID,
		Tx: types.Tx{
			From:    transaction.From,
			Type:    transaction.Type,
			Inputs:  inputs,
			Output:  output,
			Amount:  transaction.Amount,
			Payload: transaction.Payload,
		},
	}

	err = n.node.BroadcastTransaction(&msg)

	if err != nil {
		http.Error(w, fmt.Sprintf("failed to broadcast transaction: %v", err),
			http.StatusInternalServerError)
		return
	}
}

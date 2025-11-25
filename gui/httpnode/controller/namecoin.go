package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog"
	"go.dedis.ch/cs438/peer/impl"
)

func NewNamecoinController(transactionService *impl.TransactionService, l *zerolog.Logger) Namecoin {
	return Namecoin{transactionService, l}
}

type Namecoin struct {
	transactionService *impl.TransactionService
	log                *zerolog.Logger
}

func (n Namecoin) NewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			n.New(w, r)
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

func (n Namecoin) FirstUpdateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			n.FirstUpdate(w, r)
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

func (n Namecoin) UpdateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			n.Update(w, r)
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

func (n Namecoin) New(w http.ResponseWriter, r *http.Request) {
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
	}

	// todo: Send transaction to consensus, etc...
}

func (n Namecoin) FirstUpdate(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented",
		http.StatusInternalServerError)
}

func (n Namecoin) Update(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented",
		http.StatusInternalServerError)
}

package controller

import (
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog"
	"go.dedis.ch/cs438/peer"
)

func NewNamecoinController(node peer.Peer, l *zerolog.Logger) Namecoin {
	return Namecoin{node, l}
}

type Namecoin struct {
	node peer.Peer
	log  *zerolog.Logger
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

	err = n.node.HandleNamecoinCommand(buf)
	if err != nil {
		n.log.Err(err)
		http.Error(w, fmt.Sprintf("Error occured: %v", err), http.StatusBadRequest)
	}
}

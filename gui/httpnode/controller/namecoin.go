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
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		switch r.Method {
		case http.MethodOptions:
			// Handle preflight request
			w.WriteHeader(http.StatusOK)
			return
		case http.MethodPost:
			n.Handle(w, r)
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

func (n Namecoin) Handle(w http.ResponseWriter, r *http.Request) {
	//print that we received a request
	n.log.Info().Msg("Received Namecoin command")
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err),
			http.StatusInternalServerError)
		return
	}

	err = n.node.HandleNamecoinCommand(buf)
	if err != nil {
		n.log.Err(err).Msgf("HandleNamecoinCommand error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf(`{"status":"error","message":"%v"}`, err)))
		return
	}

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success","message":"Transaction received"}`))
}

func (n Namecoin) MinerIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		switch r.Method {
		case http.MethodOptions:
			// Handle preflight request
			w.WriteHeader(http.StatusOK)
			return
		case http.MethodGet:
			minerID := n.node.GetMinerID()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf(`{"minerID":"%s"}`, minerID)))
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

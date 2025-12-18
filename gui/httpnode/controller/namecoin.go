package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/types"
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
		case http.MethodPost:
			var body struct {
				MinerID string `json:"minerID"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, fmt.Sprintf("invalid body: %v", err), http.StatusBadRequest)
				return
			}
			if err := n.node.SetMinerID(body.MinerID); err != nil {
				http.Error(w, fmt.Sprintf("failed to set miner ID: %v", err), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf(`{"minerID":"%s"}`, body.MinerID)))
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

func (n Namecoin) DomainsHandler() http.HandlerFunc {
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
			domains := n.node.GetDomains()
			// Convert domains to JSON
			jsonData, err := json.Marshal(domains)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to marshal domains: %v", err),
					http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(jsonData)
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

func (n Namecoin) SpendPlanHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		switch r.Method {
		case http.MethodOptions:
			w.WriteHeader(http.StatusOK)
			return
		case http.MethodPost:
			var body struct {
				From   string `json:"from"`
				Amount uint64 `json:"amount"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, fmt.Sprintf("invalid body: %v", err), http.StatusBadRequest)
				return
			}
			inputs, outputs, err := n.node.GetSpendPlan(body.From, body.Amount)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to compute spend plan: %v", err), http.StatusBadRequest)
				return
			}
			resp := struct {
				Inputs  []types.TxInput  `json:"inputs"`
				Outputs []types.TxOutput `json:"outputs"`
			}{
				Inputs:  inputs,
				Outputs: outputs,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

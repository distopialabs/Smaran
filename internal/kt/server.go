package kt

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// PutRequest is the JSON body for POST /put.
type PutRequest struct {
	User []byte `json:"user"`
	Key  []byte `json:"key"`
}

// GetRequest is the JSON body for POST /get.
type GetRequest struct {
	User       []byte `json:"user"`
	UseCaching bool   `json:"use_caching"`
}

// GetCommitmentResponse is the JSON body returned by POST /get_commitment.
type GetCommitmentResponse struct {
	Commitment []byte `json:"commitment"`
}

// Protocol selects which KT protocol the server runs.
type Protocol string

const (
	ProtocolOptiks  Protocol = "optiks"
	ProtocolSamurai Protocol = "samurai"
)

// KTHandler holds the HTTP handler state. When protocol is "samurai", all
// endpoints return HTTP 501 (Unimplemented). When "optiks", requests are
// dispatched to the OptiksServer.
type KTHandler struct {
	protocol Protocol
	optiks   *OptiksServer
}

// NewKTHandler creates an HTTP handler for the given protocol.
func NewKTHandler(protocol Protocol, batchSize uint64) *KTHandler {
	h := &KTHandler{protocol: protocol}
	if protocol == ProtocolOptiks {
		h.optiks = NewOptiksServer(batchSize)
	}
	return h
}

// RegisterRoutes wires the three KT endpoints onto the given ServeMux.
func (h *KTHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/put", h.handlePut)
	mux.HandleFunc("/get", h.handleGet)
	mux.HandleFunc("/get_commitment", h.handleGetCommitment)
}

// handlePut handles POST /put.
// See KT.md § "Handling Put(user, key)".
func (h *KTHandler) handlePut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.protocol == ProtocolSamurai {
		http.Error(w, "Unimplemented", http.StatusNotImplemented)
		return
	}

	var req PutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}

	h.optiks.Put(req.User, req.Key)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleGet handles POST /get.
// See KT.md § "Handling Get(user)".
func (h *KTHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.protocol == ProtocolSamurai {
		http.Error(w, "Unimplemented", http.StatusNotImplemented)
		return
	}

	var req GetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}

	result, err := h.optiks.Get(req.User, req.UseCaching)
	if err != nil {
		http.Error(w, fmt.Sprintf("internal error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleGetCommitment handles POST /get_commitment.
// See KT.md § "Handling GetCommitment".
func (h *KTHandler) handleGetCommitment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.protocol == ProtocolSamurai {
		http.Error(w, "Unimplemented", http.StatusNotImplemented)
		return
	}

	commitment := h.optiks.GetCommitment()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetCommitmentResponse{Commitment: commitment})
}

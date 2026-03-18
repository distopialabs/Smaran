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

// KTHandler holds the HTTP handler state.
type KTHandler struct {
	protocol Protocol
	optiks   *OptiksServer
	samurai  *SamuraiKTServer
}

// NewKTHandler creates an HTTP handler for the given protocol.
func NewKTHandler(protocol Protocol, batchSize uint64, paramsDir string) *KTHandler {
	h := &KTHandler{protocol: protocol}
	switch protocol {
	case ProtocolOptiks:
		h.optiks = NewOptiksServer(batchSize)
	case ProtocolSamurai:
		h.samurai = NewSamuraiKTServer(batchSize, paramsDir)
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
	var req PutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}

	switch h.protocol {
	case ProtocolOptiks:
		h.optiks.Put(req.User, req.Key)
	case ProtocolSamurai:
		h.samurai.Put(req.User, req.Key)
	}

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
	var req GetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}

	var result interface{}
	var err error
	switch h.protocol {
	case ProtocolOptiks:
		result, err = h.optiks.Get(req.User, req.UseCaching)
	case ProtocolSamurai:
		result, err = h.samurai.Get(req.User)
	}
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
	var commitment []byte
	switch h.protocol {
	case ProtocolOptiks:
		commitment = h.optiks.GetCommitment()
	case ProtocolSamurai:
		commitment = h.samurai.GetCommitment()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetCommitmentResponse{Commitment: commitment})
}

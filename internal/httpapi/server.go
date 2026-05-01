package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"remitly-stock-market/internal/market"
)

type Server struct {
	bank market.BankStocks
}

func NewHandler() http.Handler {
	return NewHandlerWithBank(market.NewMemoryBank())
}

func NewHandlerWithBank(bank market.BankStocks) http.Handler {
	server := &Server{bank: bank}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/stocks", server.handleStocks)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleStocks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetStocks(w, r)
	case http.MethodPost:
		s.handlePostStocks(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleGetStocks(w http.ResponseWriter, r *http.Request) {
	stocks, err := s.bank.ListStocks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, stocksResponse{
		Stocks: stocks,
	})
}

func (s *Server) handlePostStocks(w http.ResponseWriter, r *http.Request) {
	var body stocksRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Stocks == nil {
		writeError(w, http.StatusBadRequest, "stocks field is required")
		return
	}

	if err := s.bank.SetStocks(r.Context(), *body.Stocks); err != nil {
		var validationError market.ValidationError
		if errors.As(err, &validationError) {
			writeError(w, http.StatusBadRequest, validationError.Message)
			return
		}

		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{})
}

type stocksRequest struct {
	Stocks *[]market.Stock `json:"stocks"`
}

type stocksResponse struct {
	Stocks []market.Stock `json:"stocks"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

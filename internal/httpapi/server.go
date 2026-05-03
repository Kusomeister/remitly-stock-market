package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"remitly-stock-market/internal/market"
)

type Server struct {
	store market.Market
}

func NewHandler(store market.Market) http.Handler {
	server := &Server{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/stocks", server.handleStocks)
	mux.HandleFunc("/wallets/{wallet_id}", server.handleWallet)
	mux.HandleFunc("/wallets/{wallet_id}/stocks/{stock_name}", server.handleWalletStock)
	mux.HandleFunc("/log", server.handleLog)
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
	stocks, err := s.store.ListStocks(r.Context())
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

	if err := s.store.SetStocks(r.Context(), *body.Stocks); err != nil {
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

func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	s.handleGetWallet(w, r, r.PathValue("wallet_id"))
}

func (s *Server) handleWalletStock(w http.ResponseWriter, r *http.Request) {
	walletID := r.PathValue("wallet_id")
	stockName := r.PathValue("stock_name")
	switch r.Method {
	case http.MethodGet:
		s.handleGetWalletStock(w, r, walletID, stockName)
	case http.MethodPost:
		s.handleTrade(w, r, walletID, stockName)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleGetWallet(w http.ResponseWriter, r *http.Request, walletID string) {
	wallet, err := s.store.GetWallet(r.Context(), walletID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, wallet)
}

func (s *Server) handleGetWalletStock(w http.ResponseWriter, r *http.Request, walletID, stockName string) {
	quantity, err := s.store.GetWalletStock(r.Context(), walletID, stockName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, quantity)
}

func (s *Server) handleTrade(w http.ResponseWriter, r *http.Request, walletID, stockName string) {
	var body tradeRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Type != market.OperationBuy && body.Type != market.OperationSell {
		writeError(w, http.StatusBadRequest, "invalid operation type")
		return
	}

	if err := s.store.Trade(r.Context(), walletID, stockName, body.Type); err != nil {
		switch {
		case errors.Is(err, market.ErrStockNotFound):
			writeError(w, http.StatusNotFound, "stock not found")
		case errors.Is(err, market.ErrInsufficientBankStock):
			writeError(w, http.StatusBadRequest, "insufficient bank stock")
		case errors.Is(err, market.ErrInsufficientWalletStock):
			writeError(w, http.StatusBadRequest, "insufficient wallet stock")
		case errors.Is(err, market.ErrInvalidOperation):
			writeError(w, http.StatusBadRequest, "invalid operation type")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{})
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	logEntries, err := s.store.ListLog(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, logResponse{Log: logEntries})
}

type stocksRequest struct {
	Stocks *[]market.Stock `json:"stocks"`
}

type tradeRequest struct {
	Type market.OperationType `json:"type"`
}

type stocksResponse struct {
	Stocks []market.Stock `json:"stocks"`
}

type logResponse struct {
	Log []market.LogEntry `json:"log"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

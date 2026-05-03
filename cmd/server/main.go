package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"remitly-stock-market/internal/httpapi"
	"remitly-stock-market/internal/market"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	if strings.HasPrefix(port, ":") {
		addr = port
	}

	store := market.NewMemoryMarket()
	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(store),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

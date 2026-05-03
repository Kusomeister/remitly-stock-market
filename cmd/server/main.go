package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"remitly-stock-market/internal/httpapi"
	"remitly-stock-market/internal/market"
	"remitly-stock-market/internal/postgres"
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

	store := market.Market(market.NewMemoryMarket())
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		pool, err := postgres.Connect(context.Background(), databaseURL)
		if err != nil {
			log.Fatal(err)
		}
		defer pool.Close()

		if err := postgres.Migrate(context.Background(), pool); err != nil {
			log.Fatal(err)
		}

		store = postgres.NewStore(pool)
	}

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

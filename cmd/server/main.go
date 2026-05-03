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
	addr := listenAddress(envOrDefault("PORT", "8080"))
	metricsAddr := listenAddress(envOrDefault("METRICS_PORT", "9091"))

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

	observability := httpapi.NewObservability()
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", observability.MetricsHandler())
	metricsServer := &http.Server{
		Addr:              metricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("metrics listening on %s", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewObservedHandlerWithChaos(store, func() { os.Exit(1) }, observability),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func envOrDefault(name, defaultValue string) string {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}

	return value
}

func listenAddress(port string) string {
	if strings.HasPrefix(port, ":") {
		return port
	}

	return ":" + port
}

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remitly-stock-market/internal/market"
)

func TestObservedHandlerRecordsRequestMetrics(t *testing.T) {
	var logs bytes.Buffer
	observability := NewObservabilityWithLogWriter(&logs)
	handler := NewObservedHandler(market.NewMemoryMarket(), observability)

	rec := doRequest(handler, http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	metrics := metricsText(t, observability)
	if !strings.Contains(metrics, `remitly_http_requests_total{method="GET",route="/health",status="200"} 1`) {
		t.Fatalf("expected request counter sample, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, `remitly_http_request_duration_seconds_count{method="GET",route="/health",status="200"} 1`) {
		t.Fatalf("expected request duration observation, got:\n%s", metrics)
	}
}

func TestObservedHandlerWritesJSONRequestLog(t *testing.T) {
	var logs bytes.Buffer
	observability := NewObservabilityWithLogWriter(&logs)
	handler := NewObservedHandler(market.NewMemoryMarket(), observability)

	rec := doRequest(handler, http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var entry requestLogEntry
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("expected JSON request log: %v", err)
	}
	if entry.Method != http.MethodGet {
		t.Fatalf("expected method %q, got %q", http.MethodGet, entry.Method)
	}
	if entry.Path != "/health" {
		t.Fatalf("expected path /health, got %q", entry.Path)
	}
	if entry.Route != "/health" {
		t.Fatalf("expected route /health, got %q", entry.Route)
	}
	if entry.Status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, entry.Status)
	}
	if entry.DurationMS < 0 {
		t.Fatalf("expected non-negative duration, got %f", entry.DurationMS)
	}
}

func TestMetricsHandlerExposesPrometheusFormat(t *testing.T) {
	observability := NewObservabilityWithLogWriter(nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	observability.MetricsHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("expected prometheus text content type, got %q", contentType)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatalf("expected Go runtime metrics, got:\n%s", rec.Body.String())
	}
}

func metricsText(t *testing.T, observability *Observability) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	observability.MetricsHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d", http.StatusOK, rec.Code)
	}

	return rec.Body.String()
}

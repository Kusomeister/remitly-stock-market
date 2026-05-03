package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const unmatchedRoute = "unmatched"

type Observability struct {
	registry         *prometheus.Registry
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	requestsInflight prometheus.Gauge
	logWriter        io.Writer
	logMu            sync.Mutex
}

type requestLogEntry struct {
	Time       string  `json:"time"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Route      string  `json:"route"`
	Status     int     `json:"status"`
	DurationMS float64 `json:"duration_ms"`
	RemoteAddr string  `json:"remote_addr,omitempty"`
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func NewObservability() *Observability {
	return NewObservabilityWithLogWriter(os.Stdout)
}

func NewObservabilityWithLogWriter(logWriter io.Writer) *Observability {
	if logWriter == nil {
		logWriter = io.Discard
	}

	registry := prometheus.NewRegistry()
	observability := &Observability{
		registry: registry,
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "remitly_http_requests_total",
			Help: "Total number of HTTP requests handled by the Remitly stock market API.",
		}, []string{"method", "route", "status"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "remitly_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds for the Remitly stock market API.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		requestsInflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "remitly_http_requests_in_flight",
			Help: "Current number of HTTP requests being handled by the Remitly stock market API.",
		}),
		logWriter: logWriter,
	}

	registry.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		observability.requestsTotal,
		observability.requestDuration,
		observability.requestsInflight,
	)

	return observability
}

func (o *Observability) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		o.requestsInflight.Inc()
		defer o.requestsInflight.Dec()

		next.ServeHTTP(recorder, r)

		route := r.Pattern
		if route == "" {
			route = unmatchedRoute
		}

		status := strconv.Itoa(recorder.status)
		duration := time.Since(start)
		o.requestsTotal.WithLabelValues(r.Method, route, status).Inc()
		o.requestDuration.WithLabelValues(r.Method, route, status).Observe(duration.Seconds())
		o.writeRequestLog(r, route, recorder.status, duration)
	})
}

func (o *Observability) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(o.registry, promhttp.HandlerOpts{})
}

func (o *Observability) writeRequestLog(r *http.Request, route string, status int, duration time.Duration) {
	entry := requestLogEntry{
		Time:       time.Now().UTC().Format(time.RFC3339Nano),
		Method:     r.Method,
		Path:       r.URL.Path,
		Route:      route,
		Status:     status,
		DurationMS: float64(duration.Microseconds()) / 1000,
		RemoteAddr: r.RemoteAddr,
	}

	o.logMu.Lock()
	defer o.logMu.Unlock()
	_ = json.NewEncoder(o.logWriter).Encode(entry)
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wrote {
		return
	}

	r.status = status
	r.wrote = true
	r.ResponseWriter.WriteHeader(status)
}

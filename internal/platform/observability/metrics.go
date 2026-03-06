package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds Prometheus metrics
type Metrics struct {
	RequestTotal    *prometheus.CounterVec   // Total requests
	RequestDuration *prometheus.HistogramVec // Request latency

	TxPostedTotal     prometheus.Counter // Committed transactions
	IdempotencyReplay prometheus.Counter // Idempotency key replays
	DBErrorTotal      prometheus.Counter // Database errors
}

// NewMetrics creates metrics for a service
func NewMetrics(serviceName string) *Metrics {
	requestTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "requests_total",
			Help:        "Total number of requests",
			ConstLabels: prometheus.Labels{"service": serviceName},
		},
		[]string{"method", "status"},
	)

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "request_duration_seconds",
			Help:        "Request duration in seconds",
			Buckets:     prometheus.DefBuckets,
			ConstLabels: prometheus.Labels{"service": serviceName},
		},
		[]string{"method", "status"},
	)

	txPostedTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "ledger_tx_posted_total",
		Help:        "Total number of committed transactions",
		ConstLabels: prometheus.Labels{"service": serviceName},
	})

	idempotencyReplay := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "ledger_idempotency_replay_total",
		Help:        "Total number of idempotency key replays",
		ConstLabels: prometheus.Labels{"service": serviceName},
	})

	dbErrorTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "ledger_db_errors_total",
		Help:        "Total number of database errors",
		ConstLabels: prometheus.Labels{"service": serviceName},
	})

	prometheus.MustRegister(requestTotal, requestDuration, txPostedTotal, idempotencyReplay, dbErrorTotal)

	return &Metrics{
		RequestTotal:      requestTotal,
		RequestDuration:   requestDuration,
		TxPostedTotal:     txPostedTotal,
		IdempotencyReplay: idempotencyReplay,
		DBErrorTotal:      dbErrorTotal,
	}
}

// Handler returns the Prometheus metrics HTTP handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

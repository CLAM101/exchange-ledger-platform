package observability

import "github.com/prometheus/client_golang/prometheus"

// NewTestMetrics creates a Metrics instance backed by a custom registry
// so tests don't collide with the global Prometheus registry.
func NewTestMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	requestTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_requests_total"},
		[]string{"method", "status"},
	)
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "test_request_duration_seconds", Buckets: prometheus.DefBuckets},
		[]string{"method", "status"},
	)
	txPostedTotal := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_ledger_tx_posted_total"})
	idempotencyReplay := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_ledger_idempotency_replay_total"})
	dbErrorTotal := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_ledger_db_errors_total"})

	reg.MustRegister(requestTotal, requestDuration, txPostedTotal, idempotencyReplay, dbErrorTotal)

	return &Metrics{
		RequestTotal:      requestTotal,
		RequestDuration:   requestDuration,
		TxPostedTotal:     txPostedTotal,
		IdempotencyReplay: idempotencyReplay,
		DBErrorTotal:      dbErrorTotal,
	}
}

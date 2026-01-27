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

	prometheus.MustRegister(requestTotal, requestDuration)

	return &Metrics{
		RequestTotal:    requestTotal,
		RequestDuration: requestDuration,
	}
}

// Handler returns the Prometheus metrics HTTP handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

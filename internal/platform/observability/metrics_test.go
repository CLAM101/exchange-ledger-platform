package observability

import (
	"testing"
)

func TestNewMetrics(t *testing.T) {
	t.Parallel()

	// Test that NewMetrics creates a valid metrics instance
	metrics := NewMetrics("test-service")

	if metrics == nil {
		t.Fatal("NewMetrics returned nil")
	}

	if metrics.RequestTotal == nil {
		t.Error("RequestTotal is nil")
	}

	if metrics.RequestDuration == nil {
		t.Error("RequestDuration is nil")
	}

	if metrics.TxPostedTotal == nil {
		t.Error("TxPostedTotal is nil")
	}

	if metrics.IdempotencyReplay == nil {
		t.Error("IdempotencyReplay is nil")
	}

	if metrics.DBErrorTotal == nil {
		t.Error("DBErrorTotal is nil")
	}

	// Test that domain counters can be incremented (should not panic)
	metrics.TxPostedTotal.Inc()
	metrics.IdempotencyReplay.Inc()
	metrics.DBErrorTotal.Inc()

	// Test that handler works
	handler := metrics.Handler()
	if handler == nil {
		t.Error("Handler() returned nil")
	}

	// Test that metrics can be incremented (should not panic)
	metrics.RequestTotal.WithLabelValues("TestMethod", "success").Inc()

	// Test that metrics can be observed (should not panic)
	metrics.RequestDuration.WithLabelValues("TestMethod", "success").Observe(0.5)
}

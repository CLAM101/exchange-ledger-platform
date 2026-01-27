package observability

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestNewLogger(t *testing.T) {
	t.Parallel()

	logger, err := NewLogger("test-service")
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	// Now we're using zap
	logger.Info("test message", zap.String("key", "value"))
}

func TestRequestIDContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	requestID := "req-123"

	// Add request ID to context
	ctx = WithRequestID(ctx, requestID)

	// Retrieve request ID
	got := GetRequestID(ctx)
	if got != requestID {
		t.Errorf("GetRequestID() = %v, want %v", got, requestID)
	}
}

func TestGetRequestIDEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got := GetRequestID(ctx)
	if got != "" {
		t.Errorf("GetRequestID() = %v, want empty string", got)
	}
}

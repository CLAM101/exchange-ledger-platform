package observability

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID returns a new context with the given request ID attached.
// The request ID can be retrieved later using GetRequestID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetRequestID retrieves the request ID from the context.
// Returns an empty string if no request ID is present.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// NewLogger creates a new structured zap logger configured for production use.
// The logger includes a "service" field with the provided service name for
// consistent identification across log aggregation systems.
func NewLogger(serviceName string) (*zap.Logger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	return logger.With(zap.String("service", serviceName)), nil
}

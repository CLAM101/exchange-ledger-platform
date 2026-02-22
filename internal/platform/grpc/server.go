package grpc

import (
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewServer creates a gRPC server pre-wired with recovery, logging, and
// metrics interceptors, and with server reflection enabled.
func NewServer(logger *zap.Logger, metrics *observability.Metrics) *grpc.Server {
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			RecoveryInterceptor(logger),
			LoggingInterceptor(logger),
			MetricsInterceptor(metrics),
		),
	)

	reflection.Register(srv)

	return srv
}

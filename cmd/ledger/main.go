// Package main is the entry point for the ledger service.
// The ledger service handles core double-entry accounting operations.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	platformgrpc "github.com/CLAM101/exchange-ledger-platform/internal/platform/grpc"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("failed to run ledger service: %v", err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger, err := observability.NewLogger("ledger")
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync() //nolint:errcheck // Sync errors are acceptable in defer
	}()

	metrics := observability.NewMetrics("ledger")

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())
		metricsPort := getEnv("METRICS_PORT", "9091")
		server := &http.Server{
			Addr:              ":" + metricsPort,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		logger.Info("metrics endpoint starting", zap.String("port", metricsPort))
		if serveErr := server.ListenAndServe(); serveErr != nil {
			logger.Error("metrics server failed", zap.Error(serveErr))
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("shutting down ledger service...")
		cancel()
	}()

	port := getEnv("GRPC_PORT", "9001")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := platformgrpc.NewServer(logger, metrics)

	// TODO: Register service implementations
	// ledgerpb.RegisterLedgerServiceServer(grpcServer, ledgerService)

	logger.Info("Ledger service listening", zap.String("port", port))

	// Start gRPC server
	errChan := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			errChan <- fmt.Errorf("failed to serve: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("context cancelled, shutting down")
		grpcServer.GracefulStop()
		return nil
	case err := <-errChan:
		logger.Error("failed to serve", zap.Error(err))
		return err
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

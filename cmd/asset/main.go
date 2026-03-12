// Package main is the entry point for the asset registry service.
// The asset service provides an in-memory catalogue of supported assets.
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

	"go.uber.org/zap"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/CLAM101/exchange-ledger-platform/internal/asset"
	platformgrpc "github.com/CLAM101/exchange-ledger-platform/internal/platform/grpc"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	assetv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/asset/v1"
)

const serviceName = "asset"

func main() {
	if err := run(); err != nil {
		log.Fatalf("failed to run asset service: %v", err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger, err := observability.NewLogger(serviceName)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync() //nolint:errcheck // Sync errors are acceptable in defer
	}()

	metrics := observability.NewMetrics(serviceName)

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())
		metricsPort := getEnv("METRICS_PORT", "9095")
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

	// Handle graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("shutting down asset service...")
		cancel()
	}()

	// Health service (no DB — asset is stateless).
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus(serviceName, healthpb.HealthCheckResponse_SERVING)

	port := getEnv("GRPC_PORT", "9004")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := platformgrpc.NewServer(logger, metrics, hs)

	registry := asset.NewInMemoryRegistry(asset.DefaultAssets())
	handler := asset.NewServer(registry, logger)
	assetv1.RegisterAssetServiceServer(grpcServer, handler)

	logger.Info("Asset service listening", zap.String("port", port))

	// Start gRPC server.
	errChan := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			errChan <- fmt.Errorf("failed to serve: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("context cancelled, shutting down")
		hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		hs.SetServingStatus(serviceName, healthpb.HealthCheckResponse_NOT_SERVING)
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

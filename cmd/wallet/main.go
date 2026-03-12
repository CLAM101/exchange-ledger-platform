// Package main is the entry point for the wallet service.
// The wallet service handles deposit and withdrawal orchestration.
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	platformgrpc "github.com/CLAM101/exchange-ledger-platform/internal/platform/grpc"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	"github.com/CLAM101/exchange-ledger-platform/internal/wallet"
	accountv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/account/v1"
	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"
	walletv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/wallet/v1"
)

const serviceName = "wallet"

func main() {
	if err := run(); err != nil {
		log.Fatalf("failed to run wallet service: %v", err)
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
		metricsPort := getEnv("METRICS_PORT", "9093")
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
		logger.Info("shutting down wallet service...")
		cancel()
	}()

	// Connect to Account service.
	accountAddr := getEnv("ACCOUNT_ADDR", "localhost:9002")
	accountConn, err := grpc.NewClient(accountAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connecting to account service: %w", err)
	}
	defer accountConn.Close() //nolint:errcheck // Best-effort close on shutdown
	logger.Info("connected to account service", zap.String("addr", accountAddr))

	// Connect to Ledger service.
	ledgerAddr := getEnv("LEDGER_ADDR", "localhost:9001")
	ledgerConn, err := grpc.NewClient(ledgerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connecting to ledger service: %w", err)
	}
	defer ledgerConn.Close() //nolint:errcheck // Best-effort close on shutdown
	logger.Info("connected to ledger service", zap.String("addr", ledgerAddr))

	// Health service (no DB checker — wallet is stateless).
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus(serviceName, healthpb.HealthCheckResponse_SERVING)

	port := getEnv("GRPC_PORT", "9003")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := platformgrpc.NewServer(logger, metrics, hs)

	accountClient := accountv1.NewAccountServiceClient(accountConn)
	ledgerClient := ledgerv1.NewLedgerServiceClient(ledgerConn)
	handler := wallet.NewServer(accountClient, ledgerClient, logger)
	walletv1.RegisterWalletServiceServer(grpcServer, handler)

	logger.Info("Wallet service listening", zap.String("port", port))

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

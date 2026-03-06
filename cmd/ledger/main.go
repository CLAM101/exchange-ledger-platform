// Package main is the entry point for the ledger service.
// The ledger service handles core double-entry accounting operations.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/CLAM101/exchange-ledger-platform/internal/ledger"
	platformgrpc "github.com/CLAM101/exchange-ledger-platform/internal/platform/grpc"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	ledgerv1 "github.com/CLAM101/exchange-ledger-platform/proto/gen/ledger/v1"
)

const serviceName = "ledger"

func main() {
	if err := run(); err != nil {
		log.Fatalf("failed to run ledger service: %v", err)
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

	// Connect to MySQL.
	db, err := sql.Open("mysql", buildDSN())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close() //nolint:errcheck // Best-effort close on shutdown

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if pingErr := db.PingContext(ctx); pingErr != nil {
		return fmt.Errorf("pinging database: %w", pingErr)
	}
	logger.Info("connected to database")

	// Health service.
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus(serviceName, healthpb.HealthCheckResponse_SERVING)

	dbChecker := platformgrpc.NewDBHealthChecker(db)
	go platformgrpc.WatchHealth(ctx, hs, serviceName, dbChecker, 10*time.Second, logger)

	repo := ledger.NewMySQLRepository(db, logger, metrics)
	handler := ledger.NewServer(repo, logger)

	port := getEnv("GRPC_PORT", "9001")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := platformgrpc.NewServer(logger, metrics, hs)
	ledgerv1.RegisterLedgerServiceServer(grpcServer, handler)

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
		hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		hs.SetServingStatus(serviceName, healthpb.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()
		return nil
	case err := <-errChan:
		logger.Error("failed to serve", zap.Error(err))
		return err
	}
}

func buildDSN() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "ledger_user")
	pass := getEnv("DB_PASSWORD", "ledger_pass")
	name := getEnv("DB_NAME", "ledger")

	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		user, pass, host, port, name)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

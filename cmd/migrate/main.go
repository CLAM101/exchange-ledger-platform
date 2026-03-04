// Package main provides the database migration CLI for the exchange-ledger-platform.
//
// Usage:
//
//	go run ./cmd/migrate up      # Apply all pending migrations
//	go run ./cmd/migrate down    # Roll back the last migration
//	go run ./cmd/migrate version # Print current migration version
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	migrations "github.com/CLAM101/exchange-ledger-platform/migrations"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: migrate <up|down|version>")
	}
	cmd := os.Args[1]

	dsn := buildDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close() //nolint:errcheck // Best-effort close on shutdown

	if err = db.Ping(); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}

	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return fmt.Errorf("creating migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}

	switch cmd {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration up: %w", err)
		}
		log.Println("migrations applied successfully")
	case "down":
		if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration down: %w", err)
		}
		log.Println("migration rolled back successfully")
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			return fmt.Errorf("getting version: %w", err)
		}
		log.Printf("version: %d, dirty: %v", version, dirty)
	default:
		return fmt.Errorf("unknown command: %s (expected up, down, or version)", cmd)
	}

	return nil
}

func buildDSN() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "ledger_user")
	pass := getEnv("DB_PASSWORD", "ledger_pass")
	name := getEnv("DB_NAME", "ledger")

	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true",
		user, pass, host, port, name)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

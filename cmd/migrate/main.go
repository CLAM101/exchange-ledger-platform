// Package main provides the database migration CLI for the exchange-ledger-platform.
//
// Each service owns its own database and migrations. Specify the service name
// to select which migration set to apply.
//
// Usage:
//
//	go run ./cmd/migrate ledger up      # Apply ledger migrations
//	go run ./cmd/migrate ledger down    # Roll back the last ledger migration
//	go run ./cmd/migrate account up     # Apply account migrations
//	go run ./cmd/migrate account version
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	accountmigrations "github.com/CLAM101/exchange-ledger-platform/migrations/account"
	ledgermigrations "github.com/CLAM101/exchange-ledger-platform/migrations/ledger"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: migrate <service> <up|down|version>\n  services: ledger, account")
	}
	service := os.Args[1]
	cmd := os.Args[2]

	migrationFS, err := migrationSource(service)
	if err != nil {
		return err
	}

	dsn := buildDSN(service)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close() //nolint:errcheck // Best-effort close on shutdown

	if err = db.Ping(); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}

	source, err := iofs.New(migrationFS, ".")
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
		log.Printf("[%s] migrations applied successfully", service)
	case "down":
		if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migration down: %w", err)
		}
		log.Printf("[%s] migration rolled back successfully", service)
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			return fmt.Errorf("getting version: %w", err)
		}
		log.Printf("[%s] version: %d, dirty: %v", service, version, dirty)
	default:
		return fmt.Errorf("unknown command: %s (expected up, down, or version)", cmd)
	}

	return nil
}

// migrationSource returns the embedded filesystem for the given service.
func migrationSource(service string) (fs.FS, error) {
	switch service {
	case "ledger":
		return ledgermigrations.FS, nil
	case "account":
		return accountmigrations.FS, nil
	default:
		return nil, fmt.Errorf("unknown service: %s (expected ledger or account)", service)
	}
}

type dsnDefaults struct {
	user, pass, name string
}

var serviceDefaults = map[string]dsnDefaults{
	"ledger":  {user: "ledger_user", pass: "ledger_pass", name: "ledger"},
	"account": {user: "account_user", pass: "account_pass", name: "account"},
}

func buildDSN(service string) string {
	d := serviceDefaults[service] // always valid; migrationSource validates first
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", d.user)
	pass := getEnv("DB_PASSWORD", d.pass)
	name := getEnv("DB_NAME", d.name)

	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true",
		user, pass, host, port, name)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

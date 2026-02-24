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
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down|version>")
	}
	cmd := os.Args[1]

	dsn := buildDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("pinging database: %v", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("creating migration source: %v", err)
	}

	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		log.Fatalf("creating migration driver: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		log.Fatalf("creating migrate instance: %v", err)
	}

	switch cmd {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migration up: %v", err)
		}
		log.Println("migrations applied successfully")
	case "down":
		if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migration down: %v", err)
		}
		log.Println("migration rolled back successfully")
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("getting version: %v", err)
		}
		log.Printf("version: %d, dirty: %v", version, dirty)
	default:
		log.Fatalf("unknown command: %s (expected up, down, or version)", cmd)
	}
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

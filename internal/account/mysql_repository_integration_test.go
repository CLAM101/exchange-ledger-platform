//go:build integration

package account_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	mysqlmigrate "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"

	"github.com/CLAM101/exchange-ledger-platform/internal/account"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	accountmigrations "github.com/CLAM101/exchange-ledger-platform/migrations/account"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	dsn := buildTestDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("opening test database: %v", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("pinging test database: %v", err)
	}

	if err := runMigrations(db); err != nil {
		log.Fatalf("running migrations: %v", err)
	}

	testDB = db

	code := m.Run()

	db.Close()
	os.Exit(code)
}

func buildTestDSN() string {
	host := envOrDefault("DB_HOST", "localhost")
	port := envOrDefault("DB_PORT", "3306")
	user := envOrDefault("DB_USER", "account_user")
	pass := envOrDefault("DB_PASSWORD", "account_pass")
	name := envOrDefault("DB_NAME", "account")
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true", user, pass, host, port, name)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runMigrations(db *sql.DB) error {
	source, err := iofs.New(accountmigrations.FS, ".")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}
	driver, err := mysqlmigrate.WithInstance(db, &mysqlmigrate.Config{})
	if err != nil {
		return fmt.Errorf("creating migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

func truncateTables(t *testing.T) {
	t.Helper()
	// Order matters: user_asset_accounts has FK to users.
	for _, stmt := range []string{
		"DELETE FROM user_asset_accounts",
		"DELETE FROM users",
	} {
		if _, err := testDB.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("truncating tables: %v", err)
		}
	}
}

func newRepo(t *testing.T) *account.MySQLRepository {
	t.Helper()
	return account.NewMySQLRepository(testDB, zap.NewNop(), observability.NewTestMetrics())
}

// --- Test: CreateUser success ---

func TestCreateUser_Success(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	user := account.User{
		Email:          "alice@example.com",
		IdempotencyKey: "create-user-1",
	}

	result, err := repo.CreateUser(context.Background(), user)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if result.ID == "" {
		t.Error("expected non-empty user ID")
	}
	if result.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", result.Email, "alice@example.com")
	}
	if result.IdempotencyKey != "create-user-1" {
		t.Errorf("idempotency key = %q, want %q", result.IdempotencyKey, "create-user-1")
	}
	if result.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

// --- Test: CreateUser idempotency replay ---

func TestCreateUser_IdempotencyReplay(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	user := account.User{
		Email:          "bob@example.com",
		IdempotencyKey: "create-user-replay",
	}

	first, err := repo.CreateUser(context.Background(), user)
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}

	second, err := repo.CreateUser(context.Background(), user)
	if err != nil {
		t.Fatalf("second CreateUser: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("user IDs differ: %s vs %s", first.ID, second.ID)
	}

	// Only one row exists.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM users WHERE idempotency_key = ?`,
		"create-user-replay",
	).Scan(&count); err != nil {
		t.Fatalf("counting users: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1", count)
	}
}

// --- Test: CreateUser duplicate email ---

func TestCreateUser_DuplicateEmail(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	user1 := account.User{
		Email:          "carol@example.com",
		IdempotencyKey: "create-user-email-1",
	}
	if _, err := repo.CreateUser(context.Background(), user1); err != nil {
		t.Fatalf("CreateUser first: %v", err)
	}

	user2 := account.User{
		Email:          "carol@example.com",
		IdempotencyKey: "create-user-email-2",
	}
	_, err := repo.CreateUser(context.Background(), user2)
	if !errors.Is(err, account.ErrEmailExists) {
		t.Fatalf("expected ErrEmailExists, got: %v", err)
	}
}

// --- Test: GetUser found ---

func TestGetUser_Found(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	created, err := repo.CreateUser(context.Background(), account.User{
		Email:          "dave@example.com",
		IdempotencyKey: "get-user-found",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	fetched, err := repo.GetUser(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("ID = %q, want %q", fetched.ID, created.ID)
	}
	if fetched.Email != "dave@example.com" {
		t.Errorf("email = %q, want %q", fetched.Email, "dave@example.com")
	}
}

// --- Test: GetUser not found ---

func TestGetUser_NotFound(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	_, err := repo.GetUser(context.Background(), "nonexistent-id")
	if !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// --- Test: GetUserByEmail found ---

func TestGetUserByEmail_Found(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	created, err := repo.CreateUser(context.Background(), account.User{
		Email:          "eve@example.com",
		IdempotencyKey: "get-by-email-found",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	fetched, err := repo.GetUserByEmail(context.Background(), "eve@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("ID = %q, want %q", fetched.ID, created.ID)
	}
}

// --- Test: GetUserByEmail not found ---

func TestGetUserByEmail_NotFound(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	_, err := repo.GetUserByEmail(context.Background(), "nobody@example.com")
	if !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// --- Test: LinkAssetAccount ---

func TestLinkAssetAccount(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	user, err := repo.CreateUser(context.Background(), account.User{
		Email:          "frank@example.com",
		IdempotencyKey: "link-asset-1",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	ua := account.UserAssetAccount{
		UserID:          user.ID,
		Asset:           "BTC",
		LedgerAccountID: "ledger-acc-btc-001",
	}

	result, err := repo.LinkAssetAccount(context.Background(), ua)
	if err != nil {
		t.Fatalf("LinkAssetAccount: %v", err)
	}

	if result.UserID != user.ID {
		t.Errorf("UserID = %q, want %q", result.UserID, user.ID)
	}
	if result.Asset != "BTC" {
		t.Errorf("Asset = %q, want %q", result.Asset, "BTC")
	}
	if result.LedgerAccountID != "ledger-acc-btc-001" {
		t.Errorf("LedgerAccountID = %q, want %q", result.LedgerAccountID, "ledger-acc-btc-001")
	}
	if result.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}

	// Linking again returns the existing mapping.
	second, err := repo.LinkAssetAccount(context.Background(), ua)
	if err != nil {
		t.Fatalf("LinkAssetAccount replay: %v", err)
	}
	if second.LedgerAccountID != "ledger-acc-btc-001" {
		t.Errorf("replay LedgerAccountID = %q, want %q", second.LedgerAccountID, "ledger-acc-btc-001")
	}
}

// --- Test: GetLedgerAccountID found ---

func TestGetLedgerAccountID_Found(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	user, err := repo.CreateUser(context.Background(), account.User{
		Email:          "grace@example.com",
		IdempotencyKey: "get-ledger-found",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	_, err = repo.LinkAssetAccount(context.Background(), account.UserAssetAccount{
		UserID:          user.ID,
		Asset:           "ETH",
		LedgerAccountID: "ledger-acc-eth-001",
	})
	if err != nil {
		t.Fatalf("LinkAssetAccount: %v", err)
	}

	ledgerID, err := repo.GetLedgerAccountID(context.Background(), user.ID, "ETH")
	if err != nil {
		t.Fatalf("GetLedgerAccountID: %v", err)
	}
	if ledgerID != "ledger-acc-eth-001" {
		t.Errorf("ledger ID = %q, want %q", ledgerID, "ledger-acc-eth-001")
	}
}

// --- Test: GetLedgerAccountID not found ---

func TestGetLedgerAccountID_NotFound(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	_, err := repo.GetLedgerAccountID(context.Background(), "nonexistent-user", "BTC")
	if !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

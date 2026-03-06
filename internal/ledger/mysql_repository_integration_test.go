//go:build integration

package ledger_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	mysqlmigrate "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"

	"github.com/CLAM101/exchange-ledger-platform/internal/ledger"
	"github.com/CLAM101/exchange-ledger-platform/internal/platform/observability"
	migrations "github.com/CLAM101/exchange-ledger-platform/migrations"
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
	user := envOrDefault("DB_USER", "ledger_user")
	pass := envOrDefault("DB_PASSWORD", "ledger_pass")
	name := envOrDefault("DB_NAME", "ledger")
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, name)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runMigrations(db *sql.DB) error {
	source, err := iofs.New(migrations.FS, ".")
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
	// Order matters: entries has FK to transactions.
	for _, stmt := range []string{
		"DELETE FROM ledger_entries",
		"DELETE FROM ledger_balances",
		"DELETE FROM ledger_transactions",
	} {
		if _, err := testDB.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("truncating tables: %v", err)
		}
	}
}

func seedBalance(t *testing.T, accountID ledger.AccountID, asset ledger.Asset, amount ledger.Amount) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO ledger_balances (account_id, asset, balance) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE balance = balance + VALUES(balance)`,
		string(accountID), string(asset), int64(amount),
	)
	if err != nil {
		t.Fatalf("seeding balance for %s/%s: %v", accountID, asset, err)
	}
}

func newRepo(t *testing.T) *ledger.MySQLRepository {
	t.Helper()
	return ledger.NewMySQLRepository(testDB, zap.NewNop(), observability.NewTestMetrics())
}

// --- Test: Successful transaction posting ---

func TestPostTransaction_Success(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	seedBalance(t, "acc_source", "BTC", 10000)

	tx := ledger.Transaction{
		IdempotencyKey: "idem-success-1",
		Postings: []ledger.Posting{
			{AccountID: "acc_source", Asset: "BTC", Amount: -5000},
			{AccountID: "acc_dest", Asset: "BTC", Amount: 5000},
		},
	}

	result, err := repo.PostTransaction(context.Background(), tx)
	if err != nil {
		t.Fatalf("PostTransaction: %v", err)
	}

	if result.ID == "" {
		t.Error("expected non-empty tx ID")
	}
	if result.IdempotencyKey != "idem-success-1" {
		t.Errorf("idempotency key = %q, want %q", result.IdempotencyKey, "idem-success-1")
	}
	if result.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
	if len(result.Postings) != 2 {
		t.Fatalf("expected 2 postings, got %d", len(result.Postings))
	}

	// Verify balances were updated.
	srcBal, err := repo.GetBalance(context.Background(), "acc_source", "BTC")
	if err != nil {
		t.Fatalf("GetBalance source: %v", err)
	}
	if srcBal != 5000 {
		t.Errorf("source balance = %d, want 5000", srcBal)
	}

	dstBal, err := repo.GetBalance(context.Background(), "acc_dest", "BTC")
	if err != nil {
		t.Fatalf("GetBalance dest: %v", err)
	}
	if dstBal != 5000 {
		t.Errorf("dest balance = %d, want 5000", dstBal)
	}
}

// --- Test: Idempotency replay ---

func TestPostTransaction_IdempotencyReplay(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	seedBalance(t, "acc_src", "BTC", 10000)

	tx := ledger.Transaction{
		IdempotencyKey: "idem-replay-1",
		Postings: []ledger.Posting{
			{AccountID: "acc_src", Asset: "BTC", Amount: -1000},
			{AccountID: "acc_dst", Asset: "BTC", Amount: 1000},
		},
	}

	first, err := repo.PostTransaction(context.Background(), tx)
	if err != nil {
		t.Fatalf("first post: %v", err)
	}

	second, err := repo.PostTransaction(context.Background(), tx)
	if err != nil {
		t.Fatalf("second post: %v", err)
	}

	// Same transaction ID returned.
	if first.ID != second.ID {
		t.Errorf("tx IDs differ: %s vs %s", first.ID, second.ID)
	}

	// Balance only debited once.
	bal, err := repo.GetBalance(context.Background(), "acc_src", "BTC")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 9000 {
		t.Errorf("balance = %d, want 9000 (debited only once)", bal)
	}

	// Only one transaction row exists.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM ledger_transactions WHERE idempotency_key = ?`,
		"idem-replay-1",
	).Scan(&count); err != nil {
		t.Fatalf("counting transactions: %v", err)
	}
	if count != 1 {
		t.Errorf("transaction count = %d, want 1", count)
	}
}

// --- Test: Overdraft prevention ---

func TestPostTransaction_Overdraft(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	seedBalance(t, "acc_poor", "BTC", 500)
	seedBalance(t, "acc_rich", "BTC", 0)

	tx := ledger.Transaction{
		IdempotencyKey: "idem-overdraft-1",
		Postings: []ledger.Posting{
			{AccountID: "acc_poor", Asset: "BTC", Amount: -1000},
			{AccountID: "acc_rich", Asset: "BTC", Amount: 1000},
		},
	}

	_, err := repo.PostTransaction(context.Background(), tx)
	if !errors.Is(err, ledger.ErrOverdraft) {
		t.Fatalf("expected ErrOverdraft, got: %v", err)
	}

	// Verify no rows were created (atomic rollback).
	var txCount int
	if countErr := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM ledger_transactions WHERE idempotency_key = ?`,
		"idem-overdraft-1",
	).Scan(&txCount); countErr != nil {
		t.Fatalf("counting transactions: %v", countErr)
	}
	if txCount != 0 {
		t.Errorf("transaction count = %d, want 0 (rolled back)", txCount)
	}

	// Balance unchanged.
	bal, err := repo.GetBalance(context.Background(), "acc_poor", "BTC")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 500 {
		t.Errorf("balance = %d, want 500 (unchanged)", bal)
	}
}

// --- Test: Concurrent debits (race condition prevention) ---

func TestPostTransaction_ConcurrentDebits(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	// Account has exactly 1000 — only one of two 1000-debit transactions can succeed.
	seedBalance(t, "acc_shared", "BTC", 1000)
	seedBalance(t, "acc_dest_0", "BTC", 0)
	seedBalance(t, "acc_dest_1", "BTC", 0)

	var wg sync.WaitGroup
	results := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tx := ledger.Transaction{
				IdempotencyKey: fmt.Sprintf("idem-concurrent-%d", idx),
				Postings: []ledger.Posting{
					{AccountID: "acc_shared", Asset: "BTC", Amount: -1000},
					{AccountID: ledger.AccountID(fmt.Sprintf("acc_dest_%d", idx)), Asset: "BTC", Amount: 1000},
				},
			}
			_, err := repo.PostTransaction(context.Background(), tx)
			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	var successes, overdrafts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ledger.ErrOverdraft):
			overdrafts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successes != 1 {
		t.Errorf("successes = %d, want exactly 1", successes)
	}
	if overdrafts != 1 {
		t.Errorf("overdrafts = %d, want exactly 1", overdrafts)
	}

	// Balance must be exactly 0 — never negative.
	bal, err := repo.GetBalance(context.Background(), "acc_shared", "BTC")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 0 {
		t.Errorf("balance = %d, want 0 (never negative)", bal)
	}
}

// --- Test: GetTransaction not found ---

func TestGetTransaction_NotFound(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	_, err := repo.GetTransaction(context.Background(), "nonexistent-key")
	if !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// --- Test: GetTransaction found ---

func TestGetTransaction_Found(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	seedBalance(t, "acc_a", "BTC", 5000)
	seedBalance(t, "acc_b", "BTC", 0)

	tx := ledger.Transaction{
		IdempotencyKey: "idem-get-1",
		Postings: []ledger.Posting{
			{AccountID: "acc_a", Asset: "BTC", Amount: -1000},
			{AccountID: "acc_b", Asset: "BTC", Amount: 1000},
		},
	}

	posted, err := repo.PostTransaction(context.Background(), tx)
	if err != nil {
		t.Fatalf("PostTransaction: %v", err)
	}

	fetched, err := repo.GetTransaction(context.Background(), "idem-get-1")
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if fetched.ID != posted.ID {
		t.Errorf("ID = %q, want %q", fetched.ID, posted.ID)
	}
	if len(fetched.Postings) != 2 {
		t.Errorf("postings count = %d, want 2", len(fetched.Postings))
	}
}

// --- Test: ListEntries ---

func TestListEntries_Empty(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	entries, err := repo.ListEntries(context.Background(), "no-such-acc", "BTC", 0, 10)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestListEntries_SinglePage(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	seedBalance(t, "acc_list", "BTC", 10000)

	// Post two transactions so acc_list has 2 entries.
	for i := 0; i < 2; i++ {
		tx := ledger.Transaction{
			IdempotencyKey: fmt.Sprintf("idem-list-%d", i),
			Postings: []ledger.Posting{
				{AccountID: "acc_list", Asset: "BTC", Amount: -1000},
				{AccountID: "acc_other", Asset: "BTC", Amount: 1000},
			},
		}
		if _, err := repo.PostTransaction(context.Background(), tx); err != nil {
			t.Fatalf("PostTransaction %d: %v", i, err)
		}
	}

	entries, err := repo.ListEntries(context.Background(), "acc_list", "BTC", 0, 10)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Entries are ordered by entry_id ascending.
	if entries[0].EntryID >= entries[1].EntryID {
		t.Errorf("entries not ordered: %d >= %d", entries[0].EntryID, entries[1].EntryID)
	}
	for _, e := range entries {
		if e.AccountID != "acc_list" {
			t.Errorf("unexpected account_id: %s", e.AccountID)
		}
		if e.Asset != "BTC" {
			t.Errorf("unexpected asset: %s", e.Asset)
		}
		if e.Amount != -1000 {
			t.Errorf("unexpected amount: %d", e.Amount)
		}
	}
}

func TestListEntries_Pagination(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	seedBalance(t, "acc_page", "BTC", 100000)

	// Create 5 entries for acc_page.
	for i := 0; i < 5; i++ {
		tx := ledger.Transaction{
			IdempotencyKey: fmt.Sprintf("idem-page-%d", i),
			Postings: []ledger.Posting{
				{AccountID: "acc_page", Asset: "BTC", Amount: -100},
				{AccountID: "acc_sink", Asset: "BTC", Amount: 100},
			},
		}
		if _, err := repo.PostTransaction(context.Background(), tx); err != nil {
			t.Fatalf("PostTransaction %d: %v", i, err)
		}
	}

	// Page 1: limit 2
	page1, err := repo.ListEntries(context.Background(), "acc_page", "BTC", 0, 2)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1: expected 2 entries, got %d", len(page1))
	}

	// Page 2: cursor = last entry_id from page 1
	page2, err := repo.ListEntries(context.Background(), "acc_page", "BTC", page1[1].EntryID, 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page 2: expected 2 entries, got %d", len(page2))
	}

	// Page 3: should have 1 remaining entry
	page3, err := repo.ListEntries(context.Background(), "acc_page", "BTC", page2[1].EntryID, 2)
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page 3: expected 1 entry, got %d", len(page3))
	}

	// All entry IDs should be unique and ascending across pages.
	allIDs := make([]int64, 0, 5)
	for _, e := range page1 {
		allIDs = append(allIDs, e.EntryID)
	}
	for _, e := range page2 {
		allIDs = append(allIDs, e.EntryID)
	}
	for _, e := range page3 {
		allIDs = append(allIDs, e.EntryID)
	}
	for i := 1; i < len(allIDs); i++ {
		if allIDs[i] <= allIDs[i-1] {
			t.Errorf("entry IDs not strictly ascending: %v", allIDs)
			break
		}
	}
}

func TestListEntries_AssetFilter(t *testing.T) {
	truncateTables(t)
	repo := newRepo(t)

	seedBalance(t, "acc_multi", "BTC", 10000)
	seedBalance(t, "acc_multi", "ETH", 10000)

	// Post BTC transaction.
	txBTC := ledger.Transaction{
		IdempotencyKey: "idem-asset-btc",
		Postings: []ledger.Posting{
			{AccountID: "acc_multi", Asset: "BTC", Amount: -500},
			{AccountID: "acc_other2", Asset: "BTC", Amount: 500},
		},
	}
	if _, err := repo.PostTransaction(context.Background(), txBTC); err != nil {
		t.Fatalf("PostTransaction BTC: %v", err)
	}

	// Post ETH transaction.
	txETH := ledger.Transaction{
		IdempotencyKey: "idem-asset-eth",
		Postings: []ledger.Posting{
			{AccountID: "acc_multi", Asset: "ETH", Amount: -300},
			{AccountID: "acc_other3", Asset: "ETH", Amount: 300},
		},
	}
	if _, err := repo.PostTransaction(context.Background(), txETH); err != nil {
		t.Fatalf("PostTransaction ETH: %v", err)
	}

	// List only BTC entries for acc_multi.
	entries, err := repo.ListEntries(context.Background(), "acc_multi", "BTC", 0, 10)
	if err != nil {
		t.Fatalf("ListEntries BTC: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 BTC entry, got %d", len(entries))
	}
	if entries[0].Asset != "BTC" {
		t.Errorf("expected BTC, got %s", entries[0].Asset)
	}
}

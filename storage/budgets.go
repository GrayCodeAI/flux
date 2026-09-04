package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// Budget sentinel errors. These mirror the client package's errors by message
// so callers can match on either; the BudgetProvider depends only on the
// structural method set, not on these specific values.
var (
	ErrBudgetExceeded    = errors.New("graycode-router: virtual key budget exceeded")
	ErrUnknownVirtualKey = errors.New("graycode-router: unknown virtual key")
)

// VirtualKey is a logical key that maps to a real provider key and carries a
// spend budget.
type VirtualKey struct {
	ID        string
	Name      string
	Provider  string
	LimitUSD  float64 // <= 0 means unlimited
	UsedUSD   float64
	TokensIn  int
	TokensOut int
	CreatedAt time.Time
}

// BudgetStore is a SQLite-backed store for virtual keys, their budgets, and a
// per-request cost ledger. It satisfies the client.BudgetStore interface
// structurally (CheckBudget + RecordUsage) without importing the client package.
//
// It is safe for concurrent use (single underlying connection, like SQLiteStore).
type BudgetStore struct {
	db *sql.DB
}

// OpenBudgetStore opens (or creates) a SQLite database at path and ensures the
// budget schema exists. The database file is set to 0o600 because the
// virtual_key_secrets table stores provider API keys.
func OpenBudgetStore(path string) (*BudgetStore, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("storage: open budget store %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	s := &BudgetStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Restrict file permissions: the database stores plaintext provider API
	// keys in virtual_key_secrets. The file is created by the SQLite driver
	// with the process umask, which may be 0o644 on some systems.
	// In WAL mode SQLite also creates <path>-wal and <path>-shm sidecar
	// files; the WAL holds uncheckpointed pages (including plaintext keys)
	// so it must be tightened too. Errors are ignored for sidecars that
	// don't exist yet.
	_ = os.Chmod(path, 0o600)
	_ = os.Chmod(path+"-wal", 0o600)
	_ = os.Chmod(path+"-shm", 0o600)
	return s, nil
}

// Close releases the underlying database.
func (s *BudgetStore) Close() error { return s.db.Close() }

func (s *BudgetStore) migrate() error {
	_, err := s.db.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS virtual_keys (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			provider    TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS virtual_key_secrets (
			virtual_key_id TEXT NOT NULL REFERENCES virtual_keys(id) ON DELETE CASCADE,
			provider       TEXT NOT NULL,
			api_key        TEXT NOT NULL,
			PRIMARY KEY (virtual_key_id, provider)
		);

		CREATE TABLE IF NOT EXISTS key_budgets (
			virtual_key_id TEXT PRIMARY KEY REFERENCES virtual_keys(id) ON DELETE CASCADE,
			limit_usd      REAL NOT NULL DEFAULT 0,
			used_usd       REAL NOT NULL DEFAULT 0,
			tokens_in      INTEGER NOT NULL DEFAULT 0,
			tokens_out     INTEGER NOT NULL DEFAULT 0,
			updated_at     TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS request_costs (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			virtual_key_id TEXT NOT NULL REFERENCES virtual_keys(id) ON DELETE CASCADE,
			model          TEXT NOT NULL DEFAULT '',
			tokens_in      INTEGER NOT NULL DEFAULT 0,
			tokens_out     INTEGER NOT NULL DEFAULT 0,
			cost_usd       REAL NOT NULL DEFAULT 0,
			created_at     TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_request_costs_key ON request_costs(virtual_key_id);
	`)
	return err
}

// CreateVirtualKey inserts a virtual key with the given budget. A non-positive
// limitUSD means unlimited spend.
func (s *BudgetStore) CreateVirtualKey(ctx context.Context, id, name, provider string, limitUSD float64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO virtual_keys (id, name, provider, created_at) VALUES (?, ?, ?, ?)`,
		id, name, provider, now); err != nil {
		return fmt.Errorf("storage: create virtual key: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO key_budgets (virtual_key_id, limit_usd, used_usd, updated_at) VALUES (?, ?, 0, ?)`,
		id, limitUSD, now); err != nil {
		return fmt.Errorf("storage: create budget: %w", err)
	}
	return tx.Commit()
}

// SetProviderSecret associates a real provider API key with a virtual key.
func (s *BudgetStore) SetProviderSecret(ctx context.Context, virtualKey, provider, apiKey string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO virtual_key_secrets (virtual_key_id, provider, api_key) VALUES (?, ?, ?)
		 ON CONFLICT(virtual_key_id, provider) DO UPDATE SET api_key = excluded.api_key`,
		virtualKey, provider, apiKey)
	return err
}

// ProviderSecret returns the real API key for a virtual key + provider.
func (s *BudgetStore) ProviderSecret(ctx context.Context, virtualKey, provider string) (string, error) {
	var key string
	err := s.db.QueryRowContext(ctx,
		`SELECT api_key FROM virtual_key_secrets WHERE virtual_key_id = ? AND provider = ?`,
		virtualKey, provider).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUnknownVirtualKey
	}
	return key, err
}

// CheckBudget implements the client.BudgetStore contract: it returns
// ErrBudgetExceeded if charging estCostUSD would exceed the key's limit,
// ErrUnknownVirtualKey if the key is unknown, or nil otherwise.
func (s *BudgetStore) CheckBudget(ctx context.Context, virtualKey string, estCostUSD float64) error {
	var limit, used float64
	err := s.db.QueryRowContext(ctx,
		`SELECT limit_usd, used_usd FROM key_budgets WHERE virtual_key_id = ?`,
		virtualKey).Scan(&limit, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %q", ErrUnknownVirtualKey, virtualKey)
	}
	if err != nil {
		return err
	}
	if limit <= 0 {
		return nil // unlimited
	}
	if used+estCostUSD > limit {
		return fmt.Errorf("%w: %q (used $%.4f + est $%.4f > limit $%.4f)",
			ErrBudgetExceeded, virtualKey, used, estCostUSD, limit)
	}
	return nil
}

// RecordUsage implements the client.BudgetStore contract: it appends a ledger
// row and increments the running totals atomically.
func (s *BudgetStore) RecordUsage(ctx context.Context, virtualKey string, costUSD float64, tokensIn, tokensOut int) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE key_budgets
		   SET used_usd = used_usd + ?, tokens_in = tokens_in + ?, tokens_out = tokens_out + ?, updated_at = ?
		 WHERE virtual_key_id = ?`,
		costUSD, tokensIn, tokensOut, now, virtualKey)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %q", ErrUnknownVirtualKey, virtualKey)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO request_costs (virtual_key_id, model, tokens_in, tokens_out, cost_usd, created_at)
		 VALUES (?, '', ?, ?, ?, ?)`,
		virtualKey, tokensIn, tokensOut, costUSD, now); err != nil {
		return err
	}
	return tx.Commit()
}

// Get returns the current state of a virtual key.
func (s *BudgetStore) Get(ctx context.Context, virtualKey string) (*VirtualKey, error) {
	vk := &VirtualKey{ID: virtualKey}
	var created string
	err := s.db.QueryRowContext(ctx,
		`SELECT k.name, k.provider, k.created_at, b.limit_usd, b.used_usd, b.tokens_in, b.tokens_out
		   FROM virtual_keys k JOIN key_budgets b ON b.virtual_key_id = k.id
		  WHERE k.id = ?`, virtualKey).
		Scan(&vk.Name, &vk.Provider, &created, &vk.LimitUSD, &vk.UsedUSD, &vk.TokensIn, &vk.TokensOut)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnknownVirtualKey
	}
	if err != nil {
		return nil, err
	}
	vk.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return vk, nil
}

package repository

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var registerOnce sync.Once

// registerNowFunc registers SQL functions required by the production code
// on a custom SQLite driver so that tests work with in-memory SQLite:
//   - gen_random_uuid() — used in Wallet.ID default expression
//   - now() — used in UpdateBalance / UpdateStatus timestamps
func registerNowFunc() {
	registerOnce.Do(func() {
		sql.Register("sqlite3_with_now", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.RegisterFunc("gen_random_uuid", func() string {
					return uuid.New().String()
				}, true); err != nil {
					return err
				}
				return conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02T15:04:05Z")
				}, true)
			},
		})
	})
}

// setupTestDB creates an in-memory SQLite database with tables equivalent to
// the domain model schemas (manually created because GORM AutoMigrate generates
// Postgres-specific DDL like DEFAULT gen_random_uuid() and FOR UPDATE).
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerNowFunc()
	db, err := gorm.Open(sqlite.New(sqlite.Config{
		DriverName: "sqlite3_with_now",
		DSN:        ":memory:",
	}), &gorm.Config{})
	require.NoError(t, err)

	err = db.Exec(`CREATE TABLE wallets (
		id text PRIMARY KEY,
		owner_id text NOT NULL,
		currency text NOT NULL,
		balance text NOT NULL DEFAULT '0.00',
		status text NOT NULL DEFAULT 'ACTIVE',
		created_at datetime,
		updated_at datetime
	)`).Error
	require.NoError(t, err)
	err = db.Exec(`CREATE INDEX idx_wallets_owner_id ON wallets(owner_id)`).Error
	require.NoError(t, err)

	err = db.Exec(`CREATE TABLE ledger_entries (
		entry_id text PRIMARY KEY,
		wallet_id text NOT NULL,
		entry_type text NOT NULL,
		amount text NOT NULL,
		currency text NOT NULL,
		balance_after text NOT NULL,
		reference_id text,
		idempotency_key text NOT NULL UNIQUE,
		created_at datetime
	)`).Error
	require.NoError(t, err)
	err = db.Exec(`CREATE INDEX idx_ledger_wallet_created ON ledger_entries(wallet_id, created_at)`).Error
	require.NoError(t, err)

	return db
}

func TestWalletRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	w := &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user-1",
		Currency: "USD",
		Balance:  "0.00",
		Status:   domain.WalletStatusActive,
	}

	err := repo.Create(db, w)
	require.NoError(t, err)

	saved, err := repo.FindByID(w.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, w.ID, saved.ID)
	assert.Equal(t, "user-1", saved.OwnerID)
	assert.Equal(t, "USD", saved.Currency)
	assert.Equal(t, "0.00", saved.Balance)
	assert.Equal(t, domain.WalletStatusActive, saved.Status)
	assert.NotZero(t, saved.CreatedAt)
	assert.NotZero(t, saved.UpdatedAt)
}

func TestWalletRepository_FindByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	// Create a wallet first
	w := &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user-1",
		Currency: "USD",
		Balance:  "0.00",
		Status:   domain.WalletStatusActive,
	}
	err := repo.Create(db, w)
	require.NoError(t, err)

	// Found — should return the wallet
	saved, err := repo.FindByID(w.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, w.ID, saved.ID)
	assert.Equal(t, w.OwnerID, saved.OwnerID)

	// Not found — should return ErrNotFound
	_, err = repo.FindByID(uuid.New())
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestWalletRepository_FindByOwnerAndCurrency(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	w := &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user-1",
		Currency: "USD",
		Balance:  "0.00",
		Status:   domain.WalletStatusActive,
	}
	err := repo.Create(db, w)
	require.NoError(t, err)

	// Found — existing owner and currency
	saved, err := repo.FindByOwnerAndCurrency("user-1", "USD")
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, w.ID, saved.ID)

	// Not found — different currency for the same owner
	notFound, err := repo.FindByOwnerAndCurrency("user-1", "EUR")
	require.NoError(t, err)
	assert.Nil(t, notFound)

	// Not found — completely non-existent owner
	notFound, err = repo.FindByOwnerAndCurrency("nonexistent", "USD")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestWalletRepository_FindByIDForUpdate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	w := &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user-1",
		Currency: "USD",
		Balance:  "100.00",
		Status:   domain.WalletStatusActive,
	}
	err := repo.Create(db, w)
	require.NoError(t, err)

	tx := db.Begin()
	defer tx.Rollback()

	saved, err := repo.FindByIDForUpdate(tx, w.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, w.ID, saved.ID)
	assert.Equal(t, "100.00", saved.Balance)
	assert.Equal(t, domain.WalletStatusActive, saved.Status)
}

func TestWalletRepository_UpdateBalance(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	w := &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user-1",
		Currency: "USD",
		Balance:  "0.00",
		Status:   domain.WalletStatusActive,
	}
	err := repo.Create(db, w)
	require.NoError(t, err)

	tx := db.Begin()
	err = repo.UpdateBalance(tx, w.ID, "250.00")
	require.NoError(t, err)
	err = tx.Commit().Error
	require.NoError(t, err)

	saved, err := repo.FindByID(w.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "250.00", saved.Balance)
}

func TestWalletRepository_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	w := &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user-1",
		Currency: "USD",
		Balance:  "0.00",
		Status:   domain.WalletStatusActive,
	}
	err := repo.Create(db, w)
	require.NoError(t, err)

	tx := db.Begin()
	err = repo.UpdateStatus(tx, w.ID, domain.WalletStatusSuspended)
	require.NoError(t, err)
	err = tx.Commit().Error
	require.NoError(t, err)

	saved, err := repo.FindByID(w.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, domain.WalletStatusSuspended, saved.Status)
}

func TestWalletRepository_DB(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	gdb := repo.DB()
	assert.NotNil(t, gdb)
}

func TestWalletRepository_FindByIDForUpdate_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	tx := db.Begin()
	defer tx.Rollback()

	_, err := repo.FindByIDForUpdate(tx, uuid.New())
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestWalletRepository_FindByID_GenericError(t *testing.T) {
	// Use a closed DB to trigger a non-ErrRecordNotFound error.
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.Close()

	_, err = repo.FindByID(uuid.New())
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound))
}

func TestWalletRepository_FindByOwnerAndCurrency_GenericError(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.Close()

	_, err = repo.FindByOwnerAndCurrency("user-1", "USD")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound))
}

func TestWalletRepository_FindByIDForUpdate_GenericError(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletRepository(db)

	// Use a cancelled context so the query returns a non-RecordNotFound error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx := db.WithContext(ctx).Begin()
	defer tx.Rollback()

	_, err := repo.FindByIDForUpdate(tx, uuid.New())
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound))
}

package service_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/repository"
	"github.com/faisalaffan/ewallet-system/internal/service"
)

// ---------------------------------------------------------------------------
// Mock repositories (struct-based with function fields)
// ---------------------------------------------------------------------------

type mockWalletRepo struct {
	db                      *gorm.DB
	createFn                func(tx *gorm.DB, w *domain.Wallet) error
	findByIDFn              func(id uuid.UUID) (*domain.Wallet, error)
	findByOwnerAndCurrencyFn func(ownerID, currency string) (*domain.Wallet, error)
	findByIDForUpdateFn     func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error)
	updateBalanceFn         func(tx *gorm.DB, id uuid.UUID, balance string) error
	updateStatusFn          func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error
}

func (m *mockWalletRepo) DB() *gorm.DB {
	return m.db
}

func (m *mockWalletRepo) Create(tx *gorm.DB, w *domain.Wallet) error {
	return m.createFn(tx, w)
}

func (m *mockWalletRepo) FindByID(id uuid.UUID) (*domain.Wallet, error) {
	return m.findByIDFn(id)
}

func (m *mockWalletRepo) FindByOwnerAndCurrency(ownerID, currency string) (*domain.Wallet, error) {
	return m.findByOwnerAndCurrencyFn(ownerID, currency)
}

func (m *mockWalletRepo) FindByIDForUpdate(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
	return m.findByIDForUpdateFn(tx, id)
}

func (m *mockWalletRepo) UpdateBalance(tx *gorm.DB, id uuid.UUID, balance string) error {
	return m.updateBalanceFn(tx, id, balance)
}

func (m *mockWalletRepo) UpdateStatus(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
	return m.updateStatusFn(tx, id, status)
}

type mockLedgerRepo struct {
	db                      *gorm.DB
	createFn                func(tx *gorm.DB, entry *domain.LedgerEntry) error
	findByIdempotencyKeyFn  func(key string) (*domain.LedgerEntry, error)
	sumByWalletIDFn         func(walletID string) (topUpSum, paymentSum, transferInSum, transferOutSum string, err error)
}

func (m *mockLedgerRepo) Create(tx *gorm.DB, entry *domain.LedgerEntry) error {
	return m.createFn(tx, entry)
}

func (m *mockLedgerRepo) FindByIdempotencyKey(key string) (*domain.LedgerEntry, error) {
	return m.findByIdempotencyKeyFn(key)
}

func (m *mockLedgerRepo) SumByWalletID(walletID string) (topUpSum, paymentSum, transferInSum, transferOutSum string, err error) {
	return m.sumByWalletIDFn(walletID)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupDB opens an in-memory SQLite database and creates tables manually.
// AutoMigrate is not used because the domain models contain PostgreSQL-specific
// default expressions (e.g. gen_random_uuid()) that SQLite cannot parse.
func setupDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open in-memory SQLite: " + err.Error())
	}
	if err := db.Exec(`CREATE TABLE wallets (
		id TEXT PRIMARY KEY,
		owner_id TEXT NOT NULL,
		currency TEXT NOT NULL,
		balance TEXT NOT NULL DEFAULT '0.00',
		status TEXT NOT NULL DEFAULT 'ACTIVE',
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		panic("failed to create wallets table: " + err.Error())
	}
	if err := db.Exec(`CREATE TABLE ledger_entries (
		entry_id TEXT PRIMARY KEY,
		wallet_id TEXT NOT NULL,
		entry_type TEXT NOT NULL,
		amount TEXT NOT NULL,
		currency TEXT NOT NULL,
		balance_after TEXT NOT NULL,
		reference_id TEXT,
		idempotency_key TEXT NOT NULL UNIQUE,
		created_at DATETIME
	)`).Error; err != nil {
		panic("failed to create ledger_entries table: " + err.Error())
	}
	return db
}

// testWallet returns a wallet with ACTIVE status and 100.00 balance.
func testWallet() *domain.Wallet {
	return &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user1",
		Currency: "USD",
		Balance:  "100.00",
		Status:   domain.WalletStatusActive,
	}
}

// testWalletTo returns a second wallet with ACTIVE status and 50.00 balance.
func testWalletTo() *domain.Wallet {
	return &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user2",
		Currency: "USD",
		Balance:  "50.00",
		Status:   domain.WalletStatusActive,
	}
}

// defaultMocks returns mock repositories pre-configured with working defaults.
// Specific functions can be overridden for error-path testing.
func defaultMocks(db *gorm.DB, w *domain.Wallet) (*mockWalletRepo, *mockLedgerRepo) {
	wm := &mockWalletRepo{
		db: db,
		createFn:                func(tx *gorm.DB, w *domain.Wallet) error { return nil },
		findByIDFn:              func(id uuid.UUID) (*domain.Wallet, error) { return w, nil },
		findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) { return nil, nil },
		findByIDForUpdateFn:     func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) { return w, nil },
		updateBalanceFn:         func(tx *gorm.DB, id uuid.UUID, balance string) error { return nil },
		updateStatusFn:          func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error { return nil },
	}
	lm := &mockLedgerRepo{
		db: db,
		createFn:               func(tx *gorm.DB, entry *domain.LedgerEntry) error { return nil },
		findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) { return nil, nil },
		sumByWalletIDFn:        func(walletID string) (string, string, string, string, error) { return "0", "0", "0", "0", nil },
	}
	return wm, lm
}

// ---------------------------------------------------------------------------
// TestWalletService_Create
// ---------------------------------------------------------------------------

func TestWalletService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		db := setupDB()
		var savedWallet *domain.Wallet

		wm := &mockWalletRepo{
			db: db,
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				savedWallet = w
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}

		svc := service.NewWalletService(wm, lm)
		req := domain.CreateWalletRequest{OwnerID: "alice", Currency: "EUR"}
		w, err := svc.Create(req)

		require.NoError(t, err)
		require.NotNil(t, w)
		assert.Equal(t, "alice", w.OwnerID)
		assert.Equal(t, "EUR", w.Currency)
		assert.Equal(t, "0.00", w.Balance)
		assert.Equal(t, domain.WalletStatusActive, w.Status)
		assert.NotEqual(t, uuid.Nil, w.ID)

		// Verify the wallet passed to repo.Create matches
		require.NotNil(t, savedWallet)
		assert.Equal(t, w.ID, savedWallet.ID)
		assert.Equal(t, w.OwnerID, savedWallet.OwnerID)
	})

	t.Run("already exists", func(t *testing.T) {
		db := setupDB()
		existing := testWallet()

		wm := &mockWalletRepo{
			db: db,
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return existing, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}

		svc := service.NewWalletService(wm, lm)
		req := domain.CreateWalletRequest{OwnerID: "alice", Currency: "USD"}
		_, err := svc.Create(req)

		assert.ErrorIs(t, err, service.ErrAlreadyExists)
	})

	t.Run("invalid currency", func(t *testing.T) {
		db := setupDB()
		wm := &mockWalletRepo{
			db: db,
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}

		svc := service.NewWalletService(wm, lm)

		// Currency with length != 3
		_, err1 := svc.Create(domain.CreateWalletRequest{OwnerID: "alice", Currency: "US"})
		assert.ErrorIs(t, err1, service.ErrInvalidCurrency)

		_, err2 := svc.Create(domain.CreateWalletRequest{OwnerID: "alice", Currency: "USDD"})
		assert.ErrorIs(t, err2, service.ErrInvalidCurrency)

		_, err3 := svc.Create(domain.CreateWalletRequest{OwnerID: "alice", Currency: ""})
		assert.ErrorIs(t, err3, service.ErrInvalidCurrency)
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_GetByID
// ---------------------------------------------------------------------------

func TestWalletService_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		db := setupDB()
		want := testWallet()

		wm := &mockWalletRepo{
			db: db,
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return want, nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		got, err := svc.GetByID(want.ID)
		require.NoError(t, err)
		assert.Equal(t, want.ID, got.ID)
		assert.Equal(t, want.Balance, got.Balance)
	})

	t.Run("not found", func(t *testing.T) {
		db := setupDB()

		wm := &mockWalletRepo{
			db: db,
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.GetByID(uuid.New())
		assert.ErrorIs(t, err, service.ErrNotFound)
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_TopUp
// ---------------------------------------------------------------------------

func TestWalletService_TopUp(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		var capturedEntry *domain.LedgerEntry
		var capturedBalance string

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				capturedBalance = balance
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				capturedEntry = entry
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}

		svc := service.NewWalletService(wm, lm)
		resp, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount:         "50.00",
			IdempotencyKey: "topup-1",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, w.ID.String(), resp.WalletID)
		assert.Equal(t, "150.00", resp.Balance)
		assert.NotEmpty(t, resp.EntryID)

		// Verify ledger entry details
		require.NotNil(t, capturedEntry)
		assert.Equal(t, domain.EntryTypeTopUp, capturedEntry.EntryType)
		assert.Equal(t, "50.00", capturedEntry.Amount)
		assert.Equal(t, "150.00", capturedEntry.BalanceAfter)
		assert.Equal(t, w.Currency, capturedEntry.Currency)
		assert.Equal(t, "topup-1", capturedEntry.IdempotencyKey)
		assert.Equal(t, w.ID, capturedEntry.WalletID)

		// Verify balance update
		assert.Equal(t, "150.00", capturedBalance)
	})

	t.Run("idempotent replay", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wAfter := &domain.Wallet{
			ID:       w.ID,
			OwnerID:  w.OwnerID,
			Currency: w.Currency,
			Balance:  "150.00",
			Status:   domain.WalletStatusActive,
		}

		var idempotentEntry *domain.LedgerEntry

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return wAfter, nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				// Capture the entry for idempotency replay
				idempotentEntry = entry
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				if idempotentEntry != nil && idempotentEntry.IdempotencyKey == key {
					return idempotentEntry, nil
				}
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}

		svc := service.NewWalletService(wm, lm)

		// First call — original top-up
		resp1, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount:         "50.00",
			IdempotencyKey: "topup-idem-1",
		})
		require.NoError(t, err)
		require.NotNil(t, resp1)
		assert.Equal(t, "150.00", resp1.Balance)

		// Second call — same idempotency key → replay
		resp2, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount:         "50.00",
			IdempotencyKey: "topup-idem-1",
		})
		require.NoError(t, err)
		require.NotNil(t, resp2)
		assert.Equal(t, resp1.EntryID, resp2.EntryID, "entry ID must be identical on replay")
		assert.Equal(t, "150.00", resp2.Balance)
	})

	t.Run("wallet not found", func(t *testing.T) {
		db := setupDB()
		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.TopUp(uuid.New(), domain.TopUpRequest{
			Amount:         "50.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("wallet suspended", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		w.Status = domain.WalletStatusSuspended

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount:         "50.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrWalletSuspended)
	})

	t.Run("amount too small", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount:         "0.00",
			IdempotencyKey: "key-zero",
		})
		assert.ErrorIs(t, err, service.ErrAmountTooSmall)
	})

	t.Run("negative amount", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount:         "-10.00",
			IdempotencyKey: "key-neg",
		})
		assert.Error(t, err)
	})

	t.Run("unparseable amount", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount:         "not-a-number",
			IdempotencyKey: "key-bad",
		})
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "not found") // ensure it's not ErrNotFound
	})

	t.Run("rounding", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		var capturedBalance string

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				capturedBalance = balance
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		resp, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount:         "12.345",
			IdempotencyKey: "topup-round",
		})
		require.NoError(t, err)
		// 100.00 + 12.35 = 112.35 (rounded to 2 decimal places)
		expected, _ := decimal.NewFromString("100.00")
		amt, _ := decimal.NewFromString("12.345")
		expected = expected.Add(amt.Round(2))
		assert.Equal(t, expected.StringFixed(2), resp.Balance)
		assert.Equal(t, expected.StringFixed(2), capturedBalance)
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_Pay
// ---------------------------------------------------------------------------

func TestWalletService_Pay(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		var capturedEntry *domain.LedgerEntry
		var capturedBalance string

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				capturedBalance = balance
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				capturedEntry = entry
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		resp, err := svc.Pay(w.ID, domain.PayRequest{
			Amount:         "30.00",
			IdempotencyKey: "pay-1",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, w.ID.String(), resp.WalletID)
		assert.Equal(t, "70.00", resp.Balance)
		assert.NotEmpty(t, resp.EntryID)

		// Verify ledger entry
		require.NotNil(t, capturedEntry)
		assert.Equal(t, domain.EntryTypePayment, capturedEntry.EntryType)
		assert.Equal(t, "30.00", capturedEntry.Amount)
		assert.Equal(t, "70.00", capturedEntry.BalanceAfter)
		assert.Equal(t, "pay-1", capturedEntry.IdempotencyKey)

		// Verify balance update
		assert.Equal(t, "70.00", capturedBalance)
	})

	t.Run("idempotent replay", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wAfter := &domain.Wallet{
			ID:       w.ID,
			OwnerID:  w.OwnerID,
			Currency: w.Currency,
			Balance:  "70.00",
			Status:   domain.WalletStatusActive,
		}

		var idempotentEntry *domain.LedgerEntry

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return wAfter, nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				idempotentEntry = entry
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				if idempotentEntry != nil && idempotentEntry.IdempotencyKey == key {
					return idempotentEntry, nil
				}
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		// First call
		resp1, err := svc.Pay(w.ID, domain.PayRequest{
			Amount:         "30.00",
			IdempotencyKey: "pay-idem-1",
		})
		require.NoError(t, err)
		assert.Equal(t, "70.00", resp1.Balance)

		// Replay
		resp2, err := svc.Pay(w.ID, domain.PayRequest{
			Amount:         "30.00",
			IdempotencyKey: "pay-idem-1",
		})
		require.NoError(t, err)
		assert.Equal(t, resp1.EntryID, resp2.EntryID, "entry ID must match on idempotent replay")
		assert.Equal(t, "70.00", resp2.Balance)
	})

	t.Run("insufficient balance", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		w.Balance = "10.00"

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Pay(w.ID, domain.PayRequest{
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrInsufficientBalance)
	})

	t.Run("wallet suspended", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		w.Status = domain.WalletStatusSuspended

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Pay(w.ID, domain.PayRequest{
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrWalletSuspended)
	})

	t.Run("wallet not found", func(t *testing.T) {
		db := setupDB()
		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Pay(uuid.New(), domain.PayRequest{
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrNotFound)
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_Transfer
// ---------------------------------------------------------------------------

func TestWalletService_Transfer(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		db := setupDB()
		from := testWallet()            // 100.00 USD, ACTIVE
		to := &domain.Wallet{           // 50.00 USD, ACTIVE
			ID:       uuid.New(),
			OwnerID:  "bob",
			Currency: "USD",
			Balance:  "50.00",
			Status:   domain.WalletStatusActive,
		}
		var callCount int
		var capturedOutEntry, capturedInEntry *domain.LedgerEntry
		var capturedFromBal, capturedToBal string

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				callCount++
				if id == from.ID {
					return from, nil
				}
				if id == to.ID {
					return to, nil
				}
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				if id == from.ID {
					capturedFromBal = balance
				}
				if id == to.ID {
					capturedToBal = balance
				}
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				if id == from.ID {
					return from, nil
				}
				if id == to.ID {
					return to, nil
				}
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				if entry.EntryType == domain.EntryTypeTransferOut {
					capturedOutEntry = entry
				}
				if entry.EntryType == domain.EntryTypeTransferIn {
					capturedInEntry = entry
				}
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		resp, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "transfer-1",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "70.00", resp.FromBalance)
		assert.Equal(t, "80.00", resp.ToBalance)
		assert.NotEmpty(t, resp.TransferID)

		// Verify both ledger entries were created correctly
		require.NotNil(t, capturedOutEntry)
		assert.Equal(t, domain.EntryTypeTransferOut, capturedOutEntry.EntryType)
		assert.Equal(t, "30.00", capturedOutEntry.Amount)
		assert.Equal(t, "70.00", capturedOutEntry.BalanceAfter)
		assert.Equal(t, from.Currency, capturedOutEntry.Currency)
		assert.Equal(t, from.ID, capturedOutEntry.WalletID)
		assert.Equal(t, "transfer-1", capturedOutEntry.IdempotencyKey)
		require.NotNil(t, capturedOutEntry.ReferenceID)
		assert.Equal(t, resp.TransferID, *capturedOutEntry.ReferenceID)

		require.NotNil(t, capturedInEntry)
		assert.Equal(t, domain.EntryTypeTransferIn, capturedInEntry.EntryType)
		assert.Equal(t, "30.00", capturedInEntry.Amount)
		assert.Equal(t, "80.00", capturedInEntry.BalanceAfter)
		assert.Equal(t, to.Currency, capturedInEntry.Currency)
		assert.Equal(t, to.ID, capturedInEntry.WalletID)
		assert.Equal(t, "transfer-1_in", capturedInEntry.IdempotencyKey)
		require.NotNil(t, capturedInEntry.ReferenceID)
		assert.Equal(t, resp.TransferID, *capturedInEntry.ReferenceID)

		// Verify balance updates
		assert.Equal(t, "70.00", capturedFromBal)
		assert.Equal(t, "80.00", capturedToBal)

		// Both wallets should have been read for update
		assert.Equal(t, 2, callCount, "FindByIDForUpdate should be called twice")
	})

	t.Run("currency mismatch", func(t *testing.T) {
		db := setupDB()
		from := testWallet() // USD
		to := &domain.Wallet{
			ID:       uuid.New(),
			OwnerID:  "bob",
			Currency: "EUR",
			Balance:  "50.00",
			Status:   domain.WalletStatusActive,
		}

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				if id == from.ID {
					return from, nil
				}
				return to, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrCurrencyMismatch)
	})

	t.Run("same wallet", func(t *testing.T) {
		db := setupDB()
		w := testWallet()

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   w.ID.String(),
			ToWalletID:     w.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrSameWallet)
	})

	t.Run("insufficient balance", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		from.Balance = "10.00"
		to := &domain.Wallet{
			ID:       uuid.New(),
			OwnerID:  "bob",
			Currency: "USD",
			Balance:  "50.00",
			Status:   domain.WalletStatusActive,
		}

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				if id == from.ID {
					return from, nil
				}
				return to, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrInsufficientBalance)
	})

	t.Run("one wallet suspended", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		from.Status = domain.WalletStatusSuspended
		to := &domain.Wallet{
			ID:       uuid.New(),
			OwnerID:  "bob",
			Currency: "USD",
			Balance:  "50.00",
			Status:   domain.WalletStatusActive,
		}

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				if id == from.ID {
					return from, nil
				}
				return to, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrWalletSuspended)
	})

	t.Run("wallet not found", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := uuid.New()

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				if id == from.ID {
					return from, nil
				}
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("idempotent replay", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := &domain.Wallet{
			ID:       uuid.New(),
			OwnerID:  "bob",
			Currency: "USD",
			Balance:  "50.00",
			Status:   domain.WalletStatusActive,
		}
		afterFrom := "70.00"
		afterTo := "80.00"

		var idempotentOutEntry *domain.LedgerEntry

		wm := &mockWalletRepo{
			db: db,
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				if id == from.ID {
					return from, nil
				}
				return to, nil
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				// Return post-transfer balances for idempotent replay
				if id == from.ID {
					return &domain.Wallet{
						ID: from.ID, OwnerID: from.OwnerID, Currency: "USD",
						Balance: afterFrom, Status: domain.WalletStatusActive,
					}, nil
				}
				if id == to.ID {
					return &domain.Wallet{
						ID: to.ID, OwnerID: to.OwnerID, Currency: "USD",
						Balance: afterTo, Status: domain.WalletStatusActive,
					}, nil
				}
				return nil, repository.ErrNotFound
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				if entry.EntryType == domain.EntryTypeTransferOut {
					idempotentOutEntry = entry
				}
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				if idempotentOutEntry != nil && idempotentOutEntry.IdempotencyKey == key {
					return idempotentOutEntry, nil
				}
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		// First call
		resp1, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "transfer-idem-1",
		})
		require.NoError(t, err)
		assert.Equal(t, "70.00", resp1.FromBalance)
		assert.Equal(t, "80.00", resp1.ToBalance)

		// Replay
		resp2, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "transfer-idem-1",
		})
		require.NoError(t, err)
		require.NotNil(t, resp2)
		assert.Equal(t, resp1.TransferID, resp2.TransferID, "TransferID must match on replay")
		assert.Equal(t, "70.00", resp2.FromBalance)
		assert.Equal(t, "80.00", resp2.ToBalance)
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_Suspend
// ---------------------------------------------------------------------------

func TestWalletService_Suspend(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		var updatedStatus domain.WalletStatus

		wm := &mockWalletRepo{
			db: db,
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				updatedStatus = status
				return nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		resp, err := svc.Suspend(w.ID)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, w.ID.String(), resp.WalletID)
		assert.Equal(t, domain.WalletStatusSuspended, resp.Status)
		assert.Equal(t, domain.WalletStatusSuspended, updatedStatus)
	})

	t.Run("already suspended", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		w.Status = domain.WalletStatusSuspended
		var updateStatusCalled bool

		wm := &mockWalletRepo{
			db: db,
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				updateStatusCalled = true
				return nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		resp, err := svc.Suspend(w.ID)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, domain.WalletStatusSuspended, resp.Status)
		assert.False(t, updateStatusCalled, "UpdateStatus should not be called when already suspended")
	})

	t.Run("not found", func(t *testing.T) {
		db := setupDB()
		wm := &mockWalletRepo{
			db: db,
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateStatusFn: func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
				return nil
			},
			findByOwnerAndCurrencyFn: func(ownerID, currency string) (*domain.Wallet, error) {
				return nil, nil
			},
			createFn: func(tx *gorm.DB, w *domain.Wallet) error {
				return nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
			updateBalanceFn: func(tx *gorm.DB, id uuid.UUID, balance string) error {
				return nil
			},
		}
		lm := &mockLedgerRepo{
			db: db,
			createFn: func(tx *gorm.DB, entry *domain.LedgerEntry) error {
				return nil
			},
			findByIdempotencyKeyFn: func(key string) (*domain.LedgerEntry, error) {
				return nil, nil
			},
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}
		svc := service.NewWalletService(wm, lm)

		_, err := svc.Suspend(uuid.New())
		assert.ErrorIs(t, err, service.ErrNotFound)
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_Create_Errors
// ---------------------------------------------------------------------------

func TestWalletService_Create_Errors(t *testing.T) {
	t.Parallel()

	t.Run("find by owner and currency error", func(t *testing.T) {
		db := setupDB()
		wm, lm := defaultMocks(db, testWallet())
		wm.findByOwnerAndCurrencyFn = func(ownerID, currency string) (*domain.Wallet, error) {
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Create(domain.CreateWalletRequest{OwnerID: "alice", Currency: "USD"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})

	t.Run("create wallet error", func(t *testing.T) {
		db := setupDB()
		wm, lm := defaultMocks(db, testWallet())
		wm.createFn = func(tx *gorm.DB, w *domain.Wallet) error {
			return errors.New("create error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Create(domain.CreateWalletRequest{OwnerID: "alice", Currency: "USD"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create error")
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_GetByID_Errors
// ---------------------------------------------------------------------------

func TestWalletService_GetByID_Errors(t *testing.T) {
	t.Parallel()

	t.Run("generic error", func(t *testing.T) {
		db := setupDB()
		wm, lm := defaultMocks(db, testWallet())
		wm.findByIDFn = func(id uuid.UUID) (*domain.Wallet, error) {
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.GetByID(uuid.New())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
		assert.False(t, errors.Is(err, service.ErrNotFound))
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_TopUp_Errors
// ---------------------------------------------------------------------------

func TestWalletService_TopUp_Errors(t *testing.T) {
	t.Parallel()

	t.Run("idempotency key lookup error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		lm.findByIdempotencyKeyFn = func(key string) (*domain.LedgerEntry, error) {
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})

	t.Run("idempotent replay find wallet error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		lm.findByIdempotencyKeyFn = func(key string) (*domain.LedgerEntry, error) {
			ref := "ref"
			return &domain.LedgerEntry{
				EntryID: uuid.New(), WalletID: w.ID,
				EntryType: domain.EntryTypeTopUp, Amount: "50.00",
				BalanceAfter: "150.00", ReferenceID: &ref,
			}, nil
		}
		wm.findByIDFn = func(id uuid.UUID) (*domain.Wallet, error) {
			return nil, errors.New("not found")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("find by id for update generic error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
		assert.False(t, errors.Is(err, service.ErrNotFound))
	})

	t.Run("create ledger entry error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		lm.createFn = func(tx *gorm.DB, entry *domain.LedgerEntry) error {
			return errors.New("ledger error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ledger error")
	})

	t.Run("update balance error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		wm.updateBalanceFn = func(tx *gorm.DB, id uuid.UUID, balance string) error {
			return errors.New("balance error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.TopUp(w.ID, domain.TopUpRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "balance error")
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_Pay_Errors
// ---------------------------------------------------------------------------

func TestWalletService_Pay_Errors(t *testing.T) {
	t.Parallel()

	t.Run("invalid amount", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Pay(w.ID, domain.PayRequest{
			Amount: "invalid", IdempotencyKey: "key",
		})
		require.Error(t, err)
	})

	t.Run("idempotency key lookup error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		lm.findByIdempotencyKeyFn = func(key string) (*domain.LedgerEntry, error) {
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Pay(w.ID, domain.PayRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})

	t.Run("idempotent replay find wallet error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		lm.findByIdempotencyKeyFn = func(key string) (*domain.LedgerEntry, error) {
			ref := "ref"
			return &domain.LedgerEntry{
				EntryID: uuid.New(), WalletID: w.ID,
				EntryType: domain.EntryTypePayment, Amount: "50.00",
				BalanceAfter: "50.00", ReferenceID: &ref,
			}, nil
		}
		wm.findByIDFn = func(id uuid.UUID) (*domain.Wallet, error) {
			return nil, errors.New("not found")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Pay(w.ID, domain.PayRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("find by id for update generic error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Pay(w.ID, domain.PayRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
		assert.False(t, errors.Is(err, service.ErrNotFound))
	})

	t.Run("create ledger entry error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		lm.createFn = func(tx *gorm.DB, entry *domain.LedgerEntry) error {
			return errors.New("ledger error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Pay(w.ID, domain.PayRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ledger error")
	})

	t.Run("update balance error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		wm.updateBalanceFn = func(tx *gorm.DB, id uuid.UUID, balance string) error {
			return errors.New("balance error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Pay(w.ID, domain.PayRequest{
			Amount: "50.00", IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "balance error")
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_Transfer_Errors
// ---------------------------------------------------------------------------

func TestWalletService_Transfer_Errors(t *testing.T) {
	t.Parallel()

	t.Run("invalid amount", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "invalid",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
	})

	t.Run("invalid from wallet id", func(t *testing.T) {
		db := setupDB()
		to := testWalletTo()
		wm, lm := defaultMocks(db, testWallet())
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   "not-a-uuid",
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
	})

	t.Run("invalid to wallet id", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		wm, lm := defaultMocks(db, from)
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     "not-a-uuid",
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
	})

	t.Run("idempotency key lookup error", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		lm.findByIdempotencyKeyFn = func(key string) (*domain.LedgerEntry, error) {
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})

	t.Run("idempotent replay nil reference", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		lm.findByIdempotencyKeyFn = func(key string) (*domain.LedgerEntry, error) {
			return &domain.LedgerEntry{
				EntryID: uuid.New(), WalletID: from.ID,
				EntryType: domain.EntryTypeTransferOut, Amount: "30.00",
				BalanceAfter: "70.00", ReferenceID: nil,
			}, nil
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no reference found")
	})

	t.Run("from wallet not found", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			if id == from.ID {
				return nil, repository.ErrNotFound
			}
			return to, nil
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("from wallet generic error", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			if id == from.ID {
				return nil, errors.New("db error")
			}
			return to, nil
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
		assert.False(t, errors.Is(err, service.ErrNotFound))
	})

	t.Run("to wallet generic error", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		var callCount int
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			callCount++
			if callCount == 1 {
				return from, nil
			}
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})

	t.Run("create out ledger entry error", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			if id == from.ID {
				return from, nil
			}
			return to, nil
		}
		lm.createFn = func(tx *gorm.DB, entry *domain.LedgerEntry) error {
			return errors.New("ledger error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ledger error")
	})

	t.Run("create in ledger entry error", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			if id == from.ID {
				return from, nil
			}
			return to, nil
		}
		var callCount int
		lm.createFn = func(tx *gorm.DB, entry *domain.LedgerEntry) error {
			callCount++
			if callCount <= 1 {
				return nil
			}
			return errors.New("in ledger error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "in ledger error")
	})

	t.Run("update from balance error", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			if id == from.ID {
				return from, nil
			}
			return to, nil
		}
		wm.updateBalanceFn = func(tx *gorm.DB, id uuid.UUID, balance string) error {
			return errors.New("balance error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "balance error")
	})

	t.Run("update to balance error", func(t *testing.T) {
		db := setupDB()
		from := testWallet()
		to := testWalletTo()
		wm, lm := defaultMocks(db, from)
		wm.findByIDForUpdateFn = func(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
			if id == from.ID {
				return from, nil
			}
			return to, nil
		}
		var callCount int
		wm.updateBalanceFn = func(tx *gorm.DB, id uuid.UUID, balance string) error {
			callCount++
			if callCount == 1 {
				return nil
			}
			return errors.New("balance error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Transfer(domain.TransferRequest{
			FromWalletID:   from.ID.String(),
			ToWalletID:     to.ID.String(),
			Amount:         "30.00",
			IdempotencyKey: "key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "balance error")
	})
}

// ---------------------------------------------------------------------------
// TestWalletService_Suspend_Errors
// ---------------------------------------------------------------------------

func TestWalletService_Suspend_Errors(t *testing.T) {
	t.Parallel()

	t.Run("generic error", func(t *testing.T) {
		db := setupDB()
		wm, lm := defaultMocks(db, testWallet())
		wm.findByIDFn = func(id uuid.UUID) (*domain.Wallet, error) {
			return nil, errors.New("db error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Suspend(uuid.New())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
		assert.False(t, errors.Is(err, service.ErrNotFound))
	})

	t.Run("update status error", func(t *testing.T) {
		db := setupDB()
		w := testWallet()
		wm, lm := defaultMocks(db, w)
		wm.updateStatusFn = func(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
			return errors.New("status error")
		}
		svc := service.NewWalletService(wm, lm)
		_, err := svc.Suspend(w.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status error")
	})
}

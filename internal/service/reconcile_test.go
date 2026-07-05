package service_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/repository"
	"github.com/faisalaffan/ewallet-system/internal/service"
)

// ---------------------------------------------------------------------------
// Mock repositories for ReconcileService tests
// ---------------------------------------------------------------------------

type mockReconcileWalletRepo struct {
	findByIDFn func(id uuid.UUID) (*domain.Wallet, error)
}

func (m *mockReconcileWalletRepo) DB() *gorm.DB {
	return nil
}

func (m *mockReconcileWalletRepo) Create(tx *gorm.DB, w *domain.Wallet) error {
	return nil
}

func (m *mockReconcileWalletRepo) FindByID(id uuid.UUID) (*domain.Wallet, error) {
	return m.findByIDFn(id)
}

func (m *mockReconcileWalletRepo) FindByOwnerAndCurrency(ownerID, currency string) (*domain.Wallet, error) {
	return nil, nil
}

func (m *mockReconcileWalletRepo) FindByIDForUpdate(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
	return nil, nil
}

func (m *mockReconcileWalletRepo) UpdateBalance(tx *gorm.DB, id uuid.UUID, balance string) error {
	return nil
}

func (m *mockReconcileWalletRepo) UpdateStatus(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
	return nil
}

type mockReconcileLedgerRepo struct {
	sumByWalletIDFn func(walletID string) (topUpSum, paymentSum, transferInSum, transferOutSum string, err error)
}

func (m *mockReconcileLedgerRepo) Create(tx *gorm.DB, entry *domain.LedgerEntry) error {
	return nil
}

func (m *mockReconcileLedgerRepo) FindByIdempotencyKey(key string) (*domain.LedgerEntry, error) {
	return nil, nil
}

func (m *mockReconcileLedgerRepo) SumByWalletID(walletID string) (topUpSum, paymentSum, transferInSum, transferOutSum string, err error) {
	return m.sumByWalletIDFn(walletID)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func reconcileWallet() *domain.Wallet {
	return &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  "user1",
		Currency: "USD",
		Balance:  "100.00",
		Status:   domain.WalletStatusActive,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestReconcileService_Reconcile(t *testing.T) {
	t.Parallel()

	t.Run("success with matching balance", func(t *testing.T) {
		w := reconcileWallet()

		walletRepo := &mockReconcileWalletRepo{
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
		}
		ledgerRepo := &mockReconcileLedgerRepo{
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "100.00", "0", "0", "0", nil
			},
		}

		svc := service.NewReconcileService(walletRepo, ledgerRepo)
		resp, err := svc.Reconcile(w.ID)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, w.ID.String(), resp.WalletID)
		assert.Equal(t, "100.00", resp.CachedBalance)
		assert.Equal(t, "100.00", resp.LedgerSum)
		assert.True(t, resp.Match)
		assert.Equal(t, "0.00", resp.Diff)
	})

	t.Run("mismatch", func(t *testing.T) {
		w := reconcileWallet()

		walletRepo := &mockReconcileWalletRepo{
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
		}
		ledgerRepo := &mockReconcileLedgerRepo{
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "90.00", "0", "0", "0", nil
			},
		}

		svc := service.NewReconcileService(walletRepo, ledgerRepo)
		resp, err := svc.Reconcile(w.ID)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Match)
		assert.Equal(t, "10.00", resp.Diff)
	})

	t.Run("all entry types net to balance", func(t *testing.T) {
		w := reconcileWallet()
		w.Balance = "200.00"

		walletRepo := &mockReconcileWalletRepo{
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
		}
		ledgerRepo := &mockReconcileLedgerRepo{
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				// net = 300 + 100 - 50 - 150 = 200
				return "300", "50", "100", "150", nil
			},
		}

		svc := service.NewReconcileService(walletRepo, ledgerRepo)
		resp, err := svc.Reconcile(w.ID)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, w.ID.String(), resp.WalletID)
		assert.Equal(t, "200.00", resp.CachedBalance)
		assert.True(t, resp.Match)
		assert.Equal(t, "0.00", resp.Diff)
	})

	t.Run("wallet not found", func(t *testing.T) {
		walletRepo := &mockReconcileWalletRepo{
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, repository.ErrNotFound
			},
		}
		ledgerRepo := &mockReconcileLedgerRepo{
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "", "", "", "", nil
			},
		}

		svc := service.NewReconcileService(walletRepo, ledgerRepo)
		resp, err := svc.Reconcile(uuid.New())

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("wallet repo generic error", func(t *testing.T) {
		genericErr := errors.New("database connection failed")

		walletRepo := &mockReconcileWalletRepo{
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return nil, genericErr
			},
		}
		ledgerRepo := &mockReconcileLedgerRepo{
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "", "", "", "", nil
			},
		}

		svc := service.NewReconcileService(walletRepo, ledgerRepo)
		resp, err := svc.Reconcile(uuid.New())

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, genericErr)
	})

	t.Run("ledger sum error", func(t *testing.T) {
		w := reconcileWallet()
		ledgerErr := errors.New("ledger query failed")

		walletRepo := &mockReconcileWalletRepo{
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
		}
		ledgerRepo := &mockReconcileLedgerRepo{
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "", "", "", "", ledgerErr
			},
		}

		svc := service.NewReconcileService(walletRepo, ledgerRepo)
		resp, err := svc.Reconcile(w.ID)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ledgerErr)
	})

	t.Run("empty ledger matches zero balance", func(t *testing.T) {
		w := reconcileWallet()
		w.Balance = "0.00"

		walletRepo := &mockReconcileWalletRepo{
			findByIDFn: func(id uuid.UUID) (*domain.Wallet, error) {
				return w, nil
			},
		}
		ledgerRepo := &mockReconcileLedgerRepo{
			sumByWalletIDFn: func(walletID string) (string, string, string, string, error) {
				return "0", "0", "0", "0", nil
			},
		}

		svc := service.NewReconcileService(walletRepo, ledgerRepo)
		resp, err := svc.Reconcile(w.ID)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Match)
		assert.Equal(t, "0.00", resp.Diff)
	})
}

package repository

import (
	"testing"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedgerRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLedgerRepository(db)

	walletID := uuid.New()
	ref := "ref-1"
	entry := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       walletID,
		EntryType:      domain.EntryTypeTopUp,
		Amount:         "100.00",
		Currency:       "USD",
		BalanceAfter:   "100.00",
		ReferenceID:    &ref,
		IdempotencyKey: "idemp-001",
	}

	err := repo.Create(db, entry)
	require.NoError(t, err)

	// Verify by fetching via idempotency key
	saved, err := repo.FindByIdempotencyKey("idemp-001")
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, entry.EntryID, saved.EntryID)
	assert.Equal(t, entry.WalletID, saved.WalletID)
	assert.Equal(t, domain.EntryTypeTopUp, saved.EntryType)
	assert.Equal(t, "100.00", saved.Amount)
	assert.Equal(t, "USD", saved.Currency)
	assert.Equal(t, "100.00", saved.BalanceAfter)
	assert.NotNil(t, saved.ReferenceID)
	assert.Equal(t, "ref-1", *saved.ReferenceID)
	assert.Equal(t, "idemp-001", saved.IdempotencyKey)
	assert.NotZero(t, saved.CreatedAt)
}

func TestLedgerRepository_FindByIdempotencyKey(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLedgerRepository(db)

	walletID := uuid.New()
	ref := "ref-create"
	entry := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       walletID,
		EntryType:      domain.EntryTypePayment,
		Amount:         "50.00",
		Currency:       "USD",
		BalanceAfter:   "50.00",
		ReferenceID:    &ref,
		IdempotencyKey: "idemp-002",
	}

	err := repo.Create(db, entry)
	require.NoError(t, err)

	// Found — should return the entry
	saved, err := repo.FindByIdempotencyKey("idemp-002")
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, entry.EntryID, saved.EntryID)

	// Not found — should return nil
	notFound, err := repo.FindByIdempotencyKey("nonexistent-key")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestLedgerRepository_SumByWalletID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLedgerRepository(db)

	walletID := uuid.New()

	// Empty wallet — no entries exist
	topUp, payment, transferIn, transferOut, err := repo.SumByWalletID(walletID.String())
	require.NoError(t, err)
	assert.Equal(t, "", topUp)
	assert.Equal(t, "", payment)
	assert.Equal(t, "", transferIn)
	assert.Equal(t, "", transferOut)

	// Create wallet with topup + payment entries
	ref1 := "ref-topup"
	ref2 := "ref-payment"

	topup1 := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       walletID,
		EntryType:      domain.EntryTypeTopUp,
		Amount:         "200.00",
		Currency:       "USD",
		BalanceAfter:   "200.00",
		ReferenceID:    &ref1,
		IdempotencyKey: uuid.New().String(),
	}
	topup2 := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       walletID,
		EntryType:      domain.EntryTypeTopUp,
		Amount:         "50.00",
		Currency:       "USD",
		BalanceAfter:   "250.00",
		ReferenceID:    &ref1,
		IdempotencyKey: uuid.New().String(),
	}
	payment1 := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       walletID,
		EntryType:      domain.EntryTypePayment,
		Amount:         "30.00",
		Currency:       "USD",
		BalanceAfter:   "220.00",
		ReferenceID:    &ref2,
		IdempotencyKey: uuid.New().String(),
	}
	payment2 := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       walletID,
		EntryType:      domain.EntryTypePayment,
		Amount:         "20.00",
		Currency:       "USD",
		BalanceAfter:   "200.00",
		ReferenceID:    &ref2,
		IdempotencyKey: uuid.New().String(),
	}

	err = repo.Create(db, topup1)
	require.NoError(t, err)
	err = repo.Create(db, topup2)
	require.NoError(t, err)
	err = repo.Create(db, payment1)
	require.NoError(t, err)
	err = repo.Create(db, payment2)
	require.NoError(t, err)

	// Verify sums — SQLite's SUM on text returns numeric aggregates
	// without trailing zeros.
	topUp, payment, transferIn, transferOut, err = repo.SumByWalletID(walletID.String())
	require.NoError(t, err)
	assert.Equal(t, "250", topUp)
	assert.Equal(t, "50", payment)
	assert.Equal(t, "", transferIn)
	assert.Equal(t, "", transferOut)

	// A different wallet with entries should have independent sums
	otherWalletID := uuid.New()
	ref3 := "ref-other"
	otherEntry := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       otherWalletID,
		EntryType:      domain.EntryTypeTransferIn,
		Amount:         "500.00",
		Currency:       "USD",
		BalanceAfter:   "500.00",
		ReferenceID:    &ref3,
		IdempotencyKey: uuid.New().String(),
	}
	err = repo.Create(db, otherEntry)
	require.NoError(t, err)

	topUp, payment, transferIn, transferOut, err = repo.SumByWalletID(otherWalletID.String())
	require.NoError(t, err)
	assert.Equal(t, "", topUp)
	assert.Equal(t, "", payment)
	assert.Equal(t, "500", transferIn)
	assert.Equal(t, "", transferOut)
}

func TestLedgerRepository_FindByIdempotencyKey_Error(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLedgerRepository(db)

	// Close the underlying DB to trigger a generic error.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.Close()

	_, err = repo.FindByIdempotencyKey("any-key")
	require.Error(t, err)
}

func TestLedgerRepository_SumByWalletID_Error(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLedgerRepository(db)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.Close()

	_, _, _, _, err = repo.SumByWalletID(uuid.New().String())
	require.Error(t, err)
}

func TestLedgerRepository_SumByWalletID_AllEntryTypes(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLedgerRepository(db)

	walletID := uuid.New()
	ref := "all-types"

	// Create entries of all four types for the same wallet
	entries := []*domain.LedgerEntry{
		{
			EntryID:        uuid.New(),
			WalletID:       walletID,
			EntryType:      domain.EntryTypeTopUp,
			Amount:         "200.00",
			Currency:       "USD",
			BalanceAfter:   "200.00",
			ReferenceID:    &ref,
			IdempotencyKey: uuid.New().String(),
		},
		{
			EntryID:        uuid.New(),
			WalletID:       walletID,
			EntryType:      domain.EntryTypePayment,
			Amount:         "30.00",
			Currency:       "USD",
			BalanceAfter:   "170.00",
			ReferenceID:    &ref,
			IdempotencyKey: uuid.New().String(),
		},
		{
			EntryID:        uuid.New(),
			WalletID:       walletID,
			EntryType:      domain.EntryTypeTransferIn,
			Amount:         "100.00",
			Currency:       "USD",
			BalanceAfter:   "270.00",
			ReferenceID:    &ref,
			IdempotencyKey: uuid.New().String(),
		},
		{
			EntryID:        uuid.New(),
			WalletID:       walletID,
			EntryType:      domain.EntryTypeTransferOut,
			Amount:         "50.00",
			Currency:       "USD",
			BalanceAfter:   "220.00",
			ReferenceID:    &ref,
			IdempotencyKey: uuid.New().String(),
		},
	}
	for _, e := range entries {
		err := repo.Create(db, e)
		require.NoError(t, err)
	}

	topUp, payment, transferIn, transferOut, err := repo.SumByWalletID(walletID.String())
	require.NoError(t, err)
	assert.Equal(t, "200", topUp)
	assert.Equal(t, "30", payment)
	assert.Equal(t, "100", transferIn)
	assert.Equal(t, "50", transferOut)
}

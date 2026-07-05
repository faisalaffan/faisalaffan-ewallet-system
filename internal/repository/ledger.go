package repository

import (
	"errors"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"gorm.io/gorm"
)

type LedgerRepository struct {
	db *gorm.DB
}

func NewLedgerRepository(db *gorm.DB) *LedgerRepository {
	return &LedgerRepository{db: db}
}

func (r *LedgerRepository) Create(tx *gorm.DB, entry *domain.LedgerEntry) error {
	return tx.Create(entry).Error
}

func (r *LedgerRepository) FindByIdempotencyKey(key string) (*domain.LedgerEntry, error) {
	var entry domain.LedgerEntry
	err := r.db.Where("idempotency_key = ?", key).First(&entry).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &entry, nil
}

func (r *LedgerRepository) SumByWalletID(walletID string) (topUpSum, paymentSum, transferInSum, transferOutSum string, err error) {
	type result struct {
		EntryType string `gorm:"column:entry_type"`
		Total     string `gorm:"column:total"`
	}
	var results []result
	err = r.db.Model(&domain.LedgerEntry{}).
		Select("entry_type, SUM(amount) as total").
		Where("wallet_id = ?", walletID).
		Group("entry_type").
		Find(&results).Error
	if err != nil {
		return "0", "0", "0", "0", err
	}
	for _, r := range results {
		switch domain.EntryType(r.EntryType) {
		case domain.EntryTypeTopUp:
			topUpSum = r.Total
		case domain.EntryTypePayment:
			paymentSum = r.Total
		case domain.EntryTypeTransferIn:
			transferInSum = r.Total
		case domain.EntryTypeTransferOut:
			transferOutSum = r.Total
		}
	}
	return topUpSum, paymentSum, transferInSum, transferOutSum, nil
}

package repository

import (
	"context"
	"errors"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WalletRepository struct {
	db *gorm.DB
}

func NewWalletRepository(db *gorm.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) DB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *WalletRepository) Create(tx *gorm.DB, w *domain.Wallet) error {
	return tx.Create(w).Error
}

func (r *WalletRepository) FindByID(id uuid.UUID) (*domain.Wallet, error) {
	var w domain.Wallet
	err := r.db.First(&w, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &w, nil
}

func (r *WalletRepository) FindByOwnerAndCurrency(ownerID, currency string) (*domain.Wallet, error) {
	var w domain.Wallet
	err := r.db.Where("owner_id = ? AND currency = ?", ownerID, currency).First(&w).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &w, nil
}

func (r *WalletRepository) FindByIDForUpdate(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error) {
	var w domain.Wallet
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&w, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &w, nil
}

func (r *WalletRepository) UpdateBalance(tx *gorm.DB, id uuid.UUID, balance string) error {
	return tx.Model(&domain.Wallet{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"balance":    balance,
			"updated_at": gorm.Expr("now()"),
		}).Error
}

func (r *WalletRepository) UpdateStatus(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error {
	return tx.Model(&domain.Wallet{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": gorm.Expr("now()"),
		}).Error
}

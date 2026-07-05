package service

import (
	"errors"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/repository"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ReconcileServiceInterface defines the contract for reconcile operations used by the handler.
type ReconcileServiceInterface interface {
	Reconcile(id uuid.UUID) (*domain.ReconcileResponse, error)
}

// Ensure ReconcileService satisfies ReconcileServiceInterface at compile time.
var _ ReconcileServiceInterface = (*ReconcileService)(nil)

type ReconcileService struct {
	walletRepo WalletRepository
	ledgerRepo LedgerRepository
}

func NewReconcileService(walletRepo WalletRepository, ledgerRepo LedgerRepository) *ReconcileService {
	return &ReconcileService{walletRepo: walletRepo, ledgerRepo: ledgerRepo}
}

func (s *ReconcileService) Reconcile(id uuid.UUID) (*domain.ReconcileResponse, error) {
	w, err := s.walletRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	topUpSum, paymentSum, transferInSum, transferOutSum, err := s.ledgerRepo.SumByWalletID(id.String())
	if err != nil {
		return nil, err
	}

	cachedBalance, _ := decimal.NewFromString(w.Balance)

	topUp, _ := decimal.NewFromString(topUpSum)
	payment, _ := decimal.NewFromString(paymentSum)
	transferIn, _ := decimal.NewFromString(transferInSum)
	transferOut, _ := decimal.NewFromString(transferOutSum)

	// net = TOPUP + TRANSFER_IN - PAYMENT - TRANSFER_OUT
	ledgerSum := topUp.Add(transferIn).Sub(payment).Sub(transferOut)

	match := cachedBalance.Equal(ledgerSum)
	diff := cachedBalance.Sub(ledgerSum).StringFixed(2)

	return &domain.ReconcileResponse{
		WalletID:      w.ID.String(),
		CachedBalance: w.Balance,
		LedgerSum:     ledgerSum.StringFixed(2),
		Match:         match,
		Diff:          diff,
	}, nil
}

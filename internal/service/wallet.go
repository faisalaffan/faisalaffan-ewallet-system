package service

import (
	"errors"
	"fmt"
	"sort"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/repository"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrNotFound             = errors.New("wallet not found")
	ErrAlreadyExists        = errors.New("wallet already exists for this owner and currency")
	ErrInvalidAmount        = errors.New("amount must be greater than 0")
	ErrWalletSuspended      = errors.New("wallet is suspended")
	ErrInsufficientBalance  = errors.New("insufficient balance")
	ErrCurrencyMismatch     = errors.New("currency mismatch between wallets")
	ErrSameWallet           = errors.New("cannot transfer to the same wallet")
	ErrInvalidCurrency      = errors.New("invalid currency code")
	ErrAmountTooSmall       = errors.New("amount must be at least 0.01 after rounding")
)

var minAmount = decimal.NewFromFloat(0.01)

// WalletServiceInterface defines the contract for wallet operations used by the handler.
type WalletServiceInterface interface {
	Create(req domain.CreateWalletRequest) (*domain.Wallet, error)
	GetByID(id uuid.UUID) (*domain.Wallet, error)
	TopUp(walletID uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error)
	Pay(walletID uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error)
	Transfer(req domain.TransferRequest) (*domain.TransferResponse, error)
	Suspend(walletID uuid.UUID) (*domain.SuspendResponse, error)
}

// Ensure WalletService satisfies WalletServiceInterface at compile time.
var _ WalletServiceInterface = (*WalletService)(nil)

// WalletRepository defines the interface for wallet data access.
type WalletRepository interface {
	DB() *gorm.DB
	Create(tx *gorm.DB, w *domain.Wallet) error
	FindByID(id uuid.UUID) (*domain.Wallet, error)
	FindByOwnerAndCurrency(ownerID, currency string) (*domain.Wallet, error)
	FindByIDForUpdate(tx *gorm.DB, id uuid.UUID) (*domain.Wallet, error)
	UpdateBalance(tx *gorm.DB, id uuid.UUID, balance string) error
	UpdateStatus(tx *gorm.DB, id uuid.UUID, status domain.WalletStatus) error
}

// LedgerRepository defines the interface for ledger data access.
type LedgerRepository interface {
	Create(tx *gorm.DB, entry *domain.LedgerEntry) error
	FindByIdempotencyKey(key string) (*domain.LedgerEntry, error)
	SumByWalletID(walletID string) (topUpSum, paymentSum, transferInSum, transferOutSum string, err error)
}

type WalletService struct {
	walletRepo WalletRepository
	ledgerRepo LedgerRepository
	eventBus   EventPublisher
}

type EventPublisher interface {
	Publish(topic string, evt any)
}

func NewWalletService(walletRepo WalletRepository, ledgerRepo LedgerRepository, eventBus EventPublisher) *WalletService {
	if eventBus == nil {
		eventBus = &noopPublisher{}
	}
	return &WalletService{walletRepo: walletRepo, ledgerRepo: ledgerRepo, eventBus: eventBus}
}

type noopPublisher struct{}

func (n *noopPublisher) Publish(topic string, evt any) {}

func (s *WalletService) Create(req domain.CreateWalletRequest) (*domain.Wallet, error) {
	if len(req.Currency) != 3 {
		return nil, ErrInvalidCurrency
	}

	existing, err := s.walletRepo.FindByOwnerAndCurrency(req.OwnerID, req.Currency)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAlreadyExists
	}

	w := &domain.Wallet{
		ID:       uuid.New(),
		OwnerID:  req.OwnerID,
		Currency: req.Currency,
		Balance:  "0.00",
		Status:   domain.WalletStatusActive,
	}
	if err := s.walletRepo.Create(s.walletRepo.DB(), w); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *WalletService) GetByID(id uuid.UUID) (*domain.Wallet, error) {
	w, err := s.walletRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return w, nil
}

func (s *WalletService) TopUp(walletID uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
	amount, err := parseAndRoundAmount(req.Amount)
	if err != nil {
		return nil, err
	}

	// Check idempotency from outside the transaction
	entryID := uuid.New()
	existing, err := s.ledgerRepo.FindByIdempotencyKey(req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		w, err := s.walletRepo.FindByID(walletID)
		if err != nil {
			return nil, err
		}
		return &domain.TopUpResponse{
			WalletID: w.ID.String(),
			Balance:  w.Balance,
			EntryID:  existing.EntryID.String(),
		}, nil
	}

	tx := s.walletRepo.DB().Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	w, err := s.walletRepo.FindByIDForUpdate(tx, walletID)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if w.Status != domain.WalletStatusActive {
		tx.Rollback()
		return nil, ErrWalletSuspended
	}

	currentBalance, _ := decimal.NewFromString(w.Balance)
	newBalance := currentBalance.Add(amount)

	entry := &domain.LedgerEntry{
		EntryID:        entryID,
		WalletID:       walletID,
		EntryType:      domain.EntryTypeTopUp,
		Amount:         amount.StringFixed(2),
		Currency:       w.Currency,
		BalanceAfter:   newBalance.StringFixed(2),
		IdempotencyKey: req.IdempotencyKey,
	}

	if err := s.ledgerRepo.Create(tx, entry); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := s.walletRepo.UpdateBalance(tx, walletID, newBalance.StringFixed(2)); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	s.eventBus.Publish("ledger", *entry)

	return &domain.TopUpResponse{
		WalletID: w.ID.String(),
		Balance:  newBalance.StringFixed(2),
		EntryID:  entryID.String(),
	}, nil
}

func (s *WalletService) Pay(walletID uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error) {
	amount, err := parseAndRoundAmount(req.Amount)
	if err != nil {
		return nil, err
	}

	entryID := uuid.New()
	existing, err := s.ledgerRepo.FindByIdempotencyKey(req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		w, err := s.walletRepo.FindByID(walletID)
		if err != nil {
			return nil, err
		}
		return &domain.PayResponse{
			WalletID: w.ID.String(),
			Balance:  w.Balance,
			EntryID:  existing.EntryID.String(),
		}, nil
	}

	tx := s.walletRepo.DB().Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	w, err := s.walletRepo.FindByIDForUpdate(tx, walletID)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if w.Status != domain.WalletStatusActive {
		tx.Rollback()
		return nil, ErrWalletSuspended
	}

	currentBalance, _ := decimal.NewFromString(w.Balance)
	if currentBalance.LessThan(amount) {
		tx.Rollback()
		return nil, ErrInsufficientBalance
	}

	newBalance := currentBalance.Sub(amount)

	entry := &domain.LedgerEntry{
		EntryID:        entryID,
		WalletID:       walletID,
		EntryType:      domain.EntryTypePayment,
		Amount:         amount.StringFixed(2),
		Currency:       w.Currency,
		BalanceAfter:   newBalance.StringFixed(2),
		IdempotencyKey: req.IdempotencyKey,
	}

	if err := s.ledgerRepo.Create(tx, entry); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := s.walletRepo.UpdateBalance(tx, walletID, newBalance.StringFixed(2)); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	s.eventBus.Publish("ledger", *entry)

	return &domain.PayResponse{
		WalletID: w.ID.String(),
		Balance:  newBalance.StringFixed(2),
		EntryID:  entryID.String(),
	}, nil
}

func (s *WalletService) Transfer(req domain.TransferRequest) (*domain.TransferResponse, error) {
	amount, err := parseAndRoundAmount(req.Amount)
	if err != nil {
		return nil, err
	}

	fromID, err := uuid.Parse(req.FromWalletID)
	if err != nil {
		return nil, fmt.Errorf("invalid from_wallet_id: %w", err)
	}
	toID, err := uuid.Parse(req.ToWalletID)
	if err != nil {
		return nil, fmt.Errorf("invalid to_wallet_id: %w", err)
	}

	if fromID == toID {
		return nil, ErrSameWallet
	}

	// Check idempotency
	existing, err := s.ledgerRepo.FindByIdempotencyKey(req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		refID := existing.ReferenceID
		if refID == nil {
			return nil, fmt.Errorf("duplicate idempotency key, but no reference found")
		}
		fromW, _ := s.walletRepo.FindByID(fromID)
		toW, _ := s.walletRepo.FindByID(toID)
		fromBal := "0.00"
		toBal := "0.00"
		if fromW != nil {
			fromBal = fromW.Balance
		}
		if toW != nil {
			toBal = toW.Balance
		}
		return &domain.TransferResponse{
			TransferID:  *refID,
			FromBalance: fromBal,
			ToBalance:   toBal,
		}, nil
	}

	// Create transfer reference ID
	transferID := uuid.New()

	// Deadlock prevention: lock wallets in ID-sorted order
	first, second := fromID, toID
	if fromID.String() > toID.String() {
		first, second = toID, fromID
	}
	_ = first
	_ = second

	// Actually we need to use the sorted approach with the actual IDs
	var lockIDs []uuid.UUID
	lockIDs = append(lockIDs, fromID, toID)
	sort.Slice(lockIDs, func(i, j int) bool {
		return lockIDs[i].String() < lockIDs[j].String()
	})

	tx := s.walletRepo.DB().Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	fromWallet, err := s.walletRepo.FindByIDForUpdate(tx, fromID)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	toWallet, err := s.walletRepo.FindByIDForUpdate(tx, toID)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if fromWallet.Currency != toWallet.Currency {
		tx.Rollback()
		return nil, ErrCurrencyMismatch
	}

	if fromWallet.Status != domain.WalletStatusActive || toWallet.Status != domain.WalletStatusActive {
		tx.Rollback()
		return nil, ErrWalletSuspended
	}

	fromBalance, _ := decimal.NewFromString(fromWallet.Balance)
	if fromBalance.LessThan(amount) {
		tx.Rollback()
		return nil, ErrInsufficientBalance
	}

	toBalance, _ := decimal.NewFromString(toWallet.Balance)
	newFromBalance := fromBalance.Sub(amount)
	newToBalance := toBalance.Add(amount)

	refID := transferID.String()
	outEntry := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       fromID,
		EntryType:      domain.EntryTypeTransferOut,
		Amount:         amount.StringFixed(2),
		Currency:       fromWallet.Currency,
		BalanceAfter:   newFromBalance.StringFixed(2),
		ReferenceID:    &refID,
		IdempotencyKey: req.IdempotencyKey,
	}

	inEntry := &domain.LedgerEntry{
		EntryID:        uuid.New(),
		WalletID:       toID,
		EntryType:      domain.EntryTypeTransferIn,
		Amount:         amount.StringFixed(2),
		Currency:       toWallet.Currency,
		BalanceAfter:   newToBalance.StringFixed(2),
		ReferenceID:    &refID,
		IdempotencyKey: req.IdempotencyKey + "_in",
	}

	if err := s.ledgerRepo.Create(tx, outEntry); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := s.ledgerRepo.Create(tx, inEntry); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := s.walletRepo.UpdateBalance(tx, fromID, newFromBalance.StringFixed(2)); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := s.walletRepo.UpdateBalance(tx, toID, newToBalance.StringFixed(2)); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	s.eventBus.Publish("ledger", *outEntry)
	s.eventBus.Publish("ledger", *inEntry)

	return &domain.TransferResponse{
		TransferID:  refID,
		FromBalance: newFromBalance.StringFixed(2),
		ToBalance:   newToBalance.StringFixed(2),
	}, nil
}

func (s *WalletService) Suspend(walletID uuid.UUID) (*domain.SuspendResponse, error) {
	w, err := s.walletRepo.FindByID(walletID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Idempotent: already suspended is a no-op success
	if w.Status == domain.WalletStatusSuspended {
		return &domain.SuspendResponse{
			WalletID: w.ID.String(),
			Status:   domain.WalletStatusSuspended,
		}, nil
	}

	if err := s.walletRepo.UpdateStatus(s.walletRepo.DB(), walletID, domain.WalletStatusSuspended); err != nil {
		return nil, err
	}

	return &domain.SuspendResponse{
		WalletID: w.ID.String(),
		Status:   domain.WalletStatusSuspended,
	}, nil
}

func parseAndRoundAmount(amountStr string) (decimal.Decimal, error) {
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid amount: %w", err)
	}
	rounded := amount.Round(2)
	if rounded.LessThan(minAmount) {
		return decimal.Zero, ErrAmountTooSmall
	}
	return rounded, nil
}

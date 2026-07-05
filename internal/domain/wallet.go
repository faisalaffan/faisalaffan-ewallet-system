package domain

import (
	"time"

	"github.com/google/uuid"
)

type WalletStatus string

const (
	WalletStatusActive    WalletStatus = "ACTIVE"
	WalletStatusSuspended WalletStatus = "SUSPENDED"
)

type Wallet struct {
	ID        uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"wallet_id"`
	OwnerID   string       `gorm:"not null;index" json:"owner_id"`
	Currency  string       `gorm:"type:char(3);not null" json:"currency"`
	Balance   string       `gorm:"type:numeric(20,2);not null;default:0.00" json:"balance"`
	Status    WalletStatus `gorm:"type:varchar(20);not null;default:ACTIVE" json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

func (Wallet) TableName() string {
	return "wallets"
}

type CreateWalletRequest struct {
	OwnerID  string `json:"owner_id"`
	Currency string `json:"currency"`
}

type TopUpRequest struct {
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PayRequest struct {
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type TransferRequest struct {
	FromWalletID   string `json:"from_wallet_id"`
	ToWalletID     string `json:"to_wallet_id"`
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type TopUpResponse struct {
	WalletID string `json:"wallet_id"`
	Balance  string `json:"balance"`
	EntryID  string `json:"entry_id"`
}

type PayResponse struct {
	WalletID string `json:"wallet_id"`
	Balance  string `json:"balance"`
	EntryID  string `json:"entry_id"`
}

type TransferResponse struct {
	TransferID  string `json:"transfer_id"`
	FromBalance string `json:"from_balance"`
	ToBalance   string `json:"to_balance"`
}

type SuspendResponse struct {
	WalletID string       `json:"wallet_id"`
	Status   WalletStatus `json:"status"`
}

type ReconcileResponse struct {
	WalletID      string `json:"wallet_id"`
	CachedBalance string `json:"cached_balance"`
	LedgerSum     string `json:"ledger_sum"`
	Match         bool   `json:"match"`
	Diff          string `json:"diff"`
}

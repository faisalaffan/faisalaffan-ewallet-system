package domain

import (
	"time"

	"github.com/google/uuid"
)

type EntryType string

const (
	EntryTypeTopUp       EntryType = "TOPUP"
	EntryTypePayment     EntryType = "PAYMENT"
	EntryTypeTransferIn  EntryType = "TRANSFER_IN"
	EntryTypeTransferOut EntryType = "TRANSFER_OUT"
)

type LedgerEntry struct {
	EntryID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	WalletID       uuid.UUID `gorm:"type:uuid;not null;index:idx_ledger_wallet_created"`
	EntryType      EntryType `gorm:"type:varchar(20);not null"`
	Amount         string    `gorm:"type:numeric(20,2);not null"`
	Currency       string    `gorm:"type:char(3);not null"`
	BalanceAfter   string    `gorm:"type:numeric(20,2);not null"`
	ReferenceID    *string   `gorm:"type:varchar(36)"`
	IdempotencyKey string    `gorm:"type:varchar(64);not null"`
	CreatedAt      time.Time `gorm:"index:idx_ledger_wallet_created"`
}

func (LedgerEntry) TableName() string {
	return "ledger_entries"
}

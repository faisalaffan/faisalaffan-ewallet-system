package database

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

func NewPostgres(dsn string) (*gorm.DB, error) {
	return NewGorm(postgres.Open(dsn))
}

func NewGorm(dialector gorm.Dialector) (*gorm.DB, error) {
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

func AddOtelPlugin(db *gorm.DB) error {
	return db.Use(tracing.NewPlugin())
}

func RunMigrations(db *gorm.DB) error {
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS wallets (
			id         uuid NOT NULL DEFAULT gen_random_uuid(),
			owner_id   varchar(255) NOT NULL,
			currency   char(3) NOT NULL,
			balance    numeric(20,2) NOT NULL DEFAULT 0.00,
			status     varchar(20) NOT NULL DEFAULT 'ACTIVE',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wallets_owner_id ON wallets (owner_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_wallets_owner_currency ON wallets (owner_id, currency)`,
		`CREATE TABLE IF NOT EXISTS ledger_entries (
			entry_id         uuid NOT NULL DEFAULT gen_random_uuid(),
			wallet_id        uuid NOT NULL,
			entry_type       varchar(20) NOT NULL,
			amount           numeric(20,2) NOT NULL,
			currency         char(3) NOT NULL,
			balance_after    numeric(20,2) NOT NULL,
			reference_id     varchar(36),
			idempotency_key  varchar(64) NOT NULL,
			created_at       timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (entry_id),
			CONSTRAINT fk_ledger_wallet FOREIGN KEY (wallet_id) REFERENCES wallets (id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ledger_wallet_created ON ledger_entries (wallet_id, created_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_idempotency_key ON ledger_entries (idempotency_key)`,
	}

	for _, ddl := range ddls {
		if err := db.Exec(ddl).Error; err != nil {
			return err
		}
	}
	return nil
}

CREATE TABLE IF NOT EXISTS wallets (
    id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    owner_id   text        NOT NULL,
    currency   char(3)     NOT NULL,
    balance    numeric(20,2) NOT NULL DEFAULT 0,
    status     varchar(20) NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz,
    updated_at timestamptz,
    PRIMARY KEY (id),
    CONSTRAINT uq_wallets_owner_currency UNIQUE (owner_id, currency)
);

CREATE INDEX idx_wallets_owner_id ON wallets (owner_id);

CREATE TABLE IF NOT EXISTS ledger_entries (
    entry_id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    wallet_id        uuid        NOT NULL,
    entry_type       varchar(20) NOT NULL,
    amount           numeric(20,2) NOT NULL,
    currency         char(3)     NOT NULL,
    balance_after    numeric(20,2) NOT NULL,
    reference_id     varchar(36),
    idempotency_key  varchar(64) NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (entry_id),
    CONSTRAINT fk_ledger_wallet FOREIGN KEY (wallet_id) REFERENCES wallets (id),
    CONSTRAINT uq_idempotency_key UNIQUE (idempotency_key)
);

CREATE INDEX idx_ledger_wallet_created ON ledger_entries (wallet_id, created_at);

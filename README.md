# Multi-Currency E-Wallet Backend System

Ledger-based e-wallet backend dengan dukungan multi-currency, operasi top-up/payment/transfer, dan audit trail append-only.

## Quick Start

```bash
# Setup
cp .env.example .env
make docker-up          # Start PostgreSQL 16
make run                # Start server on :8080

# Run tests
make test               # All 119 tests
make test-race          # With race detector
make coverage           # Coverage report
```

## Stack

- **Go 1.26** + **Fiber v3** (HTTP framework)
- **GORM** + **PostgreSQL 16** (database)
- **shopspring/decimal** (precision-safe money arithmetic)
- **SQLite :memory:** (test database — zero external deps for tests)

## Architecture

```
cmd/server/main.go         → Entry point, DI wiring, graceful shutdown
internal/
  config/config.go         → Viper env loader
  domain/                  → Wallet & LedgerEntry models, DTOs, type constants
  handler/wallet.go        → HTTP handlers (7 endpoints)
  service/wallet.go        → Business rules + transaction orchestration
  service/reconcile.go     → Balance reconciliation
  repository/wallet.go     → Wallet CRUD + SELECT...FOR UPDATE
  repository/ledger.go     → Ledger append-only + idempotency
  router/router.go         → Route registration
pkg/
  database/postgres.go     → GORM connection factory
  response/response.go     → JSON response envelope (JSend)
  decimal/decimal.go       → shopspring/decimal wrappers
migrations/schema.sql      → DDL
```

Layered: **Handler → Service → Repository**. All balance-changing operations execute within a single DB transaction.

## API Reference

All `amount`/`balance` fields are **strings** in JSON to avoid floating-point precision loss.

### Create Wallet
```bash
curl -X POST http://localhost:8080/api/wallets \
  -H "Content-Type: application/json" \
  -d '{"owner_id":"user1","currency":"USD"}'
# 201 { "status":"success", "data": { "wallet_id":"...", "balance":"0.00", "status":"ACTIVE" } }
```

### Top-Up
```bash
curl -X POST http://localhost:8080/api/wallets/{id}/topup \
  -H "Content-Type: application/json" \
  -d '{"amount":"50.00","idempotency_key":"550e8400-e29b-41d4-a716-446655440000"}'
# 200 { "wallet_id":"...", "balance":"50.00", "entry_id":"..." }
```

### Payment
```bash
curl -X POST http://localhost:8080/api/wallets/{id}/pay \
  -H "Content-Type: application/json" \
  -d '{"amount":"12.50","idempotency_key":"660e8400-e29b-41d4-a716-446655440001"}'
# 200, 422 (insufficient balance), 409 (suspended)
```

### Transfer
```bash
curl -X POST http://localhost:8080/api/wallets/transfer \
  -H "Content-Type: application/json" \
  -d '{"from_wallet_id":"...","to_wallet_id":"...","amount":"25.00","idempotency_key":"770e8400-e29b-41d4-a716-446655440002"}'
# 200 { "transfer_id":"...", "from_balance":"25.00", "to_balance":"75.00" }
# 400 (currency mismatch), 422 (insufficient balance), 409 (suspended)
```

### Suspend Wallet
```bash
curl -X POST http://localhost:8080/api/wallets/{id}/suspend
# 200 { "wallet_id":"...", "status":"SUSPENDED" }
# Idempotent: wallet already suspended → 200 (no-op)
```

### Get Wallet
```bash
curl http://localhost:8080/api/wallets/{id}
# 200 { "wallet_id":"...", "owner_id":"user1", "currency":"USD", "balance":"50.00", "status":"ACTIVE" }
```

### Reconcile
```bash
curl http://localhost:8080/api/wallets/{id}/reconcile
# 200 { "wallet_id":"...", "cached_balance":"50.00", "ledger_sum":"50.00", "match":true, "diff":"0.00" }
```

## API Contract Summary

| Method | Path | Description | Key Responses |
|---|---|---|---|
| POST | `/api/wallets` | Create wallet | 201, 409 |
| GET | `/api/wallets/:id` | Get wallet | 200, 404 |
| POST | `/api/wallets/:id/topup` | Top-up | 200, 400, 404, 409 |
| POST | `/api/wallets/:id/pay` | Payment | 200, 400, 404, 409, 422 |
| POST | `/api/wallets/transfer` | Transfer | 200, 400, 404, 409, 422 |
| POST | `/api/wallets/:id/suspend` | Suspend (idempotent) | 200, 404 |
| GET | `/api/wallets/:id/reconcile` | Reconcile | 200, 404 |

## Business Rules

| Rule | Description |
|---|---|
| **BR-01** | Amount divalidasi dan dibulatkan (round half up, 2 desimal) sebelum operasi. Amount < 0.01 setelah rounding ditolak. |
| **BR-02** | Wallet harus ACTIVE untuk top-up, payment, dan transfer. |
| **BR-03** | Payment: `balance - amount >= 0`, dicek dengan row-level pessimistic lock. |
| **BR-04** | Transfer: kedua wallet harus ACTIVE, currency harus sama, source balance cukup. Dua wallet di-lock dalam 1 transaction. |
| **BR-05** | Setiap operasi balance-changing menghasilkan 1 ledger entry (transfer: 2 entry — TRANSFER_OUT + TRANSFER_IN) dalam 1 DB transaction. |
| **BR-06** | Idempotency key wajib — duplicate request mengembalikan hasil yang sama tanpa side effect. |
| **BR-07** | Suspend idempotent — wallet sudah suspended = no-op sukses. |

## Precision & Rounding

- **Tipe data**: `NUMERIC(20,2)` di Postgres, `shopspring/decimal` di application layer.
- **NO float64** untuk uang — semua amount/balance sebagai decimal.
- **Rounding**: round half up ke 2 desimal sebelum validasi minimum amount.
- **Minimum unit**: tolak amount < 0.01 setelah rounding.

## Concurrency & Atomicity

**Pessimistic locking**: `SELECT ... FOR UPDATE` pada row wallet di awal transaction.

1. Begin transaction
2. Lock wallet row (`SELECT ... FOR UPDATE`)
3. Validate (status, balance, currency)
4. Insert ledger entry (idempotency check)
5. Update wallet balance cache
6. Commit (all-or-nothing)

**Transfer deadlock prevention**: wallet di-lock dalam urutan ID ascending untuk menghindari deadlock antara dua transfer berlawanan arah.

**Read-after-write**: karena single primary DB (no read replica), read-after-write consistency didapat gratis.

## Database Schema

### wallets
| Column | Type | Constraints |
|---|---|---|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| owner_id | VARCHAR(255) | NOT NULL, INDEXED |
| currency | CHAR(3) | NOT NULL |
| balance | NUMERIC(20,2) | NOT NULL, DEFAULT 0.00 (cached) |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'ACTIVE' |
| created_at | TIMESTAMPTZ | DEFAULT now() |
| updated_at | TIMESTAMPTZ | DEFAULT now() |

UNIQUE(owner_id, currency) — satu wallet per currency per user.

### ledger_entries
| Column | Type | Constraints |
|---|---|---|
| entry_id | UUID | PK, DEFAULT gen_random_uuid() |
| wallet_id | UUID | FK → wallets |
| entry_type | VARCHAR(20) | NOT NULL (TOPUP/PAYMENT/TRANSFER_IN/TRANSFER_OUT) |
| amount | NUMERIC(20,2) | NOT NULL (selalu positif) |
| currency | CHAR(3) | NOT NULL |
| balance_after | NUMERIC(20,2) | NOT NULL (snapshot) |
| reference_id | VARCHAR(36) | NULLABLE (link TRANSFER_IN ↔ TRANSFER_OUT) |
| idempotency_key | VARCHAR(64) | UNIQUE, NOT NULL |
| created_at | TIMESTAMPTZ | DEFAULT now() |

INDEX(wallet_id, created_at), UNIQUE(idempotency_key).

## Assumptions & Trade-offs

1. **Currency minor unit**: semua currency pakai 2 desimal — tidak general untuk JPY (0 desimal) atau BHD (3 desimal).
2. **No authentication/authorization**: `owner_id` dipercaya dari request. Asumsikan sudah divalidasi di layer atas.
3. **Pessimistic locking**: dipilih atas optimistic locking untuk kesederhanaan. Trade-off: throughput lebih rendah di high concurrency.
4. **Idempotency key**: wajib disediakan client (bukan server-generated). Server tidak bisa dedup by content karena amount yang sama bisa valid dikirim 2x secara sengaja.
5. **Balance as cache**: `balance` column adalah cached value, bukan real-time SUM. Reconciliation endpoint tersedia untuk mendeteksi drift.
6. **No read replica**: read-after-write consistency diasumsikan karena single primary DB. Jika ada read replica, perlu strategi read-your-write terpisah.
7. **No FX rate / currency conversion**: transfer hanya antar wallet dengan currency yang sama.
8. **No rate limiting**: diluar scope. Bisa ditambahkan via middleware.

## Testing

```bash
make test           # 119 tests, 10 packages
make test-race      # Dengan race detector — clean
make coverage       # Coverage report
```

Test structure:
- **Repository tests**: SQLite `:memory:` — real GORM queries
- **Service tests**: mock repositories — isolated business logic
- **Handler tests**: Fiber `app.Test()` + mock services — real HTTP request/response
- **Concurrency**: race detector clean (`go test -race`)

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| APP_PORT | 8080 | HTTP listen port |
| DB_HOST | localhost | PostgreSQL host |
| DB_PORT | 5432 | PostgreSQL port |
| DB_USER | postgres | PostgreSQL user |
| DB_PASSWORD | postgres | PostgreSQL password |
| DB_NAME | ewallet | Database name |

## License

MIT

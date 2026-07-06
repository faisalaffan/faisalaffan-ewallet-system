# E-Wallet System

Ledger-based multi-currency e-wallet backend. Go 1.26 + Fiber v3 + GORM + PostgreSQL.

[![CI](https://github.com/faisalaffan/faisalaffan-ewallet-system/actions/workflows/ci.yml/badge.svg)](https://github.com/faisalaffan/faisalaffan-ewallet-system/actions)
[![codecov](https://codecov.io/gh/faisalaffan/faisalaffan-ewallet-system/branch/dev/graph/badge.svg?token=52363029-d10a-4e07-8701-27a8f942d252)](https://codecov.io/gh/faisalaffan/faisalaffan-ewallet-system)

## Run

```bash
cp .env.example .env
make docker-up
make run        # :8080
```

## Test

```bash
make test       # 119 tests, clean
make test-race
make coverage
```

## API

All amounts as strings (decimal). Auth via `Authorization: Bearer <key>` header.

| Method | Path | Notes |
|--------|------|-------|
| POST | `/api/wallets` | Create. 201/409 |
| GET | `/api/wallets/:id` | Get by ID. 200/404 |
| POST | `/api/wallets/:id/topup` | Top-up. Idempotency key required |
| POST | `/api/wallets/:id/pay` | Payment. 422 if insufficient |
| POST | `/api/wallets/transfer` | Transfer. Same currency only |
| POST | `/api/wallets/:id/suspend` | Suspend. Idempotent |
| GET | `/api/wallets/:id/reconcile` | Compare balance vs ledger sum |

## Stack

- **Go 1.26** + **Fiber v3**
- **GORM** + **PostgreSQL 16**
- **shopspring/decimal** for money arithmetic
- **SQLite :memory:** for tests

## Architecture

```
Handler → Service → Repository
```

All balance operations: begin tx → lock row (`SELECT FOR UPDATE`) → validate → ledger entry → update balance → commit.

## Env

| Key | Default |
|-----|---------|
| APP_PORT | 8080 |
| DB_HOST | localhost |
| DB_PORT | 5432 |
| DB_USER | postgres |
| DB_PASSWORD | postgres |
| DB_NAME | ewallet |

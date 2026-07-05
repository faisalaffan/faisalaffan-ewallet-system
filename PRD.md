# PRD: Multi-Currency E-Wallet Backend System

## 1. Konteks & Tujuan

Sistem ledger inti untuk aplikasi fintech yang mendukung wallet multi-currency per user, dengan operasi top-up, payment, transfer, dan audit trail berbasis ledger append-only. Target: correctness (no double-spend, no negative balance, no floating-point error) di atas kecepatan development.

**Bahasa/stack**: Go (pilihan default karena decimal handling lebih eksplisit dan concurrency primitives lebih matang untuk kasus ini). Alternatif Node.js diizinkan tapi harus pakai library decimal (bukan `Number`).

**Non-goals** (di luar scope assignment ini, sebutkan eksplisit di README):
- Auth/authorization (asumsikan owner_id sudah tervalidasi di layer atas)
- Currency conversion / FX rate
- Webhook/notification
- Rate limiting, API gateway concerns

---

## 2. Data Model

### 2.1 Wallet
```
wallet_id     UUID PK
owner_id      string (indexed)
currency      char(3) ISO 4217, e.g. USD, IDR, EUR
balance       NUMERIC(20,2) -- derived/cached, source of truth tetap ledger
status        ENUM(ACTIVE, SUSPENDED)
created_at    timestamptz
updated_at    timestamptz

UNIQUE (owner_id, currency)  -- satu wallet per currency per user
```

**Keputusan desain**: `balance` di kolom wallet adalah **cache**, bukan source of truth. Source of truth = SUM(ledger_entries). Balance di-update dalam transaksi yang sama dengan insert ledger entry, supaya query cepat tanpa perlu SUM tiap request, tapi tetap bisa direkonsiliasi kapan saja terhadap ledger (edge case #9).

### 2.2 Ledger Entry (append-only)
```
entry_id        UUID PK
wallet_id       FK -> Wallet
entry_type      ENUM(TOPUP, PAYMENT, TRANSFER_IN, TRANSFER_OUT)
amount          NUMERIC(20,2)  -- selalu positif, arah ditentukan entry_type
currency        char(3)
balance_after   NUMERIC(20,2)  -- snapshot balance setelah entry ini (audit trail lebih kuat)
reference_id    string, nullable  -- link ke transfer_id untuk pasangan TRANSFER_IN/OUT
idempotency_key string, UNIQUE   -- untuk edge case #6 (duplicate request)
created_at      timestamptz

INDEX (wallet_id, created_at)
UNIQUE (idempotency_key)
```

**Keputusan desain**: Setiap transfer menghasilkan **2 ledger entries** (TRANSFER_OUT di wallet pengirim, TRANSFER_IN di wallet penerima) dengan `reference_id` yang sama, di-insert dalam **satu DB transaction** — bukan 1 entry yang "menunjuk ke 2 wallet". Ini menyederhanakan invariant "balance wallet = SUM(ledger entries milik wallet itu)".

---

## 3. Precision & Rounding (kritis)

- Tipe data: `NUMERIC(20,2)` di Postgres, atau `decimal.Decimal` (shopspring/decimal di Go) di application layer. **Jangan pernah pakai float64/Number untuk uang.**
- Skala: 2 desimal (asumsi: semua currency di sistem ini pakai minor unit 2 desimal — USD, IDR, EUR. **Assumption eksplisit**: sistem TIDAK menghandle currency dengan minor unit berbeda seperti JPY (0 desimal) atau BHD (3 desimal). Kalau mau general, perlu tabel currency metadata `{code, decimal_places}`.
- Rounding rule: **round half up** ke 2 desimal pada saat input diterima (contoh assignment: 12.345 → 12.35). Rounding terjadi SEBELUM validasi minimum amount, bukan sesudah.
- Minimum unit: tolak amount < 0.01 setelah rounding (edge case: payment 0.001 → reject, karena setelah rounding jadi 0.00 yang juga di-reject oleh rule "amount > 0").

---

## 4. Operasi & Business Rules

| Operasi | Rule |
|---|---|
| Create Wallet | Reject jika (owner_id, currency) sudah ada. Balance awal = 0.00, status ACTIVE. |
| Top-up | amount > 0 (setelah rounding). Wallet harus ACTIVE. Insert ledger TOPUP + update balance dalam 1 transaction. |
| Payment | amount > 0. Wallet ACTIVE. **balance - amount >= 0**, dicek dengan row-level lock (lihat §5). Insert ledger PAYMENT. |
| Transfer | Kedua wallet harus ACTIVE, currency harus sama persis, amount > 0, source balance cukup. Debit source + credit destination dalam **1 DB transaction** — all-or-nothing. |
| Suspend | Set status = SUSPENDED. Operasi lain terhadap wallet ini langsung ditolak setelahnya (top-up/pay/transfer). Idempotent: suspend wallet yg sudah suspended = no-op sukses, bukan error. |
| Query | Baca `balance` cache column. Harus read-after-write consistent (lihat §5). |

**Currency mismatch**: validasi di level aplikasi SEBELUM buka transaction DB, supaya tidak ada lock yang di-hold sia-sia.

**Duplicate/idempotency**: setiap request top-up/pay/transfer wajib membawa `idempotency_key` (client-generated, misal UUID). Server: `INSERT ... ON CONFLICT (idempotency_key) DO NOTHING`, lalu return hasil entry yang sudah ada kalau conflict (bukan error 500). Ini satu-satunya cara reliable menghandle "duplicate request" dan "out-of-order request" (edge case #6, #11) tanpa asumsi network-level dedup.

---

## 5. Concurrency & Atomicity

**Masalah**: dua payment concurrent terhadap wallet yang sama tidak boleh membuat balance negatif (edge case #7 — classic race condition / lost update).

**Solusi (pilih salah satu, harus di-state di README)**:
- **Opsi A (rekomendasi, simpler)**: Pessimistic locking — `SELECT ... FOR UPDATE` pada row wallet di awal transaction sebelum baca balance, cek cukup/tidak, lalu update. Semua operasi yang mengubah balance (topup/pay/transfer) wajib lewat pola ini.
- **Opsi B**: Optimistic locking via `version` column — `UPDATE wallets SET balance=?, version=version+1 WHERE wallet_id=? AND version=?`, retry kalau affected rows = 0. Lebih scalable tapi butuh retry logic di app layer.

Untuk transfer (2 wallet sekaligus): **lock urut berdasarkan wallet_id (misal sort ascending)** untuk menghindari deadlock antara dua transfer yang arahnya berlawanan (A→B dan B→A bersamaan).

**Atomicity**: gunakan DB transaction (`BEGIN...COMMIT`) yang membungkus: lock row → validasi → insert ledger entry(ies) → update balance cache → commit. Kalau salah satu gagal, seluruh transaction rollback (menjawab edge case #8 dan #13 — partial failure/crash recovery otomatis ter-handle oleh DB transaction guarantee, TIDAK perlu manual compensating logic selama semuanya dalam 1 transaction DB).

**Read-after-write**: karena semua write dan read hit database yang sama (bukan read-replica dengan lag), read-after-write consistency didapat gratis. **Assumption**: tidak ada read replica di scope ini. Kalau ada, perlu strategi read-your-write terpisah (route ke primary, atau session-based causal consistency) — sebutkan sebagai known limitation.

---

## 6. API Contract

```
POST /wallets
  body: { owner_id, currency }
  201 -> { wallet_id, owner_id, currency, balance: "0.00", status: "ACTIVE" }
  409 -> wallet already exists for this owner+currency

POST /wallets/{id}/topup
  body: { amount: "12.50", idempotency_key }
  200 -> { wallet_id, balance, entry_id }
  400 -> invalid amount / currency issue
  404 -> wallet not found
  409 -> wallet suspended
  200 (idempotent replay) -> hasil entry yang sama, bukan double top-up

POST /wallets/{id}/pay
  body: { amount, idempotency_key }
  200/400/404/409 -> sama pola di atas
  422 -> insufficient balance

POST /wallets/transfer
  body: { from_wallet_id, to_wallet_id, amount, idempotency_key }
  200 -> { transfer_id, from_balance, to_balance }
  400 -> currency mismatch
  404 -> salah satu wallet tidak ada
  409 -> salah satu wallet suspended
  422 -> insufficient balance

POST /wallets/{id}/suspend
  200 -> { wallet_id, status: "SUSPENDED" }  (idempotent)

GET /wallets/{id}
  200 -> { wallet_id, owner_id, currency, balance, status }
```

Semua field `amount`/`balance` di JSON dikirim sebagai **string**, bukan number, untuk menghindari precision loss saat parsing di client (edge case implicit tapi penting untuk fintech API).

---

## 7. Reconciliation & Audit

Endpoint/job tambahan (nice-to-have, sebutkan di README kalau tidak diimplementasi karena scope): `GET /wallets/{id}/reconcile` — hitung `SUM(ledger_entries)` dan bandingkan dengan `balance` cache column, return mismatch kalau ada. Ini adalah jawaban konkret untuk edge case #9, bukan sekadar disain — kalau tidak sempat implement, cukup dokumentasikan sebagai known gap.

---

## 8. Deliverables Checklist (mapping ke assignment)

- [ ] Wallet CRUD (create, get, suspend) sesuai API contract
- [ ] Ledger append-only, 1 transaction DB per operasi balance-changing
- [ ] Decimal type (shopspring/decimal atau setara), no float
- [ ] Row-level locking untuk concurrency safety
- [ ] Idempotency key handling di semua write endpoint
- [ ] Currency mismatch validation
- [ ] Unit test: rounding, negative-balance rejection, currency mismatch, idempotent replay, concurrent payment (goroutine test dengan race detector `go test -race`)
- [ ] README: cara run (docker-compose/postgres), assumptions section (currency minor unit, no auth, no FX), API examples dengan curl

---

## 9. Assumptions & Trade-offs (wajib ditulis eksplisit di README hasil implementasi)

1. Semua currency pakai 2 desimal minor unit — tidak general untuk JPY/BHD.
2. Tidak ada authentication/authorization; owner_id dipercaya dari request.
3. Idempotency key wajib disediakan client, bukan di-generate server (server tidak bisa dedup by content karena amount yang sama valid untuk dikirim 2x secara sengaja).
4. Pessimistic row lock dipilih atas optimistic locking untuk kesederhanaan, trade-off: throughput lebih rendah di high concurrency dibanding optimistic + retry.
5. Balance adalah cached column, bukan real-time SUM — trade-off: butuh mekanisme reconciliation berkala untuk mendeteksi drift kalau ada bug.
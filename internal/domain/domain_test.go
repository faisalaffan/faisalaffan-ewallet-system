package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletStruct(t *testing.T) {
	id := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)

	w := Wallet{
		ID:        id,
		OwnerID:   "user-001",
		Currency:  "USD",
		Balance:   "150.75",
		Status:    WalletStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	assert.Equal(t, id, w.ID)
	assert.Equal(t, "user-001", w.OwnerID)
	assert.Equal(t, "USD", w.Currency)
	assert.Equal(t, "150.75", w.Balance)
	assert.Equal(t, WalletStatusActive, w.Status)
	assert.Equal(t, now, w.CreatedAt)
	assert.Equal(t, now, w.UpdatedAt)
	assert.Equal(t, "wallets", w.TableName())
}

func TestLedgerEntryStruct(t *testing.T) {
	entryID := uuid.MustParse("b2c3d4e5-f6a7-8901-bcde-f12345678901")
	walletID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	refID := "transfer-ref-001"
	now := time.Date(2026, 7, 5, 10, 30, 0, 0, time.UTC)

	e := LedgerEntry{
		EntryID:        entryID,
		WalletID:       walletID,
		EntryType:      EntryTypeTopUp,
		Amount:         "500.00",
		Currency:       "USD",
		BalanceAfter:   "500.00",
		ReferenceID:    &refID,
		IdempotencyKey: "idem-001",
		CreatedAt:      now,
	}

	assert.Equal(t, entryID, e.EntryID)
	assert.Equal(t, walletID, e.WalletID)
	assert.Equal(t, EntryTypeTopUp, e.EntryType)
	assert.Equal(t, "500.00", e.Amount)
	assert.Equal(t, "USD", e.Currency)
	assert.Equal(t, "500.00", e.BalanceAfter)
	assert.NotNil(t, e.ReferenceID)
	assert.Equal(t, "transfer-ref-001", *e.ReferenceID)
	assert.Equal(t, "idem-001", e.IdempotencyKey)
	assert.Equal(t, now, e.CreatedAt)
	assert.Equal(t, "ledger_entries", e.TableName())
}

func TestLedgerEntryStruct_NilReferenceID(t *testing.T) {
	e := LedgerEntry{
		EntryID:   uuid.New(),
		WalletID:  uuid.New(),
		EntryType: EntryTypePayment,
		Amount:    "100.00",
		Currency:  "USD",
		BalanceAfter: "900.00",
		IdempotencyKey: "idem-002",
	}

	assert.Nil(t, e.ReferenceID)
}

func TestWalletStatus(t *testing.T) {
	assert.Equal(t, WalletStatus("ACTIVE"), WalletStatusActive)
	assert.Equal(t, WalletStatus("SUSPENDED"), WalletStatusSuspended)
	assert.NotEqual(t, WalletStatusActive, WalletStatusSuspended)
}

func TestEntryType(t *testing.T) {
	assert.Equal(t, EntryType("TOPUP"), EntryTypeTopUp)
	assert.Equal(t, EntryType("PAYMENT"), EntryTypePayment)
	assert.Equal(t, EntryType("TRANSFER_IN"), EntryTypeTransferIn)
	assert.Equal(t, EntryType("TRANSFER_OUT"), EntryTypeTransferOut)

	// Verify all four are distinct.
	types := []EntryType{EntryTypeTopUp, EntryTypePayment, EntryTypeTransferIn, EntryTypeTransferOut}
	seen := make(map[EntryType]bool)
	for _, et := range types {
		assert.False(t, seen[et], "duplicate entry type: %s", et)
		seen[et] = true
	}
}

func TestCreateWalletRequest_JSONTags(t *testing.T) {
	req := CreateWalletRequest{OwnerID: "u1", Currency: "IDR"}
	b, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "u1", decoded["owner_id"])
	assert.Equal(t, "IDR", decoded["currency"])

	// Verify JSON field names (snake_case).
	assert.Contains(t, string(b), "owner_id")
	assert.Contains(t, string(b), "currency")
}

func TestTopUpRequest_Fields(t *testing.T) {
	req := TopUpRequest{Amount: "250.00", IdempotencyKey: "topup-idem-01"}
	assert.Equal(t, "250.00", req.Amount)
	assert.Equal(t, "topup-idem-01", req.IdempotencyKey)

	// JSON round-trip
	b, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "250.00", decoded["amount"])
	assert.Equal(t, "topup-idem-01", decoded["idempotency_key"])
}

func TestTransferRequest_Fields(t *testing.T) {
	req := TransferRequest{
		FromWalletID:   "wallet-a",
		ToWalletID:     "wallet-b",
		Amount:         "100.00",
		IdempotencyKey: "transfer-idem-01",
	}
	assert.Equal(t, "wallet-a", req.FromWalletID)
	assert.Equal(t, "wallet-b", req.ToWalletID)
	assert.Equal(t, "100.00", req.Amount)
	assert.Equal(t, "transfer-idem-01", req.IdempotencyKey)

	// JSON round-trip
	b, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "wallet-a", decoded["from_wallet_id"])
	assert.Equal(t, "wallet-b", decoded["to_wallet_id"])
	assert.Equal(t, "100.00", decoded["amount"])
	assert.Equal(t, "transfer-idem-01", decoded["idempotency_key"])
}

func TestUUIDGeneration(t *testing.T) {
	u1 := uuid.New()
	u2 := uuid.New()

	assert.NotEqual(t, u1, u2)
	assert.Equal(t, 16, len(u1))
	assert.Equal(t, 16, len(u2))

	// Both should parse back to their string representations.
	parsed1, err := uuid.Parse(u1.String())
	require.NoError(t, err)
	assert.Equal(t, u1, parsed1)

	parsed2, err := uuid.Parse(u2.String())
	require.NoError(t, err)
	assert.Equal(t, u2, parsed2)
}

func TestWalletStruct_JSONSerialization(t *testing.T) {
	w := Wallet{
		ID:       uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890"),
		OwnerID:  "user-001",
		Currency: "USD",
		Balance:  "150.75",
		Status:   WalletStatusActive,
	}

	b, err := json.Marshal(w)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "a1b2c3d4-e5f6-7890-abcd-ef1234567890", decoded["wallet_id"])
	assert.Equal(t, "user-001", decoded["owner_id"])
	assert.Equal(t, "USD", decoded["currency"])
	assert.Equal(t, "150.75", decoded["balance"])
	assert.Equal(t, "ACTIVE", decoded["status"])
}

func TestResponseStructs(t *testing.T) {
	// TopUpResponse
	topUpResp := TopUpResponse{
		WalletID: "wallet-id-1",
		Balance:  "500.00",
		EntryID:  "entry-id-1",
	}
	b, err := json.Marshal(topUpResp)
	require.NoError(t, err)
	assert.Contains(t, string(b), "wallet_id")
	assert.Contains(t, string(b), "entry_id")

	// PayResponse
	payResp := PayResponse{
		WalletID: "wallet-id-2",
		Balance:  "400.00",
		EntryID:  "entry-id-2",
	}
	b, err = json.Marshal(payResp)
	require.NoError(t, err)
	assert.Contains(t, string(b), "wallet_id")
	assert.Contains(t, string(b), "entry_id")

	// TransferResponse
	transferResp := TransferResponse{
		TransferID:  "transfer-id-1",
		FromBalance: "300.00",
		ToBalance:   "700.00",
	}
	b, err = json.Marshal(transferResp)
	require.NoError(t, err)
	assert.Contains(t, string(b), "transfer_id")
	assert.Contains(t, string(b), "from_balance")
	assert.Contains(t, string(b), "to_balance")

	// SuspendResponse
	suspendResp := SuspendResponse{
		WalletID: "wallet-id-3",
		Status:   WalletStatusSuspended,
	}
	b, err = json.Marshal(suspendResp)
	require.NoError(t, err)
	assert.Contains(t, string(b), "wallet_id")
	assert.Contains(t, string(b), "SUSPENDED")

	// ReconcileResponse
	reconcileResp := ReconcileResponse{
		WalletID:      "wallet-id-4",
		CachedBalance: "1000.00",
		LedgerSum:     "1000.00",
		Match:         true,
		Diff:          "0.00",
	}
	b, err = json.Marshal(reconcileResp)
	require.NoError(t, err)
	assert.Contains(t, string(b), "cached_balance")
	assert.Contains(t, string(b), "ledger_sum")
	assert.Contains(t, string(b), "match")
	assert.Contains(t, string(b), "diff")
}

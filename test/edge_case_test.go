package test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 1. Decimal Precision
// ---------------------------------------------------------------------------

func TestEdge_Decimal_RoundUp(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-decimal-round", "USD")

	// 12.345 → rounds to 12.35
	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "12.345",
		"idempotency_key": testOwner + "-edge-decimal-1",
	})
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "12.35", data["balance"])
}

func TestEdge_Decimal_RoundDown(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-decimal-rounddown", "USD")

	// 12.334 → rounds to 12.33
	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "12.334",
		"idempotency_key": testOwner + "-edge-decimal-2",
	})
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "12.33", data["balance"])
}

func TestEdge_Decimal_RejectTooSmall(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-decimal-small", "USD")

	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "0.001",
		"idempotency_key": testOwner + "-edge-decimal-3",
	})
	assert.Equal(t, 400, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "AMOUNT_TOO_SMALL", errObj["code"])
}

// ---------------------------------------------------------------------------
// 2. Large Balances
// ---------------------------------------------------------------------------

func TestEdge_LargeBalance(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-large", "USD")

	// 1 billion
	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "1000000000.00",
		"idempotency_key": testOwner + "-edge-large-1",
	})
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "1000000000.00", data["balance"])

	// pay small amount from large balance
	resp = doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "1.00",
		"idempotency_key": testOwner + "-edge-large-2",
	})
	require.Equal(t, 200, resp.StatusCode)
	data = decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "999999999.00", data["balance"])
}

// ---------------------------------------------------------------------------
// 5. Zero or Negative Amounts
// ---------------------------------------------------------------------------

func TestEdge_ZeroAmount(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-zero", "USD")

	// topup 0.00
	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "0.00",
		"idempotency_key": testOwner + "-edge-zero-1",
	})
	assert.Equal(t, 400, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "AMOUNT_TOO_SMALL", errObj["code"])

	// pay 0.00
	resp = doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "0.00",
		"idempotency_key": testOwner + "-edge-zero-2",
	})
	assert.Equal(t, 400, resp.StatusCode)
	body, _ = getBody(t, resp)
	errObj = body["error"].(map[string]any)
	assert.Equal(t, "AMOUNT_TOO_SMALL", errObj["code"])
}

func TestEdge_NegativeAmount(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-neg", "USD")

	// topup -50.00
	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "-50.00",
		"idempotency_key": testOwner + "-edge-neg-1",
	})
	assert.Equal(t, 400, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "AMOUNT_TOO_SMALL", errObj["code"])

	// pay -50.00
	resp = doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "-50.00",
		"idempotency_key": testOwner + "-edge-neg-2",
	})
	assert.Equal(t, 400, resp.StatusCode)
	body, _ = getBody(t, resp)
	errObj = body["error"].(map[string]any)
	assert.Equal(t, "AMOUNT_TOO_SMALL", errObj["code"])
}

// ---------------------------------------------------------------------------
// 4. Multiple Wallets Per User (one wallet per currency)
// ---------------------------------------------------------------------------

func TestEdge_MultipleCurrenciesPerUser(t *testing.T) {
	owner := testOwner + "-multi"

	// create USD wallet
	usdID := createWallet(t, owner, "USD")

	// create IDR wallet — same owner, different currency
	idrID := createWallet(t, owner, "IDR")

	// verify both exist with correct owner
	resp := doRequest(t, http.MethodGet, "/wallets/"+usdID, nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, owner, data["owner_id"])
	assert.Equal(t, "USD", data["currency"])

	resp = doRequest(t, http.MethodGet, "/wallets/"+idrID, nil)
	require.Equal(t, 200, resp.StatusCode)
	data = decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, owner, data["owner_id"])
	assert.Equal(t, "IDR", data["currency"])

	// duplicate: same owner + same currency = 409
	resp = doRequest(t, http.MethodPost, "/wallets", map[string]string{
		"owner_id": owner,
		"currency": "USD",
	})
	assert.Equal(t, 409, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "WALLET_EXISTS", errObj["code"])
}

// ---------------------------------------------------------------------------
// 8. Partial Failure During Transfer (atomicity)
// ---------------------------------------------------------------------------

func TestEdge_Transfer_Atomicity(t *testing.T) {
	from := createWallet(t, testOwner+"-edge-atomic-from", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+from+"/topup", map[string]string{
		"amount":          "500.00",
		"idempotency_key": testOwner + "-edge-atomic-topup",
	})

	// transfer to non-existent wallet — must fail atomically
	// from balance must NOT be deducted
	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  from,
		"to_wallet_id":    uuid.New().String(),
		"amount":          "100.00",
		"idempotency_key": testOwner + "-edge-atomic-1",
	})
	assert.Equal(t, 404, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "NOT_FOUND", errObj["code"])

	// verify from balance untouched
	resp = doRequest(t, http.MethodGet, "/wallets/"+from, nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "500.00", data["balance"])
}

// ---------------------------------------------------------------------------
// 9. Ledger vs Balance Mismatch
// ---------------------------------------------------------------------------

func TestEdge_LedgerBalanceMismatch(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-mismatch", "USD")

	// initial reconcile — empty ledger, balance 0
	resp := doRequest(t, http.MethodGet, "/wallets/"+walletID+"/reconcile", nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, true, data["match"])
	assert.Equal(t, "0.00", data["diff"])

	// topup + pay — verify match after operations
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "250.50",
		"idempotency_key": testOwner + "-edge-mismatch-topup",
	})
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "75.25",
		"idempotency_key": testOwner + "-edge-mismatch-pay",
	})

	resp = doRequest(t, http.MethodGet, "/wallets/"+walletID+"/reconcile", nil)
	require.Equal(t, 200, resp.StatusCode)
	data = decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, true, data["match"])
	assert.Equal(t, "0.00", data["diff"])
	assert.Equal(t, "175.25", data["cached_balance"])
	assert.Equal(t, "175.25", data["ledger_sum"])
}

// ---------------------------------------------------------------------------
// 10. Suspended Wallet Operations
// ---------------------------------------------------------------------------

func TestEdge_Suspended_CannotTopUp(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-sus-topup", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/suspend", nil)

	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "50.00",
		"idempotency_key": testOwner + "-edge-sus-topup-1",
	})
	assert.Equal(t, 409, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "WALLET_SUSPENDED", errObj["code"])
}

func TestEdge_Suspended_CannotPay(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-sus-pay", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "100.00",
		"idempotency_key": testOwner + "-edge-sus-pay-topup",
	})
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/suspend", nil)

	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "50.00",
		"idempotency_key": testOwner + "-edge-sus-pay-1",
	})
	assert.Equal(t, 409, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "WALLET_SUSPENDED", errObj["code"])
}

func TestEdge_Suspended_CannotTransferOut(t *testing.T) {
	from := createWallet(t, testOwner+"-edge-sus-tr-from", "USD")
	to := createWallet(t, testOwner+"-edge-sus-tr-to", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+from+"/topup", map[string]string{
		"amount":          "500.00",
		"idempotency_key": testOwner + "-edge-sus-tr-topup",
	})
	doRequest(t, http.MethodPost, "/wallets/"+from+"/suspend", nil)

	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  from,
		"to_wallet_id":    to,
		"amount":          "100.00",
		"idempotency_key": testOwner + "-edge-sus-tr-1",
	})
	assert.Equal(t, 409, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "WALLET_SUSPENDED", errObj["code"])
}

// ---------------------------------------------------------------------------
// 6. Duplicate Requests (idempotency)
// ---------------------------------------------------------------------------

func TestEdge_Idempotency_TopUp(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-idem-topup", "USD")
	req := map[string]string{"amount": "100.00", "idempotency_key": testOwner + "-edge-idem-1"}

	r1 := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", req)
	require.Equal(t, 200, r1.StatusCode)
	d1 := decodeBody(t, r1)["data"].(map[string]any)
	entryID := d1["entry_id"].(string)

	// retry — same entry_id, balance still 100.00 (not 200.00)
	r2 := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", req)
	require.Equal(t, 200, r2.StatusCode)
	d2 := decodeBody(t, r2)["data"].(map[string]any)
	assert.Equal(t, entryID, d2["entry_id"])
	assert.Equal(t, "100.00", d2["balance"])
}

func TestEdge_Idempotency_Pay(t *testing.T) {
	walletID := createWallet(t, testOwner+"-edge-idem-pay", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "200.00",
		"idempotency_key": testOwner + "-edge-idem-pay-topup",
	})

	req := map[string]string{"amount": "50.00", "idempotency_key": testOwner + "-edge-idem-pay-1"}

	r1 := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", req)
	require.Equal(t, 200, r1.StatusCode)
	d1 := decodeBody(t, r1)["data"].(map[string]any)
	entryID := d1["entry_id"].(string)

	// retry — same entry_id, balance still 150.00 (not 100.00)
	r2 := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", req)
	require.Equal(t, 200, r2.StatusCode)
	d2 := decodeBody(t, r2)["data"].(map[string]any)
	assert.Equal(t, entryID, d2["entry_id"])
	assert.Equal(t, "150.00", d2["balance"])
}

// ---------------------------------------------------------------------------
// 12. Read-After-Write Consistency
// ---------------------------------------------------------------------------

func TestEdge_ReadAfterWrite(t *testing.T) {
	owner := testOwner + "-raw"

	// create
	resp := doRequest(t, http.MethodPost, "/wallets", map[string]string{
		"owner_id": owner,
		"currency": "EUR",
	})
	require.Equal(t, 201, resp.StatusCode)
	walletID := decodeBody(t, resp)["data"].(map[string]any)["wallet_id"].(string)

	// read immediately — must see same data
	resp = doRequest(t, http.MethodGet, "/wallets/"+walletID, nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, owner, data["owner_id"])
	assert.Equal(t, "EUR", data["currency"])
	assert.Equal(t, "0.00", data["balance"])

	// topup → read immediately
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "77.77",
		"idempotency_key": testOwner + "-edge-raw-1",
	})
	resp = doRequest(t, http.MethodGet, "/wallets/"+walletID, nil)
	require.Equal(t, 200, resp.StatusCode)
	data = decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "77.77", data["balance"])

	// pay → read immediately
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "33.33",
		"idempotency_key": testOwner + "-edge-raw-2",
	})
	resp = doRequest(t, http.MethodGet, "/wallets/"+walletID, nil)
	require.Equal(t, 200, resp.StatusCode)
	data = decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "44.44", data["balance"])
}

